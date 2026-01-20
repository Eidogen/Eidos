package model

// DailyStats 日统计数据
type DailyStats struct {
	ID            int64   `gorm:"primaryKey;column:id" json:"id"`
	StatDate      string  `gorm:"column:stat_date;size:10;index" json:"stat_date"`
	Market        *string `gorm:"column:market;size:20;index" json:"market,omitempty"`
	TradeCount    int64   `gorm:"column:trade_count;default:0" json:"trade_count"`
	TradeVolume   string  `gorm:"column:trade_volume;size:36;default:0" json:"trade_volume"`
	TradeAmount   string  `gorm:"column:trade_amount;size:36;default:0" json:"trade_amount"`
	OrderCount    int64   `gorm:"column:order_count;default:0" json:"order_count"`
	CancelledCnt  int64   `gorm:"column:cancelled_count;default:0" json:"cancelled_count"`
	MakerFeeTotal string  `gorm:"column:maker_fee_total;size:36;default:0" json:"maker_fee_total"`
	TakerFeeTotal string  `gorm:"column:taker_fee_total;size:36;default:0" json:"taker_fee_total"`
	ActiveUsers   int64   `gorm:"column:active_users;default:0" json:"active_users"`
	NewUsers      int64   `gorm:"column:new_users;default:0" json:"new_users"`
	CreatedAt     int64   `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt     int64   `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (DailyStats) TableName() string {
	return "admin_daily_stats"
}

// OverviewStats 总览统计
type OverviewStats struct {
	TotalUsers        int64  `json:"total_users"`
	TodayActiveUsers  int64  `json:"today_active_users"`
	TotalOrders       int64  `json:"total_orders"`
	TodayOrders       int64  `json:"today_orders"`
	TotalTrades       int64  `json:"total_trades"`
	TodayTrades       int64  `json:"today_trades"`
	TotalVolume       string `json:"total_volume"`
	TodayVolume       string `json:"today_volume"`
	TotalFees         string `json:"total_fees"`
	TodayFees         string `json:"today_fees"`
	OpenOrdersCount   int64  `json:"open_orders_count"`
	PendingWithdraws  int64  `json:"pending_withdraws"`
	SystemHealthy     bool   `json:"system_healthy"`
	LastUpdatedAt     int64  `json:"last_updated_at"`
}
