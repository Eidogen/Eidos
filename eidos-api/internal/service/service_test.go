package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// MockTradingClient 模拟 TradingClientInterface
type MockTradingClient struct {
	mock.Mock
}

// MockMarketClient 模拟 MarketClientInterface
type MockMarketClient struct {
	mock.Mock
}

func (m *MockMarketClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockMarketClient) ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.MarketResponse), args.Error(1)
}

func (m *MockMarketClient) GetTicker(ctx context.Context, market string) (*dto.TickerResponse, error) {
	args := m.Called(ctx, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TickerResponse), args.Error(1)
}

func (m *MockMarketClient) ListTickers(ctx context.Context) ([]*dto.TickerResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.TickerResponse), args.Error(1)
}

func (m *MockMarketClient) GetDepth(ctx context.Context, market string, limit int) (*dto.DepthResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepthResponse), args.Error(1)
}

func (m *MockMarketClient) GetKlines(ctx context.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error) {
	args := m.Called(ctx, market, interval, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.KlineResponse), args.Error(1)
}

func (m *MockMarketClient) GetRecentTrades(ctx context.Context, market string, limit int) ([]*dto.RecentTradeResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.RecentTradeResponse), args.Error(1)
}

func (m *MockTradingClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTradingClient) CreateOrder(ctx context.Context, req *dto.CreateOrderRequest, wallet string) (*dto.CreateOrderResponse, error) {
	args := m.Called(ctx, req, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CreateOrderResponse), args.Error(1)
}

func (m *MockTradingClient) GetOrder(ctx context.Context, orderID, wallet string) (*dto.OrderResponse, error) {
	args := m.Called(ctx, orderID, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *MockTradingClient) ListOrders(ctx context.Context, wallet string, query *dto.ListOrdersQuery) ([]*dto.OrderResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.OrderResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) ListOpenOrders(ctx context.Context, wallet string, query *dto.OpenOrdersQuery) ([]*dto.OrderResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.OrderResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) CancelOrder(ctx context.Context, orderID, wallet string) error {
	args := m.Called(ctx, orderID, wallet)
	return args.Error(0)
}

func (m *MockTradingClient) GetBalance(ctx context.Context, wallet, token string) (*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.BalanceResponse), args.Error(1)
}

func (m *MockTradingClient) GetBalances(ctx context.Context, wallet string) ([]*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.BalanceResponse), args.Error(1)
}

func (m *MockTradingClient) GetBalanceLogs(ctx context.Context, wallet string, query *dto.ListTransactionsQuery) ([]*dto.TransactionResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.TransactionResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) GetDeposit(ctx context.Context, depositID, wallet string) (*dto.DepositResponse, error) {
	args := m.Called(ctx, depositID, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepositResponse), args.Error(1)
}

func (m *MockTradingClient) ListDeposits(ctx context.Context, wallet string, query *dto.ListDepositsQuery) ([]*dto.DepositResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.DepositResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) CreateWithdrawal(ctx context.Context, req *dto.CreateWithdrawalRequest, wallet string) (*dto.CreateWithdrawalResponse, error) {
	args := m.Called(ctx, req, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.CreateWithdrawalResponse), args.Error(1)
}

func (m *MockTradingClient) GetWithdrawal(ctx context.Context, withdrawID, wallet string) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, withdrawID, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

func (m *MockTradingClient) ListWithdrawals(ctx context.Context, wallet string, query *dto.ListWithdrawalsQuery) ([]*dto.WithdrawalResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.WithdrawalResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) CancelWithdrawal(ctx context.Context, withdrawID, wallet string) error {
	args := m.Called(ctx, withdrawID, wallet)
	return args.Error(0)
}

func (m *MockTradingClient) GetTrade(ctx context.Context, tradeID, wallet string) (*dto.TradeResponse, error) {
	args := m.Called(ctx, tradeID, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TradeResponse), args.Error(1)
}

func (m *MockTradingClient) ListTrades(ctx context.Context, wallet string, query *dto.ListTradesQuery) ([]*dto.TradeResponse, int64, error) {
	args := m.Called(ctx, wallet, query)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.TradeResponse), args.Get(1).(int64), args.Error(2)
}

func (m *MockTradingClient) ListTradesByOrder(ctx context.Context, orderID string) ([]*dto.TradeResponse, int64, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*dto.TradeResponse), args.Get(1).(int64), args.Error(2)
}

