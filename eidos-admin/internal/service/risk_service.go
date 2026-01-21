package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/client"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
)

// RiskService provides risk management functionality
type RiskService struct {
	riskClient *client.RiskClient
	auditRepo  *repository.AuditLogRepository
}

// NewRiskService creates a new risk service
func NewRiskService(
	riskClient *client.RiskClient,
	auditRepo *repository.AuditLogRepository,
) *RiskService {
	return &RiskService{
		riskClient: riskClient,
		auditRepo:  auditRepo,
	}
}

// === Risk Rules Management ===

// RiskRule represents a risk rule
type RiskRule struct {
	RuleID      string            `json:"rule_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Level       string            `json:"level"`
	Action      string            `json:"action"`
	IsEnabled   bool              `json:"is_enabled"`
	Params      map[string]string `json:"params"`
	Markets     []string          `json:"markets"`
	Tokens      []string          `json:"tokens"`
	Priority    int32             `json:"priority"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
	UpdatedBy   string            `json:"updated_by"`
}

// ListRiskRules retrieves all risk rules
func (s *RiskService) ListRiskRules(ctx context.Context, category string, enabledOnly bool) ([]*RiskRule, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.ListRiskRules(ctx, category, enabledOnly)
	if err != nil {
		return nil, fmt.Errorf("failed to list risk rules: %w", err)
	}

	rules := make([]*RiskRule, 0, len(resp.Rules))
	for _, r := range resp.Rules {
		rules = append(rules, &RiskRule{
			RuleID:      r.RuleId,
			Name:        r.Name,
			Description: r.Description,
			Category:    r.Category,
			Level:       r.Level.String(),
			Action:      r.Action.String(),
			IsEnabled:   r.IsEnabled,
			Params:      r.Params,
			Markets:     r.Markets,
			Tokens:      r.Tokens,
			Priority:    r.Priority,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			UpdatedBy:   r.UpdatedBy,
		})
	}

	return rules, nil
}

// GetRiskRule retrieves a specific risk rule
func (s *RiskService) GetRiskRule(ctx context.Context, ruleID string) (*RiskRule, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	r, err := s.riskClient.GetRiskRule(ctx, ruleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get risk rule: %w", err)
	}

	return &RiskRule{
		RuleID:      r.RuleId,
		Name:        r.Name,
		Description: r.Description,
		Category:    r.Category,
		Level:       r.Level.String(),
		Action:      r.Action.String(),
		IsEnabled:   r.IsEnabled,
		Params:      r.Params,
		Markets:     r.Markets,
		Tokens:      r.Tokens,
		Priority:    r.Priority,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		UpdatedBy:   r.UpdatedBy,
	}, nil
}

// UpdateRiskRuleRequest request for updating a risk rule
type UpdateRiskRuleRequest struct {
	RuleID     string              `json:"rule_id" binding:"required"`
	IsEnabled  *bool               `json:"is_enabled"`
	Params     map[string]string   `json:"params"`
	Action     commonv1.RiskAction `json:"action"`
	OperatorID int64               `json:"-"`
}

