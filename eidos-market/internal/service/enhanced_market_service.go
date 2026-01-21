// Package service provides market data services.
package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/aggregator"
	"github.com/eidos-exchange/eidos/eidos-market/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
	msync "github.com/eidos-exchange/eidos/eidos-market/internal/sync"
)

// EnhancedMarketServiceConfig configuration for enhanced service
type EnhancedMarketServiceConfig struct {
	// Base service config
	KlineConfig  aggregator.KlineAggregatorConfig
	TickerConfig aggregator.TickerCalculatorConfig
	DepthConfig  aggregator.DepthManagerConfig

	// Cache config
	CacheConfig cache.CacheManagerConfig

	// Market sync config
	MarketSyncConfig msync.MarketConfigSyncConfig

	// Enable cache-first queries
	EnableCacheFirst bool

	// Cache TTLs
	TickerCacheTTL time.Duration
	DepthCacheTTL  time.Duration
	KlineCacheTTL  time.Duration
}

// DefaultEnhancedMarketServiceConfig returns default configuration
func DefaultEnhancedMarketServiceConfig() EnhancedMarketServiceConfig {
	return EnhancedMarketServiceConfig{
		KlineConfig:      aggregator.DefaultKlineAggregatorConfig(),
		TickerConfig:     aggregator.DefaultTickerCalculatorConfig(),
		DepthConfig:      aggregator.DefaultDepthManagerConfig(),
		CacheConfig:      cache.DefaultCacheManagerConfig(),
		MarketSyncConfig: msync.DefaultMarketConfigSyncConfig(),
		EnableCacheFirst: true,
		TickerCacheTTL:   10 * time.Second,
		DepthCacheTTL:    5 * time.Second,
		KlineCacheTTL:    60 * time.Second,
	}
}

// EnhancedMarketService is the market service with cache integration
type EnhancedMarketService struct {
	*MarketService

	config       EnhancedMarketServiceConfig
	cacheManager *cache.CacheManager
	marketSync   *msync.MarketConfigSync

	// Write-behind buffer for trades
	tradeBuffer     []*model.Trade
	tradeBufferMu   sync.Mutex
	tradeBufferSize int
	tradeFlushCh    chan struct{}

	logger *zap.Logger

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewEnhancedMarketService creates a new enhanced market service
func NewEnhancedMarketService(
	klineRepo repository.KlineRepository,
	marketRepo repository.MarketRepository,
	tradeRepo repository.TradeRepository,
	cacheManager *cache.CacheManager,
	publisher Publisher,
	snapshotProvider aggregator.DepthSnapshotProvider,
	logger *zap.Logger,
	config EnhancedMarketServiceConfig,
) *EnhancedMarketService {
	ctx, cancel := context.WithCancel(context.Background())

	// Create base market service
	baseSvc := NewMarketService(
		klineRepo,
		marketRepo,
		tradeRepo,
		publisher,
		snapshotProvider,
		logger,
		MarketServiceConfig{
			KlineConfig:  config.KlineConfig,
			TickerConfig: config.TickerConfig,
			DepthConfig:  config.DepthConfig,
		},
	)

	svc := &EnhancedMarketService{
		MarketService:   baseSvc,
		config:          config,
		cacheManager:    cacheManager,
		tradeBuffer:     make([]*model.Trade, 0, 1000),
		tradeBufferSize: 1000,
		tradeFlushCh:    make(chan struct{}, 1),
		logger:          logger.Named("enhanced_market_service"),
		ctx:             ctx,
		cancel:          cancel,
	}

	return svc
}

// Start starts the enhanced market service
func (s *EnhancedMarketService) Start(ctx context.Context) error {
	// Start cache manager
	if err := s.cacheManager.Start(s.config.CacheConfig); err != nil {
		return err
	}

	// Initialize markets
	if err := s.InitMarkets(ctx); err != nil {
		s.logger.Warn("failed to init markets", zap.Error(err))
	}

	// Start trade buffer flusher
	s.wg.Add(1)
	go s.tradeFlushLoop()

	// Warm cache for all markets
	if s.config.CacheConfig.EnableCacheWarming {
		s.wg.Add(1)
		go s.warmAllCaches()
	}

	s.logger.Info("enhanced market service started")
	return nil
}

// Stop stops the enhanced market service
func (s *EnhancedMarketService) Stop() {
	s.closeOnce.Do(func() {
		s.cancel()

		// Flush remaining trades
		s.flushTradeBuffer()

		s.wg.Wait()

		// Stop base service
		s.MarketService.Stop()

		// Stop cache manager
		s.cacheManager.Stop()

		s.logger.Info("enhanced market service stopped")
	})
}

// GetTickerWithCache gets ticker with cache-first strategy
func (s *EnhancedMarketService) GetTickerWithCache(ctx context.Context, market string) (*model.Ticker, error) {
	if s.config.EnableCacheFirst {
		// Try cache first
		ticker, err := s.cacheManager.TickerCache.GetTicker24h(ctx, market)
		if err != nil {
			s.logger.Warn("cache error, falling back to memory", zap.Error(err))
		} else if ticker != nil {
			return ticker, nil
		}
	}

	// Fall back to in-memory
	ticker, err := s.MarketService.GetTicker(ctx, market)
	if err != nil {
		return nil, err
	}

	// Update cache asynchronously
	if ticker != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.cacheManager.TickerCache.SaveTicker24h(ctx, market, ticker); err != nil {
				s.logger.Warn("failed to cache ticker", zap.Error(err))
			}
		}()
	}

	return ticker, nil
}

