package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/redis/scripts"
)

var (
	// ErrRateLimitExceeded 超过限流
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// RateLimitResult 限流结果
type RateLimitResult struct {
	Allowed   bool
	Current   int64
	Remaining int64
	RetryAt   time.Time
	TTL       time.Duration
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(ctx context.Context, key string) (*RateLimitResult, error)
	AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error)
}

// FixedWindowLimiter 固定窗口限流器
type FixedWindowLimiter struct {
	client   redis.UniversalClient
	prefix   string
	window   time.Duration
	limit    int64
}

// NewFixedWindowLimiter 创建固定窗口限流器
func NewFixedWindowLimiter(client redis.UniversalClient, prefix string, window time.Duration, limit int64) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		client: client,
		prefix: prefix,
		window: window,
		limit:  limit,
	}
}

// Allow 检查是否允许
func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 N 次请求
func (l *FixedWindowLimiter) AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error) {
	fullKey := l.prefix + key

	script := scripts.RateLimitScripts.FixedWindowWithInfo
	result, err := l.client.Eval(ctx, script, []string{fullKey}, int(l.window.Seconds()), l.limit).Slice()
	if err != nil {
		return nil, fmt.Errorf("fixed window rate limit failed: %w", err)
	}

	allowed := result[0].(int64) == 1
	current := result[1].(int64)
	remaining := result[2].(int64)
	ttl := result[3].(int64)

	return &RateLimitResult{
		Allowed:   allowed,
		Current:   current,
		Remaining: remaining,
		TTL:       time.Duration(ttl) * time.Second,
	}, nil
}

// SlidingWindowLimiter 滑动窗口限流器
type SlidingWindowLimiter struct {
	client   redis.UniversalClient
	prefix   string
	window   time.Duration
	limit    int64
}

// NewSlidingWindowLimiter 创建滑动窗口限流器
func NewSlidingWindowLimiter(client redis.UniversalClient, prefix string, window time.Duration, limit int64) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		client: client,
		prefix: prefix,
		window: window,
		limit:  limit,
	}
}

// Allow 检查是否允许
func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许 N 次请求
func (l *SlidingWindowLimiter) AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error) {
	fullKey := l.prefix + key
	now := time.Now().UnixMilli()

	script := scripts.RateLimitScripts.SlidingWindowWithInfo
	result, err := l.client.Eval(ctx, script, []string{fullKey}, l.window.Milliseconds(), l.limit, now).Slice()
	if err != nil {
		return nil, fmt.Errorf("sliding window rate limit failed: %w", err)
	}

	allowed := result[0].(int64) == 1
	current := result[1].(int64)
	remaining := result[2].(int64)

	return &RateLimitResult{
		Allowed:   allowed,
		Current:   current,
		Remaining: remaining,
	}, nil
}

// TokenBucketLimiter 令牌桶限流器
type TokenBucketLimiter struct {
	client   redis.UniversalClient
	prefix   string
	capacity int64
	rate     float64 // 每秒生成令牌数
}

// NewTokenBucketLimiter 创建令牌桶限流器
func NewTokenBucketLimiter(client redis.UniversalClient, prefix string, capacity int64, rate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		client:   client,
		prefix:   prefix,
		capacity: capacity,
		rate:     rate,
	}
}

// Allow 检查是否允许
func (l *TokenBucketLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许消耗 N 个令牌
func (l *TokenBucketLimiter) AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error) {
	fullKey := l.prefix + key
	now := time.Now().UnixMilli()

	script := scripts.RateLimitScripts.TokenBucketWithInfo
	result, err := l.client.Eval(ctx, script, []string{fullKey}, l.capacity, l.rate, now, n).Slice()
	if err != nil {
		return nil, fmt.Errorf("token bucket rate limit failed: %w", err)
	}

	allowed := result[0].(int64) == 1
	tokens := result[1].(int64)
	nextAvailable := result[2].(int64)

	res := &RateLimitResult{
		Allowed:   allowed,
		Current:   l.capacity - tokens,
		Remaining: tokens,
	}

	if nextAvailable > 0 {
		res.RetryAt = time.Now().Add(time.Duration(nextAvailable) * time.Millisecond)
	}

	return res, nil
}

// LeakyBucketLimiter 漏桶限流器
type LeakyBucketLimiter struct {
	client   redis.UniversalClient
	prefix   string
	capacity int64
	rate     float64 // 每秒漏出速率
}

// NewLeakyBucketLimiter 创建漏桶限流器
func NewLeakyBucketLimiter(client redis.UniversalClient, prefix string, capacity int64, rate float64) *LeakyBucketLimiter {
	return &LeakyBucketLimiter{
		client:   client,
		prefix:   prefix,
		capacity: capacity,
		rate:     rate,
	}
}

// Allow 检查是否允许
func (l *LeakyBucketLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	fullKey := l.prefix + key
	now := time.Now().UnixMilli()

	script := scripts.RateLimitScripts.LeakyBucketWithInfo
	result, err := l.client.Eval(ctx, script, []string{fullKey}, l.capacity, l.rate, now).Slice()
	if err != nil {
		return nil, fmt.Errorf("leaky bucket rate limit failed: %w", err)
	}

	allowed := result[0].(int64) == 1
	water := result[1].(int64)
	waitTime := result[2].(int64)

	res := &RateLimitResult{
		Allowed:   allowed,
		Current:   water,
		Remaining: l.capacity - water,
	}

	if waitTime > 0 {
		res.RetryAt = time.Now().Add(time.Duration(waitTime) * time.Millisecond)
	}

	return res, nil
}

