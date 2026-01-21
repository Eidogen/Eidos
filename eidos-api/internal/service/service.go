// Package service 提供业务服务层，作为 Handler 和 gRPC Client 之间的适配器
package service

import (
	"context"
	"math"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/client"
	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// ========== Order Service ==========

// OrderService 订单服务适配器
type OrderService struct {
	client      client.TradingClientInterface
	riskService *RiskService
}

// NewOrderService 创建订单服务
func NewOrderService(c client.TradingClientInterface) *OrderService {
	return &OrderService{client: c}
}

// NewOrderServiceWithRisk 创建带风控的订单服务
func NewOrderServiceWithRisk(c client.TradingClientInterface, risk *RiskService) *OrderService {
	return &OrderService{client: c, riskService: risk}
}

// SetRiskService 设置风控服务
func (s *OrderService) SetRiskService(risk *RiskService) {
	s.riskService = risk
}

// PrepareOrder 准备订单（生成签名摘要）
// 调用 eidos-trading 服务获取订单预处理信息，返回 EIP-712 类型化数据供前端签名
func (s *OrderService) PrepareOrder(c *gin.Context, req *dto.PrepareOrderRequest) (*dto.PrepareOrderResponse, error) {
	wallet, exists := c.Get("wallet")
	if !exists {
		return nil, dto.ErrUnauthorized
	}

	prepareReq := &client.PrepareOrderRequest{
		Wallet:      wallet.(string),
		Market:      req.Market,
		Side:        req.Side,
		OrderType:   req.Type,
		Price:       req.Price,
		Amount:      req.Amount,
		TimeInForce: req.TimeInForce,
	}

	resp, err := s.client.PrepareOrder(c.Request.Context(), prepareReq)
	if err != nil {
		return nil, err
	}

	return resp.ToDTO(), nil
}

// CreateOrder 创建订单
func (s *OrderService) CreateOrder(c *gin.Context, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	ctx := c.Request.Context()

	// 风控检查
	if s.riskService != nil {
		riskReq := &CheckOrderRequest{
			Wallet: req.Wallet,
			Market: req.Market,
			Side:   req.Side,
			Type:   req.Type,
			Price:  req.Price,
			Amount: req.Amount,
		}
		result, err := s.riskService.CheckOrder(ctx, riskReq)
		if err != nil {
			return nil, dto.ErrInternalError.WithMessage("risk check failed")
		}
		if !result.Approved {
			return nil, result.ToBizError()
		}
	}

	resp, err := s.client.CreateOrder(ctx, req, req.Wallet)
	if err != nil {
		return nil, err
	}
	// CreateOrderResponse 只返回 order_id 和 status，需要获取完整订单信息
	return &dto.OrderResponse{
		OrderID: resp.OrderID,
		Status:  resp.Status,
	}, nil
}

// GetOrder 获取订单详情
func (s *OrderService) GetOrder(c *gin.Context, orderID string) (*dto.OrderResponse, error) {
	return s.client.GetOrder(c.Request.Context(), orderID, "")
}

// ListOrders 查询订单列表
func (s *OrderService) ListOrders(c *gin.Context, req *dto.ListOrdersRequest) (*dto.PaginatedResponse, error) {
	query := &dto.ListOrdersQuery{
		PaginationQuery: dto.PaginationQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
		TimeRangeQuery: dto.TimeRangeQuery{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Market: req.Market,
		Side:   req.Side,
	}
	if req.Status != "" {
		query.Statuses = []string{req.Status}
	}

	orders, total, err := s.client.ListOrders(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	return &dto.PaginatedResponse{
		Items:      orders,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(req.PageSize))),
	}, nil
}

// ListOpenOrders 查询当前挂单
func (s *OrderService) ListOpenOrders(c *gin.Context, market string) ([]*dto.OrderResponse, error) {
	wallet, _ := c.Get("wallet")
	query := &dto.OpenOrdersQuery{
		Market: market,
	}
	orders, _, err := s.client.ListOpenOrders(c.Request.Context(), wallet.(string), query)
	return orders, err
}

// CancelOrder 取消订单
func (s *OrderService) CancelOrder(c *gin.Context, orderID string) (*dto.OrderResponse, error) {
	wallet, _ := c.Get("wallet")
	err := s.client.CancelOrder(c.Request.Context(), orderID, wallet.(string))
	if err != nil {
		return nil, err
	}
	// 取消成功后返回订单信息
	return s.client.GetOrder(c.Request.Context(), orderID, "")
}

// BatchCancelOrders 批量取消订单
func (s *OrderService) BatchCancelOrders(c *gin.Context, req *dto.BatchCancelRequest) (*dto.BatchCancelResponse, error) {
	var cancelled, failed []string

	// 如果指定了订单 ID 列表
	if len(req.OrderIDs) > 0 {
		for _, orderID := range req.OrderIDs {
			err := s.client.CancelOrder(c.Request.Context(), orderID, req.Wallet)
			if err != nil {
				failed = append(failed, orderID)
			} else {
				cancelled = append(cancelled, orderID)
			}
		}
		return &dto.BatchCancelResponse{
			Cancelled: cancelled,
			Failed:    failed,
		}, nil
	}

	// 按市场/方向批量取消：先获取挂单列表，再逐个取消
	query := &dto.OpenOrdersQuery{
		Market: req.Market,
	}
	orders, _, err := s.client.ListOpenOrders(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		// 过滤方向
		if req.Side != "" && order.Side != req.Side {
			continue
		}
		err := s.client.CancelOrder(c.Request.Context(), order.OrderID, req.Wallet)
		if err != nil {
			failed = append(failed, order.OrderID)
		} else {
			cancelled = append(cancelled, order.OrderID)
		}
	}

	return &dto.BatchCancelResponse{
		Cancelled: cancelled,
		Failed:    failed,
	}, nil
}

// ========== Balance Service ==========

// BalanceService 余额服务适配器
type BalanceService struct {
	client client.TradingClientInterface
}

// NewBalanceService 创建余额服务
func NewBalanceService(c client.TradingClientInterface) *BalanceService {
	return &BalanceService{client: c}
}

// GetBalance 获取单个代币余额
func (s *BalanceService) GetBalance(c *gin.Context, wallet, token string) (*dto.BalanceResponse, error) {
	return s.client.GetBalance(c.Request.Context(), wallet, token)
}

// ListBalances 获取所有余额
func (s *BalanceService) ListBalances(c *gin.Context, wallet string) ([]*dto.BalanceResponse, error) {
	return s.client.GetBalances(c.Request.Context(), wallet)
}

// ListTransactions 查询资金流水
func (s *BalanceService) ListTransactions(c *gin.Context, req *dto.ListTransactionsRequest) (*dto.PaginatedResponse, error) {
	query := &dto.ListTransactionsQuery{
		PaginationQuery: dto.PaginationQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
		TimeRangeQuery: dto.TimeRangeQuery{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Token: req.Token,
		Type:  req.Type,
	}

	logs, total, err := s.client.GetBalanceLogs(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	return &dto.PaginatedResponse{
		Items:      logs,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(req.PageSize))),
	}, nil
}

// ========== Deposit Service ==========

// DepositService 充值服务适配器
type DepositService struct {
	client client.TradingClientInterface
}

// NewDepositService 创建充值服务
func NewDepositService(c client.TradingClientInterface) *DepositService {
	return &DepositService{client: c}
}

// GetDeposit 获取充值详情
func (s *DepositService) GetDeposit(c *gin.Context, depositID string) (*dto.DepositResponse, error) {
	return s.client.GetDeposit(c.Request.Context(), depositID, "")
}

// ListDeposits 查询充值记录
func (s *DepositService) ListDeposits(c *gin.Context, req *dto.ListDepositsRequest) (*dto.PaginatedResponse, error) {
	query := &dto.ListDepositsQuery{
		PaginationQuery: dto.PaginationQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
		TimeRangeQuery: dto.TimeRangeQuery{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Token:  req.Token,
		Status: req.Status,
	}

	deposits, total, err := s.client.ListDeposits(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	return &dto.PaginatedResponse{
		Items:      deposits,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(req.PageSize))),
	}, nil
}

// ========== Withdrawal Service ==========

// WithdrawalService 提现服务适配器
type WithdrawalService struct {
	client      client.TradingClientInterface
	riskService *RiskService
}

// NewWithdrawalService 创建提现服务
func NewWithdrawalService(c client.TradingClientInterface) *WithdrawalService {
	return &WithdrawalService{client: c}
}

// NewWithdrawalServiceWithRisk 创建带风控的提现服务
func NewWithdrawalServiceWithRisk(c client.TradingClientInterface, risk *RiskService) *WithdrawalService {
	return &WithdrawalService{client: c, riskService: risk}
}

// SetRiskService 设置风控服务
func (s *WithdrawalService) SetRiskService(risk *RiskService) {
	s.riskService = risk
}

// CreateWithdrawal 创建提现
func (s *WithdrawalService) CreateWithdrawal(c *gin.Context, req *dto.CreateWithdrawalRequest) (*dto.WithdrawalResponse, error) {
	ctx := c.Request.Context()

	// 风控检查
	if s.riskService != nil {
		riskReq := &CheckWithdrawalRequest{
			Wallet:    req.Wallet,
			Token:     req.Token,
			Amount:    req.Amount,
			ToAddress: req.ToAddress,
		}
		result, err := s.riskService.CheckWithdrawal(ctx, riskReq)
		if err != nil {
			return nil, dto.ErrInternalError.WithMessage("risk check failed")
		}
		if !result.Approved {
			return nil, result.ToBizError()
		}
		// 需要人工审核的情况
		if result.RequireManualReview {
			// 仍然创建提现，但标记为待审核状态
			// 这里继续执行，Trading 服务会处理审核状态
		}
	}

	resp, err := s.client.CreateWithdrawal(ctx, req, req.Wallet)
	if err != nil {
		return nil, err
	}
	return &dto.WithdrawalResponse{
		WithdrawID: resp.WithdrawID,
		Status:     resp.Status,
	}, nil
}

// GetWithdrawal 获取提现详情
func (s *WithdrawalService) GetWithdrawal(c *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	return s.client.GetWithdrawal(c.Request.Context(), withdrawID, "")
}

// ListWithdrawals 查询提现记录
func (s *WithdrawalService) ListWithdrawals(c *gin.Context, req *dto.ListWithdrawalsRequest) (*dto.PaginatedResponse, error) {
	query := &dto.ListWithdrawalsQuery{
		PaginationQuery: dto.PaginationQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
		TimeRangeQuery: dto.TimeRangeQuery{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Token:  req.Token,
		Status: req.Status,
	}

	withdrawals, total, err := s.client.ListWithdrawals(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	return &dto.PaginatedResponse{
		Items:      withdrawals,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(req.PageSize))),
	}, nil
}

// CancelWithdrawal 取消提现
func (s *WithdrawalService) CancelWithdrawal(c *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	wallet, _ := c.Get("wallet")
	err := s.client.CancelWithdrawal(c.Request.Context(), withdrawID, wallet.(string))
	if err != nil {
		return nil, err
	}
	return s.client.GetWithdrawal(c.Request.Context(), withdrawID, "")
}

// ========== Trade Service ==========

// TradeService 成交服务适配器
type TradeService struct {
	client client.TradingClientInterface
}

// NewTradeService 创建成交服务
func NewTradeService(c client.TradingClientInterface) *TradeService {
	return &TradeService{client: c}
}

// GetTrade 获取成交详情
func (s *TradeService) GetTrade(c *gin.Context, tradeID string) (*dto.TradeResponse, error) {
	return s.client.GetTrade(c.Request.Context(), tradeID, "")
}

// ListTrades 查询成交记录
func (s *TradeService) ListTrades(c *gin.Context, req *dto.ListTradesRequest) (*dto.PaginatedResponse, error) {
	// 如果指定了 OrderID，使用 ListTradesByOrder 接口
	if req.OrderID != "" {
		trades, total, err := s.client.ListTradesByOrder(c.Request.Context(), req.OrderID)
		if err != nil {
			return nil, err
		}
		return &dto.PaginatedResponse{
			Items:      trades,
			Total:      total,
			Page:       1,
			PageSize:   int(total),
			TotalPages: 1,
		}, nil
	}

	// 否则使用通用的 ListTrades 接口
	query := &dto.ListTradesQuery{
		PaginationQuery: dto.PaginationQuery{
			Page:     req.Page,
			PageSize: req.PageSize,
		},
		TimeRangeQuery: dto.TimeRangeQuery{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Market: req.Market,
	}

	trades, total, err := s.client.ListTrades(c.Request.Context(), req.Wallet, query)
	if err != nil {
		return nil, err
	}

	return &dto.PaginatedResponse{
		Items:      trades,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(req.PageSize))),
	}, nil
}

