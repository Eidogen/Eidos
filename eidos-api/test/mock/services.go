// Package mock 提供测试用 Mock 对象
package mock

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// OrderService Mock 订单服务
type OrderService struct {
	mock.Mock
}

func (m *OrderService) PrepareOrder(ctx *gin.Context, req *dto.PrepareOrderRequest) (*dto.PrepareOrderResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PrepareOrderResponse), args.Error(1)
}

func (m *OrderService) CreateOrder(ctx *gin.Context, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *OrderService) GetOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *OrderService) ListOrders(ctx *gin.Context, req *dto.ListOrdersRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

func (m *OrderService) ListOpenOrders(ctx *gin.Context, market string) ([]*dto.OrderResponse, error) {
	args := m.Called(ctx, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.OrderResponse), args.Error(1)
}

func (m *OrderService) CancelOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *OrderService) BatchCancelOrders(ctx *gin.Context, req *dto.BatchCancelRequest) (*dto.BatchCancelResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.BatchCancelResponse), args.Error(1)
}

// BalanceService Mock 余额服务
type BalanceService struct {
	mock.Mock
}

func (m *BalanceService) GetBalance(ctx *gin.Context, wallet, token string) (*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.BalanceResponse), args.Error(1)
}

func (m *BalanceService) ListBalances(ctx *gin.Context, wallet string) ([]*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.BalanceResponse), args.Error(1)
}

func (m *BalanceService) ListTransactions(ctx *gin.Context, req *dto.ListTransactionsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// DepositService Mock 充值服务
type DepositService struct {
	mock.Mock
}

func (m *DepositService) GetDeposit(ctx *gin.Context, depositID string) (*dto.DepositResponse, error) {
	args := m.Called(ctx, depositID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepositResponse), args.Error(1)
}

func (m *DepositService) ListDeposits(ctx *gin.Context, req *dto.ListDepositsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// WithdrawalService Mock 提现服务
type WithdrawalService struct {
	mock.Mock
}

func (m *WithdrawalService) CreateWithdrawal(ctx *gin.Context, req *dto.CreateWithdrawalRequest) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

func (m *WithdrawalService) GetWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

func (m *WithdrawalService) ListWithdrawals(ctx *gin.Context, req *dto.ListWithdrawalsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

func (m *WithdrawalService) CancelWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

// TradeService Mock 成交服务
type TradeService struct {
	mock.Mock
}

func (m *TradeService) GetTrade(ctx *gin.Context, tradeID string) (*dto.TradeResponse, error) {
	args := m.Called(ctx, tradeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TradeResponse), args.Error(1)
}

func (m *TradeService) ListTrades(ctx *gin.Context, req *dto.ListTradesRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// MarketService Mock 行情服务
type MarketService struct {
	mock.Mock
}

func (m *MarketService) ListMarkets(ctx *gin.Context) ([]*dto.MarketResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.MarketResponse), args.Error(1)
}

func (m *MarketService) GetTicker(ctx *gin.Context, market string) (*dto.TickerResponse, error) {
	args := m.Called(ctx, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TickerResponse), args.Error(1)
}

func (m *MarketService) ListTickers(ctx *gin.Context) ([]*dto.TickerResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.TickerResponse), args.Error(1)
}

func (m *MarketService) GetDepth(ctx *gin.Context, market string, limit int) (*dto.DepthResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepthResponse), args.Error(1)
}

func (m *MarketService) GetKlines(ctx *gin.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error) {
	args := m.Called(ctx, market, interval, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.KlineResponse), args.Error(1)
}

func (m *MarketService) GetRecentTrades(ctx *gin.Context, market string, limit int) ([]*dto.RecentTradeResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.RecentTradeResponse), args.Error(1)
}
