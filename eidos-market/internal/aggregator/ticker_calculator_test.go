package aggregator

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// mockTickerPublisher 模拟 Ticker 发布器
type mockTickerPublisher struct {
	mu       sync.Mutex
	tickers  []*model.Ticker
}

func (m *mockTickerPublisher) PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickers = append(m.tickers, ticker)
	return nil
}

func (m *mockTickerPublisher) GetTickers() []*model.Ticker {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tickers
}

func TestTickerCalculator_ProcessTrade(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publisher := &mockTickerPublisher{}

	config := TickerCalculatorConfig{
		PublishInterval: time.Hour, // 不自动发布
	}

	tc := NewTickerCalculator("BTC-USDC", publisher, logger, config)
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()
	now := time.Now()

	// 第一笔成交
	trade1 := &model.TradeEvent{
		TradeID:     "trade1",
		Market:      "BTC-USDC",
		Price:       "50000",
		Amount:      "1",
		QuoteAmount: "50000",
		Timestamp:   now.UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade1)

	ticker := tc.GetTicker()
	require.NotNil(t, ticker)
	assert.Equal(t, "BTC-USDC", ticker.Market)
	assert.True(t, ticker.LastPrice.Equal(decimal.NewFromInt(50000)))
	assert.True(t, ticker.Open.Equal(decimal.NewFromInt(50000)))
	assert.True(t, ticker.High.Equal(decimal.NewFromInt(50000)))
	assert.True(t, ticker.Low.Equal(decimal.NewFromInt(50000)))
	assert.True(t, ticker.Volume.Equal(decimal.NewFromInt(1)))
	assert.Equal(t, int32(1), ticker.TradeCount)

	// 第二笔成交
	trade2 := &model.TradeEvent{
		TradeID:     "trade2",
		Market:      "BTC-USDC",
		Price:       "51000",
		Amount:      "2",
		QuoteAmount: "102000",
		Timestamp:   now.Add(10 * time.Second).UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade2)

	ticker = tc.GetTicker()
	assert.True(t, ticker.LastPrice.Equal(decimal.NewFromInt(51000)))
	assert.True(t, ticker.High.Equal(decimal.NewFromInt(51000)))
	assert.True(t, ticker.Volume.Equal(decimal.NewFromInt(3)))
	assert.Equal(t, int32(2), ticker.TradeCount)

	// 第三笔成交（低价）
	trade3 := &model.TradeEvent{
		TradeID:     "trade3",
		Market:      "BTC-USDC",
		Price:       "49000",
		Amount:      "0.5",
		QuoteAmount: "24500",
		Timestamp:   now.Add(20 * time.Second).UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade3)

	ticker = tc.GetTicker()
	assert.True(t, ticker.LastPrice.Equal(decimal.NewFromInt(49000)))
	assert.True(t, ticker.High.Equal(decimal.NewFromInt(51000)))
	assert.True(t, ticker.Low.Equal(decimal.NewFromInt(49000)))
	assert.Equal(t, int32(3), ticker.TradeCount)
}

func TestTickerCalculator_PriceChange(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	config := TickerCalculatorConfig{
		PublishInterval: time.Hour,
	}

	tc := NewTickerCalculator("ETH-USDC", nil, logger, config)
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()
	now := time.Now()

	// 开盘价 3000
	trade1 := &model.TradeEvent{
		TradeID:     "trade1",
		Market:      "ETH-USDC",
		Price:       "3000",
		Amount:      "1",
		QuoteAmount: "3000",
		Timestamp:   now.UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade1)

	// 当前价 3150（上涨 5%）
	trade2 := &model.TradeEvent{
		TradeID:     "trade2",
		Market:      "ETH-USDC",
		Price:       "3150",
		Amount:      "1",
		QuoteAmount: "3150",
		Timestamp:   now.Add(time.Second).UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade2)

	ticker := tc.GetTicker()
	assert.True(t, ticker.PriceChange.Equal(decimal.NewFromInt(150)))
	assert.True(t, ticker.PriceChangePercent.Equal(decimal.NewFromInt(5)))
}

