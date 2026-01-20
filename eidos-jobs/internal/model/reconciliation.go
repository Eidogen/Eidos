package model

// ReconciliationType 对账类型
type ReconciliationType string

const (
	ReconciliationTypeIncremental ReconciliationType = "incremental"
	ReconciliationTypeFull        ReconciliationType = "full"
)

// ReconciliationJobType 对账任务类型
type ReconciliationJobType string

const (
	ReconciliationJobBalance    ReconciliationJobType = "balance"
	ReconciliationJobDeposit    ReconciliationJobType = "deposit"
	ReconciliationJobWithdrawal ReconciliationJobType = "withdrawal"
	ReconciliationJobSettlement ReconciliationJobType = "settlement"
)

// ReconciliationStatus 对账结果状态
type ReconciliationStatus string

const (
	ReconciliationStatusMatched    ReconciliationStatus = "matched"
	ReconciliationStatusMismatched ReconciliationStatus = "mismatched"
)

// ReconciliationRecord 对账记录
type ReconciliationRecord struct {
	ID             int64        `gorm:"column:id;primaryKey;autoIncrement"`
	JobType        string       `gorm:"column:job_type;type:varchar(50);not null"`
	ReconcileType  string       `gorm:"column:reconcile_type;type:varchar(20);not null"`
	Status         string       `gorm:"column:status;type:varchar(20);not null"`
	WalletAddress  *string      `gorm:"column:wallet_address;type:varchar(42)"`
	Token          *string      `gorm:"column:token;type:varchar(42)"`
	OffchainValue  *string      `gorm:"column:offchain_value;type:decimal(36,18)"`
	OnchainValue   *string      `gorm:"column:onchain_value;type:decimal(36,18)"`
	DiffValue      *string      `gorm:"column:diff_value;type:decimal(36,18)"`
	Details        JSONResult   `gorm:"column:details;type:jsonb"`
	CreatedAt      int64        `gorm:"column:created_at;not null"`
}

// TableName 表名
func (ReconciliationRecord) TableName() string {
	return "jobs_reconciliation_records"
}

// ReconciliationCheckpoint 对账检查点
type ReconciliationCheckpoint struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	JobType   string `gorm:"column:job_type;type:varchar(50);not null;uniqueIndex"`
	LastTime  int64  `gorm:"column:last_time;not null"`
	LastBlock *int64 `gorm:"column:last_block"`
	UpdatedAt int64  `gorm:"column:updated_at;not null"`
}

// TableName 表名
func (ReconciliationCheckpoint) TableName() string {
	return "jobs_reconciliation_checkpoints"
}

// DiffThreshold 差异阈值 (USDC)
const (
	DiffThresholdSmall   = "0.01"   // 小额差异: 自动忽略
	DiffThresholdMedium  = "100"    // 中等差异: 告警通知
	DiffThresholdLarge   = "100"    // 大额差异: 紧急告警
)
