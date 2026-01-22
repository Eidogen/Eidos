// Package client 提供 gRPC 客户端封装
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	commonGrpc "github.com/eidos-exchange/eidos/eidos-common/pkg/grpc"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultTimeout 默认请求超时
	DefaultTimeout = 5 * time.Second
)

// TradingClientInterface 定义 Trading 客户端接口（用于测试 mock）
type TradingClientInterface interface {
	Ping(ctx context.Context) error
	PrepareOrder(ctx context.Context, req *PrepareOrderRequest) (*PrepareOrderResponse, error)
	CreateOrder(ctx context.Context, req *dto.CreateOrderRequest, wallet string) (*dto.CreateOrderResponse, error)
	CancelOrder(ctx context.Context, orderID, wallet string) error
	GetOrder(ctx context.Context, orderID, wallet string) (*dto.OrderResponse, error)
	ListOrders(ctx context.Context, wallet string, query *dto.ListOrdersQuery) ([]*dto.OrderResponse, int64, error)
	ListOpenOrders(ctx context.Context, wallet string, query *dto.OpenOrdersQuery) ([]*dto.OrderResponse, int64, error)
	GetBalance(ctx context.Context, wallet, token string) (*dto.BalanceResponse, error)
	GetBalances(ctx context.Context, wallet string) ([]*dto.BalanceResponse, error)
	GetBalanceLogs(ctx context.Context, wallet string, query *dto.ListTransactionsQuery) ([]*dto.TransactionResponse, int64, error)
	GetTrade(ctx context.Context, tradeID, wallet string) (*dto.TradeResponse, error)
	ListTrades(ctx context.Context, wallet string, query *dto.ListTradesQuery) ([]*dto.TradeResponse, int64, error)
	ListTradesByOrder(ctx context.Context, orderID string) ([]*dto.TradeResponse, int64, error)
	GetDeposit(ctx context.Context, depositID, wallet string) (*dto.DepositResponse, error)
	ListDeposits(ctx context.Context, wallet string, query *dto.ListDepositsQuery) ([]*dto.DepositResponse, int64, error)
	CreateWithdrawal(ctx context.Context, req *dto.CreateWithdrawalRequest, wallet string) (*dto.CreateWithdrawalResponse, error)
	CancelWithdrawal(ctx context.Context, withdrawID, wallet string) error
	GetWithdrawal(ctx context.Context, withdrawID, wallet string) (*dto.WithdrawalResponse, error)
	ListWithdrawals(ctx context.Context, wallet string, query *dto.ListWithdrawalsQuery) ([]*dto.WithdrawalResponse, int64, error)
}

// 确保 TradingClient 实现 TradingClientInterface
var _ TradingClientInterface = (*TradingClient)(nil)

// TradingClient Trading 服务 gRPC 客户端
type TradingClient struct {
	conn   *grpc.ClientConn
	client pb.TradingServiceClient
}

// NewTradingClient 创建 Trading 客户端（使用现有连接）
func NewTradingClient(conn *grpc.ClientConn) *TradingClient {
	return &TradingClient{
		conn:   conn,
		client: pb.NewTradingServiceClient(conn),
	}
}

// NewTradingClientWithTarget 创建 Trading 客户端（自动创建连接）
// 使用企业级默认配置：keepalive、负载均衡、拦截器（tracing/metrics/logging）
func NewTradingClientWithTarget(target string, enableTracing bool, opts ...grpc.DialOption) (*TradingClient, error) {
	// 获取企业级默认选项
	defaultOpts := commonGrpc.DefaultDialOptions("eidos-api", enableTracing)
	opts = append(defaultOpts, opts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial trading service: %w", err)
	}

	return &TradingClient{
		conn:   conn,
		client: pb.NewTradingServiceClient(conn),
	}, nil
}

