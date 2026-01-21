package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== Mock Services ==========

// MockOrderService 订单服务 Mock
type MockOrderService struct {
	mock.Mock
}

func (m *MockOrderService) CreateOrder(ctx context.Context, req *service.CreateOrderRequest) (*model.Order, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) CancelOrder(ctx context.Context, wallet, orderID string) error {
	args := m.Called(ctx, wallet, orderID)
	return args.Error(0)
}

func (m *MockOrderService) CancelOrderByNonce(ctx context.Context, wallet string, nonce uint64, signature []byte) error {
	args := m.Called(ctx, wallet, nonce, signature)
	return args.Error(0)
}

func (m *MockOrderService) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) GetOrderByClientID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error) {
	args := m.Called(ctx, wallet, clientOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) ListOrders(ctx context.Context, wallet string, filter *repository.OrderFilter, page *repository.Pagination) ([]*model.Order, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderService) ListOpenOrders(ctx context.Context, wallet, market string) ([]*model.Order, error) {
	args := m.Called(ctx, wallet, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderService) UpdateOrderFilled(ctx context.Context, orderID string, filledAmount, filledQuote decimal.Decimal, newStatus model.OrderStatus) error {
	args := m.Called(ctx, orderID, filledAmount, filledQuote, newStatus)
	return args.Error(0)
}

func (m *MockOrderService) ExpireOrder(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockOrderService) RejectOrder(ctx context.Context, orderID string, reason string) error {
	args := m.Called(ctx, orderID, reason)
	return args.Error(0)
}

func (m *MockOrderService) HandleCancelConfirm(ctx context.Context, msg *service.OrderCancelledMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockOrderService) HandleOrderAccepted(ctx context.Context, msg *service.OrderAcceptedMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

// MockBalanceService 余额服务 Mock
type MockBalanceService struct {
	mock.Mock
}

func (m *MockBalanceService) GetBalance(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *MockBalanceService) GetBalances(ctx context.Context, wallet string) ([]*model.Balance, error) {
	args := m.Called(ctx, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Balance), args.Error(1)
}

func (m *MockBalanceService) GetAvailableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	args := m.Called(ctx, wallet, token)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockBalanceService) GetWithdrawableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	args := m.Called(ctx, wallet, token)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockBalanceService) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool, remark string) error {
	args := m.Called(ctx, wallet, token, amount, toSettled, remark)
	return args.Error(0)
}

func (m *MockBalanceService) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error {
	args := m.Called(ctx, wallet, token, amount, orderID)
	return args.Error(0)
}

func (m *MockBalanceService) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error {
	args := m.Called(ctx, wallet, token, amount, orderID)
	return args.Error(0)
}

func (m *MockBalanceService) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, tradeID string) error {
	args := m.Called(ctx, fromWallet, toWallet, token, amount, tradeID)
	return args.Error(0)
}

func (m *MockBalanceService) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal, batchID string) error {
	args := m.Called(ctx, wallet, token, amount, batchID)
	return args.Error(0)
}

func (m *MockBalanceService) DeductFee(ctx context.Context, wallet, token string, amount decimal.Decimal, tradeID string) error {
	args := m.Called(ctx, wallet, token, amount, tradeID)
	return args.Error(0)
}

func (m *MockBalanceService) RefundWithdraw(ctx context.Context, wallet, token string, amount decimal.Decimal, withdrawID string) error {
	args := m.Called(ctx, wallet, token, amount, withdrawID)
	return args.Error(0)
}

func (m *MockBalanceService) GetBalanceLogs(ctx context.Context, wallet string, filter *repository.BalanceLogFilter, page *repository.Pagination) ([]*model.BalanceLog, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.BalanceLog), args.Error(1)
}

