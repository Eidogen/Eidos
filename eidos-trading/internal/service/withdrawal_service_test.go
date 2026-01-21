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

// ========== Mock Withdrawal Repository ==========

type MockWithdrawalRepository struct {
	mock.Mock
}

func (m *MockWithdrawalRepository) Create(ctx context.Context, withdrawal *model.Withdrawal) error {
	args := m.Called(ctx, withdrawal)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) GetByID(ctx context.Context, id int64) (*model.Withdrawal, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) GetByWithdrawID(ctx context.Context, withdrawID string) (*model.Withdrawal, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Withdrawal, error) {
	args := m.Called(ctx, wallet, nonce)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) ListByWallet(ctx context.Context, wallet string, filter *repository.WithdrawalFilter, page *repository.Pagination) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) ListPending(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) ListProcessing(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) ListSubmitted(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Withdrawal), args.Error(1)
}

func (m *MockWithdrawalRepository) UpdateStatus(ctx context.Context, withdrawID string, oldStatus, newStatus model.WithdrawStatus) error {
	args := m.Called(ctx, withdrawID, oldStatus, newStatus)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkProcessing(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkSubmitted(ctx context.Context, withdrawID string, txHash string) error {
	args := m.Called(ctx, withdrawID, txHash)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkConfirmed(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkFailed(ctx context.Context, withdrawID string, reason string) error {
	args := m.Called(ctx, withdrawID, reason)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkRejected(ctx context.Context, withdrawID string, reason string) error {
	args := m.Called(ctx, withdrawID, reason)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkCancelled(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) MarkRefunded(ctx context.Context, withdrawID string) error {
	args := m.Called(ctx, withdrawID)
	return args.Error(0)
}

func (m *MockWithdrawalRepository) CountByWallet(ctx context.Context, wallet string, filter *repository.WithdrawalFilter) (int64, error) {
	args := m.Called(ctx, wallet, filter)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockWithdrawalRepository) CountPendingByWallet(ctx context.Context, wallet string) (int64, error) {
	args := m.Called(ctx, wallet)
	return args.Get(0).(int64), args.Error(1)
}

// ========== Test Cases ==========

func TestWithdrawalService_CreateWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test-signature"),
	}

	// Redis 余额
	redisBalance := &cache.RedisBalance{
		Wallet:           req.Wallet,
		Token:            req.Token,
		SettledAvailable: decimal.NewFromFloat(1000),
	}

	tokenCfg.On("IsValidToken", req.Token).Return(true)
	withdrawRepo.On("GetByWalletNonce", ctx, req.Wallet, req.Nonce).Return(nil, repository.ErrWithdrawalNotFound)
	// 现在使用 Redis 检查余额
	balanceCache.On("GetBalance", ctx, req.Wallet, req.Token).Return(redisBalance, nil)
	idGen.On("Generate").Return(int64(123456789), nil)
	// 使用 Redis 冻结
	balanceCache.On("Freeze", ctx, req.Wallet, req.Token, req.Amount, true, "W123456789").Return(nil)
	// 异步 DB 操作
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil).Maybe()
	balanceRepo.On("Freeze", mock.Anything, req.Wallet, req.Token, req.Amount, true).Return(nil).Maybe()
	nonceRepo.On("MarkUsedWithTx", mock.Anything, req.Wallet, model.NonceUsageWithdraw, req.Nonce, "W123456789").Return(nil).Maybe()
	withdrawRepo.On("Create", mock.Anything, mock.AnythingOfType("*model.Withdrawal")).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, "W123456789", withdrawal.WithdrawID)
	assert.Equal(t, req.Wallet, withdrawal.Wallet)
	assert.Equal(t, model.WithdrawStatusPending, withdrawal.Status)
}

func TestWithdrawalService_CreateWithdrawal_InvalidWallet(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "invalid",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test"),
	}

	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	assert.Nil(t, withdrawal)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWithdrawal))
}

func TestWithdrawalService_CreateWithdrawal_InvalidToken(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "INVALID",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test"),
	}

	tokenCfg.On("IsValidToken", "INVALID").Return(false)

	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	assert.Nil(t, withdrawal)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWithdrawal))
}

