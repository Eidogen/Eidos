package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/aggregator"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// ============ Mock implementations ============

// mockKlineRepo 模拟 K 线仓储
type mockKlineRepo struct {
	mu     sync.Mutex
	klines []*model.Kline
}

func (m *mockKlineRepo) Upsert(ctx context.Context, kline *model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.klines = append(m.klines, kline)
	return nil
}

func (m *mockKlineRepo) BatchUpsert(ctx context.Context, klines []*model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.klines = append(m.klines, klines...)
	return nil
}

func (m *mockKlineRepo) Query(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*model.Kline
	for _, k := range m.klines {
		if k.Market == market && k.Interval == interval {
			if (startTime == 0 || k.OpenTime >= startTime) && (endTime == 0 || k.OpenTime <= endTime) {
				result = append(result, k)
			}
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

func (m *mockKlineRepo) GetLatest(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var latest *model.Kline
	for _, k := range m.klines {
		if k.Market == market && k.Interval == interval {
			if latest == nil || k.OpenTime > latest.OpenTime {
				latest = k
			}
		}
	}
	return latest, nil
}

// mockMarketRepo 模拟市场仓储
type mockMarketRepo struct {
	markets []*model.Market
}

func (m *mockMarketRepo) ListActiveMarkets(ctx context.Context) ([]*model.Market, error) {
	var result []*model.Market
	for _, market := range m.markets {
		if market.Status == model.MarketStatusActive {
			result = append(result, market)
		}
	}
	return result, nil
}

func (m *mockMarketRepo) GetBySymbol(ctx context.Context, symbol string) (*model.Market, error) {
	for _, market := range m.markets {
		if market.Symbol == symbol {
			return market, nil
		}
	}
	return nil, nil
}

func (m *mockMarketRepo) Create(ctx context.Context, market *model.Market) error {
	m.markets = append(m.markets, market)
	return nil
}

func (m *mockMarketRepo) Update(ctx context.Context, market *model.Market) error {
	for i, existing := range m.markets {
		if existing.Symbol == market.Symbol {
			m.markets[i] = market
			return nil
		}
	}
	return nil
}

// mockTradeRepo 模拟成交仓储
type mockTradeRepo struct {
	mu     sync.Mutex
	trades []*model.Trade
}

func (m *mockTradeRepo) Create(ctx context.Context, trade *model.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trades = append(m.trades, trade)
	return nil
}

func (m *mockTradeRepo) BatchCreate(ctx context.Context, trades []*model.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trades = append(m.trades, trades...)
	return nil
}

func (m *mockTradeRepo) ListRecent(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*model.Trade
	for i := len(m.trades) - 1; i >= 0 && len(result) < limit; i-- {
		if m.trades[i].Market == market {
			result = append(result, m.trades[i])
		}
	}
	return result, nil
}

func (m *mockTradeRepo) ListByTimeRange(ctx context.Context, market string, startTime, endTime int64, limit int) ([]*model.Trade, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*model.Trade
	for _, t := range m.trades {
		if t.Market == market {
			if (startTime == 0 || t.Timestamp >= startTime) && (endTime == 0 || t.Timestamp <= endTime) {
				result = append(result, t)
			}
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockTradeRepo) GetByID(ctx context.Context, tradeID string) (*model.Trade, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, trade := range m.trades {
		if trade.TradeID == tradeID {
			return trade, nil
		}
	}
	return nil, nil
}

// mockPublisher 模拟发布器
type mockPublisher struct {
	mu      sync.Mutex
	klines  []*model.Kline
	tickers []*model.Ticker
	depths  []*model.Depth
	trades  []*model.Trade
}

func (m *mockPublisher) PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.klines = append(m.klines, kline)
	return nil
}

func (m *mockPublisher) PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickers = append(m.tickers, ticker)
	return nil
}

func (m *mockPublisher) PublishDepth(ctx context.Context, market string, depth *model.Depth) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depths = append(m.depths, depth)
	return nil
}

func (m *mockPublisher) PublishTrade(ctx context.Context, market string, trade *model.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trades = append(m.trades, trade)
	return nil
}

// mockSnapshotProvider 模拟快照提供者
type mockSnapshotProvider struct{}

func (m *mockSnapshotProvider) GetSnapshot(ctx context.Context, market string) (*model.Depth, error) {
	return &model.Depth{
		Market:   market,
		Sequence: 0,
		Bids:     []*model.PriceLevel{},
		Asks:     []*model.PriceLevel{},
	}, nil
}

// ============ Test helpers ============

func newTestMarketService() (*MarketService, *mockKlineRepo, *mockMarketRepo, *mockTradeRepo, *mockPublisher) {
	klineRepo := &mockKlineRepo{}
	marketRepo := &mockMarketRepo{
		markets: []*model.Market{
			{
				Symbol:         "BTC-USDC",
				BaseToken:      "BTC",
				QuoteToken:     "USDC",
				Status:         model.MarketStatusActive,
				TradingEnabled: true,
			},
			{
				Symbol:         "ETH-USDC",
				BaseToken:      "ETH",
				QuoteToken:     "USDC",
				Status:         model.MarketStatusActive,
				TradingEnabled: true,
			},
		},
	}
	tradeRepo := &mockTradeRepo{}
	publisher := &mockPublisher{}
	snapshotProvider := &mockSnapshotProvider{}
	logger := zap.NewNop()

	config := MarketServiceConfig{
		KlineConfig: aggregator.KlineAggregatorConfig{
			FlushInterval: time.Hour, // 设置较长的间隔避免测试中触发
		},
		TickerConfig: aggregator.TickerCalculatorConfig{
			PublishInterval: time.Hour, // 设置较长的间隔避免测试中触发
		},
		DepthConfig: aggregator.DepthManagerConfig{
			MaxLevels:       100,
			PublishInterval: time.Hour, // 设置较长的间隔避免测试中触发
		},
	}

	svc := NewMarketService(klineRepo, marketRepo, tradeRepo, publisher, snapshotProvider, logger, config)
	return svc, klineRepo, marketRepo, tradeRepo, publisher
}

// ============ Tests ============

func TestMarketService_NewMarketService(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.aggregators)
}

func TestMarketService_InitMarkets(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	ctx := context.Background()

	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	// 应该初始化了两个市场
	assert.Len(t, svc.aggregators, 2)

	// 检查 BTC-USDC
	svc.mu.RLock()
	btcAgg := svc.aggregators["BTC-USDC"]
	ethAgg := svc.aggregators["ETH-USDC"]
	svc.mu.RUnlock()

	assert.NotNil(t, btcAgg)
	assert.NotNil(t, ethAgg)

	// 清理
	svc.Stop()
}

func TestMarketService_ProcessTrade(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 处理成交
	trade := &model.TradeEvent{
		TradeID:     "trade-1",
		Market:      "BTC-USDC",
		Price:       "50000",
		Amount:      "1.5",
		QuoteAmount: "75000",
		MakerSide:   1,
		Timestamp:   1700000000000,
	}

	err := svc.ProcessTrade(ctx, trade)
	require.NoError(t, err)

	// 等待异步操作
	time.Sleep(50 * time.Millisecond)

	// 验证聚合器已创建
	svc.mu.RLock()
	agg := svc.aggregators["BTC-USDC"]
	svc.mu.RUnlock()
	assert.NotNil(t, agg)
}

func TestMarketService_ProcessTrade_InvalidMarket(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	trade := &model.TradeEvent{
		TradeID: "trade-1",
		Market:  "", // 空市场
	}

	err := svc.ProcessTrade(ctx, trade)
	assert.Equal(t, ErrInvalidMarket, err)
}

func TestMarketService_ProcessOrderBookUpdate(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 先初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	// 处理订单簿更新
	update := &model.DepthUpdate{
		Market:   "BTC-USDC",
		Sequence: 1,
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(10)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
		},
	}

	err = svc.ProcessOrderBookUpdate(ctx, update)
	require.NoError(t, err)

	// 验证深度已更新
	depth, err := svc.GetDepth(ctx, "BTC-USDC", 10)
	require.NoError(t, err)
	assert.Len(t, depth.Bids, 1)
	assert.Len(t, depth.Asks, 1)
}

