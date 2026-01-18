package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// ========== Mock Implementations ==========

// MockOrderRepository 订单仓储模拟
type MockOrderRepository struct {
	mock.Mock
}

func (m *MockOrderRepository) Create(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockOrderRepository) GetByID(ctx context.Context, id int64) (*model.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderRepository) GetByOrderID(ctx context.Context, orderID string) (*model.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderRepository) GetByClientOrderID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error) {
	args := m.Called(ctx, wallet, clientOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderRepository) GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Order, error) {
	args := m.Called(ctx, wallet, nonce)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Order), args.Error(1)
}

func (m *MockOrderRepository) ListByWallet(ctx context.Context, wallet string, filter *repository.OrderFilter, page *repository.Pagination) ([]*model.Order, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderRepository) ListByMarket(ctx context.Context, market string, filter *repository.OrderFilter, page *repository.Pagination) ([]*model.Order, error) {
	args := m.Called(ctx, market, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderRepository) ListOpenOrders(ctx context.Context, wallet string, market string) ([]*model.Order, error) {
	args := m.Called(ctx, wallet, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

func (m *MockOrderRepository) Update(ctx context.Context, order *model.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *MockOrderRepository) UpdateStatus(ctx context.Context, orderID string, oldStatus, newStatus model.OrderStatus) error {
	args := m.Called(ctx, orderID, oldStatus, newStatus)
	return args.Error(0)
}

func (m *MockOrderRepository) UpdateFilled(ctx context.Context, orderID string, filledAmount, filledQuote string, newStatus model.OrderStatus) error {
	args := m.Called(ctx, orderID, filledAmount, filledQuote, newStatus)
	return args.Error(0)
}

func (m *MockOrderRepository) BatchCreate(ctx context.Context, orders []*model.Order) error {
	args := m.Called(ctx, orders)
	return args.Error(0)
}

func (m *MockOrderRepository) CountByWallet(ctx context.Context, wallet string, filter *repository.OrderFilter) (int64, error) {
	args := m.Called(ctx, wallet, filter)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockOrderRepository) ListExpiredOrders(ctx context.Context, expireBefore int64, status model.OrderStatus, limit int) ([]*model.Order, error) {
	args := m.Called(ctx, expireBefore, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Order), args.Error(1)
}

// MockBalanceRepository 余额仓储模拟
type MockBalanceRepository struct {
	mock.Mock
	txFunc func(context.Context) error
}

func (m *MockBalanceRepository) GetByWalletToken(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *MockBalanceRepository) ListByWallet(ctx context.Context, wallet string) ([]*model.Balance, error) {
	args := m.Called(ctx, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Balance), args.Error(1)
}

func (m *MockBalanceRepository) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, settled bool) error {
	args := m.Called(ctx, wallet, token, amount, settled)
	return args.Error(0)
}

func (m *MockBalanceRepository) Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, settled bool) error {
	args := m.Called(ctx, wallet, token, amount, settled)
	return args.Error(0)
}

func (m *MockBalanceRepository) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, settled bool) error {
	args := m.Called(ctx, wallet, token, amount, settled)
	return args.Error(0)
}

func (m *MockBalanceRepository) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, settled bool) error {
	args := m.Called(ctx, wallet, token, amount, settled)
	return args.Error(0)
}

func (m *MockBalanceRepository) CreateBalanceLog(ctx context.Context, log *model.BalanceLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func (m *MockBalanceRepository) ListBalanceLogs(ctx context.Context, wallet string, filter *repository.BalanceLogFilter, page *repository.Pagination) ([]*model.BalanceLog, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.BalanceLog), args.Error(1)
}

func (m *MockBalanceRepository) Transaction(ctx context.Context, fn func(context.Context) error) error {
	// 执行事务函数
	if m.txFunc != nil {
		return m.txFunc(ctx)
	}
	return fn(ctx)
}

func (m *MockBalanceRepository) GetOrCreate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *MockBalanceRepository) GetByWalletTokenForUpdate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *MockBalanceRepository) Update(ctx context.Context, balance *model.Balance) error {
	args := m.Called(ctx, balance)
	return args.Error(0)
}

func (m *MockBalanceRepository) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, fromWallet, toWallet, token, amount)
	return args.Error(0)
}

func (m *MockBalanceRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, wallet, token, amount)
	return args.Error(0)
}

func (m *MockBalanceRepository) GetFeeAccount(ctx context.Context, bucketID int, token string) (*model.FeeAccount, error) {
	args := m.Called(ctx, bucketID, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.FeeAccount), args.Error(1)
}

