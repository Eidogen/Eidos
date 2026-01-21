package middleware

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow() bool
	AllowN(n int) bool
}

// TokenBucketLimiter 令牌桶限流器
type TokenBucketLimiter struct {
	rate       float64 // 每秒产生的令牌数
	burst      int     // 桶容量
	tokens     float64 // 当前令牌数
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTokenBucketLimiter 创建令牌桶限流器
func NewTokenBucketLimiter(rate float64, burst int) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow 检查是否允许一个请求
func (l *TokenBucketLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN 检查是否允许 n 个请求
func (l *TokenBucketLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()
	l.lastUpdate = now

	// 添加新令牌
	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burst) {
		l.tokens = float64(l.burst)
	}

	// 检查是否有足够的令牌
	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}

	return false
}

// SlidingWindowLimiter 滑动窗口限流器
type SlidingWindowLimiter struct {
	windowSize time.Duration
	maxRequests int
	requests    []time.Time
	mu          sync.Mutex
}

// NewSlidingWindowLimiter 创建滑动窗口限流器
func NewSlidingWindowLimiter(windowSize time.Duration, maxRequests int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		windowSize:  windowSize,
		maxRequests: maxRequests,
		requests:    make([]time.Time, 0, maxRequests),
	}
}

// Allow 检查是否允许请求
func (l *SlidingWindowLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN 检查是否允许 n 个请求
func (l *SlidingWindowLimiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-l.windowSize)

	// 清理过期请求
	validRequests := make([]time.Time, 0, len(l.requests))
	for _, t := range l.requests {
		if t.After(windowStart) {
			validRequests = append(validRequests, t)
		}
	}
	l.requests = validRequests

	// 检查是否超过限制
	if len(l.requests)+n > l.maxRequests {
		return false
	}

	// 记录新请求
	for i := 0; i < n; i++ {
		l.requests = append(l.requests, now)
	}

	return true
}

// MethodRateLimiter 方法级别限流器
type MethodRateLimiter struct {
	limiters map[string]RateLimiter
	mu       sync.RWMutex
	factory  func() RateLimiter
}

// NewMethodRateLimiter 创建方法级别限流器
func NewMethodRateLimiter(factory func() RateLimiter) *MethodRateLimiter {
	return &MethodRateLimiter{
		limiters: make(map[string]RateLimiter),
		factory:  factory,
	}
}

// GetLimiter 获取方法的限流器
func (m *MethodRateLimiter) GetLimiter(method string) RateLimiter {
	m.mu.RLock()
	limiter, ok := m.limiters[method]
	m.mu.RUnlock()

	if ok {
		return limiter
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if limiter, ok = m.limiters[method]; ok {
		return limiter
	}

	limiter = m.factory()
	m.limiters[method] = limiter
	return limiter
}

// Allow 检查方法是否允许请求
func (m *MethodRateLimiter) Allow(method string) bool {
	return m.GetLimiter(method).Allow()
}

// RateLimitUnaryServerInterceptor 限流拦截器
func RateLimitUnaryServerInterceptor(limiter RateLimiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(ctx, req)
	}
}

// MethodRateLimitUnaryServerInterceptor 方法级别限流拦截器
func MethodRateLimitUnaryServerInterceptor(limiter *MethodRateLimiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !limiter.Allow(info.FullMethod) {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded for method: "+info.FullMethod)
		}
		return handler(ctx, req)
	}
}

// RateLimitStreamServerInterceptor 流式限流拦截器
func RateLimitStreamServerInterceptor(limiter RateLimiter) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !limiter.Allow() {
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(srv, ss)
	}
}
