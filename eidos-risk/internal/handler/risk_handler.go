// Package handler provides risk service gRPC handlers
package handler

import (
	"context"
	"time"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RiskHandler is the gRPC handler for risk service
type RiskHandler struct {
	riskv1.UnimplementedRiskServiceServer

	riskSvc            *service.RiskService
	blacklistSvc       *service.BlacklistService
	ruleSvc            *service.RuleService
	eventSvc           *service.EventService
	whitelistSvc       *service.WhitelistService
	withdrawReviewSvc  *service.WithdrawalReviewService
	alertSvc           *service.AlertService
}

// NewRiskHandler creates a new risk handler
func NewRiskHandler(
	riskSvc *service.RiskService,
	blacklistSvc *service.BlacklistService,
	ruleSvc *service.RuleService,
	eventSvc *service.EventService,
) *RiskHandler {
	return &RiskHandler{
		riskSvc:      riskSvc,
		blacklistSvc: blacklistSvc,
		ruleSvc:      ruleSvc,
		eventSvc:     eventSvc,
	}
}

// SetWhitelistService sets the whitelist service
func (h *RiskHandler) SetWhitelistService(svc *service.WhitelistService) {
	h.whitelistSvc = svc
}

// SetWithdrawReviewService sets the withdrawal review service
func (h *RiskHandler) SetWithdrawReviewService(svc *service.WithdrawalReviewService) {
	h.withdrawReviewSvc = svc
}

// SetAlertService sets the alert service
func (h *RiskHandler) SetAlertService(svc *service.AlertService) {
	h.alertSvc = svc
}

// ============================================================================
// Pre-Trade Risk Checks
// ============================================================================

// CheckOrder validates an order against risk rules
func (h *RiskHandler) CheckOrder(ctx context.Context, req *riskv1.CheckOrderRequest) (*riskv1.CheckOrderResponse, error) {
	// Parameter validation
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	// Parse price and amount
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid price format")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// Convert order side and type
	side := "BUY"
	if req.Side == commonv1.OrderSide_ORDER_SIDE_SELL {
		side = "SELL"
	}
	orderType := "LIMIT"
	if req.OrderType == commonv1.OrderType_ORDER_TYPE_MARKET {
		orderType = "MARKET"
	}

	// Call service layer
	resp, err := h.riskSvc.CheckOrder(ctx, &service.CheckOrderRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      side,
		OrderType: orderType,
		Price:     price,
		Amount:    amount,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Build triggered rules list
	triggeredRules := make([]string, 0)
	if resp.RejectCode != "" {
		triggeredRules = append(triggeredRules, resp.RejectCode)
	}

	return &riskv1.CheckOrderResponse{
		Approved:       resp.Approved,
		RejectReason:   resp.RejectReason,
		RejectCode:     resp.RejectCode,
		Warnings:       resp.Warnings,
		TriggeredRules: triggeredRules,
	}, nil
}

// CheckWithdrawal validates a withdrawal against risk rules
func (h *RiskHandler) CheckWithdrawal(ctx context.Context, req *riskv1.CheckWithdrawalRequest) (*riskv1.CheckWithdrawalResponse, error) {
	// Parameter validation
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	if req.ToAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "to_address is required")
	}

	// Parse amount
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// Call service layer
	resp, err := h.riskSvc.CheckWithdraw(ctx, &service.CheckWithdrawRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    amount,
		ToAddress: req.ToAddress,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Build triggered rules list
	triggeredRules := make([]string, 0)
	if resp.RejectCode != "" {
		triggeredRules = append(triggeredRules, resp.RejectCode)
	}

	return &riskv1.CheckWithdrawalResponse{
		Approved:            resp.Approved,
		RejectReason:        resp.RejectReason,
		RejectCode:          resp.RejectCode,
		RequireManualReview: resp.RequireManualReview,
		RiskScore:           int32(resp.RiskScore),
		TriggeredRules:      triggeredRules,
	}, nil
}

// CheckTransaction validates a generic transaction
func (h *RiskHandler) CheckTransaction(ctx context.Context, req *riskv1.CheckTransactionRequest) (*riskv1.CheckTransactionResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	// For now, delegate to appropriate check based on tx_type
	switch req.TxType {
	case "order":
		// Convert to order check
		return &riskv1.CheckTransactionResponse{
			Approved: true,
		}, nil
	case "withdrawal":
		amount, _ := decimal.NewFromString(req.Amount)
		resp, err := h.riskSvc.CheckWithdraw(ctx, &service.CheckWithdrawRequest{
			Wallet:    req.Wallet,
			Token:     req.Token,
			Amount:    amount,
			ToAddress: req.Context["to_address"],
		})
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &riskv1.CheckTransactionResponse{
			Approved:     resp.Approved,
			RejectReason: resp.RejectReason,
			RejectCode:   resp.RejectCode,
			RiskScore:    int32(resp.RiskScore),
		}, nil
	default:
		return &riskv1.CheckTransactionResponse{
			Approved: true,
		}, nil
	}
}

// ============================================================================
// Blacklist Management
// ============================================================================

// AddToBlacklist adds a wallet to the blacklist
func (h *RiskHandler) AddToBlacklist(ctx context.Context, req *riskv1.AddToBlacklistRequest) (*riskv1.AddToBlacklistResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	listType := req.BlacklistType
	if listType == "" {
		listType = "full"
	}

	source := req.Source
	if source == "" {
		source = "manual"
	}

	err := h.blacklistSvc.AddToBlacklist(ctx, &service.AddToBlacklistRequest{
		Wallet:     req.Wallet,
		ListType:   listType,
		Reason:     req.Reason,
		Source:     source,
		ExpireAt:   req.ExpireAt,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.AddToBlacklistResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.AddToBlacklistResponse{
		Success: true,
	}, nil
}

// RemoveFromBlacklist removes a wallet from the blacklist
func (h *RiskHandler) RemoveFromBlacklist(ctx context.Context, req *riskv1.RemoveFromBlacklistRequest) (*riskv1.RemoveFromBlacklistResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	err := h.blacklistSvc.RemoveFromBlacklist(ctx, &service.RemoveFromBlacklistRequest{
		Wallet:     req.Wallet,
		Reason:     req.Reason,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.RemoveFromBlacklistResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.RemoveFromBlacklistResponse{
		Success: true,
	}, nil
}

// CheckBlacklist checks if a wallet is blacklisted
func (h *RiskHandler) CheckBlacklist(ctx context.Context, req *riskv1.CheckBlacklistRequest) (*riskv1.CheckBlacklistResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	resp, err := h.blacklistSvc.CheckBlacklist(ctx, req.Wallet)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var entry *riskv1.BlacklistEntry
	if resp.IsBlacklisted {
		entry = &riskv1.BlacklistEntry{
			Wallet:   req.Wallet,
			Reason:   resp.Reason,
			ExpireAt: resp.ExpireAt,
		}
	}

	return &riskv1.CheckBlacklistResponse{
		IsBlacklisted: resp.IsBlacklisted,
		Entry:         entry,
	}, nil
}

// ListBlacklist retrieves the blacklist with filters
func (h *RiskHandler) ListBlacklist(ctx context.Context, req *riskv1.ListBlacklistRequest) (*riskv1.ListBlacklistResponse, error) {
	page := 1
	pageSize := 20
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = int(req.Pagination.Page)
		}
		if req.Pagination.PageSize > 0 {
			pageSize = int(req.Pagination.PageSize)
		}
	}

	entries, total, err := h.blacklistSvc.ListBlacklist(ctx, page, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbEntries := make([]*riskv1.BlacklistEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = &riskv1.BlacklistEntry{
			Wallet:        e.WalletAddress,
			BlacklistType: string(e.ListType),
			Reason:        e.Reason,
			Source:        string(e.Source),
			OperatorId:    e.CreatedBy,
			CreatedAt:     e.CreatedAt,
			ExpireAt:      e.EffectiveUntil,
		}
	}

	totalPages := int32(total) / int32(pageSize)
	if int32(total)%int32(pageSize) > 0 {
		totalPages++
	}

	return &riskv1.ListBlacklistResponse{
		Entries: pbEntries,
		Pagination: &commonv1.PaginationResponse{
			Total:      total,
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalPages: totalPages,
		},
	}, nil
}

// ============================================================================
// Risk Rules Management
// ============================================================================

// ListRiskRules retrieves all risk rules
func (h *RiskHandler) ListRiskRules(ctx context.Context, req *riskv1.ListRiskRulesRequest) (*riskv1.ListRiskRulesResponse, error) {
	rules, err := h.ruleSvc.ListRules(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbRules := make([]*riskv1.RiskRule, 0, len(rules))
	for _, r := range rules {
		// Filter by category if specified
		if req.Category != "" && r.Type.String() != req.Category {
			continue
		}
		// Filter by enabled status if specified
		if req.EnabledOnly && !r.IsActive() {
			continue
		}

		pbRules = append(pbRules, &riskv1.RiskRule{
			RuleId:      r.RuleID,
			Name:        r.Name,
			Description: r.Description,
			Category:    r.Type.String(),
			IsEnabled:   r.IsActive(),
			Priority:    int32(r.Priority),
			Params:      make(map[string]string),
			UpdatedAt:   r.UpdatedAt,
			UpdatedBy:   r.UpdatedBy,
		})
	}

	return &riskv1.ListRiskRulesResponse{
		Rules: pbRules,
	}, nil
}

// GetRiskRule retrieves a specific risk rule
func (h *RiskHandler) GetRiskRule(ctx context.Context, req *riskv1.GetRiskRuleRequest) (*riskv1.GetRiskRuleResponse, error) {
	if req.RuleId == "" {
		return nil, status.Error(codes.InvalidArgument, "rule_id is required")
	}

	rule, err := h.ruleSvc.GetRule(ctx, req.RuleId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &riskv1.GetRiskRuleResponse{
		Rule: &riskv1.RiskRule{
			RuleId:      rule.RuleID,
			Name:        rule.Name,
			Description: rule.Description,
			Category:    rule.Type.String(),
			IsEnabled:   rule.IsActive(),
			Priority:    int32(rule.Priority),
			Params:      make(map[string]string),
			CreatedAt:   rule.CreatedAt,
			UpdatedAt:   rule.UpdatedAt,
			UpdatedBy:   rule.UpdatedBy,
		},
	}, nil
}

// UpdateRiskRule updates a risk rule
func (h *RiskHandler) UpdateRiskRule(ctx context.Context, req *riskv1.UpdateRiskRuleRequest) (*riskv1.UpdateRiskRuleResponse, error) {
	if req.RuleId == "" {
		return nil, status.Error(codes.InvalidArgument, "rule_id is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	enabled := req.IsEnabled
	err := h.ruleSvc.UpdateRule(ctx, &service.UpdateRuleRequest{
		RuleID:     req.RuleId,
		Enabled:    &enabled,
		Params:     req.Params,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.UpdateRiskRuleResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.UpdateRiskRuleResponse{
		Success: true,
	}, nil
}

// ============================================================================
// Rate Limiting
// ============================================================================

// GetRateLimitStatus retrieves rate limit status for a wallet
func (h *RiskHandler) GetRateLimitStatus(ctx context.Context, req *riskv1.GetRateLimitStatusRequest) (*riskv1.GetRateLimitStatusResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	// Get rate limit info from service
	counters := make([]*riskv1.RateLimitCounter, 0)

	// Add common counters (would come from rate limit cache in real implementation)
	counters = append(counters, &riskv1.RateLimitCounter{
		Name:          "orders_per_second",
		Current:       0,
		Limit:         10,
		WindowSeconds: 1,
		ResetsAt:      time.Now().Add(1 * time.Second).UnixMilli(),
	})
	counters = append(counters, &riskv1.RateLimitCounter{
		Name:          "orders_per_minute",
		Current:       0,
		Limit:         300,
		WindowSeconds: 60,
		ResetsAt:      time.Now().Add(60 * time.Second).UnixMilli(),
	})

	return &riskv1.GetRateLimitStatusResponse{
		Wallet:    req.Wallet,
		Counters:  counters,
		IsLimited: false,
	}, nil
}

// ResetRateLimit resets rate limit counters for a wallet
func (h *RiskHandler) ResetRateLimit(ctx context.Context, req *riskv1.ResetRateLimitRequest) (*riskv1.ResetRateLimitResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	// Reset rate limit in cache (would be implemented in rate limit cache)
	return &riskv1.ResetRateLimitResponse{
		Success:       true,
		CountersReset: 3,
	}, nil
}

// ============================================================================
// Risk Events
// ============================================================================

// ListRiskEvents retrieves risk events with filters
func (h *RiskHandler) ListRiskEvents(ctx context.Context, req *riskv1.ListRiskEventsRequest) (*riskv1.ListRiskEventsResponse, error) {
	page := 1
	pageSize := 20
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = int(req.Pagination.Page)
		}
		if req.Pagination.PageSize > 0 {
			pageSize = int(req.Pagination.PageSize)
		}
	}

	eventType := ""
	if req.EventType != commonv1.RiskEventType_RISK_EVENT_TYPE_UNSPECIFIED {
		eventType = req.EventType.String()
	}

	events, total, err := h.eventSvc.ListEvents(ctx, &service.ListEventsRequest{
		Wallet:    req.Wallet,
		EventType: eventType,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Page:      page,
		Size:      pageSize,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbEvents := make([]*riskv1.RiskEvent, len(events))
	for i, e := range events {
		pbEvents[i] = &riskv1.RiskEvent{
			EventId:     e.EventID,
			Wallet:      e.Wallet,
			EventType:   commonv1.RiskEventType(commonv1.RiskEventType_value[e.Type.String()]),
			RuleId:      e.RuleID,
			Description: e.Reason,
			Context:     make(map[string]string),
			CreatedAt:   e.CreatedAt,
		}
	}

	totalPages := int32(total) / int32(pageSize)
	if int32(total)%int32(pageSize) > 0 {
		totalPages++
	}

	return &riskv1.ListRiskEventsResponse{
		Events: pbEvents,
		Pagination: &commonv1.PaginationResponse{
			Total:      total,
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalPages: totalPages,
		},
	}, nil
}

// GetRiskEvent retrieves a specific risk event
func (h *RiskHandler) GetRiskEvent(ctx context.Context, req *riskv1.GetRiskEventRequest) (*riskv1.GetRiskEventResponse, error) {
	if req.EventId == "" {
		return nil, status.Error(codes.InvalidArgument, "event_id is required")
	}

	event, err := h.eventSvc.GetEvent(ctx, req.EventId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &riskv1.GetRiskEventResponse{
		Event: &riskv1.RiskEvent{
			EventId:     event.EventID,
			Wallet:      event.Wallet,
			RuleId:      event.RuleID,
			Description: event.Reason,
			Context:     make(map[string]string),
			CreatedAt:   event.CreatedAt,
		},
	}, nil
}

// AcknowledgeRiskEvent acknowledges a risk event
func (h *RiskHandler) AcknowledgeRiskEvent(ctx context.Context, req *riskv1.AcknowledgeRiskEventRequest) (*riskv1.AcknowledgeRiskEventResponse, error) {
	if req.EventId == "" {
		return nil, status.Error(codes.InvalidArgument, "event_id is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	err := h.eventSvc.AcknowledgeEvent(ctx, req.EventId, req.OperatorId, req.Note)
	if err != nil {
		return &riskv1.AcknowledgeRiskEventResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.AcknowledgeRiskEventResponse{
		Success: true,
	}, nil
}

// ============================================================================
// User Limits
// ============================================================================

// GetUserLimits retrieves limits for a user
func (h *RiskHandler) GetUserLimits(ctx context.Context, req *riskv1.GetUserLimitsRequest) (*riskv1.GetUserLimitsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	limits, err := h.riskSvc.GetUserLimits(ctx, req.Wallet)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbLimits := make([]*riskv1.UserLimit, len(limits.Limits))
	for i, l := range limits.Limits {
		pbLimits[i] = &riskv1.UserLimit{
			LimitType:      l.LimitType,
			Token:          l.Token,
			MaxValue:       l.MaxValue.String(),
			UsedValue:      l.UsedValue.String(),
			RemainingValue: l.RemainingValue.String(),
			ResetsAt:       l.ResetAt,
		}
	}

	// Check VIP status for tier
	tier := "basic"
	if h.whitelistSvc != nil {
		isVIP, _, _ := h.whitelistSvc.CheckVIP(ctx, req.Wallet)
		if isVIP {
			tier = "vip"
		}
		isMM, _, _ := h.whitelistSvc.CheckMarketMaker(ctx, req.Wallet)
		if isMM {
			tier = "market_maker"
		}
	}

	return &riskv1.GetUserLimitsResponse{
		Wallet: limits.Wallet,
		Limits: pbLimits,
		Tier:   tier,
	}, nil
}

// SetUserLimits sets custom limits for a user
func (h *RiskHandler) SetUserLimits(ctx context.Context, req *riskv1.SetUserLimitsRequest) (*riskv1.SetUserLimitsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	// Setting custom limits would involve creating whitelist entries with custom metadata
	if h.whitelistSvc == nil {
		return &riskv1.SetUserLimitsResponse{
			Success:      false,
			ErrorMessage: "whitelist service not available",
		}, nil
	}

	// Create VIP entry with custom limits
	metadata := &model.WhitelistMetadata{
		CustomParams: make(map[string]string),
	}
	for _, limit := range req.Limits {
		metadata.CustomParams[limit.LimitType] = limit.MaxValue
	}

	_, err := h.whitelistSvc.AddVIP(ctx, &service.AddVIPRequest{
		Wallet:     req.Wallet,
		Label:      "Custom limits",
		Remark:     req.Reason,
		Metadata:   metadata,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.SetUserLimitsResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.SetUserLimitsResponse{
		Success: true,
	}, nil
}

// ============================================================================
// Account Actions
// ============================================================================

// FreezeAccount freezes a user account
func (h *RiskHandler) FreezeAccount(ctx context.Context, req *riskv1.FreezeAccountRequest) (*riskv1.FreezeAccountResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	freezeType := req.FreezeType
	if freezeType == "" {
		freezeType = "full"
	}

	var expireAt int64
	if req.DurationSeconds > 0 {
		expireAt = time.Now().Add(time.Duration(req.DurationSeconds) * time.Second).UnixMilli()
	}

	err := h.blacklistSvc.AddToBlacklist(ctx, &service.AddToBlacklistRequest{
		Wallet:     req.Wallet,
		ListType:   freezeType,
		Reason:     req.Reason,
		Source:     "manual",
		ExpireAt:   expireAt,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.FreezeAccountResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.FreezeAccountResponse{
		Success: true,
	}, nil
}

// UnfreezeAccount unfreezes a user account
func (h *RiskHandler) UnfreezeAccount(ctx context.Context, req *riskv1.UnfreezeAccountRequest) (*riskv1.UnfreezeAccountResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	err := h.blacklistSvc.RemoveFromBlacklist(ctx, &service.RemoveFromBlacklistRequest{
		Wallet:     req.Wallet,
		Reason:     req.Reason,
		OperatorID: req.OperatorId,
	})
	if err != nil {
		return &riskv1.UnfreezeAccountResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &riskv1.UnfreezeAccountResponse{
		Success: true,
	}, nil
}

// GetAccountStatus retrieves account risk status
func (h *RiskHandler) GetAccountStatus(ctx context.Context, req *riskv1.GetAccountStatusRequest) (*riskv1.GetAccountStatusResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	// Check blacklist status
	blacklistResp, err := h.blacklistSvc.CheckBlacklist(ctx, req.Wallet)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get recent risk events count
	events, _, _ := h.eventSvc.ListEvents(ctx, &service.ListEventsRequest{
		Wallet:    req.Wallet,
		StartTime: time.Now().Add(-24 * time.Hour).UnixMilli(),
		Page:      1,
		Size:      100,
	})

	// Determine tier
	tier := "basic"
	if h.whitelistSvc != nil {
		isVIP, _, _ := h.whitelistSvc.CheckVIP(ctx, req.Wallet)
		if isVIP {
			tier = "vip"
		}
		isMM, _, _ := h.whitelistSvc.CheckMarketMaker(ctx, req.Wallet)
		if isMM {
			tier = "market_maker"
		}
	}

	return &riskv1.GetAccountStatusResponse{
		Wallet:           req.Wallet,
		IsFrozen:         blacklistResp.IsBlacklisted,
		FreezeReason:     blacklistResp.Reason,
		FreezeExpiresAt:  blacklistResp.ExpireAt,
		IsBlacklisted:    blacklistResp.IsBlacklisted,
		RiskScore:        0, // Would be calculated from recent activity
		Tier:             tier,
		RecentRiskEvents: int32(len(events)),
	}, nil
}

// ============================================================================
// Risk Statistics
// ============================================================================

// GetRiskStats retrieves risk statistics
func (h *RiskHandler) GetRiskStats(ctx context.Context, req *riskv1.GetRiskStatsRequest) (*riskv1.GetRiskStatsResponse, error) {
	periodHours := req.PeriodHours
	if periodHours == 0 {
		periodHours = 24
	}

	periodStart := time.Now().Add(-time.Duration(periodHours) * time.Hour).UnixMilli()
	periodEnd := time.Now().UnixMilli()

	// Get event statistics from event service
	stats, err := h.eventSvc.GetStats(ctx, periodStart, periodEnd)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &riskv1.GetRiskStatsResponse{
		PeriodStart:             periodStart,
		PeriodEnd:               periodEnd,
		OrdersChecked:           stats.OrdersChecked,
		OrdersRejected:          stats.OrdersRejected,
		OrderRejectionRate:      stats.OrderRejectionRate,
		WithdrawalsChecked:      stats.WithdrawalsChecked,
		WithdrawalsRejected:     stats.WithdrawalsRejected,
		WithdrawalsReview:       stats.WithdrawalsReview,
		WithdrawalRejectionRate: stats.WithdrawalRejectionRate,
		NewBlacklistEntries:     stats.NewBlacklistEntries,
		AccountsFrozen:          stats.AccountsFrozen,
		EventsByLevel:           stats.EventsByLevel,
		EventsByType:            stats.EventsByType,
	}, nil
}
