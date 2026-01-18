package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rateLimitKeyPrefix = "risk:ratelimit:"
)

// RateLimitCache 频率限制缓存
type RateLimitCache struct {
	client redis.UniversalClient
}

// NewRateLimitCache 创建频率限制缓存
func NewRateLimitCache(client redis.UniversalClient) *RateLimitCache {
	return &RateLimitCache{client: client}
}

// CheckAndIncrement 检查并增加计数，返回是否允许
// windowSec: 时间窗口(秒), maxCount: 最大次数
func (c *RateLimitCache) CheckAndIncrement(ctx context.Context, wallet, action string, windowSec, maxCount int) (bool, error) {
	key := rateLimitKey(wallet, action, windowSec)

	// 使用 Lua 脚本保证原子性
	script := redis.NewScript(`
		local key = KEYS[1]
		local maxCount = tonumber(ARGV[1])
		local windowSec = tonumber(ARGV[2])

		local current = tonumber(redis.call('GET', key) or '0')
		if current >= maxCount then
			return 0
		end

		local newCount = redis.call('INCR', key)
		if newCount == 1 then
			redis.call('EXPIRE', key, windowSec)
		end

		return 1
	`)

	result, err := script.Run(ctx, c.client, []string{key}, maxCount, windowSec).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// GetCount 获取当前计数
func (c *RateLimitCache) GetCount(ctx context.Context, wallet, action string, windowSec int) (int, error) {
	key := rateLimitKey(wallet, action, windowSec)
	count, err := c.client.Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

// Reset 重置计数
func (c *RateLimitCache) Reset(ctx context.Context, wallet, action string, windowSec int) error {
	key := rateLimitKey(wallet, action, windowSec)
	return c.client.Del(ctx, key).Err()
}

// GetTTL 获取剩余过期时间
func (c *RateLimitCache) GetTTL(ctx context.Context, wallet, action string, windowSec int) (time.Duration, error) {
	key := rateLimitKey(wallet, action, windowSec)
	return c.client.TTL(ctx, key).Result()
}

// IncrementBy 增加指定计数
func (c *RateLimitCache) IncrementBy(ctx context.Context, wallet, action string, windowSec, delta int) (int, error) {
	key := rateLimitKey(wallet, action, windowSec)

	pipe := c.client.Pipeline()
	incrCmd := pipe.IncrBy(ctx, key, int64(delta))
	pipe.Expire(ctx, key, time.Duration(windowSec)*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	return int(incrCmd.Val()), nil
}

// GetMultiple 批量获取计数
func (c *RateLimitCache) GetMultiple(ctx context.Context, wallet string, actions []string, windowSec int) (map[string]int, error) {
	pipe := c.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)

	for _, action := range actions {
		key := rateLimitKey(wallet, action, windowSec)
		cmds[action] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[string]int)
	for action, cmd := range cmds {
		count, _ := cmd.Int()
		result[action] = count
	}

	return result, nil
}

// SlidingWindowCheck 滑动窗口限流检查
func (c *RateLimitCache) SlidingWindowCheck(ctx context.Context, wallet, action string, windowSec, maxCount int) (bool, error) {
	key := fmt.Sprintf("%s%s:%s:sliding:%d", rateLimitKeyPrefix, wallet, action, windowSec)
	now := time.Now().UnixMilli()
	windowStart := now - int64(windowSec*1000)

	// 使用 Lua 脚本实现滑动窗口
	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local windowStart = tonumber(ARGV[2])
		local maxCount = tonumber(ARGV[3])
		local windowMs = tonumber(ARGV[4])

		-- 移除窗口外的记录
		redis.call('ZREMRANGEBYSCORE', key, '-inf', windowStart)

		-- 获取当前窗口内的数量
		local count = redis.call('ZCARD', key)

		if count >= maxCount then
			return 0
		end

		-- 添加新记录
		redis.call('ZADD', key, now, now)
		redis.call('PEXPIRE', key, windowMs)

		return 1
	`)

	result, err := script.Run(ctx, c.client, []string{key}, now, windowStart, maxCount, windowSec*1000).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// rateLimitKey 生成限流键
func rateLimitKey(wallet, action string, windowSec int) string {
	return fmt.Sprintf("%s%s:%s:%d", rateLimitKeyPrefix, wallet, action, windowSec)
}
