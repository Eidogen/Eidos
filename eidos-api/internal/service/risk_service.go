// Package service provides business service layer
package service

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-api/internal/client"
	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// RiskService 风控服务
type RiskService struct {
	client  client.RiskClientInterface
	enabled bool
}

// NewRiskService 创建风控服务
func NewRiskService(c client.RiskClientInterface, enabled bool) *RiskService {
	return &RiskService{
		client:  c,
		enabled: enabled,
	}
}

// CheckOrder 检查订单风控
// 如果风控服务不可用或禁用，返回允许通过
func (s *RiskService) CheckOrder(ctx context.Context, req *CheckOrderRequest) (*CheckOrderResult, error) {
	if !s.enabled || s.client == nil {
		return &CheckOrderResult{Approved: true}, nil
	}

	checkReq := &client.CheckOrderRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      req.Side,
		OrderType: req.Type,
		Price:     req.Price,
		Amount:    req.Amount,
	}

	resp, err := s.client.CheckOrder(ctx, checkReq)
	if err != nil {
		// 风控服务错误时，根据策略决定是否放行
		// 这里采用保守策略：服务错误时拒绝
		return &CheckOrderResult{
			Approved:     false,
			RejectReason: "risk service unavailable",
			RejectCode:   "RISK_SERVICE_ERROR",
		}, nil
	}

	return &CheckOrderResult{
		Approved:     resp.Approved,
		RejectReason: resp.RejectReason,
		RejectCode:   resp.RejectCode,
		Warnings:     resp.Warnings,
	}, nil
}

// CheckWithdrawal 检查提现风控
func (s *RiskService) CheckWithdrawal(ctx context.Context, req *CheckWithdrawalRequest) (*CheckWithdrawalResult, error) {
	if !s.enabled || s.client == nil {
		return &CheckWithdrawalResult{Approved: true}, nil
	}

	checkReq := &client.CheckWithdrawRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    req.Amount,
		ToAddress: req.ToAddress,
	}

	resp, err := s.client.CheckWithdraw(ctx, checkReq)
	if err != nil {
		return &CheckWithdrawalResult{
			Approved:     false,
			RejectReason: "risk service unavailable",
			RejectCode:   "RISK_SERVICE_ERROR",
		}, nil
	}

	return &CheckWithdrawalResult{
		Approved:            resp.Approved,
		RejectReason:        resp.RejectReason,
		RejectCode:          resp.RejectCode,
		RequireManualReview: resp.RequireManualReview,
	}, nil
}

// CheckBlacklist 检查黑名单
func (s *RiskService) CheckBlacklist(ctx context.Context, wallet string) (*CheckBlacklistResult, error) {
	if !s.enabled || s.client == nil {
		return &CheckBlacklistResult{IsBlacklisted: false}, nil
	}

	resp, err := s.client.CheckBlacklist(ctx, wallet)
	if err != nil {
		// 黑名单检查失败时保守处理：不阻止
		return &CheckBlacklistResult{IsBlacklisted: false}, nil
	}

	return &CheckBlacklistResult{
		IsBlacklisted: resp.IsBlacklisted,
		Reason:        resp.Reason,
		ExpireAt:      resp.ExpireAt,
	}, nil
}

// CheckOrderRequest 订单检查请求
type CheckOrderRequest struct {
	Wallet string
	Market string
	Side   string
	Type   string
	Price  string
	Amount string
}

// CheckOrderResult 订单检查结果
type CheckOrderResult struct {
	Approved     bool
	RejectReason string
	RejectCode   string
	Warnings     []string
}

// ToBizError 转换为业务错误
func (r *CheckOrderResult) ToBizError() *dto.BizError {
	if r.Approved {
		return nil
	}

	switch r.RejectCode {
	case "INSUFFICIENT_BALANCE":
		return dto.ErrInsufficientBalance
	case "PRICE_TOO_HIGH":
		return dto.ErrPriceTooHigh
	case "PRICE_TOO_LOW":
		return dto.ErrPriceTooLow
	case "AMOUNT_TOO_SMALL":
		return dto.ErrAmountTooSmall
	case "AMOUNT_TOO_LARGE":
		return dto.ErrAmountTooLarge
	case "NOTIONAL_TOO_SMALL":
		return dto.ErrNotionalTooSmall
	case "MARKET_SUSPENDED":
		return dto.ErrMarketSuspended
	case "BLACKLISTED":
		return dto.ErrForbidden.WithMessage("account is blacklisted")
	case "RISK_SERVICE_ERROR":
		return dto.ErrServiceUnavailable.WithMessage("risk service unavailable")
	default:
		if r.RejectReason != "" {
			return dto.NewBizError(15001, r.RejectReason, 400)
		}
		return dto.NewBizError(15000, "order rejected by risk control", 400)
	}
}

// CheckWithdrawalRequest 提现检查请求
type CheckWithdrawalRequest struct {
	Wallet    string
	Token     string
	Amount    string
	ToAddress string
}

// CheckWithdrawalResult 提现检查结果
type CheckWithdrawalResult struct {
	Approved            bool
	RejectReason        string
	RejectCode          string
	RequireManualReview bool
}

// ToBizError 转换为业务错误
func (r *CheckWithdrawalResult) ToBizError() *dto.BizError {
	if r.Approved {
		return nil
	}

	switch r.RejectCode {
	case "INSUFFICIENT_BALANCE":
		return dto.ErrInsufficientBalance
	case "WITHDRAW_LIMIT_EXCEEDED":
		return dto.ErrWithdrawLimitExceeded
	case "WITHDRAW_AMOUNT_TOO_SMALL":
		return dto.ErrWithdrawAmountTooSmall
	case "INVALID_ADDRESS":
		return dto.ErrInvalidWithdrawAddress
	case "TOKEN_NOT_SUPPORTED":
		return dto.ErrTokenNotSupported
	case "BLACKLISTED":
		return dto.ErrForbidden.WithMessage("account is blacklisted")
	case "RISK_SERVICE_ERROR":
		return dto.ErrServiceUnavailable.WithMessage("risk service unavailable")
	case "MANUAL_REVIEW_REQUIRED":
		return dto.NewBizError(15002, "withdrawal requires manual review", 202)
	default:
		if r.RejectReason != "" {
			return dto.NewBizError(15003, r.RejectReason, 400)
		}
		return dto.NewBizError(15000, "withdrawal rejected by risk control", 400)
	}
}

// CheckBlacklistResult 黑名单检查结果
type CheckBlacklistResult struct {
	IsBlacklisted bool
	Reason        string
	ExpireAt      int64
}
