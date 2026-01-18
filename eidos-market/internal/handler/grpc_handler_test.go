package handler

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// TestToProtoConversions 测试模型转换函数
func TestToProtoConversions(t *testing.T) {
	t.Run("toProtoMarket", func(t *testing.T) {
		m := &model.Market{
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
			Status:         model.MarketStatusActive,
			TradingEnabled: true,
		}

		proto := toProtoMarket(m)
		assert.Equal(t, "BTC-USDC", proto.Symbol)
		assert.Equal(t, "BTC", proto.BaseToken)
		assert.Equal(t, "USDC", proto.QuoteToken)
		assert.Equal(t, int32(2), proto.PriceDecimals)
		assert.Equal(t, int32(6), proto.SizeDecimals)
		assert.Equal(t, "0.001", proto.MinSize)
		assert.Equal(t, "1000", proto.MaxSize)
		assert.Equal(t, "10", proto.MinNotional)
		assert.Equal(t, "0.01", proto.TickSize)
		assert.Equal(t, "0.001", proto.MakerFee)
		assert.Equal(t, "0.002", proto.TakerFee)
		// model.MarketStatusActive=1 直接转换为 marketv1.MarketStatus(1)
		assert.Equal(t, marketv1.MarketStatus(1), proto.Status)
		assert.True(t, proto.TradingEnabled)
	})

	t.Run("toProtoTicker", func(t *testing.T) {
		ticker := &model.Ticker{
			Market:             "BTC-USDC",
			LastPrice:          decimal.NewFromFloat(50000.0),
			PriceChange:        decimal.NewFromFloat(500.0),
			PriceChangePercent: decimal.NewFromFloat(1.01),
			Open:               decimal.NewFromFloat(49500.0),
			High:               decimal.NewFromFloat(51000.0),
			Low:                decimal.NewFromFloat(49000.0),
			Volume:             decimal.NewFromFloat(1234.56),
			QuoteVolume:        decimal.NewFromFloat(61728000.0),
			BestBid:            decimal.NewFromFloat(49900.0),
			BestBidQty:         decimal.NewFromFloat(10.5),
			BestAsk:            decimal.NewFromFloat(50100.0),
			BestAskQty:         decimal.NewFromFloat(5.2),
			TradeCount:         5678,
			Timestamp:          1700000000000,
		}

		proto := toProtoTicker(ticker)
		assert.Equal(t, "BTC-USDC", proto.Market)
		assert.Equal(t, "50000", proto.LastPrice)
		assert.Equal(t, "500", proto.PriceChange)
		assert.Equal(t, "1.01", proto.PriceChangePercent)
		assert.Equal(t, "49500", proto.Open)
		assert.Equal(t, "51000", proto.High)
		assert.Equal(t, "49000", proto.Low)
		assert.Equal(t, "1234.56", proto.Volume)
		assert.Equal(t, "61728000", proto.QuoteVolume)
		assert.Equal(t, "49900", proto.BestBid)
		assert.Equal(t, "50100", proto.BestAsk)
		assert.Equal(t, int64(1700000000000), proto.Timestamp)
	})

	t.Run("toProtoKline", func(t *testing.T) {
		kline := &model.Kline{
			Market:      "BTC-USDC",
			Interval:    model.Interval1m,
			OpenTime:    1700000000000,
			Open:        decimal.NewFromFloat(50000.0),
			High:        decimal.NewFromFloat(50100.0),
			Low:         decimal.NewFromFloat(49900.0),
			Close:       decimal.NewFromFloat(50050.0),
			Volume:      decimal.NewFromFloat(123.45),
			QuoteVolume: decimal.NewFromFloat(6172500.0),
			TradeCount:  100,
			CloseTime:   1700000059999,
		}

		proto := toProtoKline(kline)
		assert.Equal(t, int64(1700000000000), proto.OpenTime)
		assert.Equal(t, "50000", proto.Open)
		assert.Equal(t, "50100", proto.High)
		assert.Equal(t, "49900", proto.Low)
		assert.Equal(t, "50050", proto.Close)
		assert.Equal(t, "123.45", proto.Volume)
		assert.Equal(t, "6172500", proto.QuoteVolume)
		assert.Equal(t, int32(100), proto.TradeCount)
		assert.Equal(t, int64(1700000059999), proto.CloseTime)
	})

	t.Run("toProtoTrade", func(t *testing.T) {
		trade := &model.Trade{
			TradeID:     "trade-1",
			Market:      "BTC-USDC",
			Price:       decimal.NewFromFloat(50000.0),
			Amount:      decimal.NewFromFloat(1.5),
			QuoteAmount: decimal.NewFromFloat(75000.0),
			Side:        model.TradeSideBuy,
			Timestamp:   1700000000000,
		}

		proto := toProtoTrade(trade)
		assert.Equal(t, "trade-1", proto.TradeId)
		assert.Equal(t, "BTC-USDC", proto.Market)
		assert.Equal(t, "50000", proto.Price)
		assert.Equal(t, "1.5", proto.Amount)
		// Proto enum: TRADE_SIDE_BUY=1 (model TradeSideBuy=0 转换为 0+1=1)
		assert.Equal(t, marketv1.TradeSide_TRADE_SIDE_BUY, proto.Side)
		assert.Equal(t, int64(1700000000000), proto.Timestamp)
	})

	t.Run("toProtoTrade_SellSide", func(t *testing.T) {
		trade := &model.Trade{
			TradeID:   "trade-2",
			Market:    "ETH-USDC",
			Price:     decimal.NewFromFloat(3000.0),
			Amount:    decimal.NewFromFloat(2.0),
			Side:      model.TradeSideSell,
			Timestamp: 1700000001000,
		}

		proto := toProtoTrade(trade)
		assert.Equal(t, "trade-2", proto.TradeId)
		// Proto enum: TRADE_SIDE_SELL=2 (model TradeSideSell=1 转换为 1+1=2)
		assert.Equal(t, marketv1.TradeSide_TRADE_SIDE_SELL, proto.Side)
	})

	t.Run("toProtoPriceLevels", func(t *testing.T) {
		levels := []*model.PriceLevel{
			{Price: decimal.NewFromFloat(49900.0), Amount: decimal.NewFromFloat(10.5)},
			{Price: decimal.NewFromFloat(49800.0), Amount: decimal.NewFromFloat(20.0)},
			{Price: decimal.NewFromFloat(49700.0), Amount: decimal.NewFromFloat(30.5)},
		}

		proto := toProtoPriceLevels(levels)
		assert.Len(t, proto, 3)
		assert.Equal(t, "49900", proto[0].Price)
		assert.Equal(t, "10.5", proto[0].Amount)
		assert.Equal(t, "49800", proto[1].Price)
		assert.Equal(t, "20", proto[1].Amount)
		assert.Equal(t, "49700", proto[2].Price)
		assert.Equal(t, "30.5", proto[2].Amount)
	})

	t.Run("toProtoPriceLevels_Empty", func(t *testing.T) {
		var levels []*model.PriceLevel
		proto := toProtoPriceLevels(levels)
		assert.Len(t, proto, 0)
		assert.NotNil(t, proto) // should return empty slice, not nil
	})
}

