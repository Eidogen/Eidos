package model

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// SettlementBatchStatus 结算批次状态
type SettlementBatchStatus int8

const (
	SettlementBatchStatusPending   SettlementBatchStatus = 0 // 待提交
	SettlementBatchStatusSubmitted SettlementBatchStatus = 1 // 已提交 (链上交易已广播)
	SettlementBatchStatusConfirmed SettlementBatchStatus = 2 // 已确认 (交易上链)
	SettlementBatchStatusFailed    SettlementBatchStatus = 3 // 失败
)

func (s SettlementBatchStatus) String() string {
	switch s {
	case SettlementBatchStatusPending:
		return "PENDING"
	case SettlementBatchStatusSubmitted:
		return "SUBMITTED"
	case SettlementBatchStatusConfirmed:
		return "CONFIRMED"
	case SettlementBatchStatusFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal 判断是否为终态
func (s SettlementBatchStatus) IsTerminal() bool {
	return s == SettlementBatchStatusConfirmed || s == SettlementBatchStatusFailed
}

// SettlementBatch 结算批次
type SettlementBatch struct {
	ID           int64                 `gorm:"primaryKey;autoIncrement" json:"id"`
	BatchID      string                `gorm:"column:batch_id;type:varchar(64);uniqueIndex;not null" json:"batch_id"`
	TradeCount   int                   `gorm:"column:trade_count;type:int;not null" json:"trade_count"`
	TradeIDs     string                `gorm:"column:trade_ids;type:text;not null" json:"trade_ids"` // JSON 数组
	ChainID      int64                 `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	TxHash       string                `gorm:"column:tx_hash;type:varchar(66)" json:"tx_hash"`
	BlockNumber  int64                 `gorm:"column:block_number;type:bigint" json:"block_number"`
	GasUsed      int64                 `gorm:"column:gas_used;type:bigint" json:"gas_used"`
	GasPrice     string                `gorm:"column:gas_price;type:varchar(36)" json:"gas_price"` // wei
	Status       SettlementBatchStatus `gorm:"column:status;type:smallint;index;not null;default:0" json:"status"`
	ErrorMessage string                `gorm:"column:error_message;type:varchar(500)" json:"error_message"`
	RetryCount   int                   `gorm:"column:retry_count;type:int;not null;default:0" json:"retry_count"`
	SubmittedAt  int64                 `gorm:"column:submitted_at;type:bigint" json:"submitted_at"`
	ConfirmedAt  int64                 `gorm:"column:confirmed_at;type:bigint" json:"confirmed_at"`
	CreatedAt    int64                 `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt    int64                 `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (SettlementBatch) TableName() string {
	return "chain_settlement_batches"
}

// GetTradeIDList 解析 TradeIDs 为字符串数组
func (s *SettlementBatch) GetTradeIDList() ([]string, error) {
	var ids []string
	if err := json.Unmarshal([]byte(s.TradeIDs), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// SetTradeIDList 设置 TradeIDs
func (s *SettlementBatch) SetTradeIDList(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	s.TradeIDs = string(data)
	s.TradeCount = len(ids)
	return nil
}

// SettlementRollbackLog 结算回滚审计日志
type SettlementRollbackLog struct {
	ID               int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	BatchID          string          `gorm:"column:batch_id;type:varchar(64);not null" json:"batch_id"`
	TradeCount       int             `gorm:"column:trade_count;type:int;not null" json:"trade_count"`
	AffectedUsers    int             `gorm:"column:affected_users;type:int;not null" json:"affected_users"`
	TotalBaseAmount  decimal.Decimal `gorm:"column:total_base_amount;type:decimal(36,18);not null" json:"total_base_amount"`
	TotalQuoteAmount decimal.Decimal `gorm:"column:total_quote_amount;type:decimal(36,18);not null" json:"total_quote_amount"`
	FailureReason    string          `gorm:"column:failure_reason;type:varchar(500);not null" json:"failure_reason"`
	RollbackReason   string          `gorm:"column:rollback_reason;type:varchar(500);not null" json:"rollback_reason"`
	Operator         string          `gorm:"column:operator;type:varchar(42);not null" json:"operator"`
	ApprovedBy       string          `gorm:"column:approved_by;type:varchar(42);not null" json:"approved_by"`
	RollbackAt       int64           `gorm:"column:rollback_at;type:bigint;not null" json:"rollback_at"`
	OriginalTxHash   string          `gorm:"column:original_tx_hash;type:varchar(66)" json:"original_tx_hash"`
	CreatedAt        int64           `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
}

// TableName 返回表名
func (SettlementRollbackLog) TableName() string {
	return "chain_settlement_rollback_logs"
}

// SettlementTrade 待结算的成交信息 (从 Kafka 消费)
type SettlementTrade struct {
	TradeID      string          `json:"trade_id"`
	Market       string          `json:"market"`
	MakerWallet  string          `json:"maker_wallet"`
	TakerWallet  string          `json:"taker_wallet"`
	MakerOrderID string          `json:"maker_order_id"`
	TakerOrderID string          `json:"taker_order_id"`
	Price        decimal.Decimal `json:"price"`
	Amount       decimal.Decimal `json:"amount"`      // Base token 数量
	QuoteAmount  decimal.Decimal `json:"quote_amount"` // Quote token 数量
	MakerFee     decimal.Decimal `json:"maker_fee"`
	TakerFee     decimal.Decimal `json:"taker_fee"`
	MakerSide    int8            `json:"maker_side"` // 1=buy, 2=sell
	MatchedAt    int64           `json:"matched_at"`
}

// SettlementConfirmation 结算确认事件 (发送到 Kafka)
type SettlementConfirmation struct {
	BatchID     string   `json:"batch_id"`
	TradeIDs    []string `json:"trade_ids"`
	TxHash      string   `json:"tx_hash"`
	BlockNumber int64    `json:"block_number"`
	GasUsed     int64    `json:"gas_used"`
	Status      string   `json:"status"` // CONFIRMED/FAILED
	Error       string   `json:"error,omitempty"`
	ConfirmedAt int64    `json:"confirmed_at"`
}
