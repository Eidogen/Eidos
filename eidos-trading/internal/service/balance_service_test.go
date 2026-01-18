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

// ========== Mock Token Config Provider ==========
// 注意: MockBalanceRedisRepository 定义在 order_service_test.go 中

type MockTokenConfigProvider struct {
	mock.Mock
}

func (m *MockTokenConfigProvider) IsValidToken(token string) bool {
	args := m.Called(token)
	return args.Bool(0)
}

func (m *MockTokenConfigProvider) GetTokenDecimals(token string) int32 {
	args := m.Called(token)
	return args.Get(0).(int32)
}

func (m *MockTokenConfigProvider) GetSupportedTokens() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

// ========== Extended MockBalanceRepository ==========

type ExtendedMockBalanceRepository struct {
	MockBalanceRepository
}

func (m *ExtendedMockBalanceRepository) GetOrCreate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *ExtendedMockBalanceRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, wallet, token, amount)
	return args.Error(0)
}

func (m *ExtendedMockBalanceRepository) CreditFeeAccount(ctx context.Context, bucketID int, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, bucketID, token, amount)
	return args.Error(0)
}

func (m *ExtendedMockBalanceRepository) GetFeeAccount(ctx context.Context, bucketID int, token string) (*model.FeeAccount, error) {
	args := m.Called(ctx, bucketID, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.FeeAccount), args.Error(1)
}

func (m *ExtendedMockBalanceRepository) GetByWalletTokenForUpdate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Balance), args.Error(1)
}

func (m *ExtendedMockBalanceRepository) Update(ctx context.Context, balance *model.Balance) error {
	args := m.Called(ctx, balance)
	return args.Error(0)
}

func (m *ExtendedMockBalanceRepository) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal) error {
	args := m.Called(ctx, fromWallet, toWallet, token, amount)
	return args.Error(0)
}

func (m *ExtendedMockBalanceRepository) UpsertBalance(ctx context.Context, balance *model.Balance) error {
	args := m.Called(ctx, balance)
	return args.Error(0)
}

func (m *ExtendedMockBalanceRepository) ListBalances(ctx context.Context, offset, limit int) ([]*model.Balance, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Balance), args.Error(1)
}

// ========== Test Cases ==========

func TestBalanceService_GetBalance_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 现在 GetBalance 从 Redis 读取
	expectedRedisBalance := &cache.RedisBalance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: decimal.NewFromFloat(1000),
		SettledFrozen:    decimal.Zero,
		PendingAvailable: decimal.NewFromFloat(500),
		PendingFrozen:    decimal.Zero,
	}

	tokenCfg.On("IsValidToken", token).Return(true)
	balanceCache.On("GetOrCreateBalance", ctx, wallet, token).Return(expectedRedisBalance, nil)

	balance, err := svc.GetBalance(ctx, wallet, token)

	assert.NoError(t, err)
	assert.NotNil(t, balance)
	assert.Equal(t, wallet, balance.Wallet)
	assert.Equal(t, token, balance.Token)
	assert.True(t, expectedRedisBalance.SettledAvailable.Equal(balance.SettledAvailable))
}

func TestBalanceService_GetBalance_InvalidToken(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "INVALID"

	tokenCfg.On("IsValidToken", token).Return(false)

	balance, err := svc.GetBalance(ctx, wallet, token)

	assert.Nil(t, balance)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestBalanceService_GetBalances_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 现在 GetBalances 使用 GetBalancesBatch 批量获取
	tokens := []string{"USDT", "ETH"}
	tokenCfg.On("GetSupportedTokens").Return(tokens)

	usdtBalance := &cache.RedisBalance{
		Wallet:           wallet,
		Token:            "USDT",
		SettledAvailable: decimal.NewFromFloat(1000),
	}
	ethBalance := &cache.RedisBalance{
		Wallet:           wallet,
		Token:            "ETH",
		SettledAvailable: decimal.NewFromFloat(10),
	}

	balanceCache.On("GetBalancesBatch", ctx, wallet, tokens).Return([]*cache.RedisBalance{usdtBalance, ethBalance}, nil)

	balances, err := svc.GetBalances(ctx, wallet)

	assert.NoError(t, err)
	assert.Len(t, balances, 2)
}

func TestBalanceService_Credit_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	tokenCfg.On("IsValidToken", token).Return(true)
	// 现在先操作 Redis
	balanceCache.On("Credit", ctx, wallet, token, amount, true).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Credit", mock.Anything, wallet, token, amount, true).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.Credit(ctx, wallet, token, amount, true, "Test credit")

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Credit", ctx, wallet, token, amount, true)
}

func TestBalanceService_Credit_InvalidAmount(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(-100) // Negative amount

	err := svc.Credit(ctx, wallet, token, amount, true, "Test credit")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidAmount))
}

func TestBalanceService_Credit_InvalidToken(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "INVALID"
	amount := decimal.NewFromFloat(100)

	tokenCfg.On("IsValidToken", token).Return(false)

	err := svc.Credit(ctx, wallet, token, amount, true, "Test credit")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidToken))
}

func TestBalanceService_Freeze_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	orderID := "O123"

	// 现在先操作 Redis
	balanceCache.On("Freeze", ctx, wallet, token, amount, false, orderID).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Freeze", mock.Anything, wallet, token, amount, false).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.Freeze(ctx, wallet, token, amount, orderID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Freeze", ctx, wallet, token, amount, false, orderID)
}

func TestBalanceService_Freeze_InsufficientBalance(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	orderID := "O123"

	balanceCache.On("Freeze", ctx, wallet, token, amount, false, orderID).Return(cache.ErrRedisInsufficientBalance)

	err := svc.Freeze(ctx, wallet, token, amount, orderID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrInsufficientBalance))
}