func (m *MockBalanceRepository) CreditFeeAccount(ctx context.Context, bucketID int, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, bucketID, token, amount)
	return args.Error(0)
}

func (m *MockBalanceRepository) UpsertBalance(ctx context.Context, balance *model.Balance) error {
	args := m.Called(ctx, balance)
	return args.Error(0)
}

func (m *MockBalanceRepository) ListBalances(ctx context.Context, offset, limit int) ([]*model.Balance, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Balance), args.Error(1)
}

// MockNonceRepository Nonce 仓储模拟
type MockNonceRepository struct {
	mock.Mock
}

func (m *MockNonceRepository) IsUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64) (bool, error) {
	args := m.Called(ctx, wallet, usage, nonce)
	return args.Bool(0), args.Error(1)
}

func (m *MockNonceRepository) MarkUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, refID string) error {
	args := m.Called(ctx, wallet, usage, nonce, refID)
	return args.Error(0)
}

func (m *MockNonceRepository) MarkUsedWithTx(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, refID string) error {
	args := m.Called(ctx, wallet, usage, nonce, refID)
	return args.Error(0)
}

func (m *MockNonceRepository) GetUsedNonces(ctx context.Context, wallet string, usage model.NonceUsage, limit int) ([]uint64, error) {
	args := m.Called(ctx, wallet, usage, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uint64), args.Error(1)
}

func (m *MockNonceRepository) GetLatestNonce(ctx context.Context, wallet string, usage model.NonceUsage) (uint64, error) {
	args := m.Called(ctx, wallet, usage)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockNonceRepository) CleanExpired(ctx context.Context, beforeTime int64, batchSize int) (int64, error) {
	args := m.Called(ctx, beforeTime, batchSize)
	return args.Get(0).(int64), args.Error(1)
}

// MockIDGenerator ID 生成器模拟
type MockIDGenerator struct {
	mock.Mock
}

func (m *MockIDGenerator) Generate() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// MockMarketConfigProvider 市场配置提供者模拟
type MockMarketConfigProvider struct {
	mock.Mock
}

func (m *MockMarketConfigProvider) GetMarket(market string) (*MarketConfig, error) {
	args := m.Called(market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MarketConfig), args.Error(1)
}

func (m *MockMarketConfigProvider) IsValidMarket(market string) bool {
	args := m.Called(market)
	return args.Bool(0)
}

// MockBalanceRedisRepository Redis 资金仓储模拟
type MockBalanceRedisRepository struct {
	mock.Mock
}

func (m *MockBalanceRedisRepository) GetBalance(ctx context.Context, wallet, token string) (*cache.RedisBalance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cache.RedisBalance), args.Error(1)
}

func (m *MockBalanceRedisRepository) GetOrCreateBalance(ctx context.Context, wallet, token string) (*cache.RedisBalance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cache.RedisBalance), args.Error(1)
}

func (m *MockBalanceRedisRepository) GetBalancesBatch(ctx context.Context, wallet string, tokens []string) ([]*cache.RedisBalance, error) {
	args := m.Called(ctx, wallet, tokens)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*cache.RedisBalance), args.Error(1)
}

func (m *MockBalanceRedisRepository) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool, orderID string) error {
	args := m.Called(ctx, wallet, token, amount, fromSettled, orderID)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) FreezeForOrder(ctx context.Context, req *cache.FreezeForOrderRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	args := m.Called(ctx, wallet, token, amount, toSettled)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) UnfreezeByOrderID(ctx context.Context, wallet, token, orderID string) error {
	args := m.Called(ctx, wallet, token, orderID)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	args := m.Called(ctx, wallet, token, amount, toSettled)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error {
	args := m.Called(ctx, wallet, token, amount, fromSettled)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) ClearTrade(ctx context.Context, req *cache.ClearTradeRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, fromSettled bool) error {
	args := m.Called(ctx, fromWallet, toWallet, token, amount, fromSettled)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, wallet, token, amount)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) GetOrderFreeze(ctx context.Context, orderID string) (*cache.OrderFreezeRecord, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*cache.OrderFreezeRecord), args.Error(1)
}