// createTestContext 创建测试 gin.Context
func createTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

// ========== OrderService Tests ==========

func TestNewOrderService(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	assert.NotNil(t, svc)
}

func TestOrderService_PrepareOrder(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	resp, err := svc.PrepareOrder(c, &dto.PrepareOrderRequest{})
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrNotImplemented, err)
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.CreateOrderRequest{
		Wallet: "0x1234",
		Market: "BTC-USDC",
		Side:   "buy",
		Type:   "limit",
		Price:  "42000",
		Amount: "0.1",
	}
	mockClient.On("CreateOrder", mock.Anything, req, "0x1234").
		Return(&dto.CreateOrderResponse{OrderID: "order-123", Status: "open"}, nil)

	resp, err := svc.CreateOrder(c, req)
	assert.NoError(t, err)
	assert.Equal(t, "order-123", resp.OrderID)
	assert.Equal(t, "open", resp.Status)
	mockClient.AssertExpectations(t)
}

func TestOrderService_CreateOrder_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.CreateOrderRequest{
		Wallet: "0x1234",
		Market: "BTC-USDC",
	}
	mockClient.On("CreateOrder", mock.Anything, req, "0x1234").
		Return(nil, dto.ErrInsufficientBalance)

	resp, err := svc.CreateOrder(c, req)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrInsufficientBalance, err)
	mockClient.AssertExpectations(t)
}

func TestOrderService_GetOrder_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetOrder", mock.Anything, "order-123", "").
		Return(&dto.OrderResponse{OrderID: "order-123", Market: "BTC-USDC"}, nil)

	resp, err := svc.GetOrder(c, "order-123")
	assert.NoError(t, err)
	assert.Equal(t, "order-123", resp.OrderID)
	mockClient.AssertExpectations(t)
}

func TestOrderService_GetOrder_NotFound(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetOrder", mock.Anything, "order-notfound", "").
		Return(nil, dto.ErrOrderNotFound)

	resp, err := svc.GetOrder(c, "order-notfound")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrOrderNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestOrderService_ListOrders_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListOrdersRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
		Market:   "BTC-USDC",
	}
	orders := []*dto.OrderResponse{
		{OrderID: "order-1"},
		{OrderID: "order-2"},
	}
	mockClient.On("ListOrders", mock.Anything, "0x1234", mock.Anything).
		Return(orders, int64(100), nil)

	resp, err := svc.ListOrders(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(100), resp.Total)
	assert.Equal(t, 10, resp.TotalPages)
	assert.Len(t, resp.Items.([]*dto.OrderResponse), 2)
	mockClient.AssertExpectations(t)
}

func TestOrderService_ListOrders_WithStatus(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListOrdersRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
		Status:   "open",
	}
	mockClient.On("ListOrders", mock.Anything, "0x1234", mock.MatchedBy(func(q *dto.ListOrdersQuery) bool {
		return len(q.Statuses) == 1 && q.Statuses[0] == "open"
	})).Return([]*dto.OrderResponse{}, int64(0), nil)

	_, err := svc.ListOrders(c, req)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestOrderService_ListOrders_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListOrdersRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	mockClient.On("ListOrders", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.ListOrders(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

func TestOrderService_ListOpenOrders_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()
	c.Set("wallet", "0x1234")

	orders := []*dto.OrderResponse{
		{OrderID: "order-1", Status: "open"},
	}
	mockClient.On("ListOpenOrders", mock.Anything, "0x1234", mock.Anything).
		Return(orders, int64(1), nil)

	resp, err := svc.ListOpenOrders(c, "BTC-USDC")
	assert.NoError(t, err)
	assert.Len(t, resp, 1)
	mockClient.AssertExpectations(t)
}

func TestOrderService_CancelOrder_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()
	c.Set("wallet", "0x1234")

	mockClient.On("CancelOrder", mock.Anything, "order-123", "0x1234").Return(nil)
	mockClient.On("GetOrder", mock.Anything, "order-123", "").
		Return(&dto.OrderResponse{OrderID: "order-123", Status: "cancelled"}, nil)

	resp, err := svc.CancelOrder(c, "order-123")
	assert.NoError(t, err)
	assert.Equal(t, "cancelled", resp.Status)
	mockClient.AssertExpectations(t)
}

