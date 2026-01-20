package model

// ConfigType 配置值类型
type ConfigType string

const (
	ConfigTypeString  ConfigType = "string"
	ConfigTypeNumber  ConfigType = "number"
	ConfigTypeBoolean ConfigType = "boolean"
	ConfigTypeJSON    ConfigType = "json"
)

// SystemConfig 系统配置
type SystemConfig struct {
	ID          int64      `gorm:"primaryKey;column:id" json:"id"`
	ConfigKey   string     `gorm:"column:config_key;size:100;uniqueIndex" json:"config_key"`
	ConfigValue string     `gorm:"column:config_value;type:text" json:"config_value"`
	ConfigType  ConfigType `gorm:"column:config_type;size:20;default:string" json:"config_type"`
	Description string     `gorm:"column:description;size:500" json:"description"`
	IsSecret    bool       `gorm:"column:is_secret;default:false" json:"is_secret"`
	Scope       string     `gorm:"column:scope;size:50;default:global" json:"scope"`
	CreatedBy   int64      `gorm:"column:created_by" json:"created_by"`
	CreatedAt   int64      `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedBy   int64      `gorm:"column:updated_by" json:"updated_by"`
	UpdatedAt   int64      `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (SystemConfig) TableName() string {
	return "admin_system_configs"
}

// ConfigVersion 配置版本
type ConfigVersion struct {
	ID        int64  `gorm:"primaryKey;column:id" json:"id"`
	Scope     string `gorm:"column:scope;size:50;uniqueIndex" json:"scope"`
	Version   int64  `gorm:"column:version" json:"version"`
	UpdatedBy int64  `gorm:"column:updated_by" json:"updated_by"`
	UpdatedAt int64  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (ConfigVersion) TableName() string {
	return "admin_config_versions"
}

// 配置 scope 常量
const (
	ConfigScopeGlobal        = "global"
	ConfigScopeMarketConfigs = "market_configs"
	ConfigScopeSystemConfigs = "system_configs"
	ConfigScopeRiskRules     = "risk_rules"
)
