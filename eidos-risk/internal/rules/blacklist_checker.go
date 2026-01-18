package rules

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
)

const (
	BlacklistRuleID   = "BLACKLIST_CHECK"
	BlacklistRuleName = "黑名单检查"
	BlacklistCode     = "BLACKLISTED"
)

// BlacklistChecker 黑名单检查器
type BlacklistChecker struct {
	cache *cache.BlacklistCache
}

// NewBlacklistChecker 创建黑名单检查器
func NewBlacklistChecker(cache *cache.BlacklistCache) *BlacklistChecker {
	return &BlacklistChecker{cache: cache}
}

// Name 返回检查器名称
func (c *BlacklistChecker) Name() string {
	return "blacklist_checker"
}

// CheckOrder 检查下单请求
func (c *BlacklistChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	return c.checkWallet(ctx, req.Wallet, "trade")
}

// CheckWithdraw 检查提现请求
func (c *BlacklistChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	// 检查发起钱包
	if result := c.checkWallet(ctx, req.Wallet, "withdraw"); !result.Passed {
		return result
	}

	// 检查目标地址
	if result := c.checkWallet(ctx, req.ToAddress, "withdraw"); !result.Passed {
		result.Reason = "目标地址在黑名单中"
		return result
	}

	return NewPassedResult()
}

// checkWallet 检查钱包是否在黑名单中
func (c *BlacklistChecker) checkWallet(ctx context.Context, wallet, action string) *CheckResult {
	entry, err := c.cache.Get(ctx, wallet)
	if err != nil {
		// 缓存查询失败，默认放行但记录警告
		return NewPassedWithWarnings([]string{"黑名单缓存查询失败"})
	}

	if entry == nil {
		return NewPassedResult()
	}

	// 检查是否过期
	now := time.Now().UnixMilli()
	if entry.EffectiveUntil > 0 && now > entry.EffectiveUntil {
		return NewPassedResult()
	}

	// 检查黑名单类型是否匹配操作
	switch entry.ListType {
	case "full":
		// 禁止所有操作
		return NewRejectedResult(
			BlacklistRuleID,
			BlacklistRuleName,
			"账户已被限制所有操作: "+entry.Reason,
			BlacklistCode,
		)
	case "trade":
		if action == "trade" {
			return NewRejectedResult(
				BlacklistRuleID,
				BlacklistRuleName,
				"账户已被限制交易: "+entry.Reason,
				BlacklistCode,
			)
		}
	case "withdraw":
		if action == "withdraw" {
			return NewRejectedResult(
				BlacklistRuleID,
				BlacklistRuleName,
				"账户已被限制提现: "+entry.Reason,
				BlacklistCode,
			)
		}
	}

	return NewPassedResult()
}
