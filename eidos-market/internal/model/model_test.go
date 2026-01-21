package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============ Kline Tests ============

func TestIsValidInterval(t *testing.T) {
	tests := []struct {
		interval string
		expected bool
	}{
		{"1m", true},
		{"5m", true},
		{"15m", true},
		{"30m", true},
		{"1h", true},
		{"4h", true},
		{"1d", true},
		{"1w", true},
		{"2m", false},
		{"1s", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.interval, func(t *testing.T) {
			result := IsValidInterval(tt.interval)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateOpenTime(t *testing.T) {
	// 使用实际的时间间隔来计算预期值
	t.Run("1m exact boundary", func(t *testing.T) {
		// 1700000040000 / 60000 = 28333334（取整）* 60000 = 1700000040000
		ts := int64(1700000040000)
		result := CalculateOpenTime(ts, Interval1m)
		expected := (ts / 60000) * 60000
		assert.Equal(t, expected, result)
	})

	t.Run("1m mid", func(t *testing.T) {
		ts := int64(1700000070000) // 分钟中间
		result := CalculateOpenTime(ts, Interval1m)
		expected := (ts / 60000) * 60000
		assert.Equal(t, expected, result)
	})

	t.Run("5m", func(t *testing.T) {
		ts := int64(1700000100000)
		result := CalculateOpenTime(ts, Interval5m)
		expected := (ts / 300000) * 300000
		assert.Equal(t, expected, result)
	})

	t.Run("1h", func(t *testing.T) {
		ts := int64(1700001800000)
		result := CalculateOpenTime(ts, Interval1h)
		expected := (ts / 3600000) * 3600000
		assert.Equal(t, expected, result)
	})

	t.Run("1d", func(t *testing.T) {
		ts := int64(1700043200000)
		result := CalculateOpenTime(ts, Interval1d)
		expected := (ts / 86400000) * 86400000
		assert.Equal(t, expected, result)
	})
}

func TestCalculateCloseTime(t *testing.T) {
	tests := []struct {
		name     string
		openTime int64
		interval KlineInterval
		expected int64
	}{
		{"1m", 1700000000000, Interval1m, 1700000059999},
		{"5m", 1700000000000, Interval5m, 1700000299999},
		{"1h", 1700000000000, Interval1h, 1700003599999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCloseTime(tt.openTime, tt.interval)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKline_IsEmpty(t *testing.T) {
	k1 := &Kline{TradeCount: 0}
	k2 := &Kline{TradeCount: 5}

	assert.True(t, k1.IsEmpty())
	assert.False(t, k2.IsEmpty())
}

func TestKline_Clone(t *testing.T) {
	original := &Kline{
		Market:      "BTC-USDC",
		Interval:    Interval1m,
		OpenTime:    1700000000000,
		Open:        decimal.NewFromFloat(50000),
		High:        decimal.NewFromFloat(51000),
		Low:         decimal.NewFromFloat(49000),
		Close:       decimal.NewFromFloat(50500),
		Volume:      decimal.NewFromFloat(100),
		QuoteVolume: decimal.NewFromFloat(5000000),
		TradeCount:  50,
		CloseTime:   1700000059999,
	}

	clone := original.Clone()

	assert.Equal(t, original.Market, clone.Market)
	assert.Equal(t, original.Interval, clone.Interval)
	assert.True(t, original.Open.Equal(clone.Open))
	assert.True(t, original.High.Equal(clone.High))
	assert.True(t, original.Low.Equal(clone.Low))
	assert.True(t, original.Close.Equal(clone.Close))
	assert.True(t, original.Volume.Equal(clone.Volume))
	assert.Equal(t, original.TradeCount, clone.TradeCount)

	// 确保是深拷贝
	clone.Market = "ETH-USDC"
	assert.NotEqual(t, original.Market, clone.Market)
}

func TestKline_TableName(t *testing.T) {
	k := Kline{}
	assert.Equal(t, "market_klines", k.TableName())
}

// ============ Depth Tests ============

func TestIsValidDepthLimit(t *testing.T) {
	tests := []struct {
		limit    int
		expected bool
	}{
		{5, true},
		{10, true},
		{20, true},
		{50, true},
		{100, true},
		{0, false},
		{15, false},
		{200, false},
	}

	for _, tt := range tests {
		result := IsValidDepthLimit(tt.limit)
		assert.Equal(t, tt.expected, result)
	}
}

func TestNormalizeDepthLimit(t *testing.T) {
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 20},
		{-1, 20},
		{1, 5},
		{5, 5},
		{6, 10},
		{10, 10},
		{15, 20},
		{20, 20},
		{30, 50},
		{50, 50},
		{80, 100},
		{100, 100},
		{200, 100},
	}

	for _, tt := range tests {
		result := NormalizeDepthLimit(tt.limit)
		assert.Equal(t, tt.expected, result)
	}
}

func TestPriceLevel_Clone(t *testing.T) {
	original := &PriceLevel{
		Price:  decimal.NewFromFloat(50000),
		Amount: decimal.NewFromFloat(10),
	}

	clone := original.Clone()

	assert.True(t, original.Price.Equal(clone.Price))
	assert.True(t, original.Amount.Equal(clone.Amount))
}

func TestDepth_Clone(t *testing.T) {
	original := &Depth{
		Market:   "BTC-USDC",
		Sequence: 100,
		Bids: []*PriceLevel{
			{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(10)},
			{Price: decimal.NewFromFloat(49900), Amount: decimal.NewFromFloat(20)},
		},
		Asks: []*PriceLevel{
			{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
		},
		Timestamp: 1700000000000,
	}

	clone := original.Clone()

	assert.Equal(t, original.Market, clone.Market)
	assert.Equal(t, original.Sequence, clone.Sequence)
	assert.Len(t, clone.Bids, 2)
	assert.Len(t, clone.Asks, 1)

	// 确保是深拷贝
	clone.Bids[0].Price = decimal.NewFromFloat(99999)
	assert.False(t, original.Bids[0].Price.Equal(clone.Bids[0].Price))
}

func TestDepth_Limit(t *testing.T) {
	depth := &Depth{
		Market:   "BTC-USDC",
		Sequence: 100,
		Bids: []*PriceLevel{
			{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(10)},
			{Price: decimal.NewFromFloat(49900), Amount: decimal.NewFromFloat(20)},
			{Price: decimal.NewFromFloat(49800), Amount: decimal.NewFromFloat(30)},
		},
		Asks: []*PriceLevel{
			{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
			{Price: decimal.NewFromFloat(50200), Amount: decimal.NewFromFloat(15)},
		},
	}

	limited := depth.Limit(2)

	assert.Len(t, limited.Bids, 2)
	assert.Len(t, limited.Asks, 2)
	assert.Equal(t, depth.Market, limited.Market)
	assert.Equal(t, depth.Sequence, limited.Sequence)
}

func TestDepth_Limit_SmallerThanActual(t *testing.T) {
	depth := &Depth{
		Market: "BTC-USDC",
		Bids: []*PriceLevel{
			{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(10)},
		},
		Asks: []*PriceLevel{
			{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
		},
	}

	limited := depth.Limit(10)

	assert.Len(t, limited.Bids, 1)
	assert.Len(t, limited.Asks, 1)
}

func TestDepth_GetBestBid(t *testing.T) {
	t.Run("with bids", func(t *testing.T) {
		depth := &Depth{
			Bids: []*PriceLevel{
				{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(10)},
			},
		}
		price, amount := depth.GetBestBid()
		assert.True(t, price.Equal(decimal.NewFromFloat(50000)))
		assert.True(t, amount.Equal(decimal.NewFromFloat(10)))
	})

	t.Run("empty bids", func(t *testing.T) {
		depth := &Depth{Bids: []*PriceLevel{}}
		price, amount := depth.GetBestBid()
		assert.True(t, price.IsZero())
		assert.True(t, amount.IsZero())
	})
}

func TestDepth_GetBestAsk(t *testing.T) {
	t.Run("with asks", func(t *testing.T) {
		depth := &Depth{
			Asks: []*PriceLevel{
				{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
			},
		}
		price, amount := depth.GetBestAsk()
		assert.True(t, price.Equal(decimal.NewFromFloat(50100)))
		assert.True(t, amount.Equal(decimal.NewFromFloat(5)))
	})

	t.Run("empty asks", func(t *testing.T) {
		depth := &Depth{Asks: []*PriceLevel{}}
		price, amount := depth.GetBestAsk()
		assert.True(t, price.IsZero())
		assert.True(t, amount.IsZero())
	})
}

func TestSortBids(t *testing.T) {
	levels := []*PriceLevel{
		{Price: decimal.NewFromFloat(49000), Amount: decimal.NewFromFloat(10)},
		{Price: decimal.NewFromFloat(51000), Amount: decimal.NewFromFloat(20)},
		{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(15)},
	}

	SortBids(levels)

	assert.True(t, levels[0].Price.Equal(decimal.NewFromFloat(51000)))
	assert.True(t, levels[1].Price.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, levels[2].Price.Equal(decimal.NewFromFloat(49000)))
}

func TestSortAsks(t *testing.T) {
	levels := []*PriceLevel{
		{Price: decimal.NewFromFloat(51000), Amount: decimal.NewFromFloat(10)},
		{Price: decimal.NewFromFloat(49000), Amount: decimal.NewFromFloat(20)},
		{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(15)},
	}

	SortAsks(levels)

	assert.True(t, levels[0].Price.Equal(decimal.NewFromFloat(49000)))
	assert.True(t, levels[1].Price.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, levels[2].Price.Equal(decimal.NewFromFloat(51000)))
}

// ============ Market Tests ============

func TestMarketStatus_String(t *testing.T) {
	tests := []struct {
		status   MarketStatus
		expected string
	}{
		{MarketStatusInactive, "INACTIVE"},
		{MarketStatusActive, "ACTIVE"},
		{MarketStatusSuspend, "SUSPEND"},
		{MarketStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.status.String())
	}
}

func TestMarket_IsActive(t *testing.T) {
	tests := []struct {
		status   MarketStatus
		expected bool
	}{
		{MarketStatusActive, true},
		{MarketStatusInactive, false},
		{MarketStatusSuspend, false},
	}

	for _, tt := range tests {
		m := &Market{Status: tt.status}
		assert.Equal(t, tt.expected, m.IsActive())
	}
}

func TestMarket_IsTradingEnabled(t *testing.T) {
	tests := []struct {
		status         MarketStatus
		tradingEnabled bool
		expected       bool
	}{
		{MarketStatusActive, true, true},
		{MarketStatusActive, false, false},
		{MarketStatusInactive, true, false},
		{MarketStatusSuspend, true, false},
	}

	for _, tt := range tests {
		m := &Market{Status: tt.status, TradingEnabled: tt.tradingEnabled}
		assert.Equal(t, tt.expected, m.IsTradingEnabled())
	}
}

func TestMarket_Clone(t *testing.T) {
	original := &Market{
		ID:             1,
		Symbol:         "BTC-USDC",
		BaseToken:      "BTC",
		QuoteToken:     "USDC",
		PriceDecimals:  2,
		SizeDecimals:   6,
		MinSize:        decimal.NewFromFloat(0.001),
		MaxSize:        decimal.NewFromFloat(1000),
		MinNotional:    decimal.NewFromFloat(10),
		TickSize:       decimal.NewFromFloat(0.01),
		MakerFee:       decimal.NewFromFloat(0.001),
		TakerFee:       decimal.NewFromFloat(0.002),
		Status:         MarketStatusActive,
		TradingEnabled: true,
		CreatedBy:      "0x123",
		CreatedAt:      1700000000000,
		UpdatedBy:      "0x456",
		UpdatedAt:      1700000001000,
	}

	clone := original.Clone()

	assert.Equal(t, original.ID, clone.ID)
	assert.Equal(t, original.Symbol, clone.Symbol)
	assert.Equal(t, original.BaseToken, clone.BaseToken)
	assert.True(t, original.MinSize.Equal(clone.MinSize))
	assert.Equal(t, original.Status, clone.Status)

	// 确保是深拷贝
	clone.Symbol = "ETH-USDC"
	assert.NotEqual(t, original.Symbol, clone.Symbol)
}

func TestMarket_TableName(t *testing.T) {
	m := Market{}
	assert.Equal(t, "market_markets", m.TableName())
}

// ============ Trade Tests ============

func TestTradeSide_String(t *testing.T) {
	tests := []struct {
		side     TradeSide
		expected string
	}{
		{TradeSideBuy, "BUY"},
		{TradeSideSell, "SELL"},
		{TradeSide(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.side.String())
	}
}

func TestTrade_Clone(t *testing.T) {
	original := &Trade{
		TradeID:     "trade-1",
		Market:      "BTC-USDC",
		Price:       decimal.NewFromFloat(50000),
		Amount:      decimal.NewFromFloat(1.5),
		QuoteAmount: decimal.NewFromFloat(75000),
		Side:        TradeSideBuy,
		Timestamp:   1700000000000,
		CreatedAt:   1700000000000,
	}

	clone := original.Clone()

	assert.Equal(t, original.TradeID, clone.TradeID)
	assert.Equal(t, original.Market, clone.Market)
	assert.True(t, original.Price.Equal(clone.Price))
	assert.Equal(t, original.Side, clone.Side)

	// 确保是深拷贝
	clone.TradeID = "trade-2"
	assert.NotEqual(t, original.TradeID, clone.TradeID)
}

func TestTrade_TableName(t *testing.T) {
	tr := Trade{}
	assert.Equal(t, "market_trades", tr.TableName())
}

func TestTradeEvent_ToTrade(t *testing.T) {
	t.Run("maker is buyer, taker is seller", func(t *testing.T) {
		event := &TradeEvent{
			TradeID:     "trade-1",
			Market:      "BTC-USDC",
			Price:       "50000",
			Amount:      "1.5",
			QuoteAmount: "75000",
			MakerSide:   0, // Maker 是买方
			Timestamp:   1700000000000,
		}

		trade, err := event.ToTrade()
		require.NoError(t, err)

		assert.Equal(t, "trade-1", trade.TradeID)
		assert.Equal(t, TradeSideSell, trade.Side) // Taker 是卖方
		assert.True(t, trade.Price.Equal(decimal.NewFromFloat(50000)))
	})

	t.Run("maker is seller, taker is buyer", func(t *testing.T) {
		event := &TradeEvent{
			TradeID:     "trade-2",
			Market:      "BTC-USDC",
			Price:       "50000",
			Amount:      "1.5",
			QuoteAmount: "75000",
			MakerSide:   1, // Maker 是卖方
			Timestamp:   1700000000000,
		}

		trade, err := event.ToTrade()
		require.NoError(t, err)

		assert.Equal(t, TradeSideBuy, trade.Side) // Taker 是买方
	})

	t.Run("invalid price format", func(t *testing.T) {
		event := &TradeEvent{
			Price:       "invalid",
			Amount:      "1.5",
			QuoteAmount: "75000",
		}

		_, err := event.ToTrade()
		assert.Error(t, err)
	})

	t.Run("invalid amount format", func(t *testing.T) {
		event := &TradeEvent{
			Price:       "50000",
			Amount:      "invalid",
			QuoteAmount: "75000",
		}

		_, err := event.ToTrade()
		assert.Error(t, err)
	})
}

func TestTradeEvent_Getters(t *testing.T) {
	event := &TradeEvent{
		Price:       "50000.50",
		Amount:      "1.5",
		QuoteAmount: "75000.75",
	}

	price, err := event.GetPrice()
	require.NoError(t, err)
	assert.True(t, price.Equal(decimal.NewFromFloat(50000.50)))

	amount, err := event.GetAmount()
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(1.5)))

	quoteAmount, err := event.GetQuoteAmount()
	require.NoError(t, err)
	assert.True(t, quoteAmount.Equal(decimal.NewFromFloat(75000.75)))
}

func TestTradeEvent_Getters_Error(t *testing.T) {
	event := &TradeEvent{
		Price:       "invalid",
		Amount:      "invalid",
		QuoteAmount: "invalid",
	}

	_, err := event.GetPrice()
	assert.Error(t, err)

	_, err = event.GetAmount()
	assert.Error(t, err)

	_, err = event.GetQuoteAmount()
	assert.Error(t, err)
}

// ============ Ticker Tests ============

func TestTicker_Clone(t *testing.T) {
	original := &Ticker{
		Market:             "BTC-USDC",
		LastPrice:          decimal.NewFromFloat(50000),
		PriceChange:        decimal.NewFromFloat(500),
		PriceChangePercent: decimal.NewFromFloat(1.01),
		Open:               decimal.NewFromFloat(49500),
		High:               decimal.NewFromFloat(51000),
		Low:                decimal.NewFromFloat(49000),
		Volume:             decimal.NewFromFloat(1234.56),
		QuoteVolume:        decimal.NewFromFloat(61728000),
		BestBid:            decimal.NewFromFloat(49900),
		BestBidQty:         decimal.NewFromFloat(10.5),
		BestAsk:            decimal.NewFromFloat(50100),
		BestAskQty:         decimal.NewFromFloat(5.2),
		TradeCount:         5678,
		Timestamp:          1700000000000,
	}

	clone := original.Clone()

	assert.Equal(t, original.Market, clone.Market)
	assert.True(t, original.LastPrice.Equal(clone.LastPrice))
	assert.True(t, original.PriceChange.Equal(clone.PriceChange))
	assert.True(t, original.BestBid.Equal(clone.BestBid))
	assert.Equal(t, original.TradeCount, clone.TradeCount)

	// 确保是深拷贝
	clone.Market = "ETH-USDC"
	assert.NotEqual(t, original.Market, clone.Market)
}

func TestTickerBucket_Clone(t *testing.T) {
	original := &TickerBucket{
		Minute:      1700000000000,
		Open:        decimal.NewFromFloat(50000),
		High:        decimal.NewFromFloat(51000),
		Low:         decimal.NewFromFloat(49000),
		Close:       decimal.NewFromFloat(50500),
		Volume:      decimal.NewFromFloat(100),
		QuoteVolume: decimal.NewFromFloat(5000000),
		TradeCount:  50,
		FirstTrade:  1700000001000,
		LastTrade:   1700000059000,
	}

	clone := original.Clone()

	assert.Equal(t, original.Minute, clone.Minute)
	assert.True(t, original.Open.Equal(clone.Open))
	assert.True(t, original.High.Equal(clone.High))
	assert.Equal(t, original.TradeCount, clone.TradeCount)
}

func TestNewTickerBucket(t *testing.T) {
	minute := int64(1700000000000)
	bucket := NewTickerBucket(minute)

	assert.Equal(t, minute, bucket.Minute)
	assert.True(t, bucket.Open.IsZero())
	assert.True(t, bucket.High.IsZero())
	assert.True(t, bucket.Low.IsZero())
	assert.True(t, bucket.Close.IsZero())
	assert.True(t, bucket.Volume.IsZero())
	assert.True(t, bucket.QuoteVolume.IsZero())
	assert.Equal(t, int32(0), bucket.TradeCount)
	assert.Equal(t, int64(0), bucket.FirstTrade)
	assert.Equal(t, int64(0), bucket.LastTrade)
}

func TestTickerBucket_IsEmpty(t *testing.T) {
	bucket := NewTickerBucket(1700000000000)
	assert.True(t, bucket.IsEmpty())

	bucket.TradeCount = 1
	assert.False(t, bucket.IsEmpty())
}

func TestTickerBucket_Update(t *testing.T) {
	bucket := NewTickerBucket(1700000000000)

	// 第一笔成交
	bucket.Update(
		decimal.NewFromFloat(50000),
		decimal.NewFromFloat(1),
		decimal.NewFromFloat(50000),
		1700000001000,
	)

	assert.True(t, bucket.Open.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, bucket.High.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, bucket.Low.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, bucket.Close.Equal(decimal.NewFromFloat(50000)))
	assert.True(t, bucket.Volume.Equal(decimal.NewFromFloat(1)))
	assert.Equal(t, int32(1), bucket.TradeCount)
	assert.Equal(t, int64(1700000001000), bucket.FirstTrade)

	// 第二笔成交 - 更高价格
	bucket.Update(
		decimal.NewFromFloat(51000),
		decimal.NewFromFloat(2),
		decimal.NewFromFloat(102000),
		1700000002000,
	)

	assert.True(t, bucket.Open.Equal(decimal.NewFromFloat(50000)))  // 不变
	assert.True(t, bucket.High.Equal(decimal.NewFromFloat(51000)))  // 更新
	assert.True(t, bucket.Low.Equal(decimal.NewFromFloat(50000)))   // 不变
	assert.True(t, bucket.Close.Equal(decimal.NewFromFloat(51000))) // 更新
	assert.True(t, bucket.Volume.Equal(decimal.NewFromFloat(3)))
	assert.Equal(t, int32(2), bucket.TradeCount)

	// 第三笔成交 - 更低价格
	bucket.Update(
		decimal.NewFromFloat(49000),
		decimal.NewFromFloat(0.5),
		decimal.NewFromFloat(24500),
		1700000003000,
	)

	assert.True(t, bucket.Low.Equal(decimal.NewFromFloat(49000))) // 更新
	assert.True(t, bucket.Close.Equal(decimal.NewFromFloat(49000)))
	assert.Equal(t, int64(1700000003000), bucket.LastTrade)
}

func TestCalculateMinute(t *testing.T) {
	// 使用实际计算来验证
	tests := []int64{
		1700000000000,
		1700000030000,
		1700000059999,
		1700000060000,
		1700000090000,
	}

	for _, ts := range tests {
		result := CalculateMinute(ts)
		expected := (ts / 60000) * 60000
		assert.Equal(t, expected, result)
	}
}