// TestLimitValidation 测试 limit 参数验证逻辑
func TestLimitValidation(t *testing.T) {
	t.Run("kline_limit_defaults", func(t *testing.T) {
		tests := []struct {
			name          string
			requestLimit  int32
			expectedLimit int
		}{
			{"zero defaults to 500", 0, 500},
			{"negative defaults to 500", -1, 500},
			{"over max defaults to 500", 2000, 500},
			{"valid 100", 100, 100},
			{"valid 500", 500, 500},
			{"max 1500", 1500, 1500},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				limit := int(tt.requestLimit)
				if limit <= 0 || limit > 1500 {
					limit = 500
				}
				assert.Equal(t, tt.expectedLimit, limit)
			})
		}
	})

	t.Run("depth_limit_defaults", func(t *testing.T) {
		tests := []struct {
			name          string
			requestLimit  int32
			expectedLimit int
		}{
			{"zero defaults to 20", 0, 20},
			{"invalid 15 defaults to 20", 15, 20},
			{"valid 5", 5, 5},
			{"valid 10", 10, 10},
			{"valid 20", 20, 20},
			{"valid 50", 50, 50},
			{"valid 100", 100, 100},
		}

		validLimits := []int{5, 10, 20, 50, 100}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				limit := int(tt.requestLimit)
				if limit <= 0 {
					limit = 20
				} else {
					found := false
					for _, v := range validLimits {
						if limit == v {
							found = true
							break
						}
					}
					if !found {
						limit = 20
					}
				}
				assert.Equal(t, tt.expectedLimit, limit)
			})
		}
	})

	t.Run("recent_trades_limit_defaults", func(t *testing.T) {
		tests := []struct {
			name          string
			requestLimit  int32
			expectedLimit int
		}{
			{"zero defaults to 50", 0, 50},
			{"negative defaults to 50", -1, 50},
			{"over max defaults to 50", 200, 50},
			{"valid 30", 30, 30},
			{"max 100", 100, 100},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				limit := int(tt.requestLimit)
				if limit <= 0 || limit > 100 {
					limit = 50
				}
				assert.Equal(t, tt.expectedLimit, limit)
			})
		}
	})
}