func (m *MockBalanceService) GetTotalFeeBalance(ctx context.Context, token string) (decimal.Decimal, error) {
	args := m.Called(ctx, token)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

// MockTradeService 成交服务 Mock
type MockTradeService struct {
	mock.Mock
}

func (m *MockTradeService) GetTrade(ctx context.Context, tradeID string) (*model.Trade, error) {
	args := m.Called(ctx, tradeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Trade), args.Error(1)
}

func (m *MockTradeService) ListTradesByOrder(ctx context.Context, orderID string) ([]*model.Trade, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeService) ListTrades(ctx context.Context, wallet string, filter *repository.TradeFilter, page *repository.Pagination) ([]*model.Trade, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeService) ListRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeService) CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*model.SettlementBatch, error) {
	args := m.Called(ctx, tradeIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SettlementBatch), args.Error(1)
}

func (m *MockTradeService) ConfirmSettlement(ctx context.Context, batchID, txHash string) error {
	args := m.Called(ctx, batchID, txHash)
	return args.Error(0)
}

func (m *MockTradeService) FailSettlement(ctx context.Context, batchID, reason string) error {
	args := m.Called(ctx, batchID, reason)
	return args.Error(0)
}

func (m *MockTradeService) RollbackTrades(ctx context.Context, tradeIDs []string) error {
	args := m.Called(ctx, tradeIDs)
	return args.Error(0)
}

// MockClearingService 清算服务 Mock
type MockClearingService struct {
	mock.Mock
}

func (m *MockClearingService) ProcessTradeResult(ctx context.Context, msg *worker.TradeResultMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockClearingService) HandleSettlementConfirm(ctx context.Context, msg *worker.SettlementConfirmedMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockClearingService) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockDepositService 充值服务 Mock
type MockDepositService struct {
	mock.Mock
}

func (m *MockDepositService) ProcessDepositEvent(ctx context.Context, event *service.DepositEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockDepositService) ConfirmDeposit(ctx context.Context, depositID string) error {
	args := m.Called(ctx, depositID)
	return args.Error(0)
}

func (m *MockDepositService) CreditDeposit(ctx context.Context, depositID string) error {
	args := m.Called(ctx, depositID)
	return args.Error(0)
}

func (m *MockDepositService) GetDeposit(ctx context.Context, depositID string) (*model.Deposit, error) {
	args := m.Called(ctx, depositID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Deposit), args.Error(1)
}

func (m *MockDepositService) GetDepositByTxHash(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error) {
	args := m.Called(ctx, txHash, logIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Deposit), args.Error(1)
}

func (m *MockDepositService) ListDeposits(ctx context.Context, wallet string, filter *repository.DepositFilter, page *repository.Pagination) ([]*model.Deposit, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositService) GetPendingDeposits(ctx context.Context, limit int) ([]*model.Deposit, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositService) GetConfirmedDeposits(ctx context.Context, limit int) ([]*model.Deposit, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositService) ProcessDeposit(ctx context.Context, msg *service.DepositMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

// MockWithdrawalService 提现服务 Mock
type MockWithdrawalService struct {
	mock.Mock
}

func (m *MockWithdrawalService) CreateWithdrawal(ctx context.Context, req *service.CreateWithdrawalRequest) (*model.Withdrawal, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalService) GetWithdrawal(ctx context.Context, withdrawID string) (*model.Withdrawal, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalService) ListWithdrawals(ctx context.Context, wallet string, filter *repository.WithdrawalFilter, page *repository.Pagination) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalService) ApproveWithdrawal(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalService) RejectWithdrawal(ctx context.Context, withdrawID, reason string) error {
	args := m.Called(ctx, withdrawID, reason)
	return args.Error(0)
}

func (m *MockWithdrawalService) SubmitWithdrawal(ctx context.Context, withdrawID, txHash string) error {
	args := m.Called(ctx, withdrawID, txHash)
	return args.Error(0)
}

func (m *MockWithdrawalService) ConfirmWithdrawal(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalService) FailWithdrawal(ctx context.Context, withdrawID, reason string) error {
	args := m.Called(ctx, withdrawID, reason)
	return args.Error(0)
}