func TestMarketService_ProcessOrderBookUpdate_InvalidMarket(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	update := &model.DepthUpdate{
		Market: "", // 空市场
	}

	err := svc.ProcessOrderBookUpdate(ctx, update)
	assert.Equal(t, ErrInvalidMarket, err)
}

func TestMarketService_GetKlines(t *testing.T) {
	svc, klineRepo, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 添加一些 K 线数据
	klineRepo.klines = []*model.Kline{
		{Market: "BTC-USDC", Interval: model.Interval1m, OpenTime: 1700000000000},
		{Market: "BTC-USDC", Interval: model.Interval1m, OpenTime: 1700000060000},
		{Market: "BTC-USDC", Interval: model.Interval5m, OpenTime: 1700000000000},
	}

	klines, err := svc.GetKlines(ctx, "BTC-USDC", model.Interval1m, 0, 0, 10)
	require.NoError(t, err)
	assert.Len(t, klines, 2)
}

func TestMarketService_GetKlines_InvalidInterval(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	_, err := svc.GetKlines(ctx, "BTC-USDC", "invalid", 0, 0, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid interval")
}

func TestMarketService_GetCurrentKline(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	// 处理一笔成交以生成 K 线
	trade := &model.TradeEvent{
		TradeID:     "trade-1",
		Market:      "BTC-USDC",
		Price:       "50000",
		Amount:      "1.5",
		QuoteAmount: "75000",
		Timestamp:   time.Now().UnixMilli(),
	}
	svc.ProcessTrade(ctx, trade)

	kline, err := svc.GetCurrentKline(ctx, "BTC-USDC", model.Interval1m)
	require.NoError(t, err)
	assert.NotNil(t, kline)
}

func TestMarketService_GetCurrentKline_MarketNotFound(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	_, err := svc.GetCurrentKline(ctx, "UNKNOWN-USDC", model.Interval1m)
	assert.Equal(t, ErrMarketNotFound, err)
}

func TestMarketService_GetTicker(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	ticker, err := svc.GetTicker(ctx, "BTC-USDC")
	require.NoError(t, err)
	assert.NotNil(t, ticker)
	assert.Equal(t, "BTC-USDC", ticker.Market)
}

func TestMarketService_GetTicker_MarketNotFound(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	_, err := svc.GetTicker(ctx, "UNKNOWN-USDC")
	assert.Equal(t, ErrMarketNotFound, err)
}

func TestMarketService_GetAllTickers(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	tickers, err := svc.GetAllTickers(ctx)
	require.NoError(t, err)
	assert.Len(t, tickers, 2)
}

func TestMarketService_GetDepth(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	depth, err := svc.GetDepth(ctx, "BTC-USDC", 20)
	require.NoError(t, err)
	assert.NotNil(t, depth)
	assert.Equal(t, "BTC-USDC", depth.Market)
}

func TestMarketService_GetDepth_MarketNotFound(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	_, err := svc.GetDepth(ctx, "UNKNOWN-USDC", 20)
	assert.Equal(t, ErrMarketNotFound, err)
}

func TestMarketService_GetRecentTrades(t *testing.T) {
	svc, _, _, tradeRepo, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 添加一些成交
	tradeRepo.trades = []*model.Trade{
		{TradeID: "trade-1", Market: "BTC-USDC"},
		{TradeID: "trade-2", Market: "BTC-USDC"},
		{TradeID: "trade-3", Market: "ETH-USDC"},
	}

	trades, err := svc.GetRecentTrades(ctx, "BTC-USDC", 10)
	require.NoError(t, err)
	assert.Len(t, trades, 2)
}

func TestMarketService_GetRecentTrades_DefaultLimit(t *testing.T) {
	svc, _, _, tradeRepo, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	tradeRepo.trades = []*model.Trade{
		{TradeID: "trade-1", Market: "BTC-USDC"},
	}

	// 测试边界值
	trades, err := svc.GetRecentTrades(ctx, "BTC-USDC", 0)
	require.NoError(t, err)
	assert.Len(t, trades, 1)

	trades, err = svc.GetRecentTrades(ctx, "BTC-USDC", 200)
	require.NoError(t, err)
	assert.Len(t, trades, 1)
}

func TestMarketService_GetMarkets(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	markets, err := svc.GetMarkets(ctx)
	require.NoError(t, err)
	assert.Len(t, markets, 2)
}

func TestMarketService_GetMarket(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	market, err := svc.GetMarket(ctx, "BTC-USDC")
	require.NoError(t, err)
	assert.NotNil(t, market)
	assert.Equal(t, "BTC-USDC", market.Symbol)
}

func TestMarketService_Stats(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	stats := svc.Stats()
	assert.Len(t, stats, 2)
	assert.Contains(t, stats, "BTC-USDC")
	assert.Contains(t, stats, "ETH-USDC")
}

func TestMarketService_Stop(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	// 停止服务
	svc.Stop()

	// 可以多次调用 Stop
	svc.Stop()
}

func TestMarketService_Concurrent(t *testing.T) {
	svc, _, _, _, _ := newTestMarketService()
	defer svc.Stop()

	ctx := context.Background()

	// 初始化市场
	err := svc.InitMarkets(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 并发处理成交
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			trade := &model.TradeEvent{
				TradeID:     "trade-" + string(rune(idx)),
				Market:      "BTC-USDC",
				Price:       "50000",
				Amount:      "1",
				QuoteAmount: "50000",
				Timestamp:   time.Now().UnixMilli(),
			}
			svc.ProcessTrade(ctx, trade)
		}(i)
	}

	// 并发查询
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.GetTicker(ctx, "BTC-USDC")
			svc.GetDepth(ctx, "BTC-USDC", 20)
			svc.GetAllTickers(ctx)
		}()
	}

	wg.Wait()
}

func TestDefaultMarketServiceConfig(t *testing.T) {
	config := DefaultMarketServiceConfig()

	assert.NotZero(t, config.KlineConfig.FlushInterval)
	assert.NotZero(t, config.TickerConfig.PublishInterval)
	assert.NotZero(t, config.DepthConfig.MaxLevels)
}
