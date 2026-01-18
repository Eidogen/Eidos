// Package ratelimit 提供限流功能
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlidingWindow 基于 Redis ZSET 的滑动窗口限流器
type SlidingWindow struct {
	rdb    *redis.Client
	script *redis.Script
}

// Lua 脚本：原子操作检查并记录请求
const slidingWindowLua = `
local key = KEYS[1]
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local unique_id = ARGV[4]

-- 清理过期数据
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

-- 检查当前窗口计数
local count = redis.call('ZCARD', key)

if count < limit then
    -- 添加新请求记录
    redis.call('ZADD', key, now, unique_id)
    -- 设置 key 过期时间
    redis.call('PEXPIRE', key, window)
    return 1
else
    return 0
end
`

// NewSlidingWindow 创建滑动窗口限流器
func NewSlidingWindow(rdb *redis.Client) *SlidingWindow {
	return &SlidingWindow{
		rdb:    rdb,
		script: redis.NewScript(slidingWindowLua),
	}
}

// Allow 检查是否允许请求
// key: 限流键
// window: 时间窗口
// limit: 窗口内允许的最大请求数
// uniqueID: 请求唯一标识（防止同一请求重复计数）
func (sw *SlidingWindow) Allow(ctx context.Context, key string, window time.Duration, limit int, uniqueID string) (bool, error) {
	now := time.Now().UnixMilli()

	result, err := sw.script.Run(ctx, sw.rdb, []string{key},
		window.Milliseconds(),
		limit,
		now,
		uniqueID,
	).Int()

	if err != nil {
		return false, fmt.Errorf("execute lua script: %w", err)
	}

	return result == 1, nil
}

// AllowN 检查是否允许 N 个请求
func (sw *SlidingWindow) AllowN(ctx context.Context, key string, window time.Duration, limit, n int, uniqueID string) (bool, error) {
	now := time.Now().UnixMilli()

	// 清理过期数据
	sw.rdb.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-window.Milliseconds()))

	// 检查当前计数
	count, err := sw.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("zcard: %w", err)
	}

	if int(count)+n > limit {
		return false, nil
	}

	// 添加 N 个记录
	members := make([]redis.Z, n)
	for i := 0; i < n; i++ {
		members[i] = redis.Z{
			Score:  float64(now),
			Member: fmt.Sprintf("%s:%d", uniqueID, i),
		}
	}

	sw.rdb.ZAdd(ctx, key, members...)
	sw.rdb.PExpire(ctx, key, window)

	return true, nil
}

// Remaining 返回剩余配额
func (sw *SlidingWindow) Remaining(ctx context.Context, key string, window time.Duration, limit int) (int, error) {
	now := time.Now().UnixMilli()

	// 清理过期数据
	sw.rdb.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-window.Milliseconds()))

	// 获取当前计数
	count, err := sw.rdb.ZCard(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("zcard: %w", err)
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// Reset 重置限流计数
func (sw *SlidingWindow) Reset(ctx context.Context, key string) error {
	return sw.rdb.Del(ctx, key).Err()
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	Remaining(ctx context.Context, key string) (int, error)
}

// Config 限流配置
type Config struct {
	Window time.Duration
	Limit  int
	Prefix string
}

// Limiter 配置化的限流器
type Limiter struct {
	sw     *SlidingWindow
	config Config
}

// NewLimiter 创建配置化限流器
func NewLimiter(sw *SlidingWindow, cfg Config) *Limiter {
	return &Limiter{
		sw:     sw,
		config: cfg,
	}
}

// Allow 检查是否允许请求
func (l *Limiter) Allow(ctx context.Context, identifier, uniqueID string) (bool, error) {
	key := l.config.Prefix + identifier
	return l.sw.Allow(ctx, key, l.config.Window, l.config.Limit, uniqueID)
}

// Remaining 返回剩余配额
func (l *Limiter) Remaining(ctx context.Context, identifier string) (int, error) {
	key := l.config.Prefix + identifier
	return l.sw.Remaining(ctx, key, l.config.Window, l.config.Limit)
}