// UpdateRiskRule updates a risk rule
func (s *RiskService) UpdateRiskRule(ctx context.Context, req *UpdateRiskRuleRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	// Get old rule for audit
	oldRule, _ := s.riskClient.GetRiskRule(ctx, req.RuleID)

	grpcReq := &riskv1.UpdateRiskRuleRequest{
		RuleId:     req.RuleID,
		Params:     req.Params,
		Action:     req.Action,
		OperatorId: strconv.FormatInt(req.OperatorID, 10),
	}
	if req.IsEnabled != nil {
		grpcReq.IsEnabled = *req.IsEnabled
	}

	resp, err := s.riskClient.UpdateRiskRule(ctx, grpcReq)
	if err != nil {
		return fmt.Errorf("failed to update risk rule: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("update risk rule failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	oldValue := model.JSONMap{}
	if oldRule != nil {
		oldValue = model.JSONMap{
			"is_enabled": oldRule.IsEnabled,
			"params":     oldRule.Params,
			"action":     oldRule.Action.String(),
		}
	}
	newValue := model.JSONMap{
		"is_enabled": req.IsEnabled,
		"params":     req.Params,
		"action":     req.Action.String(),
	}
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, "risk_rule",
		req.RuleID, "更新风控规则", oldValue, newValue)

	return nil
}

// === Blacklist Management ===

// BlacklistEntry represents a blacklist entry
type BlacklistEntry struct {
	Wallet        string            `json:"wallet"`
	BlacklistType string            `json:"blacklist_type"`
	Reason        string            `json:"reason"`
	Source        string            `json:"source"`
	OperatorID    string            `json:"operator_id"`
	CreatedAt     int64             `json:"created_at"`
	ExpireAt      int64             `json:"expire_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// BlacklistListResponse response with blacklist entries
type BlacklistListResponse struct {
	Entries  []*BlacklistEntry `json:"entries"`
	Total    int64             `json:"total"`
	Page     int32             `json:"page"`
	PageSize int32             `json:"page_size"`
}

// ListBlacklist retrieves blacklist entries
func (s *RiskService) ListBlacklist(ctx context.Context, blacklistType, source, search string, page, pageSize int32) (*BlacklistListResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	resp, err := s.riskClient.ListBlacklist(ctx, blacklistType, source, search, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list blacklist: %w", err)
	}

	entries := make([]*BlacklistEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		entries = append(entries, &BlacklistEntry{
			Wallet:        e.Wallet,
			BlacklistType: e.BlacklistType,
			Reason:        e.Reason,
			Source:        e.Source,
			OperatorID:    e.OperatorId,
			CreatedAt:     e.CreatedAt,
			ExpireAt:      e.ExpireAt,
			Metadata:      e.Metadata,
		})
	}

	return &BlacklistListResponse{
		Entries:  entries,
		Total:    int64(len(entries)), // Note: actual pagination should come from response
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// CheckBlacklist checks if a wallet is blacklisted
func (s *RiskService) CheckBlacklist(ctx context.Context, wallet string) (*riskv1.CheckBlacklistResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	return s.riskClient.CheckBlacklist(ctx, wallet)
}

// AddToBlacklistRequest request for adding to blacklist
type AddToBlacklistRequest struct {
	Wallet        string            `json:"wallet" binding:"required"`
	BlacklistType string            `json:"blacklist_type" binding:"required,oneof=full withdraw_only trade_only"`
	Reason        string            `json:"reason" binding:"required,max=500"`
	Source        string            `json:"source"` // manual, external
	ExpireAt      int64             `json:"expire_at"` // 0 = permanent
	Metadata      map[string]string `json:"metadata"`
	OperatorID    int64             `json:"-"`
}

// AddToBlacklist adds a wallet to the blacklist
func (s *RiskService) AddToBlacklist(ctx context.Context, req *AddToBlacklistRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	if req.Source == "" {
		req.Source = "manual"
	}

	resp, err := s.riskClient.AddToBlacklist(ctx, &riskv1.AddToBlacklistRequest{
		Wallet:        req.Wallet,
		BlacklistType: req.BlacklistType,
		Reason:        req.Reason,
		Source:        req.Source,
		OperatorId:    strconv.FormatInt(req.OperatorID, 10),
		ExpireAt:      req.ExpireAt,
		Metadata:      req.Metadata,
	})
	if err != nil {
		return fmt.Errorf("failed to add to blacklist: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("add to blacklist failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionCreate, "blacklist",
		req.Wallet, "添加黑名单", nil, model.JSONMap{
			"wallet":         req.Wallet,
			"blacklist_type": req.BlacklistType,
			"reason":         req.Reason,
			"source":         req.Source,
			"expire_at":      req.ExpireAt,
		})

	return nil
}

// RemoveFromBlacklistRequest request for removing from blacklist
type RemoveFromBlacklistRequest struct {
	Wallet     string `json:"wallet" binding:"required"`
	Reason     string `json:"reason" binding:"required,max=500"`
	OperatorID int64  `json:"-"`
}

// RemoveFromBlacklist removes a wallet from the blacklist
func (s *RiskService) RemoveFromBlacklist(ctx context.Context, req *RemoveFromBlacklistRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	// Get current entry for audit
	checkResp, _ := s.riskClient.CheckBlacklist(ctx, req.Wallet)

	resp, err := s.riskClient.RemoveFromBlacklist(ctx, req.Wallet, req.Reason, strconv.FormatInt(req.OperatorID, 10))
	if err != nil {
		return fmt.Errorf("failed to remove from blacklist: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("remove from blacklist failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	oldValue := model.JSONMap{}
	if checkResp != nil && checkResp.IsBlacklisted && checkResp.Entry != nil {
		oldValue = model.JSONMap{
			"blacklist_type": checkResp.Entry.BlacklistType,
			"reason":         checkResp.Entry.Reason,
		}
	}
	s.recordAudit(ctx, req.OperatorID, model.AuditActionDelete, "blacklist",
		req.Wallet, "移除黑名单", oldValue, model.JSONMap{
			"reason": req.Reason,
		})

	return nil
}

// === Risk Events ===

// RiskEventListRequest request for listing risk events
type RiskEventListRequest struct {
	Wallet             string                  `form:"wallet"`
	EventType          commonv1.RiskEventType  `form:"event_type"`
	Level              commonv1.RiskLevel      `form:"level"`
	UnacknowledgedOnly bool                    `form:"unacknowledged_only"`
	StartTime          int64                   `form:"start_time"`
	EndTime            int64                   `form:"end_time"`
	Page               int32                   `form:"page"`
	PageSize           int32                   `form:"page_size"`
}

// ListRiskEvents retrieves risk events with filters
func (s *RiskService) ListRiskEvents(ctx context.Context, req *RiskEventListRequest) (*riskv1.ListRiskEventsResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	return s.riskClient.ListRiskEvents(ctx, &riskv1.ListRiskEventsRequest{
		Wallet:             req.Wallet,
		EventType:          req.EventType,
		Level:              req.Level,
		UnacknowledgedOnly: req.UnacknowledgedOnly,
		StartTime:          req.StartTime,
		EndTime:            req.EndTime,
		Pagination: &commonv1.PaginationRequest{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
	})
}

// GetRiskEvent retrieves a specific risk event
func (s *RiskService) GetRiskEvent(ctx context.Context, eventID string) (*riskv1.RiskEvent, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	return s.riskClient.GetRiskEvent(ctx, eventID)
}

// AcknowledgeRiskEventRequest request for acknowledging a risk event
type AcknowledgeRiskEventRequest struct {
	EventID    string `json:"event_id" binding:"required"`
	Note       string `json:"note" binding:"max=500"`
	OperatorID int64  `json:"-"`
}

// AcknowledgeRiskEvent acknowledges a risk event
func (s *RiskService) AcknowledgeRiskEvent(ctx context.Context, req *AcknowledgeRiskEventRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.AcknowledgeRiskEvent(ctx, req.EventID, strconv.FormatInt(req.OperatorID, 10), req.Note)
	if err != nil {
		return fmt.Errorf("failed to acknowledge risk event: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("acknowledge risk event failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, "risk_event",
		req.EventID, "确认风险事件", nil, model.JSONMap{
			"note": req.Note,
		})

	return nil
}

// === Risk Statistics ===

// GetRiskStats retrieves risk statistics
func (s *RiskService) GetRiskStats(ctx context.Context, periodHours int32) (*riskv1.GetRiskStatsResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	if periodHours < 1 {
		periodHours = 24
	}

	return s.riskClient.GetRiskStats(ctx, periodHours)
}

// recordAudit records an audit log entry
func (s *RiskService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceType, resourceID, description string, oldValue, newValue model.JSONMap) {

	auditLog := &model.AuditLog{
		AdminID:      operatorID,
		Action:       action,
		ResourceType: model.ResourceType(resourceType),
		ResourceID:   resourceID,
		Description:  description,
		OldValue:     oldValue,
		NewValue:     newValue,
		Status:       model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
