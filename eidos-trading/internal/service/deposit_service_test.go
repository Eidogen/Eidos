package service

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// ========== Mock Deposit Repository ==========

type MockDepositRepository struct {
	mock.Mock
}

func (m *MockDepositRepository) Create(ctx context.Context, deposit *model.Deposit) error {
	args := m.Called(ctx, deposit)
	return args.Error(0)
}

func (m *MockDepositRepository) GetByID(ctx context.Context, id int64) (*model.Deposit, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) GetByDepositID(ctx context.Context, depositID string) (*model.Deposit, error) {
	args := m.Called(ctx, depositID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) GetByTxHashLogIndex(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error) {
	args := m.Called(ctx, txHash, logIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) ListByWallet(ctx context.Context, wallet string, filter *repository.DepositFilter, page *repository.Pagination) ([]*model.Deposit, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) ListPending(ctx context.Context, limit int) ([]*model.Deposit, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) ListConfirmed(ctx context.Context, limit int) ([]*model.Deposit, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Deposit), args.Error(1)
}

func (m *MockDepositRepository) UpdateStatus(ctx context.Context, depositID string, oldStatus, newStatus model.DepositStatus) error {
	args := m.Called(ctx, depositID, oldStatus, newStatus)
	return args.Error(0)
}

func (m *MockDepositRepository) MarkConfirmed(ctx context.Context, depositID string) error {
	args := m.Called(ctx, depositID)
	return args.Error(0)
}

func (m *MockDepositRepository) MarkCredited(ctx context.Context, depositID string) error {
	args := m.Called(ctx, depositID)
	return args.Error(0)
}

func (m *MockDepositRepository) CountByWallet(ctx context.Context, wallet string, filter *repository.DepositFilter) (int64, error) {
	args := m.Called(ctx, wallet, filter)
	return args.Get(0).(int64), args.Error(1)
}

// ========== Test Cases ==========

func TestDepositService_ProcessDepositEvent_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "0x1234567890123456789012345678901234567890123456789012345678901234",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		DetectedAt: 1700000000000,
	}

	tokenCfg.On("IsValidToken", "USDT").Return(true)
	depositRepo.On("GetByTxHashLogIndex", ctx, event.TxHash, event.LogIndex).Return(nil, repository.ErrDepositNotFound)
	idGen.On("Generate").Return(int64(123456789), nil)
	depositRepo.On("Create", ctx, mock.AnythingOfType("*model.Deposit")).Return(nil)

	err := svc.ProcessDepositEvent(ctx, event)

	assert.NoError(t, err)
}

func TestDepositService_ProcessDepositEvent_Idempotent(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "0x1234567890123456789012345678901234567890123456789012345678901234",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		DetectedAt: 1700000000000,
	}

	existingDeposit := &model.Deposit{
		DepositID: "D123",
		TxHash:    event.TxHash,
		LogIndex:  event.LogIndex,
	}

	tokenCfg.On("IsValidToken", "USDT").Return(true)
	depositRepo.On("GetByTxHashLogIndex", ctx, event.TxHash, event.LogIndex).Return(existingDeposit, nil)

	// Should return nil (idempotent)
	err := svc.ProcessDepositEvent(ctx, event)

	assert.NoError(t, err)
}

func TestDepositService_ProcessDepositEvent_InvalidTxHash(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "invalid",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		DetectedAt: 1700000000000,
	}

	err := svc.ProcessDepositEvent(ctx, event)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDeposit))
}

func TestDepositService_ProcessDepositEvent_InvalidWallet(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "0x1234567890123456789012345678901234567890123456789012345678901234",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "invalid",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(100),
		DetectedAt: 1700000000000,
	}

	err := svc.ProcessDepositEvent(ctx, event)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDeposit))
}

func TestDepositService_ProcessDepositEvent_InvalidToken(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "0x1234567890123456789012345678901234567890123456789012345678901234",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "INVALID",
		Amount:     decimal.NewFromFloat(100),
		DetectedAt: 1700000000000,
	}

	tokenCfg.On("IsValidToken", "INVALID").Return(false)

	err := svc.ProcessDepositEvent(ctx, event)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDeposit))
}

func TestDepositService_ProcessDepositEvent_NegativeAmount(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	event := &DepositEvent{
		TxHash:     "0x1234567890123456789012345678901234567890123456789012345678901234",
		LogIndex:   0,
		BlockNum:   12345,
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Amount:     decimal.NewFromFloat(-100), // Negative
		DetectedAt: 1700000000000,
	}

	tokenCfg.On("IsValidToken", "USDT").Return(true)

	err := svc.ProcessDepositEvent(ctx, event)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDeposit))
}

func TestDepositService_ConfirmDeposit_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	depositID := "D123456789"

	depositRepo.On("MarkConfirmed", ctx, depositID).Return(nil)

	err := svc.ConfirmDeposit(ctx, depositID)

	assert.NoError(t, err)
}