// ========== Market Service ==========

// MarketService 行情服务适配器
type MarketService struct {
	client client.MarketClientInterface
}

// NewMarketService 创建行情服务
func NewMarketService(c client.MarketClientInterface) *MarketService {
	return &MarketService{client: c}
}

// ListMarkets 获取交易对列表
func (s *MarketService) ListMarkets(c *gin.Context) ([]*dto.MarketResponse, error) {
	return s.client.ListMarkets(c.Request.Context())
}

// GetTicker 获取单个 Ticker
func (s *MarketService) GetTicker(c *gin.Context, market string) (*dto.TickerResponse, error) {
	return s.client.GetTicker(c.Request.Context(), market)
}

// ListTickers 获取所有 Ticker
func (s *MarketService) ListTickers(c *gin.Context) ([]*dto.TickerResponse, error) {
	return s.client.ListTickers(c.Request.Context())
}

// GetDepth 获取深度数据
func (s *MarketService) GetDepth(c *gin.Context, market string, limit int) (*dto.DepthResponse, error) {
	return s.client.GetDepth(c.Request.Context(), market, limit)
}

// GetKlines 获取 K 线数据
func (s *MarketService) GetKlines(c *gin.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error) {
	return s.client.GetKlines(c.Request.Context(), market, interval, startTime, endTime, limit)
}

