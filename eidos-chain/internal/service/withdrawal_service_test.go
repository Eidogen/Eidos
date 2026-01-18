package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
)

// mockWithdrawalRepository 模拟提现仓储
type mockWithdrawalRepository struct {
	mock.Mock
	mu          sync.RWMutex
	withdrawals map[string]*model.WithdrawalTx
}

func newMockWithdrawalRepository() *mockWithdrawalRepository {
	return &mockWithdrawalRepository{
		withdrawals: make(map[string]*model.WithdrawalTx),
	}
}

func (m *mockWithdrawalRepository) Create(ctx context.Context, withdrawal *model.WithdrawalTx) error {
	args := m.Called(ctx, withdrawal)
	if args.Error(0) == nil {
		m.mu.Lock()
		m.withdrawals[withdrawal.WithdrawID] = withdrawal
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *mockWithdrawalRepository) GetByWithdrawID(ctx context.Context, withdrawID string) (*model.WithdrawalTx, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.WithdrawalTx), args.Error(1)
}

func (m *mockWithdrawalRepository) GetByID(ctx context.Context, id int64) (*model.WithdrawalTx, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.WithdrawalTx), args.Error(1)
}

func (m *mockWithdrawalRepository) Update(ctx context.Context, withdrawal *model.WithdrawalTx) error {
	args := m.Called(ctx, withdrawal)
	return args.Error(0)
}

func (m *mockWithdrawalRepository) UpdateStatus(ctx context.Context, withdrawID string, status model.WithdrawalTxStatus, txHash string, blockNumber int64) error {
	args := m.Called(ctx, withdrawID, status, txHash, blockNumber)
	return args.Error(0)
}

func (m *mockWithdrawalRepository) UpdateFailed(ctx context.Context, withdrawID string, errMsg string) error {
	args := m.Called(ctx, withdrawID, errMsg)
	return args.Error(0)
}

func (m *mockWithdrawalRepository) ListPending(ctx context.Context, limit int) ([]*model.WithdrawalTx, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WithdrawalTx), args.Error(1)
}

func (m *mockWithdrawalRepository) ListByWallet(ctx context.Context, wallet string, page *repository.Pagination) ([]*model.WithdrawalTx, error) {
	args := m.Called(ctx, wallet, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WithdrawalTx), args.Error(1)
}

func (m *mockWithdrawalRepository) ListByStatus(ctx context.Context, status model.WithdrawalTxStatus, page *repository.Pagination) ([]*model.WithdrawalTx, error) {
	args := m.Called(ctx, status, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WithdrawalTx), args.Error(1)
}

func (m *mockWithdrawalRepository) ListByTimeRange(ctx context.Context, start, end int64, page *repository.Pagination) ([]*model.WithdrawalTx, error) {
	args := m.Called(ctx, start, end, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WithdrawalTx), args.Error(1)
}

// createTestWithdrawalService 创建测试用的提现服务
func createTestWithdrawalService(repo *mockWithdrawalRepository) *WithdrawalService {
	return &WithdrawalService{
		repo:         repo,
		maxRetries:   3,
		retryBackoff: 10 * time.Millisecond,
		chainID:      31337,
	}
}

func TestWithdrawalService_ProcessWithdrawalRequest_AlreadyExists(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()

	request := &model.WithdrawalRequest{
		WithdrawID:   "withdraw-123",
		Wallet:       "0xwallet",
		ToAddress:    "0xrecipient",
		Token:        "USDC",
		TokenAddress: "0xtoken",
		Amount:       decimal.NewFromFloat(1000),
	}

	// 如果提现已确认，应该直接返回 nil
	confirmedWithdrawal := &model.WithdrawalTx{
		WithdrawID: "withdraw-123",
		Status:     model.WithdrawalTxStatusConfirmed,
	}
	repo.On("GetByWithdrawID", mock.Anything, "withdraw-123").Return(confirmedWithdrawal, nil)

	err := svc.ProcessWithdrawalRequest(ctx, request)
	assert.NoError(t, err)
	// Create 不应该被调用，因为提现已完成
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestWithdrawalService_ProcessWithdrawalRequest_AlreadyInProgress(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()

	request := &model.WithdrawalRequest{
		WithdrawID:   "withdraw-456",
		Wallet:       "0xwallet",
		ToAddress:    "0xrecipient",
		Token:        "USDC",
		TokenAddress: "0xtoken",
		Amount:       decimal.NewFromFloat(500),
	}

	// 如果提现正在处理中，应该返回 ErrWithdrawalInProgress
	pendingWithdrawal := &model.WithdrawalTx{
		WithdrawID: "withdraw-456",
		Status:     model.WithdrawalTxStatusPending,
	}
	repo.On("GetByWithdrawID", mock.Anything, "withdraw-456").Return(pendingWithdrawal, nil)

	err := svc.ProcessWithdrawalRequest(ctx, request)
	assert.Error(t, err)
	assert.Equal(t, ErrWithdrawalInProgress, err)
}

func TestWithdrawalService_GetWithdrawalStatus(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()
	withdrawID := "withdraw-123"

	expectedWithdrawal := &model.WithdrawalTx{
		WithdrawID:    withdrawID,
		WalletAddress: "0xwallet",
		Token:         "USDC",
		Amount:        decimal.NewFromFloat(1000),
		Status:        model.WithdrawalTxStatusConfirmed,
	}

	repo.On("GetByWithdrawID", ctx, withdrawID).Return(expectedWithdrawal, nil)

	withdrawal, err := svc.GetWithdrawalStatus(ctx, withdrawID)
	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, withdrawID, withdrawal.WithdrawID)
	assert.Equal(t, model.WithdrawalTxStatusConfirmed, withdrawal.Status)
}

