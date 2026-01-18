package rules

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
)

const (
	SelfTradeRuleID   = "SELF_TRADE_CHECK"
	SelfTradeRuleName = "自成交检查"
	SelfTradeCode     = "SELF_TRADE"
)

// SelfTradeChecker 自成交检查器
type SelfTradeChecker struct {
	cache *cache.OrderCache
}

// NewSelfTradeChecker 创建自成交检查器
func NewSelfTradeChecker(cache *cache.OrderCache) *SelfTradeChecker {
	return &SelfTradeChecker{cache: cache}
}

// Name 返回检查器名称
func (c *SelfTradeChecker) Name() string {
	return "selftrade_checker"
}

// CheckOrder 检查下单请求
func (c *SelfTradeChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	// 获取用户在该市场的活跃订单
	hasOppositeOrder, err := c.cache.HasOppositeOrder(ctx, req.Wallet, req.Market, req.Side, req.Price)
	if err != nil {
		// 缓存查询失败，默认放行但记录警告
		return NewPassedWithWarnings([]string{"自成交检查失败，无法获取订单信息"})
	}

	if hasOppositeOrder {
		return NewRejectedResult(
			SelfTradeRuleID,
			SelfTradeRuleName,
			"存在可能导致自成交的对手订单",
			SelfTradeCode,
		)
	}

	return NewPassedResult()
}

// CheckWithdraw 检查提现请求 (不适用)
func (c *SelfTradeChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	return NewPassedResult()
}