func TestOrderService_CancelOrder_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()
	c.Set("wallet", "0x1234")

	mockClient.On("CancelOrder", mock.Anything, "order-123", "0x1234").
		Return(dto.ErrOrderNotCancellable)

	resp, err := svc.CancelOrder(c, "order-123")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrOrderNotCancellable, err)
	mockClient.AssertExpectations(t)
}

func TestOrderService_BatchCancelOrders_ByIDs_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.BatchCancelRequest{
		Wallet:   "0x1234",
		OrderIDs: []string{"order-1", "order-2", "order-3"},
	}

	mockClient.On("CancelOrder", mock.Anything, "order-1", "0x1234").Return(nil)
	mockClient.On("CancelOrder", mock.Anything, "order-2", "0x1234").Return(errors.New("cancel failed"))
	mockClient.On("CancelOrder", mock.Anything, "order-3", "0x1234").Return(nil)

	resp, err := svc.BatchCancelOrders(c, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Cancelled, 2)
	assert.Len(t, resp.Failed, 1)
	assert.Contains(t, resp.Cancelled, "order-1")
	assert.Contains(t, resp.Cancelled, "order-3")
	assert.Contains(t, resp.Failed, "order-2")
	mockClient.AssertExpectations(t)
}

func TestOrderService_BatchCancelOrders_ByMarket_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.BatchCancelRequest{
		Wallet: "0x1234",
		Market: "BTC-USDC",
	}

	orders := []*dto.OrderResponse{
		{OrderID: "order-1", Market: "BTC-USDC", Side: "buy"},
		{OrderID: "order-2", Market: "BTC-USDC", Side: "sell"},
	}
	mockClient.On("ListOpenOrders", mock.Anything, "0x1234", mock.Anything).
		Return(orders, int64(2), nil)
	mockClient.On("CancelOrder", mock.Anything, "order-1", "0x1234").Return(nil)
	mockClient.On("CancelOrder", mock.Anything, "order-2", "0x1234").Return(nil)

	resp, err := svc.BatchCancelOrders(c, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Cancelled, 2)
	assert.Empty(t, resp.Failed)
	mockClient.AssertExpectations(t)
}

func TestOrderService_BatchCancelOrders_ByMarketAndSide(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.BatchCancelRequest{
		Wallet: "0x1234",
		Market: "BTC-USDC",
		Side:   "buy",
	}

	orders := []*dto.OrderResponse{
		{OrderID: "order-1", Market: "BTC-USDC", Side: "buy"},
		{OrderID: "order-2", Market: "BTC-USDC", Side: "sell"},
		{OrderID: "order-3", Market: "BTC-USDC", Side: "buy"},
	}
	mockClient.On("ListOpenOrders", mock.Anything, "0x1234", mock.Anything).
		Return(orders, int64(3), nil)
	mockClient.On("CancelOrder", mock.Anything, "order-1", "0x1234").Return(nil)
	mockClient.On("CancelOrder", mock.Anything, "order-3", "0x1234").Return(nil)

	resp, err := svc.BatchCancelOrders(c, req)
	assert.NoError(t, err)
	assert.Len(t, resp.Cancelled, 2)
	assert.Contains(t, resp.Cancelled, "order-1")
	assert.Contains(t, resp.Cancelled, "order-3")
	mockClient.AssertExpectations(t)
}

func TestOrderService_BatchCancelOrders_ListError(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewOrderService(mockClient)
	c, _ := createTestContext()

	req := &dto.BatchCancelRequest{
		Wallet: "0x1234",
		Market: "BTC-USDC",
	}

	mockClient.On("ListOpenOrders", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.BatchCancelOrders(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

// ========== BalanceService Tests ==========

func TestNewBalanceService(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	assert.NotNil(t, svc)
}

func TestBalanceService_GetBalance_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetBalance", mock.Anything, "0x1234", "USDC").
		Return(&dto.BalanceResponse{Token: "USDC", TotalAvailable: "1000.00"}, nil)

	resp, err := svc.GetBalance(c, "0x1234", "USDC")
	assert.NoError(t, err)
	assert.Equal(t, "USDC", resp.Token)
	assert.Equal(t, "1000.00", resp.TotalAvailable)
	mockClient.AssertExpectations(t)
}

func TestBalanceService_GetBalance_NotFound(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetBalance", mock.Anything, "0x1234", "BTC").
		Return(nil, dto.ErrTokenNotSupported)

	resp, err := svc.GetBalance(c, "0x1234", "BTC")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrTokenNotSupported, err)
	mockClient.AssertExpectations(t)
}

func TestBalanceService_ListBalances_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	c, _ := createTestContext()

	balances := []*dto.BalanceResponse{
		{Token: "USDC", TotalAvailable: "1000.00"},
		{Token: "BTC", TotalAvailable: "0.5"},
	}
	mockClient.On("GetBalances", mock.Anything, "0x1234").Return(balances, nil)

	resp, err := svc.ListBalances(c, "0x1234")
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	mockClient.AssertExpectations(t)
}