func TestWithdrawalService_CreateWithdrawal_DuplicateNonce(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test"),
	}

	// 现在 nonce 重复检查通过 GetByWalletNonce 实现幂等
	// 如果 nonce 已存在，返回已存在的提现记录（幂等）
	existingWithdrawal := &model.Withdrawal{
		WithdrawID: "W999",
		Wallet:     req.Wallet,
		Nonce:      req.Nonce,
		Status:     model.WithdrawStatusPending,
	}

	tokenCfg.On("IsValidToken", req.Token).Return(true)
	withdrawRepo.On("GetByWalletNonce", ctx, req.Wallet, req.Nonce).Return(existingWithdrawal, nil)

	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	// 幂等：返回已存在的提现
	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, "W999", withdrawal.WithdrawID)
}

func TestWithdrawalService_CreateWithdrawal_InsufficientBalance(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(1000), // More than available
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test"),
	}

	// 现在使用 Redis 检查余额
	redisBalance := &cache.RedisBalance{
		Wallet:           req.Wallet,
		Token:            req.Token,
		SettledAvailable: decimal.NewFromFloat(100), // Only 100 available
	}

	tokenCfg.On("IsValidToken", req.Token).Return(true)
	withdrawRepo.On("GetByWalletNonce", ctx, req.Wallet, req.Nonce).Return(nil, repository.ErrWithdrawalNotFound)
	balanceCache.On("GetBalance", ctx, req.Wallet, req.Token).Return(redisBalance, nil)

	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	assert.Nil(t, withdrawal)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInsufficientBalance))
}

func TestWithdrawalService_CreateWithdrawal_Idempotent(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	nonceRepo := new(MockNonceRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nonceRepo, idGen, tokenCfg, nil)

	ctx := context.Background()
	req := &CreateWithdrawalRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0x9999999999999999999999999999999999999999",
		Nonce:     1,
		Signature: []byte("test"),
	}

	existingWithdrawal := &model.Withdrawal{
		WithdrawID: "W123",
		Wallet:     req.Wallet,
		Nonce:      req.Nonce,
		Status:     model.WithdrawStatusPending,
	}

	tokenCfg.On("IsValidToken", req.Token).Return(true)
	withdrawRepo.On("GetByWalletNonce", ctx, req.Wallet, req.Nonce).Return(existingWithdrawal, nil)

	// Should return existing withdrawal (idempotent)
	withdrawal, err := svc.CreateWithdrawal(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, "W123", withdrawal.WithdrawID)
}

func TestWithdrawalService_GetWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	svc := NewWithdrawalService(withdrawRepo, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"

	expectedWithdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(expectedWithdrawal, nil)

	withdrawal, err := svc.GetWithdrawal(ctx, withdrawID)

	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, withdrawID, withdrawal.WithdrawID)
}

func TestWithdrawalService_ApproveWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	svc := NewWithdrawalService(withdrawRepo, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		Status:     model.WithdrawStatusPending,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)
	withdrawRepo.On("MarkProcessing", ctx, withdrawID).Return(nil)

	err := svc.ApproveWithdrawal(ctx, withdrawID)

	assert.NoError(t, err)
}

func TestWithdrawalService_RejectWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"
	reason := "Risk control rejection"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		Status:     model.WithdrawStatusPending,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)
	// 现在使用 Redis Unfreeze
	balanceCache.On("Unfreeze", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil)
	// 同步 DB 操作
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil)
	balanceRepo.On("Unfreeze", mock.Anything, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil).Maybe()
	withdrawRepo.On("MarkRejected", mock.Anything, withdrawID, reason).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.RejectWithdrawal(ctx, withdrawID, reason)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Unfreeze", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true)
}

func TestWithdrawalService_RejectWithdrawal_NotPending(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"
	reason := "Risk control rejection"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Status:     model.WithdrawStatusConfirmed, // Already confirmed
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)

	err := svc.RejectWithdrawal(ctx, withdrawID, reason)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWithdrawalNotPending))
}

func TestWithdrawalService_SubmitWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	svc := NewWithdrawalService(withdrawRepo, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"
	txHash := "0x1234567890123456789012345678901234567890123456789012345678901234"

	withdrawRepo.On("MarkSubmitted", ctx, withdrawID, txHash).Return(nil)

	err := svc.SubmitWithdrawal(ctx, withdrawID, txHash)

	assert.NoError(t, err)
}

