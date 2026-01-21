// Package cache provides Redis caching implementations.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// Ticker bucket cache keys
const (
	// KeyTickerBucketHash stores all ticker buckets for a market as a hash
	// Key: eidos:ticker_buckets:{market}
	// Field: minute timestamp (string)
	// Value: JSON serialized TickerBucket
	KeyTickerBucketHash = "eidos:ticker_buckets:%s"

	// KeyTickerBucketExpiry TTL for ticker bucket hash (25 hours to allow some buffer)
	KeyTickerBucketExpiry = 25 * time.Hour

	// KeyTicker24h stores the calculated 24h ticker
	KeyTicker24h = "eidos:ticker24h:%s"

	// KeyTicker24hExpiry TTL for 24h ticker (10 seconds)
	KeyTicker24hExpiry = 10 * time.Second

	// KeyLastTradePrice stores the last trade price for a market
	KeyLastTradePrice = "eidos:last_price:%s"

	// MaxTickerBuckets maximum number of buckets to keep (24h * 60min + buffer)
	MaxTickerBuckets = 1500
)

// TickerCache provides caching for ticker data with bucket persistence
type TickerCache struct {
	client redis.UniversalClient
	logger *zap.Logger
}

// NewTickerCache creates a new ticker cache
func NewTickerCache(client redis.UniversalClient, logger *zap.Logger) *TickerCache {
	return &TickerCache{
		client: client,
		logger: logger.Named("ticker_cache"),
	}
}

// SaveTickerBucket saves a ticker bucket to cache
func (c *TickerCache) SaveTickerBucket(ctx context.Context, market string, bucket *model.TickerBucket) error {
	key := fmt.Sprintf(KeyTickerBucketHash, market)
	field := strconv.FormatInt(bucket.Minute, 10)

	data, err := json.Marshal(bucket)
	if err != nil {
		return fmt.Errorf("marshal bucket: %w", err)
	}

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, field, data)
	pipe.Expire(ctx, key, KeyTickerBucketExpiry)
	_, err = pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("save bucket: %w", err)
	}

	return nil
}

// SaveTickerBuckets saves multiple ticker buckets to cache
func (c *TickerCache) SaveTickerBuckets(ctx context.Context, market string, buckets []*model.TickerBucket) error {
	if len(buckets) == 0 {
		return nil
	}

	key := fmt.Sprintf(KeyTickerBucketHash, market)

	// Prepare hash fields
	fields := make(map[string]interface{}, len(buckets))
	for _, bucket := range buckets {
		if bucket == nil || bucket.TradeCount == 0 {
			continue
		}
		data, err := json.Marshal(bucket)
		if err != nil {
			c.logger.Warn("failed to marshal bucket",
				zap.Int64("minute", bucket.Minute),
				zap.Error(err))
			continue
		}
		fields[strconv.FormatInt(bucket.Minute, 10)] = data
	}

	if len(fields) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, KeyTickerBucketExpiry)
	_, err := pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("save buckets: %w", err)
	}

	return nil
}

// LoadTickerBuckets loads all ticker buckets from cache for a market
func (c *TickerCache) LoadTickerBuckets(ctx context.Context, market string) ([]*model.TickerBucket, error) {
	key := fmt.Sprintf(KeyTickerBucketHash, market)

	result, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("load buckets: %w", err)
	}

	now := time.Now().UnixMilli()
	windowStart := now - 24*60*60*1000 // 24 hours ago

	buckets := make([]*model.TickerBucket, 0, len(result))
	expiredFields := make([]string, 0)

	for field, data := range result {
		var bucket model.TickerBucket
		if err := json.Unmarshal([]byte(data), &bucket); err != nil {
			c.logger.Warn("failed to unmarshal bucket",
				zap.String("field", field),
				zap.Error(err))
			continue
		}

		// Skip expired buckets
		if bucket.Minute < windowStart {
			expiredFields = append(expiredFields, field)
			continue
		}

		buckets = append(buckets, &bucket)
	}

	// Clean up expired buckets asynchronously
	if len(expiredFields) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := c.client.HDel(ctx, key, expiredFields...).Err(); err != nil {
				c.logger.Warn("failed to clean expired buckets", zap.Error(err))
			}
		}()
	}

	return buckets, nil
}