// GetRecentTrades 获取最近成交
func (s *MarketService) GetRecentTrades(c *gin.Context, market string, limit int) ([]*dto.RecentTradeResponse, error) {
	return s.client.GetRecentTrades(c.Request.Context(), market, limit)
}

// ========== Health Check Adapter ==========

// TradingHealthAdapter 适配 TradingClient 健康检查接口
type TradingHealthAdapter struct {
	client client.TradingClientInterface
}

// NewTradingHealthAdapter 创建健康检查适配器
func NewTradingHealthAdapter(c client.TradingClientInterface) *TradingHealthAdapter {
	return &TradingHealthAdapter{client: c}
}

// Ping 检查 Trading 服务连接
func (a *TradingHealthAdapter) Ping() error {
	return a.client.Ping(context.Background())
}

// MarketHealthAdapter 适配 MarketClient 健康检查接口
type MarketHealthAdapter struct {
	client client.MarketClientInterface
}

// NewMarketHealthAdapter 创建 Market 健康检查适配器
func NewMarketHealthAdapter(c client.MarketClientInterface) *MarketHealthAdapter {
	return &MarketHealthAdapter{client: c}
}

// Ping 检查 Market 服务连接
func (a *MarketHealthAdapter) Ping() error {
	return a.client.Ping(context.Background())
}