func TestBalanceService_ListTransactions_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTransactionsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 20,
		Token:    "USDC",
	}
	logs := []*dto.TransactionResponse{
		{ID: "log-1", Type: "deposit"},
	}
	mockClient.On("GetBalanceLogs", mock.Anything, "0x1234", mock.Anything).
		Return(logs, int64(50), nil)

	resp, err := svc.ListTransactions(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(50), resp.Total)
	assert.Equal(t, 3, resp.TotalPages)
	mockClient.AssertExpectations(t)
}

func TestBalanceService_ListTransactions_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewBalanceService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTransactionsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 20,
	}
	mockClient.On("GetBalanceLogs", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.ListTransactions(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

// ========== DepositService Tests ==========

func TestNewDepositService(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewDepositService(mockClient)
	assert.NotNil(t, svc)
}

func TestDepositService_GetDeposit_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewDepositService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetDeposit", mock.Anything, "deposit-123", "").
		Return(&dto.DepositResponse{DepositID: "deposit-123", Status: "credited"}, nil)

	resp, err := svc.GetDeposit(c, "deposit-123")
	assert.NoError(t, err)
	assert.Equal(t, "deposit-123", resp.DepositID)
	mockClient.AssertExpectations(t)
}

func TestDepositService_GetDeposit_NotFound(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewDepositService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetDeposit", mock.Anything, "deposit-notfound", "").
		Return(nil, dto.ErrDepositNotFound)

	resp, err := svc.GetDeposit(c, "deposit-notfound")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrDepositNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestDepositService_ListDeposits_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewDepositService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListDepositsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	deposits := []*dto.DepositResponse{
		{DepositID: "deposit-1"},
	}
	mockClient.On("ListDeposits", mock.Anything, "0x1234", mock.Anything).
		Return(deposits, int64(20), nil)

	resp, err := svc.ListDeposits(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(20), resp.Total)
	mockClient.AssertExpectations(t)
}

func TestDepositService_ListDeposits_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewDepositService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListDepositsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	mockClient.On("ListDeposits", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.ListDeposits(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

// ========== WithdrawalService Tests ==========

func TestNewWithdrawalService(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	assert.NotNil(t, svc)
}

func TestWithdrawalService_CreateWithdrawal_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	req := &dto.CreateWithdrawalRequest{
		Wallet:    "0x1234",
		Token:     "USDC",
		Amount:    "100.00",
		ToAddress: "0x5678",
	}
	mockClient.On("CreateWithdrawal", mock.Anything, req, "0x1234").
		Return(&dto.CreateWithdrawalResponse{WithdrawID: "withdraw-123", Status: "pending"}, nil)

	resp, err := svc.CreateWithdrawal(c, req)
	assert.NoError(t, err)
	assert.Equal(t, "withdraw-123", resp.WithdrawID)
	assert.Equal(t, "pending", resp.Status)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_CreateWithdrawal_InsufficientBalance(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	req := &dto.CreateWithdrawalRequest{
		Wallet:    "0x1234",
		Token:     "USDC",
		Amount:    "10000.00",
		ToAddress: "0x5678",
	}
	mockClient.On("CreateWithdrawal", mock.Anything, req, "0x1234").
		Return(nil, dto.ErrInsufficientBalance)

	resp, err := svc.CreateWithdrawal(c, req)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrInsufficientBalance, err)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_GetWithdrawal_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetWithdrawal", mock.Anything, "withdraw-123", "").
		Return(&dto.WithdrawalResponse{WithdrawID: "withdraw-123", Status: "confirmed"}, nil)

	resp, err := svc.GetWithdrawal(c, "withdraw-123")
	assert.NoError(t, err)
	assert.Equal(t, "withdraw-123", resp.WithdrawID)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_GetWithdrawal_NotFound(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetWithdrawal", mock.Anything, "withdraw-notfound", "").
		Return(nil, dto.ErrWithdrawNotFound)

	resp, err := svc.GetWithdrawal(c, "withdraw-notfound")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrWithdrawNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_ListWithdrawals_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListWithdrawalsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	withdrawals := []*dto.WithdrawalResponse{
		{WithdrawID: "withdraw-1"},
	}
	mockClient.On("ListWithdrawals", mock.Anything, "0x1234", mock.Anything).
		Return(withdrawals, int64(15), nil)

	resp, err := svc.ListWithdrawals(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(15), resp.Total)
	assert.Equal(t, 2, resp.TotalPages)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_ListWithdrawals_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListWithdrawalsRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	mockClient.On("ListWithdrawals", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.ListWithdrawals(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_CancelWithdrawal_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()
	c.Set("wallet", "0x1234")

	mockClient.On("CancelWithdrawal", mock.Anything, "withdraw-123", "0x1234").Return(nil)
	mockClient.On("GetWithdrawal", mock.Anything, "withdraw-123", "").
		Return(&dto.WithdrawalResponse{WithdrawID: "withdraw-123", Status: "cancelled"}, nil)

	resp, err := svc.CancelWithdrawal(c, "withdraw-123")
	assert.NoError(t, err)
	assert.Equal(t, "cancelled", resp.Status)
	mockClient.AssertExpectations(t)
}

func TestWithdrawalService_CancelWithdrawal_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewWithdrawalService(mockClient)
	c, _ := createTestContext()
	c.Set("wallet", "0x1234")

	mockClient.On("CancelWithdrawal", mock.Anything, "withdraw-123", "0x1234").
		Return(dto.ErrOrderNotCancellable)

	resp, err := svc.CancelWithdrawal(c, "withdraw-123")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrOrderNotCancellable, err)
	mockClient.AssertExpectations(t)
}

// ========== TradeService Tests ==========

func TestNewTradeService(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	assert.NotNil(t, svc)
}

func TestTradeService_GetTrade_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetTrade", mock.Anything, "trade-123", "").
		Return(&dto.TradeResponse{TradeID: "trade-123", Market: "BTC-USDC"}, nil)

	resp, err := svc.GetTrade(c, "trade-123")
	assert.NoError(t, err)
	assert.Equal(t, "trade-123", resp.TradeID)
	mockClient.AssertExpectations(t)
}

func TestTradeService_GetTrade_NotFound(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetTrade", mock.Anything, "trade-notfound", "").
		Return(nil, dto.ErrTradeNotFound)

	resp, err := svc.GetTrade(c, "trade-notfound")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrTradeNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestTradeService_ListTrades_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTradesRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
		Market:   "BTC-USDC",
	}
	trades := []*dto.TradeResponse{
		{TradeID: "trade-1"},
		{TradeID: "trade-2"},
	}
	mockClient.On("ListTrades", mock.Anything, "0x1234", mock.Anything).
		Return(trades, int64(50), nil)

	resp, err := svc.ListTrades(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(50), resp.Total)
	assert.Equal(t, 5, resp.TotalPages)
	mockClient.AssertExpectations(t)
}

func TestTradeService_ListTrades_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTradesRequest{
		Wallet:   "0x1234",
		Page:     1,
		PageSize: 10,
	}
	mockClient.On("ListTrades", mock.Anything, "0x1234", mock.Anything).
		Return(nil, int64(0), errors.New("db error"))

	resp, err := svc.ListTrades(c, req)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

func TestTradeService_ListTradesByOrder_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTradesRequest{
		Wallet:   "0x1234",
		OrderID:  "order-123",
		Page:     1,
		PageSize: 10,
	}
	trades := []*dto.TradeResponse{
		{TradeID: "trade-1", MakerOrderID: "order-123"},
		{TradeID: "trade-2", TakerOrderID: "order-123"},
	}
	mockClient.On("ListTradesByOrder", mock.Anything, "order-123").
		Return(trades, int64(2), nil)

	resp, err := svc.ListTrades(c, req)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, 1, resp.TotalPages)
	mockClient.AssertExpectations(t)
}

