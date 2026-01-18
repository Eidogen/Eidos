// Package snapshot 快照管理器单元测试
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/go-redis/redismock/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	db, _ := redismock.NewClientMock()

	cfg := &SnapshotConfig{
		Interval:  time.Minute,
		MaxCount:  5,
		TwoPhase:  false,
		KeyPrefix: "test-snapshot",
	}

	m := NewManager(db, cfg)

	assert.NotNil(t, m)
	assert.Equal(t, "test-snapshot", m.config.KeyPrefix)
	assert.Equal(t, 5, m.config.MaxCount)
}

func TestNewManager_Defaults(t *testing.T) {
	db, _ := redismock.NewClientMock()

	cfg := &SnapshotConfig{
		Interval: time.Minute,
		// KeyPrefix 和 MaxCount 未设置
	}

	m := NewManager(db, cfg)

	assert.Equal(t, "snapshot", m.config.KeyPrefix) // 默认前缀
	assert.Equal(t, 10, m.config.MaxCount)          // 默认数量
}

func TestManager_CreateSnapshot(t *testing.T) {
	db, _ := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		MaxCount:  10,
	})

	// 创建订单簿
	ob := orderbook.NewOrderBook("BTC-USDC")
	ob.AddOrder(&model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000),
		Amount:      decimal.NewFromFloat(1.5),
		Remaining:   decimal.NewFromFloat(1.5),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	})
	ob.AddOrder(&model.Order{
		OrderID:     "order-2",
		Wallet:      "0x456",
		Market:      "BTC-USDC",
		Side:        model.OrderSideSell,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(51000),
		Amount:      decimal.NewFromFloat(2.0),
		Remaining:   decimal.NewFromFloat(2.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    2,
	})
	ob.LastPrice = decimal.NewFromFloat(50500)
	ob.Sequence = 100

	offsets := map[string]int64{
		"orders:0":          100,
		"cancel-requests:0": 50,
	}

	snapshot := m.createSnapshot("BTC-USDC", ob, offsets)

	assert.Equal(t, "BTC-USDC", snapshot.Market)
	assert.Greater(t, snapshot.Timestamp, int64(0))
	assert.Equal(t, offsets, snapshot.KafkaOffsets)
	assert.Equal(t, 1, len(snapshot.Bids))
	assert.Equal(t, 1, len(snapshot.Asks))
	assert.True(t, decimal.NewFromFloat(50500).Equal(snapshot.LastPrice))
	assert.Equal(t, int64(100), snapshot.Sequence)
	assert.NotEmpty(t, snapshot.Checksum)
}

func TestManager_CalculateChecksum(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids: []SnapshotPriceLevel{
			{
				Price: decimal.NewFromFloat(50000),
				Orders: []SnapshotOrder{
					{OrderID: "order-1", Remaining: decimal.NewFromFloat(1.5)},
				},
			},
		},
		Asks:      []SnapshotPriceLevel{},
		LastPrice: decimal.NewFromFloat(50500),
		Sequence:  100,
	}

	checksum1 := m.calculateChecksum(snapshot)
	checksum2 := m.calculateChecksum(snapshot)

	// 相同快照应产生相同的 checksum
	assert.Equal(t, checksum1, checksum2)

	// 修改后应产生不同的 checksum
	snapshot.Sequence = 101
	checksum3 := m.calculateChecksum(snapshot)
	assert.NotEqual(t, checksum1, checksum3)
}

func TestManager_VerifyChecksum(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.NewFromFloat(50000),
		Sequence:     100,
	}

	// 计算正确的 checksum
	snapshot.Checksum = m.calculateChecksum(snapshot)

	// 验证应通过
	assert.True(t, m.verifyChecksum(snapshot))

	// 篡改数据后验证应失败
	snapshot.Sequence = 999
	assert.False(t, m.verifyChecksum(snapshot))
}

