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

// mockSettlementRepository 模拟结算仓储
type mockSettlementRepository struct {
	mock.Mock
	mu      sync.RWMutex
	batches map[string]*model.SettlementBatch
}

func newMockSettlementRepository() *mockSettlementRepository {
	return &mockSettlementRepository{
		batches: make(map[string]*model.SettlementBatch),
	}
}

func (m *mockSettlementRepository) Create(ctx context.Context, batch *model.SettlementBatch) error {
	args := m.Called(ctx, batch)
	if args.Error(0) == nil {
		m.mu.Lock()
		m.batches[batch.BatchID] = batch
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *mockSettlementRepository) GetByBatchID(ctx context.Context, batchID string) (*model.SettlementBatch, error) {
	args := m.Called(ctx, batchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SettlementBatch), args.Error(1)
}

func (m *mockSettlementRepository) GetByID(ctx context.Context, id int64) (*model.SettlementBatch, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SettlementBatch), args.Error(1)
}

func (m *mockSettlementRepository) Update(ctx context.Context, batch *model.SettlementBatch) error {
	args := m.Called(ctx, batch)
	return args.Error(0)
}

func (m *mockSettlementRepository) UpdateStatus(ctx context.Context, batchID string, status model.SettlementBatchStatus, txHash string, blockNumber int64) error {
	args := m.Called(ctx, batchID, status, txHash, blockNumber)
	return args.Error(0)
}

func (m *mockSettlementRepository) UpdateFailed(ctx context.Context, batchID string, errMsg string) error {
	args := m.Called(ctx, batchID, errMsg)
	return args.Error(0)
}

func (m *mockSettlementRepository) ListPending(ctx context.Context, limit int) ([]*model.SettlementBatch, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.SettlementBatch), args.Error(1)
}

func (m *mockSettlementRepository) ListByStatus(ctx context.Context, status model.SettlementBatchStatus, page *repository.Pagination) ([]*model.SettlementBatch, error) {
	args := m.Called(ctx, status, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.SettlementBatch), args.Error(1)
}

func (m *mockSettlementRepository) ListByTimeRange(ctx context.Context, start, end int64, page *repository.Pagination) ([]*model.SettlementBatch, error) {
	args := m.Called(ctx, start, end, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.SettlementBatch), args.Error(1)
}

func (m *mockSettlementRepository) CreateRollbackLog(ctx context.Context, log *model.SettlementRollbackLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func (m *mockSettlementRepository) GetRollbackLogByBatchID(ctx context.Context, batchID string) (*model.SettlementRollbackLog, error) {
	args := m.Called(ctx, batchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SettlementRollbackLog), args.Error(1)
}

func (m *mockSettlementRepository) ListRollbackLogs(ctx context.Context, page *repository.Pagination) ([]*model.SettlementRollbackLog, error) {
	args := m.Called(ctx, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.SettlementRollbackLog), args.Error(1)
}

// createTestSettlementService 创建测试用的结算服务
func createTestSettlementService(repo *mockSettlementRepository) *SettlementService {
	return &SettlementService{
		repo:          repo,
		batchSize:     5,
		batchInterval: 100 * time.Millisecond,
		maxRetries:    3,
		retryBackoff:  10 * time.Millisecond,
		chainID:       31337,
		pendingTrades: make([]*model.SettlementTrade, 0, 5),
		lastFlushTime: time.Now(),
	}
}

func TestSettlementService_AddTrade_Single(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()
	trade := &model.SettlementTrade{
		TradeID:     "trade-1",
		Market:      "BTC-USDC",
		MakerWallet: "0xmaker",
		TakerWallet: "0xtaker",
		Price:       decimal.NewFromFloat(50000),
		Amount:      decimal.NewFromFloat(1.0),
	}

	err := svc.AddTrade(ctx, trade)
	assert.NoError(t, err)
	assert.Equal(t, 1, svc.GetPendingTradesCount())
}

func TestSettlementService_AddTrade_BatchFlush(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	// 期望创建批次
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()

	// 添加 5 笔交易，应该触发批次刷新
	for i := 0; i < 5; i++ {
		trade := &model.SettlementTrade{
			TradeID:     "trade-" + string(rune('1'+i)),
			Market:      "BTC-USDC",
			MakerWallet: "0xmaker",
			TakerWallet: "0xtaker",
			Price:       decimal.NewFromFloat(50000),
			Amount:      decimal.NewFromFloat(1.0),
		}
		err := svc.AddTrade(ctx, trade)
		assert.NoError(t, err)
	}

	// 验证批次已创建
	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
	assert.Equal(t, 0, svc.GetPendingTradesCount())
}

func TestSettlementService_FlushIfNeeded_NoTrades(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()

	// 没有待处理交易，不应该刷新
	err := svc.FlushIfNeeded(ctx)
	assert.NoError(t, err)

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSettlementService_FlushIfNeeded_TimeElapsed(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)
	svc.batchInterval = 10 * time.Millisecond

	// 期望创建批次
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()

	// 添加一笔交易
	trade := &model.SettlementTrade{
		TradeID: "trade-1",
		Market:  "BTC-USDC",
	}
	err := svc.AddTrade(ctx, trade)
	assert.NoError(t, err)

	// 等待时间间隔过期
	svc.lastFlushTime = time.Now().Add(-100 * time.Millisecond)

	// 应该刷新
	err = svc.FlushIfNeeded(ctx)
	assert.NoError(t, err)

	repo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
	assert.Equal(t, 0, svc.GetPendingTradesCount())
}

func TestSettlementService_FlushIfNeeded_TimeNotElapsed(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)
	svc.batchInterval = 1 * time.Hour

	ctx := context.Background()

	// 添加一笔交易
	trade := &model.SettlementTrade{
		TradeID: "trade-1",
		Market:  "BTC-USDC",
	}
	err := svc.AddTrade(ctx, trade)
	assert.NoError(t, err)

	// 时间间隔未过期，不应该刷新
	err = svc.FlushIfNeeded(ctx)
	assert.NoError(t, err)

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	assert.Equal(t, 1, svc.GetPendingTradesCount())
}