func TestWithdrawalService_ProcessPendingWithdrawals_NoPending(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()

	repo.On("ListPending", ctx, 10).Return([]*model.WithdrawalTx{}, nil)

	err := svc.ProcessPendingWithdrawals(ctx)
	assert.NoError(t, err)

	repo.AssertCalled(t, "ListPending", ctx, 10)
}

func TestWithdrawalService_RetryWithdrawal_AlreadyConfirmed(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()
	withdrawID := "withdraw-123"

	confirmedWithdrawal := &model.WithdrawalTx{
		WithdrawID: withdrawID,
		Status:     model.WithdrawalTxStatusConfirmed,
	}

	repo.On("GetByWithdrawID", ctx, withdrawID).Return(confirmedWithdrawal, nil)

	err := svc.RetryWithdrawal(ctx, withdrawID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already confirmed")
}

func TestWithdrawalService_SetOnWithdrawalConfirmed(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	svc.SetOnWithdrawalConfirmed(func(ctx context.Context, confirmation *model.WithdrawalConfirmation) error {
		return nil
	})

	assert.NotNil(t, svc.onWithdrawalConfirmed)
}

// ======== 并发测试 ========

func TestWithdrawalService_ConcurrentGetStatus(t *testing.T) {
	repo := newMockWithdrawalRepository()
	svc := createTestWithdrawalService(repo)

	ctx := context.Background()
	withdrawID := "withdraw-123"

	expectedWithdrawal := &model.WithdrawalTx{
		WithdrawID: withdrawID,
		Status:     model.WithdrawalTxStatusPending,
	}

	repo.On("GetByWithdrawID", mock.Anything, withdrawID).Return(expectedWithdrawal, nil)

	var wg sync.WaitGroup
	concurrency := 20
	iterations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				withdrawal, err := svc.GetWithdrawalStatus(ctx, withdrawID)
				assert.NoError(t, err)
				assert.NotNil(t, withdrawal)
			}
		}()
	}

	wg.Wait()
}

// TestWithdrawalServiceErrors 测试错误常量
func TestWithdrawalServiceErrors(t *testing.T) {
	assert.Equal(t, "withdrawal not found", ErrWithdrawalNotFound.Error())
	assert.Equal(t, "withdrawal already in progress", ErrWithdrawalInProgress.Error())
	assert.Equal(t, "insufficient balance for withdrawal", ErrInsufficientBalance.Error())
	assert.Equal(t, "invalid withdrawal signature", ErrInvalidSignature.Error())
}

// TestWithdrawalTx_Fields 测试 WithdrawalTx 字段
func TestWithdrawalTx_Fields(t *testing.T) {
	withdrawal := &model.WithdrawalTx{
		ID:            1,
		WithdrawID:    "withdraw-123",
		WalletAddress: "0xwallet",
		ToAddress:     "0xrecipient",
		Token:         "USDC",
		TokenAddress:  "0xtoken",
		Amount:        decimal.NewFromFloat(1000),
		ChainID:       31337,
		Status:        model.WithdrawalTxStatusPending,
	}

	assert.Equal(t, int64(1), withdrawal.ID)
	assert.Equal(t, "withdraw-123", withdrawal.WithdrawID)
	assert.Equal(t, "0xwallet", withdrawal.WalletAddress)
	assert.Equal(t, "0xrecipient", withdrawal.ToAddress)
	assert.Equal(t, "USDC", withdrawal.Token)
	assert.Equal(t, model.WithdrawalTxStatusPending, withdrawal.Status)
}

// TestWithdrawalTxStatus_Values 测试 WithdrawalTxStatus 枚举值
func TestWithdrawalTxStatus_Values(t *testing.T) {
	assert.Equal(t, model.WithdrawalTxStatus(0), model.WithdrawalTxStatusPending)
	assert.Equal(t, model.WithdrawalTxStatus(1), model.WithdrawalTxStatusSubmitted)
	assert.Equal(t, model.WithdrawalTxStatus(2), model.WithdrawalTxStatusConfirmed)
	assert.Equal(t, model.WithdrawalTxStatus(3), model.WithdrawalTxStatusFailed)
}