// LoadTickerBucketsInRange loads ticker buckets within a time range
func (c *TickerCache) LoadTickerBucketsInRange(ctx context.Context, market string, startTime, endTime int64) ([]*model.TickerBucket, error) {
	key := fmt.Sprintf(KeyTickerBucketHash, market)

	result, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("load buckets: %w", err)
	}

	buckets := make([]*model.TickerBucket, 0)

	for _, data := range result {
		var bucket model.TickerBucket
		if err := json.Unmarshal([]byte(data), &bucket); err != nil {
			continue
		}

		// Filter by time range
		if bucket.Minute >= startTime && bucket.Minute <= endTime {
			buckets = append(buckets, &bucket)
		}
	}

	return buckets, nil
}

// SaveTicker24h saves the calculated 24h ticker
func (c *TickerCache) SaveTicker24h(ctx context.Context, market string, ticker *model.Ticker) error {
	key := fmt.Sprintf(KeyTicker24h, market)

	data, err := json.Marshal(ticker)
	if err != nil {
		return fmt.Errorf("marshal ticker: %w", err)
	}

	if err := c.client.Set(ctx, key, data, KeyTicker24hExpiry).Err(); err != nil {
		return fmt.Errorf("save ticker: %w", err)
	}

	return nil
}

// GetTicker24h gets the cached 24h ticker
func (c *TickerCache) GetTicker24h(ctx context.Context, market string) (*model.Ticker, error) {
	key := fmt.Sprintf(KeyTicker24h, market)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheMisses.WithLabelValues("ticker24h").Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("get ticker: %w", err)
	}

	var ticker model.Ticker
	if err := json.Unmarshal(data, &ticker); err != nil {
		return nil, fmt.Errorf("unmarshal ticker: %w", err)
	}

	metrics.CacheHits.WithLabelValues("ticker24h").Inc()
	return &ticker, nil
}

// SaveLastTradePrice saves the last trade price for a market
func (c *TickerCache) SaveLastTradePrice(ctx context.Context, market string, price string, timestamp int64) error {
	key := fmt.Sprintf(KeyLastTradePrice, market)

	data := map[string]interface{}{
		"price":     price,
		"timestamp": timestamp,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal price: %w", err)
	}

	// Keep for 7 days
	return c.client.Set(ctx, key, jsonData, 7*24*time.Hour).Err()
}

// GetLastTradePrice gets the last trade price for a market
func (c *TickerCache) GetLastTradePrice(ctx context.Context, market string) (price string, timestamp int64, err error) {
	key := fmt.Sprintf(KeyLastTradePrice, market)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return "", 0, nil
		}
		return "", 0, fmt.Errorf("get price: %w", err)
	}

	var result struct {
		Price     string `json:"price"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", 0, fmt.Errorf("unmarshal price: %w", err)
	}

	return result.Price, result.Timestamp, nil
}

// CleanExpiredBuckets removes expired buckets from cache
func (c *TickerCache) CleanExpiredBuckets(ctx context.Context, market string) (int64, error) {
	key := fmt.Sprintf(KeyTickerBucketHash, market)

	result, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("get buckets: %w", err)
	}

	now := time.Now().UnixMilli()
	windowStart := now - 24*60*60*1000 // 24 hours ago

	expiredFields := make([]string, 0)

	for field := range result {
		minute, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			continue
		}
		if minute < windowStart {
			expiredFields = append(expiredFields, field)
		}
	}

	if len(expiredFields) == 0 {
		return 0, nil
	}

	deleted, err := c.client.HDel(ctx, key, expiredFields...).Result()
	if err != nil {
		return 0, fmt.Errorf("delete expired: %w", err)
	}

	return deleted, nil
}

// GetBucketCount returns the number of buckets cached for a market
func (c *TickerCache) GetBucketCount(ctx context.Context, market string) (int64, error) {
	key := fmt.Sprintf(KeyTickerBucketHash, market)
	return c.client.HLen(ctx, key).Result()
}
