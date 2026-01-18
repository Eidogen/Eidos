package middleware

import (
	"fmt"
	"strconv"
	"time"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RateLimitKeyPrefixIP IP 限流 Key 前缀
	RateLimitKeyPrefixIP = "eidos:api:ratelimit:ip:"
	// RateLimitKeyPrefixWallet 钱包限流 Key 前缀
	RateLimitKeyPrefixWallet = "eidos:api:ratelimit:wallet:"
	// RateLimitKeyPrefixGlobal 全局限流 Key 前缀
	RateLimitKeyPrefixGlobal = "eidos:api:ratelimit:global:"
)

// RateLimitConfig 限流中间件配置
type RateLimitConfig struct {
	Limiter  *ratelimit.SlidingWindow
	Limit    int
	Window   time.Duration
	KeyFunc  func(*gin.Context) string
	Category string // 用于错误信息
}

// RateLimitByIP 返回基于 IP 的限流中间件
func RateLimitByIP(sw *ratelimit.SlidingWindow, limit int, window time.Duration) gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Limiter:  sw,
		Limit:    limit,
		Window:   window,
		Category: "ip",
		KeyFunc: func(c *gin.Context) string {
			return RateLimitKeyPrefixIP + c.ClientIP()
		},
	})
}

// RateLimitByWallet 返回基于钱包的限流中间件
func RateLimitByWallet(sw *ratelimit.SlidingWindow, limit int, window time.Duration, category string) gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Limiter:  sw,
		Limit:    limit,
		Window:   window,
		Category: category,
		KeyFunc: func(c *gin.Context) string {
			wallet, _ := c.Get("wallet")
			if w, ok := wallet.(string); ok {
				return RateLimitKeyPrefixWallet + w + ":" + category
			}
			return RateLimitKeyPrefixWallet + "unknown:" + category
		},
	})
}

// RateLimitGlobal 返回全局限流中间件
func RateLimitGlobal(sw *ratelimit.SlidingWindow, limit int, window time.Duration, category string) gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Limiter:  sw,
		Limit:    limit,
		Window:   window,
		Category: category,
		KeyFunc: func(c *gin.Context) string {
			return RateLimitKeyPrefixGlobal + category
		},
	})
}

// RateLimit 返回通用限流中间件
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := cfg.KeyFunc(c)
		uniqueID := uuid.New().String()

		allowed, err := cfg.Limiter.Allow(c.Request.Context(), key, cfg.Window, cfg.Limit, uniqueID)
		if err != nil {
			// 限流检查出错时放行，但记录错误
			c.Next()
			return
		}

		if !allowed {
			// 计算重试时间
			retryAfter := int(cfg.Window.Seconds())

			// 设置响应头
			c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.Limit))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(cfg.Window).Unix(), 10))
			c.Header("Retry-After", strconv.Itoa(retryAfter))

			// 返回限流错误
			c.AbortWithStatusJSON(dto.ErrRateLimitExceeded.HTTPStatus, dto.NewRateLimitResponse(
				cfg.Limit,
				formatWindow(cfg.Window),
				retryAfter,
			))
			return
		}

		// 获取剩余配额
		remaining, _ := cfg.Limiter.Remaining(c.Request.Context(), key, cfg.Window, cfg.Limit)

		// 设置响应头
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(cfg.Window).Unix(), 10))

		c.Next()
	}
}

// formatWindow 格式化时间窗口
func formatWindow(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
