package model

// AuditAction 审计操作类型
type AuditAction string

const (
	AuditActionCheckOrder    AuditAction = "CHECK_ORDER"
	AuditActionCheckWithdraw AuditAction = "CHECK_WITHDRAW"
	AuditActionAddBlacklist  AuditAction = "ADD_BLACKLIST"
	AuditActionRemoveBlack   AuditAction = "REMOVE_BLACKLIST"
	AuditActionUpdateRule    AuditAction = "UPDATE_RULE"
	AuditActionResolveEvent  AuditAction = "RESOLVE_EVENT"
)

// AuditResult 审计结果
type AuditResult string

const (
	AuditResultAllowed  AuditResult = "ALLOWED"
	AuditResultRejected AuditResult = "REJECTED"
	AuditResultPending  AuditResult = "PENDING"
)

// AuditLog 审计日志
type AuditLog struct {
	ID            int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	Action        AuditAction `gorm:"column:action;type:varchar(50);not null" json:"action"`
	WalletAddress string      `gorm:"column:wallet_address;type:varchar(42)" json:"wallet_address"`
	Request       string      `gorm:"column:request;type:jsonb;not null" json:"request"` // JSON 格式的请求内容
	Result        AuditResult `gorm:"column:result;type:varchar(20);not null" json:"result"`
	Reason        string      `gorm:"column:reason;type:varchar(500)" json:"reason"` // 拒绝原因
	DurationMs    int         `gorm:"column:duration_ms;type:int;not null" json:"duration_ms"`
	CreatedAt     int64       `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
}

// TableName 返回表名
func (AuditLog) TableName() string {
	return "eidos_risk_audit_logs"
}

// IsAllowed 检查是否允许
func (a *AuditLog) IsAllowed() bool {
	return a.Result == AuditResultAllowed
}

// IsRejected 检查是否拒绝
func (a *AuditLog) IsRejected() bool {
	return a.Result == AuditResultRejected
}
