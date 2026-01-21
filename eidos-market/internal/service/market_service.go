package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-market/internal/aggregator"
	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
)

var (
	ErrMarketNotFound = errors.New("market not found")
	ErrInvalidMarket  = errors.New("invalid market")
)

// Publisher 统一发布接口
type Publisher interface {
	aggregator.KlinePublisher
	aggregator.TickerPublisher
	aggregator.DepthPublisher
	PublishTrade(ctx context.Context, market string, trade *model.Trade) error
}

// MarketServiceConfig 服务配置
type MarketServiceConfig struct {
	KlineConfig  aggregator.KlineAggregatorConfig
	TickerConfig aggregator.TickerCalculatorConfig
	DepthConfig  aggregator.DepthManagerConfig
}

// DefaultMarketServiceConfig 默认配置
func DefaultMarketServiceConfig() MarketServiceConfig {
	return MarketServiceConfig{
		KlineConfig:  aggregator.DefaultKlineAggregatorConfig(),
		TickerConfig: aggregator.DefaultTickerCalculatorConfig(),
		DepthConfig:  aggregator.DefaultDepthManagerConfig(),
	}
}

// MarketAggregator 单市场聚合器集合
type MarketAggregator struct {
	kline  *aggregator.KlineAggregator
	ticker *aggregator.TickerCalculator
	depth  *aggregator.DepthManager
}

// MarketService 行情服务
type MarketService struct {
	config      MarketServiceConfig
	aggregators map[string]*MarketAggregator // market -> aggregator
	mu          sync.RWMutex

	klineRepo  repository.KlineRepository
	marketRepo repository.MarketRepository
	tradeRepo  repository.TradeRepository

	publisher        Publisher
	snapshotProvider aggregator.DepthSnapshotProvider
	logger           *slog.Logger

	// 后台任务管理
	asyncWg sync.WaitGroup
}

// NewMarketService 创建行情服务
func NewMarketService(
	klineRepo repository.KlineRepository,
	marketRepo repository.MarketRepository,
	tradeRepo repository.TradeRepository,
	publisher Publisher,
	snapshotProvider aggregator.DepthSnapshotProvider,
	logger *slog.Logger,
	config MarketServiceConfig,
) *MarketService {
	return &MarketService{
		config:           config,
		aggregators:      make(map[string]*MarketAggregator),
		klineRepo:        klineRepo,
		marketRepo:       marketRepo,
		tradeRepo:        tradeRepo,
		publisher:        publisher,
		snapshotProvider: snapshotProvider,
		logger:           logger,
	}
}

// InitMarkets 初始化所有活跃的市场
func (s *MarketService) InitMarkets(ctx context.Context) error {
	markets, err := s.marketRepo.ListActiveMarkets(ctx)
	if err != nil {
		return err
	}

	for _, market := range markets {
		s.initMarket(market.Symbol)
	}

	s.logger.Info("markets initialized", "count", len(markets))
	return nil
}

// initMarket 初始化单个市场
func (s *MarketService) initMarket(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.aggregators[symbol]; exists {
		return
	}

	logger := s.logger.With("market", symbol)

	agg := &MarketAggregator{
		kline: aggregator.NewKlineAggregator(
			symbol,
			s.klineRepo,
			s.publisher,
			logger,
			s.config.KlineConfig,
		),
		ticker: aggregator.NewTickerCalculator(
			symbol,
			s.publisher,
			logger,
			s.config.TickerConfig,
		),
		depth: aggregator.NewDepthManager(
			symbol,
			s.publisher,
			s.snapshotProvider,
			logger,
			s.config.DepthConfig,
		),
	}

	// 启动聚合器
	agg.kline.Start()
	agg.ticker.Start()
	agg.depth.Start()

	s.aggregators[symbol] = agg
	metrics.ActiveMarkets.Inc()
	s.logger.Info("market initialized", "symbol", symbol)
}

// getOrCreateAggregator 获取或创建市场聚合器
func (s *MarketService) getOrCreateAggregator(market string) *MarketAggregator {
	s.mu.RLock()
	agg, exists := s.aggregators[market]
	s.mu.RUnlock()

	if !exists {
		s.initMarket(market)
		s.mu.RLock()
		agg = s.aggregators[market]
		s.mu.RUnlock()
	}

	return agg
}

