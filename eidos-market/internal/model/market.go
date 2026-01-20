package model

import (
	"github.com/shopspring/decimal"
)

// MarketStatus 交易对状态
type MarketStatus int8

const (
	MarketStatusInactive MarketStatus = 0 // 未激活
	MarketStatusActive   MarketStatus = 1 // 已激活
	MarketStatusSuspend  MarketStatus = 2 // 暂停交易
)

func (s MarketStatus) String() string {
	switch s {
	case MarketStatusInactive:
		return "INACTIVE"
	case MarketStatusActive:
		return "ACTIVE"
	case MarketStatusSuspend:
		return "SUSPEND"
	default:
		return "UNKNOWN"
	}
}

// Market 交易对配置
type Market struct {
	ID             int64           `json:"id" gorm:"primaryKey;autoIncrement"`
	Symbol         string          `json:"symbol" gorm:"type:varchar(20);uniqueIndex;not null"`          // 交易对符号，如 BTC-USDC
	BaseToken      string          `json:"base_token" gorm:"type:varchar(10);not null"`                  // 基础代币
	QuoteToken     string          `json:"quote_token" gorm:"type:varchar(10);not null"`                 // 计价代币
	PriceDecimals  int8            `json:"price_decimals" gorm:"type:smallint;not null;default:2"`       // 价格小数位
	SizeDecimals   int8            `json:"size_decimals" gorm:"type:smallint;not null;default:6"`        // 数量小数位
	MinSize        decimal.Decimal `json:"min_size" gorm:"type:decimal(36,18);not null"`                 // 最小下单数量
	MaxSize        decimal.Decimal `json:"max_size" gorm:"type:decimal(36,18)"`                          // 最大下单数量
	MinNotional    decimal.Decimal `json:"min_notional" gorm:"type:decimal(36,18);not null"`             // 最小下单金额
	TickSize       decimal.Decimal `json:"tick_size" gorm:"type:decimal(36,18);not null"`                // 价格最小变动单位
	MakerFee       decimal.Decimal `json:"maker_fee" gorm:"type:decimal(10,6);not null;default:0.001"`   // Maker 费率
	TakerFee       decimal.Decimal `json:"taker_fee" gorm:"type:decimal(10,6);not null;default:0.001"`   // Taker 费率
	Status         MarketStatus    `json:"status" gorm:"type:smallint;index;not null;default:1"`         // 状态
	TradingEnabled bool            `json:"trading_enabled" gorm:"type:boolean;not null;default:true"`    // 是否启用交易
	CreatedBy      string          `json:"created_by" gorm:"type:varchar(42)"`                           // 创建者钱包
	CreatedAt      int64           `json:"created_at" gorm:"type:bigint;not null;autoCreateTime:milli"`  // 创建时间
	UpdatedBy      string          `json:"updated_by" gorm:"type:varchar(42)"`                           // 更新者钱包
	UpdatedAt      int64           `json:"updated_at" gorm:"type:bigint;not null;autoUpdateTime:milli"`  // 更新时间
}

// TableName GORM表名
func (Market) TableName() string {
	return "market_markets"
}

// IsActive 检查交易对是否激活
func (m *Market) IsActive() bool {
	return m.Status == MarketStatusActive
}

// IsTradingEnabled 检查是否可交易
func (m *Market) IsTradingEnabled() bool {
	return m.Status == MarketStatusActive && m.TradingEnabled
}

// Clone 克隆交易对配置
func (m *Market) Clone() *Market {
	return &Market{
		ID:             m.ID,
		Symbol:         m.Symbol,
		BaseToken:      m.BaseToken,
		QuoteToken:     m.QuoteToken,
		PriceDecimals:  m.PriceDecimals,
		SizeDecimals:   m.SizeDecimals,
		MinSize:        m.MinSize,
		MaxSize:        m.MaxSize,
		MinNotional:    m.MinNotional,
		TickSize:       m.TickSize,
		MakerFee:       m.MakerFee,
		TakerFee:       m.TakerFee,
		Status:         m.Status,
		TradingEnabled: m.TradingEnabled,
		CreatedBy:      m.CreatedBy,
		CreatedAt:      m.CreatedAt,
		UpdatedBy:      m.UpdatedBy,
		UpdatedAt:      m.UpdatedAt,
	}
}
