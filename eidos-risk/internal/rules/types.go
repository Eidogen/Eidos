// Package rules 定义风控规则引擎
package rules

import (
	"context"

	"github.com/shopspring/decimal"
)

// CheckResult 检查结果
type CheckResult struct {
	Passed    bool     // 是否通过
	RuleID    string   // 触发的规则ID
	RuleName  string   // 触发的规则名称
	Reason    string   // 拒绝原因
	Code      string   // 错误码
	Warnings  []string // 警告信息
	RiskScore int      // 风险分值 (0-100)
}

// NewPassedResult 创建通过结果
func NewPassedResult() *CheckResult {
	return &CheckResult{Passed: true}
}

// NewPassedWithWarnings 创建带警告的通过结果
func NewPassedWithWarnings(warnings []string) *CheckResult {
	return &CheckResult{
		Passed:   true,
		Warnings: warnings,
	}
}

// NewRejectedResult 创建拒绝结果
func NewRejectedResult(ruleID, ruleName, reason, code string) *CheckResult {
	return &CheckResult{
		Passed:   false,
		RuleID:   ruleID,
		RuleName: ruleName,
		Reason:   reason,
		Code:     code,
	}
}

// NewPassResult 创建通过结果 (带检查器名称)
func NewPassResult(checkerName string) *CheckResult {
	return &CheckResult{
		Passed:   true,
		RuleName: checkerName,
	}
}

// NewRejectResult 创建拒绝结果 (简化版)
func NewRejectResult(checkerName, reason, code string) *CheckResult {
	return &CheckResult{
		Passed:   false,
		RuleName: checkerName,
		Reason:   reason,
		Code:     code,
	}
}

// NewWarningResult 创建警告结果
func NewWarningResult(checkerName, warning string) *CheckResult {
	return &CheckResult{
		Passed:   true,
		RuleName: checkerName,
		Warnings: []string{warning},
	}
}

// Merge 合并两个检查结果
func (r *CheckResult) Merge(other *CheckResult) {
	if other == nil {
		return
	}
	// 如果另一个结果被拒绝，则合并结果也被拒绝
	if !other.Passed {
		r.Passed = false
		r.RuleID = other.RuleID
		r.RuleName = other.RuleName
		r.Reason = other.Reason
		r.Code = other.Code
	}
	// 合并警告
	r.Warnings = append(r.Warnings, other.Warnings...)
	// 取最高风险分
	if other.RiskScore > r.RiskScore {
		r.RiskScore = other.RiskScore
	}
}

// OrderCheckRequest 下单检查请求
type OrderCheckRequest struct {
	Wallet    string
	Market    string
	Side      string // BUY/SELL
	OrderType string // LIMIT/MARKET
	Price     decimal.Decimal
	Amount    decimal.Decimal
	Notional  decimal.Decimal // 名义价值 = price * amount
}

// WithdrawCheckRequest 提现检查请求
type WithdrawCheckRequest struct {
	Wallet    string
	Token     string
	Amount    decimal.Decimal
	ToAddress string
}

// UserLimits 用户限额信息
type UserLimits struct {
	Wallet string
	Limits []LimitInfo
}

// LimitInfo 限额信息
type LimitInfo struct {
	LimitType      string          // daily_withdraw, single_order, pending_settle
	Token          string          // 适用代币，空表示所有
	MaxValue       decimal.Decimal // 最大限额
	UsedValue      decimal.Decimal // 已使用
	RemainingValue decimal.Decimal // 剩余
	ResetAt        int64           // 重置时间
}

// RuleChecker 规则检查器接口
type RuleChecker interface {
	// Name 返回检查器名称
	Name() string
	// CheckOrder 检查下单请求
	CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult
	// CheckWithdraw 检查提现请求
	CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult
}

// RulePriority 规则优先级
type RulePriority int

const (
	PriorityHighest  RulePriority = 1   // 最高优先级 (黑名单)
	PriorityHigh     RulePriority = 10  // 高优先级 (频率限制)
	PriorityNormal   RulePriority = 50  // 普通优先级 (金额限制)
	PriorityLow      RulePriority = 100 // 低优先级 (警告类)
)
