package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// ============ Mock Implementations ============

// mockKlineRepository 模拟 K 线仓储
type mockKlineRepository struct {
	klines      map[string][]*model.Kline // market:interval -> klines
	upsertError error
	queryError  error
}

func newMockKlineRepository() *mockKlineRepository {
	return &mockKlineRepository{
		klines: make(map[string][]*model.Kline),
	}
}

func (m *mockKlineRepository) Upsert(ctx context.Context, kline *model.Kline) error {
	if m.upsertError != nil {
		return m.upsertError
	}
	key := kline.Market + ":" + string(kline.Interval)
	m.klines[key] = append(m.klines[key], kline)
	return nil
}

func (m *mockKlineRepository) BatchUpsert(ctx context.Context, klines []*model.Kline) error {
	if m.upsertError != nil {
		return m.upsertError
	}
	for _, k := range klines {
		if err := m.Upsert(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockKlineRepository) Query(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {
	if m.queryError != nil {
		return nil, m.queryError
	}
	key := market + ":" + string(interval)
	klines := m.klines[key]

	// Filter by time range
	var result []*model.Kline
	for _, k := range klines {
		if startTime > 0 && k.OpenTime < startTime {
			continue
		}
		if endTime > 0 && k.OpenTime > endTime {
			continue
		}
		result = append(result, k)
	}

	// Apply limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (m *mockKlineRepository) GetLatest(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	if m.queryError != nil {
		return nil, m.queryError
	}
	key := market + ":" + string(interval)
	klines := m.klines[key]
	if len(klines) == 0 {
		return nil, nil
	}
	return klines[len(klines)-1], nil
}

// mockMarketRepository 模拟交易对仓储
type mockMarketRepository struct {
	markets     map[string]*model.Market
	createError error
	updateError error
}

func newMockMarketRepository() *mockMarketRepository {
	return &mockMarketRepository{
		markets: make(map[string]*model.Market),
	}
}

func (m *mockMarketRepository) ListActiveMarkets(ctx context.Context) ([]*model.Market, error) {
	var result []*model.Market
	for _, market := range m.markets {
		if market.Status == model.MarketStatusActive {
			result = append(result, market)
		}
	}
	return result, nil
}

func (m *mockMarketRepository) GetBySymbol(ctx context.Context, symbol string) (*model.Market, error) {
	market, ok := m.markets[symbol]
	if !ok {
		return nil, nil
	}
	return market, nil
}

func (m *mockMarketRepository) Create(ctx context.Context, market *model.Market) error {
	if m.createError != nil {
		return m.createError
	}
	m.markets[market.Symbol] = market
	return nil
}

func (m *mockMarketRepository) Update(ctx context.Context, market *model.Market) error {
	if m.updateError != nil {
		return m.updateError
	}
	m.markets[market.Symbol] = market
	return nil
}

// mockTradeRepository 模拟成交仓储
type mockTradeRepository struct {
	trades      map[string][]*model.Trade // market -> trades
	createError error
}

func newMockTradeRepository() *mockTradeRepository {
	return &mockTradeRepository{
		trades: make(map[string][]*model.Trade),
	}
}

func (m *mockTradeRepository) Create(ctx context.Context, trade *model.Trade) error {
	if m.createError != nil {
		return m.createError
	}
	m.trades[trade.Market] = append(m.trades[trade.Market], trade)
	return nil
}

func (m *mockTradeRepository) BatchCreate(ctx context.Context, trades []*model.Trade) error {
	if m.createError != nil {
		return m.createError
	}
	for _, t := range trades {
		if err := m.Create(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockTradeRepository) ListRecent(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	trades := m.trades[market]
	if limit > 0 && len(trades) > limit {
		return trades[len(trades)-limit:], nil
	}
	return trades, nil
}

func (m *mockTradeRepository) GetByID(ctx context.Context, tradeID string) (*model.Trade, error) {
	for _, trades := range m.trades {
		for _, t := range trades {
			if t.TradeID == tradeID {
				return t, nil
			}
		}
	}
	return nil, nil
}

// ============ Tests ============

func TestKlineRepository_Interface(t *testing.T) {
	repo := newMockKlineRepository()
	ctx := context.Background()

	t.Run("Upsert", func(t *testing.T) {
		kline := &model.Kline{
			Market:   "BTC-USDC",
			Interval: model.Interval1m,
			OpenTime: 1700000000000,
			Open:     decimal.NewFromFloat(50000),
			High:     decimal.NewFromFloat(50100),
			Low:      decimal.NewFromFloat(49900),
			Close:    decimal.NewFromFloat(50050),
		}

		err := repo.Upsert(ctx, kline)
		require.NoError(t, err)

		// Verify it was stored
		latest, err := repo.GetLatest(ctx, "BTC-USDC", model.Interval1m)
		require.NoError(t, err)
		assert.Equal(t, kline.OpenTime, latest.OpenTime)
	})

	t.Run("BatchUpsert", func(t *testing.T) {
		klines := []*model.Kline{
			{Market: "ETH-USDC", Interval: model.Interval1m, OpenTime: 1700000000000},
			{Market: "ETH-USDC", Interval: model.Interval1m, OpenTime: 1700000060000},
		}

		err := repo.BatchUpsert(ctx, klines)
		require.NoError(t, err)

		result, err := repo.Query(ctx, "ETH-USDC", model.Interval1m, 0, 0, 100)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("Query with time filter", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, 1700000000000, 1700000060000, 100)
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("Query with limit", func(t *testing.T) {
		// Add more klines
		for i := 0; i < 10; i++ {
			kline := &model.Kline{
				Market:   "LIMIT-TEST",
				Interval: model.Interval1m,
				OpenTime: int64(1700000000000 + i*60000),
			}
			_ = repo.Upsert(ctx, kline)
		}

		result, err := repo.Query(ctx, "LIMIT-TEST", model.Interval1m, 0, 0, 5)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("GetLatest not found", func(t *testing.T) {
		result, err := repo.GetLatest(ctx, "UNKNOWN", model.Interval1m)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Upsert error", func(t *testing.T) {
		repo.upsertError = errors.New("database error")
		err := repo.Upsert(ctx, &model.Kline{})
		require.Error(t, err)
		repo.upsertError = nil
	})
}

func TestMarketRepository_Interface(t *testing.T) {
	repo := newMockMarketRepository()
	ctx := context.Background()

	t.Run("Create and GetBySymbol", func(t *testing.T) {
		market := &model.Market{
			Symbol:     "BTC-USDC",
			BaseToken:  "BTC",
			QuoteToken: "USDC",
			Status:     model.MarketStatusActive,
		}

		err := repo.Create(ctx, market)
		require.NoError(t, err)

		result, err := repo.GetBySymbol(ctx, "BTC-USDC")
		require.NoError(t, err)
		assert.Equal(t, "BTC-USDC", result.Symbol)
	})

	t.Run("ListActiveMarkets", func(t *testing.T) {
		// Add inactive market
		inactiveMarket := &model.Market{
			Symbol: "INACTIVE-USDC",
			Status: model.MarketStatusInactive,
		}
		_ = repo.Create(ctx, inactiveMarket)

		// Add active market
		activeMarket := &model.Market{
			Symbol: "ETH-USDC",
			Status: model.MarketStatusActive,
		}
		_ = repo.Create(ctx, activeMarket)

		result, err := repo.ListActiveMarkets(ctx)
		require.NoError(t, err)

		// Should only contain active markets
		for _, m := range result {
			assert.Equal(t, model.MarketStatusActive, m.Status)
		}
	})

	t.Run("GetBySymbol not found", func(t *testing.T) {
		result, err := repo.GetBySymbol(ctx, "UNKNOWN")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Update", func(t *testing.T) {
		market := &model.Market{
			Symbol:         "BTC-USDC",
			TradingEnabled: false,
		}

		err := repo.Update(ctx, market)
		require.NoError(t, err)

		result, _ := repo.GetBySymbol(ctx, "BTC-USDC")
		assert.False(t, result.TradingEnabled)
	})
}

func TestTradeRepository_Interface(t *testing.T) {
	repo := newMockTradeRepository()
	ctx := context.Background()

	t.Run("Create and GetByID", func(t *testing.T) {
		trade := &model.Trade{
			TradeID:   "trade-001",
			Market:    "BTC-USDC",
			Price:     decimal.NewFromFloat(50000),
			Amount:    decimal.NewFromFloat(1.5),
			Timestamp: 1700000000000,
		}

		err := repo.Create(ctx, trade)
		require.NoError(t, err)

		result, err := repo.GetByID(ctx, "trade-001")
		require.NoError(t, err)
		assert.Equal(t, "trade-001", result.TradeID)
	})

	t.Run("BatchCreate", func(t *testing.T) {
		trades := []*model.Trade{
			{TradeID: "trade-002", Market: "ETH-USDC", Timestamp: 1700000001000},
			{TradeID: "trade-003", Market: "ETH-USDC", Timestamp: 1700000002000},
		}

		err := repo.BatchCreate(ctx, trades)
		require.NoError(t, err)

		result, err := repo.ListRecent(ctx, "ETH-USDC", 100)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("ListRecent with limit", func(t *testing.T) {
		// Add more trades
		for i := 0; i < 10; i++ {
			trade := &model.Trade{
				TradeID:   "trade-limit-" + string(rune('0'+i)),
				Market:    "LIMIT-TEST",
				Timestamp: int64(1700000000000 + i*1000),
			}
			_ = repo.Create(ctx, trade)
		}

		result, err := repo.ListRecent(ctx, "LIMIT-TEST", 5)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("GetByID not found", func(t *testing.T) {
		result, err := repo.GetByID(ctx, "unknown-trade")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Create error", func(t *testing.T) {
		repo.createError = errors.New("database error")
		err := repo.Create(ctx, &model.Trade{Market: "ERROR"})
		require.Error(t, err)
		repo.createError = nil
	})
}

func TestKlineRepository_BatchUpsertEmpty(t *testing.T) {
	repo := newMockKlineRepository()
	ctx := context.Background()

	err := repo.BatchUpsert(ctx, []*model.Kline{})
	require.NoError(t, err)
}

func TestTradeRepository_BatchCreateEmpty(t *testing.T) {
	repo := newMockTradeRepository()
	ctx := context.Background()

	err := repo.BatchCreate(ctx, []*model.Trade{})
	require.NoError(t, err)
}
