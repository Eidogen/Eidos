// Package snapshot 快照管理器
// 实现两阶段快照机制，确保快照与 Kafka 偏移量的原子性
package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// SnapshotConfig 快照配置
type SnapshotConfig struct {
	Interval  time.Duration // 快照间隔
	MaxCount  int           // 保留的最大快照数量
	TwoPhase  bool          // 是否启用两阶段快照
	KeyPrefix string        // Redis key 前缀
}

// Snapshot 订单簿快照
type Snapshot struct {
	Market       string               `json:"market"`
	Timestamp    int64                `json:"timestamp"`
	KafkaOffsets map[string]int64     `json:"kafka_offsets"` // topic:partition -> offset
	Bids         []SnapshotPriceLevel `json:"bids"`
	Asks         []SnapshotPriceLevel `json:"asks"`
	LastPrice    decimal.Decimal      `json:"last_price"`
	Sequence     int64                `json:"sequence"`
	Checksum     string               `json:"checksum"`
}

// SnapshotPriceLevel 快照价格档位
type SnapshotPriceLevel struct {
	Price  decimal.Decimal `json:"price"`
	Orders []SnapshotOrder `json:"orders"`
}

// SnapshotOrder 快照订单
type SnapshotOrder struct {
	OrderID     string          `json:"order_id"`
	Wallet      string          `json:"wallet"`
	Side        int8            `json:"side"`
	Type        int8            `json:"type"`
	TimeInForce int8            `json:"time_in_force"`
	Price       decimal.Decimal `json:"price"`
	Amount      decimal.Decimal `json:"amount"`
	Remaining   decimal.Decimal `json:"remaining"`
	Timestamp   int64           `json:"timestamp"`
	Sequence    int64           `json:"sequence"`
}

// Manager 快照管理器
type Manager struct {
	redis  redis.UniversalClient
	config *SnapshotConfig
}

// NewManager 创建快照管理器
func NewManager(rdb redis.UniversalClient, cfg *SnapshotConfig) *Manager {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "snapshot"
	}
	if cfg.MaxCount <= 0 {
		cfg.MaxCount = 10
	}

	return &Manager{
		redis:  rdb,
		config: cfg,
	}
}

// SaveSnapshot 保存快照 (两阶段)
// 阶段1: 预写快照到 preparing key
// 阶段2: Kafka offset 提交后，重命名为 latest
func (m *Manager) SaveSnapshot(ctx context.Context, market string, ob *orderbook.OrderBook, offsets map[string]int64) error {
	snapshot := m.createSnapshot(market, ob, offsets)

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	preparingKey := m.preparingKey(market)
	latestKey := m.latestKey(market)
	historyKey := m.historyKey(market, snapshot.Timestamp)

	if m.config.TwoPhase {
		// 两阶段快照

		// 阶段1: 预写快照
		if err := m.redis.Set(ctx, preparingKey, data, time.Hour).Err(); err != nil {
			return fmt.Errorf("save preparing snapshot: %w", err)
		}

		// 返回，等待调用方提交 Kafka offset 后调用 ConfirmSnapshot
		return nil
	}

	// 非两阶段: 直接保存
	pipe := m.redis.Pipeline()
	pipe.Set(ctx, latestKey, data, 0)
	pipe.Set(ctx, historyKey, data, 24*time.Hour) // 历史快照保留24小时
	pipe.ZAdd(ctx, m.historyListKey(market), redis.Z{
		Score:  float64(snapshot.Timestamp),
		Member: historyKey,
	})
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	// 清理旧快照
	m.cleanOldSnapshots(ctx, market)

	return nil
}

// ConfirmSnapshot 确认快照 (两阶段第二步)
// 在 Kafka offset 提交成功后调用
func (m *Manager) ConfirmSnapshot(ctx context.Context, market string) error {
	preparingKey := m.preparingKey(market)
	latestKey := m.latestKey(market)

	// 检查预写快照是否存在
	data, err := m.redis.Get(ctx, preparingKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("preparing snapshot not found")
		}
		return fmt.Errorf("get preparing snapshot: %w", err)
	}

	// 解析快照获取时间戳
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}

	historyKey := m.historyKey(market, snapshot.Timestamp)

	// 原子操作: 重命名并保存历史
	pipe := m.redis.Pipeline()
	pipe.Set(ctx, latestKey, data, 0)
	pipe.Set(ctx, historyKey, data, 24*time.Hour)
	pipe.ZAdd(ctx, m.historyListKey(market), redis.Z{
		Score:  float64(snapshot.Timestamp),
		Member: historyKey,
	})
	pipe.Del(ctx, preparingKey)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("confirm snapshot: %w", err)
	}

	// 清理旧快照
	m.cleanOldSnapshots(ctx, market)

	return nil
}