func (m *MockWithdrawalService) CancelWithdrawal(ctx context.Context, wallet, withdrawID string) error {
	args := m.Called(ctx, wallet, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalService) GetPendingWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalService) GetSubmittedWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalService) HandleConfirm(ctx context.Context, msg *service.WithdrawalConfirmedMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

// ========== Test Helper ==========

func createTestHandler() (*TradingHandler, *MockOrderService, *MockBalanceService, *MockTradeService, *MockClearingService, *MockDepositService, *MockWithdrawalService) {
	orderService := new(MockOrderService)
	balanceService := new(MockBalanceService)
	tradeService := new(MockTradeService)
	clearingService := new(MockClearingService)
	depositService := new(MockDepositService)
	withdrawalService := new(MockWithdrawalService)

	handler := NewTradingHandler(
		orderService,
		balanceService,
		tradeService,
		clearingService,
		depositService,
		withdrawalService,
	)

	return handler, orderService, balanceService, tradeService, clearingService, depositService, withdrawalService
}

// ========== Order Tests ==========

func TestTradingHandler_CreateOrder_Success(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	expectedOrder := &model.Order{
		OrderID:         "O123456789",
		Wallet:          "0x1234567890abcdef",
		Market:          "ETH-USDC",
		Side:            model.OrderSideBuy,
		Type:            model.OrderTypeLimit,
		Price:           decimal.NewFromFloat(3000.0),
		Amount:          decimal.NewFromFloat(1.0),
		RemainingAmount: decimal.NewFromFloat(1.0),
		Status:          model.OrderStatusPending,
		Nonce:           12345,
		CreatedAt:       time.Now().UnixMilli(),
	}

	orderService.On("CreateOrder", ctx, mock.MatchedBy(func(req *service.CreateOrderRequest) bool {
		return req.Wallet == "0x1234567890abcdef" &&
			req.Market == "ETH-USDC" &&
			req.Side == model.OrderSideBuy
	})).Return(expectedOrder, nil)

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet:    "0x1234567890abcdef",
		Market:    "ETH-USDC",
		Side:      commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:      commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:     "3000.0",
		Amount:    "1.0",
		Nonce:     12345,
		Signature: []byte("test_signature"),
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "O123456789", resp.Order.OrderId)
	assert.Equal(t, commonv1.OrderSide_ORDER_SIDE_BUY, resp.Order.Side)
	orderService.AssertExpectations(t)
}

func TestTradingHandler_CreateOrder_MissingWallet(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "",
		Market: "ETH-USDC",
		Price:  "3000.0",
		Amount: "1.0",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "wallet")
}

func TestTradingHandler_CreateOrder_MissingMarket(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "0x1234567890abcdef",
		Market: "",
		Price:  "3000.0",
		Amount: "1.0",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "market")
}

func TestTradingHandler_CreateOrder_InvalidPrice(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "0x1234567890abcdef",
		Market: "ETH-USDC",
		Price:  "invalid_price",
		Amount: "1.0",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "price")
}

func TestTradingHandler_CreateOrder_InvalidAmount(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "0x1234567890abcdef",
		Market: "ETH-USDC",
		Price:  "3000.0",
		Amount: "not_a_number",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "amount")
}