// Close 关闭连接
func (c *TradingClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping 检查连接状态
func (c *TradingClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 使用 GetBalance 作为健康检查（使用空钱包地址）
	// 如果 Trading 服务有专门的健康检查方法，应该优先使用
	_, err := c.client.GetBalances(ctx, &pb.GetBalancesRequest{
		Wallet: "0x0000000000000000000000000000000000000000",
	})

	// 忽略业务错误（如 NotFound），只检查连接错误
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			return err
		}
		// 连接相关的错误
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			return err
		}
	}
	return nil
}

// ========== 订单接口 ==========

// CreateOrder 创建订单
func (c *TradingClient) CreateOrder(ctx context.Context, req *dto.CreateOrderRequest, wallet string) (*dto.CreateOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	pbReq := &pb.CreateOrderRequest{
		Wallet:        wallet,
		Market:        req.Market,
		Side:          convertOrderSide(req.Side),
		Type:          convertOrderType(req.Type),
		Price:         req.Price,
		Amount:        req.Amount,
		TimeInForce:   convertTimeInForce(req.TimeInForce),
		Nonce:         req.Nonce,
		Signature:     []byte(req.Signature),
		ClientOrderId: req.ClientOrderID,
		ExpireAt:      req.ExpireAt,
	}

	resp, err := c.client.CreateOrder(ctx, pbReq)
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return &dto.CreateOrderResponse{
		OrderID: resp.Order.OrderId,
		Status:  convertOrderStatusToString(resp.Order.Status),
	}, nil
}

// CancelOrder 取消订单
func (c *TradingClient) CancelOrder(ctx context.Context, orderID, wallet string) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	_, err := c.client.CancelOrder(ctx, &pb.CancelOrderRequest{
		OrderId: orderID,
		Wallet:  wallet,
	})
	if err != nil {
		return convertGRPCError(err)
	}

	return nil
}

