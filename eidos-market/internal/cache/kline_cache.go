// Package cache provides Redis caching implementations.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// Kline cache keys
const (
	// KeyCurrentKline stores the current (incomplete) kline
	// Key: eidos:kline_current:{market}:{interval}
	KeyCurrentKline = "eidos:kline_current:%s:%s"

	// KeyKlineHistory stores historical klines as a sorted set (by open_time)
	// Key: eidos:kline_history:{market}:{interval}
	KeyKlineHistory = "eidos:kline_history:%s:%s"

	// KeyCurrentKlineExpiry TTL for current kline (2x interval duration)
	// Will be set dynamically based on interval

	// MaxHistoricalKlines maximum klines to keep in cache per interval
	MaxHistoricalKlines = 1000
)

// KlineCache provides caching for K-line data
type KlineCache struct {
	client redis.UniversalClient
	logger *slog.Logger
}

// NewKlineCache creates a new kline cache
func NewKlineCache(client redis.UniversalClient, logger *slog.Logger) *KlineCache {
	return &KlineCache{
		client: client,
		logger: logger.With("component", "kline_cache"),
	}
}

// SaveCurrentKline saves the current (incomplete) kline
func (c *KlineCache) SaveCurrentKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	key := fmt.Sprintf(KeyCurrentKline, market, interval)

	data, err := json.Marshal(kline)
	if err != nil {
		return fmt.Errorf("marshal kline: %w", err)
	}

	// Set TTL to 2x the interval duration
	duration := model.IntervalDurations[interval]
	ttl := time.Duration(duration*2) * time.Millisecond

	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetCurrentKline gets the current (incomplete) kline
func (c *KlineCache) GetCurrentKline(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	key := fmt.Sprintf(KeyCurrentKline, market, interval)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheMisses.WithLabelValues("kline_current").Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("get kline: %w", err)
	}

	var kline model.Kline
	if err := json.Unmarshal(data, &kline); err != nil {
		return nil, fmt.Errorf("unmarshal kline: %w", err)
	}

	metrics.CacheHits.WithLabelValues("kline_current").Inc()
	return &kline, nil
}

// SaveCompletedKline saves a completed kline to history
func (c *KlineCache) SaveCompletedKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	key := fmt.Sprintf(KeyKlineHistory, market, interval)

	data, err := json.Marshal(kline)
	if err != nil {
		return fmt.Errorf("marshal kline: %w", err)
	}

	// Use open_time as score for sorted set
	member := redis.Z{
		Score:  float64(kline.OpenTime),
		Member: data,
	}

	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, member)
	// Remove old entries beyond MaxHistoricalKlines
	pipe.ZRemRangeByRank(ctx, key, 0, -MaxHistoricalKlines-1)
	// Set expiry based on interval
	ttl := c.getHistoryTTL(interval)
	pipe.Expire(ctx, key, ttl)

	_, err = pipe.Exec(ctx)
	return err
}

// SaveCompletedKlines saves multiple completed klines to history
func (c *KlineCache) SaveCompletedKlines(ctx context.Context, market string, interval model.KlineInterval, klines []*model.Kline) error {
	if len(klines) == 0 {
		return nil
	}

	key := fmt.Sprintf(KeyKlineHistory, market, interval)

	members := make([]redis.Z, 0, len(klines))
	for _, kline := range klines {
		data, err := json.Marshal(kline)
		if err != nil {
			continue
		}
		members = append(members, redis.Z{
			Score:  float64(kline.OpenTime),
			Member: data,
		})
	}

	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, members...)
	pipe.ZRemRangeByRank(ctx, key, 0, -MaxHistoricalKlines-1)
	ttl := c.getHistoryTTL(interval)
	pipe.Expire(ctx, key, ttl)

	_, err := pipe.Exec(ctx)
	return err
}