func TestTradingHandler_CreateOrder_InsufficientBalance(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orderService.On("CreateOrder", ctx, mock.Anything).Return(nil, service.ErrInsufficientBalance)

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "0x1234567890abcdef",
		Market: "ETH-USDC",
		Side:   commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:   commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:  "3000.0",
		Amount: "100.0",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestTradingHandler_CreateOrder_DuplicateOrder(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orderService.On("CreateOrder", ctx, mock.Anything).Return(nil, service.ErrDuplicateOrder)

	resp, err := handler.CreateOrder(ctx, &pb.CreateOrderRequest{
		Wallet: "0x1234567890abcdef",
		Market: "ETH-USDC",
		Side:   commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:   commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:  "3000.0",
		Amount: "1.0",
		Nonce:  12345,
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
}

func TestTradingHandler_CancelOrder_Success(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orderService.On("CancelOrder", ctx, "0x1234567890abcdef", "O123456789").Return(nil)

	resp, err := handler.CancelOrder(ctx, &pb.CancelOrderRequest{
		Wallet:  "0x1234567890abcdef",
		OrderId: "O123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	orderService.AssertExpectations(t)
}

func TestTradingHandler_CancelOrder_MissingParams(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CancelOrder(ctx, &pb.CancelOrderRequest{
		Wallet:  "",
		OrderId: "O123456789",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTradingHandler_CancelOrder_NotCancellable(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orderService.On("CancelOrder", ctx, "0x1234567890abcdef", "O123456789").Return(service.ErrOrderNotCancellable)

	resp, err := handler.CancelOrder(ctx, &pb.CancelOrderRequest{
		Wallet:  "0x1234567890abcdef",
		OrderId: "O123456789",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestTradingHandler_GetOrder_Success(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	expectedOrder := &model.Order{
		OrderID:         "O123456789",
		Wallet:          "0x1234567890abcdef",
		Market:          "ETH-USDC",
		Side:            model.OrderSideBuy,
		Type:            model.OrderTypeLimit,
		Price:           decimal.NewFromFloat(3000.0),
		Amount:          decimal.NewFromFloat(1.0),
		FilledAmount:    decimal.NewFromFloat(0.5),
		RemainingAmount: decimal.NewFromFloat(0.5),
		Status:          model.OrderStatusPartial,
	}

	orderService.On("GetOrder", ctx, "O123456789").Return(expectedOrder, nil)

	resp, err := handler.GetOrder(ctx, &pb.GetOrderRequest{
		OrderId: "O123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "O123456789", resp.OrderId)
	assert.Equal(t, commonv1.OrderStatus_ORDER_STATUS_PARTIAL, resp.Status)
	orderService.AssertExpectations(t)
}

func TestTradingHandler_GetOrder_NotFound(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orderService.On("GetOrder", ctx, "O_NOT_FOUND").Return(nil, repository.ErrOrderNotFound)

	resp, err := handler.GetOrder(ctx, &pb.GetOrderRequest{
		OrderId: "O_NOT_FOUND",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestTradingHandler_ListOrders_Success(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orders := []*model.Order{
		{
			OrderID: "O1",
			Wallet:  "0x1234567890abcdef",
			Market:  "ETH-USDC",
			Status:  model.OrderStatusOpen,
		},
		{
			OrderID: "O2",
			Wallet:  "0x1234567890abcdef",
			Market:  "ETH-USDC",
			Status:  model.OrderStatusFilled,
		},
	}

	orderService.On("ListOrders", ctx, "0x1234567890abcdef", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			page := args.Get(3).(*repository.Pagination)
			page.Total = 2
		}).
		Return(orders, nil)

	resp, err := handler.ListOrders(ctx, &pb.ListOrdersRequest{
		Wallet:   "0x1234567890abcdef",
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, len(resp.Orders))
	assert.Equal(t, int64(2), resp.Total)
	orderService.AssertExpectations(t)
}

func TestTradingHandler_ListOpenOrders_Success(t *testing.T) {
	handler, orderService, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	orders := []*model.Order{
		{
			OrderID: "O1",
			Wallet:  "0x1234567890abcdef",
			Market:  "ETH-USDC",
			Status:  model.OrderStatusOpen,
		},
	}

	orderService.On("ListOpenOrders", ctx, "0x1234567890abcdef", "ETH-USDC").Return(orders, nil)

	resp, err := handler.ListOpenOrders(ctx, &pb.ListOpenOrdersRequest{
		Wallet: "0x1234567890abcdef",
		Market: "ETH-USDC",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.Orders))
	orderService.AssertExpectations(t)
}

// ========== Balance Tests ==========

func TestTradingHandler_GetBalance_Success(t *testing.T) {
	handler, _, balanceService, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	expectedBalance := &model.Balance{
		Wallet:           "0x1234567890abcdef",
		Token:            "USDC",
		SettledAvailable: decimal.NewFromFloat(1000.0),
		SettledFrozen:    decimal.NewFromFloat(100.0),
		PendingAvailable: decimal.NewFromFloat(50.0),
		PendingFrozen:    decimal.NewFromFloat(0),
	}

	balanceService.On("GetBalance", ctx, "0x1234567890abcdef", "USDC").Return(expectedBalance, nil)

	resp, err := handler.GetBalance(ctx, &pb.GetBalanceRequest{
		Wallet: "0x1234567890abcdef",
		Token:  "USDC",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "USDC", resp.Token)
	assert.Equal(t, "1000", resp.SettledAvailable)
	balanceService.AssertExpectations(t)
}

func TestTradingHandler_GetBalance_MissingParams(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.GetBalance(ctx, &pb.GetBalanceRequest{
		Wallet: "0x1234567890abcdef",
		Token:  "",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTradingHandler_GetBalances_Success(t *testing.T) {
	handler, _, balanceService, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	balances := []*model.Balance{
		{
			Wallet:           "0x1234567890abcdef",
			Token:            "USDC",
			SettledAvailable: decimal.NewFromFloat(1000.0),
		},
		{
			Wallet:           "0x1234567890abcdef",
			Token:            "ETH",
			SettledAvailable: decimal.NewFromFloat(2.5),
		},
	}

	balanceService.On("GetBalances", ctx, "0x1234567890abcdef").Return(balances, nil)

	resp, err := handler.GetBalances(ctx, &pb.GetBalancesRequest{
		Wallet: "0x1234567890abcdef",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, len(resp.Balances))
	balanceService.AssertExpectations(t)
}

func TestTradingHandler_GetBalanceLogs_Success(t *testing.T) {
	handler, _, balanceService, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	logs := []*model.BalanceLog{
		{
			ID:     1,
			Wallet: "0x1234567890abcdef",
			Token:  "USDC",
			Type:   model.BalanceLogTypeDeposit,
			Amount: decimal.NewFromFloat(1000.0),
		},
	}

	balanceService.On("GetBalanceLogs", ctx, "0x1234567890abcdef", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			page := args.Get(3).(*repository.Pagination)
			page.Total = 1
		}).
		Return(logs, nil)

	resp, err := handler.GetBalanceLogs(ctx, &pb.GetBalanceLogsRequest{
		Wallet:   "0x1234567890abcdef",
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.Logs))
	balanceService.AssertExpectations(t)
}

// ========== Trade Tests ==========

func TestTradingHandler_GetTrade_Success(t *testing.T) {
	handler, _, _, tradeService, _, _, _ := createTestHandler()
	ctx := context.Background()

	expectedTrade := &model.Trade{
		TradeID:          "T123456789",
		Market:           "ETH-USDC",
		MakerOrderID:     "O1",
		TakerOrderID:     "O2",
		MakerWallet:      "0x1111",
		TakerWallet:      "0x2222",
		Price:            decimal.NewFromFloat(3000.0),
		Amount:           decimal.NewFromFloat(1.0),
		QuoteAmount:      decimal.NewFromFloat(3000.0),
		SettlementStatus: model.SettlementStatusMatchedOffchain,
	}

	tradeService.On("GetTrade", ctx, "T123456789").Return(expectedTrade, nil)

	resp, err := handler.GetTrade(ctx, &pb.GetTradeRequest{
		TradeId: "T123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "T123456789", resp.TradeId)
	tradeService.AssertExpectations(t)
}

func TestTradingHandler_GetTrade_NotFound(t *testing.T) {
	handler, _, _, tradeService, _, _, _ := createTestHandler()
	ctx := context.Background()

	tradeService.On("GetTrade", ctx, "T_NOT_FOUND").Return(nil, repository.ErrTradeNotFound)

	resp, err := handler.GetTrade(ctx, &pb.GetTradeRequest{
		TradeId: "T_NOT_FOUND",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestTradingHandler_ListTrades_Success(t *testing.T) {
	handler, _, _, tradeService, _, _, _ := createTestHandler()
	ctx := context.Background()

	trades := []*model.Trade{
		{
			TradeID: "T1",
			Market:  "ETH-USDC",
		},
	}

	tradeService.On("ListTrades", ctx, "0x1234567890abcdef", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			page := args.Get(3).(*repository.Pagination)
			page.Total = 1
		}).
		Return(trades, nil)

	resp, err := handler.ListTrades(ctx, &pb.ListTradesRequest{
		Wallet:   "0x1234567890abcdef",
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.Trades))
	tradeService.AssertExpectations(t)
}

func TestTradingHandler_ListTradesByOrder_Success(t *testing.T) {
	handler, _, _, tradeService, _, _, _ := createTestHandler()
	ctx := context.Background()

	trades := []*model.Trade{
		{
			TradeID:      "T1",
			MakerOrderID: "O123",
		},
		{
			TradeID:      "T2",
			TakerOrderID: "O123",
		},
	}

	tradeService.On("ListTradesByOrder", ctx, "O123").Return(trades, nil)

	resp, err := handler.ListTradesByOrder(ctx, &pb.ListTradesByOrderRequest{
		OrderId: "O123",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, len(resp.Trades))
	tradeService.AssertExpectations(t)
}

// ========== Deposit Tests ==========

func TestTradingHandler_GetDeposit_Success(t *testing.T) {
	handler, _, _, _, _, depositService, _ := createTestHandler()
	ctx := context.Background()

	expectedDeposit := &model.Deposit{
		DepositID: "D123456789",
		Wallet:    "0x1234567890abcdef",
		Token:     "USDC",
		Amount:    decimal.NewFromFloat(1000.0),
		Status:    model.DepositStatusCredited,
	}

	depositService.On("GetDeposit", ctx, "D123456789").Return(expectedDeposit, nil)

	resp, err := handler.GetDeposit(ctx, &pb.GetDepositRequest{
		DepositId: "D123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "D123456789", resp.DepositId)
	depositService.AssertExpectations(t)
}

func TestTradingHandler_GetDeposit_NotFound(t *testing.T) {
	handler, _, _, _, _, depositService, _ := createTestHandler()
	ctx := context.Background()

	depositService.On("GetDeposit", ctx, "D_NOT_FOUND").Return(nil, repository.ErrDepositNotFound)

	resp, err := handler.GetDeposit(ctx, &pb.GetDepositRequest{
		DepositId: "D_NOT_FOUND",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestTradingHandler_ListDeposits_Success(t *testing.T) {
	handler, _, _, _, _, depositService, _ := createTestHandler()
	ctx := context.Background()

	deposits := []*model.Deposit{
		{
			DepositID: "D1",
			Wallet:    "0x1234567890abcdef",
			Token:     "USDC",
		},
	}

	depositService.On("ListDeposits", ctx, "0x1234567890abcdef", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			page := args.Get(3).(*repository.Pagination)
			page.Total = 1
		}).
		Return(deposits, nil)

	resp, err := handler.ListDeposits(ctx, &pb.ListDepositsRequest{
		Wallet:   "0x1234567890abcdef",
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.Deposits))
	depositService.AssertExpectations(t)
}

// ========== Withdrawal Tests ==========

func TestTradingHandler_CreateWithdrawal_Success(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	expectedWithdrawal := &model.Withdrawal{
		WithdrawID: "W123456789",
		Wallet:     "0x1234567890abcdef",
		Token:      "USDC",
		Amount:     decimal.NewFromFloat(500.0),
		ToAddress:  "0xdestination",
		Status:     model.WithdrawStatusPending,
	}

	withdrawalService.On("CreateWithdrawal", ctx, mock.MatchedBy(func(req *service.CreateWithdrawalRequest) bool {
		return req.Wallet == "0x1234567890abcdef" &&
			req.Token == "USDC" &&
			req.Amount.Equal(decimal.NewFromFloat(500.0))
	})).Return(expectedWithdrawal, nil)

	resp, err := handler.CreateWithdrawal(ctx, &pb.CreateWithdrawalRequest{
		Wallet:    "0x1234567890abcdef",
		Token:     "USDC",
		Amount:    "500.0",
		ToAddress: "0xdestination",
		Nonce:     12345,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "W123456789", resp.Withdrawal.WithdrawId)
	withdrawalService.AssertExpectations(t)
}

func TestTradingHandler_CreateWithdrawal_InvalidAmount(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.CreateWithdrawal(ctx, &pb.CreateWithdrawalRequest{
		Wallet:    "0x1234567890abcdef",
		Token:     "USDC",
		Amount:    "invalid_amount",
		ToAddress: "0xdestination",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTradingHandler_CreateWithdrawal_InsufficientBalance(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	withdrawalService.On("CreateWithdrawal", ctx, mock.Anything).Return(nil, service.ErrInsufficientBalance)

	resp, err := handler.CreateWithdrawal(ctx, &pb.CreateWithdrawalRequest{
		Wallet:    "0x1234567890abcdef",
		Token:     "USDC",
		Amount:    "10000.0",
		ToAddress: "0xdestination",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestTradingHandler_CancelWithdrawal_Success(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	withdrawalService.On("CancelWithdrawal", ctx, "0x1234567890abcdef", "W123456789").Return(nil)

	resp, err := handler.CancelWithdrawal(ctx, &pb.CancelWithdrawalRequest{
		Wallet:     "0x1234567890abcdef",
		WithdrawId: "W123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	withdrawalService.AssertExpectations(t)
}

func TestTradingHandler_CancelWithdrawal_NotPending(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	withdrawalService.On("CancelWithdrawal", ctx, "0x1234567890abcdef", "W123456789").Return(service.ErrWithdrawalNotPending)

	resp, err := handler.CancelWithdrawal(ctx, &pb.CancelWithdrawalRequest{
		Wallet:     "0x1234567890abcdef",
		WithdrawId: "W123456789",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestTradingHandler_GetWithdrawal_Success(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	expectedWithdrawal := &model.Withdrawal{
		WithdrawID: "W123456789",
		Wallet:     "0x1234567890abcdef",
		Token:      "USDC",
		Amount:     decimal.NewFromFloat(500.0),
		Status:     model.WithdrawStatusConfirmed,
	}

	withdrawalService.On("GetWithdrawal", ctx, "W123456789").Return(expectedWithdrawal, nil)

	resp, err := handler.GetWithdrawal(ctx, &pb.GetWithdrawalRequest{
		WithdrawId: "W123456789",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "W123456789", resp.WithdrawId)
	withdrawalService.AssertExpectations(t)
}

func TestTradingHandler_ListWithdrawals_Success(t *testing.T) {
	handler, _, _, _, _, _, withdrawalService := createTestHandler()
	ctx := context.Background()

	withdrawals := []*model.Withdrawal{
		{
			WithdrawID: "W1",
			Wallet:     "0x1234567890abcdef",
			Token:      "USDC",
		},
	}

	withdrawalService.On("ListWithdrawals", ctx, "0x1234567890abcdef", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			page := args.Get(3).(*repository.Pagination)
			page.Total = 1
		}).
		Return(withdrawals, nil)

	resp, err := handler.ListWithdrawals(ctx, &pb.ListWithdrawalsRequest{
		Wallet:   "0x1234567890abcdef",
		Page:     1,
		PageSize: 10,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, len(resp.Withdrawals))
	withdrawalService.AssertExpectations(t)
}

// ========== Internal API Tests ==========

func TestTradingHandler_ProcessTradeResult_Success(t *testing.T) {
	handler, _, _, _, clearingService, _, _ := createTestHandler()
	ctx := context.Background()

	clearingService.On("ProcessTradeResult", ctx, mock.MatchedBy(func(msg *worker.TradeResultMessage) bool {
		return msg.TradeID == "T123456789" &&
			msg.Market == "ETH-USDC" &&
			msg.Price == "3000.0"
	})).Return(nil)

	resp, err := handler.ProcessTradeResult(ctx, &pb.ProcessTradeResultRequest{
		TradeId:      "T123456789",
		Market:       "ETH-USDC",
		MakerOrderId: "O1",
		TakerOrderId: "O2",
		MakerWallet:  "0x1111",
		TakerWallet:  "0x2222",
		Price:        "3000.0",
		Amount:       "1.0",
		QuoteAmount:  "3000.0",
		MakerFee:     "1.5",
		TakerFee:     "3.0",
		MakerSide:    commonv1.OrderSide_ORDER_SIDE_BUY,
		MatchedAt:    time.Now().UnixMilli(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	clearingService.AssertExpectations(t)
}

func TestTradingHandler_ProcessTradeResult_Error(t *testing.T) {
	handler, _, _, _, clearingService, _, _ := createTestHandler()
	ctx := context.Background()

	clearingService.On("ProcessTradeResult", ctx, mock.Anything).Return(errors.New("clearing failed"))

	resp, err := handler.ProcessTradeResult(ctx, &pb.ProcessTradeResultRequest{
		TradeId:     "T123456789",
		Market:      "ETH-USDC",
		Price:       "3000.0",
		Amount:      "1.0",
		QuoteAmount: "3000.0",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestTradingHandler_ProcessDepositEvent_Success(t *testing.T) {
	handler, _, _, _, _, depositService, _ := createTestHandler()
	ctx := context.Background()

	depositService.On("ProcessDepositEvent", ctx, mock.MatchedBy(func(event *service.DepositEvent) bool {
		return event.TxHash == "0xabc123" &&
			event.Wallet == "0x1234567890abcdef" &&
			event.Token == "USDC" &&
			event.Amount.Equal(decimal.NewFromFloat(1000.0))
	})).Return(nil)

	resp, err := handler.ProcessDepositEvent(ctx, &pb.ProcessDepositEventRequest{
		TxHash:     "0xabc123",
		LogIndex:   0,
		BlockNum:   12345678,
		Wallet:     "0x1234567890abcdef",
		Token:      "USDC",
		Amount:     "1000.0",
		DetectedAt: time.Now().UnixMilli(),
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	depositService.AssertExpectations(t)
}

func TestTradingHandler_ProcessDepositEvent_InvalidAmount(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.ProcessDepositEvent(ctx, &pb.ProcessDepositEventRequest{
		TxHash: "0xabc123",
		Wallet: "0x1234567890abcdef",
		Token:  "USDC",
		Amount: "invalid",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTradingHandler_ConfirmSettlement_Success(t *testing.T) {
	handler, _, _, tradeService, _, _, _ := createTestHandler()
	ctx := context.Background()

	tradeService.On("ConfirmSettlement", ctx, "B123456789", "0xdef456").Return(nil)

	resp, err := handler.ConfirmSettlement(ctx, &pb.ConfirmSettlementRequest{
		BatchId: "B123456789",
		TxHash:  "0xdef456",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	tradeService.AssertExpectations(t)
}

func TestTradingHandler_ConfirmSettlement_MissingParams(t *testing.T) {
	handler, _, _, _, _, _, _ := createTestHandler()
	ctx := context.Background()

	resp, err := handler.ConfirmSettlement(ctx, &pb.ConfirmSettlementRequest{
		BatchId: "",
		TxHash:  "0xdef456",
	})

	assert.Nil(t, resp)
	assert.Error(t, err)
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

// ========== Error Mapping Tests ==========

func TestHandleServiceError_MapsCorrectly(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		expectedCode codes.Code
	}{
		{
			name:         "InvalidOrder",
			inputErr:     service.ErrInvalidOrder,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "OrderNotCancellable",
			inputErr:     service.ErrOrderNotCancellable,
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "DuplicateOrder",
			inputErr:     service.ErrDuplicateOrder,
			expectedCode: codes.AlreadyExists,
		},
		{
			name:         "InvalidNonce",
			inputErr:     service.ErrInvalidNonce,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "InsufficientBalance",
			inputErr:     service.ErrInsufficientBalance,
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "InvalidWithdrawal",
			inputErr:     service.ErrInvalidWithdrawal,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "WithdrawalNotFound",
			inputErr:     service.ErrWithdrawalNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "WithdrawalNotPending",
			inputErr:     service.ErrWithdrawalNotPending,
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "OrderNotFound",
			inputErr:     repository.ErrOrderNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "TradeNotFound",
			inputErr:     repository.ErrTradeNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "DepositNotFound",
			inputErr:     repository.ErrDepositNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "BalanceNotFound",
			inputErr:     repository.ErrBalanceNotFound,
			expectedCode: codes.NotFound,
		},
		{
			name:         "UnknownError",
			inputErr:     errors.New("unknown error"),
			expectedCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := handleServiceError(tt.inputErr)
			st, ok := status.FromError(grpcErr)
			assert.True(t, ok)
			assert.Equal(t, tt.expectedCode, st.Code())
		})
	}
}
