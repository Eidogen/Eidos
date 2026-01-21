package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// TradingClient wraps the trading service gRPC client
type TradingClient struct {
	conn   *grpc.ClientConn
	client tradingv1.TradingServiceClient
}

// NewTradingClient creates a new trading client
func NewTradingClient(ctx context.Context, target string) (*TradingClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &TradingClient{
		conn:   conn,
		client: tradingv1.NewTradingServiceClient(conn),
	}, nil
}

// Close closes the connection
func (c *TradingClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetOrder retrieves an order by ID
func (c *TradingClient) GetOrder(ctx context.Context, orderID string) (*tradingv1.Order, error) {
	return c.client.GetOrder(ctx, &tradingv1.GetOrderRequest{
		OrderId: orderID,
	})
}

// ListOrders retrieves orders with filters
func (c *TradingClient) ListOrders(ctx context.Context, req *tradingv1.ListOrdersRequest) (*tradingv1.ListOrdersResponse, error) {
	return c.client.ListOrders(ctx, req)
}

// ListOpenOrders retrieves active orders
func (c *TradingClient) ListOpenOrders(ctx context.Context, wallet, market string) (*tradingv1.ListOrdersResponse, error) {
	return c.client.ListOpenOrders(ctx, &tradingv1.ListOpenOrdersRequest{
		Wallet: wallet,
		Market: market,
	})
}

// CancelOrder cancels an order
func (c *TradingClient) CancelOrder(ctx context.Context, wallet, orderID string) error {
	_, err := c.client.CancelOrder(ctx, &tradingv1.CancelOrderRequest{
		Wallet:  wallet,
		OrderId: orderID,
	})
	return err
}

// BatchCancelOrders cancels multiple orders
func (c *TradingClient) BatchCancelOrders(ctx context.Context, req *tradingv1.BatchCancelOrdersRequest) (*tradingv1.BatchCancelOrdersResponse, error) {
	return c.client.BatchCancelOrders(ctx, req)
}

// GetBalance retrieves balance for a token
func (c *TradingClient) GetBalance(ctx context.Context, wallet, token string) (*tradingv1.Balance, error) {
	return c.client.GetBalance(ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  token,
	})
}

// GetBalances retrieves all balances for a wallet
func (c *TradingClient) GetBalances(ctx context.Context, wallet string, hideZero bool) (*tradingv1.GetBalancesResponse, error) {
	return c.client.GetBalances(ctx, &tradingv1.GetBalancesRequest{
		Wallet:   wallet,
		HideZero: hideZero,
	})
}

// GetBalanceLogs retrieves balance change history
func (c *TradingClient) GetBalanceLogs(ctx context.Context, req *tradingv1.GetBalanceLogsRequest) (*tradingv1.GetBalanceLogsResponse, error) {
	return c.client.GetBalanceLogs(ctx, req)
}

// GetTrade retrieves trade details
func (c *TradingClient) GetTrade(ctx context.Context, tradeID string) (*tradingv1.Trade, error) {
	return c.client.GetTrade(ctx, &tradingv1.GetTradeRequest{
		TradeId: tradeID,
	})
}

// ListTrades retrieves trade history
func (c *TradingClient) ListTrades(ctx context.Context, req *tradingv1.ListTradesRequest) (*tradingv1.ListTradesResponse, error) {
	return c.client.ListTrades(ctx, req)
}

// ListTradesByOrder retrieves trades for an order
func (c *TradingClient) ListTradesByOrder(ctx context.Context, orderID string) (*tradingv1.ListTradesResponse, error) {
	return c.client.ListTradesByOrder(ctx, &tradingv1.ListTradesByOrderRequest{
		OrderId: orderID,
	})
}

// GetDeposit retrieves deposit details
func (c *TradingClient) GetDeposit(ctx context.Context, depositID string) (*tradingv1.Deposit, error) {
	return c.client.GetDeposit(ctx, &tradingv1.GetDepositRequest{
		DepositId: depositID,
	})
}

// ListDeposits retrieves deposit history
func (c *TradingClient) ListDeposits(ctx context.Context, req *tradingv1.ListDepositsRequest) (*tradingv1.ListDepositsResponse, error) {
	return c.client.ListDeposits(ctx, req)
}

// GetWithdrawal retrieves withdrawal details
func (c *TradingClient) GetWithdrawal(ctx context.Context, withdrawID string) (*tradingv1.Withdrawal, error) {
	return c.client.GetWithdrawal(ctx, &tradingv1.GetWithdrawalRequest{
		WithdrawId: withdrawID,
	})
}

// ListWithdrawals retrieves withdrawal history
func (c *TradingClient) ListWithdrawals(ctx context.Context, req *tradingv1.ListWithdrawalsRequest) (*tradingv1.ListWithdrawalsResponse, error) {
	return c.client.ListWithdrawals(ctx, req)
}

// CancelWithdrawal cancels a pending withdrawal
func (c *TradingClient) CancelWithdrawal(ctx context.Context, wallet, withdrawID string) error {
	_, err := c.client.CancelWithdrawal(ctx, &tradingv1.CancelWithdrawalRequest{
		Wallet:     wallet,
		WithdrawId: withdrawID,
	})
	return err
}

// ConfirmSettlement confirms on-chain settlement (internal API)
func (c *TradingClient) ConfirmSettlement(ctx context.Context, req *tradingv1.ConfirmSettlementRequest) error {
	_, err := c.client.ConfirmSettlement(ctx, req)
	return err
}

// RollbackSettlement rolls back a failed settlement (internal API)
func (c *TradingClient) RollbackSettlement(ctx context.Context, req *tradingv1.RollbackSettlementRequest) error {
	_, err := c.client.RollbackSettlement(ctx, req)
	return err
}

// AdminCancelOrder cancels order for admin (without wallet ownership check)
// This is a wrapper that should be used by admin only
func (c *TradingClient) AdminCancelOrder(ctx context.Context, orderID string) error {
	// Get order first to get wallet
	order, err := c.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	_, err = c.client.CancelOrder(ctx, &tradingv1.CancelOrderRequest{
		Wallet:  order.Wallet,
		OrderId: orderID,
	})
	return err
}

// AdminBatchCancelOrders cancels all orders for a wallet (admin operation)
func (c *TradingClient) AdminBatchCancelOrders(ctx context.Context, wallet, market string) (*tradingv1.BatchCancelOrdersResponse, error) {
	return c.client.BatchCancelOrders(ctx, &tradingv1.BatchCancelOrdersRequest{
		Wallet: wallet,
		Market: market,
	})
}

// ProcessTradeResult processes trade result from matching (internal)
func (c *TradingClient) ProcessTradeResult(ctx context.Context, req *tradingv1.ProcessTradeResultRequest) error {
	_, err := c.client.ProcessTradeResult(ctx, req)
	return err
}

// ProcessDepositEvent processes deposit event (internal)
func (c *TradingClient) ProcessDepositEvent(ctx context.Context, req *tradingv1.ProcessDepositEventRequest) error {
	_, err := c.client.ProcessDepositEvent(ctx, req)
	return err
}

// Unexported but useful type aliases
var _ = (*emptypb.Empty)(nil)