func TestTradeService_ListTradesByOrder_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	svc := NewTradeService(mockClient)
	c, _ := createTestContext()

	req := &dto.ListTradesRequest{
		Wallet:  "0x1234",
		OrderID: "order-notfound",
	}
	mockClient.On("ListTradesByOrder", mock.Anything, "order-notfound").
		Return(nil, int64(0), dto.ErrOrderNotFound)

	resp, err := svc.ListTrades(c, req)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrOrderNotFound, err)
	mockClient.AssertExpectations(t)
}

// ========== TradingHealthAdapter Tests ==========

func TestNewTradingHealthAdapter(t *testing.T) {
	mockClient := new(MockTradingClient)
	adapter := NewTradingHealthAdapter(mockClient)
	assert.NotNil(t, adapter)
}

func TestTradingHealthAdapter_Ping_Success(t *testing.T) {
	mockClient := new(MockTradingClient)
	adapter := NewTradingHealthAdapter(mockClient)

	mockClient.On("Ping", mock.Anything).Return(nil)

	err := adapter.Ping()
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestTradingHealthAdapter_Ping_Error(t *testing.T) {
	mockClient := new(MockTradingClient)
	adapter := NewTradingHealthAdapter(mockClient)

	mockClient.On("Ping", mock.Anything).Return(errors.New("connection refused"))

	err := adapter.Ping()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	mockClient.AssertExpectations(t)
}

// ========== Pagination Calculation Tests ==========

func TestPaginationCalculation(t *testing.T) {
	tests := []struct {
		name       string
		total      int64
		pageSize   int
		wantPages  int
	}{
		{"exact_division", 100, 10, 10},
		{"with_remainder", 101, 10, 11},
		{"single_page", 5, 10, 1},
		{"zero_items", 0, 10, 0},
		{"single_item", 1, 10, 1},
		{"large_total", 999, 10, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockTradingClient)
			svc := NewOrderService(mockClient)
			c, _ := createTestContext()

			req := &dto.ListOrdersRequest{
				Wallet:   "0x1234",
				Page:     1,
				PageSize: tt.pageSize,
			}
			mockClient.On("ListOrders", mock.Anything, "0x1234", mock.Anything).
				Return([]*dto.OrderResponse{}, tt.total, nil)

			resp, err := svc.ListOrders(c, req)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantPages, resp.TotalPages)
			mockClient.AssertExpectations(t)
		})
	}
}