// LoadSnapshot 加载快照
// 自动处理两阶段快照的不一致状态
func (m *Manager) LoadSnapshot(ctx context.Context, market string, committedOffsets map[string]int64) (*Snapshot, error) {
	preparingKey := m.preparingKey(market)
	latestKey := m.latestKey(market)

	// 检查是否有未完成的预写快照
	preparingData, err := m.redis.Get(ctx, preparingKey).Bytes()
	if err == nil {
		// 存在预写快照，检查 Kafka offset
		var preparingSnapshot Snapshot
		if err := json.Unmarshal(preparingData, &preparingSnapshot); err != nil {
			// 解析失败，删除并使用 latest
			m.redis.Del(ctx, preparingKey)
		} else {
			// 比较 offset
			if m.offsetsCommitted(preparingSnapshot.KafkaOffsets, committedOffsets) {
				// offset 已提交，确认预写快照
				if err := m.ConfirmSnapshot(ctx, market); err == nil {
					return &preparingSnapshot, nil
				}
			} else {
				// offset 未提交，删除预写快照
				m.redis.Del(ctx, preparingKey)
			}
		}
	}

	// 加载 latest 快照
	latestData, err := m.redis.Get(ctx, latestKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("no snapshot found for market: %s", market)
		}
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(latestData, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	// 验证 checksum
	if !m.verifyChecksum(&snapshot) {
		return nil, fmt.Errorf("snapshot checksum mismatch")
	}

	return &snapshot, nil
}

// RestoreOrderBook 从快照恢复订单簿
func (m *Manager) RestoreOrderBook(snapshot *Snapshot) *orderbook.OrderBook {
	ob := orderbook.NewOrderBook(snapshot.Market)

	// 恢复买单
	for _, pl := range snapshot.Bids {
		for _, so := range pl.Orders {
			order := &model.Order{
				OrderID:     so.OrderID,
				Wallet:      so.Wallet,
				Market:      snapshot.Market,
				Side:        model.OrderSide(so.Side),
				Type:        model.OrderType(so.Type),
				TimeInForce: model.TimeInForce(so.TimeInForce),
				Price:       so.Price,
				Amount:      so.Amount,
				Remaining:   so.Remaining,
				Timestamp:   so.Timestamp,
				Sequence:    so.Sequence,
			}
			ob.AddOrder(order)
		}
	}

	// 恢复卖单
	for _, pl := range snapshot.Asks {
		for _, so := range pl.Orders {
			order := &model.Order{
				OrderID:     so.OrderID,
				Wallet:      so.Wallet,
				Market:      snapshot.Market,
				Side:        model.OrderSide(so.Side),
				Type:        model.OrderType(so.Type),
				TimeInForce: model.TimeInForce(so.TimeInForce),
				Price:       so.Price,
				Amount:      so.Amount,
				Remaining:   so.Remaining,
				Timestamp:   so.Timestamp,
				Sequence:    so.Sequence,
			}
			ob.AddOrder(order)
		}
	}

	// 恢复最新价和序列号
	ob.LastPrice = snapshot.LastPrice
	ob.Sequence = snapshot.Sequence

	return ob
}

// ListSnapshots 列出所有快照
func (m *Manager) ListSnapshots(ctx context.Context, market string) ([]SnapshotInfo, error) {
	historyListKey := m.historyListKey(market)

	// 获取所有历史快照 key
	keys, err := m.redis.ZRevRange(ctx, historyListKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	infos := make([]SnapshotInfo, 0, len(keys))
	for _, key := range keys {
		data, err := m.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		infos = append(infos, SnapshotInfo{
			Market:    snapshot.Market,
			Timestamp: snapshot.Timestamp,
			BidLevels: len(snapshot.Bids),
			AskLevels: len(snapshot.Asks),
			LastPrice: snapshot.LastPrice,
			Sequence:  snapshot.Sequence,
		})
	}

	return infos, nil
}

// createSnapshot 创建快照
func (m *Manager) createSnapshot(market string, ob *orderbook.OrderBook, offsets map[string]int64) *Snapshot {
	snapshot := &Snapshot{
		Market:       market,
		Timestamp:    time.Now().UnixNano(),
		KafkaOffsets: offsets,
		Bids:         make([]SnapshotPriceLevel, 0),
		Asks:         make([]SnapshotPriceLevel, 0),
		LastPrice:    ob.LastPrice,
		Sequence:     ob.Sequence,
	}

	// 序列化买单
	ob.Bids.InOrder(func(pl *model.PriceLevel) bool {
		snapshotPL := SnapshotPriceLevel{
			Price:  pl.Price,
			Orders: make([]SnapshotOrder, 0, pl.OrderCount),
		}

		order := pl.Head
		for order != nil {
			snapshotPL.Orders = append(snapshotPL.Orders, SnapshotOrder{
				OrderID:     order.OrderID,
				Wallet:      order.Wallet,
				Side:        int8(order.Side),
				Type:        int8(order.Type),
				TimeInForce: int8(order.TimeInForce),
				Price:       order.Price,
				Amount:      order.Amount,
				Remaining:   order.Remaining,
				Timestamp:   order.Timestamp,
				Sequence:    order.Sequence,
			})
			order = order.Next
		}

		snapshot.Bids = append(snapshot.Bids, snapshotPL)
		return true
	})

	// 序列化卖单
	ob.Asks.InOrder(func(pl *model.PriceLevel) bool {
		snapshotPL := SnapshotPriceLevel{
			Price:  pl.Price,
			Orders: make([]SnapshotOrder, 0, pl.OrderCount),
		}

		order := pl.Head
		for order != nil {
			snapshotPL.Orders = append(snapshotPL.Orders, SnapshotOrder{
				OrderID:     order.OrderID,
				Wallet:      order.Wallet,
				Side:        int8(order.Side),
				Type:        int8(order.Type),
				TimeInForce: int8(order.TimeInForce),
				Price:       order.Price,
				Amount:      order.Amount,
				Remaining:   order.Remaining,
				Timestamp:   order.Timestamp,
				Sequence:    order.Sequence,
			})
			order = order.Next
		}

		snapshot.Asks = append(snapshot.Asks, snapshotPL)
		return true
	})

	// 计算 checksum
	snapshot.Checksum = m.calculateChecksum(snapshot)

	return snapshot
}

// calculateChecksum 计算校验和
func (m *Manager) calculateChecksum(snapshot *Snapshot) string {
	// 使用确定性的序列化方式
	h := sha256.New()

	h.Write([]byte(snapshot.Market))
	h.Write([]byte(fmt.Sprintf("%d", snapshot.Timestamp)))
	h.Write([]byte(snapshot.LastPrice.String()))
	h.Write([]byte(fmt.Sprintf("%d", snapshot.Sequence)))

	// 按顺序处理 offsets
	keys := make([]string, 0, len(snapshot.KafkaOffsets))
	for k := range snapshot.KafkaOffsets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(fmt.Sprintf("%s:%d", k, snapshot.KafkaOffsets[k])))
	}

	// 处理订单
	for _, pl := range snapshot.Bids {
		h.Write([]byte(pl.Price.String()))
		for _, o := range pl.Orders {
			h.Write([]byte(o.OrderID))
			h.Write([]byte(o.Remaining.String()))
		}
	}
	for _, pl := range snapshot.Asks {
		h.Write([]byte(pl.Price.String()))
		for _, o := range pl.Orders {
			h.Write([]byte(o.OrderID))
			h.Write([]byte(o.Remaining.String()))
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// verifyChecksum 验证校验和
func (m *Manager) verifyChecksum(snapshot *Snapshot) bool {
	expectedChecksum := snapshot.Checksum
	snapshot.Checksum = ""
	actualChecksum := m.calculateChecksum(snapshot)
	snapshot.Checksum = expectedChecksum

	return expectedChecksum == actualChecksum
}

// offsetsCommitted 检查 offset 是否已提交
func (m *Manager) offsetsCommitted(snapshotOffsets, committedOffsets map[string]int64) bool {
	for key, snapshotOffset := range snapshotOffsets {
		committedOffset, exists := committedOffsets[key]
		if !exists || committedOffset < snapshotOffset {
			return false
		}
	}
	return true
}

// cleanOldSnapshots 清理旧快照
func (m *Manager) cleanOldSnapshots(ctx context.Context, market string) {
	historyListKey := m.historyListKey(market)

	// 获取需要删除的旧快照
	count, _ := m.redis.ZCard(ctx, historyListKey).Result()
	if count <= int64(m.config.MaxCount) {
		return
	}

	// 删除最旧的快照
	toDelete := count - int64(m.config.MaxCount)
	keys, _ := m.redis.ZRange(ctx, historyListKey, 0, toDelete-1).Result()

	if len(keys) > 0 {
		pipe := m.redis.Pipeline()
		for _, key := range keys {
			pipe.Del(ctx, key)
		}
		pipe.ZRemRangeByRank(ctx, historyListKey, 0, toDelete-1)
		pipe.Exec(ctx)
	}
}

// Key 生成方法
func (m *Manager) preparingKey(market string) string {
	return fmt.Sprintf("%s:%s:preparing", m.config.KeyPrefix, market)
}

func (m *Manager) latestKey(market string) string {
	return fmt.Sprintf("%s:%s:latest", m.config.KeyPrefix, market)
}

func (m *Manager) historyKey(market string, timestamp int64) string {
	return fmt.Sprintf("%s:%s:%d", m.config.KeyPrefix, market, timestamp)
}

func (m *Manager) historyListKey(market string) string {
	return fmt.Sprintf("%s:%s:history", m.config.KeyPrefix, market)
}

// SnapshotInfo 快照信息
type SnapshotInfo struct {
	Market    string          `json:"market"`
	Timestamp int64           `json:"timestamp"`
	BidLevels int             `json:"bid_levels"`
	AskLevels int             `json:"ask_levels"`
	LastPrice decimal.Decimal `json:"last_price"`
	Sequence  int64           `json:"sequence"`
}
