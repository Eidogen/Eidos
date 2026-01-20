package model

import "github.com/shopspring/decimal"

// WithdrawalReviewStatus 提现审核状态
type WithdrawalReviewStatus string

const (
	WithdrawalReviewStatusPending  WithdrawalReviewStatus = "pending"  // 待审核
	WithdrawalReviewStatusApproved WithdrawalReviewStatus = "approved" // 已通过
	WithdrawalReviewStatusRejected WithdrawalReviewStatus = "rejected" // 已拒绝
)

// AutoDecision 自动决策类型
type AutoDecision string

const (
	AutoDecisionAutoApprove  AutoDecision = "AUTO_APPROVE"  // 自动通过
	AutoDecisionManualReview AutoDecision = "MANUAL_REVIEW" // 需人工审核
	AutoDecisionAutoReject   AutoDecision = "AUTO_REJECT"   // 自动拒绝
)

// WithdrawalReview 提现审核记录
type WithdrawalReview struct {
	ID            int64                  `gorm:"primaryKey;autoIncrement" json:"id"`
	ReviewID      string                 `gorm:"column:review_id;type:varchar(64);uniqueIndex;not null" json:"review_id"`
	WithdrawalID  string                 `gorm:"column:withdrawal_id;type:varchar(64);uniqueIndex;not null" json:"withdrawal_id"`
	WalletAddress string                 `gorm:"column:wallet_address;type:varchar(42);index;not null" json:"wallet_address"`
	Token         string                 `gorm:"column:token;type:varchar(42);not null" json:"token"`
	Amount        decimal.Decimal        `gorm:"column:amount;type:decimal(36,18);not null" json:"amount"`
	ToAddress     string                 `gorm:"column:to_address;type:varchar(42);not null" json:"to_address"`
	RiskScore     int                    `gorm:"column:risk_score;type:int;not null" json:"risk_score"` // 0-100
	RiskFactors   string                 `gorm:"column:risk_factors;type:jsonb;not null" json:"risk_factors"`
	AutoDecision  AutoDecision           `gorm:"column:auto_decision;type:varchar(20)" json:"auto_decision"`
	Status        WithdrawalReviewStatus `gorm:"column:status;type:varchar(20);index;not null;default:'pending'" json:"status"`
	Reviewer      string                 `gorm:"column:reviewer;type:varchar(42)" json:"reviewer"`
	ReviewComment string                 `gorm:"column:review_comment;type:varchar(500)" json:"review_comment"`
	ReviewedAt    int64                  `gorm:"column:reviewed_at;type:bigint" json:"reviewed_at"`
	CreatedAt     int64                  `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	ExpiresAt     int64                  `gorm:"column:expires_at;type:bigint;not null" json:"expires_at"` // 24小时后
}

// TableName 返回表名
func (WithdrawalReview) TableName() string {
	return "risk_withdrawal_reviews"
}

// IsPending 检查是否待审核
func (w *WithdrawalReview) IsPending() bool {
	return w.Status == WithdrawalReviewStatusPending
}

// IsApproved 检查是否已通过
func (w *WithdrawalReview) IsApproved() bool {
	return w.Status == WithdrawalReviewStatusApproved
}

// IsRejected 检查是否已拒绝
func (w *WithdrawalReview) IsRejected() bool {
	return w.Status == WithdrawalReviewStatusRejected
}

// IsExpired 检查是否已过期
func (w *WithdrawalReview) IsExpired(now int64) bool {
	return now > w.ExpiresAt
}

// IsLowRisk 检查是否低风险
func (w *WithdrawalReview) IsLowRisk() bool {
	return w.RiskScore < 30
}

// IsMediumRisk 检查是否中等风险
func (w *WithdrawalReview) IsMediumRisk() bool {
	return w.RiskScore >= 30 && w.RiskScore < 70
}

// IsHighRisk 检查是否高风险
func (w *WithdrawalReview) IsHighRisk() bool {
	return w.RiskScore >= 70
}

// RiskFactorEntry 风险因素条目
type RiskFactorEntry struct {
	Factor string `json:"factor"` // 因素名称
	Score  int    `json:"score"`  // 分值
	Reason string `json:"reason"` // 原因说明
}