func TestTickerCalculator_UpdateBestPrices(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	config := TickerCalculatorConfig{
		PublishInterval: time.Hour,
	}

	tc := NewTickerCalculator("BTC-USDC", nil, logger, config)
	tc.Start()
	defer tc.Stop()

	// 更新最优买卖价
	bestBid := decimal.NewFromInt(49900)
	bestBidQty := decimal.NewFromInt(10)
	bestAsk := decimal.NewFromInt(50100)
	bestAskQty := decimal.NewFromInt(5)

	tc.UpdateBestPrices(bestBid, bestBidQty, bestAsk, bestAskQty)

	ticker := tc.GetTicker()
	assert.True(t, ticker.BestBid.Equal(bestBid))
	assert.True(t, ticker.BestBidQty.Equal(bestBidQty))
	assert.True(t, ticker.BestAsk.Equal(bestAsk))
	assert.True(t, ticker.BestAskQty.Equal(bestAskQty))
}

func TestTickerCalculator_BucketExpiry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	config := TickerCalculatorConfig{
		PublishInterval: time.Hour,
	}

	tc := NewTickerCalculator("BTC-USDC", nil, logger, config)
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()

	// 插入一个"老"的桶（超过24小时）
	oldMinute := model.CalculateMinute(time.Now().Add(-25 * time.Hour).UnixMilli())
	tc.mu.Lock()
	idx := tc.bucketIndex(oldMinute)
	tc.buckets[idx] = &model.TickerBucket{
		Minute:     oldMinute,
		Open:       decimal.NewFromInt(40000),
		High:       decimal.NewFromInt(40000),
		Low:        decimal.NewFromInt(40000),
		Close:      decimal.NewFromInt(40000),
		Volume:     decimal.NewFromInt(100),
		TradeCount: 10,
	}
	tc.mu.Unlock()

	// 插入新的成交
	trade := &model.TradeEvent{
		TradeID:     "trade1",
		Market:      "BTC-USDC",
		Price:       "50000",
		Amount:      "1",
		QuoteAmount: "50000",
		Timestamp:   time.Now().UnixMilli(),
	}
	tc.ProcessTrade(ctx, trade)

	// 获取 Ticker，老桶应该被忽略
	ticker := tc.GetTicker()
	assert.True(t, ticker.Open.Equal(decimal.NewFromInt(50000)))
	assert.True(t, ticker.Volume.Equal(decimal.NewFromInt(1)))
	assert.Equal(t, int32(1), ticker.TradeCount)
}

func TestTickerCalculator_Stats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	config := TickerCalculatorConfig{
		PublishInterval: time.Hour,
	}

	tc := NewTickerCalculator("BTC-USDC", nil, logger, config)
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()

	// 处理 5 笔成交
	for i := 0; i < 5; i++ {
		trade := &model.TradeEvent{
			TradeID:     string(rune('a' + i)),
			Market:      "BTC-USDC",
			Price:       "50000",
			Amount:      "1",
			QuoteAmount: "50000",
			Timestamp:   time.Now().UnixMilli(),
		}
		tc.ProcessTrade(ctx, trade)
	}

	stats := tc.Stats()
	assert.Equal(t, int64(5), stats["trade_count"])
	assert.GreaterOrEqual(t, stats["active_buckets"], int64(1))
}

func TestCalculateMinute(t *testing.T) {
	// 使用正确对齐的时间戳: 1699999980000 是精确的分钟边界 (28333333 * 60000)
	baseMinute := int64(1699999980000)

	tests := []struct {
		name      string
		timestamp int64
		expected  int64
	}{
		{
			name:      "exact minute",
			timestamp: baseMinute,
			expected:  baseMinute,
		},
		{
			name:      "30 seconds after",
			timestamp: baseMinute + 30000,
			expected:  baseMinute,
		},
		{
			name:      "59 seconds after",
			timestamp: baseMinute + 59999,
			expected:  baseMinute,
		},
		{
			name:      "exactly at next minute",
			timestamp: baseMinute + 60000,
			expected:  baseMinute + 60000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := model.CalculateMinute(tt.timestamp)
			assert.Equal(t, tt.expected, result)
		})
	}
}
