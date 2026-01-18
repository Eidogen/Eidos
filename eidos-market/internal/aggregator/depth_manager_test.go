package aggregator

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// mockDepthPublisher 模拟深度发布器
type mockDepthPublisher struct {
	mu     sync.Mutex
	depths []*model.Depth
}

func (m *mockDepthPublisher) PublishDepth(ctx context.Context, market string, depth *model.Depth) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depths = append(m.depths, depth)
	return nil
}

func (m *mockDepthPublisher) GetDepths() []*model.Depth {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.depths
}

func TestDepthManager_ApplyUpdate(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels:       100,
		PublishInterval: 0, // 不自动发布
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 第一次更新
	update1 := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
			{Price: decimal.NewFromInt(49900), Amount: decimal.NewFromInt(20)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50100), Amount: decimal.NewFromInt(5)},
			{Price: decimal.NewFromInt(50200), Amount: decimal.NewFromInt(15)},
		},
	}
	err := dm.ApplyUpdate(ctx, update1)
	require.NoError(t, err)

	depth := dm.GetDepth(100)
	assert.Equal(t, "BTC-USDC", depth.Market)
	assert.Len(t, depth.Bids, 2)
	assert.Len(t, depth.Asks, 2)
	assert.Equal(t, uint64(1), depth.Sequence)

	// 验证买单排序（价格降序）
	assert.True(t, depth.Bids[0].Price.Equal(decimal.NewFromInt(50000)))
	assert.True(t, depth.Bids[1].Price.Equal(decimal.NewFromInt(49900)))

	// 验证卖单排序（价格升序）
	assert.True(t, depth.Asks[0].Price.Equal(decimal.NewFromInt(50100)))
	assert.True(t, depth.Asks[1].Price.Equal(decimal.NewFromInt(50200)))
}

func TestDepthManager_UpdateExisting(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 初始深度
	update1 := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50100), Amount: decimal.NewFromInt(5)},
		},
	}
	dm.ApplyUpdate(ctx, update1)

	// 更新买单数量
	update2 := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 2,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(15)}, // 更新数量
		},
	}
	dm.ApplyUpdate(ctx, update2)

	depth := dm.GetDepth(100)
	assert.True(t, depth.Bids[0].Amount.Equal(decimal.NewFromInt(15)))
}

func TestDepthManager_RemoveLevel(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 初始深度
	update1 := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
			{Price: decimal.NewFromInt(49900), Amount: decimal.NewFromInt(20)},
		},
	}
	dm.ApplyUpdate(ctx, update1)

	// 删除一个档位（amount = 0）
	update2 := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 2,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.Zero}, // 删除
		},
	}
	dm.ApplyUpdate(ctx, update2)

	depth := dm.GetDepth(100)
	assert.Len(t, depth.Bids, 1)
	assert.True(t, depth.Bids[0].Price.Equal(decimal.NewFromInt(49900)))
}

func TestDepthManager_Idempotent(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
		},
	}

	// 应用两次相同的更新
	dm.ApplyUpdate(ctx, update)
	dm.ApplyUpdate(ctx, update) // 应该被忽略（幂等）

	assert.Equal(t, uint64(1), dm.GetSequence())
}

func TestDepthManager_GetBestPrices(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
			{Price: decimal.NewFromInt(49900), Amount: decimal.NewFromInt(20)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50100), Amount: decimal.NewFromInt(5)},
			{Price: decimal.NewFromInt(50200), Amount: decimal.NewFromInt(15)},
		},
	}
	dm.ApplyUpdate(ctx, update)

	bestBid, bestBidQty, bestAsk, bestAskQty := dm.GetBestPrices()
	assert.True(t, bestBid.Equal(decimal.NewFromInt(50000)))
	assert.True(t, bestBidQty.Equal(decimal.NewFromInt(10)))
	assert.True(t, bestAsk.Equal(decimal.NewFromInt(50100)))
	assert.True(t, bestAskQty.Equal(decimal.NewFromInt(5)))
}

func TestDepthManager_Limit(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 添加 10 个档位
	bids := make([]*model.PriceLevel, 10)
	for i := 0; i < 10; i++ {
		bids[i] = &model.PriceLevel{
			Price:  decimal.NewFromInt(int64(50000 - i*100)),
			Amount: decimal.NewFromInt(int64(i + 1)),
		}
	}

	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids:     bids,
	}
	dm.ApplyUpdate(ctx, update)

	// 请求 5 档
	depth := dm.GetDepth(5)
	assert.Len(t, depth.Bids, 5)
	assert.True(t, depth.Bids[0].Price.Equal(decimal.NewFromInt(50000))) // 最高价
	assert.True(t, depth.Bids[4].Price.Equal(decimal.NewFromInt(49600)))
}

func TestDepthManager_ApplySnapshot(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 先应用增量更新
	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
		},
	}
	dm.ApplyUpdate(ctx, update)

	// 应用全量快照
	snapshot := &model.Depth{
		Market:   "BTC-USDC",
		Sequence: 100,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(51000), Amount: decimal.NewFromInt(20)},
			{Price: decimal.NewFromInt(50900), Amount: decimal.NewFromInt(30)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromInt(51100), Amount: decimal.NewFromInt(10)},
		},
	}
	dm.ApplySnapshot(snapshot)

	depth := dm.GetDepth(100)
	assert.Len(t, depth.Bids, 2)
	assert.Len(t, depth.Asks, 1)
	assert.Equal(t, uint64(100), depth.Sequence)
	assert.True(t, depth.Bids[0].Price.Equal(decimal.NewFromInt(51000)))
}

func TestDepthManager_Stats(t *testing.T) {
	logger := zap.NewNop()

	config := DepthManagerConfig{
		MaxLevels: 100,
	}

	dm := NewDepthManager("BTC-USDC", nil, nil, logger, config)
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()

	// 应用更新
	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50000), Amount: decimal.NewFromInt(10)},
			{Price: decimal.NewFromInt(49900), Amount: decimal.NewFromInt(20)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromInt(50100), Amount: decimal.NewFromInt(5)},
		},
	}
	dm.ApplyUpdate(ctx, update)

	stats := dm.Stats()
	assert.Equal(t, int64(1), stats["update_count"])
	assert.Equal(t, int64(2), stats["bid_levels"])
	assert.Equal(t, int64(1), stats["ask_levels"])
	assert.Equal(t, int64(1), stats["sequence"])
}
