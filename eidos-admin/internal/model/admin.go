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
	// 交易对管理权限
	PermMarketRead  = "market:read"
	PermMarketWrite = "market:write"

	// 用户管理权限
	PermUserRead  = "user:read"
	PermUserWrite = "user:write"

	// 订单管理权限
	PermOrderRead  = "order:read"
	PermOrderWrite = "order:write"

	// 成交管理权限
	PermTradeRead = "trade:read"

	// 充值管理权限
	PermDepositRead = "deposit:read"

	// 提现管理权限
	PermWithdrawalRead  = "withdrawal:read"
	PermWithdrawalWrite = "withdrawal:write"

	// 风控管理权限
	PermRiskRead  = "risk:read"
	PermRiskWrite = "risk:write"

	// 统计查询权限
	PermStatsRead = "stats:read"

	// 系统配置权限
	PermConfigRead  = "config:read"
	PermConfigWrite = "config:write"

	// 审计日志权限
	PermAuditRead = "audit:read"

	// 管理员管理权限
	PermAdminRead  = "admin:read"
	PermAdminWrite = "admin:write"

	// 服务管理权限
	PermServiceAdmin = "service:admin"
)

// RolePermissions 角色权限映射
var RolePermissions = map[Role][]string{
	RoleSuperAdmin: {
		// 全部权限
		PermMarketRead, PermMarketWrite,
		PermUserRead, PermUserWrite,
		PermOrderRead, PermOrderWrite,
		PermTradeRead,
		PermDepositRead,
		PermWithdrawalRead, PermWithdrawalWrite,
		PermRiskRead, PermRiskWrite,
		PermStatsRead,
		PermConfigRead, PermConfigWrite,
		PermAuditRead,
		PermAdminRead, PermAdminWrite,
		PermServiceAdmin,
	},
	RoleOperator: {
		// 运营人员权限 - 可以管理用户、订单、提现、风控
		PermMarketRead, PermMarketWrite,
		PermUserRead, PermUserWrite,
		PermOrderRead, PermOrderWrite,
		PermTradeRead,
		PermDepositRead,
		PermWithdrawalRead, PermWithdrawalWrite,
		PermRiskRead, PermRiskWrite,
		PermStatsRead,
	},
	RoleSupport: {
		// 客服人员权限 - 只读查看用户、订单等
		PermUserRead,
		PermOrderRead,
		PermTradeRead,
		PermDepositRead,
		PermWithdrawalRead,
		PermRiskRead,
	},
	RoleViewer: {
		// 只读查看权限
		PermMarketRead,
		PermUserRead,
		PermOrderRead,
		PermTradeRead,
		PermDepositRead,
		PermWithdrawalRead,
		PermRiskRead,
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
