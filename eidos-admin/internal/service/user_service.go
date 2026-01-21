package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/client"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// UserService provides user management functionality
type UserService struct {
	tradingClient *client.TradingClient
	riskClient    *client.RiskClient
	auditRepo     *repository.AuditLogRepository
}

// NewUserService creates a new user service
func NewUserService(
	tradingClient *client.TradingClient,
	riskClient *client.RiskClient,
	auditRepo *repository.AuditLogRepository,
) *UserService {
	return &UserService{
		tradingClient: tradingClient,
		riskClient:    riskClient,
		auditRepo:     auditRepo,
	}
}

// UserInfo represents user information with risk status
type UserInfo struct {
	Wallet           string   `json:"wallet"`
	IsFrozen         bool     `json:"is_frozen"`
	FreezeType       string   `json:"freeze_type,omitempty"`
	FreezeReason     string   `json:"freeze_reason,omitempty"`
	FreezeExpiresAt  int64    `json:"freeze_expires_at,omitempty"`
	IsBlacklisted    bool     `json:"is_blacklisted"`
	RiskScore        int32    `json:"risk_score"`
	Tier             string   `json:"tier"`
	TotalVolume      string   `json:"total_volume"`
	CreatedAt        int64    `json:"created_at"`
	LastActivityAt   int64    `json:"last_activity_at"`
	RecentRiskEvents int32    `json:"recent_risk_events"`
	Balances         []Balance `json:"balances,omitempty"`
}

// Balance represents a token balance
type Balance struct {
	Token             string `json:"token"`
	SettledAvailable  string `json:"settled_available"`
	SettledFrozen     string `json:"settled_frozen"`
	PendingAvailable  string `json:"pending_available"`
	PendingFrozen     string `json:"pending_frozen"`
	TotalAvailable    string `json:"total_available"`
	Total             string `json:"total"`
	Withdrawable      string `json:"withdrawable"`
	PendingDeposit    string `json:"pending_deposit"`
	PendingWithdrawal string `json:"pending_withdrawal"`
}

// GetUser retrieves user information by wallet address
func (s *UserService) GetUser(ctx context.Context, wallet string) (*UserInfo, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}

	// Get account status from risk service
	status, err := s.riskClient.GetAccountStatus(ctx, wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to get account status: %w", err)
	}

	user := &UserInfo{
		Wallet:           status.Wallet,
		IsFrozen:         status.IsFrozen,
		FreezeType:       status.FreezeType,
		FreezeReason:     status.FreezeReason,
		FreezeExpiresAt:  status.FreezeExpiresAt,
		IsBlacklisted:    status.IsBlacklisted,
		RiskScore:        status.RiskScore,
		Tier:             status.Tier,
		TotalVolume:      status.TotalVolume,
		CreatedAt:        status.CreatedAt,
		LastActivityAt:   status.LastActivityAt,
		RecentRiskEvents: status.RecentRiskEvents,
	}

	// Get balances if trading client available
	if s.tradingClient != nil {
		balances, err := s.tradingClient.GetBalances(ctx, wallet, false)
		if err == nil && balances != nil {
			user.Balances = make([]Balance, 0, len(balances.Balances))
			for _, b := range balances.Balances {
				user.Balances = append(user.Balances, Balance{
					Token:             b.Token,
					SettledAvailable:  b.SettledAvailable,
					SettledFrozen:     b.SettledFrozen,
					PendingAvailable:  b.PendingAvailable,
					PendingFrozen:     b.PendingFrozen,
					TotalAvailable:    b.TotalAvailable,
					Total:             b.Total,
					Withdrawable:      b.Withdrawable,
					PendingDeposit:    b.PendingDeposit,
					PendingWithdrawal: b.PendingWithdrawal,
				})
			}
		}
	}

	return user, nil
}

