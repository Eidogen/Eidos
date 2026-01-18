package rules

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
)

const (
	RateLimitRuleID   = "RATE_LIMIT_CHECK"
	RateLimitRuleName = "频率限制检查"
	RateLimitCode     = "RATE_LIMITED"
)

// RateLimitConfig 频率限制配置
type RateLimitConfig struct {
	OrdersPerSecond  int // 每秒下单数限制
	OrdersPerMinute  int // 每分钟下单数限制
	CancelsPerMinute int // 每分钟取消数限制
}

// RateLimitChecker 频率限制检查器
type RateLimitChecker struct {
	cache  *cache.RateLimitCache
	config *RateLimitConfig
}

// NewRateLimitChecker 创建频率限制检查器
func NewRateLimitChecker(cache *cache.RateLimitCache, config *RateLimitConfig) *RateLimitChecker {
	if config == nil {
		config = &RateLimitConfig{
			OrdersPerSecond:  10,
			OrdersPerMinute:  300,
			CancelsPerMinute: 100,
		}
	}
	return &RateLimitChecker{
		cache:  cache,
		config: config,
	}
}

// Name 返回检查器名称
func (c *RateLimitChecker) Name() string {
	return "ratelimit_checker"
}

// CheckOrder 检查下单请求
func (c *RateLimitChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	// 检查每秒限制
	allowed, err := c.cache.CheckAndIncrement(ctx, req.Wallet, "order_sec", 1, c.config.OrdersPerSecond)
	if err != nil {
		return NewPassedWithWarnings([]string{"频率限制检查失败"})
	}
	if !allowed {
		return NewRejectedResult(
			RateLimitRuleID,
			RateLimitRuleName,
			fmt.Sprintf("下单过于频繁，每秒最多%d笔", c.config.OrdersPerSecond),
			RateLimitCode,
		)
	}

	// 检查每分钟限制
	allowed, err = c.cache.CheckAndIncrement(ctx, req.Wallet, "order_min", 60, c.config.OrdersPerMinute)
	if err != nil {
		return NewPassedWithWarnings([]string{"频率限制检查失败"})
	}
	if !allowed {
		return NewRejectedResult(
			RateLimitRuleID,
			RateLimitRuleName,
			fmt.Sprintf("下单过于频繁，每分钟最多%d笔", c.config.OrdersPerMinute),
			RateLimitCode,
		)
	}

	return NewPassedResult()
}

// CheckWithdraw 检查提现请求
func (c *RateLimitChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	// 提现每小时最多3次
	allowed, err := c.cache.CheckAndIncrement(ctx, req.Wallet, "withdraw_hour", 3600, 3)
	if err != nil {
		return NewPassedWithWarnings([]string{"频率限制检查失败"})
	}
	if !allowed {
		return NewRejectedResult(
			RateLimitRuleID,
			RateLimitRuleName,
			"提现过于频繁，每小时最多3次",
			RateLimitCode,
		)
	}

	return NewPassedResult()
}

// CheckCancel 检查取消订单请求
func (c *RateLimitChecker) CheckCancel(ctx context.Context, wallet string) *CheckResult {
	allowed, err := c.cache.CheckAndIncrement(ctx, wallet, "cancel_min", 60, c.config.CancelsPerMinute)
	if err != nil {
		return NewPassedWithWarnings([]string{"频率限制检查失败"})
	}
	if !allowed {
		return NewRejectedResult(
			RateLimitRuleID,
			RateLimitRuleName,
			fmt.Sprintf("取消订单过于频繁，每分钟最多%d次", c.config.CancelsPerMinute),
			RateLimitCode,
		)
	}
	return NewPassedResult()
}