func (m *MockBalanceRedisRepository) UpdateOrderFreeze(ctx context.Context, orderID string, settledAmount, pendingAmount decimal.Decimal) error {
	args := m.Called(ctx, orderID, settledAmount, pendingAmount)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) DeleteOrderFreeze(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) CreditFeeBucket(ctx context.Context, market, token string, bucketID int, amount decimal.Decimal) error {
	args := m.Called(ctx, market, token, bucketID, amount)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) CheckTradeProcessed(ctx context.Context, tradeID string) (bool, error) {
	args := m.Called(ctx, tradeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockBalanceRedisRepository) MarkTradeProcessed(ctx context.Context, tradeID string, ttl time.Duration) error {
	args := m.Called(ctx, tradeID, ttl)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) GetUserPendingTotal(ctx context.Context, wallet string) (decimal.Decimal, error) {
	args := m.Called(ctx, wallet)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockBalanceRedisRepository) GetGlobalPendingTotal(ctx context.Context) (decimal.Decimal, error) {
	args := m.Called(ctx)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockBalanceRedisRepository) SyncBalanceFromDB(ctx context.Context, balance *model.Balance) error {
	args := m.Called(ctx, balance)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) UnfreezeForCancel(ctx context.Context, req *cache.UnfreezeForCancelRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) WriteCancelOutbox(ctx context.Context, req *cache.WriteCancelOutboxRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) CancelPendingOrder(ctx context.Context, req *cache.CancelPendingOrderRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) RollbackTrade(ctx context.Context, req *cache.RollbackTradeRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) DecrUserOpenOrders(ctx context.Context, wallet string) error {
	args := m.Called(ctx, wallet)
	return args.Error(0)
}

func (m *MockBalanceRedisRepository) GetUserOpenOrders(ctx context.Context, wallet string) (int, error) {
	args := m.Called(ctx, wallet)
	return args.Int(0), args.Error(1)
}

// ========== Test Cases ==========

func TestOrderService_CreateOrder_Success(t *testing.T) {
	// 准备 mock
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	// 创建服务
	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// 设置期望
	marketCfg.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:       "ETH-USDT",
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
	}, nil)

	orderRepo.On("GetByClientOrderID", ctx, req.Wallet, "").Return(nil, repository.ErrOrderNotFound)
	idGen.On("Generate").Return(int64(123456789), nil)
	// 新流程: 使用 Redis FreezeForOrder 代替 DB 事务
	balanceCache.On("FreezeForOrder", mock.Anything, mock.AnythingOfType("*cache.FreezeForOrderRequest")).Return(nil)
	// DB 持久化是异步的，设置期望但不验证
	nonceRepo.On("MarkUsedWithTx", mock.Anything, req.Wallet, model.NonceUsageOrder, uint64(1), "O123456789").Return(nil).Maybe()
	orderRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()
	balanceRepo.On("Transaction", mock.Anything, mock.Anything).Return(nil).Maybe()

	// 执行测试
	order, err := svc.CreateOrder(ctx, req)

	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, "O123456789", order.OrderID)
	assert.Equal(t, req.Wallet, order.Wallet)
	assert.Equal(t, req.Market, order.Market)
	assert.Equal(t, model.OrderSideBuy, order.Side)
	assert.Equal(t, model.OrderTypeLimit, order.Type)
	assert.Equal(t, model.OrderStatusPending, order.Status)
}

func TestOrderService_CreateOrder_InvalidWallet(t *testing.T) {
	svc := NewOrderService(nil, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "invalid",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		Signature: []byte("test"),
	}

	order, err := svc.CreateOrder(ctx, req)

	assert.Nil(t, order)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidOrder))
}

func TestOrderService_CreateOrder_InvalidMarket(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "INVALID-MARKET",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		Signature: []byte("test"),
	}

	marketCfg.On("GetMarket", "INVALID-MARKET").Return(nil, errors.New("market not found"))

	order, err := svc.CreateOrder(ctx, req)

	assert.Nil(t, order)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidOrder))
}

func TestOrderService_CreateOrder_DuplicateNonce(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		Signature: []byte("test"),
	}

	marketCfg.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:       "ETH-USDT",
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
	}, nil)
	orderRepo.On("GetByClientOrderID", ctx, req.Wallet, "").Return(nil, repository.ErrOrderNotFound)
	idGen.On("Generate").Return(int64(123456789), nil)
	// 新流程: Redis FreezeForOrder 返回 nonce 已使用错误
	balanceCache.On("FreezeForOrder", mock.Anything, mock.AnythingOfType("*cache.FreezeForOrderRequest")).Return(cache.ErrRedisNonceUsed)

	order, err := svc.CreateOrder(ctx, req)

	assert.Nil(t, order)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidNonce))
}

