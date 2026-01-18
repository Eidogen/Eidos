package model

import (
	"github.com/shopspring/decimal"
)

// RiskEventType 风控事件类型
type RiskEventType int8

const (
	RiskEventTypeOrderCheck        RiskEventType = 1 // 下单检查
	RiskEventTypeCancelCheck       RiskEventType = 2 // 取消订单检查
	RiskEventTypeWithdrawCheck     RiskEventType = 3 // 提现检查
	RiskEventTypeTradeMonitor      RiskEventType = 4 // 成交监控
	RiskEventTypeLargeTrade        RiskEventType = 5 // 大额交易
	RiskEventTypePendingLimitCheck RiskEventType = 6 // 待结算限额检查
	RiskEventTypeRateLimitCheck    RiskEventType = 7 // 频率限制检查
	RiskEventTypeBlacklistCheck    RiskEventType = 8 // 黑名单检查
)

// String 返回事件类型的字符串表示
func (t RiskEventType) String() string {
	switch t {
	case RiskEventTypeOrderCheck:
		return "ORDER_CHECK"
	case RiskEventTypeCancelCheck:
		return "CANCEL_CHECK"
	case RiskEventTypeWithdrawCheck:
		return "WITHDRAW_CHECK"
	case RiskEventTypeTradeMonitor:
		return "TRADE_MONITOR"
	case RiskEventTypeLargeTrade:
		return "LARGE_TRADE"
	case RiskEventTypePendingLimitCheck:
		return "PENDING_LIMIT_CHECK"
	case RiskEventTypeRateLimitCheck:
		return "RATE_LIMIT_CHECK"
	case RiskEventTypeBlacklistCheck:
		return "BLACKLIST_CHECK"
	default:
		return "UNKNOWN"
	}
}

// RiskEventResult 风控事件结果
type RiskEventResult int8

const (
	RiskEventResultPassed   RiskEventResult = 0 // 通过
	RiskEventResultRejected RiskEventResult = 1 // 拒绝
	RiskEventResultAlert    RiskEventResult = 2 // 告警
	RiskEventResultDelayed  RiskEventResult = 3 // 延迟
	RiskEventResultFrozen   RiskEventResult = 4 // 冻结
)

// RiskEventStatus 风控事件状态
type RiskEventStatus int8

const (
	RiskEventStatusPending  RiskEventStatus = 0 // 待处理
	RiskEventStatusResolved RiskEventStatus = 1 // 已解决
	RiskEventStatusIgnored  RiskEventStatus = 2 // 已忽略
)

// String 返回事件状态的字符串表示
func (s RiskEventStatus) String() string {
	switch s {
	case RiskEventStatusPending:
		return "PENDING"
	case RiskEventStatusResolved:
		return "RESOLVED"
	case RiskEventStatusIgnored:
		return "IGNORED"
	default:
		return "UNKNOWN"
	}
}

// String 返回事件结果的字符串表示
func (r RiskEventResult) String() string {
	switch r {
	case RiskEventResultPassed:
		return "PASSED"
	case RiskEventResultRejected:
		return "REJECTED"
	case RiskEventResultAlert:
		return "ALERT"
	case RiskEventResultDelayed:
		return "DELAYED"
	case RiskEventResultFrozen:
		return "FROZEN"
	default:
		return "UNKNOWN"
	}
}

// RiskEvent 风控事件记录
type RiskEvent struct {
	ID             int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID        string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"event_id"`             // 事件ID
	Type           RiskEventType   `gorm:"type:smallint;not null;index" json:"type"`                          // 事件类型
	Wallet         string          `gorm:"type:varchar(42);index;not null" json:"wallet"`                     // 钱包地址
	Market         string          `gorm:"type:varchar(20);index" json:"market"`                              // 交易对
	Token          string          `gorm:"type:varchar(20)" json:"token"`                                     // Token
	ReferenceID    string          `gorm:"type:varchar(64);index" json:"reference_id"`                        // 关联ID (订单ID/交易ID/提现ID)
	ReferenceType  string          `gorm:"type:varchar(32)" json:"reference_type"`                            // 关联类型 (ORDER/TRADE/WITHDRAW)
	Amount         decimal.Decimal `gorm:"type:decimal(36,18)" json:"amount"`                                 // 金额
	RuleID         string          `gorm:"type:varchar(64);index" json:"rule_id"`                             // 触发的规则ID
	RuleName       string          `gorm:"type:varchar(128)" json:"rule_name"`                                // 触发的规则名称
	RiskLevel      string          `gorm:"type:varchar(20);index" json:"risk_level"`                          // 风险级别 (LOW/MEDIUM/HIGH/CRITICAL)
	Result         RiskEventResult `gorm:"type:smallint;not null;index" json:"result"`                        // 检查结果
	Status         RiskEventStatus `gorm:"type:smallint;not null;default:0;index" json:"status"`              // 事件状态
	Reason         string          `gorm:"type:varchar(512)" json:"reason"`                                   // 原因说明
	Details        string          `gorm:"type:text" json:"details"`                                          // 详细信息 (JSON)
	ProcessedAt    int64           `gorm:"type:bigint" json:"processed_at"`                                   // 处理时间
	ProcessedBy    string          `gorm:"type:varchar(64)" json:"processed_by"`                              // 处理者
	ProcessingNote string          `gorm:"type:varchar(512)" json:"processing_note"`                          // 处理备注
	CreatedAt      int64           `gorm:"type:bigint;not null;autoCreateTime:milli;index" json:"created_at"` // 创建时间
}

// TableName 返回表名
func (RiskEvent) TableName() string {
	return "risk_events"
}

// IsPassed 检查是否通过
func (e *RiskEvent) IsPassed() bool {
	return e.Result == RiskEventResultPassed
}

// IsRejected 检查是否被拒绝
func (e *RiskEvent) IsRejected() bool {
	return e.Result == RiskEventResultRejected
}

// NeedsProcessing 检查是否需要人工处理
func (e *RiskEvent) NeedsProcessing() bool {
	return e.Result == RiskEventResultAlert || e.Result == RiskEventResultDelayed
}

// IsProcessed 检查是否已处理
func (e *RiskEvent) IsProcessed() bool {
	return e.ProcessedAt > 0
}