func TestBalanceService_Unfreeze_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	orderID := "O123"

	// 现在先操作 Redis
	balanceCache.On("Unfreeze", ctx, wallet, token, amount, false).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Unfreeze", mock.Anything, wallet, token, amount, false).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.Unfreeze(ctx, wallet, token, amount, orderID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Unfreeze", ctx, wallet, token, amount, false)
}

func TestBalanceService_Transfer_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	fromWallet := "0x1111111111111111111111111111111111111111"
	toWallet := "0x2222222222222222222222222222222222222222"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	tradeID := "T123"

	// 现在使用原子 Transfer 操作 (单个 Lua 脚本)
	balanceCache.On("Transfer", ctx, fromWallet, toWallet, token, amount, false).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Debit", mock.Anything, fromWallet, token, amount, false).Return(nil).Maybe()
	balanceRepo.On("Credit", mock.Anything, toWallet, token, amount, false).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.Transfer(ctx, fromWallet, toWallet, token, amount, tradeID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Transfer", ctx, fromWallet, toWallet, token, amount, false)
}

func TestBalanceService_Settle_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	batchID := "B123"

	// 现在先操作 Redis
	balanceCache.On("Settle", ctx, wallet, token, amount).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Settle", mock.Anything, wallet, token, amount).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.Settle(ctx, wallet, token, amount, batchID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Settle", ctx, wallet, token, amount)
}

func TestBalanceService_DeductFee_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(1)
	tradeID := "T123"

	// 现在先操作 Redis
	balanceCache.On("Debit", ctx, wallet, token, amount, false).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Debit", mock.Anything, wallet, token, amount, false).Return(nil).Maybe()
	balanceRepo.On("CreditFeeAccount", mock.Anything, mock.AnythingOfType("int"), token, amount).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.DeductFee(ctx, wallet, token, amount, tradeID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Debit", ctx, wallet, token, amount, false)
}

func TestBalanceService_DeductFee_ZeroAmount(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.Zero // Zero fee
	tradeID := "T123"

	// Should return nil without doing anything
	err := svc.DeductFee(ctx, wallet, token, amount, tradeID)

	assert.NoError(t, err)
}

func TestBalanceService_RefundWithdraw_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	withdrawID := "W123"

	// 现在先操作 Redis
	balanceCache.On("Credit", ctx, wallet, token, amount, true).Return(nil)
	// 异步写 DB - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Credit", mock.Anything, wallet, token, amount, true).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.RefundWithdraw(ctx, wallet, token, amount, withdrawID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Credit", ctx, wallet, token, amount, true)
}

func TestBalanceService_GetAvailableBalance_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	redisBalance := &cache.RedisBalance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: decimal.NewFromFloat(1000),
		PendingAvailable: decimal.NewFromFloat(500),
	}

	tokenCfg.On("IsValidToken", token).Return(true)
	balanceCache.On("GetOrCreateBalance", ctx, wallet, token).Return(redisBalance, nil)

	available, err := svc.GetAvailableBalance(ctx, wallet, token)

	assert.NoError(t, err)
	// TotalAvailable = SettledAvailable + PendingAvailable = 1500
	assert.True(t, available.Equal(decimal.NewFromFloat(1500)))
}

func TestBalanceService_GetWithdrawableBalance_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	redisBalance := &cache.RedisBalance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: decimal.NewFromFloat(1000),
		PendingAvailable: decimal.NewFromFloat(500), // Not withdrawable
	}

	tokenCfg.On("IsValidToken", token).Return(true)
	balanceCache.On("GetOrCreateBalance", ctx, wallet, token).Return(redisBalance, nil)

	withdrawable, err := svc.GetWithdrawableBalance(ctx, wallet, token)

	assert.NoError(t, err)
	// Only SettledAvailable is withdrawable
	assert.True(t, withdrawable.Equal(decimal.NewFromFloat(1000)))
}

func TestBalanceService_GetTotalFeeBalance_Success(t *testing.T) {
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewBalanceService(balanceRepo, nil, balanceCache, tokenCfg, nil)

	ctx := context.Background()
	token := "USDT"

	// Mock 16 buckets
	for i := 0; i < 16; i++ {
		feeAccount := &model.FeeAccount{
			BucketID: i,
			Token:    token,
			Balance:  decimal.NewFromFloat(10), // 10 per bucket
		}
		balanceRepo.On("GetFeeAccount", ctx, i, token).Return(feeAccount, nil)
	}

	total, err := svc.GetTotalFeeBalance(ctx, token)

	assert.NoError(t, err)
	// 16 buckets * 10 = 160
	assert.True(t, total.Equal(decimal.NewFromFloat(160)))
}

func TestHashToBucket(t *testing.T) {
	testCases := []struct {
		wallet   string
		buckets  int
		expected int
	}{
		{
			wallet:   "0x1234567890123456789012345678901234567890",
			buckets:  16,
			expected: int('0') % 16, // Last char is '0'
		},
		{
			wallet:   "0x123456789012345678901234567890123456789a",
			buckets:  16,
			expected: int('a') % 16, // Last char is 'a'
		},
		{
			wallet:   "",
			buckets:  16,
			expected: 0, // Edge case: empty string
		},
		{
			wallet:   "a",
			buckets:  16,
			expected: 0, // Edge case: too short
		},
	}

	for _, tc := range testCases {
		result := hashToBucket(tc.wallet, tc.buckets)
		assert.Equal(t, tc.expected, result)
	}
}
