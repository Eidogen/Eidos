package model

// MarketStatus 交易对状态
type MarketStatus int

const (
	MarketStatusActive    MarketStatus = 1 // 活跃
	MarketStatusSuspended MarketStatus = 2 // 暂停交易
	MarketStatusOffline   MarketStatus = 3 // 下线
)

// MarketConfig 交易对配置
type MarketConfig struct {
	ID              int64        `gorm:"primaryKey;column:id" json:"id"`
	Symbol          string       `gorm:"column:symbol;size:20;uniqueIndex" json:"symbol"`
	BaseToken       string       `gorm:"column:base_token;size:20" json:"base_token"`
	QuoteToken      string       `gorm:"column:quote_token;size:20" json:"quote_token"`
	BaseTokenAddr   string       `gorm:"column:base_token_addr;size:42" json:"base_token_addr"`
	QuoteTokenAddr  string       `gorm:"column:quote_token_addr;size:42" json:"quote_token_addr"`
	PriceDecimals   int          `gorm:"column:price_decimals" json:"price_decimals"`
	SizeDecimals    int          `gorm:"column:size_decimals" json:"size_decimals"`
	MinSize         string       `gorm:"column:min_size;size:36" json:"min_size"`
	MaxSize         string       `gorm:"column:max_size;size:36" json:"max_size"`
	MinNotional     string       `gorm:"column:min_notional;size:36" json:"min_notional"`
	TickSize        string       `gorm:"column:tick_size;size:36" json:"tick_size"`
	MakerFee        string       `gorm:"column:maker_fee;size:36" json:"maker_fee"`
	TakerFee        string       `gorm:"column:taker_fee;size:36" json:"taker_fee"`
	Status          MarketStatus `gorm:"column:status;default:1" json:"status"`
	TradingEnabled  bool         `gorm:"column:trading_enabled;default:false" json:"trading_enabled"`
	DisplayOrder    int          `gorm:"column:display_order;default:0" json:"display_order"`
	Description     string       `gorm:"column:description;size:500" json:"description"`
	CreatedBy       int64        `gorm:"column:created_by" json:"created_by"`
	CreatedAt       int64        `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedBy       int64        `gorm:"column:updated_by" json:"updated_by"`
	UpdatedAt       int64        `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (MarketConfig) TableName() string {
	return "eidos_admin_market_configs"
}