func TestWithdrawalService_ConfirmWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		Status:     model.WithdrawStatusSubmitted,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)
	// 现在使用 Redis Debit
	balanceCache.On("Debit", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil)
	// 同步 DB 操作
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil)
	balanceRepo.On("Debit", mock.Anything, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil).Maybe()
	withdrawRepo.On("MarkConfirmed", mock.Anything, withdrawID).Return(nil).Maybe()

	err := svc.ConfirmWithdrawal(ctx, withdrawID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Debit", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true)
}

func TestWithdrawalService_FailWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	withdrawID := "W123456789"
	reason := "Transaction failed on chain"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		Status:     model.WithdrawStatusSubmitted,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)
	// 现在使用 Redis Unfreeze
	balanceCache.On("Unfreeze", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil)
	// 同步 DB 操作
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil)
	balanceRepo.On("Unfreeze", mock.Anything, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil).Maybe()
	withdrawRepo.On("MarkFailed", mock.Anything, withdrawID, reason).Return(nil).Maybe()
	withdrawRepo.On("MarkRefunded", mock.Anything, withdrawID).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.FailWithdrawal(ctx, withdrawID, reason)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Unfreeze", ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true)
}

func TestWithdrawalService_CancelWithdrawal_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	withdrawID := "W123456789"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     wallet,
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		Status:     model.WithdrawStatusPending,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)
	// 现在使用 Redis Unfreeze
	balanceCache.On("Unfreeze", ctx, wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil)
	// 异步 DB 操作
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil).Maybe()
	balanceRepo.On("Unfreeze", mock.Anything, wallet, withdrawal.Token, withdrawal.Amount, true).Return(nil).Maybe()
	withdrawRepo.On("MarkCancelled", mock.Anything, withdrawID).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.CancelWithdrawal(ctx, wallet, withdrawID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Unfreeze", ctx, wallet, withdrawal.Token, withdrawal.Amount, true)
}

func TestWithdrawalService_CancelWithdrawal_WrongWallet(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	withdrawID := "W123456789"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     "0x9999999999999999999999999999999999999999", // Different wallet
		Status:     model.WithdrawStatusPending,
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)

	err := svc.CancelWithdrawal(ctx, wallet, withdrawID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWithdrawalNotFound))
}

func TestWithdrawalService_CancelWithdrawal_NotPending(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)

	svc := NewWithdrawalService(withdrawRepo, balanceRepo, balanceCache, nil, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	withdrawID := "W123456789"

	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     wallet,
		Status:     model.WithdrawStatusProcessing, // Not pending
	}

	withdrawRepo.On("GetByWithdrawID", ctx, withdrawID).Return(withdrawal, nil)

	err := svc.CancelWithdrawal(ctx, wallet, withdrawID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWithdrawalNotPending))
}

func TestWithdrawalService_GetPendingWithdrawals_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	svc := NewWithdrawalService(withdrawRepo, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	limit := 10

	expectedWithdrawals := []*model.Withdrawal{
		{WithdrawID: "W1", Status: model.WithdrawStatusPending},
		{WithdrawID: "W2", Status: model.WithdrawStatusPending},
	}

	withdrawRepo.On("ListPending", ctx, limit).Return(expectedWithdrawals, nil)

	withdrawals, err := svc.GetPendingWithdrawals(ctx, limit)

	assert.NoError(t, err)
	assert.Len(t, withdrawals, 2)
}

func TestWithdrawalService_GetSubmittedWithdrawals_Success(t *testing.T) {
	withdrawRepo := new(MockWithdrawalRepository)
	svc := NewWithdrawalService(withdrawRepo, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	limit := 10

	expectedWithdrawals := []*model.Withdrawal{
		{WithdrawID: "W1", Status: model.WithdrawStatusSubmitted},
		{WithdrawID: "W2", Status: model.WithdrawStatusSubmitted},
	}

	withdrawRepo.On("ListSubmitted", ctx, limit).Return(expectedWithdrawals, nil)

	withdrawals, err := svc.GetSubmittedWithdrawals(ctx, limit)

	assert.NoError(t, err)
	assert.Len(t, withdrawals, 2)
}
