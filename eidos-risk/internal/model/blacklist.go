package model

// BlacklistType 黑名单类型
type BlacklistType string

const (
	BlacklistTypeTrade    BlacklistType = "trade"    // 禁止下单
	BlacklistTypeWithdraw BlacklistType = "withdraw" // 禁止提现
	BlacklistTypeFull     BlacklistType = "full"     // 禁止所有操作
)

// BlacklistSource 黑名单来源
type BlacklistSource string

const (
	BlacklistSourceManual   BlacklistSource = "manual"   // 手动添加
	BlacklistSourceAuto     BlacklistSource = "auto"     // 自动触发
	BlacklistSourceExternal BlacklistSource = "external" // 外部数据 (如OFAC)
)

// BlacklistStatus 黑名单状态
type BlacklistStatus string

const (
	BlacklistStatusActive  BlacklistStatus = "active"  // 活跃
	BlacklistStatusExpired BlacklistStatus = "expired" // 已过期
	BlacklistStatusRemoved BlacklistStatus = "removed" // 已移除
)

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	ID             int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	WalletAddress  string          `gorm:"column:wallet_address;type:varchar(42);not null" json:"wallet_address"`
	ListType       BlacklistType   `gorm:"column:list_type;type:varchar(20);not null" json:"list_type"`
	Reason         string          `gorm:"column:reason;type:varchar(500);not null" json:"reason"`
	Source         BlacklistSource `gorm:"column:source;type:varchar(50);not null" json:"source"`
	EffectiveFrom  int64           `gorm:"column:effective_from;type:bigint;not null" json:"effective_from"`
	EffectiveUntil int64           `gorm:"column:effective_until;type:bigint" json:"effective_until"` // 0 或 NULL 表示永久
	Status         BlacklistStatus `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	CreatedBy      string          `gorm:"column:created_by;type:varchar(42)" json:"created_by"`
	CreatedAt      int64           `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedBy      string          `gorm:"column:updated_by;type:varchar(42)" json:"updated_by"`
	UpdatedAt      int64           `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (BlacklistEntry) TableName() string {
	return "risk_blacklist"
}

// IsActive 检查是否活跃
func (b *BlacklistEntry) IsActive() bool {
	return b.Status == BlacklistStatusActive
}

// IsPermanent 检查是否永久黑名单
func (b *BlacklistEntry) IsPermanent() bool {
	return b.EffectiveUntil == 0
}

// IsExpired 检查是否已过期
func (b *BlacklistEntry) IsExpired(now int64) bool {
	if b.EffectiveUntil == 0 {
		return false
	}
	return now > b.EffectiveUntil
}

// IsEffective 检查是否在有效期内
func (b *BlacklistEntry) IsEffective(now int64) bool {
	if now < b.EffectiveFrom {
		return false
	}
	if b.EffectiveUntil > 0 && now > b.EffectiveUntil {
		return false
	}
	return true
}

// CanTrade 检查是否可以交易
func (b *BlacklistEntry) CanTrade() bool {
	return b.ListType != BlacklistTypeTrade && b.ListType != BlacklistTypeFull
}

// CanWithdraw 检查是否可以提现
func (b *BlacklistEntry) CanWithdraw() bool {
	return b.ListType != BlacklistTypeWithdraw && b.ListType != BlacklistTypeFull
}
