// Package middleware provides HTTP middleware
package middleware

import (
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
)

// RateLimitTier rate limit tier for different user levels
type RateLimitTier struct {
	Name           string
	OrdersPerSec   int
	QueriesPerSec  int
	WithdrawPerMin int
	CancelsPerSec  int
}

// Predefined rate limit tiers
var (
	TierDefault = &RateLimitTier{
		Name:           "default",
		OrdersPerSec:   10,
		QueriesPerSec:  100,
		WithdrawPerMin: 5,
		CancelsPerSec:  20,
	}

	TierVIP = &RateLimitTier{
		Name:           "vip",
		OrdersPerSec:   50,
		QueriesPerSec:  500,
		WithdrawPerMin: 20,
		CancelsPerSec:  100,
	}

	TierMarketMaker = &RateLimitTier{
		Name:           "market_maker",
		OrdersPerSec:   200,
		QueriesPerSec:  1000,
		WithdrawPerMin: 50,
		CancelsPerSec:  500,
	}
)

// TierResolver resolves rate limit tier for a wallet
type TierResolver interface {
	GetTier(wallet string) *RateLimitTier
}

// DefaultTierResolver default tier resolver (always returns default tier)
type DefaultTierResolver struct{}

// GetTier returns default tier
func (r *DefaultTierResolver) GetTier(string) *RateLimitTier {
	return TierDefault
}

// RateLimitByWalletTier rate limits by wallet with tier support
func RateLimitByWalletTier(sw *ratelimit.SlidingWindow, resolver TierResolver, category string) gin.HandlerFunc {
	if resolver == nil {
		resolver = &DefaultTierResolver{}
	}

	return func(c *gin.Context) {
		wallet, exists := c.Get("wallet")
		if !exists {
			c.Next()
			return
		}

		walletAddr := wallet.(string)
		tier := resolver.GetTier(walletAddr)

		var limit int
		var window time.Duration

		switch category {
		case "orders":
			limit = tier.OrdersPerSec
			window = time.Second
		case "queries":
			limit = tier.QueriesPerSec
			window = time.Second
		case "withdrawals":
			limit = tier.WithdrawPerMin
			window = time.Minute
		case "cancels":
			limit = tier.CancelsPerSec
			window = time.Second
		default:
			limit = tier.QueriesPerSec
			window = time.Second
		}

		key := RateLimitKeyPrefixWallet + walletAddr + ":" + category
		RateLimit(RateLimitConfig{
			Limiter:  sw,
			Limit:    limit,
			Window:   window,
			Category: category,
			KeyFunc: func(_ *gin.Context) string {
				return key
			},
		})(c)
	}
}

// RateLimitByEndpoint rate limits by specific endpoint
func RateLimitByEndpoint(sw *ratelimit.SlidingWindow, endpoint string, limit int, window time.Duration) gin.HandlerFunc {
	return RateLimit(RateLimitConfig{
		Limiter:  sw,
		Limit:    limit,
		Window:   window,
		Category: endpoint,
		KeyFunc: func(c *gin.Context) string {
			// Use IP + endpoint as key
			return RateLimitKeyPrefixIP + c.ClientIP() + ":" + endpoint
		},
	})
}

// GetRealClientIP extracts the real client IP from various headers
func GetRealClientIP(c *gin.Context) string {
	// Check X-Real-IP header (nginx)
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			return ip
		}
	}

	// Check X-Forwarded-For header
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if parsedIP := net.ParseIP(ip); parsedIP != nil {
				return ip
			}
		}
	}

	// Check CF-Connecting-IP (Cloudflare)
	if ip := c.GetHeader("CF-Connecting-IP"); ip != "" {
		if parsedIP := net.ParseIP(ip); parsedIP != nil {
			return ip
		}
	}

	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

// IPWhitelist middleware to bypass rate limiting for whitelisted IPs
func IPWhitelist(whitelist []string) gin.HandlerFunc {
	whitelistSet := make(map[string]bool)
	for _, ip := range whitelist {
		whitelistSet[ip] = true
	}

	return func(c *gin.Context) {
		clientIP := GetRealClientIP(c)
		if whitelistSet[clientIP] {
			c.Set("rate_limit_bypass", true)
		}
		c.Next()
	}
}