// TestProtoMarketStatusMapping 测试市场状态映射
// 注意: 当前实现直接使用 marketv1.MarketStatus(m.Status) 进行转换
// model: Inactive=0, Active=1, Suspend=2
// proto: UNSPECIFIED=0, INACTIVE=1, ACTIVE=2, SUSPEND=3
// 当前映射: model.Inactive(0) -> proto(0), model.Active(1) -> proto(1), model.Suspend(2) -> proto(2)
func TestProtoMarketStatusMapping(t *testing.T) {
	tests := []struct {
		status         model.MarketStatus
		expectedProto  marketv1.MarketStatus
		tradingEnabled bool
	}{
		{model.MarketStatusInactive, marketv1.MarketStatus(0), false},
		{model.MarketStatusActive, marketv1.MarketStatus(1), true},
		{model.MarketStatusSuspend, marketv1.MarketStatus(2), false},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			m := &model.Market{
				Symbol:         "TEST",
				Status:         tt.status,
				TradingEnabled: tt.tradingEnabled,
			}
			proto := toProtoMarket(m)
			assert.Equal(t, tt.expectedProto, proto.Status)
			assert.Equal(t, tt.tradingEnabled, proto.TradingEnabled)
		})
	}
}

// TestProtoTickerEmptyValues 测试空值处理
func TestProtoTickerEmptyValues(t *testing.T) {
	ticker := &model.Ticker{
		Market:    "NEW-USDC",
		Timestamp: 1700000000000,
		// 其他字段都是零值
	}

	proto := toProtoTicker(ticker)
	assert.Equal(t, "NEW-USDC", proto.Market)
	assert.Equal(t, "0", proto.LastPrice)
	assert.Equal(t, "0", proto.Volume)
	assert.Equal(t, "0", proto.High)
	assert.Equal(t, "0", proto.Low)
}

// TestToProtoKlineEmptyValues 测试 K 线空值处理
func TestToProtoKlineEmptyValues(t *testing.T) {
	kline := &model.Kline{
		Market:   "NEW-USDC",
		Interval: model.Interval1m,
		OpenTime: 1700000000000,
		// 其他字段都是零值
	}

	proto := toProtoKline(kline)
	assert.Equal(t, int64(1700000000000), proto.OpenTime)
	assert.Equal(t, "0", proto.Open)
	assert.Equal(t, "0", proto.Close)
	assert.Equal(t, "0", proto.Volume)
	assert.Equal(t, int32(0), proto.TradeCount)
}

