package model

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// DepositStatus 充值状态
type DepositStatus int8

const (
	DepositStatusPending   DepositStatus = 0 // 待确认 (检测到链上事件)
	DepositStatusConfirmed DepositStatus = 1 // 已确认 (达到确认区块数)
	DepositStatusCredited  DepositStatus = 2 // 已入账 (余额已增加)
)

func (s DepositStatus) String() string {
	switch s {
	case DepositStatusPending:
		return "PENDING"
	case DepositStatusConfirmed:
		return "CONFIRMED"
	case DepositStatusCredited:
		return "CREDITED"
	default:
		return "UNKNOWN"
	}
}

// Deposit 充值记录
// 对应数据库表 deposits
// 幂等键: tx_hash + log_index
type Deposit struct {
	ID          int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	DepositID   string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"deposit_id"`        // 充值 ID
	Wallet      string          `gorm:"type:varchar(42);index;not null" json:"wallet"`                  // 用户钱包
	Token       string          `gorm:"type:varchar(20);not null" json:"token"`                         // 代币
	Amount      decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"amount"`                     // 充值金额
	TxHash      string          `gorm:"type:varchar(66);uniqueIndex:uk_tx_log;not null" json:"tx_hash"` // 链上交易哈希
	LogIndex    uint32          `gorm:"type:int;uniqueIndex:uk_tx_log;not null" json:"log_index"`       // 事件日志索引
	BlockNum    int64           `gorm:"type:bigint;not null" json:"block_num"`                          // 区块高度
	Status      DepositStatus   `gorm:"type:smallint;index;not null;default:0" json:"status"`           // 状态
	DetectedAt  int64           `gorm:"type:bigint;not null" json:"detected_at"`                        // 检测时间
	ConfirmedAt int64           `gorm:"type:bigint" json:"confirmed_at"`                                // 确认时间
	CreditedAt  int64           `gorm:"type:bigint" json:"credited_at"`                                 // 入账时间
	CreatedAt   int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt   int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (Deposit) TableName() string {
	return "deposits"
}

// GetIdempotencyKey 获取幂等键
// 格式: tx_hash:log_index (与数据库唯一索引 uk_tx_log 对应)
func (d *Deposit) GetIdempotencyKey() string {
	return fmt.Sprintf("%s:%d", d.TxHash, d.LogIndex)
}
