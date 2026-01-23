package model

// AuditAction 审计动作类型
type AuditAction string

const (
	AuditActionLogin        AuditAction = "login"
	AuditActionLogout       AuditAction = "logout"
	AuditActionCreate       AuditAction = "create"
	AuditActionUpdate       AuditAction = "update"
	AuditActionDelete       AuditAction = "delete"
	AuditActionStatusChange AuditAction = "status_change"
	AuditActionReload       AuditAction = "reload"
	AuditActionForceCancel  AuditAction = "force_cancel"
)

// ResourceType 资源类型
type ResourceType string

const (
	ResourceTypeAdmin        ResourceType = "admin"
	ResourceTypeMarket       ResourceType = "market"
	ResourceTypeToken        ResourceType = "token"
	ResourceTypeSystemConfig ResourceType = "system_config"
	ResourceTypeOrder        ResourceType = "order"
	ResourceTypeUser         ResourceType = "user"
	ResourceTypeService      ResourceType = "service"
)

// AuditStatus 审计状态
type AuditStatus string

const (
	AuditStatusSuccess AuditStatus = "success"
	AuditStatusFailed  AuditStatus = "failed"
)

// AuditLog 操作审计日志
type AuditLog struct {
	ID            int64        `gorm:"primaryKey;column:id" json:"id"`
	AdminID       int64        `gorm:"column:admin_id;index" json:"admin_id"`
	AdminUsername string       `gorm:"column:admin_username;size:50" json:"admin_username"`
	IPAddress     string       `gorm:"column:ip_address;size:45" json:"ip_address"`
	UserAgent     string       `gorm:"column:user_agent;size:500" json:"user_agent"`
	Action        AuditAction  `gorm:"column:action;size:50;index" json:"action"`
	ResourceType  ResourceType `gorm:"column:resource_type;size:50;index" json:"resource_type"`
	ResourceID    string       `gorm:"column:resource_id;size:100" json:"resource_id"`
	Description   string       `gorm:"column:description;size:500" json:"description"`
	OldValue      JSONMap      `gorm:"column:old_value;type:jsonb" json:"old_value,omitempty"`
	NewValue      JSONMap      `gorm:"column:new_value;type:jsonb" json:"new_value,omitempty"`
	Status        AuditStatus  `gorm:"column:status;size:20" json:"status"`
	ErrorMessage  string       `gorm:"column:error_message;size:500" json:"error_message,omitempty"`
	CreatedAt     int64        `gorm:"column:created_at;autoCreateTime:milli;index" json:"created_at"`
}

// TableName 表名
func (AuditLog) TableName() string {
	return "admin_audit_logs"
}
