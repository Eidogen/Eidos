package model

import "github.com/shopspring/decimal"

// WithdrawalTxStatus 提现交易状态
type WithdrawalTxStatus int8

const (
	WithdrawalTxStatusPending   WithdrawalTxStatus = 0 // 待提交
	WithdrawalTxStatusSubmitted WithdrawalTxStatus = 1 // 已提交
	WithdrawalTxStatusConfirmed WithdrawalTxStatus = 2 // 已确认
	WithdrawalTxStatusFailed    WithdrawalTxStatus = 3 // 失败
)

func (s WithdrawalTxStatus) String() string {
	switch s {
	case WithdrawalTxStatusPending:
		return "PENDING"
	case WithdrawalTxStatusSubmitted:
		return "SUBMITTED"
	case WithdrawalTxStatusConfirmed:
		return "CONFIRMED"
	case WithdrawalTxStatusFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal 判断是否为终态
func (s WithdrawalTxStatus) IsTerminal() bool {
	return s == WithdrawalTxStatusConfirmed || s == WithdrawalTxStatusFailed
}

// WithdrawalTx 提现交易记录
type WithdrawalTx struct {
	ID            int64              `gorm:"primaryKey;autoIncrement" json:"id"`
	WithdrawID    string             `gorm:"column:withdraw_id;type:varchar(64);uniqueIndex;not null" json:"withdraw_id"`
	WalletAddress string             `gorm:"column:wallet_address;type:varchar(42);index;not null" json:"wallet_address"`
	ToAddress     string             `gorm:"column:to_address;type:varchar(42);not null" json:"to_address"`
	Token         string             `gorm:"column:token;type:varchar(20);not null" json:"token"`
	TokenAddress  string             `gorm:"column:token_address;type:varchar(42);not null" json:"token_address"`
	Amount        decimal.Decimal    `gorm:"column:amount;type:decimal(36,18);not null" json:"amount"`
	ChainID       int64              `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	TxHash        string             `gorm:"column:tx_hash;type:varchar(66)" json:"tx_hash"`
	BlockNumber   int64              `gorm:"column:block_number;type:bigint" json:"block_number"`
	GasUsed       int64              `gorm:"column:gas_used;type:bigint" json:"gas_used"`
	Status        WithdrawalTxStatus `gorm:"column:status;type:smallint;index;not null;default:0" json:"status"`
	ErrorMessage  string             `gorm:"column:error_message;type:varchar(500)" json:"error_message"`
	RetryCount    int                `gorm:"column:retry_count;type:int;not null;default:0" json:"retry_count"`
	SubmittedAt   int64              `gorm:"column:submitted_at;type:bigint" json:"submitted_at"`
	ConfirmedAt   int64              `gorm:"column:confirmed_at;type:bigint" json:"confirmed_at"`
	CreatedAt     int64              `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt     int64              `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (WithdrawalTx) TableName() string {
	return "chain_withdrawal_txs"
}

// WithdrawalRequest 提现请求 (从 Kafka 消费)
type WithdrawalRequest struct {
	WithdrawID   string          `json:"withdraw_id"`
	Wallet       string          `json:"wallet"`
	ToAddress    string          `json:"to_address"`
	Token        string          `json:"token"`
	TokenAddress string          `json:"token_address"`
	Amount       decimal.Decimal `json:"amount"`
	Nonce        int64           `json:"nonce"`
	Signature    string          `json:"signature"` // EIP-712 签名 (hex)
	CreatedAt    int64           `json:"created_at"`
}

// WithdrawalConfirmation 提现确认事件 (发送到 Kafka)
type WithdrawalConfirmation struct {
	WithdrawID  string `json:"withdraw_id"`
	Wallet      string `json:"wallet"`
	TxHash      string `json:"tx_hash"`
	BlockNumber int64  `json:"block_number"`
	GasUsed     int64  `json:"gas_used"`
	Status      string `json:"status"` // CONFIRMED/FAILED
	Error       string `json:"error,omitempty"`
	ConfirmedAt int64  `json:"confirmed_at"`
}
