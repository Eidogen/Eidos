package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// Redis 缓存键格式
const (
	KeyTicker       = "eidos:ticker:%s"           // market
	KeyDepth        = "eidos:orderbook:%s"        // market
	KeyTrades       = "eidos:trades:%s"           // market (List)
	KeyKline        = "eidos:kline:%s:%s"         // market:interval
	KeyTickerBucket = "eidos:ticker_bucket:%s:%d" // market:minute
	KeyMarkets      = "eidos:markets"             // 所有交易对
)

// 默认 TTL
const (
	DefaultTickerTTL = 10 * time.Second
	DefaultDepthTTL  = 5 * time.Second
	DefaultKlineTTL  = 60 * time.Second
	MaxTradesLen     = 100
)

// RedisCache Redis 缓存实现
type RedisCache struct {
	client redis.UniversalClient
	logger *zap.Logger
}

// NewRedisCache 创建 Redis 缓存
func NewRedisCache(client redis.UniversalClient, logger *zap.Logger) *RedisCache {
	return &RedisCache{
		client: client,
		logger: logger.Named("redis_cache"),
	}
}

// SetTicker 缓存 Ticker
func (c *RedisCache) SetTicker(ctx context.Context, market string, ticker *model.Ticker, ttl time.Duration) error {
	key := fmt.Sprintf(KeyTicker, market)
	data, err := json.Marshal(ticker)
	if err != nil {
		return err
	}
	if ttl == 0 {
		ttl = DefaultTickerTTL
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetTicker 获取缓存的 Ticker
func (c *RedisCache) GetTicker(ctx context.Context, market string) (*model.Ticker, error) {
	key := fmt.Sprintf(KeyTicker, market)
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var ticker model.Ticker
	if err := json.Unmarshal(data, &ticker); err != nil {
		return nil, err
	}
	return &ticker, nil
}

// SetDepth 缓存深度
func (c *RedisCache) SetDepth(ctx context.Context, market string, depth *model.Depth, ttl time.Duration) error {
	key := fmt.Sprintf(KeyDepth, market)
	data, err := json.Marshal(depth)
	if err != nil {
		return err
	}
	if ttl == 0 {
		ttl = DefaultDepthTTL
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetDepth 获取缓存的深度
func (c *RedisCache) GetDepth(ctx context.Context, market string) (*model.Depth, error) {
	key := fmt.Sprintf(KeyDepth, market)
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var depth model.Depth
	if err := json.Unmarshal(data, &depth); err != nil {
		return nil, err
	}
	return &depth, nil
}

// PushTrade 推送成交到缓存（FIFO 列表）
func (c *RedisCache) PushTrade(ctx context.Context, market string, trade *model.Trade) error {
	key := fmt.Sprintf(KeyTrades, market)
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}
	pipe := c.client.Pipeline()
	pipe.LPush(ctx, key, data)
	pipe.LTrim(ctx, key, 0, MaxTradesLen-1)
	_, err = pipe.Exec(ctx)
	return err
}

// GetRecentTrades 获取最近成交
func (c *RedisCache) GetRecentTrades(ctx context.Context, market string, limit int64) ([]*model.Trade, error) {
	key := fmt.Sprintf(KeyTrades, market)
	data, err := c.client.LRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	trades := make([]*model.Trade, 0, len(data))
	for _, item := range data {
		var trade model.Trade
		if err := json.Unmarshal([]byte(item), &trade); err != nil {
			continue
		}
		trades = append(trades, &trade)
	}
	return trades, nil
}

// SetKline 缓存当前 K 线
func (c *RedisCache) SetKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline, ttl time.Duration) error {
	key := fmt.Sprintf(KeyKline, market, interval)
	data, err := json.Marshal(kline)
	if err != nil {
		return err
	}
	if ttl == 0 {
		ttl = DefaultKlineTTL
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetKline 获取缓存的当前 K 线
func (c *RedisCache) GetKline(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	key := fmt.Sprintf(KeyKline, market, interval)
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var kline model.Kline
	if err := json.Unmarshal(data, &kline); err != nil {
		return nil, err
	}
	return &kline, nil
}

// SetMarkets 缓存所有交易对
func (c *RedisCache) SetMarkets(ctx context.Context, markets []*model.Market, ttl time.Duration) error {
	data, err := json.Marshal(markets)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, KeyMarkets, data, ttl).Err()
}

// GetMarkets 获取缓存的所有交易对
func (c *RedisCache) GetMarkets(ctx context.Context) ([]*model.Market, error) {
	data, err := c.client.Get(ctx, KeyMarkets).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var markets []*model.Market
	if err := json.Unmarshal(data, &markets); err != nil {
		return nil, err
	}
	return markets, nil
}

// Ping 健康检查
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
