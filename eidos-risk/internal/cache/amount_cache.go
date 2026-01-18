package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

const (
	dailyWithdrawKeyPrefix  = "risk:withdraw:daily:"
	pendingSettleKeyPrefix  = "risk:pending:"
	systemPendingSettleKey  = "risk:pending:system"
)

// AmountCache 金额缓存
type AmountCache struct {
	client redis.UniversalClient
}

// NewAmountCache 创建金额缓存
func NewAmountCache(client redis.UniversalClient) *AmountCache {
	return &AmountCache{client: client}
}

// GetDailyWithdraw 获取用户每日提现金额
func (c *AmountCache) GetDailyWithdraw(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	key := dailyWithdrawKey(wallet, token)

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return decimal.NewFromString(val)
}

// AddDailyWithdraw 增加用户每日提现金额
func (c *AmountCache) AddDailyWithdraw(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	key := dailyWithdrawKey(wallet, token)

	// 使用 Lua 脚本保证原子性
	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = ARGV[1]
		local ttl = tonumber(ARGV[2])

		local current = redis.call('GET', key)
		if current then
			current = current + amount
		else
			current = amount
		end

		redis.call('SET', key, current)

		-- 只有新键才设置过期时间
		if redis.call('TTL', key) == -1 then
			redis.call('EXPIRE', key, ttl)
		end

		return current
	`)

	// 计算到今天结束的秒数
	ttl := c.getTimeUntilMidnight()

	_, err := script.Run(ctx, c.client, []string{key}, amount.String(), int(ttl.Seconds())).Result()
	return err
}

// GetPendingSettle 获取用户待结算金额
func (c *AmountCache) GetPendingSettle(ctx context.Context, wallet string) (decimal.Decimal, error) {
	key := pendingSettleKeyPrefix + wallet

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return decimal.NewFromString(val)
}

// AddPendingSettle 增加用户待结算金额
func (c *AmountCache) AddPendingSettle(ctx context.Context, wallet string, amount decimal.Decimal) error {
	key := pendingSettleKeyPrefix + wallet

	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = ARGV[1]

		local current = redis.call('GET', key)
		if current then
			current = current + amount
		else
			current = amount
		end

		redis.call('SET', key, current)
		return current
	`)

	_, err := script.Run(ctx, c.client, []string{key}, amount.String()).Result()
	return err
}

// ReducePendingSettle 减少用户待结算金额
func (c *AmountCache) ReducePendingSettle(ctx context.Context, wallet string, amount decimal.Decimal) error {
	key := pendingSettleKeyPrefix + wallet

	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = tonumber(ARGV[1])

		local current = tonumber(redis.call('GET', key) or '0')
		current = current - amount
		if current < 0 then
			current = 0
		end

		if current == 0 then
			redis.call('DEL', key)
		else
			redis.call('SET', key, current)
		end

		return current
	`)

	_, err := script.Run(ctx, c.client, []string{key}, amount.String()).Result()
	return err
}

// GetSystemPendingSettle 获取系统总待结算金额
func (c *AmountCache) GetSystemPendingSettle(ctx context.Context) (decimal.Decimal, error) {
	val, err := c.client.Get(ctx, systemPendingSettleKey).Result()
	if err != nil {
		if err == redis.Nil {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return decimal.NewFromString(val)
}

// AddSystemPendingSettle 增加系统总待结算金额
func (c *AmountCache) AddSystemPendingSettle(ctx context.Context, amount decimal.Decimal) error {
	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = ARGV[1]

		local current = redis.call('GET', key)
		if current then
			current = current + amount
		else
			current = amount
		end

		redis.call('SET', key, current)
		return current
	`)

	_, err := script.Run(ctx, c.client, []string{systemPendingSettleKey}, amount.String()).Result()
	return err
}

// ReduceSystemPendingSettle 减少系统总待结算金额
func (c *AmountCache) ReduceSystemPendingSettle(ctx context.Context, amount decimal.Decimal) error {
	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = tonumber(ARGV[1])

		local current = tonumber(redis.call('GET', key) or '0')
		current = current - amount
		if current < 0 then
			current = 0
		end

		redis.call('SET', key, current)
		return current
	`)

	_, err := script.Run(ctx, c.client, []string{systemPendingSettleKey}, amount.String()).Result()
	return err
}

// GetDailyResetTime 获取每日重置时间戳 (下一个UTC 00:00)
func (c *AmountCache) GetDailyResetTime() int64 {
	now := time.Now().UTC()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return tomorrow.UnixMilli()
}

// getTimeUntilMidnight 获取到午夜的时间
func (c *AmountCache) getTimeUntilMidnight() time.Duration {
	now := time.Now().UTC()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return tomorrow.Sub(now)
}

// SetPendingSettle 设置用户待结算金额 (用于同步)
func (c *AmountCache) SetPendingSettle(ctx context.Context, wallet string, amount decimal.Decimal) error {
	key := pendingSettleKeyPrefix + wallet

	if amount.IsZero() {
		return c.client.Del(ctx, key).Err()
	}

	return c.client.Set(ctx, key, amount.String(), 0).Err()
}

// ClearDailyWithdraw 清除用户每日提现记录
func (c *AmountCache) ClearDailyWithdraw(ctx context.Context, wallet, token string) error {
	key := dailyWithdrawKey(wallet, token)
	return c.client.Del(ctx, key).Err()
}

// dailyWithdrawKey 生成每日提现键
func dailyWithdrawKey(wallet, token string) string {
	today := time.Now().UTC().Format("20060102")
	if token == "" {
		return fmt.Sprintf("%s%s:%s", dailyWithdrawKeyPrefix, wallet, today)
	}
	return fmt.Sprintf("%s%s:%s:%s", dailyWithdrawKeyPrefix, wallet, token, today)
}