// GetDepthWithCache gets depth with cache-first strategy
func (s *EnhancedMarketService) GetDepthWithCache(ctx context.Context, market string, limit int) (*model.Depth, error) {
	if s.config.EnableCacheFirst {
		// Try cache first
		depth, err := s.cacheManager.DepthCache.GetDepthAtLevel(ctx, market, limit)
		if err != nil {
			s.logger.Warn("cache error, falling back to memory", zap.Error(err))
		} else if depth != nil {
			return depth, nil
		}
	}

	// Fall back to in-memory
	depth, err := s.MarketService.GetDepth(ctx, market, limit)
	if err != nil {
		return nil, err
	}

	// Update cache asynchronously
	if depth != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.cacheManager.DepthCache.SaveDepthWithLevels(ctx, market, depth); err != nil {
				s.logger.Warn("failed to cache depth", zap.Error(err))
			}
		}()
	}

	return depth, nil
}

// GetKlinesWithCache gets klines with cache-first strategy
func (s *EnhancedMarketService) GetKlinesWithCache(ctx context.Context, market string, interval model.KlineInterval, startTime, endTime int64, limit int) ([]*model.Kline, error) {
	if s.config.EnableCacheFirst {
		// Try cache first
		klines, err := s.cacheManager.KlineCache.GetKlines(ctx, market, interval, startTime, endTime, limit)
		if err != nil {
			s.logger.Warn("cache error, falling back to db", zap.Error(err))
		} else if len(klines) > 0 {
			// Check if we have enough data in cache
			if limit <= 0 || len(klines) >= limit {
				return klines, nil
			}
		}
	}

	// Fall back to database
	klines, err := s.MarketService.GetKlines(ctx, market, interval, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}

	// Update cache asynchronously
	if len(klines) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.cacheManager.KlineCache.SaveCompletedKlines(ctx, market, interval, klines); err != nil {
				s.logger.Warn("failed to cache klines", zap.Error(err))
			}
		}()
	}

	return klines, nil
}

// ProcessTradeWithBuffer processes trade with write-behind buffer
func (s *EnhancedMarketService) ProcessTradeWithBuffer(ctx context.Context, trade *model.TradeEvent) error {
	// Process in real-time for aggregators
	if err := s.MarketService.ProcessTrade(ctx, trade); err != nil {
		return err
	}

	// Convert and add to buffer
	t, err := trade.ToTrade()
	if err != nil {
		return nil // Already logged in base service
	}

	s.tradeBufferMu.Lock()
	s.tradeBuffer = append(s.tradeBuffer, t)
	shouldFlush := len(s.tradeBuffer) >= s.tradeBufferSize
	s.tradeBufferMu.Unlock()

	if shouldFlush {
		select {
		case s.tradeFlushCh <- struct{}{}:
		default:
		}
	}

	// Update last trade price cache
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.cacheManager.TickerCache.SaveLastTradePrice(ctx, trade.Market, trade.Price, trade.Timestamp); err != nil {
			s.logger.Warn("failed to cache last price", zap.Error(err))
		}
	}()

	return nil
}

