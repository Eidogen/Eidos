package aggregator

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// DepthPublisher 深度发布接口
type DepthPublisher interface {
	PublishDepth(ctx context.Context, market string, depth *model.Depth) error
}

// DepthSnapshotProvider 深度快照提供者（来自 eidos-matching）
// 实现: eidos-market/internal/client/matching_client.go
// 用途: 当检测到序列号缺口时，调用 eidos-matching 的 GetOrderbook gRPC 接口获取全量快照
type DepthSnapshotProvider interface {
	GetSnapshot(ctx context.Context, market string) (*model.Depth, error)
}

// DepthManagerConfig 深度管理器配置
type DepthManagerConfig struct {
	MaxLevels            int           // 最大档位数
	PublishInterval      time.Duration // 发布间隔
	SnapshotSyncInterval time.Duration // 全量同步间隔（检测序列号缺口时触发）
}

// DefaultDepthManagerConfig 默认配置
func DefaultDepthManagerConfig() DepthManagerConfig {
	return DepthManagerConfig{
		MaxLevels:            100,
		PublishInterval:      100 * time.Millisecond,
		SnapshotSyncInterval: 30 * time.Second,
	}
}

// DepthManager 深度管理器
// 维护订单簿深度，支持增量更新和全量同步
type DepthManager struct {
	market   string
	config   DepthManagerConfig
	bids     map[string]*model.PriceLevel // price_string -> level
	asks     map[string]*model.PriceLevel
	sequence uint64
	mu       sync.RWMutex

	// 排序后的缓存
	sortedBids []*model.PriceLevel
	sortedAsks []*model.PriceLevel
	cacheValid bool

	publisher        DepthPublisher
	snapshotProvider DepthSnapshotProvider
	logger           *slog.Logger

	// 脏标记和发布
	dirty     atomic.Bool
	closeCh   chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	closed    atomic.Bool

	// 统计指标
	updateCount  atomic.Int64
	publishCount atomic.Int64
	syncCount    atomic.Int64
	gapCount     atomic.Int64
}

// NewDepthManager 创建深度管理器
func NewDepthManager(
	market string,
	publisher DepthPublisher,
	snapshotProvider DepthSnapshotProvider,
	logger *slog.Logger,
	config DepthManagerConfig,
) *DepthManager {
	return &DepthManager{
		market:           market,
		config:           config,
		bids:             make(map[string]*model.PriceLevel),
		asks:             make(map[string]*model.PriceLevel),
		publisher:        publisher,
		snapshotProvider: snapshotProvider,
		logger:           logger.With("market", market),
		closeCh:          make(chan struct{}),
	}
}

// Start 启动深度管理器
func (dm *DepthManager) Start() {
	if dm.publisher != nil {
		dm.wg.Add(1)
		go dm.publishLoop()
	}
	dm.logger.Info("depth manager started")
}

// Stop 停止深度管理器
func (dm *DepthManager) Stop() {
	dm.closeOnce.Do(func() {
		dm.closed.Store(true)
		close(dm.closeCh)
		dm.wg.Wait()
		dm.logger.Info("depth manager stopped",
			"total_updates", dm.updateCount.Load(),
			"total_publishes", dm.publishCount.Load(),
			"total_syncs", dm.syncCount.Load(),
			"total_gaps", dm.gapCount.Load())
	})
}

// ApplyUpdate 应用增量更新
func (dm *DepthManager) ApplyUpdate(ctx context.Context, update *model.DepthUpdate) error {
	if dm.closed.Load() {
		return nil
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 序列号检查 - 幂等处理
	if update.Sequence <= dm.sequence {
		return nil
	}

	// 检测序列号缺口
	if dm.sequence > 0 && update.Sequence != dm.sequence+1 {
		dm.gapCount.Add(1)
		metrics.DepthSequenceGaps.WithLabelValues(dm.market).Inc()
		dm.logger.Warn("sequence gap detected",
			"expected", dm.sequence+1,
			"got", update.Sequence)

		// 触发全量同步（通过 DepthSnapshotProvider 从 eidos-matching 获取快照）
		if dm.snapshotProvider != nil {
			go dm.syncSnapshot(ctx)
		}
	}

	// 应用买单更新
	for _, level := range update.Bids {
		key := level.Price.String()
		if level.Amount.IsZero() {
			delete(dm.bids, key)
		} else {
			dm.bids[key] = &model.PriceLevel{
				Price:  level.Price,
				Amount: level.Amount,
			}
		}
	}

	// 应用卖单更新
	for _, level := range update.Asks {
		key := level.Price.String()
		if level.Amount.IsZero() {
			delete(dm.asks, key)
		} else {
			dm.asks[key] = &model.PriceLevel{
				Price:  level.Price,
				Amount: level.Amount,
			}
		}
	}

	dm.sequence = update.Sequence
	dm.cacheValid = false
	dm.dirty.Store(true)
	dm.updateCount.Add(1)

	return nil
}

// ApplySnapshot 应用全量快照
func (dm *DepthManager) ApplySnapshot(snapshot *model.Depth) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 清空现有数据
	dm.bids = make(map[string]*model.PriceLevel)
	dm.asks = make(map[string]*model.PriceLevel)

	// 应用买单
	for _, level := range snapshot.Bids {
		dm.bids[level.Price.String()] = &model.PriceLevel{
			Price:  level.Price,
			Amount: level.Amount,
		}
	}

	// 应用卖单
	for _, level := range snapshot.Asks {
		dm.asks[level.Price.String()] = &model.PriceLevel{
			Price:  level.Price,
			Amount: level.Amount,
		}
	}

	dm.sequence = snapshot.Sequence
	dm.cacheValid = false
	dm.dirty.Store(true)
	dm.syncCount.Add(1)

	dm.logger.Info("snapshot applied",
		"sequence", snapshot.Sequence,
		"bids", len(snapshot.Bids),
		"asks", len(snapshot.Asks))
}