func TestManager_OffsetsCommitted(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	tests := []struct {
		name             string
		snapshotOffsets  map[string]int64
		committedOffsets map[string]int64
		expected         bool
	}{
		{
			name:             "all offsets committed",
			snapshotOffsets:  map[string]int64{"orders:0": 100, "cancels:0": 50},
			committedOffsets: map[string]int64{"orders:0": 100, "cancels:0": 50},
			expected:         true,
		},
		{
			name:             "offsets ahead",
			snapshotOffsets:  map[string]int64{"orders:0": 100},
			committedOffsets: map[string]int64{"orders:0": 150},
			expected:         true,
		},
		{
			name:             "offsets behind",
			snapshotOffsets:  map[string]int64{"orders:0": 100},
			committedOffsets: map[string]int64{"orders:0": 50},
			expected:         false,
		},
		{
			name:             "missing offset",
			snapshotOffsets:  map[string]int64{"orders:0": 100},
			committedOffsets: map[string]int64{},
			expected:         false,
		},
		{
			name:             "empty snapshot offsets",
			snapshotOffsets:  map[string]int64{},
			committedOffsets: map[string]int64{"orders:0": 100},
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.offsetsCommitted(tt.snapshotOffsets, tt.committedOffsets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_KeyGeneration(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snap"})

	assert.Equal(t, "snap:BTC-USDC:preparing", m.preparingKey("BTC-USDC"))
	assert.Equal(t, "snap:BTC-USDC:latest", m.latestKey("BTC-USDC"))
	assert.Equal(t, "snap:BTC-USDC:12345", m.historyKey("BTC-USDC", 12345))
	assert.Equal(t, "snap:BTC-USDC:history", m.historyListKey("BTC-USDC"))
}

func TestManager_RestoreOrderBook(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids: []SnapshotPriceLevel{
			{
				Price: decimal.NewFromFloat(50000),
				Orders: []SnapshotOrder{
					{
						OrderID:     "order-1",
						Wallet:      "0x123",
						Side:        int8(model.OrderSideBuy),
						Type:        int8(model.OrderTypeLimit),
						TimeInForce: int8(model.TimeInForceGTC),
						Price:       decimal.NewFromFloat(50000),
						Amount:      decimal.NewFromFloat(1.5),
						Remaining:   decimal.NewFromFloat(1.0),
						Timestamp:   1704067200000,
						Sequence:    1,
					},
				},
			},
		},
		Asks: []SnapshotPriceLevel{
			{
				Price: decimal.NewFromFloat(51000),
				Orders: []SnapshotOrder{
					{
						OrderID:     "order-2",
						Wallet:      "0x456",
						Side:        int8(model.OrderSideSell),
						Type:        int8(model.OrderTypeLimit),
						TimeInForce: int8(model.TimeInForceGTC),
						Price:       decimal.NewFromFloat(51000),
						Amount:      decimal.NewFromFloat(2.0),
						Remaining:   decimal.NewFromFloat(2.0),
						Timestamp:   1704067200000,
						Sequence:    2,
					},
				},
			},
		},
		LastPrice: decimal.NewFromFloat(50500),
		Sequence:  100,
	}

	ob := m.RestoreOrderBook(snapshot)

	assert.Equal(t, "BTC-USDC", ob.Market)
	assert.True(t, decimal.NewFromFloat(50500).Equal(ob.LastPrice))
	assert.Equal(t, int64(100), ob.Sequence)

	// 验证订单
	order1 := ob.GetOrder("order-1")
	assert.NotNil(t, order1)
	assert.Equal(t, "order-1", order1.OrderID)
	assert.Equal(t, model.OrderSideBuy, order1.Side)
	assert.True(t, decimal.NewFromFloat(1.0).Equal(order1.Remaining))

	order2 := ob.GetOrder("order-2")
	assert.NotNil(t, order2)
	assert.Equal(t, "order-2", order2.OrderID)
	assert.Equal(t, model.OrderSideSell, order2.Side)

	// 验证最佳价格
	bestBid := ob.BestBidPrice()
	bestAsk := ob.BestAskPrice()
	assert.True(t, decimal.NewFromFloat(50000).Equal(bestBid))
	assert.True(t, decimal.NewFromFloat(51000).Equal(bestAsk))
}

func TestManager_SaveSnapshot_TwoPhase(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		MaxCount:  3,
		TwoPhase:  true,
	})

	ob := orderbook.NewOrderBook("BTC-USDC")
	offsets := map[string]int64{"orders:0": 100}

	// Mock 预写快照 - CustomMatch 忽略 value 参数，只验证 key
	mock.CustomMatch(func(expected, actual []interface{}) error {
		// actual 格式: [set, key, value, ex, ttl]
		// 忽略具体的 value 内容
		return nil
	}).ExpectSet("snapshot:BTC-USDC:preparing", "", time.Hour).SetVal("OK")

	err := m.SaveSnapshot(context.Background(), "BTC-USDC", ob, offsets)
	assert.NoError(t, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_SaveSnapshot_TwoPhase_Error(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		TwoPhase:  true,
	})

	ob := orderbook.NewOrderBook("BTC-USDC")
	offsets := map[string]int64{"orders:0": 100}

	// Mock Redis 错误
	mock.CustomMatch(func(expected, actual []interface{}) error {
		return nil
	}).ExpectSet("snapshot:BTC-USDC:preparing", "", time.Hour).SetErr(fmt.Errorf("redis connection error"))

	err := m.SaveSnapshot(context.Background(), "BTC-USDC", ob, offsets)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save preparing snapshot")
}

func TestManager_LoadSnapshot_NotFound(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// Mock: preparing key 不存在
	mock.ExpectGet("snapshot:BTC-USDC:preparing").RedisNil()
	// Mock: latest key 也不存在
	mock.ExpectGet("snapshot:BTC-USDC:latest").RedisNil()

	_, err := m.LoadSnapshot(context.Background(), "BTC-USDC", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no snapshot found")
}

func TestSnapshotOrder_Fields(t *testing.T) {
	so := SnapshotOrder{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Side:        int8(model.OrderSideBuy),
		Type:        int8(model.OrderTypeLimit),
		TimeInForce: int8(model.TimeInForceGTC),
		Price:       decimal.NewFromFloat(50000),
		Amount:      decimal.NewFromFloat(1.5),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   1704067200000,
		Sequence:    1,
	}

	assert.Equal(t, "order-1", so.OrderID)
	assert.Equal(t, int8(model.OrderSideBuy), so.Side)
	assert.True(t, decimal.NewFromFloat(50000).Equal(so.Price))
}

func TestSnapshotInfo_Fields(t *testing.T) {
	info := SnapshotInfo{
		Market:    "BTC-USDC",
		Timestamp: 1704067200000,
		BidLevels: 5,
		AskLevels: 3,
		LastPrice: decimal.NewFromFloat(50000),
		Sequence:  100,
	}

	assert.Equal(t, "BTC-USDC", info.Market)
	assert.Equal(t, int64(1704067200000), info.Timestamp)
	assert.Equal(t, 5, info.BidLevels)
	assert.Equal(t, 3, info.AskLevels)
}

func TestSnapshot_Fields(t *testing.T) {
	snapshot := &Snapshot{
		Market:       "ETH-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.NewFromFloat(3000),
		Sequence:     50,
		Checksum:     "abc123",
	}

	assert.Equal(t, "ETH-USDC", snapshot.Market)
	assert.Equal(t, int64(1704067200000), snapshot.Timestamp)
	assert.Equal(t, int64(100), snapshot.KafkaOffsets["orders:0"])
	assert.True(t, decimal.NewFromFloat(3000).Equal(snapshot.LastPrice))
	assert.Equal(t, int64(50), snapshot.Sequence)
	assert.Equal(t, "abc123", snapshot.Checksum)
}

func TestSnapshotConfig_Fields(t *testing.T) {
	cfg := &SnapshotConfig{
		Interval:  5 * time.Minute,
		MaxCount:  10,
		TwoPhase:  true,
		KeyPrefix: "snap",
	}

	assert.Equal(t, 5*time.Minute, cfg.Interval)
	assert.Equal(t, 10, cfg.MaxCount)
	assert.True(t, cfg.TwoPhase)
	assert.Equal(t, "snap", cfg.KeyPrefix)
}

func TestSnapshotPriceLevel_Fields(t *testing.T) {
	pl := SnapshotPriceLevel{
		Price: decimal.NewFromFloat(50000),
		Orders: []SnapshotOrder{
			{OrderID: "order-1", Remaining: decimal.NewFromFloat(1.5)},
		},
	}

	assert.True(t, decimal.NewFromFloat(50000).Equal(pl.Price))
	assert.Equal(t, 1, len(pl.Orders))
	assert.Equal(t, "order-1", pl.Orders[0].OrderID)
}

func TestManager_CreateSnapshot_WithOrders(t *testing.T) {
	db, _ := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		MaxCount:  3,
	})

	// 创建带订单的订单簿
	ob := orderbook.NewOrderBook("BTC-USDC")
	ob.AddOrder(&model.Order{
		OrderID:     "bid-1",
		Wallet:      "0x123",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000),
		Amount:      decimal.NewFromFloat(1.5),
		Remaining:   decimal.NewFromFloat(1.5),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	})
	ob.AddOrder(&model.Order{
		OrderID:     "ask-1",
		Wallet:      "0x456",
		Market:      "BTC-USDC",
		Side:        model.OrderSideSell,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(51000),
		Amount:      decimal.NewFromFloat(2.0),
		Remaining:   decimal.NewFromFloat(2.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    2,
	})
	ob.LastPrice = decimal.NewFromFloat(50500)
	ob.Sequence = 100

	offsets := map[string]int64{"orders:0": 100, "cancels:0": 50}

	// 直接测试 createSnapshot 方法
	snapshot := m.createSnapshot("BTC-USDC", ob, offsets)

	assert.Equal(t, "BTC-USDC", snapshot.Market)
	assert.Equal(t, 1, len(snapshot.Bids))
	assert.Equal(t, 1, len(snapshot.Asks))
	assert.Equal(t, offsets, snapshot.KafkaOffsets)
	assert.True(t, decimal.NewFromFloat(50500).Equal(snapshot.LastPrice))
	assert.Equal(t, int64(100), snapshot.Sequence)
	assert.NotEmpty(t, snapshot.Checksum)

	// 验证买单内容
	assert.Equal(t, "bid-1", snapshot.Bids[0].Orders[0].OrderID)
	assert.True(t, decimal.NewFromFloat(50000).Equal(snapshot.Bids[0].Price))

	// 验证卖单内容
	assert.Equal(t, "ask-1", snapshot.Asks[0].Orders[0].OrderID)
	assert.True(t, decimal.NewFromFloat(51000).Equal(snapshot.Asks[0].Price))
}

