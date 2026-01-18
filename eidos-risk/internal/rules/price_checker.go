package rules

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/shopspring/decimal"
)

const (
	PriceDeviationRuleID   = "PRICE_DEVIATION_CHECK"
	PriceDeviationRuleName = "价格偏离检查"
	PriceDeviationCode     = "PRICE_DEVIATION"
)

// PriceDeviationConfig 价格偏离配置
type PriceDeviationConfig struct {
	MaxDeviationPercent decimal.Decimal // 最大价格偏离百分比 (如 0.1 = 10%)
	WarningPercent      decimal.Decimal // 警告阈值百分比 (如 0.05 = 5%)
}

// PriceChecker 价格偏离检查器
type PriceChecker struct {
	cache  *cache.MarketCache
	config *PriceDeviationConfig
}

// NewPriceChecker 创建价格检查器
func NewPriceChecker(cache *cache.MarketCache, config *PriceDeviationConfig) *PriceChecker {
	if config == nil {
		config = &PriceDeviationConfig{
			MaxDeviationPercent: decimal.NewFromFloat(0.1),  // 10%
			WarningPercent:      decimal.NewFromFloat(0.05), // 5%
		}
	}
	return &PriceChecker{
		cache:  cache,
		config: config,
	}
}

// Name 返回检查器名称
func (c *PriceChecker) Name() string {
	return "price_checker"
}

// CheckOrder 检查下单请求
func (c *PriceChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	// 市价单跳过价格检查
	if req.OrderType == "MARKET" {
		return NewPassedResult()
	}

	// 获取当前市场价格
	lastPrice, err := c.cache.GetLastPrice(ctx, req.Market)
	if err != nil || lastPrice.IsZero() {
		// 无法获取市场价格（可能是新市场），放行
		return NewPassedResult()
	}

	// 计算价格偏离
	deviation := req.Price.Sub(lastPrice).Abs().Div(lastPrice)

	// 检查是否超过最大偏离
	if deviation.GreaterThan(c.config.MaxDeviationPercent) {
		deviationPercent := deviation.Mul(decimal.NewFromInt(100)).StringFixed(2)
		maxPercent := c.config.MaxDeviationPercent.Mul(decimal.NewFromInt(100)).StringFixed(2)

		return NewRejectedResult(
			PriceDeviationRuleID,
			PriceDeviationRuleName,
			fmt.Sprintf("订单价格偏离市场价格 %s%%，超出允许范围 %s%%",
				deviationPercent, maxPercent),
			PriceDeviationCode,
		)
	}

	// 检查是否触发警告
	if deviation.GreaterThan(c.config.WarningPercent) {
		deviationPercent := deviation.Mul(decimal.NewFromInt(100)).StringFixed(2)
		return NewPassedWithWarnings([]string{
			fmt.Sprintf("订单价格偏离市场价格 %s%%", deviationPercent),
		})
	}

	return NewPassedResult()
}

// CheckWithdraw 检查提现请求 (不适用)
func (c *PriceChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	return NewPassedResult()
}
