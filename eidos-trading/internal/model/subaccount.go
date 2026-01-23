// Package model 定义交易服务的数据模型
package model

import (
	"github.com/shopspring/decimal"
)

// SubAccountStatus 子账户状态
type SubAccountStatus int8

const (
	SubAccountStatusActive  SubAccountStatus = 1 // 活跃
	SubAccountStatusFrozen  SubAccountStatus = 2 // 冻结
	SubAccountStatusDeleted SubAccountStatus = 3 // 已删除
)

// String 返回状态的字符串表示
func (s SubAccountStatus) String() string {
	switch s {
	case SubAccountStatusActive:
		return "ACTIVE"
	case SubAccountStatusFrozen:
		return "FROZEN"
	case SubAccountStatusDeleted:
		return "DELETED"
	default:
		return "UNKNOWN"
	}
}

// SubAccountType 子账户类型
type SubAccountType int8

const (
	SubAccountTypeTrading SubAccountType = 1 // 现货交易账户
	SubAccountTypeMargin  SubAccountType = 2 // 保证金账户 (Phase 2)
	SubAccountTypeFutures SubAccountType = 3 // 合约账户 (Phase 2)
)

// String 返回类型的字符串表示
func (t SubAccountType) String() string {
	switch t {
	case SubAccountTypeTrading:
		return "TRADING"
	case SubAccountTypeMargin:
		return "MARGIN"
	case SubAccountTypeFutures:
		return "FUTURES"
	default:
		return "UNKNOWN"
	}
}