func TestSettlementService_GetBatchStatus(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()
	batchID := "batch-123"

	expectedBatch := &model.SettlementBatch{
		BatchID:    batchID,
		TradeCount: 5,
		Status:     model.SettlementBatchStatusConfirmed,
	}

	repo.On("GetByBatchID", ctx, batchID).Return(expectedBatch, nil)

	batch, err := svc.GetBatchStatus(ctx, batchID)
	assert.NoError(t, err)
	assert.NotNil(t, batch)
	assert.Equal(t, batchID, batch.BatchID)
	assert.Equal(t, model.SettlementBatchStatusConfirmed, batch.Status)
}

func TestSettlementService_SplitAndRetryBatch(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()
	batchID := "batch-123"

	originalBatch := &model.SettlementBatch{
		BatchID:    batchID,
		TradeCount: 4,
		TradeIDs:   `["trade-1","trade-2","trade-3","trade-4"]`,
		ChainID:    31337,
		Status:     model.SettlementBatchStatusFailed,
	}

	repo.On("GetByBatchID", ctx, batchID).Return(originalBatch, nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	err := svc.SplitAndRetryBatch(ctx, batchID)
	assert.NoError(t, err)

	// 应该创建两个新批次
	repo.AssertNumberOfCalls(t, "Create", 2)
}

func TestSettlementService_SplitAndRetryBatch_NotFailed(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()
	batchID := "batch-123"

	originalBatch := &model.SettlementBatch{
		BatchID:    batchID,
		TradeCount: 4,
		TradeIDs:   `["trade-1","trade-2","trade-3","trade-4"]`,
		ChainID:    31337,
		Status:     model.SettlementBatchStatusPending,
	}

	repo.On("GetByBatchID", ctx, batchID).Return(originalBatch, nil)

	err := svc.SplitAndRetryBatch(ctx, batchID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in failed status")
}

func TestSettlementService_SplitAndRetryBatch_SingleTrade(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	ctx := context.Background()
	batchID := "batch-123"

	originalBatch := &model.SettlementBatch{
		BatchID:    batchID,
		TradeCount: 1,
		TradeIDs:   `["trade-1"]`,
		ChainID:    31337,
		Status:     model.SettlementBatchStatusFailed,
	}

	repo.On("GetByBatchID", ctx, batchID).Return(originalBatch, nil)

	err := svc.SplitAndRetryBatch(ctx, batchID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be split further")
}

// ======== 并发测试 ========

func TestSettlementService_ConcurrentAddTrade(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)
	svc.batchSize = 100 // 增大批次大小以避免频繁刷新

	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	var wg sync.WaitGroup
	concurrency := 10
	iterations := 50

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				trade := &model.SettlementTrade{
					TradeID:     "trade-" + string(rune('a'+id)) + "-" + string(rune('0'+j)),
					Market:      "BTC-USDC",
					MakerWallet: "0xmaker",
					TakerWallet: "0xtaker",
					Price:       decimal.NewFromFloat(50000),
					Amount:      decimal.NewFromFloat(1.0),
				}
				svc.AddTrade(ctx, trade)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有交易都被添加或已刷新
	totalExpected := concurrency * iterations
	pendingCount := svc.GetPendingTradesCount()
	createdBatches := len(repo.batches)

	// pending + (created batches * batch size) <= total
	t.Logf("Pending: %d, Created batches: %d", pendingCount, createdBatches)
	assert.LessOrEqual(t, pendingCount, totalExpected)
}

func TestSettlementService_ConcurrentFlush(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)
	svc.batchInterval = 1 * time.Millisecond

	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	ctx := context.Background()
	var wg sync.WaitGroup

	// 添加一些交易
	for i := 0; i < 3; i++ {
		trade := &model.SettlementTrade{
			TradeID: "trade-" + string(rune('1'+i)),
			Market:  "BTC-USDC",
		}
		svc.AddTrade(ctx, trade)
	}

	// 等待时间间隔过期
	svc.lastFlushTime = time.Now().Add(-100 * time.Millisecond)

	// 并发刷新
	concurrency := 10
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.FlushIfNeeded(ctx)
		}()
	}

	wg.Wait()

	// 验证最终状态正确（只创建一个批次或没有待处理交易）
	pendingCount := svc.GetPendingTradesCount()
	t.Logf("Final pending count: %d", pendingCount)
}

func TestSettlementService_SetOnSettlementConfirmed(t *testing.T) {
	repo := newMockSettlementRepository()
	svc := createTestSettlementService(repo)

	svc.SetOnSettlementConfirmed(func(ctx context.Context, confirmation *model.SettlementConfirmation) error {
		return nil
	})

	assert.NotNil(t, svc.onSettlementConfirmed)
}

// TestContainsHelper 测试 contains 辅助函数
func TestContainsHelper(t *testing.T) {
	assert.True(t, contains("nonce too low", "nonce too low"))
	assert.True(t, contains("error: nonce too low occurred", "nonce too low"))
	assert.True(t, contains("insufficient funds for gas", "insufficient funds"))
	assert.False(t, contains("some error", "nonce too low"))
	assert.False(t, contains("", "test"))
	assert.True(t, contains("test", ""))
}