// GetKlines gets klines from cache
func (c *KlineCache) GetKlines(ctx context.Context, market string, interval model.KlineInterval, startTime, endTime int64, limit int) ([]*model.Kline, error) {
	key := fmt.Sprintf(KeyKlineHistory, market, interval)

	// Use score range for time-based query
	opt := &redis.ZRangeBy{
		Min: strconv.FormatInt(startTime, 10),
		Max: strconv.FormatInt(endTime, 10),
	}

	if endTime == 0 {
		opt.Max = "+inf"
	}

	result, err := c.client.ZRangeByScore(ctx, key, opt).Result()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheMisses.WithLabelValues("kline_history").Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("get klines: %w", err)
	}

	if len(result) == 0 {
		metrics.CacheMisses.WithLabelValues("kline_history").Inc()
		return nil, nil
	}

	metrics.CacheHits.WithLabelValues("kline_history").Inc()

	klines := make([]*model.Kline, 0, len(result))
	for _, data := range result {
		var kline model.Kline
		if err := json.Unmarshal([]byte(data), &kline); err != nil {
			continue
		}
		klines = append(klines, &kline)
	}

	// Apply limit
	if limit > 0 && len(klines) > limit {
		// Return the most recent ones
		klines = klines[len(klines)-limit:]
	}

	return klines, nil
}

// GetLatestKlines gets the most recent klines from cache
func (c *KlineCache) GetLatestKlines(ctx context.Context, market string, interval model.KlineInterval, limit int) ([]*model.Kline, error) {
	key := fmt.Sprintf(KeyKlineHistory, market, interval)

	if limit <= 0 {
		limit = 100
	}

	// Get the most recent entries
	result, err := c.client.ZRevRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest klines: %w", err)
	}

	if len(result) == 0 {
		return nil, nil
	}

	klines := make([]*model.Kline, 0, len(result))
	for i := len(result) - 1; i >= 0; i-- { // Reverse to get ascending order
		var kline model.Kline
		if err := json.Unmarshal([]byte(result[i]), &kline); err != nil {
			continue
		}
		klines = append(klines, &kline)
	}

	return klines, nil
}

// GetKlineCount returns the number of cached klines
func (c *KlineCache) GetKlineCount(ctx context.Context, market string, interval model.KlineInterval) (int64, error) {
	key := fmt.Sprintf(KeyKlineHistory, market, interval)
	return c.client.ZCard(ctx, key).Result()
}

// InvalidateKlines invalidates kline cache for a market
func (c *KlineCache) InvalidateKlines(ctx context.Context, market string, interval model.KlineInterval) error {
	keys := []string{
		fmt.Sprintf(KeyCurrentKline, market, interval),
		fmt.Sprintf(KeyKlineHistory, market, interval),
	}

	return c.client.Del(ctx, keys...).Err()
}

// InvalidateAllKlines invalidates all kline cache for a market
func (c *KlineCache) InvalidateAllKlines(ctx context.Context, market string) error {
	keys := make([]string, 0, len(model.AllIntervals)*2)

	for _, interval := range model.AllIntervals {
		keys = append(keys, fmt.Sprintf(KeyCurrentKline, market, interval))
		keys = append(keys, fmt.Sprintf(KeyKlineHistory, market, interval))
	}

	return c.client.Del(ctx, keys...).Err()
}

// getHistoryTTL returns the TTL for kline history based on interval
func (c *KlineCache) getHistoryTTL(interval model.KlineInterval) time.Duration {
	switch interval {
	case model.Interval1m:
		return 24 * time.Hour // 1 day for 1m klines
	case model.Interval5m:
		return 7 * 24 * time.Hour // 7 days for 5m klines
	case model.Interval15m, model.Interval30m:
		return 14 * 24 * time.Hour // 14 days for 15m, 30m klines
	case model.Interval1h, model.Interval4h:
		return 30 * 24 * time.Hour // 30 days for 1h, 4h klines
	case model.Interval1d, model.Interval1w:
		return 365 * 24 * time.Hour // 1 year for 1d, 1w klines
	default:
		return 24 * time.Hour
	}
}

// WarmCache warms the kline cache by loading recent data
func (c *KlineCache) WarmCache(ctx context.Context, market string, interval model.KlineInterval, klines []*model.Kline) error {
	if len(klines) == 0 {
		return nil
	}

	c.logger.Info("warming kline cache",
		"market", market,
		"interval", string(interval),
		"count", len(klines))

	return c.SaveCompletedKlines(ctx, market, interval, klines)
}