// SubAccount 子账户
// 一个主钱包可以创建多个独立的子账户
// 对应数据库表 trading_subaccounts
type SubAccount struct {
	ID           int64            `gorm:"primaryKey;autoIncrement" json:"id"`
	SubAccountID string           `gorm:"type:varchar(64);uniqueIndex;not null" json:"sub_account_id"` // 子账户 ID (格式: wallet前缀 + 序号)
	Wallet       string           `gorm:"type:varchar(42);index;not null" json:"wallet"`               // 主钱包地址
	Name         string           `gorm:"type:varchar(50);not null" json:"name"`                       // 子账户名称
	Type         SubAccountType   `gorm:"type:smallint;not null;default:1" json:"type"`                // 账户类型
	Status       SubAccountStatus `gorm:"type:smallint;not null;default:1" json:"status"`              // 状态
	IsDefault    bool             `gorm:"type:boolean;not null;default:false" json:"is_default"`       // 是否为默认子账户
	Remark       string           `gorm:"type:varchar(255)" json:"remark"`                             // 备注
	CreatedAt    int64            `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt    int64            `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (SubAccount) TableName() string {
	return "trading_subaccounts"
}

// CanTrade 检查子账户是否可以交易
func (s *SubAccount) CanTrade() bool {
	return s.Status == SubAccountStatusActive
}

// CanTransfer 检查子账户是否可以划转
func (s *SubAccount) CanTransfer() bool {
	return s.Status == SubAccountStatusActive
}

// SubAccountBalance 子账户余额
// 每个子账户有独立的余额，与主账户余额分开管理
// 对应数据库表 trading_subaccount_balances
type SubAccountBalance struct {
	ID               int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	SubAccountID     string          `gorm:"type:varchar(64);uniqueIndex:uk_subaccount_token;not null" json:"sub_account_id"` // 子账户 ID
	Wallet           string          `gorm:"type:varchar(42);index;not null" json:"wallet"`                                   // 主钱包地址 (冗余，方便查询)
	Token            string          `gorm:"type:varchar(20);uniqueIndex:uk_subaccount_token;not null" json:"token"`
	Available        decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"available"` // 可用余额
	Frozen           decimal.Decimal `gorm:"type:decimal(36,18);not null;default:0" json:"frozen"`    // 冻结余额 (挂单占用)
	Version          int64           `gorm:"type:bigint;not null;default:1" json:"version"`           // 乐观锁版本号
	CreatedAt        int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt        int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (SubAccountBalance) TableName() string {
	return "trading_subaccount_balances"
}

// Total 返回总余额
func (b *SubAccountBalance) Total() decimal.Decimal {
	return b.Available.Add(b.Frozen)
}

// CanFreeze 检查是否有足够余额可冻结
func (b *SubAccountBalance) CanFreeze(amount decimal.Decimal) bool {
	return b.Available.GreaterThanOrEqual(amount)
}

// SubAccountTransferType 子账户划转类型
type SubAccountTransferType int8

const (
	SubAccountTransferTypeIn  SubAccountTransferType = 1 // 划入 (主账户 → 子账户)
	SubAccountTransferTypeOut SubAccountTransferType = 2 // 划出 (子账户 → 主账户)
)

// String 返回类型的字符串表示
func (t SubAccountTransferType) String() string {
	switch t {
	case SubAccountTransferTypeIn:
		return "TRANSFER_IN"
	case SubAccountTransferTypeOut:
		return "TRANSFER_OUT"
	default:
		return "UNKNOWN"
	}
}

// SubAccountTransfer 子账户划转记录
// 对应数据库表 trading_subaccount_transfers
type SubAccountTransfer struct {
	ID           int64                  `gorm:"primaryKey;autoIncrement" json:"id"`
	TransferID   string                 `gorm:"type:varchar(64);uniqueIndex;not null" json:"transfer_id"`   // 划转 ID
	Wallet       string                 `gorm:"type:varchar(42);index;not null" json:"wallet"`              // 主钱包地址
	SubAccountID string                 `gorm:"type:varchar(64);index;not null" json:"sub_account_id"`      // 子账户 ID
	Type         SubAccountTransferType `gorm:"type:smallint;not null" json:"type"`                         // 划转类型
	Token        string                 `gorm:"type:varchar(20);not null" json:"token"`                     // 代币
	Amount       decimal.Decimal        `gorm:"type:decimal(36,18);not null" json:"amount"`                 // 金额
	Remark       string                 `gorm:"type:varchar(255)" json:"remark"`                            // 备注
	CreatedAt    int64                  `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
}

// TableName 返回表名
func (SubAccountTransfer) TableName() string {
	return "trading_subaccount_transfers"
}

// SubAccountBalanceLog 子账户余额流水
// 对应数据库表 trading_subaccount_balance_logs
type SubAccountBalanceLog struct {
	ID            int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	SubAccountID  string          `gorm:"type:varchar(64);index;not null" json:"sub_account_id"`
	Wallet        string          `gorm:"type:varchar(42);index;not null" json:"wallet"`
	Token         string          `gorm:"type:varchar(20);not null" json:"token"`
	Type          BalanceLogType  `gorm:"type:smallint;index;not null" json:"type"`                                // 流水类型 (复用 BalanceLogType)
	Amount        decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"amount"`                              // 变动金额
	BalanceBefore decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"balance_before"`                      // 变动前余额
	BalanceAfter  decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"balance_after"`                       // 变动后余额
	OrderID       string          `gorm:"type:varchar(64);index" json:"order_id"`                                  // 关联订单 ID
	TradeID       string          `gorm:"type:varchar(64);uniqueIndex:uk_trade_subaccount_token" json:"trade_id"`  // 关联成交 ID (幂等键)
	TransferID    string          `gorm:"type:varchar(64);index" json:"transfer_id"`                               // 关联划转 ID
	Remark        string          `gorm:"type:varchar(255)" json:"remark"`
	CreatedAt     int64           `gorm:"type:bigint;not null;autoCreateTime:milli;index" json:"created_at"`
}

// TableName 返回表名
func (SubAccountBalanceLog) TableName() string {
	return "trading_subaccount_balance_logs"
}

// 子账户限制常量
const (
	MaxSubAccountsPerWallet = 20 // 每个钱包最多创建的子账户数量
	SubAccountNameMinLen    = 2  // 子账户名称最小长度
	SubAccountNameMaxLen    = 50 // 子账户名称最大长度
)