// ProcessOrderBookUpdateWithCache processes orderbook update with cache
func (s *EnhancedMarketService) ProcessOrderBookUpdateWithCache(ctx context.Context, update *model.DepthUpdate) error {
	if err := s.MarketService.ProcessOrderBookUpdate(ctx, update); err != nil {
		return err
	}

	// Get updated depth and cache it
	depth, err := s.MarketService.GetDepth(ctx, update.Market, 100)
	if err == nil && depth != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.cacheManager.DepthCache.SaveDepthWithLevels(ctx, update.Market, depth); err != nil {
				s.logger.Warn("failed to cache depth", zap.Error(err))
			}
		}()
	}

	return nil
}

// tradeFlushLoop periodically flushes the trade buffer
func (s *EnhancedMarketService) tradeFlushLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushTradeBuffer()
		case <-s.tradeFlushCh:
			s.flushTradeBuffer()
		case <-s.ctx.Done():
			return
		}
	}
}

// flushTradeBuffer flushes the trade buffer to database
func (s *EnhancedMarketService) flushTradeBuffer() {
	s.tradeBufferMu.Lock()
	if len(s.tradeBuffer) == 0 {
		s.tradeBufferMu.Unlock()
		return
	}
	trades := s.tradeBuffer
	s.tradeBuffer = make([]*model.Trade, 0, s.tradeBufferSize)
	s.tradeBufferMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.tradeRepo.BatchCreate(ctx, trades); err != nil {
		s.logger.Error("failed to batch create trades", zap.Error(err), zap.Int("count", len(trades)))
		metrics.TradeProcessingErrors.WithLabelValues("batch", "db_error").Inc()
	} else {
		s.logger.Debug("flushed trade buffer", zap.Int("count", len(trades)))
	}
}

// warmAllCaches warms caches for all active markets
func (s *EnhancedMarketService) warmAllCaches() {
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()

	markets, err := s.marketRepo.ListActiveMarkets(ctx)
	if err != nil {
		s.logger.Error("failed to list markets for warming", zap.Error(err))
		return
	}

	for _, market := range markets {
		select {
		case <-s.ctx.Done():
			return
		default:
			if err := s.cacheManager.WarmMarketCache(ctx, market.Symbol, s.config.CacheConfig); err != nil {
				s.logger.Warn("failed to warm cache",
					zap.String("market", market.Symbol),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("cache warming completed", zap.Int("markets", len(markets)))
}

// OnMarketAdded implements MarketChangeListener
func (s *EnhancedMarketService) OnMarketAdded(ctx context.Context, market *model.Market) error {
	s.initMarket(market.Symbol)

	// Warm cache for new market
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.cacheManager.WarmMarketCache(ctx, market.Symbol, s.config.CacheConfig); err != nil {
			s.logger.Warn("failed to warm cache for new market",
				zap.String("market", market.Symbol),
				zap.Error(err))
		}
	}()

	return nil
}

// OnMarketUpdated implements MarketChangeListener
func (s *EnhancedMarketService) OnMarketUpdated(ctx context.Context, market *model.Market) error {
	// Market config updated - may need to invalidate some caches
	s.logger.Info("market updated", zap.String("symbol", market.Symbol))
	return nil
}

// OnMarketRemoved implements MarketChangeListener
func (s *EnhancedMarketService) OnMarketRemoved(ctx context.Context, symbol string) error {
	// Invalidate all caches for removed market
	if err := s.cacheManager.InvalidateMarketCache(ctx, symbol); err != nil {
		s.logger.Warn("failed to invalidate cache for removed market",
			zap.String("market", symbol),
			zap.Error(err))
	}
	return nil
}

// ProcessTrade overrides base service to add caching
func (s *EnhancedMarketService) ProcessTrade(ctx context.Context, trade *model.TradeEvent) error {
	return s.ProcessTradeWithBuffer(ctx, trade)
}

// ProcessOrderBookUpdate overrides base service to add caching
func (s *EnhancedMarketService) ProcessOrderBookUpdate(ctx context.Context, update *model.DepthUpdate) error {
	return s.ProcessOrderBookUpdateWithCache(ctx, update)
}
