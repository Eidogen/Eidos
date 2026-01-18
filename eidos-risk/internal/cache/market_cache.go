package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	marketPriceKeyPrefix = "risk:market:price:"
	marketTickerKeyPrefix = "risk:market:ticker:"
	marketTTL            = 5 * time.Minute
)

// MarketCache 市场缓存
type MarketCache struct {
	client redis.UniversalClient
}

// NewMarketCache 创建市场缓存
func NewMarketCache(client redis.UniversalClient) *MarketCache {
	return &MarketCache{client: client}
}

// SetLastPrice 设置最新价格
func (c *MarketCache) SetLastPrice(ctx context.Context, market string, price decimal.Decimal) error {
	key := marketPriceKeyPrefix + market
	return c.client.Set(ctx, key, price.String(), marketTTL).Err()
}

// GetLastPrice 获取最新价格
func (c *MarketCache) GetLastPrice(ctx context.Context, market string) (decimal.Decimal, error) {
	key := marketPriceKeyPrefix + market

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return decimal.NewFromString(val)
}

// SetTicker 设置行情数据
func (c *MarketCache) SetTicker(ctx context.Context, market string, ticker map[string]string) error {
	key := marketTickerKeyPrefix + market

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, ticker)
	pipe.Expire(ctx, key, marketTTL)

	_, err := pipe.Exec(ctx)
	return err
}

// GetTicker 获取行情数据
func (c *MarketCache) GetTicker(ctx context.Context, market string) (map[string]string, error) {
	key := marketTickerKeyPrefix + market
	return c.client.HGetAll(ctx, key).Result()
}

// GetPriceChange24h 获取24小时价格变化百分比
func (c *MarketCache) GetPriceChange24h(ctx context.Context, market string) (decimal.Decimal, error) {
	ticker, err := c.GetTicker(ctx, market)
	if err != nil {
		return decimal.Zero, err
	}

	changeStr, ok := ticker["price_change_percent"]
	if !ok {
		return decimal.Zero, nil
	}

	return decimal.NewFromString(changeStr)
}

// GetVolume24h 获取24小时成交量
func (c *MarketCache) GetVolume24h(ctx context.Context, market string) (decimal.Decimal, error) {
	ticker, err := c.GetTicker(ctx, market)
	if err != nil {
		return decimal.Zero, err
	}

	volumeStr, ok := ticker["volume_24h"]
	if !ok {
		return decimal.Zero, nil
	}

	return decimal.NewFromString(volumeStr)
}

// BatchSetLastPrice 批量设置最新价格
func (c *MarketCache) BatchSetLastPrice(ctx context.Context, prices map[string]decimal.Decimal) error {
	if len(prices) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()

	for market, price := range prices {
		key := marketPriceKeyPrefix + market
		pipe.Set(ctx, key, price.String(), marketTTL)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// BatchGetLastPrice 批量获取最新价格
func (c *MarketCache) BatchGetLastPrice(ctx context.Context, markets []string) (map[string]decimal.Decimal, error) {
	if len(markets) == 0 {
		return make(map[string]decimal.Decimal), nil
	}

	pipe := c.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)

	for _, market := range markets {
		key := marketPriceKeyPrefix + market
		cmds[market] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[string]decimal.Decimal)
	for market, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			continue
		}
		price, err := decimal.NewFromString(val)
		if err != nil {
			continue
		}
		result[market] = price
	}

	return result, nil
}

// IsMarketActive 检查市场是否活跃
func (c *MarketCache) IsMarketActive(ctx context.Context, market string) (bool, error) {
	key := marketPriceKeyPrefix + market
	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// GetMarketStats 获取市场统计
func (c *MarketCache) GetMarketStats(ctx context.Context, market string) (map[string]interface{}, error) {
	price, _ := c.GetLastPrice(ctx, market)
	ticker, _ := c.GetTicker(ctx, market)

	stats := map[string]interface{}{
		"market":     market,
		"last_price": price.String(),
	}

	for k, v := range ticker {
		stats[k] = v
	}

	return stats, nil
}

// marketPriceKey 生成市场价格键
func marketPriceKey(market string) string {
	return fmt.Sprintf("%s%s", marketPriceKeyPrefix, market)
}
