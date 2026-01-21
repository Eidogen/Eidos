package service

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/client"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	chainv1 "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// WithdrawalService provides withdrawal management functionality
type WithdrawalService struct {
	tradingClient *client.TradingClient
	chainClient   *client.ChainClient
	auditRepo     *repository.AuditLogRepository
}

// NewWithdrawalService creates a new withdrawal service
func NewWithdrawalService(
	tradingClient *client.TradingClient,
	chainClient *client.ChainClient,
	auditRepo *repository.AuditLogRepository,
) *WithdrawalService {
	return &WithdrawalService{
		tradingClient: tradingClient,
		chainClient:   chainClient,
		auditRepo:     auditRepo,
	}
}

// WithdrawalDetail represents detailed withdrawal information
type WithdrawalDetail struct {
	// From trading service
	WithdrawID   string `json:"withdraw_id"`
	Wallet       string `json:"wallet"`
	Token        string `json:"token"`
	Amount       string `json:"amount"`
	Fee          string `json:"fee"`
	NetAmount    string `json:"net_amount"`
	ToAddress    string `json:"to_address"`
	Status       string `json:"status"`
	TxHash       string `json:"tx_hash,omitempty"`
	BlockNum     int64  `json:"block_num,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
	RequestedAt  int64  `json:"requested_at"`
	SubmittedAt  int64  `json:"submitted_at,omitempty"`
	ConfirmedAt  int64  `json:"confirmed_at,omitempty"`

	// From chain service
	GasUsed  int64  `json:"gas_used,omitempty"`
	GasPrice string `json:"gas_price,omitempty"`
}

// GetWithdrawal retrieves withdrawal details
func (s *WithdrawalService) GetWithdrawal(ctx context.Context, withdrawID string) (*WithdrawalDetail, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	withdrawal, err := s.tradingClient.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawal: %w", err)
	}

	detail := &WithdrawalDetail{
		WithdrawID:   withdrawal.WithdrawId,
		Wallet:       withdrawal.Wallet,
		Token:        withdrawal.Token,
		Amount:       withdrawal.Amount,
		Fee:          withdrawal.Fee,
		NetAmount:    withdrawal.NetAmount,
		ToAddress:    withdrawal.ToAddress,
		Status:       withdrawal.Status.String(),
		TxHash:       withdrawal.TxHash,
		BlockNum:     withdrawal.BlockNum,
		RejectReason: withdrawal.RejectReason,
		RequestedAt:  withdrawal.RequestedAt,
		SubmittedAt:  withdrawal.SubmittedAt,
		ConfirmedAt:  withdrawal.ConfirmedAt,
	}

	// Get chain status if available
	if s.chainClient != nil && withdrawal.TxHash != "" {
		chainStatus, err := s.chainClient.GetWithdrawalStatus(ctx, withdrawID)
		if err == nil && chainStatus != nil {
			detail.GasUsed = chainStatus.GasUsed
			detail.GasPrice = chainStatus.GasPrice
		}
	}

	return detail, nil
}

// WithdrawalListRequest request for listing withdrawals
type WithdrawalListRequest struct {
	Wallet    string                   `form:"wallet"`
	Token     string                   `form:"token"`
	Status    commonv1.WithdrawStatus  `form:"status"`
	StartTime int64                    `form:"start_time"`
	EndTime   int64                    `form:"end_time"`
	Page      int32                    `form:"page"`
	PageSize  int32                    `form:"page_size"`
}

// WithdrawalListResponse response with withdrawals
type WithdrawalListResponse struct {
	Withdrawals []*tradingv1.Withdrawal `json:"withdrawals"`
	Total       int64                   `json:"total"`
	Page        int32                   `json:"page"`
	PageSize    int32                   `json:"page_size"`
}

// ListWithdrawals retrieves withdrawals with filters
func (s *WithdrawalService) ListWithdrawals(ctx context.Context, req *WithdrawalListRequest) (*WithdrawalListResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	resp, err := s.tradingClient.ListWithdrawals(ctx, &tradingv1.ListWithdrawalsRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Status:    req.Status,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list withdrawals: %w", err)
	}

	return &WithdrawalListResponse{
		Withdrawals: resp.Withdrawals,
		Total:       resp.Total,
		Page:        resp.Page,
		PageSize:    resp.PageSize,
	}, nil
}

// ListPendingWithdrawals retrieves pending withdrawals from chain service
func (s *WithdrawalService) ListPendingWithdrawals(ctx context.Context, wallet, token string, page, pageSize int32) (*chainv1.ListPendingWithdrawalsResponse, error) {
	if s.chainClient == nil {
		return nil, fmt.Errorf("chain client not available")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	return s.chainClient.ListPendingWithdrawals(ctx, wallet, token, page, pageSize)
}

// RejectWithdrawalRequest request for rejecting a withdrawal
type RejectWithdrawalRequest struct {
	WithdrawID  string `json:"withdraw_id" binding:"required"`
	Reason      string `json:"reason" binding:"required,max=500"`
	OperatorID  int64  `json:"-"`
}

// RejectWithdrawal rejects a pending withdrawal (cancels it)
func (s *WithdrawalService) RejectWithdrawal(ctx context.Context, req *RejectWithdrawalRequest) error {
	if s.tradingClient == nil {
		return fmt.Errorf("trading client not available")
	}

	// Get withdrawal details first
	withdrawal, err := s.tradingClient.GetWithdrawal(ctx, req.WithdrawID)
	if err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}

	// Cancel the withdrawal
	if err := s.tradingClient.CancelWithdrawal(ctx, withdrawal.Wallet, req.WithdrawID); err != nil {
		return fmt.Errorf("failed to cancel withdrawal: %w", err)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionStatusChange, "withdrawal",
		req.WithdrawID, "拒绝提现申请", model.JSONMap{
			"wallet":    withdrawal.Wallet,
			"token":     withdrawal.Token,
			"amount":    withdrawal.Amount,
			"to_address": withdrawal.ToAddress,
			"status":    withdrawal.Status.String(),
		}, model.JSONMap{
			"reason": req.Reason,
		})

	return nil
}

// RetryWithdrawalRequest request for retrying a failed withdrawal
type RetryWithdrawalRequest struct {
	WithdrawID     string `json:"withdraw_id" binding:"required"`
	GasBumpPercent int32  `json:"gas_bump_percent"` // 0-100, 0 = no bump
	OperatorID     int64  `json:"-"`
}

// RetryWithdrawalResponse response for retry
type RetryWithdrawalResponse struct {
	Success   bool   `json:"success"`
	NewTxHash string `json:"new_tx_hash,omitempty"`
	Message   string `json:"message"`
}

// RetryWithdrawal retries a failed withdrawal
func (s *WithdrawalService) RetryWithdrawal(ctx context.Context, req *RetryWithdrawalRequest) (*RetryWithdrawalResponse, error) {
	if s.chainClient == nil {
		return nil, fmt.Errorf("chain client not available")
	}

	resp, err := s.chainClient.RetryWithdrawal(ctx, req.WithdrawID, req.GasBumpPercent)
	if err != nil {
		return nil, fmt.Errorf("failed to retry withdrawal: %w", err)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, "withdrawal",
		req.WithdrawID, "重试提现交易", nil, model.JSONMap{
			"gas_bump_percent": req.GasBumpPercent,
			"success":          resp.Success,
			"new_tx_hash":      resp.NewTxHash,
			"message":          resp.Message,
		})

	return &RetryWithdrawalResponse{
		Success:   resp.Success,
		NewTxHash: resp.NewTxHash,
		Message:   resp.Message,
	}, nil
}

// DepositListRequest request for listing deposits
type DepositListRequest struct {
	Wallet    string                  `form:"wallet"`
	Token     string                  `form:"token"`
	Status    commonv1.DepositStatus  `form:"status"`
	StartTime int64                   `form:"start_time"`
	EndTime   int64                   `form:"end_time"`
	Page      int32                   `form:"page"`
	PageSize  int32                   `form:"page_size"`
}

// DepositListResponse response with deposits
type DepositListResponse struct {
	Deposits []*tradingv1.Deposit `json:"deposits"`
	Total    int64                `json:"total"`
	Page     int32                `json:"page"`
	PageSize int32                `json:"page_size"`
}

// ListDeposits retrieves deposits with filters
func (s *WithdrawalService) ListDeposits(ctx context.Context, req *DepositListRequest) (*DepositListResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	resp, err := s.tradingClient.ListDeposits(ctx, &tradingv1.ListDepositsRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Status:    req.Status,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deposits: %w", err)
	}

	return &DepositListResponse{
		Deposits: resp.Deposits,
		Total:    resp.Total,
		Page:     resp.Page,
		PageSize: resp.PageSize,
	}, nil
}

// GetDeposit retrieves deposit details
func (s *WithdrawalService) GetDeposit(ctx context.Context, depositID string) (*tradingv1.Deposit, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	return s.tradingClient.GetDeposit(ctx, depositID)
}

// recordAudit records an audit log entry
func (s *WithdrawalService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
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