// IPBlacklist middleware to block blacklisted IPs
func IPBlacklist(blacklist []string) gin.HandlerFunc {
	blacklistSet := make(map[string]bool)
	for _, ip := range blacklist {
		blacklistSet[ip] = true
	}

	return func(c *gin.Context) {
		clientIP := GetRealClientIP(c)
		if blacklistSet[clientIP] {
			c.AbortWithStatusJSON(dto.ErrForbidden.HTTPStatus, dto.NewErrorResponse(
				dto.ErrForbidden.WithMessage("IP is blacklisted"),
			))
			return
		}
		c.Next()
	}
}

// AdaptiveRateLimit implements adaptive rate limiting based on system load
type AdaptiveRateLimit struct {
	sw          *ratelimit.SlidingWindow
	baseLimit   int
	window      time.Duration
	loadFactor  float64 // 0.0 to 1.0, where 1.0 means full load
}

// NewAdaptiveRateLimit creates adaptive rate limiter
func NewAdaptiveRateLimit(sw *ratelimit.SlidingWindow, baseLimit int, window time.Duration) *AdaptiveRateLimit {
	return &AdaptiveRateLimit{
		sw:         sw,
		baseLimit:  baseLimit,
		window:     window,
		loadFactor: 0.0,
	}
}

// SetLoadFactor updates the load factor (should be called periodically based on system metrics)
func (a *AdaptiveRateLimit) SetLoadFactor(factor float64) {
	if factor < 0 {
		factor = 0
	}
	if factor > 1 {
		factor = 1
	}
	a.loadFactor = factor
}

// GetEffectiveLimit returns the effective limit based on current load
func (a *AdaptiveRateLimit) GetEffectiveLimit() int {
	// Reduce limit as load increases
	// At 100% load, reduce to 50% of base limit
	reduction := a.loadFactor * 0.5
	effectiveLimit := float64(a.baseLimit) * (1.0 - reduction)
	if effectiveLimit < 1 {
		effectiveLimit = 1
	}
	return int(effectiveLimit)
}

// Middleware returns the rate limit middleware
func (a *AdaptiveRateLimit) Middleware(keyFunc func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check bypass
		if bypass, exists := c.Get("rate_limit_bypass"); exists && bypass.(bool) {
			c.Next()
			return
		}

		key := keyFunc(c)
		limit := a.GetEffectiveLimit()

		cfg := RateLimitConfig{
			Limiter:  a.sw,
			Limit:    limit,
			Window:   a.window,
			Category: "adaptive",
			KeyFunc: func(_ *gin.Context) string {
				return key
			},
		}
		RateLimit(cfg)(c)
	}
}

// ConcurrentRequestLimiter limits concurrent requests per key
type ConcurrentRequestLimiter struct {
	limiter *ratelimit.ConcurrencyLimiter
	limit   int
}

// NewConcurrentRequestLimiter creates concurrent request limiter
func NewConcurrentRequestLimiter(limiter *ratelimit.ConcurrencyLimiter, limit int) *ConcurrentRequestLimiter {
	return &ConcurrentRequestLimiter{
		limiter: limiter,
		limit:   limit,
	}
}

// Middleware returns the concurrent request limit middleware
func (l *ConcurrentRequestLimiter) Middleware(keyFunc func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)

		acquired, err := l.limiter.Acquire(c.Request.Context(), key, l.limit)
		if err != nil {
			// On error, allow request
			c.Next()
			return
		}

		if !acquired {
			c.AbortWithStatusJSON(dto.ErrRateLimitExceeded.HTTPStatus, dto.NewErrorResponse(
				dto.ErrRateLimitExceeded.WithMessage("too many concurrent requests"),
			))
			return
		}

		defer l.limiter.Release(c.Request.Context(), key)
		c.Next()
	}
}
