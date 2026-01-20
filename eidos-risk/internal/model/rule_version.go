package model

// RuleChangeType 规则变更类型
type RuleChangeType string

const (
	RuleChangeTypeCreate  RuleChangeType = "CREATE"  // 创建
	RuleChangeTypeUpdate  RuleChangeType = "UPDATE"  // 更新
	RuleChangeTypeEnable  RuleChangeType = "ENABLE"  // 启用
	RuleChangeTypeDisable RuleChangeType = "DISABLE" // 禁用
)

// RiskRuleVersion 规则版本历史
type RiskRuleVersion struct {
	ID             int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleID         string         `gorm:"column:rule_id;type:varchar(64);not null" json:"rule_id"`
	Version        int            `gorm:"column:version;type:int;not null" json:"version"`
	ConfigSnapshot string         `gorm:"column:config_snapshot;type:jsonb;not null" json:"config_snapshot"` // 该版本的完整配置快照
	Enabled        bool           `gorm:"column:enabled;type:boolean;not null" json:"enabled"`
	ChangeType     RuleChangeType `gorm:"column:change_type;type:varchar(20);not null" json:"change_type"`
	ChangeReason   string         `gorm:"column:change_reason;type:varchar(500)" json:"change_reason"`
	ChangedBy      string         `gorm:"column:changed_by;type:varchar(42);not null" json:"changed_by"`
	ChangedAt      int64          `gorm:"column:changed_at;type:bigint;not null" json:"changed_at"`
	EffectiveAt    int64          `gorm:"column:effective_at;type:bigint;not null" json:"effective_at"`
}

// TableName 返回表名
func (RiskRuleVersion) TableName() string {
	return "risk_rule_versions"
}
