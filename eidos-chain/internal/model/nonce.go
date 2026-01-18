package model

// WalletNonce 热钱包 Nonce
type WalletNonce struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	WalletAddress string `gorm:"column:wallet_address;type:varchar(42);not null" json:"wallet_address"`
	ChainID       int64  `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	CurrentNonce  int64  `gorm:"column:current_nonce;type:bigint;not null;default:0" json:"current_nonce"`
	SyncedAt      int64  `gorm:"column:synced_at;type:bigint;not null" json:"synced_at"`
	CreatedAt     int64  `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt     int64  `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (WalletNonce) TableName() string {
	return "eidos_chain_wallet_nonces"
}

// PendingTxStatus 待确认交易状态
type PendingTxStatus int8

const (
	PendingTxStatusPending   PendingTxStatus = 0 // 待确认
	PendingTxStatusConfirmed PendingTxStatus = 1 // 已确认
	PendingTxStatusFailed    PendingTxStatus = 2 // 失败
	PendingTxStatusReplaced  PendingTxStatus = 3 // 已替换
)

func (s PendingTxStatus) String() string {
	switch s {
	case PendingTxStatusPending:
		return "PENDING"
	case PendingTxStatusConfirmed:
		return "CONFIRMED"
	case PendingTxStatusFailed:
		return "FAILED"
	case PendingTxStatusReplaced:
		return "REPLACED"
	default:
		return "UNKNOWN"
	}
}

// PendingTxType 待确认交易类型
type PendingTxType string

const (
	PendingTxTypeSettlement PendingTxType = "settlement"
	PendingTxTypeWithdrawal PendingTxType = "withdrawal"
)

// PendingTx 待确认交易
type PendingTx struct {
	ID            int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	TxHash        string          `gorm:"column:tx_hash;type:varchar(66);uniqueIndex;not null" json:"tx_hash"`
	TxType        PendingTxType   `gorm:"column:tx_type;type:varchar(20);not null" json:"tx_type"`
	RefID         string          `gorm:"column:ref_id;type:varchar(64);index;not null" json:"ref_id"` // batch_id 或 withdraw_id
	WalletAddress string          `gorm:"column:wallet_address;type:varchar(42);not null" json:"wallet_address"`
	ChainID       int64           `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	Nonce         int64           `gorm:"column:nonce;type:bigint;not null" json:"nonce"`
	GasPrice      string          `gorm:"column:gas_price;type:varchar(36);not null" json:"gas_price"`
	GasLimit      int64           `gorm:"column:gas_limit;type:bigint;not null" json:"gas_limit"`
	SubmittedAt   int64           `gorm:"column:submitted_at;type:bigint;not null" json:"submitted_at"`
	TimeoutAt     int64           `gorm:"column:timeout_at;type:bigint;index;not null" json:"timeout_at"`
	Status        PendingTxStatus `gorm:"column:status;type:smallint;index;not null;default:0" json:"status"`
	CreatedAt     int64           `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt     int64           `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (PendingTx) TableName() string {
	return "eidos_chain_pending_txs"
}
