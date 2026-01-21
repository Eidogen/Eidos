// Package cache provides Redis caching implementations.
package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
)

// CacheManager manages all market data caches
type CacheManager struct {
	client redis.UniversalClient
	logger *zap.Logger

	// Specialized caches
	TickerCache *TickerCache
	DepthCache  *DepthCache
	KlineCache  *KlineCache
	RedisCache  *RedisCache // General purpose cache
	PubSub      *PubSub

	// Repositories for cache warming
	klineRepo repository.KlineRepository
	tradeRepo repository.TradeRepository

	// Background tasks
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// CacheManagerConfig configuration for cache manager
type CacheManagerConfig struct {
	// Enable cache warming on startup
	EnableCacheWarming bool
	// Number of historical klines to cache per interval
	KlineWarmupCount int
	// Number of recent trades to cache per market
	TradeWarmupCount int
	// Cache cleanup interval
	CleanupInterval time.Duration
}

// DefaultCacheManagerConfig returns default configuration
func DefaultCacheManagerConfig() CacheManagerConfig {
	return CacheManagerConfig{
		EnableCacheWarming: true,
		KlineWarmupCount:   500,
		TradeWarmupCount:   100,
		CleanupInterval:    5 * time.Minute,
	}
}

// NewCacheManager creates a new cache manager
func NewCacheManager(
	client redis.UniversalClient,
	klineRepo repository.KlineRepository,
	tradeRepo repository.TradeRepository,
	logger *zap.Logger,
) *CacheManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &CacheManager{
		client:      client,
		logger:      logger.Named("cache_manager"),
		TickerCache: NewTickerCache(client, logger),
		DepthCache:  NewDepthCache(client, logger),
		KlineCache:  NewKlineCache(client, logger),
		RedisCache:  NewRedisCache(client, logger),
		PubSub:      NewPubSub(client, logger),
		klineRepo:   klineRepo,
		tradeRepo:   tradeRepo,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the cache manager background tasks
func (cm *CacheManager) Start(config CacheManagerConfig) error {
	// Start periodic cleanup
	cm.wg.Add(1)
	go cm.cleanupLoop(config.CleanupInterval)

	cm.logger.Info("cache manager started",
		zap.Bool("warming_enabled", config.EnableCacheWarming),
		zap.Duration("cleanup_interval", config.CleanupInterval))

	return nil
}

// Stop stops the cache manager
func (cm *CacheManager) Stop() {
	cm.closeOnce.Do(func() {
		cm.cancel()
		cm.wg.Wait()
		cm.logger.Info("cache manager stopped")
	})
}

// WarmMarketCache warms the cache for a specific market
func (cm *CacheManager) WarmMarketCache(ctx context.Context, market string, config CacheManagerConfig) error {
	if !config.EnableCacheWarming {
		return nil
	}

	cm.logger.Info("warming cache for market", zap.String("market", market))

	var wg sync.WaitGroup
	errCh := make(chan error, len(model.AllIntervals)+1)

	// Warm kline caches for each interval
	for _, interval := range model.AllIntervals {
		wg.Add(1)
		go func(interval model.KlineInterval) {
			defer wg.Done()
			if err := cm.warmKlineCache(ctx, market, interval, config.KlineWarmupCount); err != nil {
				errCh <- fmt.Errorf("warm klines %s: %w", interval, err)
			}
		}(interval)
	}

	// Warm trade cache
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := cm.warmTradeCache(ctx, market, config.TradeWarmupCount); err != nil {
			errCh <- fmt.Errorf("warm trades: %w", err)
		}
	}()

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		cm.logger.Warn("cache warming completed with errors",
			zap.String("market", market),
			zap.Int("errors", len(errs)))
		return errs[0]
	}

	cm.logger.Info("cache warming completed", zap.String("market", market))
	return nil
}

// warmKlineCache warms the kline cache from database
func (cm *CacheManager) warmKlineCache(ctx context.Context, market string, interval model.KlineInterval, count int) error {
	if cm.klineRepo == nil {
		return nil
	}

	klines, err := cm.klineRepo.Query(ctx, market, interval, 0, 0, count)
	if err != nil {
		return err
	}

	if len(klines) == 0 {
		return nil
	}

	return cm.KlineCache.WarmCache(ctx, market, interval, klines)
}

// warmTradeCache warms the trade cache from database
func (cm *CacheManager) warmTradeCache(ctx context.Context, market string, count int) error {
	if cm.tradeRepo == nil {
		return nil
	}

	trades, err := cm.tradeRepo.ListRecent(ctx, market, count)
	if err != nil {
		return err
	}

	// Push trades to Redis cache
	for _, trade := range trades {
		if err := cm.RedisCache.PushTrade(ctx, market, trade); err != nil {
			cm.logger.Warn("failed to cache trade", zap.Error(err))
		}
	}

	return nil
}

// cleanupLoop periodically cleans up expired cache entries
func (cm *CacheManager) cleanupLoop(interval time.Duration) {
	defer cm.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.cleanup()
		case <-cm.ctx.Done():
			return
		}
	}
}

// cleanup cleans up expired cache entries
func (cm *CacheManager) cleanup() {
	ctx, cancel := context.WithTimeout(cm.ctx, 30*time.Second)
	defer cancel()

	// Get all market keys and clean expired ticker buckets
	pattern := "eidos:ticker_buckets:*"
	keys, err := cm.client.Keys(ctx, pattern).Result()
	if err != nil {
		cm.logger.Warn("failed to get ticker bucket keys", zap.Error(err))
		return
	}

	for _, key := range keys {
		// Extract market from key
		market := key[len("eidos:ticker_buckets:"):]
		deleted, err := cm.TickerCache.CleanExpiredBuckets(ctx, market)
		if err != nil {
			cm.logger.Warn("failed to clean expired buckets",
				zap.String("market", market),
				zap.Error(err))
			continue
		}
		if deleted > 0 {
			cm.logger.Debug("cleaned expired ticker buckets",
				zap.String("market", market),
				zap.Int64("count", deleted))
		}
	}
}

// InvalidateMarketCache invalidates all cache for a market
func (cm *CacheManager) InvalidateMarketCache(ctx context.Context, market string) error {
	// Invalidate all klines
	if err := cm.KlineCache.InvalidateAllKlines(ctx, market); err != nil {
		cm.logger.Warn("failed to invalidate kline cache", zap.Error(err))
	}

	// Invalidate depth
	if err := cm.DepthCache.InvalidateDepth(ctx, market); err != nil {
		cm.logger.Warn("failed to invalidate depth cache", zap.Error(err))
	}

	// Delete other market-specific keys
	keys := []string{
		fmt.Sprintf(KeyTicker, market),
		fmt.Sprintf(KeyDepth, market),
		fmt.Sprintf(KeyTrades, market),
		fmt.Sprintf(KeyTickerBucketHash, market),
		fmt.Sprintf(KeyTicker24h, market),
		fmt.Sprintf(KeyLastTradePrice, market),
	}

	return cm.client.Del(ctx, keys...).Err()
}

// HealthCheck performs a health check on the cache
func (cm *CacheManager) HealthCheck(ctx context.Context) error {
	return cm.client.Ping(ctx).Err()
}
