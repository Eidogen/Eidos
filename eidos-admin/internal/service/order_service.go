package service

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/client"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// OrderService provides order management functionality
type OrderService struct {
	tradingClient  *client.TradingClient
	matchingClient *client.MatchingClient
	auditRepo      *repository.AuditLogRepository
}

// NewOrderService creates a new order service
func NewOrderService(
	tradingClient *client.TradingClient,
	matchingClient *client.MatchingClient,
	auditRepo *repository.AuditLogRepository,
) *OrderService {
	return &OrderService{
		tradingClient:  tradingClient,
		matchingClient: matchingClient,
		auditRepo:      auditRepo,
	}
}

// OrderDetail represents detailed order information
type OrderDetail struct {
	*tradingv1.Order
	Trades []*tradingv1.Trade `json:"trades,omitempty"`
}

// GetOrder retrieves order details by ID
func (s *OrderService) GetOrder(ctx context.Context, orderID string) (*OrderDetail, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	order, err := s.tradingClient.GetOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	detail := &OrderDetail{Order: order}

	// Get trades for the order
	if order.TradeCount > 0 {
		trades, err := s.tradingClient.ListTradesByOrder(ctx, orderID)
		if err == nil && trades != nil {
			detail.Trades = trades.Trades
		}
	}

	return detail, nil
}

// OrderListRequest request for listing orders
type OrderListRequest struct {
	Wallet    string                      `form:"wallet"`
	Market    string                      `form:"market"`
	Side      commonv1.OrderSide          `form:"side"`
	Type      commonv1.OrderType          `form:"type"`
	Statuses  []commonv1.OrderStatus      `form:"statuses"`
	StartTime int64                       `form:"start_time"`
	EndTime   int64                       `form:"end_time"`
	Page      int32                       `form:"page"`
	PageSize  int32                       `form:"page_size"`
}

// OrderListResponse response with orders
type OrderListResponse struct {
	Orders   []*tradingv1.Order `json:"orders"`
	Total    int64              `json:"total"`
	Page     int32              `json:"page"`
	PageSize int32              `json:"page_size"`
}

// ListOrders retrieves orders with filters
func (s *OrderService) ListOrders(ctx context.Context, req *OrderListRequest) (*OrderListResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	resp, err := s.tradingClient.ListOrders(ctx, &tradingv1.ListOrdersRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      req.Side,
		Type:      req.Type,
		Statuses:  req.Statuses,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list orders: %w", err)
	}

	return &OrderListResponse{
		Orders:   resp.Orders,
		Total:    resp.Total,
		Page:     resp.Page,
		PageSize: resp.PageSize,
	}, nil
}

// ListOpenOrders retrieves active orders
func (s *OrderService) ListOpenOrders(ctx context.Context, wallet, market string) ([]*tradingv1.Order, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.ListOpenOrders(ctx, wallet, market)
	if err != nil {
		return nil, fmt.Errorf("failed to list open orders: %w", err)
	}

	return resp.Orders, nil
}

// CancelOrderRequest request for cancelling an order
type CancelOrderRequest struct {
	OrderID    string `json:"order_id" binding:"required"`
	Reason     string `json:"reason" binding:"required,max=500"`
	OperatorID int64  `json:"-"`
}

// CancelOrder cancels an order by admin
func (s *OrderService) CancelOrder(ctx context.Context, req *CancelOrderRequest) error {
	if s.tradingClient == nil {
		return fmt.Errorf("trading client not available")
	}

	// Get order details first
	order, err := s.tradingClient.GetOrder(ctx, req.OrderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}

	// Cancel the order
	if err := s.tradingClient.AdminCancelOrder(ctx, req.OrderID); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionForceCancel, model.ResourceTypeOrder,
		req.OrderID, "管理员取消订单", model.JSONMap{
			"wallet":           order.Wallet,
			"market":           order.Market,
			"side":             order.Side.String(),
			"price":            order.Price,
			"amount":           order.Amount,
			"remaining_amount": order.RemainingAmount,
			"status":           order.Status.String(),
		}, model.JSONMap{
			"reason": req.Reason,
		})

	return nil
}

