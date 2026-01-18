package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestRedis 设置测试用 Redis
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return mr, rdb
}

// TestRateLimitByIP_UnderLimit 测试 IP 限流在限制内
func TestRateLimitByIP_UnderLimit_AllowsRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 10
	window := time.Minute

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)

	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 发送 5 个请求，都应该通过
	for i := 0; i < 5; i++ {
		w = httptest.NewRecorder()
		c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
		c.Request.RemoteAddr = "192.168.1.1:12345"
		r.ServeHTTP(w, c.Request)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	}
}

// TestRateLimitByIP_OverLimit 测试 IP 限流超过限制
func TestRateLimitByIP_OverLimit_BlocksRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 3
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 发送 limit 个请求，都应该通过
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Request %d should pass", i+1)
	}

	// 第 limit+1 个请求应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrRateLimitExceeded.Code, resp.Code)
}

// TestRateLimitByIP_DifferentIPs 测试不同 IP 有独立限额
func TestRateLimitByIP_DifferentIPs_IndependentLimits(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 2
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// IP1 发送 limit 个请求
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// IP1 的下一个请求应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// IP2 应该仍然可以请求
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRateLimitByWallet_UnderLimit 测试钱包限流在限制内
func TestRateLimitByWallet_UnderLimit_AllowsRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 10
	window := time.Second

	r := gin.New()
	// 模拟认证中间件设置 wallet
	r.Use(func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		c.Next()
	})
	r.Use(RateLimitByWallet(sw, limit, window, "api"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 发送 5 个请求，都应该通过
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

// TestRateLimitByWallet_OverLimit 测试钱包限流超过限制
func TestRateLimitByWallet_OverLimit_BlocksRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 3
	window := time.Minute

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		c.Next()
	})
	r.Use(RateLimitByWallet(sw, limit, window, "api"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 发送 limit 个请求
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 第 limit+1 个请求应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimitByWallet_DifferentWallets 测试不同钱包有独立限额
func TestRateLimitByWallet_DifferentWallets_IndependentLimits(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 2
	window := time.Minute

	wallet1 := "0x1111111111111111111111111111111111111111"
	wallet2 := "0x2222222222222222222222222222222222222222"

	r := gin.New()
	r.Use(RateLimitByWallet(sw, limit, window, "api"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 设置 wallet1 并发送 limit 个请求
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = req
		ctx.Set("wallet", wallet1)
		r.HandleContext(ctx)
		// 使用 ServeHTTP 时需要通过中间件设置 wallet
	}

	// 使用完整的请求流程测试
	r2 := gin.New()
	r2.Use(func(c *gin.Context) {
		wallet := c.GetHeader("X-Wallet")
		if wallet != "" {
			c.Set("wallet", wallet)
		}
		c.Next()
	})
	r2.Use(RateLimitByWallet(sw, limit, window, "api"))
	r2.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Wallet1 发送 limit 个请求
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Wallet", wallet1)
		r2.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Wallet1 的下一个请求应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Wallet", wallet1)
	r2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Wallet2 应该仍然可以请求
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Wallet", wallet2)
	r2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRateLimitByWallet_NoWallet 测试没有 wallet 时使用默认 key
func TestRateLimitByWallet_NoWallet_UsesUnknownKey(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 2
	window := time.Minute

	r := gin.New()
	// 不设置 wallet
	r.Use(RateLimitByWallet(sw, limit, window, "api"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 所有没有 wallet 的请求共享同一个限额
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 超过限额后应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimitGlobal_OverLimit 测试全局限流
func TestRateLimitGlobal_OverLimit_BlocksRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 5
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitGlobal(sw, limit, window, "test"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 发送 limit 个请求
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1." + string(rune('1'+i)) + ":12345" // 不同 IP
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// 即使不同 IP，全局限流后也应该被拒绝
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// TestRateLimit_RedisError 测试 Redis 错误时放行
func TestRateLimit_RedisError_AllowsRequests(t *testing.T) {
	mr, rdb := setupTestRedis(t)

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 3
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 关闭 Redis 模拟错误
	mr.Close()

	// 请求应该仍然通过（降级策略）
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestFormatWindow 测试时间窗口格式化
func TestFormatWindow(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{time.Minute, "1m"},
		{30 * time.Minute, "30m"},
		{time.Second, "1s"},
		{30 * time.Second, "30s"},
		{90 * time.Minute, "1h"}, // 90 分钟 = 1.5 小时，取整为 1h
	}

	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			got := formatWindow(tt.duration)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRateLimitHeaders 测试限流响应头
func TestRateLimitHeaders_AreSetCorrectly(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 10
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	// Remaining 应该小于 limit
	remaining := w.Header().Get("X-RateLimit-Remaining")
	assert.NotEmpty(t, remaining)
	// Reset 应该是未来的时间戳
	reset := w.Header().Get("X-RateLimit-Reset")
	assert.NotEmpty(t, reset)
}

// BenchmarkRateLimitByIP 基准测试 IP 限流
func BenchmarkRateLimitByIP(b *testing.B) {
	mr := miniredis.RunT(b)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	limit := 1000000 // 高限额避免触发限流
	window := time.Minute

	r := gin.New()
	r.Use(RateLimitByIP(sw, limit, window))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		r.ServeHTTP(w, req)
	}
}

// TestSlidingWindow_Allow 测试滑动窗口允许方法
func TestSlidingWindow_Allow(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	ctx := context.Background()
	key := "test:allow"
	window := time.Minute
	limit := 3

	// 前 3 个请求应该允许
	for i := 0; i < limit; i++ {
		allowed, err := sw.Allow(ctx, key, window, limit, "req-"+string(rune('a'+i)))
		assert.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 第 4 个请求应该被拒绝
	allowed, err := sw.Allow(ctx, key, window, limit, "req-d")
	assert.NoError(t, err)
	assert.False(t, allowed)
}

// TestSlidingWindow_Remaining 测试剩余配额
func TestSlidingWindow_Remaining(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	ctx := context.Background()
	key := "test:remaining"
	window := time.Minute
	limit := 5

	// 初始剩余应该是 limit
	remaining, err := sw.Remaining(ctx, key, window, limit)
	assert.NoError(t, err)
	assert.Equal(t, limit, remaining)

	// 发送 2 个请求
	sw.Allow(ctx, key, window, limit, "req-1")
	sw.Allow(ctx, key, window, limit, "req-2")

	// 剩余应该是 limit - 2
	remaining, err = sw.Remaining(ctx, key, window, limit)
	assert.NoError(t, err)
	assert.Equal(t, limit-2, remaining)
}

// TestSlidingWindow_Reset 测试重置限流
func TestSlidingWindow_Reset(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer mr.Close()
	defer rdb.Close()

	sw := ratelimit.NewSlidingWindow(rdb)
	ctx := context.Background()
	key := "test:reset"
	window := time.Minute
	limit := 3

	// 消耗所有配额
	for i := 0; i < limit; i++ {
		sw.Allow(ctx, key, window, limit, "req-"+string(rune('a'+i)))
	}

	// 确认配额已用完
	allowed, _ := sw.Allow(ctx, key, window, limit, "req-extra")
	assert.False(t, allowed)

	// 重置
	err := sw.Reset(ctx, key)
	assert.NoError(t, err)

	// 重置后应该可以再次请求
	allowed, err = sw.Allow(ctx, key, window, limit, "req-after-reset")
	assert.NoError(t, err)
	assert.True(t, allowed)
}
