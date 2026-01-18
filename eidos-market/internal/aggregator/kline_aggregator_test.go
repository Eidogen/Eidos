package aggregator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// mockKlineRepository 模拟 K 线仓储
type mockKlineRepository struct {
	mu     sync.Mutex
	klines []*model.Kline
}

func (m *mockKlineRepository) Upsert(ctx context.Context, kline *model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.klines = append(m.klines, kline)
	return nil
}

func (m *mockKlineRepository) BatchUpsert(ctx context.Context, klines []*model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.klines = append(m.klines, klines...)
	return nil
}

func (m *mockKlineRepository) Query(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {
	return nil, nil
}

func (m *mockKlineRepository) GetLatest(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	return nil, nil
}

func (m *mockKlineRepository) GetKlines() []*model.Kline {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.klines
}

// mockKlinePublisher 模拟发布器
type mockKlinePublisher struct {
	mu       sync.Mutex
	publishes []struct {
		market   string
		interval model.KlineInterval
		kline    *model.Kline
	}
}

func (m *mockKlinePublisher) PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes = append(m.publishes, struct {
		market   string
		interval model.KlineInterval
		kline    *model.Kline
	}{market, interval, kline})
	return nil
}

func (m *mockKlinePublisher) GetPublishes() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishes)
}

func TestKlineAggregator_ProcessTrade(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockKlineRepository{}
	publisher := &mockKlinePublisher{}

	config := KlineAggregatorConfig{
		FlushInterval: time.Hour, // 不自动刷盘
		BufferSize:    100,
	}

	agg := NewKlineAggregator("BTC-USDC", repo, publisher, logger, config)
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()
	// 使用固定的分钟起始时间戳，确保所有成交在同一分钟内
	// 1699999980000 是精确的分钟边界
	baseMinute := int64(1699999980000)

	// 第一笔成交
	trade1 := &model.TradeEvent{
		TradeID:     "trade1",
		Market:      "BTC-USDC",
		Price:       "50000",
		Amount:      "1",
		QuoteAmount: "50000",
		Timestamp:   baseMinute + 1000, // 分钟开始后1秒
	}
	agg.ProcessTrade(ctx, trade1)

	// 验证 K 线
	kline := agg.GetKline(model.Interval1m)
	require.NotNil(t, kline)
	assert.Equal(t, "BTC-USDC", kline.Market)
	assert.Equal(t, model.Interval1m, kline.Interval)
	assert.True(t, kline.Open.Equal(decimal.NewFromInt(50000)))
	assert.True(t, kline.High.Equal(decimal.NewFromInt(50000)))
	assert.True(t, kline.Low.Equal(decimal.NewFromInt(50000)))
	assert.True(t, kline.Close.Equal(decimal.NewFromInt(50000)))
	assert.True(t, kline.Volume.Equal(decimal.NewFromInt(1)))
	assert.Equal(t, int32(1), kline.TradeCount)

	// 第二笔成交（更高价）
	trade2 := &model.TradeEvent{
		TradeID:     "trade2",
		Market:      "BTC-USDC",
		Price:       "51000",
		Amount:      "2",
		QuoteAmount: "102000",
		Timestamp:   baseMinute + 11000, // 分钟开始后11秒
	}
	agg.ProcessTrade(ctx, trade2)

	kline = agg.GetKline(model.Interval1m)
	assert.True(t, kline.Open.Equal(decimal.NewFromInt(50000)))  // Open 不变
	assert.True(t, kline.High.Equal(decimal.NewFromInt(51000)))  // High 更新
	assert.True(t, kline.Low.Equal(decimal.NewFromInt(50000)))   // Low 不变
	assert.True(t, kline.Close.Equal(decimal.NewFromInt(51000))) // Close 更新
	assert.True(t, kline.Volume.Equal(decimal.NewFromInt(3)))    // Volume 累加
	assert.Equal(t, int32(2), kline.TradeCount)

	// 第三笔成交（更低价）
	trade3 := &model.TradeEvent{
		TradeID:     "trade3",
		Market:      "BTC-USDC",
		Price:       "49000",
		Amount:      "0.5",
		QuoteAmount: "24500",
		Timestamp:   baseMinute + 21000, // 分钟开始后21秒
	}
	agg.ProcessTrade(ctx, trade3)

	kline = agg.GetKline(model.Interval1m)
	assert.True(t, kline.Open.Equal(decimal.NewFromInt(50000)))
	assert.True(t, kline.High.Equal(decimal.NewFromInt(51000)))
	assert.True(t, kline.Low.Equal(decimal.NewFromInt(49000)))   // Low 更新
	assert.True(t, kline.Close.Equal(decimal.NewFromInt(49000))) // Close 更新
	assert.True(t, kline.Volume.Equal(decimal.NewFromFloat(3.5)))
	assert.Equal(t, int32(3), kline.TradeCount)
}

