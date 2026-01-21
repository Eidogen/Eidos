package service

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// MockOrderService implements OrderService for testing
type MockOrderService struct {
	mock.Mock
}

func (m *MockOrderService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*model.Order, error) {
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

func (m *MockOrderService) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderService) ListOpenOrders(ctx context.Context, wallet, market string) ([]*model.Order, error) {
	args := m.Called(ctx, wallet, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderService) ExpireOrder(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockOrderService) RejectOrder(ctx context.Context, orderID, reason string) error {
	args := m.Called(ctx, orderID, reason)
	return args.Error(0)
}

func (m *MockOrderService) HandleCancelConfirm(ctx context.Context, msg *OrderCancelledMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockOrderService) UpdateOrderFilled(ctx context.Context, orderID string, filledAmount, filledQuote decimal.Decimal, newStatus model.OrderStatus) error {
	args := m.Called(ctx, orderID, filledAmount, filledQuote, newStatus)
	return args.Error(0)
}

func (m *MockOrderService) CancelOrderByNonce(ctx context.Context, wallet string, nonce uint64, signature []byte) error {
	args := m.Called(ctx, wallet, nonce, signature)
	return args.Error(0)
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

func (m *MockOrderService) HandleOrderAccepted(ctx context.Context, msg *OrderAcceptedMessage) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_NoOrders(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	// No open orders
	orderRepo.On("ListOpenOrders", ctx, "", "").Return([]*model.Order{}, nil)

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err)
	assert.Empty(t, results)
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_SufficientBalance(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
		},
	}

	// Order with sufficient balance
	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(500),
		SettledFrozen:    decimal.NewFromFloat(1000),
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
	}, nil)

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err)
	assert.Empty(t, results) // No issues found
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_InsufficientBalance(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
			CreatedAt:    1000,
		},
		{
			OrderID:      "O2",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(500),
			Status:       model.OrderStatusOpen,
			CreatedAt:    2000, // Newer order, should be cancelled first
		},
	}

	// Total frozen = 1500, but only 800 available -> shortfall = 700
	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(300),
		SettledFrozen:    decimal.NewFromFloat(500),
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
	}, nil)

	// Should cancel orders starting from newest (O2 first, then O1) to cover the shortfall (700)
	// O2 (500) is not enough, so O1 (1000) will also be cancelled
	orderService.On("CancelOrder", ctx, wallet, "O2").Return(nil)
	orderService.On("CancelOrder", ctx, wallet, "O1").Return(nil)

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, wallet, results[0].Wallet)
	assert.Equal(t, "USDT", results[0].Token)
	assert.Len(t, results[0].CancelledOrders, 2)
	assert.Contains(t, results[0].CancelledOrders, "O2")
	assert.Contains(t, results[0].CancelledOrders, "O1")
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_DryRun(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	cfg.DryRun = true // Enable dry run
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
			CreatedAt:    1000,
		},
	}

	// Insufficient balance
	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(100),
		SettledFrozen:    decimal.Zero,
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
	}, nil)

	// In dry run mode, CancelOrder should NOT be called

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Len(t, results[0].OrdersToCancel, 1)
	assert.Empty(t, results[0].CancelledOrders) // Not cancelled in dry run
	orderService.AssertNotCalled(t, "CancelOrder")
}

func TestBalanceMonitorService_CheckWalletBalance(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(500),
			Status:       model.OrderStatusOpen,
		},
		{
			OrderID:      "O2",
			Wallet:       wallet,
			FreezeToken:  "ETH", // Different token
			FreezeAmount: decimal.NewFromFloat(1),
			Status:       model.OrderStatusOpen,
		},
	}

	orderRepo.On("ListOpenOrders", ctx, wallet, "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, token).Return(&cache.RedisBalance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: decimal.NewFromFloat(200),
		SettledFrozen:    decimal.NewFromFloat(100),
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
	}, nil)

	result, err := svc.CheckWalletBalance(ctx, wallet, token)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, wallet, result.Wallet)
	assert.Equal(t, token, result.Token)
	assert.True(t, result.TotalFrozen.Equal(decimal.NewFromFloat(500)))
	// Available = 200 + 100 = 300, Frozen = 500, Shortfall = 200
	assert.True(t, result.ShortfallAmount.Equal(decimal.NewFromFloat(200)))
}

func TestBalanceMonitorService_GetInsufficientBalanceWallets(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet1 := "0x1111111111111111111111111111111111111111"
	wallet2 := "0x2222222222222222222222222222222222222222"

	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet1,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
		},
		{
			OrderID:      "O2",
			Wallet:       wallet2,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(500),
			Status:       model.OrderStatusOpen,
		},
	}

	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)

	// Wallet1 has insufficient balance
	balanceCache.On("GetBalance", ctx, wallet1, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet1,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(100),
		SettledFrozen:    decimal.NewFromFloat(100),
	}, nil)

	// Wallet2 has sufficient balance
	balanceCache.On("GetBalance", ctx, wallet2, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet2,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(1000),
		SettledFrozen:    decimal.NewFromFloat(500),
	}, nil)

	wallets, err := svc.GetInsufficientBalanceWallets(ctx, 10)

	assert.NoError(t, err)
	assert.Len(t, wallets, 1)
	assert.Contains(t, wallets, wallet1)
}

func TestBalanceMonitorService_StartStop(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	cfg.CheckInterval = 100 * 1000 // Very long to avoid actual checks
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	// Start the service
	svc.Start(ctx)
	assert.True(t, svc.IsRunning())

	// Stop the service
	svc.Stop()
	assert.False(t, svc.IsRunning())
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_CancelFails(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
			CreatedAt:    1000,
		},
	}

	// Insufficient balance
	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, "USDT").Return(&cache.RedisBalance{
		Wallet:           wallet,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(100),
		SettledFrozen:    decimal.Zero,
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
	}, nil)

	// Cancel fails
	orderService.On("CancelOrder", ctx, wallet, "O1").Return(errors.New("cancel failed"))

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err) // Main function doesn't error even if cancels fail
	assert.Len(t, results, 1)
	assert.Len(t, results[0].FailedOrders, 1)
	assert.Contains(t, results[0].FailedOrders, "O1")
}

func TestBalanceMonitorService_CheckAndCancelInsufficientOrders_BalanceNotFound(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceCache := new(MockBalanceRedisRepository)
	orderService := new(MockOrderService)
	marketCfg := new(MockMarketConfigProvider)

	cfg := DefaultBalanceMonitorConfig()
	svc := NewBalanceMonitorService(cfg, orderRepo, balanceCache, orderService, marketCfg)

	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	orders := []*model.Order{
		{
			OrderID:      "O1",
			Wallet:       wallet,
			FreezeToken:  "USDT",
			FreezeAmount: decimal.NewFromFloat(1000),
			Status:       model.OrderStatusOpen,
			CreatedAt:    1000,
		},
	}

	// Balance not found (new user or bug)
	orderRepo.On("ListOpenOrders", ctx, "", "").Return(orders, nil)
	balanceCache.On("GetBalance", ctx, wallet, "USDT").Return(nil, cache.ErrRedisBalanceNotFound)

	// Should cancel the order since balance doesn't exist
	orderService.On("CancelOrder", ctx, wallet, "O1").Return(nil)

	results, err := svc.CheckAndCancelInsufficientOrders(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Len(t, results[0].CancelledOrders, 1)
}
