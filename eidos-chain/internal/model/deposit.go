package model

import "github.com/shopspring/decimal"

// DepositRecordStatus 充值记录状态
type DepositRecordStatus int8

const (
	DepositRecordStatusPending   DepositRecordStatus = 0 // 待确认
	DepositRecordStatusConfirmed DepositRecordStatus = 1 // 已确认 (达到确认数)
	DepositRecordStatusCredited  DepositRecordStatus = 2 // 已入账
)

func (s DepositRecordStatus) String() string {
	switch s {
	case DepositRecordStatusPending:
		return "PENDING"
	case DepositRecordStatusConfirmed:
		return "CONFIRMED"
	case DepositRecordStatusCredited:
		return "CREDITED"
	default:
		return "UNKNOWN"
	}
}

// DepositRecord 充值记录
type DepositRecord struct {
	ID                    int64               `gorm:"primaryKey;autoIncrement" json:"id"`
	DepositID             string              `gorm:"column:deposit_id;type:varchar(64);not null" json:"deposit_id"`
	WalletAddress         string              `gorm:"column:wallet_address;type:varchar(42);index;not null" json:"wallet_address"`
	Token                 string              `gorm:"column:token;type:varchar(20);not null" json:"token"`
	TokenAddress          string              `gorm:"column:token_address;type:varchar(42);not null" json:"token_address"`
	Amount                decimal.Decimal     `gorm:"column:amount;type:decimal(36,18);not null" json:"amount"`
	ChainID               int64               `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	TxHash                string              `gorm:"column:tx_hash;type:varchar(66);not null" json:"tx_hash"`
	BlockNumber           int64               `gorm:"column:block_number;type:bigint;not null" json:"block_number"`
	BlockHash             string              `gorm:"column:block_hash;type:varchar(66);not null" json:"block_hash"`
	LogIndex              int                 `gorm:"column:log_index;type:int;not null" json:"log_index"`
	Confirmations         int                 `gorm:"column:confirmations;type:int;not null;default:0" json:"confirmations"`
	RequiredConfirmations int                 `gorm:"column:required_confirmations;type:int;not null" json:"required_confirmations"`
	Status                DepositRecordStatus `gorm:"column:status;type:smallint;index;not null;default:0" json:"status"`
	CreditedAt            int64               `gorm:"column:credited_at;type:bigint" json:"credited_at"`
	CreatedAt             int64               `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt             int64               `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (DepositRecord) TableName() string {
	return "chain_deposit_records"
}

// DepositEvent 充值事件 (发送到 Kafka)
type DepositEvent struct {
	DepositID    string          `json:"deposit_id"`
	Wallet       string          `json:"wallet"`
	Token        string          `json:"token"`
	TokenAddress string          `json:"token_address"`
	Amount       decimal.Decimal `json:"amount"`
	TxHash       string          `json:"tx_hash"`
	BlockNumber  int64           `json:"block_number"`
	LogIndex     int             `json:"log_index"`
	DetectedAt   int64           `json:"detected_at"`
}