// GetUserBalances retrieves user balances
func (s *UserService) GetUserBalances(ctx context.Context, wallet string, hideZero bool) ([]Balance, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.GetBalances(ctx, wallet, hideZero)
	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}

	balances := make([]Balance, 0, len(resp.Balances))
	for _, b := range resp.Balances {
		balances = append(balances, Balance{
			Token:             b.Token,
			SettledAvailable:  b.SettledAvailable,
			SettledFrozen:     b.SettledFrozen,
			PendingAvailable:  b.PendingAvailable,
			PendingFrozen:     b.PendingFrozen,
			TotalAvailable:    b.TotalAvailable,
			Total:             b.Total,
			Withdrawable:      b.Withdrawable,
			PendingDeposit:    b.PendingDeposit,
			PendingWithdrawal: b.PendingWithdrawal,
		})
	}

	return balances, nil
}

// FreezeAccountRequest request to freeze an account
type FreezeAccountRequest struct {
	Wallet          string `json:"wallet" binding:"required"`
	FreezeType      string `json:"freeze_type" binding:"required,oneof=full trading withdrawal"`
	Reason          string `json:"reason" binding:"required,max=500"`
	DurationSeconds int64  `json:"duration_seconds"` // 0 = indefinite
	OperatorID      int64  `json:"-"`
}