func TestOrderService_CreateOrder_InsufficientBalance(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	req := &CreateOrderRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "ETH-USDT",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(2000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		Signature: []byte("test"),
	}

	marketCfg.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:       "ETH-USDT",
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
	}, nil)
	orderRepo.On("GetByClientOrderID", ctx, req.Wallet, "").Return(nil, repository.ErrOrderNotFound)
	idGen.On("Generate").Return(int64(123456789), nil)
	// 新流程: Redis FreezeForOrder 返回余额不足错误
	balanceCache.On("FreezeForOrder", mock.Anything, mock.AnythingOfType("*cache.FreezeForOrderRequest")).Return(cache.ErrRedisInsufficientBalance)

	order, err := svc.CreateOrder(ctx, req)

	assert.Nil(t, order)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInsufficientBalance))
}

func TestOrderService_CreateOrder_IdempotentWithClientOrderID(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	existingOrder := &model.Order{
		OrderID:       "O123",
		Wallet:        "0x1234567890123456789012345678901234567890",
		Market:        "ETH-USDT",
		ClientOrderID: "client-123",
		Status:        model.OrderStatusOpen,
	}

	req := &CreateOrderRequest{
		Wallet:        "0x1234567890123456789012345678901234567890",
		Market:        "ETH-USDT",
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(2000),
		Amount:        decimal.NewFromFloat(1),
		Nonce:         1,
		Signature:     []byte("test"),
		ClientOrderID: "client-123",
	}

	marketCfg.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:       "ETH-USDT",
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
	}, nil)
	orderRepo.On("GetByClientOrderID", ctx, req.Wallet, "client-123").Return(existingOrder, nil)

	// 应返回已存在的订单
	order, err := svc.CreateOrder(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, "O123", order.OrderID)
}

func TestOrderService_CancelOrder_OpenOrder_Success(t *testing.T) {
	// 测试取消 OPEN 状态订单 (发送到撮合引擎)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID:      orderID,
		Wallet:       wallet,
		Market:       "ETH-USDT",
		Side:         model.OrderSideBuy,
		Type:         model.OrderTypeLimit,
		Status:       model.OrderStatusOpen,
		Amount:       decimal.NewFromFloat(1),
		FilledAmount: decimal.Zero,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2002),
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)
	// OPEN 订单: 只写入 Cancel Outbox，不解冻资金 (资金在 HandleCancelConfirm 时解冻)
	balanceCache.On("WriteCancelOutbox", ctx, mock.AnythingOfType("*cache.WriteCancelOutboxRequest")).Return(nil)
	// DB 异步更新订单状态为 CANCELLING
	orderRepo.On("UpdateStatus", mock.Anything, orderID, model.OrderStatusOpen, model.OrderStatusCancelling).Return(nil).Maybe()

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.NoError(t, err)
	// 等待异步 goroutine 完成
	time.Sleep(100 * time.Millisecond)
}

func TestOrderService_CancelOrder_PendingOrder_Success(t *testing.T) {
	// 测试取消 PENDING 状态订单 (直接在 Redis 中取消)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID:      orderID,
		Wallet:       wallet,
		Market:       "ETH-USDT",
		Side:         model.OrderSideBuy,
		Type:         model.OrderTypeLimit,
		Status:       model.OrderStatusPending, // PENDING 状态
		Amount:       decimal.NewFromFloat(1),
		FilledAmount: decimal.Zero,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2002),
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)
	// PENDING 订单: 使用 CancelPendingOrder 直接在 Redis 中原子取消
	balanceCache.On("CancelPendingOrder", ctx, mock.AnythingOfType("*cache.CancelPendingOrderRequest")).Return(nil)
	// DB 异步更新
	balanceRepo.On("Transaction", mock.Anything, mock.Anything).Return(nil).Maybe()
	orderRepo.On("UpdateStatus", mock.Anything, orderID, model.OrderStatusPending, model.OrderStatusCancelled).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.NoError(t, err)
	// 等待异步 goroutine 完成
	time.Sleep(100 * time.Millisecond)
}