// ========== MarketService Tests ==========

func TestNewMarketService(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	assert.NotNil(t, svc)
}

func TestMarketService_ListMarkets_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	markets := []*dto.MarketResponse{
		{Symbol: "BTC-USDC", BaseToken: "BTC", QuoteToken: "USDC", IsActive: true},
		{Symbol: "ETH-USDC", BaseToken: "ETH", QuoteToken: "USDC", IsActive: true},
	}
	mockClient.On("ListMarkets", mock.Anything).Return(markets, nil)

	resp, err := svc.ListMarkets(c)
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	assert.Equal(t, "BTC-USDC", resp[0].Symbol)
	mockClient.AssertExpectations(t)
}

func TestMarketService_ListMarkets_Error(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("ListMarkets", mock.Anything).Return(nil, dto.ErrServiceUnavailable)

	resp, err := svc.ListMarkets(c)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrServiceUnavailable, err)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetTicker_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	ticker := &dto.TickerResponse{
		Market:    "BTC-USDC",
		LastPrice: "42000.50",
		Volume:    "1234.56",
		High:      "43000.00",
		Low:       "41000.00",
	}
	mockClient.On("GetTicker", mock.Anything, "BTC-USDC").Return(ticker, nil)

	resp, err := svc.GetTicker(c, "BTC-USDC")
	assert.NoError(t, err)
	assert.Equal(t, "BTC-USDC", resp.Market)
	assert.Equal(t, "42000.50", resp.LastPrice)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetTicker_NotFound(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetTicker", mock.Anything, "INVALID-PAIR").Return(nil, dto.ErrMarketNotFound)

	resp, err := svc.GetTicker(c, "INVALID-PAIR")
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrMarketNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestMarketService_ListTickers_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	tickers := []*dto.TickerResponse{
		{Market: "BTC-USDC", LastPrice: "42000.50"},
		{Market: "ETH-USDC", LastPrice: "2500.00"},
	}
	mockClient.On("ListTickers", mock.Anything).Return(tickers, nil)

	resp, err := svc.ListTickers(c)
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	mockClient.AssertExpectations(t)
}