// ProcessTrade 处理成交事件
func (s *MarketService) ProcessTrade(ctx context.Context, trade *model.TradeEvent) error {
	if trade.Market == "" {
		metrics.TradeProcessingErrors.WithLabelValues(trade.Market, "invalid_market").Inc()
		return ErrInvalidMarket
	}

	startTime := time.Now()
	defer func() {
		metrics.TradeProcessingDuration.WithLabelValues(trade.Market).Observe(time.Since(startTime).Seconds())
	}()

	agg := s.getOrCreateAggregator(trade.Market)

	// 更新 K 线
	if err := agg.kline.ProcessTrade(ctx, trade); err != nil {
		metrics.TradeProcessingErrors.WithLabelValues(trade.Market, "kline_error").Inc()
		return err
	}

	// 更新 Ticker
	if err := agg.ticker.ProcessTrade(ctx, trade); err != nil {
		metrics.TradeProcessingErrors.WithLabelValues(trade.Market, "ticker_error").Inc()
		return err
	}

	// 转换成交事件
	t, err := trade.ToTrade()
	if err != nil {
		s.logger.Error("failed to convert trade event",
			"trade_id", trade.TradeID,
			"error", err)
		metrics.TradeProcessingErrors.WithLabelValues(trade.Market, "convert_error").Inc()
		return err
	}

	// 保存成交历史（异步，带超时）
	if s.tradeRepo != nil {
		s.asyncWg.Add(1)
		go func() {
			defer s.asyncWg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.tradeRepo.Create(ctx, t); err != nil {
				s.logger.Error("failed to save trade", "error", err)
				metrics.TradeProcessingErrors.WithLabelValues(trade.Market, "db_error").Inc()
			}
		}()
	}

	// 发布成交事件（异步，带超时）
	if s.publisher != nil {
		s.asyncWg.Add(1)
		go func() {
			defer s.asyncWg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.publisher.PublishTrade(ctx, trade.Market, t); err != nil {
				s.logger.Warn("failed to publish trade", "error", err)
				metrics.PubSubPublishErrors.WithLabelValues("trade").Inc()
			} else {
				metrics.PubSubPublishTotal.WithLabelValues("trade").Inc()
			}
		}()
	}

	metrics.TradesProcessedTotal.WithLabelValues(trade.Market).Inc()
	return nil
}

// ProcessOrderBookUpdate 处理订单簿更新
func (s *MarketService) ProcessOrderBookUpdate(ctx context.Context, update *model.DepthUpdate) error {
	if update.Market == "" {
		return ErrInvalidMarket
	}

	agg := s.getOrCreateAggregator(update.Market)

	// 更新深度
	if err := agg.depth.ApplyUpdate(ctx, update); err != nil {
		return err
	}

	// 更新 Ticker 的最优价格
	bestBid, bestBidQty, bestAsk, bestAskQty := agg.depth.GetBestPrices()
	agg.ticker.UpdateBestPrices(bestBid, bestBidQty, bestAsk, bestAskQty)

	metrics.DepthUpdatesTotal.WithLabelValues(update.Market).Inc()
	return nil
}

// GetKlines 查询 K 线
func (s *MarketService) GetKlines(
	ctx context.Context,
	market string,
	interval model.KlineInterval,
	startTime, endTime int64,
	limit int,
) ([]*model.Kline, error) {
	if !model.IsValidInterval(string(interval)) {
		return nil, errors.New("invalid interval")
	}

	return s.klineRepo.Query(ctx, market, interval, startTime, endTime, limit)
}

// GetCurrentKline 获取当前 K 线
func (s *MarketService) GetCurrentKline(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	s.mu.RLock()
	agg, exists := s.aggregators[market]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrMarketNotFound
	}

	kline := agg.kline.GetKline(interval)
	if kline == nil {
		return nil, ErrMarketNotFound
	}

	return kline, nil
}

// GetTicker 查询 Ticker
func (s *MarketService) GetTicker(ctx context.Context, market string) (*model.Ticker, error) {
	s.mu.RLock()
	agg, exists := s.aggregators[market]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrMarketNotFound
	}

	return agg.ticker.GetTicker(), nil
}

// GetAllTickers 查询所有 Ticker
func (s *MarketService) GetAllTickers(ctx context.Context) ([]*model.Ticker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tickers := make([]*model.Ticker, 0, len(s.aggregators))
	for _, agg := range s.aggregators {
		tickers = append(tickers, agg.ticker.GetTicker())
	}

	return tickers, nil
}

// GetDepth 查询深度
func (s *MarketService) GetDepth(ctx context.Context, market string, limit int) (*model.Depth, error) {
	s.mu.RLock()
	agg, exists := s.aggregators[market]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrMarketNotFound
	}

	// 标准化档位数
	limit = model.NormalizeDepthLimit(limit)

	return agg.depth.GetDepth(limit), nil
}

// GetRecentTrades 查询最近成交
func (s *MarketService) GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	return s.tradeRepo.ListRecent(ctx, market, limit)
}

// GetMarkets 获取所有交易对
func (s *MarketService) GetMarkets(ctx context.Context) ([]*model.Market, error) {
	return s.marketRepo.ListActiveMarkets(ctx)
}

// GetMarket 获取单个交易对
func (s *MarketService) GetMarket(ctx context.Context, symbol string) (*model.Market, error) {
	return s.marketRepo.GetBySymbol(ctx, symbol)
}

// Stop 停止服务
func (s *MarketService) Stop() {
	// 等待所有异步任务完成
	s.logger.Info("waiting for async tasks to complete")
	s.asyncWg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()

	for symbol, agg := range s.aggregators {
		agg.kline.Stop()
		agg.ticker.Stop()
		agg.depth.Stop()
		s.logger.Info("market stopped", "symbol", symbol)
	}

	s.logger.Info("market service stopped")
}

// Stats 返回统计信息
func (s *MarketService) Stats() map[string]map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]map[string]int64)
	for symbol, agg := range s.aggregators {
		stats[symbol] = map[string]int64{}
		for k, v := range agg.kline.Stats() {
			stats[symbol]["kline_"+k] = v
		}
		for k, v := range agg.ticker.Stats() {
			stats[symbol]["ticker_"+k] = v
		}
		for k, v := range agg.depth.Stats() {
			stats[symbol]["depth_"+k] = v
		}
	}

	return stats
}