func TestOrderService_CancelOrder_PendingOrderAlreadySent(t *testing.T) {
	// 测试取消 PENDING 订单但订单已发送到撮合引擎 (回退到 OPEN 订单取消流程)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID:      orderID,
		Wallet:       wallet,
		Market:       "ETH-USDT",
		Side:         model.OrderSideBuy,
		Type:         model.OrderTypeLimit,
		Status:       model.OrderStatusPending, // PENDING 状态
		Amount:       decimal.NewFromFloat(1),
		FilledAmount: decimal.Zero,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2002),
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)
	// CancelPendingOrder 返回 ErrRedisOrderAlreadySent，表示订单已发送到撮合引擎
	balanceCache.On("CancelPendingOrder", ctx, mock.AnythingOfType("*cache.CancelPendingOrderRequest")).Return(cache.ErrRedisOrderAlreadySent)
	// 回退到 OPEN 订单取消流程
	balanceCache.On("WriteCancelOutbox", ctx, mock.AnythingOfType("*cache.WriteCancelOutboxRequest")).Return(nil)
	orderRepo.On("UpdateStatus", mock.Anything, orderID, model.OrderStatusPending, model.OrderStatusCancelling).Return(nil).Maybe()

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
}

func TestOrderService_CancelOrder_NotFound(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	orderRepo.On("GetByOrderID", ctx, orderID).Return(nil, repository.ErrOrderNotFound)

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrOrderNotFound))
}

func TestOrderService_CancelOrder_WrongWallet(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID: orderID,
		Wallet:  "0x9999999999999999999999999999999999999999",
		Status:  model.OrderStatusOpen,
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrOrderNotFound))
}

func TestOrderService_CancelOrder_NotCancellable(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	nonceRepo := new(MockNonceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewOrderService(orderRepo, balanceRepo, nonceRepo, balanceCache, idGen, marketCfg, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID: orderID,
		Wallet:  wallet,
		Status:  model.OrderStatusFilled, // 已成交，不可取消
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)

	err := svc.CancelOrder(ctx, wallet, orderID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrOrderNotCancellable))
}

func TestOrderService_GetOrder_Success(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	svc := NewOrderService(orderRepo, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	orderID := "O123456789"

	expectedOrder := &model.Order{
		OrderID: orderID,
		Wallet:  "0x1234567890123456789012345678901234567890",
		Market:  "ETH-USDT",
		Status:  model.OrderStatusOpen,
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(expectedOrder, nil)

	order, err := svc.GetOrder(ctx, orderID)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, orderID, order.OrderID)
}

func TestOrderService_GetOrder_NotFound(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	svc := NewOrderService(orderRepo, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	orderID := "O123456789"

	orderRepo.On("GetByOrderID", ctx, orderID).Return(nil, repository.ErrOrderNotFound)

	order, err := svc.GetOrder(ctx, orderID)

	assert.Nil(t, order)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrOrderNotFound))
}

func TestOrderService_ListOpenOrders_Success(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	svc := NewOrderService(orderRepo, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	expectedOrders := []*model.Order{
		{OrderID: "O1", Status: model.OrderStatusOpen},
		{OrderID: "O2", Status: model.OrderStatusPartial},
	}

	orderRepo.On("ListOpenOrders", ctx, wallet, market).Return(expectedOrders, nil)

	orders, err := svc.ListOpenOrders(ctx, wallet, market)

	assert.NoError(t, err)
	assert.Len(t, orders, 2)
}

func TestOrderService_ExpireOrder_Success(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewOrderService(orderRepo, balanceRepo, nil, balanceCache, nil, nil, nil, nil, nil)

	ctx := context.Background()
	orderID := "O123456789"

	existingOrder := &model.Order{
		OrderID:      orderID,
		Wallet:       "0x1234567890123456789012345678901234567890",
		Status:       model.OrderStatusOpen,
		Amount:       decimal.NewFromFloat(1),
		FilledAmount: decimal.Zero,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2000),
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)
	// 新流程: 使用 Redis UnfreezeByOrderID 解冻
	balanceCache.On("UnfreezeByOrderID", ctx, existingOrder.Wallet, "USDT", orderID).Return(nil)
	balanceCache.On("DecrUserOpenOrders", ctx, existingOrder.Wallet).Return(nil) // Added expectation
	// DB 异步更新
	orderRepo.On("UpdateStatus", mock.Anything, orderID, model.OrderStatusOpen, model.OrderStatusExpired).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.ExpireOrder(ctx, orderID)

	assert.NoError(t, err)
	// 等待异步 goroutine 完成
	time.Sleep(100 * time.Millisecond)
}

