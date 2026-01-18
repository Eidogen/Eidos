// Package handler 提供风控服务的 gRPC 处理器
package handler

import (
	"context"

	commonv1 "github.com/eidos-exchange/eidos/proto/common/v1"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RiskHandler 风控服务 gRPC 处理器
type RiskHandler struct {
	riskv1.UnimplementedRiskServiceServer

	riskSvc      *service.RiskService
	blacklistSvc *service.BlacklistService
	ruleSvc      *service.RuleService
	eventSvc     *service.EventService
}

// NewRiskHandler 创建风控处理器
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

// CheckOrder 检查下单请求
func (h *RiskHandler) CheckOrder(ctx context.Context, req *riskv1.CheckOrderRequest) (*riskv1.CheckOrderResponse, error) {
	// 参数验证
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	// 解析价格和数量
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid price format")
	}
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// 转换订单方向和类型
	side := "BUY"
	if req.Side == commonv1.OrderSide_ORDER_SIDE_SELL {
		side = "SELL"
	}
	orderType := "LIMIT"
	if req.OrderType == commonv1.OrderType_ORDER_TYPE_MARKET {
		orderType = "MARKET"
	}

	// 调用服务层
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

	return &riskv1.CheckOrderResponse{
		Approved:     resp.Approved,
		RejectReason: resp.RejectReason,
		RejectCode:   resp.RejectCode,
		Warnings:     resp.Warnings,
	}, nil
}

// CheckWithdraw 检查提现请求
func (h *RiskHandler) CheckWithdraw(ctx context.Context, req *riskv1.CheckWithdrawRequest) (*riskv1.CheckWithdrawResponse, error) {
	// 参数验证
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	if req.ToAddress == "" {
		return nil, status.Error(codes.InvalidArgument, "to_address is required")
	}

	// 解析金额
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid amount format")
	}

	// 调用服务层
	resp, err := h.riskSvc.CheckWithdraw(ctx, &service.CheckWithdrawRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    amount,
		ToAddress: req.ToAddress,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &riskv1.CheckWithdrawResponse{
		Approved:            resp.Approved,
		RejectReason:        resp.RejectReason,
		RejectCode:          resp.RejectCode,
		RequireManualReview: resp.RequireManualReview,
	}, nil
}

// AddToBlacklist 添加到黑名单
func (h *RiskHandler) AddToBlacklist(ctx context.Context, req *riskv1.AddToBlacklistRequest) (*riskv1.AddToBlacklistResponse, error) {
	// 参数验证
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	if req.OperatorId == "" {
		return nil, status.Error(codes.InvalidArgument, "operator_id is required")
	}

	err := h.blacklistSvc.AddToBlacklist(ctx, &service.AddToBlacklistRequest{
		Wallet:     req.Wallet,
		ListType:   "full", // 默认全部限制
		Reason:     req.Reason,
		Source:     "manual",
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

// RemoveFromBlacklist 从黑名单移除
func (h *RiskHandler) RemoveFromBlacklist(ctx context.Context, req *riskv1.RemoveFromBlacklistRequest) (*riskv1.RemoveFromBlacklistResponse, error) {
	// 参数验证
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

// CheckBlacklist 检查黑名单
func (h *RiskHandler) CheckBlacklist(ctx context.Context, req *riskv1.CheckBlacklistRequest) (*riskv1.CheckBlacklistResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	resp, err := h.blacklistSvc.CheckBlacklist(ctx, req.Wallet)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &riskv1.CheckBlacklistResponse{
		IsBlacklisted: resp.IsBlacklisted,
		Reason:        resp.Reason,
		ExpireAt:      resp.ExpireAt,
	}, nil
}

// ListBlacklist 获取黑名单列表
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

	// 转换为 proto 格式
	pbEntries := make([]*riskv1.BlacklistEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = &riskv1.BlacklistEntry{
			Wallet:     e.WalletAddress,
			Reason:     e.Reason,
			OperatorId: e.CreatedBy,
			CreatedAt:  e.CreatedAt,
			ExpireAt:   e.EffectiveUntil,
		}
	}

	totalPages := int32(total) / int32(pageSize)
	if int32(total)%int32(pageSize) > 0 {
		totalPages++
	}

	return &riskv1.ListBlacklistResponse{
		Entries: pbEntries,
		Pagination: &commonv1.PaginationResponse{
			Total:      int32(total),
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalPages: totalPages,
		},
	}, nil
}

// ListRiskRules 获取风控规则列表
func (h *RiskHandler) ListRiskRules(ctx context.Context, req *riskv1.ListRiskRulesRequest) (*riskv1.ListRiskRulesResponse, error) {
	rules, err := h.ruleSvc.ListRules(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbRules := make([]*riskv1.RiskRule, len(rules))
	for i, r := range rules {
		pbRules[i] = &riskv1.RiskRule{
			RuleId:      r.RuleID,
			Name:        r.Name,
			Description: r.Description,
			Category:    r.Type.String(),
			IsEnabled:   r.IsActive(),
			Params:      make(map[string]string),
			UpdatedAt:   r.UpdatedAt,
		}
	}

	return &riskv1.ListRiskRulesResponse{
		Rules: pbRules,
	}, nil
}

// UpdateRiskRule 更新风控规则
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

// ListRiskEvents 获取风控事件列表
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

	events, total, err := h.eventSvc.ListEvents(ctx, &service.ListEventsRequest{
		Wallet:    req.Wallet,
		EventType: req.EventType,
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
			EventType:   e.Type.String(),
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
			Total:      int32(total),
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalPages: totalPages,
		},
	}, nil
}

// GetUserLimits 获取用户限额信息
func (h *RiskHandler) GetUserLimits(ctx context.Context, req *riskv1.GetUserLimitsRequest) (*riskv1.GetUserLimitsResponse, error) {
	if req.Wallet == "" {
		return nil, status.Error(codes.InvalidArgument, "wallet is required")
	}

	limits, err := h.riskSvc.GetUserLimits(ctx, req.Wallet)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	pbLimits := make([]*riskv1.Limit, len(limits.Limits))
	for i, l := range limits.Limits {
		pbLimits[i] = &riskv1.Limit{
			LimitType:      l.LimitType,
			Token:          l.Token,
			MaxValue:       l.MaxValue.String(),
			UsedValue:      l.UsedValue.String(),
			RemainingValue: l.RemainingValue.String(),
			ResetAt:        l.ResetAt,
		}
	}

	return &riskv1.GetUserLimitsResponse{
		Wallet: limits.Wallet,
		Limits: pbLimits,
	}, nil
}
