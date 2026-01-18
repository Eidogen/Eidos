package rules

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/shopspring/decimal"
)

const (
	WithdrawRuleID   = "WITHDRAW_CHECK"
	WithdrawRuleName = "提现安全检查"
	WithdrawCode     = "WITHDRAW_REJECTED"
)

var (
	// 以太坊地址正则
	ethAddressRegex = regexp.MustCompile("^0x[a-fA-F0-9]{40}$")
)

// WithdrawConfig 提现检查配置
type WithdrawConfig struct {
	MinWithdrawAmount    decimal.Decimal // 最小提现金额
	RequireWhitelistAddr bool            // 是否需要白名单地址
	HighRiskThreshold    decimal.Decimal // 高风险阈值（需人工审核）
}

// WithdrawChecker 提现安全检查器
type WithdrawChecker struct {
	cache  *cache.WithdrawCache
	config *WithdrawConfig
}

// NewWithdrawChecker 创建提现检查器
func NewWithdrawChecker(cache *cache.WithdrawCache, config *WithdrawConfig) *WithdrawChecker {
	if config == nil {
		config = &WithdrawConfig{
			MinWithdrawAmount:    decimal.NewFromFloat(0.001),
			RequireWhitelistAddr: false,
			HighRiskThreshold:    decimal.NewFromInt(10000),
		}
	}
	return &WithdrawChecker{
		cache:  cache,
		config: config,
	}
}

// Name 返回检查器名称
func (c *WithdrawChecker) Name() string {
	return "withdraw_checker"
}

// CheckOrder 检查下单请求 (不适用)
func (c *WithdrawChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	return NewPassedResult()
}

// CheckWithdraw 检查提现请求
func (c *WithdrawChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	var warnings []string
	riskScore := 0

	// 1. 验证地址格式
	if !isValidAddress(req.ToAddress) {
		return NewRejectedResult(
			WithdrawRuleID,
			WithdrawRuleName,
			"目标地址格式无效",
			WithdrawCode,
		)
	}

	// 2. 检查最小提现金额
	if req.Amount.LessThan(c.config.MinWithdrawAmount) {
		return NewRejectedResult(
			WithdrawRuleID,
			WithdrawRuleName,
			fmt.Sprintf("提现金额低于最小限制 %s", c.config.MinWithdrawAmount.String()),
			WithdrawCode,
		)
	}

	// 3. 检查是否为新地址
	isNewAddr, err := c.cache.IsNewAddress(ctx, req.Wallet, req.ToAddress)
	if err == nil && isNewAddr {
		warnings = append(warnings, "首次提现到新地址")
		riskScore += 20
	}

	// 4. 检查是否为合约地址
	isContract, err := c.cache.IsContractAddress(ctx, req.ToAddress)
	if err == nil && isContract {
		warnings = append(warnings, "目标地址为合约地址")
		riskScore += 10
	}

	// 5. 检查是否白名单地址
	if c.config.RequireWhitelistAddr {
		inWhitelist, err := c.cache.IsWhitelistedAddress(ctx, req.Wallet, req.ToAddress)
		if err != nil || !inWhitelist {
			return NewRejectedResult(
				WithdrawRuleID,
				WithdrawRuleName,
				"目标地址不在白名单中",
				WithdrawCode,
			)
		}
	}

	// 6. 大额提现需人工审核
	if req.Amount.GreaterThanOrEqual(c.config.HighRiskThreshold) {
		warnings = append(warnings, "大额提现，建议人工审核")
		riskScore += 30
	}

	// 7. 检查提现频率异常
	recentCount, err := c.cache.GetRecentWithdrawCount(ctx, req.Wallet, 3600) // 1小时内
	if err == nil && recentCount >= 3 {
		warnings = append(warnings, "短时间内多次提现")
		riskScore += 20
	}

	result := NewPassedResult()
	if len(warnings) > 0 {
		result.Warnings = warnings
	}
	result.RiskScore = riskScore

	return result
}

// isValidAddress 验证地址格式
func isValidAddress(addr string) bool {
	if addr == "" {
		return false
	}

	// 检查以太坊地址格式
	addr = strings.TrimSpace(addr)
	return ethAddressRegex.MatchString(addr)
}