// FreezeAccount freezes a user account
func (s *UserService) FreezeAccount(ctx context.Context, req *FreezeAccountRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.FreezeAccount(
		ctx,
		req.Wallet,
		req.FreezeType,
		req.Reason,
		strconv.FormatInt(req.OperatorID, 10),
		req.DurationSeconds,
	)
	if err != nil {
		return fmt.Errorf("failed to freeze account: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("freeze account failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionStatusChange, model.ResourceTypeUser,
		req.Wallet, "冻结用户账户", nil, model.JSONMap{
			"freeze_type": req.FreezeType,
			"reason":      req.Reason,
			"duration":    req.DurationSeconds,
		})

	return nil
}

// UnfreezeAccountRequest request to unfreeze an account
type UnfreezeAccountRequest struct {
	Wallet     string `json:"wallet" binding:"required"`
	Reason     string `json:"reason" binding:"required,max=500"`
	OperatorID int64  `json:"-"`
}

// UnfreezeAccount unfreezes a user account
func (s *UserService) UnfreezeAccount(ctx context.Context, req *UnfreezeAccountRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.UnfreezeAccount(
		ctx,
		req.Wallet,
		req.Reason,
		strconv.FormatInt(req.OperatorID, 10),
	)
	if err != nil {
		return fmt.Errorf("failed to unfreeze account: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("unfreeze account failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionStatusChange, model.ResourceTypeUser,
		req.Wallet, "解冻用户账户", nil, model.JSONMap{
			"reason": req.Reason,
		})

	return nil
}

// GetUserLimits retrieves limits for a user
func (s *UserService) GetUserLimits(ctx context.Context, wallet string) (*riskv1.GetUserLimitsResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}
	return s.riskClient.GetUserLimits(ctx, wallet)
}

// SetUserLimitsRequest request to set user limits
type SetUserLimitsRequest struct {
	Wallet     string            `json:"wallet" binding:"required"`
	Limits     []*riskv1.UserLimit `json:"limits" binding:"required"`
	Reason     string            `json:"reason" binding:"required,max=500"`
	OperatorID int64             `json:"-"`
}

// SetUserLimits sets custom limits for a user
func (s *UserService) SetUserLimits(ctx context.Context, req *SetUserLimitsRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.SetUserLimits(ctx, &riskv1.SetUserLimitsRequest{
		Wallet:     req.Wallet,
		Limits:     req.Limits,
		OperatorId: strconv.FormatInt(req.OperatorID, 10),
		Reason:     req.Reason,
	})
	if err != nil {
		return fmt.Errorf("failed to set user limits: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("set user limits failed: %s", resp.ErrorMessage)
	}

	// Record audit log
	limitsMap := make([]map[string]interface{}, 0, len(req.Limits))
	for _, l := range req.Limits {
		limitsMap = append(limitsMap, map[string]interface{}{
			"limit_type": l.LimitType,
			"token":      l.Token,
			"max_value":  l.MaxValue,
		})
	}
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, model.ResourceTypeUser,
		req.Wallet, "设置用户限额", nil, model.JSONMap{
			"limits": limitsMap,
			"reason": req.Reason,
		})

	return nil
}

// GetRateLimitStatus retrieves rate limit status for a user
func (s *UserService) GetRateLimitStatus(ctx context.Context, wallet string) (*riskv1.GetRateLimitStatusResponse, error) {
	if s.riskClient == nil {
		return nil, fmt.Errorf("risk client not available")
	}
	return s.riskClient.GetRateLimitStatus(ctx, wallet, "")
}

// ResetRateLimitRequest request to reset rate limits
type ResetRateLimitRequest struct {
	Wallet      string `json:"wallet" binding:"required"`
	CounterName string `json:"counter_name"` // empty = all
	Reason      string `json:"reason" binding:"required,max=500"`
	OperatorID  int64  `json:"-"`
}

// ResetRateLimit resets rate limit counters for a user
func (s *UserService) ResetRateLimit(ctx context.Context, req *ResetRateLimitRequest) error {
	if s.riskClient == nil {
		return fmt.Errorf("risk client not available")
	}

	resp, err := s.riskClient.ResetRateLimit(
		ctx,
		req.Wallet,
		req.CounterName,
		strconv.FormatInt(req.OperatorID, 10),
		req.Reason,
	)
	if err != nil {
		return fmt.Errorf("failed to reset rate limit: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("reset rate limit failed")
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, model.ResourceTypeUser,
		req.Wallet, "重置用户限流", nil, model.JSONMap{
			"counter_name":   req.CounterName,
			"reason":         req.Reason,
			"counters_reset": resp.CountersReset,
		})

	return nil
}

// UserOrdersResponse response with user orders
type UserOrdersResponse struct {
	Orders []*tradingv1.Order `json:"orders"`
	Total  int64              `json:"total"`
	Page   int32              `json:"page"`
	PageSize int32            `json:"page_size"`
}

// GetUserOrders retrieves orders for a user
func (s *UserService) GetUserOrders(ctx context.Context, wallet, market string, page, pageSize int32) (*UserOrdersResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.ListOrders(ctx, &tradingv1.ListOrdersRequest{
		Wallet:   wallet,
		Market:   market,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}

	return &UserOrdersResponse{
		Orders:   resp.Orders,
		Total:    resp.Total,
		Page:     resp.Page,
		PageSize: resp.PageSize,
	}, nil
}

// UserTradesResponse response with user trades
type UserTradesResponse struct {
	Trades   []*tradingv1.Trade `json:"trades"`
	Total    int64              `json:"total"`
	Page     int32              `json:"page"`
	PageSize int32              `json:"page_size"`
}

// GetUserTrades retrieves trades for a user
func (s *UserService) GetUserTrades(ctx context.Context, wallet, market string, page, pageSize int32) (*UserTradesResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.ListTrades(ctx, &tradingv1.ListTradesRequest{
		Wallet:   wallet,
		Market:   market,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	return &UserTradesResponse{
		Trades:   resp.Trades,
		Total:    resp.Total,
		Page:     resp.Page,
		PageSize: resp.PageSize,
	}, nil
}

// GetUserDeposits retrieves deposits for a user
func (s *UserService) GetUserDeposits(ctx context.Context, wallet, token string, page, pageSize int32) (*tradingv1.ListDepositsResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	return s.tradingClient.ListDeposits(ctx, &tradingv1.ListDepositsRequest{
		Wallet:   wallet,
		Token:    token,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetUserWithdrawals retrieves withdrawals for a user
func (s *UserService) GetUserWithdrawals(ctx context.Context, wallet, token string, page, pageSize int32) (*tradingv1.ListWithdrawalsResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	return s.tradingClient.ListWithdrawals(ctx, &tradingv1.ListWithdrawalsRequest{
		Wallet:   wallet,
		Token:    token,
		Page:     page,
		PageSize: pageSize,
	})
}

// recordAudit records an audit log entry
func (s *UserService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceType model.ResourceType, resourceID, description string, oldValue, newValue model.JSONMap) {

	auditLog := &model.AuditLog{
		AdminID:      operatorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Description:  description,
		OldValue:     oldValue,
		NewValue:     newValue,
		Status:       model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