func TestMarketService_ListTickers_Error(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("ListTickers", mock.Anything).Return(nil, errors.New("connection failed"))

	resp, err := svc.ListTickers(c)
	assert.Nil(t, resp)
	assert.Error(t, err)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetDepth_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	depth := &dto.DepthResponse{
		Market: "BTC-USDC",
		Bids:   [][]string{{"42000", "1.5"}, {"41999", "2.0"}},
		Asks:   [][]string{{"42001", "1.0"}, {"42002", "0.8"}},
	}
	mockClient.On("GetDepth", mock.Anything, "BTC-USDC", 50).Return(depth, nil)

	resp, err := svc.GetDepth(c, "BTC-USDC", 50)
	assert.NoError(t, err)
	assert.Equal(t, "BTC-USDC", resp.Market)
	assert.Len(t, resp.Bids, 2)
	assert.Len(t, resp.Asks, 2)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetDepth_NotFound(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetDepth", mock.Anything, "INVALID-PAIR", 50).Return(nil, dto.ErrMarketNotFound)

	resp, err := svc.GetDepth(c, "INVALID-PAIR", 50)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrMarketNotFound, err)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetKlines_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	klines := []*dto.KlineResponse{
		{OpenTime: 1704067200000, Open: "42000", High: "42500", Low: "41800", Close: "42300"},
		{OpenTime: 1704070800000, Open: "42300", High: "42800", Low: "42200", Close: "42600"},
	}
	mockClient.On("GetKlines", mock.Anything, "BTC-USDC", "1h", int64(1704067200000), int64(1704154000000), 100).
		Return(klines, nil)

	resp, err := svc.GetKlines(c, "BTC-USDC", "1h", 1704067200000, 1704154000000, 100)
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	assert.Equal(t, "42000", resp[0].Open)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetKlines_InvalidInterval(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetKlines", mock.Anything, "BTC-USDC", "invalid", int64(0), int64(0), 100).
		Return(nil, dto.ErrInvalidParams)

	resp, err := svc.GetKlines(c, "BTC-USDC", "invalid", 0, 0, 100)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrInvalidParams, err)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetRecentTrades_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	trades := []*dto.RecentTradeResponse{
		{TradeID: "trade-1", Price: "42000.50", Amount: "0.5", Side: "buy", Timestamp: 1704067200000},
		{TradeID: "trade-2", Price: "42001.00", Amount: "0.3", Side: "sell", Timestamp: 1704067201000},
	}
	mockClient.On("GetRecentTrades", mock.Anything, "BTC-USDC", 50).Return(trades, nil)

	resp, err := svc.GetRecentTrades(c, "BTC-USDC", 50)
	assert.NoError(t, err)
	assert.Len(t, resp, 2)
	assert.Equal(t, "buy", resp[0].Side)
	assert.Equal(t, "sell", resp[1].Side)
	mockClient.AssertExpectations(t)
}

func TestMarketService_GetRecentTrades_NotFound(t *testing.T) {
	mockClient := new(MockMarketClient)
	svc := NewMarketService(mockClient)
	c, _ := createTestContext()

	mockClient.On("GetRecentTrades", mock.Anything, "INVALID-PAIR", 50).Return(nil, dto.ErrMarketNotFound)

	resp, err := svc.GetRecentTrades(c, "INVALID-PAIR", 50)
	assert.Nil(t, resp)
	assert.Equal(t, dto.ErrMarketNotFound, err)
	mockClient.AssertExpectations(t)
}

// ========== MarketHealthAdapter Tests ==========

func TestNewMarketHealthAdapter(t *testing.T) {
	mockClient := new(MockMarketClient)
	adapter := NewMarketHealthAdapter(mockClient)
	assert.NotNil(t, adapter)
}

func TestMarketHealthAdapter_Ping_Success(t *testing.T) {
	mockClient := new(MockMarketClient)
	adapter := NewMarketHealthAdapter(mockClient)

	mockClient.On("Ping", mock.Anything).Return(nil)

	err := adapter.Ping()
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestMarketHealthAdapter_Ping_Error(t *testing.T) {
	mockClient := new(MockMarketClient)
	adapter := NewMarketHealthAdapter(mockClient)

	mockClient.On("Ping", mock.Anything).Return(errors.New("market service unavailable"))

	err := adapter.Ping()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "market service unavailable")
	mockClient.AssertExpectations(t)
}