func TestManager_ConfirmSnapshot_NotFound(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		TwoPhase:  true,
	})

	// Mock: preparing key 不存在
	mock.ExpectGet("snapshot:BTC-USDC:preparing").RedisNil()

	err := m.ConfirmSnapshot(context.Background(), "BTC-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "preparing snapshot not found")
}

func TestManager_ConfirmSnapshot_InvalidJSON(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		TwoPhase:  true,
	})

	// Mock: 获取无效 JSON 的预写快照
	mock.ExpectGet("snapshot:BTC-USDC:preparing").SetVal("invalid json")

	err := m.ConfirmSnapshot(context.Background(), "BTC-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal snapshot")
}

func TestManager_LoadSnapshot_ChecksumMismatch(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// 创建一个错误 checksum 的快照
	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.NewFromFloat(50000),
		Sequence:     100,
		Checksum:     "invalid-checksum",
	}
	data, _ := json.Marshal(snapshot)

	// Mock: preparing key 不存在
	mock.ExpectGet("snapshot:BTC-USDC:preparing").RedisNil()
	// Mock: latest key 返回错误 checksum 的数据
	mock.ExpectGet("snapshot:BTC-USDC:latest").SetVal(string(data))

	_, err := m.LoadSnapshot(context.Background(), "BTC-USDC", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestManager_LoadSnapshot_PreparingInvalidJSON(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// 创建一个有效的 latest 快照
	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.NewFromFloat(50000),
		Sequence:     100,
	}
	snapshot.Checksum = m.calculateChecksum(snapshot)
	data, _ := json.Marshal(snapshot)

	// Mock: 获取无效 JSON 的预写快照
	mock.ExpectGet("snapshot:BTC-USDC:preparing").SetVal("invalid json")
	// 删除无效的预写快照
	mock.ExpectDel("snapshot:BTC-USDC:preparing").SetVal(1)
	// 加载 latest
	mock.ExpectGet("snapshot:BTC-USDC:latest").SetVal(string(data))

	result, err := m.LoadSnapshot(context.Background(), "BTC-USDC", nil)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "BTC-USDC", result.Market)
}

