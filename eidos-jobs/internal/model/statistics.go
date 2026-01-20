package model

// StatType 统计类型
type StatType string

const (
	StatTypeHourly StatType = "hourly"
	StatTypeDaily  StatType = "daily"
)

// Statistics 统计数据
type Statistics struct {
	ID          int64   `gorm:"column:id;primaryKey;autoIncrement"`
	StatType    string  `gorm:"column:stat_type;type:varchar(50);not null"`
	StatDate    string  `gorm:"column:stat_date;type:date;not null"`
	StatHour    *int    `gorm:"column:stat_hour"` // 0-23, 仅 hourly 类型
	Market      *string `gorm:"column:market;type:varchar(32)"`
	MetricName  string  `gorm:"column:metric_name;type:varchar(100);not null"`
	MetricValue string  `gorm:"column:metric_value;type:decimal(36,18);not null"`
	CreatedAt   int64   `gorm:"column:created_at;not null"`
}

// TableName 表名
func (Statistics) TableName() string {
	return "jobs_statistics"
}

// MetricNames 指标名称常量
const (
	MetricTradeVolume   = "trade_volume"    // 交易量
	MetricTradeCount    = "trade_count"     // 成交笔数
	MetricFeeTotal      = "fee_total"       // 手续费收入
	MetricActiveUsers   = "active_users"    // 活跃用户数
	MetricNewUsers      = "new_users"       // 新增用户数
	MetricOrderCount    = "order_count"     // 订单数量
	MetricCancelCount   = "cancel_count"    // 取消数量
	MetricDepositVolume = "deposit_volume"  // 充值量
	MetricWithdrawVolume = "withdraw_volume" // 提现量
)
