// Package cache provides Redis caching implementations.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// Depth cache keys
const (
	// KeyDepthSnapshot stores the full depth snapshot
	KeyDepthSnapshot = "eidos:depth_snapshot:%s"

	// KeyDepthSnapshotExpiry TTL for depth snapshot (30 seconds)
	KeyDepthSnapshotExpiry = 30 * time.Second

	// KeyDepthSequence stores the current depth sequence number
	KeyDepthSequence = "eidos:depth_seq:%s"

	// KeyDepthBestPrices stores best bid/ask prices for quick access
	KeyDepthBestPrices = "eidos:depth_best:%s"

	// KeyDepthBestPricesExpiry TTL for best prices (5 seconds)
	KeyDepthBestPricesExpiry = 5 * time.Second
)

// DepthCache provides caching for order book depth data
type DepthCache struct {
	client redis.UniversalClient
	logger *slog.Logger
}

// NewDepthCache creates a new depth cache
func NewDepthCache(client redis.UniversalClient, logger *slog.Logger) *DepthCache {
	return &DepthCache{
		client: client,
		logger: logger.With("component", "depth_cache"),
	}
}

// SaveDepthSnapshot saves a full depth snapshot
func (c *DepthCache) SaveDepthSnapshot(ctx context.Context, market string, depth *model.Depth) error {
	key := fmt.Sprintf(KeyDepthSnapshot, market)

	data, err := json.Marshal(depth)
	if err != nil {
		return fmt.Errorf("marshal depth: %w", err)
	}

	pipe := c.client.Pipeline()
	pipe.Set(ctx, key, data, KeyDepthSnapshotExpiry)

	// Also update sequence number
	seqKey := fmt.Sprintf(KeyDepthSequence, market)
	pipe.Set(ctx, seqKey, depth.Sequence, 0) // No expiry for sequence

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save depth: %w", err)
	}

	return nil
}

// GetDepthSnapshot gets a cached depth snapshot
func (c *DepthCache) GetDepthSnapshot(ctx context.Context, market string, limit int) (*model.Depth, error) {
	key := fmt.Sprintf(KeyDepthSnapshot, market)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheMisses.WithLabelValues("depth").Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("get depth: %w", err)
	}

	var depth model.Depth
	if err := json.Unmarshal(data, &depth); err != nil {
		return nil, fmt.Errorf("unmarshal depth: %w", err)
	}

	metrics.CacheHits.WithLabelValues("depth").Inc()

	// Apply limit if specified
	if limit > 0 {
		return depth.Limit(limit), nil
	}

	return &depth, nil
}

// GetDepthSequence gets the current depth sequence number
func (c *DepthCache) GetDepthSequence(ctx context.Context, market string) (uint64, error) {
	key := fmt.Sprintf(KeyDepthSequence, market)

	seq, err := c.client.Get(ctx, key).Uint64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("get sequence: %w", err)
	}

	return seq, nil
}

// BestPrices represents the best bid/ask prices
type BestPrices struct {
	BestBid    decimal.Decimal `json:"best_bid"`
	BestBidQty decimal.Decimal `json:"best_bid_qty"`
	BestAsk    decimal.Decimal `json:"best_ask"`
	BestAskQty decimal.Decimal `json:"best_ask_qty"`
	Timestamp  int64           `json:"timestamp"`
}

// SaveBestPrices saves best bid/ask prices for quick access
func (c *DepthCache) SaveBestPrices(ctx context.Context, market string, prices *BestPrices) error {
	key := fmt.Sprintf(KeyDepthBestPrices, market)

	data, err := json.Marshal(prices)
	if err != nil {
		return fmt.Errorf("marshal prices: %w", err)
	}

	return c.client.Set(ctx, key, data, KeyDepthBestPricesExpiry).Err()
}

// GetBestPrices gets cached best bid/ask prices
func (c *DepthCache) GetBestPrices(ctx context.Context, market string) (*BestPrices, error) {
	key := fmt.Sprintf(KeyDepthBestPrices, market)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("get prices: %w", err)
	}

	var prices BestPrices
	if err := json.Unmarshal(data, &prices); err != nil {
		return nil, fmt.Errorf("unmarshal prices: %w", err)
	}

	return &prices, nil
}

// SaveDepthWithLevels saves depth at multiple levels (5, 10, 20, 50, 100)
func (c *DepthCache) SaveDepthWithLevels(ctx context.Context, market string, depth *model.Depth) error {
	levels := []int{5, 10, 20, 50, 100}

	pipe := c.client.Pipeline()

	for _, level := range levels {
		limited := depth.Limit(level)
		key := fmt.Sprintf("eidos:depth:%s:%d", market, level)

		data, err := json.Marshal(limited)
		if err != nil {
			continue
		}

		pipe.Set(ctx, key, data, KeyDepthSnapshotExpiry)
	}

	// Save full snapshot
	fullKey := fmt.Sprintf(KeyDepthSnapshot, market)
	fullData, err := json.Marshal(depth)
	if err == nil {
		pipe.Set(ctx, fullKey, fullData, KeyDepthSnapshotExpiry)
	}

	// Update sequence
	seqKey := fmt.Sprintf(KeyDepthSequence, market)
	pipe.Set(ctx, seqKey, depth.Sequence, 0)

	// Update best prices
	if len(depth.Bids) > 0 || len(depth.Asks) > 0 {
		bestPrices := &BestPrices{
			Timestamp: depth.Timestamp,
		}
		if len(depth.Bids) > 0 {
			bestPrices.BestBid = depth.Bids[0].Price
			bestPrices.BestBidQty = depth.Bids[0].Amount
		}
		if len(depth.Asks) > 0 {
			bestPrices.BestAsk = depth.Asks[0].Price
			bestPrices.BestAskQty = depth.Asks[0].Amount
		}
		bestData, err := json.Marshal(bestPrices)
		if err == nil {
			bestKey := fmt.Sprintf(KeyDepthBestPrices, market)
			pipe.Set(ctx, bestKey, bestData, KeyDepthBestPricesExpiry)
		}
	}

	_, err = pipe.Exec(ctx)
	return err
}

// GetDepthAtLevel gets cached depth at a specific level
func (c *DepthCache) GetDepthAtLevel(ctx context.Context, market string, level int) (*model.Depth, error) {
	// Normalize level
	normalizedLevel := model.NormalizeDepthLimit(level)
	key := fmt.Sprintf("eidos:depth:%s:%d", market, normalizedLevel)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			metrics.CacheMisses.WithLabelValues("depth").Inc()
			return nil, nil
		}
		return nil, fmt.Errorf("get depth: %w", err)
	}

	var depth model.Depth
	if err := json.Unmarshal(data, &depth); err != nil {
		return nil, fmt.Errorf("unmarshal depth: %w", err)
	}

	metrics.CacheHits.WithLabelValues("depth").Inc()
	return &depth, nil
}

// InvalidateDepth invalidates all depth cache for a market
func (c *DepthCache) InvalidateDepth(ctx context.Context, market string) error {
	levels := []int{5, 10, 20, 50, 100}

	keys := make([]string, 0, len(levels)+3)
	for _, level := range levels {
		keys = append(keys, fmt.Sprintf("eidos:depth:%s:%d", market, level))
	}
	keys = append(keys, fmt.Sprintf(KeyDepthSnapshot, market))
	keys = append(keys, fmt.Sprintf(KeyDepthBestPrices, market))
	// Don't delete sequence - it's needed for gap detection

	return c.client.Del(ctx, keys...).Err()
}