// BatchCancelOrdersRequest request for batch cancelling orders
type BatchCancelOrdersRequest struct {
	Wallet     string             `json:"wallet" binding:"required"`
	Market     string             `json:"market"` // empty = all markets
	Side       commonv1.OrderSide `json:"side"`   // 0 = both sides
	OrderIDs   []string           `json:"order_ids"` // specific orders to cancel
	Reason     string             `json:"reason" binding:"required,max=500"`
	OperatorID int64              `json:"-"`
}

// BatchCancelOrdersResponse response for batch cancel
type BatchCancelOrdersResponse struct {
	CancelledCount int32                         `json:"cancelled_count"`
	FailedCount    int32                         `json:"failed_count"`
	Failures       []*tradingv1.CancelFailure    `json:"failures,omitempty"`
}

// BatchCancelOrders cancels multiple orders
func (s *OrderService) BatchCancelOrders(ctx context.Context, req *BatchCancelOrdersRequest) (*BatchCancelOrdersResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.BatchCancelOrders(ctx, &tradingv1.BatchCancelOrdersRequest{
		Wallet:   req.Wallet,
		Market:   req.Market,
		Side:     req.Side,
		OrderIds: req.OrderIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to batch cancel orders: %w", err)
	}

	// Record audit log
	s.recordAudit(ctx, req.OperatorID, model.AuditActionForceCancel, model.ResourceTypeOrder,
		req.Wallet, "管理员批量取消订单", nil, model.JSONMap{
			"wallet":          req.Wallet,
			"market":          req.Market,
			"side":            req.Side.String(),
			"order_ids":       req.OrderIDs,
			"reason":          req.Reason,
			"cancelled_count": resp.CancelledCount,
			"failed_count":    resp.FailedCount,
		})

	return &BatchCancelOrdersResponse{
		CancelledCount: resp.CancelledCount,
		FailedCount:    resp.FailedCount,
		Failures:       resp.Failures,
	}, nil
}

// TradeListRequest request for listing trades
type TradeListRequest struct {
	Wallet     string `form:"wallet"`
	Market     string `form:"market"`
	StartTime  int64  `form:"start_time"`
	EndTime    int64  `form:"end_time"`
	Page       int32  `form:"page"`
	PageSize   int32  `form:"page_size"`
}

// TradeListResponse response with trades
type TradeListResponse struct {
	Trades   []*tradingv1.Trade `json:"trades"`
	Total    int64              `json:"total"`
	Page     int32              `json:"page"`
	PageSize int32              `json:"page_size"`
}

// ListTrades retrieves trades with filters
func (s *OrderService) ListTrades(ctx context.Context, req *TradeListRequest) (*TradeListResponse, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	resp, err := s.tradingClient.ListTrades(ctx, &tradingv1.ListTradesRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list trades: %w", err)
	}

	return &TradeListResponse{
		Trades:   resp.Trades,
		Total:    resp.Total,
		Page:     resp.Page,
		PageSize: resp.PageSize,
	}, nil
}

// GetTrade retrieves trade details
func (s *OrderService) GetTrade(ctx context.Context, tradeID string) (*tradingv1.Trade, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	return s.tradingClient.GetTrade(ctx, tradeID)
}

// GetTradesByOrder retrieves trades for a specific order
func (s *OrderService) GetTradesByOrder(ctx context.Context, orderID string) ([]*tradingv1.Trade, error) {
	if s.tradingClient == nil {
		return nil, fmt.Errorf("trading client not available")
	}

	resp, err := s.tradingClient.ListTradesByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	return resp.Trades, nil
}

// recordAudit records an audit log entry
func (s *OrderService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
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