func TestKlineAggregator_KlineRollover(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockKlineRepository{}

	config := KlineAggregatorConfig{
		FlushInterval: time.Hour,
		BufferSize:    100,
	}

	agg := NewKlineAggregator("ETH-USDC", repo, nil, logger, config)
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()

	// 计算当前分钟的开始时间
	now := time.Now()
	currentMinute := (now.UnixMilli() / 60000) * 60000

	// 第一笔成交（当前分钟）
	trade1 := &model.TradeEvent{
		TradeID:   "trade1",
		Market:    "ETH-USDC",
		Price:     "3000",
		Amount:    "1",
		QuoteAmount: "3000",
		Timestamp: currentMinute + 10000, // 当前分钟 + 10秒
	}
	agg.ProcessTrade(ctx, trade1)

	kline1 := agg.GetKline(model.Interval1m)
	assert.Equal(t, currentMinute, kline1.OpenTime)

	// 第二笔成交（下一分钟）
	nextMinute := currentMinute + 60000
	trade2 := &model.TradeEvent{
		TradeID:   "trade2",
		Market:    "ETH-USDC",
		Price:     "3100",
		Amount:    "2",
		QuoteAmount: "6200",
		Timestamp: nextMinute + 5000,
	}
	agg.ProcessTrade(ctx, trade2)

	kline2 := agg.GetKline(model.Interval1m)
	assert.Equal(t, nextMinute, kline2.OpenTime) // 新的 K 线
	assert.True(t, kline2.Open.Equal(decimal.NewFromInt(3100)))
	assert.Equal(t, int32(1), kline2.TradeCount)
}

func TestKlineAggregator_AllIntervals(t *testing.T) {
	logger := zap.NewNop()
	repo := &mockKlineRepository{}

	config := KlineAggregatorConfig{
		FlushInterval: time.Hour,
		BufferSize:    100,
	}

	agg := NewKlineAggregator("BTC-USDC", repo, nil, logger, config)
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()

	trade := &model.TradeEvent{
		TradeID:   "trade1",
		Market:    "BTC-USDC",
		Price:     "50000",
		Amount:    "1",
		QuoteAmount: "50000",
		Timestamp: time.Now().UnixMilli(),
	}
	agg.ProcessTrade(ctx, trade)

	// 验证所有周期都有 K 线
	allKlines := agg.GetAllKlines()
	assert.Len(t, allKlines, len(model.AllIntervals))

	for _, interval := range model.AllIntervals {
		kline, ok := allKlines[interval]
		assert.True(t, ok, "interval %s should exist", interval)
		assert.NotNil(t, kline)
		assert.Equal(t, int32(1), kline.TradeCount)
	}
}

func TestKlineAggregator_Stats(t *testing.T) {
	logger := zap.NewNop()

	config := KlineAggregatorConfig{
		FlushInterval: time.Hour,
		BufferSize:    100,
	}

	agg := NewKlineAggregator("BTC-USDC", nil, nil, logger, config)
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()

	// 处理 3 笔成交
	for i := 0; i < 3; i++ {
		trade := &model.TradeEvent{
			TradeID:   string(rune('a' + i)),
			Market:    "BTC-USDC",
			Price:     "50000",
			Amount:    "1",
			QuoteAmount: "50000",
			Timestamp: time.Now().UnixMilli(),
		}
		agg.ProcessTrade(ctx, trade)
	}

	stats := agg.Stats()
	assert.Equal(t, int64(3), stats["trade_count"])
}

func TestCalculateOpenTime(t *testing.T) {
	tests := []struct {
		name      string
		timestamp int64
		interval  model.KlineInterval
		expected  int64
	}{
		{
			name:      "1m - exact minute",
			timestamp: 1699999980000, // 精确到分钟边界
			interval:  model.Interval1m,
			expected:  1699999980000,
		},
		{
			name:      "1m - mid minute",
			timestamp: 1699999980000 + 30000, // 分钟中间
			interval:  model.Interval1m,
			expected:  1699999980000,
		},
		{
			name:      "5m",
			timestamp: 1700000100000 + 60000, // 5分钟区间内
			interval:  model.Interval5m,
			expected:  1700000100000,
		},
		{
			name:      "1h",
			timestamp: 1699999200000 + 1000000, // 小时区间内
			interval:  model.Interval1h,
			expected:  1699999200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.CalculateOpenTime(tt.timestamp, tt.interval)
			assert.Equal(t, tt.expected, result)
		})
	}
}