// GetOrder 获取订单详情
// Note: wallet 参数保留用于未来权限校验，当前 proto 只需要 orderID
func (c *TradingClient) GetOrder(ctx context.Context, orderID, _ string) (*dto.OrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	order, err := c.client.GetOrder(ctx, &pb.GetOrderRequest{
		OrderId: orderID,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return convertProtoOrder(order), nil
}

// ListOrders 获取订单列表
func (c *TradingClient) ListOrders(ctx context.Context, wallet string, query *dto.ListOrdersQuery) ([]*dto.OrderResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.ListOrdersRequest{
		Wallet:    wallet,
		Market:    query.Market,
		Side:      convertOrderSide(query.Side),
		Page:      int32(query.Page),
		PageSize:  int32(query.PageSize),
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	for _, s := range query.Statuses {
		req.Statuses = append(req.Statuses, convertOrderStatus(s))
	}

	resp, err := c.client.ListOrders(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	orders := make([]*dto.OrderResponse, 0, len(resp.Orders))
	for _, o := range resp.Orders {
		orders = append(orders, convertProtoOrder(o))
	}

	return orders, resp.Total, nil
}

// ListOpenOrders 获取挂单列表
// Note: proto 中 ListOpenOrdersRequest 只支持 wallet 和 market 过滤，不支持分页
func (c *TradingClient) ListOpenOrders(ctx context.Context, wallet string, query *dto.OpenOrdersQuery) ([]*dto.OrderResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.ListOpenOrdersRequest{
		Wallet: wallet,
		Market: query.Market,
	}

	resp, err := c.client.ListOpenOrders(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	orders := make([]*dto.OrderResponse, 0, len(resp.Orders))
	for _, o := range resp.Orders {
		orders = append(orders, convertProtoOrder(o))
	}

	return orders, resp.Total, nil
}

// ========== 余额接口 ==========

// GetBalance 获取单个代币余额
func (c *TradingClient) GetBalance(ctx context.Context, wallet, token string) (*dto.BalanceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	balance, err := c.client.GetBalance(ctx, &pb.GetBalanceRequest{
		Wallet: wallet,
		Token:  token,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return convertProtoBalance(balance), nil
}

// GetBalances 获取所有余额
func (c *TradingClient) GetBalances(ctx context.Context, wallet string) ([]*dto.BalanceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetBalances(ctx, &pb.GetBalancesRequest{
		Wallet: wallet,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	balances := make([]*dto.BalanceResponse, 0, len(resp.Balances))
	for _, b := range resp.Balances {
		balances = append(balances, convertProtoBalance(b))
	}

	return balances, nil
}

// GetBalanceLogs 获取余额流水
func (c *TradingClient) GetBalanceLogs(ctx context.Context, wallet string, query *dto.ListTransactionsQuery) ([]*dto.TransactionResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.GetBalanceLogsRequest{
		Wallet:    wallet,
		Token:     query.Token,
		Page:      int32(query.Page),
		PageSize:  int32(query.PageSize),
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	if query.Type != "" {
		req.Type = convertBalanceChangeType(query.Type)
	}

	resp, err := c.client.GetBalanceLogs(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	logs := make([]*dto.TransactionResponse, 0, len(resp.Logs))
	for _, l := range resp.Logs {
		logs = append(logs, convertProtoBalanceLog(l))
	}

	return logs, resp.Total, nil
}

// ========== 成交接口 ==========

// GetTrade 获取成交详情
// Note: wallet 参数保留用于未来权限校验，当前 proto 只需要 tradeID
func (c *TradingClient) GetTrade(ctx context.Context, tradeID, _ string) (*dto.TradeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	trade, err := c.client.GetTrade(ctx, &pb.GetTradeRequest{
		TradeId: tradeID,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return convertProtoTrade(trade), nil
}

// ListTrades 获取成交列表
func (c *TradingClient) ListTrades(ctx context.Context, wallet string, query *dto.ListTradesQuery) ([]*dto.TradeResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.ListTradesRequest{
		Wallet:    wallet,
		Market:    query.Market,
		Page:      int32(query.Page),
		PageSize:  int32(query.PageSize),
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	resp, err := c.client.ListTrades(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	trades := make([]*dto.TradeResponse, 0, len(resp.Trades))
	for _, t := range resp.Trades {
		trades = append(trades, convertProtoTrade(t))
	}

	return trades, resp.Total, nil
}

// ListTradesByOrder 获取订单相关的成交列表
func (c *TradingClient) ListTradesByOrder(ctx context.Context, orderID string) ([]*dto.TradeResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.ListTradesByOrder(ctx, &pb.ListTradesByOrderRequest{
		OrderId: orderID,
	})
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	trades := make([]*dto.TradeResponse, 0, len(resp.Trades))
	for _, t := range resp.Trades {
		trades = append(trades, convertProtoTrade(t))
	}

	return trades, resp.Total, nil
}

// ========== 充值接口 ==========

// GetDeposit 获取充值详情
// Note: wallet 参数保留用于未来权限校验，当前 proto 只需要 depositID
func (c *TradingClient) GetDeposit(ctx context.Context, depositID, _ string) (*dto.DepositResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	deposit, err := c.client.GetDeposit(ctx, &pb.GetDepositRequest{
		DepositId: depositID,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return convertProtoDeposit(deposit), nil
}

// ListDeposits 获取充值列表
func (c *TradingClient) ListDeposits(ctx context.Context, wallet string, query *dto.ListDepositsQuery) ([]*dto.DepositResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.ListDepositsRequest{
		Wallet:    wallet,
		Token:     query.Token,
		Page:      int32(query.Page),
		PageSize:  int32(query.PageSize),
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	if query.Status != "" {
		req.Status = convertDepositStatus(query.Status)
	}

	resp, err := c.client.ListDeposits(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	deposits := make([]*dto.DepositResponse, 0, len(resp.Deposits))
	for _, d := range resp.Deposits {
		deposits = append(deposits, convertProtoDeposit(d))
	}

	return deposits, resp.Total, nil
}

// ========== 提现接口 ==========

// CreateWithdrawal 创建提现
func (c *TradingClient) CreateWithdrawal(ctx context.Context, req *dto.CreateWithdrawalRequest, wallet string) (*dto.CreateWithdrawalResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	pbReq := &pb.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     req.Token,
		Amount:    req.Amount,
		ToAddress: req.ToAddress,
		Nonce:     req.Nonce,
		Signature: []byte(req.Signature),
	}

	resp, err := c.client.CreateWithdrawal(ctx, pbReq)
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return &dto.CreateWithdrawalResponse{
		WithdrawID: resp.Withdrawal.WithdrawId,
		Status:     convertWithdrawStatusToString(resp.Withdrawal.Status),
	}, nil
}

// CancelWithdrawal 取消提现
func (c *TradingClient) CancelWithdrawal(ctx context.Context, withdrawID, wallet string) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	_, err := c.client.CancelWithdrawal(ctx, &pb.CancelWithdrawalRequest{
		WithdrawId: withdrawID,
		Wallet:     wallet,
	})
	if err != nil {
		return convertGRPCError(err)
	}

	return nil
}

// GetWithdrawal 获取提现详情
// Note: wallet 参数保留用于未来权限校验，当前 proto 只需要 withdrawID
func (c *TradingClient) GetWithdrawal(ctx context.Context, withdrawID, _ string) (*dto.WithdrawalResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	withdrawal, err := c.client.GetWithdrawal(ctx, &pb.GetWithdrawalRequest{
		WithdrawId: withdrawID,
	})
	if err != nil {
		return nil, convertGRPCError(err)
	}

	return convertProtoWithdrawal(withdrawal), nil
}

// ListWithdrawals 获取提现列表
func (c *TradingClient) ListWithdrawals(ctx context.Context, wallet string, query *dto.ListWithdrawalsQuery) ([]*dto.WithdrawalResponse, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	req := &pb.ListWithdrawalsRequest{
		Wallet:    wallet,
		Token:     query.Token,
		Page:      int32(query.Page),
		PageSize:  int32(query.PageSize),
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}

	if query.Status != "" {
		req.Status = convertWithdrawStatus(query.Status)
	}

	resp, err := c.client.ListWithdrawals(ctx, req)
	if err != nil {
		return nil, 0, convertGRPCError(err)
	}

	withdrawals := make([]*dto.WithdrawalResponse, 0, len(resp.Withdrawals))
	for _, w := range resp.Withdrawals {
		withdrawals = append(withdrawals, convertProtoWithdrawal(w))
	}

	return withdrawals, resp.Total, nil
}

// ========== 错误转换 ==========

// convertGRPCError 将 gRPC 错误转换为业务错误
func convertGRPCError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return dto.ErrInternalError
	}

	switch st.Code() {
	case codes.NotFound:
		msg := st.Message()
		switch {
		case contains(msg, "order"):
			return dto.ErrOrderNotFound
		case contains(msg, "trade"):
			return dto.ErrTradeNotFound
		case contains(msg, "deposit"):
			return dto.ErrDepositNotFound
		case contains(msg, "withdraw"):
			return dto.ErrWithdrawNotFound
		default:
			return dto.ErrOrderNotFound
		}
	case codes.InvalidArgument:
		return dto.ErrInvalidParams.WithMessage(st.Message())
	case codes.FailedPrecondition:
		msg := st.Message()
		switch {
		case contains(msg, "insufficient"):
			return dto.ErrInsufficientBalance
		case contains(msg, "cancelled"):
			return dto.ErrOrderAlreadyCancelled
		case contains(msg, "filled"):
			return dto.ErrOrderAlreadyFilled
		case contains(msg, "not cancellable"):
			return dto.ErrOrderNotCancellable
		default:
			return dto.ErrInvalidParams.WithMessage(st.Message())
		}
	case codes.AlreadyExists:
		return dto.ErrDuplicateOrder
	case codes.Unavailable:
		return dto.ErrServiceUnavailable
	case codes.DeadlineExceeded:
		return dto.ErrTimeout
	default:
		return dto.ErrInternalError
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
