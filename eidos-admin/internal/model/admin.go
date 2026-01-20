package model

// Role 管理员角色
type Role string

const (
	RoleSuperAdmin Role = "super_admin" // 超级管理员，全部权限
	RoleOperator   Role = "operator"    // 运营人员
	RoleSupport    Role = "support"     // 客服人员
	RoleViewer     Role = "viewer"      // 只读查看
)

// AdminStatus 管理员状态
type AdminStatus int

const (
	AdminStatusActive   AdminStatus = 1 // 活跃
	AdminStatusDisabled AdminStatus = 2 // 禁用
)

// Admin 管理员
type Admin struct {
	ID            int64       `gorm:"primaryKey;column:id" json:"id"`
	Username      string      `gorm:"column:username;size:50;uniqueIndex" json:"username"`
	PasswordHash  string      `gorm:"column:password_hash;size:100" json:"-"`
	Nickname      string      `gorm:"column:nickname;size:50" json:"nickname"`
	Email         string      `gorm:"column:email;size:100" json:"email"`
	Phone         string      `gorm:"column:phone;size:20" json:"phone"`
	Role          Role        `gorm:"column:role;size:20" json:"role"`
	Status        AdminStatus `gorm:"column:status;default:1" json:"status"`
	LastLoginAt   *int64      `gorm:"column:last_login_at" json:"last_login_at"`
	LastLoginIP   string      `gorm:"column:last_login_ip;size:45" json:"last_login_ip"`
	LoginAttempts int         `gorm:"column:login_attempts;default:0" json:"login_attempts"`
	LockedUntil   *int64      `gorm:"column:locked_until" json:"locked_until"`
	CreatedBy     int64       `gorm:"column:created_by" json:"created_by"`
	CreatedAt     int64       `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedBy     int64       `gorm:"column:updated_by" json:"updated_by"`
	UpdatedAt     int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (Admin) TableName() string {
	return "admin_admins"
}

// Permission 权限常量
const (
	PermMarketRead   = "market:read"
	PermMarketWrite  = "market:write"
	PermUserRead     = "user:read"
	PermOrderRead    = "order:read"
	PermOrderWrite   = "order:write"
	PermStatsRead    = "stats:read"
	PermConfigRead   = "config:read"
	PermConfigWrite  = "config:write"
	PermAuditRead    = "audit:read"
	PermAdminRead    = "admin:read"
	PermAdminWrite   = "admin:write"
	PermServiceAdmin = "service:admin"
)

// RolePermissions 角色权限映射
var RolePermissions = map[Role][]string{
	RoleSuperAdmin: {
		PermMarketRead, PermMarketWrite,
		PermUserRead,
		PermOrderRead, PermOrderWrite,
		PermStatsRead,
		PermConfigRead, PermConfigWrite,
		PermAuditRead,
		PermAdminRead, PermAdminWrite,
		PermServiceAdmin,
	},
	RoleOperator: {
		PermMarketRead, PermMarketWrite,
		PermUserRead,
		PermOrderRead, PermOrderWrite,
		PermStatsRead,
	},
	RoleSupport: {
		PermUserRead,
		PermOrderRead,
	},
	RoleViewer: {
		PermMarketRead,
		PermUserRead,
		PermOrderRead,
		PermStatsRead,
		PermConfigRead,
	},
}

// HasPermission 检查角色是否具有指定权限
func (r Role) HasPermission(perm string) bool {
	perms, ok := RolePermissions[r]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