func TestDepositService_CreditDeposit_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	depositID := "D123456789"

	deposit := &model.Deposit{
		DepositID: depositID,
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		TxHash:    "0x1234567890123456789012345678901234567890123456789012345678901234",
		Status:    model.DepositStatusConfirmed,
	}

	depositRepo.On("GetByDepositID", ctx, depositID).Return(deposit, nil)
	// 现在使用 Redis Credit
	balanceCache.On("Credit", ctx, deposit.Wallet, deposit.Token, deposit.Amount, true).Return(nil)
	// 异步 DB 操作 - 需要 mock 以避免 goroutine 崩溃
	balanceRepo.On("Transaction", mock.Anything, mock.AnythingOfType("func(context.Context) error")).Return(nil).Maybe()
	balanceRepo.On("Credit", mock.Anything, deposit.Wallet, deposit.Token, deposit.Amount, true).Return(nil).Maybe()
	depositRepo.On("MarkCredited", mock.Anything, depositID).Return(nil).Maybe()
	balanceRepo.On("CreateBalanceLog", mock.Anything, mock.AnythingOfType("*model.BalanceLog")).Return(nil).Maybe()

	err := svc.CreditDeposit(ctx, depositID)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "Credit", ctx, deposit.Wallet, deposit.Token, deposit.Amount, true)
}

func TestDepositService_CreditDeposit_NotConfirmed(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	depositID := "D123456789"

	deposit := &model.Deposit{
		DepositID: depositID,
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		Status:    model.DepositStatusPending, // Not confirmed yet
	}

	depositRepo.On("GetByDepositID", ctx, depositID).Return(deposit, nil)

	err := svc.CreditDeposit(ctx, depositID)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrDepositNotConfirmed))
}

func TestDepositService_GetDeposit_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	depositID := "D123456789"

	expectedDeposit := &model.Deposit{
		DepositID: depositID,
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
	}

	depositRepo.On("GetByDepositID", ctx, depositID).Return(expectedDeposit, nil)

	deposit, err := svc.GetDeposit(ctx, depositID)

	assert.NoError(t, err)
	assert.NotNil(t, deposit)
	assert.Equal(t, depositID, deposit.DepositID)
}

func TestDepositService_GetDepositByTxHash_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	txHash := "0x1234567890123456789012345678901234567890123456789012345678901234"
	logIndex := uint32(0)

	expectedDeposit := &model.Deposit{
		DepositID: "D123",
		TxHash:    txHash,
		LogIndex:  logIndex,
	}

	depositRepo.On("GetByTxHashLogIndex", ctx, txHash, logIndex).Return(expectedDeposit, nil)

	deposit, err := svc.GetDepositByTxHash(ctx, txHash, logIndex)

	assert.NoError(t, err)
	assert.NotNil(t, deposit)
	assert.Equal(t, txHash, deposit.TxHash)
}

func TestDepositService_ListDeposits_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	expectedDeposits := []*model.Deposit{
		{DepositID: "D1", Wallet: wallet},
		{DepositID: "D2", Wallet: wallet},
	}

	depositRepo.On("ListByWallet", ctx, wallet, (*repository.DepositFilter)(nil), (*repository.Pagination)(nil)).Return(expectedDeposits, nil)

	deposits, err := svc.ListDeposits(ctx, wallet, nil, nil)

	assert.NoError(t, err)
	assert.Len(t, deposits, 2)
}

func TestDepositService_GetPendingDeposits_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	limit := 10

	expectedDeposits := []*model.Deposit{
		{DepositID: "D1", Status: model.DepositStatusPending},
		{DepositID: "D2", Status: model.DepositStatusPending},
	}

	depositRepo.On("ListPending", ctx, limit).Return(expectedDeposits, nil)

	deposits, err := svc.GetPendingDeposits(ctx, limit)

	assert.NoError(t, err)
	assert.Len(t, deposits, 2)
}

func TestDepositService_GetConfirmedDeposits_Success(t *testing.T) {
	depositRepo := new(MockDepositRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	tokenCfg := new(MockTokenConfigProvider)

	svc := NewDepositService(depositRepo, balanceRepo, balanceCache, idGen, tokenCfg)

	ctx := context.Background()
	limit := 10

	expectedDeposits := []*model.Deposit{
		{DepositID: "D1", Status: model.DepositStatusConfirmed},
		{DepositID: "D2", Status: model.DepositStatusConfirmed},
	}

	depositRepo.On("ListConfirmed", ctx, limit).Return(expectedDeposits, nil)

	deposits, err := svc.GetConfirmedDeposits(ctx, limit)

	assert.NoError(t, err)
	assert.Len(t, deposits, 2)
}
