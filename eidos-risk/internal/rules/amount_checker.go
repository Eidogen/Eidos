package rules

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/shopspring/decimal"
)

const (
	AmountLimitRuleID   = "AMOUNT_LIMIT_CHECK"
	AmountLimitRuleName = "金额限制检查"
	AmountLimitCode     = "EXCEED_LIMIT"
)

// AmountLimitConfig 金额限制配置
type AmountLimitConfig struct {
	SingleOrderMax      decimal.Decimal // 单笔订单金额上限
	DailyWithdrawMax    decimal.Decimal // 单用户每日提现上限
	SingleWithdrawMax   decimal.Decimal // 单笔提现上限
	PendingSettleMax    decimal.Decimal // 用户待结算余额上限
	LargeTradeThreshold decimal.Decimal // 大额交易阈值（触发告警）
}

// AmountChecker 金额限制检查器
type AmountChecker struct {
	cache  *cache.AmountCache
	config *AmountLimitConfig
}

// NewAmountChecker 创建金额限制检查器
func NewAmountChecker(cache *cache.AmountCache, config *AmountLimitConfig) *AmountChecker {
	if config == nil {
		config = &AmountLimitConfig{
			SingleOrderMax:      decimal.NewFromInt(50000),
			DailyWithdrawMax:    decimal.NewFromInt(100000),
			SingleWithdrawMax:   decimal.NewFromInt(10000),
			PendingSettleMax:    decimal.NewFromInt(10000),
			LargeTradeThreshold: decimal.NewFromInt(100000),
		}
	}
	return &AmountChecker{
		cache:  cache,
		config: config,
	}
}

// Name 返回检查器名称
func (c *AmountChecker) Name() string {
	return "amount_checker"
}

// CheckOrder 检查下单请求
func (c *AmountChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	var warnings []string

	// 检查单笔订单金额
	if req.Notional.GreaterThan(c.config.SingleOrderMax) {
		return NewRejectedResult(
			AmountLimitRuleID,
			AmountLimitRuleName,
			fmt.Sprintf("单笔订单金额超出限制，最大允许 %s", c.config.SingleOrderMax.String()),
			AmountLimitCode,
		)
	}

	// 检查待结算余额
	pendingAmount, err := c.cache.GetPendingSettle(ctx, req.Wallet)
	if err == nil {
		newPending := pendingAmount.Add(req.Notional)
		if newPending.GreaterThan(c.config.PendingSettleMax) {
			return NewRejectedResult(
				AmountLimitRuleID,
				AmountLimitRuleName,
				fmt.Sprintf("待结算余额超出限制，当前 %s，最大允许 %s",
					pendingAmount.String(), c.config.PendingSettleMax.String()),
				AmountLimitCode,
			)
		}
	}

	// 大额交易告警
	if req.Notional.GreaterThanOrEqual(c.config.LargeTradeThreshold) {
		warnings = append(warnings, fmt.Sprintf("大额交易: %s", req.Notional.String()))
	}

	if len(warnings) > 0 {
		return NewPassedWithWarnings(warnings)
	}
	return NewPassedResult()
}

// CheckWithdraw 检查提现请求
func (c *AmountChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	// 检查单笔提现金额
	if req.Amount.GreaterThan(c.config.SingleWithdrawMax) {
		return NewRejectedResult(
			AmountLimitRuleID,
			AmountLimitRuleName,
			fmt.Sprintf("单笔提现金额超出限制，最大允许 %s", c.config.SingleWithdrawMax.String()),
			AmountLimitCode,
		)
	}

	// 检查每日提现额度
	dailyUsed, err := c.cache.GetDailyWithdraw(ctx, req.Wallet, req.Token)
	if err == nil {
		newDaily := dailyUsed.Add(req.Amount)
		if newDaily.GreaterThan(c.config.DailyWithdrawMax) {
			return NewRejectedResult(
				AmountLimitRuleID,
				AmountLimitRuleName,
				fmt.Sprintf("每日提现额度已用完，已使用 %s，最大允许 %s",
					dailyUsed.String(), c.config.DailyWithdrawMax.String()),
				AmountLimitCode,
			)
		}
	}

	// 计算风险分值
	result := NewPassedResult()

	// 根据提现金额计算风险分值
	ratio := req.Amount.Div(c.config.DailyWithdrawMax)
	if ratio.GreaterThan(decimal.NewFromFloat(0.5)) {
		result.RiskScore = 30
	}
	if ratio.GreaterThan(decimal.NewFromFloat(0.8)) {
		result.RiskScore = 60
	}

	return result
}

// GetUserLimits 获取用户限额信息
func (c *AmountChecker) GetUserLimits(ctx context.Context, wallet string) ([]LimitInfo, error) {
	limits := make([]LimitInfo, 0)

	// 每日提现限额
	dailyUsed, err := c.cache.GetDailyWithdraw(ctx, wallet, "")
	if err != nil {
		dailyUsed = decimal.Zero
	}
	limits = append(limits, LimitInfo{
		LimitType:      "daily_withdraw",
		Token:          "",
		MaxValue:       c.config.DailyWithdrawMax,
		UsedValue:      dailyUsed,
		RemainingValue: c.config.DailyWithdrawMax.Sub(dailyUsed),
		ResetAt:        c.cache.GetDailyResetTime(),
	})

	// 单笔订单限额
	limits = append(limits, LimitInfo{
		LimitType:      "single_order",
		Token:          "",
		MaxValue:       c.config.SingleOrderMax,
		UsedValue:      decimal.Zero,
		RemainingValue: c.config.SingleOrderMax,
		ResetAt:        0,
	})

	// 待结算限额
	pendingAmount, err := c.cache.GetPendingSettle(ctx, wallet)
	if err != nil {
		pendingAmount = decimal.Zero
	}
	limits = append(limits, LimitInfo{
		LimitType:      "pending_settle",
		Token:          "",
		MaxValue:       c.config.PendingSettleMax,
		UsedValue:      pendingAmount,
		RemainingValue: c.config.PendingSettleMax.Sub(pendingAmount),
		ResetAt:        0,
	})

	return limits, nil
}