func TestManager_LoadSnapshot_PreparingExists_OffsetsNotCommitted(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// 创建预写快照数据
	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.NewFromFloat(50000),
		Sequence:     100,
	}
	snapshot.Checksum = m.calculateChecksum(snapshot)
	data, _ := json.Marshal(snapshot)

	// Mock: 获取预写快照
	mock.ExpectGet("snapshot:BTC-USDC:preparing").SetVal(string(data))
	// offset 未提交，删除预写快照
	mock.ExpectDel("snapshot:BTC-USDC:preparing").SetVal(1)
	// 加载 latest
	mock.ExpectGet("snapshot:BTC-USDC:latest").SetVal(string(data))

	// committed offsets < snapshot offsets
	committedOffsets := map[string]int64{"orders:0": 50}

	result, err := m.LoadSnapshot(context.Background(), "BTC-USDC", committedOffsets)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestManager_LoadSnapshot_LatestInvalidJSON(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// Mock: preparing key 不存在
	mock.ExpectGet("snapshot:BTC-USDC:preparing").RedisNil()
	// Mock: latest key 返回无效 JSON
	mock.ExpectGet("snapshot:BTC-USDC:latest").SetVal("invalid json")

	_, err := m.LoadSnapshot(context.Background(), "BTC-USDC", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal snapshot")
}

func TestManager_LoadSnapshot_LatestRedisError(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// Mock: preparing key 不存在
	mock.ExpectGet("snapshot:BTC-USDC:preparing").RedisNil()
	// Mock: latest key Redis 错误
	mock.ExpectGet("snapshot:BTC-USDC:latest").SetErr(fmt.Errorf("redis connection error"))

	_, err := m.LoadSnapshot(context.Background(), "BTC-USDC", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get latest snapshot")
}

func TestManager_RestoreOrderBook_Empty(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	// 空快照
	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{},
		Bids:         []SnapshotPriceLevel{},
		Asks:         []SnapshotPriceLevel{},
		LastPrice:    decimal.Zero,
		Sequence:     0,
	}

	ob := m.RestoreOrderBook(snapshot)
	assert.Equal(t, "BTC-USDC", ob.Market)
	assert.True(t, decimal.Zero.Equal(ob.LastPrice))
	assert.Equal(t, int64(0), ob.Sequence)
}

func TestManager_RestoreOrderBook_MultiplePriceLevels(t *testing.T) {
	db, _ := redismock.NewClientMock()
	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	snapshot := &Snapshot{
		Market:       "BTC-USDC",
		Timestamp:    1704067200000,
		KafkaOffsets: map[string]int64{"orders:0": 100},
		Bids: []SnapshotPriceLevel{
			{
				Price: decimal.NewFromFloat(50000),
				Orders: []SnapshotOrder{
					{
						OrderID:     "bid-1",
						Wallet:      "0x123",
						Side:        int8(model.OrderSideBuy),
						Type:        int8(model.OrderTypeLimit),
						TimeInForce: int8(model.TimeInForceGTC),
						Price:       decimal.NewFromFloat(50000),
						Amount:      decimal.NewFromFloat(1.0),
						Remaining:   decimal.NewFromFloat(0.5),
						Timestamp:   1704067200000,
						Sequence:    1,
					},
				},
			},
			{
				Price: decimal.NewFromFloat(49000),
				Orders: []SnapshotOrder{
					{
						OrderID:     "bid-2",
						Wallet:      "0x456",
						Side:        int8(model.OrderSideBuy),
						Type:        int8(model.OrderTypeLimit),
						TimeInForce: int8(model.TimeInForceGTC),
						Price:       decimal.NewFromFloat(49000),
						Amount:      decimal.NewFromFloat(2.0),
						Remaining:   decimal.NewFromFloat(2.0),
						Timestamp:   1704067200000,
						Sequence:    2,
					},
				},
			},
		},
		Asks: []SnapshotPriceLevel{
			{
				Price: decimal.NewFromFloat(51000),
				Orders: []SnapshotOrder{
					{
						OrderID:     "ask-1",
						Wallet:      "0x789",
						Side:        int8(model.OrderSideSell),
						Type:        int8(model.OrderTypeLimit),
						TimeInForce: int8(model.TimeInForceGTC),
						Price:       decimal.NewFromFloat(51000),
						Amount:      decimal.NewFromFloat(3.0),
						Remaining:   decimal.NewFromFloat(3.0),
						Timestamp:   1704067200000,
						Sequence:    3,
					},
				},
			},
		},
		LastPrice: decimal.NewFromFloat(50500),
		Sequence:  100,
	}

	ob := m.RestoreOrderBook(snapshot)

	// 验证买单
	bid1 := ob.GetOrder("bid-1")
	assert.NotNil(t, bid1)
	assert.Equal(t, model.OrderSideBuy, bid1.Side)
	assert.True(t, decimal.NewFromFloat(0.5).Equal(bid1.Remaining))

	bid2 := ob.GetOrder("bid-2")
	assert.NotNil(t, bid2)

	// 验证卖单
	ask1 := ob.GetOrder("ask-1")
	assert.NotNil(t, ask1)
	assert.Equal(t, model.OrderSideSell, ask1.Side)

	// 验证最佳价格
	assert.True(t, decimal.NewFromFloat(50000).Equal(ob.BestBidPrice()))
	assert.True(t, decimal.NewFromFloat(51000).Equal(ob.BestAskPrice()))
}

func TestManager_CleanOldSnapshots_NoCleanupNeeded(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{
		KeyPrefix: "snapshot",
		MaxCount:  5,
	})

	// 只有 3 个快照，不需要清理
	mock.ExpectZCard("snapshot:BTC-USDC:history").SetVal(3)

	m.cleanOldSnapshots(context.Background(), "BTC-USDC")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestManager_ListSnapshots_Empty(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	mock.ExpectZRevRange("snapshot:BTC-USDC:history", 0, -1).SetVal([]string{})

	infos, err := m.ListSnapshots(context.Background(), "BTC-USDC")
	assert.NoError(t, err)
	assert.Empty(t, infos)
}

func TestManager_ListSnapshots_Error(t *testing.T) {
	db, mock := redismock.NewClientMock()

	m := NewManager(db, &SnapshotConfig{KeyPrefix: "snapshot"})

	mock.ExpectZRevRange("snapshot:BTC-USDC:history", 0, -1).SetErr(fmt.Errorf("redis error"))

	_, err := m.ListSnapshots(context.Background(), "BTC-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list snapshots")
}
