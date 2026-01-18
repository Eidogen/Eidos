// Package model 定义风控服务的数据模型
package model

import (
	"github.com/shopspring/decimal"
)

// RiskRuleType 风控规则类型
type RiskRuleType int8

const (
	RiskRuleTypeAmountLimit   RiskRuleType = 1 // 金额限制
	RiskRuleTypeRateLimit     RiskRuleType = 2 // 频率限制
	RiskRuleTypeBlacklist     RiskRuleType = 3 // 黑名单
	RiskRuleTypeWhitelist     RiskRuleType = 4 // 白名单
	RiskRuleTypePendingLimit  RiskRuleType = 5 // 待结算限制
	RiskRuleTypeWithdrawLimit RiskRuleType = 6 // 提现限制
)

// String 返回规则类型的字符串表示
func (t RiskRuleType) String() string {
	switch t {
	case RiskRuleTypeAmountLimit:
		return "AMOUNT_LIMIT"
	case RiskRuleTypeRateLimit:
		return "RATE_LIMIT"
	case RiskRuleTypeBlacklist:
		return "BLACKLIST"
	case RiskRuleTypeWhitelist:
		return "WHITELIST"
	case RiskRuleTypePendingLimit:
		return "PENDING_LIMIT"
	case RiskRuleTypeWithdrawLimit:
		return "WITHDRAW_LIMIT"
	default:
		return "UNKNOWN"
	}
}

// RiskRuleStatus 风控规则状态
type RiskRuleStatus int8

const (
	RiskRuleStatusInactive RiskRuleStatus = 0 // 未激活
	RiskRuleStatusActive   RiskRuleStatus = 1 // 已激活
)

// String 返回规则状态的字符串表示
func (s RiskRuleStatus) String() string {
	if s == RiskRuleStatusActive {
		return "ACTIVE"
	}
	return "INACTIVE"
}

// RiskRuleScope 风控规则作用范围
type RiskRuleScope int8

const (
	RiskRuleScopeGlobal RiskRuleScope = 1 // 全局
	RiskRuleScopeMarket RiskRuleScope = 2 // 特定市场
	RiskRuleScopeUser   RiskRuleScope = 3 // 特定用户
	RiskRuleScopeToken  RiskRuleScope = 4 // 特定Token
)

// String 返回规则作用范围的字符串表示
func (s RiskRuleScope) String() string {
	switch s {
	case RiskRuleScopeGlobal:
		return "GLOBAL"
	case RiskRuleScopeMarket:
		return "MARKET"
	case RiskRuleScopeUser:
		return "USER"
	case RiskRuleScopeToken:
		return "TOKEN"
	default:
		return "UNKNOWN"
	}
}

// RiskRule 风控规则
type RiskRule struct {
	ID          int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleID      string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"rule_id"`           // 规则ID
	Name        string          `gorm:"type:varchar(128);not null" json:"name"`                         // 规则名称
	Description string          `gorm:"type:varchar(512)" json:"description"`                           // 规则描述
	Type        RiskRuleType    `gorm:"type:smallint;not null" json:"type"`                             // 规则类型
	Scope       RiskRuleScope   `gorm:"type:smallint;not null;default:1" json:"scope"`                  // 作用范围
	ScopeValue  string          `gorm:"type:varchar(64)" json:"scope_value"`                            // 作用范围值 (如市场名、用户地址)
	Threshold   decimal.Decimal `gorm:"type:decimal(36,18)" json:"threshold"`                           // 阈值
	Period      int64           `gorm:"type:bigint" json:"period"`                                      // 时间周期 (秒)
	MaxCount    int             `gorm:"type:integer" json:"max_count"`                                  // 最大次数 (频率限制)
	Action      RiskAction      `gorm:"type:smallint;not null;default:1" json:"action"`                 // 触发动作
	Priority    int             `gorm:"type:integer;default:100" json:"priority"`                       // 优先级 (数字越小优先级越高)
	Status      RiskRuleStatus  `gorm:"type:smallint;not null;default:1" json:"status"`                 // 状态
	EffectiveAt int64           `gorm:"type:bigint" json:"effective_at"`                                // 生效时间
	ExpireAt    int64           `gorm:"type:bigint" json:"expire_at"`                                   // 过期时间 (0表示永不过期)
	CreatedBy   string          `gorm:"type:varchar(64)" json:"created_by"`                             // 创建者
	CreatedAt   int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`    // 创建时间
	UpdatedBy   string          `gorm:"type:varchar(64)" json:"updated_by"`                             // 更新者
	UpdatedAt   int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`    // 更新时间
}

// TableName 返回表名
func (RiskRule) TableName() string {
	return "risk_rules"
}

// IsActive 检查规则是否激活
func (r *RiskRule) IsActive() bool {
	return r.Status == RiskRuleStatusActive
}

// IsExpired 检查规则是否过期
func (r *RiskRule) IsExpired(now int64) bool {
	if r.ExpireAt == 0 {
		return false // 永不过期
	}
	return now > r.ExpireAt
}

// IsEffective 检查规则是否已生效
func (r *RiskRule) IsEffective(now int64) bool {
	if r.EffectiveAt == 0 {
		return true // 立即生效
	}
	return now >= r.EffectiveAt
}

// IsValid 检查规则是否有效 (激活且在有效期内)
func (r *RiskRule) IsValid(now int64) bool {
	return r.IsActive() && r.IsEffective(now) && !r.IsExpired(now)
}

// RiskAction 风控动作
type RiskAction int8

const (
	RiskActionAlertOnly RiskAction = 1 // 仅告警
	RiskActionReject    RiskAction = 2 // 拒绝
	RiskActionDelay     RiskAction = 3 // 延迟处理
	RiskActionFreeze    RiskAction = 4 // 冻结账户
)

// String 返回风控动作的字符串表示
func (a RiskAction) String() string {
	switch a {
	case RiskActionAlertOnly:
		return "ALERT_ONLY"
	case RiskActionReject:
		return "REJECT"
	case RiskActionDelay:
		return "DELAY"
	case RiskActionFreeze:
		return "FREEZE"
	default:
		return "UNKNOWN"
	}
}
