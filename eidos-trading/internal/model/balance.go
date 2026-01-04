package model

import (
	"github.com/shopspring/decimal"
)

// Balance 用户余额
// 采用四字段模型: settled/pending × available/frozen
// 对应数据库表 balances
type Balance struct {
	ID               int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	Wallet           string          `gorm:"type:varchar(42);uniqueIndex:uk_wallet_token;not null" json:"wallet"`
	Token            string          `gorm:"type:varchar(20);uniqueIndex:uk_wallet_token;not null" json:"token"`
	SettledAvailable decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"settled_available"` // 已结算可用 (可提现)
	SettledFrozen    decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"settled_frozen"`    // 已结算冻结
	PendingAvailable decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"pending_available"` // 待结算可用
	PendingFrozen    decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"pending_frozen"`    // 待结算冻结
	PendingTotal     decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"pending_total"`     // 待结算总额 (用于风控限额)
	Version          int64           `gorm:"type:bigint;not null;default:1" json:"version"`                   // 乐观锁版本号
	CreatedAt        int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt        int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (Balance) TableName() string {
	return "balances"
}

// TotalAvailable 返回总可用余额 (交易可用)
// = 已结算可用 + 待结算可用
func (b *Balance) TotalAvailable() decimal.Decimal {
	return b.SettledAvailable.Add(b.PendingAvailable)
}

// TotalFrozen 返回总冻结余额
// = 已结算冻结 + 待结算冻结
func (b *Balance) TotalFrozen() decimal.Decimal {
	return b.SettledFrozen.Add(b.PendingFrozen)
}

// Total 返回总余额
// = 已结算 (可用+冻结) + 待结算 (可用+冻结)
func (b *Balance) Total() decimal.Decimal {
	return b.SettledAvailable.Add(b.SettledFrozen).Add(b.PendingAvailable).Add(b.PendingFrozen)
}

// WithdrawableAmount 返回可提现余额
// 只有已结算部分可以提现
func (b *Balance) WithdrawableAmount() decimal.Decimal {
	return b.SettledAvailable
}

// CanFreeze 检查是否有足够余额可冻结
func (b *Balance) CanFreeze(amount decimal.Decimal) bool {
	return b.TotalAvailable().GreaterThanOrEqual(amount)
}

// BalanceLogType 余额流水类型
type BalanceLogType int8

const (
	BalanceLogTypeDeposit        BalanceLogType = 1 // 充值
	BalanceLogTypeWithdraw       BalanceLogType = 2 // 提现
	BalanceLogTypeFreeze         BalanceLogType = 3 // 冻结 (下单)
	BalanceLogTypeUnfreeze       BalanceLogType = 4 // 解冻 (取消订单)
	BalanceLogTypeTrade          BalanceLogType = 5 // 成交划转
	BalanceLogTypeFee            BalanceLogType = 6 // 手续费
	BalanceLogTypeSettlement     BalanceLogType = 7 // 结算确认 (pending → settled)
	BalanceLogTypeRollback       BalanceLogType = 8 // 回滚
	BalanceLogTypeWithdrawRefund BalanceLogType = 9 // 提现失败退回
)

func (t BalanceLogType) String() string {
	switch t {
	case BalanceLogTypeDeposit:
		return "DEPOSIT"
	case BalanceLogTypeWithdraw:
		return "WITHDRAW"
	case BalanceLogTypeFreeze:
		return "FREEZE"
	case BalanceLogTypeUnfreeze:
		return "UNFREEZE"
	case BalanceLogTypeTrade:
		return "TRADE"
	case BalanceLogTypeFee:
		return "FEE"
	case BalanceLogTypeSettlement:
		return "SETTLEMENT"
	case BalanceLogTypeRollback:
		return "ROLLBACK"
	case BalanceLogTypeWithdrawRefund:
		return "WITHDRAW_REFUND"
	default:
		return "UNKNOWN"
	}
}

// BalanceLog 余额流水
// 对应数据库表 balance_logs
type BalanceLog struct {
	ID            int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	Wallet        string          `gorm:"type:varchar(42);index;not null" json:"wallet"`
	Token         string          `gorm:"type:varchar(20);not null" json:"token"`
	Type          BalanceLogType  `gorm:"type:smallint;index;not null" json:"type"`                           // 流水类型
	Amount        decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"amount"`                         // 变动金额 (正数加，负数减)
	BalanceBefore decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"balance_before"`                 // 变动前余额
	BalanceAfter  decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"balance_after"`                  // 变动后余额
	OrderID       string          `gorm:"type:varchar(64);index" json:"order_id"`                             // 关联订单 ID
	TradeID       string          `gorm:"type:varchar(64);uniqueIndex:uk_trade_wallet_token" json:"trade_id"` // 关联成交 ID (幂等键)
	TxHash        string          `gorm:"type:varchar(66)" json:"tx_hash"`                                    // 链上交易哈希
	Remark        string          `gorm:"type:varchar(255)" json:"remark"`                                    // 备注
	CreatedAt     int64           `gorm:"type:bigint;not null;autoCreateTime:milli;index" json:"created_at"`
}

// TableName 返回表名
func (BalanceLog) TableName() string {
	return "balance_logs"
}

// FeeAccount 手续费账户 (分桶)
// 对应数据库表 fee_accounts
type FeeAccount struct {
	ID        int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	BucketID  int             `gorm:"type:int;uniqueIndex:uk_bucket_token;not null" json:"bucket_id"` // 分桶 ID (0-15)
	Token     string          `gorm:"type:varchar(20);uniqueIndex:uk_bucket_token;not null" json:"token"`
	Balance   decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"balance"`
	Version   int64           `gorm:"type:bigint;not null;default:1" json:"version"` // 乐观锁
	UpdatedAt int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (FeeAccount) TableName() string {
	return "fee_accounts"
}