func TestOrderService_RejectOrder_Success(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewOrderService(orderRepo, balanceRepo, nil, balanceCache, nil, nil, nil, nil, nil)

	ctx := context.Background()
	orderID := "O123456789"
	reason := "risk control rejection"

	existingOrder := &model.Order{
		OrderID:      orderID,
		Wallet:       "0x1234567890123456789012345678901234567890",
		Status:       model.OrderStatusPending,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2000),
	}

	orderRepo.On("GetByOrderID", ctx, orderID).Return(existingOrder, nil)
	// 新流程: 使用 Redis UnfreezeByOrderID 解冻
	balanceCache.On("UnfreezeByOrderID", ctx, existingOrder.Wallet, "USDT", orderID).Return(nil)
	balanceCache.On("DecrUserOpenOrders", ctx, existingOrder.Wallet).Return(nil) // Added expectation
	// DB 异步更新
	orderRepo.On("Update", mock.Anything, mock.AnythingOfType("*model.Order")).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.RejectOrder(ctx, orderID, reason)

	assert.NoError(t, err)
	// 等待异步 goroutine 完成
	time.Sleep(100 * time.Millisecond)
}

func TestOrderService_CalculateFreezeAmount_BuyOrder(t *testing.T) {
	svc := &orderService{}

	req := &CreateOrderRequest{
		Side:   model.OrderSideBuy,
		Price:  decimal.NewFromFloat(2000),
		Amount: decimal.NewFromFloat(1),
	}

	cfg := &MarketConfig{
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
	}

	token, amount := svc.calculateFreezeAmount(req, cfg)

	assert.Equal(t, "USDT", token)
	// 2000 * 1 * 1.001 = 2002
	assert.True(t, amount.Equal(decimal.NewFromFloat(2002)))
}

func TestOrderService_CalculateFreezeAmount_SellOrder(t *testing.T) {
	svc := &orderService{}

	req := &CreateOrderRequest{
		Side:   model.OrderSideSell,
		Price:  decimal.NewFromFloat(2000),
		Amount: decimal.NewFromFloat(1),
	}

	cfg := &MarketConfig{
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}

	token, amount := svc.calculateFreezeAmount(req, cfg)

	assert.Equal(t, "ETH", token)
	assert.True(t, amount.Equal(decimal.NewFromFloat(1)))
}

func TestOrderService_ValidateCreateOrderRequest(t *testing.T) {
	svc := &orderService{}

	testCases := []struct {
		name    string
		req     *CreateOrderRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &CreateOrderRequest{
				Wallet:    "0x1234567890123456789012345678901234567890",
				Market:    "ETH-USDT",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.NewFromFloat(2000),
				Amount:    decimal.NewFromFloat(1),
				Signature: []byte("test"),
			},
			wantErr: false,
		},
		{
			name: "invalid wallet - too short",
			req: &CreateOrderRequest{
				Wallet:    "0x123",
				Market:    "ETH-USDT",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.NewFromFloat(2000),
				Amount:    decimal.NewFromFloat(1),
				Signature: []byte("test"),
			},
			wantErr: true,
		},
		{
			name: "invalid wallet - no 0x prefix",
			req: &CreateOrderRequest{
				Wallet:    "1234567890123456789012345678901234567890ab",
				Market:    "ETH-USDT",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.NewFromFloat(2000),
				Amount:    decimal.NewFromFloat(1),
				Signature: []byte("test"),
			},
			wantErr: true,
		},
		{
			name: "empty market",
			req: &CreateOrderRequest{
				Wallet:    "0x1234567890123456789012345678901234567890",
				Market:    "",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.NewFromFloat(2000),
				Amount:    decimal.NewFromFloat(1),
				Signature: []byte("test"),
			},
			wantErr: true,
		},
		{
			name: "negative amount",
			req: &CreateOrderRequest{
				Wallet:    "0x1234567890123456789012345678901234567890",
				Market:    "ETH-USDT",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.NewFromFloat(2000),
				Amount:    decimal.NewFromFloat(-1),
				Signature: []byte("test"),
			},
			wantErr: true,
		},
		{
			name: "limit order without price",
			req: &CreateOrderRequest{
				Wallet:    "0x1234567890123456789012345678901234567890",
				Market:    "ETH-USDT",
				Side:      model.OrderSideBuy,
				Type:      model.OrderTypeLimit,
				Price:     decimal.Zero,
				Amount:    decimal.NewFromFloat(1),
				Signature: []byte("test"),
			},
			wantErr: true,
		},
		{
			name: "no signature",
			req: &CreateOrderRequest{
				Wallet: "0x1234567890123456789012345678901234567890",
				Market: "ETH-USDT",
				Side:   model.OrderSideBuy,
				Type:   model.OrderTypeLimit,
				Price:  decimal.NewFromFloat(2000),
				Amount: decimal.NewFromFloat(1),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateCreateOrderRequest(tc.req)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
