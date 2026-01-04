package app

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
)

// ========== 市场配置提供者 ==========

type marketConfigProvider struct {
	cfg *config.Config
}

// NewMarketConfigProvider 创建市场配置提供者
func NewMarketConfigProvider(cfg *config.Config) service.MarketConfigProvider {
	return &marketConfigProvider{cfg: cfg}
}

// GetMarket 获取市场配置
func (p *marketConfigProvider) GetMarket(market string) (*service.MarketConfig, error) {
	for _, m := range p.cfg.Markets {
		if m.Market == market {
			return &service.MarketConfig{
				Market:          m.Market,
				BaseToken:       m.BaseToken,
				QuoteToken:      m.QuoteToken,
				MinAmount:       m.MinAmount,
				MaxAmount:       m.MaxAmount,
				MinPrice:        m.MinPrice,
				MaxPrice:        m.MaxPrice,
				PricePrecision:  m.PricePrecision,
				AmountPrecision: m.AmountPrecision,
				MakerFeeRate:    m.MakerFeeRate,
				TakerFeeRate:    m.TakerFeeRate,
				Status:          m.Status,
			}, nil
		}
	}
	return nil, fmt.Errorf("market %s not found", market)
}

// IsValidMarket 检查市场是否有效
func (p *marketConfigProvider) IsValidMarket(market string) bool {
	for _, m := range p.cfg.Markets {
		if m.Market == market && m.Status == 1 {
			return true
		}
	}
	return false
}

// ========== 代币配置提供者 ==========

type tokenConfigProvider struct {
	cfg *config.Config
}

// NewTokenConfigProvider 创建代币配置提供者
func NewTokenConfigProvider(cfg *config.Config) service.TokenConfigProvider {
	return &tokenConfigProvider{cfg: cfg}
}

// IsValidToken 检查代币是否有效
func (p *tokenConfigProvider) IsValidToken(token string) bool {
	for _, t := range p.cfg.Tokens {
		if t.Symbol == token {
			return true
		}
	}
	return false
}

// GetTokenDecimals 获取代币精度
func (p *tokenConfigProvider) GetTokenDecimals(token string) int32 {
	for _, t := range p.cfg.Tokens {
		if t.Symbol == token {
			return t.Decimals
		}
	}
	return 18 // 默认精度
}

// GetSupportedTokens 获取支持的代币列表
func (p *tokenConfigProvider) GetSupportedTokens() []string {
	tokens := make([]string, 0, len(p.cfg.Tokens))
	for _, t := range p.cfg.Tokens {
		tokens = append(tokens, t.Symbol)
	}
	return tokens
}

// ========== 风控配置提供者 ==========

type riskConfigProvider struct {
	cfg *config.Config
}

// NewRiskConfigProvider 创建风控配置提供者
func NewRiskConfigProvider(cfg *config.Config) service.RiskConfigProvider {
	return &riskConfigProvider{cfg: cfg}
}

// GetUserPendingLimit 获取用户待结算限额 (USDT)
func (p *riskConfigProvider) GetUserPendingLimit() decimal.Decimal {
	return p.cfg.RiskControl.UserPendingLimit
}

// GetGlobalPendingLimit 获取全局待结算限额 (USDT)
func (p *riskConfigProvider) GetGlobalPendingLimit() decimal.Decimal {
	return p.cfg.RiskControl.GlobalPendingLimit
}

// GetMaxOpenOrdersPerUser 获取用户最大活跃订单数
func (p *riskConfigProvider) GetMaxOpenOrdersPerUser() int {
	return p.cfg.RiskControl.MaxOpenOrdersPerUser
}