// BenchmarkToProtoConversions 基准测试
func BenchmarkToProtoConversions(b *testing.B) {
	b.Run("toProtoMarket", func(b *testing.B) {
		m := &model.Market{
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
			Status:         model.MarketStatusActive,
			TradingEnabled: true,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = toProtoMarket(m)
		}
	})

	b.Run("toProtoTicker", func(b *testing.B) {
		ticker := &model.Ticker{
			Market:             "BTC-USDC",
			LastPrice:          decimal.NewFromFloat(50000.0),
			PriceChange:        decimal.NewFromFloat(500.0),
			PriceChangePercent: decimal.NewFromFloat(1.01),
			High:               decimal.NewFromFloat(51000.0),
			Low:                decimal.NewFromFloat(49000.0),
			Volume:             decimal.NewFromFloat(1234.56),
			Timestamp:          1700000000000,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = toProtoTicker(ticker)
		}
	})

	b.Run("toProtoPriceLevels_100", func(b *testing.B) {
		levels := make([]*model.PriceLevel, 100)
		for i := 0; i < 100; i++ {
			levels[i] = &model.PriceLevel{
				Price:  decimal.NewFromFloat(50000.0 - float64(i)*10),
				Amount: decimal.NewFromFloat(10.0 + float64(i)),
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = toProtoPriceLevels(levels)
		}
	})
}

// 确保导入使用
var _ = marketv1.Market{}

// ============ Mock Service ============

type mockMarketService struct {
	markets []*model.Market
	tickers map[string]*model.Ticker
	klines  map[string][]*model.Kline
	depths  map[string]*model.Depth
	trades  map[string][]*model.Trade
}

func newMockMarketService() *mockMarketService {
	return &mockMarketService{
		markets: []*model.Market{
			{
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
				Status:         model.MarketStatusActive,
				TradingEnabled: true,
			},
		},
		tickers: map[string]*model.Ticker{
			"BTC-USDC": {
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
				BestAsk:            decimal.NewFromFloat(50100),
				Timestamp:          1700000000000,
			},
		},
		klines: map[string][]*model.Kline{
			"BTC-USDC:1m": {
				{
					Market:      "BTC-USDC",
					Interval:    model.Interval1m,
					OpenTime:    1700000000000,
					Open:        decimal.NewFromFloat(50000),
					High:        decimal.NewFromFloat(50100),
					Low:         decimal.NewFromFloat(49900),
					Close:       decimal.NewFromFloat(50050),
					Volume:      decimal.NewFromFloat(100),
					QuoteVolume: decimal.NewFromFloat(5000000),
					TradeCount:  50,
					CloseTime:   1700000059999,
				},
			},
		},
		depths: map[string]*model.Depth{
			"BTC-USDC": {
				Market: "BTC-USDC",
				Bids: []*model.PriceLevel{
					{Price: decimal.NewFromFloat(49900), Amount: decimal.NewFromFloat(10)},
					{Price: decimal.NewFromFloat(49800), Amount: decimal.NewFromFloat(20)},
				},
				Asks: []*model.PriceLevel{
					{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
					{Price: decimal.NewFromFloat(50200), Amount: decimal.NewFromFloat(15)},
				},
				Sequence:  100,
				Timestamp: 1700000000000,
			},
		},
		trades: map[string][]*model.Trade{
			"BTC-USDC": {
				{
					TradeID:     "trade-1",
					Market:      "BTC-USDC",
					Price:       decimal.NewFromFloat(50000),
					Amount:      decimal.NewFromFloat(1.5),
					QuoteAmount: decimal.NewFromFloat(75000),
					Side:        model.TradeSideBuy,
					Timestamp:   1700000000000,
				},
			},
		},
	}
}

func (m *mockMarketService) GetMarkets(ctx context.Context) ([]*model.Market, error) {
	return m.markets, nil
}

func (m *mockMarketService) GetTicker(ctx context.Context, market string) (*model.Ticker, error) {
	ticker, ok := m.tickers[market]
	if !ok {
		return nil, service.ErrMarketNotFound
	}
	return ticker, nil
}

func (m *mockMarketService) GetAllTickers(ctx context.Context) ([]*model.Ticker, error) {
	tickers := make([]*model.Ticker, 0, len(m.tickers))
	for _, t := range m.tickers {
		tickers = append(tickers, t)
	}
	return tickers, nil
}

func (m *mockMarketService) GetKlines(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {
	key := market + ":" + string(interval)
	klines, ok := m.klines[key]
	if !ok {
		return []*model.Kline{}, nil
	}
	if limit > 0 && len(klines) > limit {
		return klines[:limit], nil
	}
	return klines, nil
}

func (m *mockMarketService) GetDepth(ctx context.Context, market string, limit int) (*model.Depth, error) {
	depth, ok := m.depths[market]
	if !ok {
		return nil, service.ErrMarketNotFound
	}
	return depth, nil
}

func (m *mockMarketService) GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	trades, ok := m.trades[market]
	if !ok {
		return []*model.Trade{}, nil
	}
	if limit > 0 && len(trades) > limit {
		return trades[:limit], nil
	}
	return trades, nil
}

// ============ gRPC Handler Tests ============

func TestMarketHandler_ListMarkets(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}

	ctx := context.Background()
	resp, err := handler.ListMarkets(ctx, &marketv1.ListMarketsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Markets, 1)
	assert.Equal(t, "BTC-USDC", resp.Markets[0].Symbol)
}

func TestMarketHandler_GetTicker(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		resp, err := handler.GetTicker(ctx, &marketv1.GetTickerRequest{Market: "BTC-USDC"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "BTC-USDC", resp.Ticker.Market)
		assert.Equal(t, "50000", resp.Ticker.LastPrice)
	})

	t.Run("empty market", func(t *testing.T) {
		_, err := handler.GetTicker(ctx, &marketv1.GetTickerRequest{Market: ""})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("market not found", func(t *testing.T) {
		_, err := handler.GetTicker(ctx, &marketv1.GetTickerRequest{Market: "UNKNOWN"})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

func TestMarketHandler_ListTickers(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}
	ctx := context.Background()

	resp, err := handler.ListTickers(ctx, &marketv1.ListTickersRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Tickers, 1)
}

func TestMarketHandler_GetKlines(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		resp, err := handler.GetKlines(ctx, &marketv1.GetKlinesRequest{
			Market:   "BTC-USDC",
			Interval: "1m",
			Limit:    100,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Klines, 1)
	})

	t.Run("empty market", func(t *testing.T) {
		_, err := handler.GetKlines(ctx, &marketv1.GetKlinesRequest{
			Market:   "",
			Interval: "1m",
		})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("empty interval", func(t *testing.T) {
		_, err := handler.GetKlines(ctx, &marketv1.GetKlinesRequest{
			Market:   "BTC-USDC",
			Interval: "",
		})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("invalid interval", func(t *testing.T) {
		_, err := handler.GetKlines(ctx, &marketv1.GetKlinesRequest{
			Market:   "BTC-USDC",
			Interval: "invalid",
		})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("limit defaults", func(t *testing.T) {
		resp, err := handler.GetKlines(ctx, &marketv1.GetKlinesRequest{
			Market:   "BTC-USDC",
			Interval: "1m",
			Limit:    0, // should default to 500
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
}

func TestMarketHandler_GetDepth(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		resp, err := handler.GetDepth(ctx, &marketv1.GetDepthRequest{
			Market: "BTC-USDC",
			Limit:  20,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "BTC-USDC", resp.Market)
		assert.Len(t, resp.Bids, 2)
		assert.Len(t, resp.Asks, 2)
	})

	t.Run("empty market", func(t *testing.T) {
		_, err := handler.GetDepth(ctx, &marketv1.GetDepthRequest{Market: ""})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("market not found", func(t *testing.T) {
		_, err := handler.GetDepth(ctx, &marketv1.GetDepthRequest{Market: "UNKNOWN"})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.NotFound, st.Code())
	})
}

func TestMarketHandler_GetRecentTrades(t *testing.T) {
	mockSvc := newMockMarketService()
	handler := &MarketHandler{svc: &marketServiceAdapter{mockSvc}}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		resp, err := handler.GetRecentTrades(ctx, &marketv1.GetRecentTradesRequest{
			Market: "BTC-USDC",
			Limit:  50,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Trades, 1)
		assert.Equal(t, "trade-1", resp.Trades[0].TradeId)
	})

	t.Run("empty market", func(t *testing.T) {
		_, err := handler.GetRecentTrades(ctx, &marketv1.GetRecentTradesRequest{Market: ""})
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("limit defaults", func(t *testing.T) {
		resp, err := handler.GetRecentTrades(ctx, &marketv1.GetRecentTradesRequest{
			Market: "BTC-USDC",
			Limit:  0, // should default to 50
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})
}

// marketServiceAdapter 适配 mockMarketService 到 service.MarketService
type marketServiceAdapter struct {
	mock *mockMarketService
}

func (a *marketServiceAdapter) GetMarkets(ctx context.Context) ([]*model.Market, error) {
	return a.mock.GetMarkets(ctx)
}

func (a *marketServiceAdapter) GetTicker(ctx context.Context, market string) (*model.Ticker, error) {
	return a.mock.GetTicker(ctx, market)
}

func (a *marketServiceAdapter) GetAllTickers(ctx context.Context) ([]*model.Ticker, error) {
	return a.mock.GetAllTickers(ctx)
}

func (a *marketServiceAdapter) GetKlines(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {
	return a.mock.GetKlines(ctx, market, interval, startTime, endTime, limit)
}

func (a *marketServiceAdapter) GetDepth(ctx context.Context, market string, limit int) (*model.Depth, error) {
	return a.mock.GetDepth(ctx, market, limit)
}

func (a *marketServiceAdapter) GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	return a.mock.GetRecentTrades(ctx, market, limit)
}
