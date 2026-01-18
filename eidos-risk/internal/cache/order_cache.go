package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	userOrdersKeyPrefix = "risk:orders:"
	orderCountKeyPrefix = "risk:order_count:"
	orderTTL            = 24 * time.Hour
)

// OrderInfo 订单信息 (用于风控检查)
type OrderInfo struct {
	OrderID string          `json:"order_id"`
	Market  string          `json:"market"`
	Side    string          `json:"side"`
	Price   decimal.Decimal `json:"price"`
	Amount  decimal.Decimal `json:"amount"`
}

// OrderCache 订单缓存
type OrderCache struct {
	client redis.UniversalClient
}

// NewOrderCache 创建订单缓存
func NewOrderCache(client redis.UniversalClient) *OrderCache {
	return &OrderCache{client: client}
}

// AddOrder 添加活跃订单
func (c *OrderCache) AddOrder(ctx context.Context, wallet string, order *OrderInfo) error {
	key := userOrdersKey(wallet, order.Market)

	data, err := json.Marshal(order)
	if err != nil {
		return err
	}

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, order.OrderID, data)
	pipe.Expire(ctx, key, orderTTL)

	_, err = pipe.Exec(ctx)
	return err
}

// RemoveOrder 移除订单
func (c *OrderCache) RemoveOrder(ctx context.Context, wallet, market, orderID string) error {
	key := userOrdersKey(wallet, market)
	return c.client.HDel(ctx, key, orderID).Err()
}

// GetOrder 获取订单
func (c *OrderCache) GetOrder(ctx context.Context, wallet, market, orderID string) (*OrderInfo, error) {
	key := userOrdersKey(wallet, market)

	data, err := c.client.HGet(ctx, key, orderID).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var order OrderInfo
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, err
	}

	return &order, nil
}

// GetUserOrders 获取用户在指定市场的所有活跃订单
func (c *OrderCache) GetUserOrders(ctx context.Context, wallet, market string) ([]*OrderInfo, error) {
	key := userOrdersKey(wallet, market)

	data, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	orders := make([]*OrderInfo, 0, len(data))
	for _, v := range data {
		var order OrderInfo
		if err := json.Unmarshal([]byte(v), &order); err != nil {
			continue
		}
		orders = append(orders, &order)
	}

	return orders, nil
}

// HasOppositeOrder 检查是否存在可能导致自成交的对手订单
func (c *OrderCache) HasOppositeOrder(ctx context.Context, wallet, market, side string, price decimal.Decimal) (bool, error) {
	orders, err := c.GetUserOrders(ctx, wallet, market)
	if err != nil {
		return false, err
	}

	// 检查是否存在对手方向的订单且价格可能匹配
	for _, order := range orders {
		if order.Side == side {
			continue // 同方向跳过
		}

		// 检查价格是否可能匹配
		// 买单检查是否有价格 <= 买价的卖单
		// 卖单检查是否有价格 >= 卖价的买单
		if side == "BUY" && order.Side == "SELL" {
			if order.Price.LessThanOrEqual(price) {
				return true, nil
			}
		} else if side == "SELL" && order.Side == "BUY" {
			if order.Price.GreaterThanOrEqual(price) {
				return true, nil
			}
		}
	}

	return false, nil
}

// GetOpenOrderCount 获取用户活跃订单数量
func (c *OrderCache) GetOpenOrderCount(ctx context.Context, wallet string) (int, error) {
	key := orderCountKeyPrefix + wallet
	count, err := c.client.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// IncrementOrderCount 增加订单计数
func (c *OrderCache) IncrementOrderCount(ctx context.Context, wallet string) (int, error) {
	key := orderCountKeyPrefix + wallet

	pipe := c.client.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, orderTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	return int(incrCmd.Val()), nil
}

// DecrementOrderCount 减少订单计数
func (c *OrderCache) DecrementOrderCount(ctx context.Context, wallet string) error {
	key := orderCountKeyPrefix + wallet

	script := redis.NewScript(`
		local key = KEYS[1]
		local current = tonumber(redis.call('GET', key) or '0')
		if current > 0 then
			redis.call('DECR', key)
		end
		return current - 1
	`)

	_, err := script.Run(ctx, c.client, []string{key}).Result()
	return err
}

// SetOrderCount 设置订单计数 (用于同步)
func (c *OrderCache) SetOrderCount(ctx context.Context, wallet string, count int) error {
	key := orderCountKeyPrefix + wallet
	return c.client.Set(ctx, key, count, orderTTL).Err()
}

// ClearUserOrders 清空用户订单缓存
func (c *OrderCache) ClearUserOrders(ctx context.Context, wallet, market string) error {
	key := userOrdersKey(wallet, market)
	return c.client.Del(ctx, key).Err()
}

// GetAllMarketOrders 获取用户所有市场的订单 (用于清算)
func (c *OrderCache) GetAllMarketOrders(ctx context.Context, wallet string) (map[string][]*OrderInfo, error) {
	// 使用 SCAN 查找所有匹配的键
	pattern := fmt.Sprintf("%s%s:*", userOrdersKeyPrefix, wallet)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()

	result := make(map[string][]*OrderInfo)

	for iter.Next(ctx) {
		key := iter.Val()
		data, err := c.client.HGetAll(ctx, key).Result()
		if err != nil {
			continue
		}

		orders := make([]*OrderInfo, 0, len(data))
		for _, v := range data {
			var order OrderInfo
			if err := json.Unmarshal([]byte(v), &order); err != nil {
				continue
			}
			orders = append(orders, &order)
		}

		if len(orders) > 0 {
			result[orders[0].Market] = orders
		}
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// userOrdersKey 生成用户订单键
func userOrdersKey(wallet, market string) string {
	return fmt.Sprintf("%s%s:%s", userOrdersKeyPrefix, wallet, market)
}
