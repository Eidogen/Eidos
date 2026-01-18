package model

import "github.com/shopspring/decimal"

// ReconciliationStatus 对账状态
type ReconciliationStatus string

const (
	ReconciliationStatusOK          ReconciliationStatus = "OK"          // 一致
	ReconciliationStatusDiscrepancy ReconciliationStatus = "DISCREPANCY" // 有差异
	ReconciliationStatusResolved    ReconciliationStatus = "RESOLVED"    // 已解决
	ReconciliationStatusIgnored     ReconciliationStatus = "IGNORED"     // 已忽略
)

// ReconciliationRecord 对账记录
type ReconciliationRecord struct {
	ID                int64                `gorm:"primaryKey;autoIncrement" json:"id"`
	WalletAddress     string               `gorm:"column:wallet_address;type:varchar(42);index;not null" json:"wallet_address"`
	Token             string               `gorm:"column:token;type:varchar(20);not null" json:"token"`
	OnChainBalance    decimal.Decimal      `gorm:"column:on_chain_balance;type:decimal(36,18);not null" json:"on_chain_balance"`
	OffChainSettled   decimal.Decimal      `gorm:"column:off_chain_settled;type:decimal(36,18);not null" json:"off_chain_settled"`
	OffChainAvailable decimal.Decimal      `gorm:"column:off_chain_available;type:decimal(36,18);not null" json:"off_chain_available"`
	OffChainFrozen    decimal.Decimal      `gorm:"column:off_chain_frozen;type:decimal(36,18);not null" json:"off_chain_frozen"`
	PendingSettle     decimal.Decimal      `gorm:"column:pending_settle;type:decimal(36,18);not null" json:"pending_settle"`
	Difference        decimal.Decimal      `gorm:"column:difference;type:decimal(36,18);not null" json:"difference"`
	Status            ReconciliationStatus `gorm:"column:status;type:varchar(20);index;not null" json:"status"`
	Resolution        string               `gorm:"column:resolution;type:varchar(500)" json:"resolution"`
	ResolvedBy        string               `gorm:"column:resolved_by;type:varchar(42)" json:"resolved_by"`
	ResolvedAt        int64                `gorm:"column:resolved_at;type:bigint" json:"resolved_at"`
	CheckedAt         int64                `gorm:"column:checked_at;type:bigint;index;not null" json:"checked_at"`
	CreatedAt         int64                `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
}

// TableName 返回表名
func (ReconciliationRecord) TableName() string {
	return "eidos_chain_reconciliation_records"
}

// HasDiscrepancy 是否有差异
func (r *ReconciliationRecord) HasDiscrepancy() bool {
	return !r.Difference.IsZero()
}
