package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
)

// RuleService 规则管理服务
type RuleService struct {
	repo *repository.RiskRuleRepository

	// 风控告警回调
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error
}

// NewRuleService 创建规则服务
func NewRuleService(repo *repository.RiskRuleRepository) *RuleService {
	return &RuleService{repo: repo}
}

// SetOnRiskAlert 设置风控告警回调
func (s *RuleService) SetOnRiskAlert(fn func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = fn
}

// ListRules 获取规则列表
func (s *RuleService) ListRules(ctx context.Context) ([]*model.RiskRule, error) {
	pagination := &repository.Pagination{Page: 1, PageSize: 1000}
	rules, _, err := s.repo.ListAll(ctx, pagination)
	return rules, err
}

// GetRule 获取单个规则
func (s *RuleService) GetRule(ctx context.Context, ruleID string) (*model.RiskRule, error) {
	return s.repo.GetByRuleID(ctx, ruleID)
}

// UpdateRule 更新规则
func (s *RuleService) UpdateRule(ctx context.Context, req *UpdateRuleRequest) error {
	now := time.Now().UnixMilli()

	// 获取当前规则
	rule, err := s.repo.GetByRuleID(ctx, req.RuleID)
	if err != nil {
		return err
	}

	// 获取当前最新版本号
	latestVersion, err := s.repo.GetLatestVersion(ctx, req.RuleID)
	if err != nil {
		return err
	}

	// 创建版本快照
	oldConfigJSON, _ := json.Marshal(map[string]interface{}{
		"description": rule.Description,
		"threshold":   rule.Threshold.String(),
		"period":      rule.Period,
		"max_count":   rule.MaxCount,
		"action":      rule.Action.String(),
		"priority":    rule.Priority,
		"status":      rule.Status.String(),
	})

	changeType := model.RuleChangeTypeUpdate
	if req.Enabled != nil {
		if *req.Enabled {
			changeType = model.RuleChangeTypeEnable
		} else {
			changeType = model.RuleChangeTypeDisable
		}
	}

	version := &model.RiskRuleVersion{
		RuleID:         req.RuleID,
		Version:        latestVersion + 1,
		ConfigSnapshot: string(oldConfigJSON),
		Enabled:        rule.Status == model.RiskRuleStatusActive,
		ChangeType:     changeType,
		ChangeReason:   req.Reason,
		ChangedBy:      req.OperatorID,
		ChangedAt:      now,
		EffectiveAt:    now,
	}

	if err := s.repo.CreateVersion(ctx, version); err != nil {
		logger.Error("failed to create rule version", "error", err)
	}

	// 更新规则
	if req.Enabled != nil {
		if err := s.repo.SetEnabled(ctx, req.RuleID, *req.Enabled, req.OperatorID); err != nil {
			return err
		}
	}

	// 如果有其他字段更新
	if req.Params != nil {
		rule.UpdatedBy = req.OperatorID
		rule.UpdatedAt = now
		if err := s.repo.Update(ctx, rule); err != nil {
			return err
		}
	}

	// 发送告警
	s.sendAlert(ctx, &RiskAlertMessage{
		AlertID:     rule.RuleID,
		AlertType:   "RULE_UPDATED",
		Severity:    "info",
		Description: "风控规则已更新",
		Context: map[string]string{
			"rule_id":     req.RuleID,
			"change_type": string(changeType),
			"operator_id": req.OperatorID,
		},
		CreatedAt: now,
	})

	logger.Info("risk rule updated",
		"rule_id", req.RuleID,
		"change_type", string(changeType),
		"operator", req.OperatorID)

	return nil
}

// GetRuleVersions 获取规则版本历史
func (s *RuleService) GetRuleVersions(ctx context.Context, ruleID string) ([]*model.RiskRuleVersion, error) {
	return s.repo.ListVersions(ctx, ruleID)
}

// sendAlert 发送风控告警
func (s *RuleService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send risk alert",
				"alert_id", alert.AlertID,
				"error", err)
		}
	}
}

// UpdateRuleRequest 更新规则请求
type UpdateRuleRequest struct {
	RuleID     string
	Enabled    *bool
	Params     map[string]string
	Reason     string
	OperatorID string
}