// ConcurrencyLimiter 并发限制器
type ConcurrencyLimiter struct {
	client  redis.UniversalClient
	prefix  string
	limit   int64
	timeout time.Duration
}

// NewConcurrencyLimiter 创建并发限制器
func NewConcurrencyLimiter(client redis.UniversalClient, prefix string, limit int64, timeout time.Duration) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		client:  client,
		prefix:  prefix,
		limit:   limit,
		timeout: timeout,
	}
}

// Acquire 获取并发槽
func (l *ConcurrencyLimiter) Acquire(ctx context.Context, key, requestID string) (bool, error) {
	fullKey := l.prefix + key

	script := scripts.RateLimitScripts.ConcurrencyLimit
	result, err := l.client.Eval(ctx, script, []string{fullKey}, l.limit, requestID, l.timeout.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("concurrency limit acquire failed: %w", err)
	}

	return result == 1, nil
}

// Release 释放并发槽
func (l *ConcurrencyLimiter) Release(ctx context.Context, key, requestID string) error {
	fullKey := l.prefix + key

	script := scripts.RateLimitScripts.ConcurrencyRelease
	_, err := l.client.Eval(ctx, script, []string{fullKey}, requestID).Int64()
	if err != nil {
		return fmt.Errorf("concurrency limit release failed: %w", err)
	}

	return nil
}

// WithConcurrencyLimit 在并发限制下执行
func (l *ConcurrencyLimiter) WithConcurrencyLimit(ctx context.Context, key, requestID string, fn func() error) error {
	acquired, err := l.Acquire(ctx, key, requestID)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrRateLimitExceeded
	}

	defer l.Release(ctx, key, requestID)

	return fn()
}

// QuotaLimiter 配额限制器
type QuotaLimiter struct {
	client redis.UniversalClient
	prefix string
	period time.Duration
	limit  int64
}

// NewQuotaLimiter 创建配额限制器
func NewQuotaLimiter(client redis.UniversalClient, prefix string, period time.Duration, limit int64) *QuotaLimiter {
	return &QuotaLimiter{
		client: client,
		prefix: prefix,
		period: period,
		limit:  limit,
	}
}

// Allow 检查是否允许
func (l *QuotaLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否允许消耗 N 个配额
func (l *QuotaLimiter) AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error) {
	fullKey := l.prefix + key

	script := scripts.RateLimitScripts.QuotaLimitWithInfo
	result, err := l.client.Eval(ctx, script, []string{fullKey}, int(l.period.Seconds()), l.limit, n).Slice()
	if err != nil {
		return nil, fmt.Errorf("quota limit failed: %w", err)
	}

	allowed := result[0].(int64) == 1
	used := result[1].(int64)
	remaining := result[2].(int64)
	ttl := result[3].(int64)

	return &RateLimitResult{
		Allowed:   allowed,
		Current:   used,
		Remaining: remaining,
		TTL:       time.Duration(ttl) * time.Second,
		RetryAt:   time.Now().Add(time.Duration(ttl) * time.Second),
	}, nil
}

// CompositeRateLimiter 组合限流器
type CompositeRateLimiter struct {
	limiters []struct {
		name    string
		limiter RateLimiter
	}
}

// NewCompositeRateLimiter 创建组合限流器
func NewCompositeRateLimiter() *CompositeRateLimiter {
	return &CompositeRateLimiter{}
}

// Add 添加限流器
func (l *CompositeRateLimiter) Add(name string, limiter RateLimiter) *CompositeRateLimiter {
	l.limiters = append(l.limiters, struct {
		name    string
		limiter RateLimiter
	}{name: name, limiter: limiter})
	return l
}

// Allow 检查是否所有限流器都允许
func (l *CompositeRateLimiter) Allow(ctx context.Context, key string) (*RateLimitResult, error) {
	return l.AllowN(ctx, key, 1)
}

// AllowN 检查是否所有限流器都允许 N 次请求
func (l *CompositeRateLimiter) AllowN(ctx context.Context, key string, n int64) (*RateLimitResult, error) {
	var minRemaining int64 = -1
	var latestRetryAt time.Time

	for _, item := range l.limiters {
		result, err := item.limiter.AllowN(ctx, key, n)
		if err != nil {
			return nil, fmt.Errorf("limiter %s failed: %w", item.name, err)
		}

		if !result.Allowed {
			return result, nil
		}

		if minRemaining == -1 || result.Remaining < minRemaining {
			minRemaining = result.Remaining
		}

		if result.RetryAt.After(latestRetryAt) {
			latestRetryAt = result.RetryAt
		}
	}

	return &RateLimitResult{
		Allowed:   true,
		Remaining: minRemaining,
		RetryAt:   latestRetryAt,
	}, nil
}

// RateLimitMiddleware 限流中间件辅助函数
type RateLimitMiddleware struct {
	limiter RateLimiter
	keyFunc func(interface{}) string
}

// NewRateLimitMiddleware 创建限流中间件
func NewRateLimitMiddleware(limiter RateLimiter, keyFunc func(interface{}) string) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
		keyFunc: keyFunc,
	}
}

// Check 检查请求是否被限流
func (m *RateLimitMiddleware) Check(ctx context.Context, request interface{}) (*RateLimitResult, error) {
	key := m.keyFunc(request)
	return m.limiter.Allow(ctx, key)
}