// syncSnapshot 同步全量快照
func (dm *DepthManager) syncSnapshot(ctx context.Context) {
	if dm.snapshotProvider == nil {
		return
	}

	snapshot, err := dm.snapshotProvider.GetSnapshot(ctx, dm.market)
	if err != nil {
		dm.logger.Error("failed to sync snapshot", "error", err)
		metrics.DepthSnapshotRequests.WithLabelValues(dm.market, "error").Inc()
		return
	}

	dm.ApplySnapshot(snapshot)
	metrics.DepthSnapshotRequests.WithLabelValues(dm.market, "success").Inc()
}

// rebuildCache 重建排序缓存
func (dm *DepthManager) rebuildCache() {
	// 重建买单缓存（价格降序）
	dm.sortedBids = make([]*model.PriceLevel, 0, len(dm.bids))
	for _, level := range dm.bids {
		dm.sortedBids = append(dm.sortedBids, level.Clone())
	}
	model.SortBids(dm.sortedBids)

	// 限制档位数
	if len(dm.sortedBids) > dm.config.MaxLevels {
		dm.sortedBids = dm.sortedBids[:dm.config.MaxLevels]
	}

	// 重建卖单缓存（价格升序）
	dm.sortedAsks = make([]*model.PriceLevel, 0, len(dm.asks))
	for _, level := range dm.asks {
		dm.sortedAsks = append(dm.sortedAsks, level.Clone())
	}
	model.SortAsks(dm.sortedAsks)

	// 限制档位数
	if len(dm.sortedAsks) > dm.config.MaxLevels {
		dm.sortedAsks = dm.sortedAsks[:dm.config.MaxLevels]
	}

	dm.cacheValid = true

	// 更新深度档位指标
	metrics.DepthLevels.WithLabelValues(dm.market, "bid").Set(float64(len(dm.sortedBids)))
	metrics.DepthLevels.WithLabelValues(dm.market, "ask").Set(float64(len(dm.sortedAsks)))
}

// GetDepth 获取深度快照
func (dm *DepthManager) GetDepth(limit int) *model.Depth {
	// 先检查并重建缓存（使用写锁）
	dm.ensureCacheValid()

	dm.mu.RLock()
	defer dm.mu.RUnlock()

	depth := &model.Depth{
		Market:    dm.market,
		Sequence:  dm.sequence,
		Timestamp: time.Now().UnixMilli(),
	}

	// 复制买单
	bidCount := len(dm.sortedBids)
	if limit > 0 && bidCount > limit {
		bidCount = limit
	}
	depth.Bids = make([]*model.PriceLevel, bidCount)
	for i := 0; i < bidCount; i++ {
		depth.Bids[i] = dm.sortedBids[i].Clone()
	}

	// 复制卖单
	askCount := len(dm.sortedAsks)
	if limit > 0 && askCount > limit {
		askCount = limit
	}
	depth.Asks = make([]*model.PriceLevel, askCount)
	for i := 0; i < askCount; i++ {
		depth.Asks[i] = dm.sortedAsks[i].Clone()
	}

	return depth
}

// ensureCacheValid 确保缓存有效（使用写锁）
func (dm *DepthManager) ensureCacheValid() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.cacheValid {
		dm.rebuildCache()
	}
}

// GetBestPrices 获取最优买卖价
func (dm *DepthManager) GetBestPrices() (bestBid, bestBidQty, bestAsk, bestAskQty decimal.Decimal) {
	// 先检查并重建缓存（使用写锁）
	dm.ensureCacheValid()

	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if len(dm.sortedBids) > 0 {
		bestBid = dm.sortedBids[0].Price
		bestBidQty = dm.sortedBids[0].Amount
	}

	if len(dm.sortedAsks) > 0 {
		bestAsk = dm.sortedAsks[0].Price
		bestAskQty = dm.sortedAsks[0].Amount
	}

	return
}

// GetSequence 获取当前序列号
func (dm *DepthManager) GetSequence() uint64 {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.sequence
}

// publishLoop 定时发布循环
func (dm *DepthManager) publishLoop() {
	defer dm.wg.Done()

	ticker := time.NewTicker(dm.config.PublishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if dm.dirty.CompareAndSwap(true, false) {
				dm.publish()
			}
		case <-dm.closeCh:
			return
		}
	}
}

// publish 发布深度
func (dm *DepthManager) publish() {
	if dm.publisher == nil {
		return
	}

	depth := dm.GetDepth(dm.config.MaxLevels)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := dm.publisher.PublishDepth(ctx, dm.market, depth); err != nil {
		dm.logger.Warn("failed to publish depth", "error", err)
		metrics.PubSubPublishErrors.WithLabelValues("depth").Inc()
	} else {
		dm.publishCount.Add(1)
		metrics.PubSubPublishTotal.WithLabelValues("depth").Inc()
	}
}

// Stats 返回统计信息
func (dm *DepthManager) Stats() map[string]int64 {
	dm.mu.RLock()
	bidLevels := len(dm.bids)
	askLevels := len(dm.asks)
	dm.mu.RUnlock()

	return map[string]int64{
		"update_count":  dm.updateCount.Load(),
		"publish_count": dm.publishCount.Load(),
		"sync_count":    dm.syncCount.Load(),
		"gap_count":     dm.gapCount.Load(),
		"bid_levels":    int64(bidLevels),
		"ask_levels":    int64(askLevels),
		"sequence":      int64(dm.GetSequence()),
	}
}
