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

// mockCheckpointRepository 模拟检查点仓储
type mockCheckpointRepository struct {
	mock.Mock
}

func (m *mockCheckpointRepository) GetByChainID(ctx context.Context, chainID int64) (*model.BlockCheckpoint, error) {
	args := m.Called(ctx, chainID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.BlockCheckpoint), args.Error(1)
}

func (m *mockCheckpointRepository) Upsert(ctx context.Context, checkpoint *model.BlockCheckpoint) error {
	args := m.Called(ctx, checkpoint)
	return args.Error(0)
}

func (m *mockCheckpointRepository) UpdateBlockNumber(ctx context.Context, chainID int64, blockNumber int64, blockHash string) error {
	args := m.Called(ctx, chainID, blockNumber, blockHash)
	return args.Error(0)
}

func (m *mockCheckpointRepository) CreateEvent(ctx context.Context, event *model.ChainEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *mockCheckpointRepository) GetEventByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.ChainEvent, error) {
	args := m.Called(ctx, txHash, logIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ChainEvent), args.Error(1)
}

func (m *mockCheckpointRepository) ListUnprocessedEvents(ctx context.Context, chainID int64, limit int) ([]*model.ChainEvent, error) {
	args := m.Called(ctx, chainID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.ChainEvent), args.Error(1)
}

func (m *mockCheckpointRepository) MarkEventProcessed(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockCheckpointRepository) ListEventsByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.ChainEvent, error) {
	args := m.Called(ctx, chainID, startBlock, endBlock)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.ChainEvent), args.Error(1)
}

// mockDepositRepository 模拟充值仓储
type mockDepositRepository struct {
	mock.Mock
	mu       sync.RWMutex
	deposits map[string]*model.DepositRecord
}

func newMockDepositRepository() *mockDepositRepository {
	return &mockDepositRepository{
		deposits: make(map[string]*model.DepositRecord),
	}
}

func (m *mockDepositRepository) Create(ctx context.Context, record *model.DepositRecord) error {
	args := m.Called(ctx, record)
	if args.Error(0) == nil {
		m.mu.Lock()
		m.deposits[record.DepositID] = record
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *mockDepositRepository) GetByDepositID(ctx context.Context, depositID string) (*model.DepositRecord, error) {
	args := m.Called(ctx, depositID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DepositRecord), args.Error(1)
}

func (m *mockDepositRepository) GetByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.DepositRecord, error) {
	args := m.Called(ctx, txHash, logIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DepositRecord), args.Error(1)
}

func (m *mockDepositRepository) Update(ctx context.Context, record *model.DepositRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

func (m *mockDepositRepository) ExistsByTxHash(ctx context.Context, txHash string) (bool, error) {
	args := m.Called(ctx, txHash)
	return args.Bool(0), args.Error(1)
}

func (m *mockDepositRepository) UpdateStatus(ctx context.Context, depositID string, status model.DepositRecordStatus) error {
	args := m.Called(ctx, depositID, status)
	return args.Error(0)
}

func (m *mockDepositRepository) UpdateConfirmations(ctx context.Context, depositID string, confirmations int) error {
	args := m.Called(ctx, depositID, confirmations)
	return args.Error(0)
}

func (m *mockDepositRepository) ListPendingConfirmation(ctx context.Context, limit int) ([]*model.DepositRecord, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DepositRecord), args.Error(1)
}

func (m *mockDepositRepository) ListByWallet(ctx context.Context, wallet string, page *repository.Pagination) ([]*model.DepositRecord, error) {
	args := m.Called(ctx, wallet, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DepositRecord), args.Error(1)
}

func (m *mockDepositRepository) ListByStatus(ctx context.Context, status model.DepositRecordStatus, page *repository.Pagination) ([]*model.DepositRecord, error) {
	args := m.Called(ctx, status, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DepositRecord), args.Error(1)
}

func (m *mockDepositRepository) ListByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.DepositRecord, error) {
	args := m.Called(ctx, chainID, startBlock, endBlock)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DepositRecord), args.Error(1)
}

// createTestIndexerService 创建测试用的索引服务
func createTestIndexerService(checkpointRepo *mockCheckpointRepository, depositRepo *mockDepositRepository) *IndexerService {
	return &IndexerService{
		checkpointRepo:     checkpointRepo,
		depositRepo:        depositRepo,
		chainID:            31337,
		pollInterval:       100 * time.Millisecond,
		checkpointInterval: 10,
		requiredConfirms:   0, // Arbitrum
		running:            false,
	}
}

func TestIndexerService_StartStop(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	// 初始应该未运行
	assert.False(t, svc.IsRunning())

	// 模拟启动状态
	svc.running = true
	svc.stopCh = make(chan struct{})

	assert.True(t, svc.IsRunning())

	// 停止
	err := svc.Stop()
	assert.NoError(t, err)
	assert.False(t, svc.IsRunning())
}

func TestIndexerService_StartWhenAlreadyRunning(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	// 设置为运行中
	svc.running = true

	ctx := context.Background()
	err := svc.Start(ctx)
	assert.ErrorIs(t, err, ErrIndexerAlreadyRunning)
}

func TestIndexerService_StopWhenNotRunning(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	err := svc.Stop()
	assert.ErrorIs(t, err, ErrIndexerNotRunning)
}

func TestIndexerService_GetCurrentBlock(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	// 初始应该为 0
	assert.Equal(t, uint64(0), svc.GetCurrentBlock())

	// 设置当前区块
	svc.mu.Lock()
	svc.currentBlock = 12345
	svc.mu.Unlock()

	assert.Equal(t, uint64(12345), svc.GetCurrentBlock())
}

func TestIndexerService_SetOnDepositDetected(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	callbackCalled := false
	svc.SetOnDepositDetected(func(ctx context.Context, deposit *model.DepositEvent) error {
		callbackCalled = true
		return nil
	})

	assert.NotNil(t, svc.onDepositDetected)

	// 模拟调用回调
	ctx := context.Background()
	deposit := &model.DepositEvent{
		DepositID: "dep-1",
		Wallet:    "0x123",
		Token:     "USDC",
		Amount:    decimal.NewFromFloat(1000),
	}
	err := svc.onDepositDetected(ctx, deposit)
	assert.NoError(t, err)
	assert.True(t, callbackCalled)
}

func TestIndexerService_GetDepositStatus(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	ctx := context.Background()
	depositID := "dep-123"

	expectedRecord := &model.DepositRecord{
		DepositID:     depositID,
		WalletAddress: "0x123",
		Token:         "USDC",
		Amount:        decimal.NewFromFloat(1000),
		Status:        model.DepositRecordStatusConfirmed,
	}

	depositRepo.On("GetByDepositID", ctx, depositID).Return(expectedRecord, nil)

	record, err := svc.GetDepositStatus(ctx, depositID)
	assert.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, depositID, record.DepositID)
	assert.Equal(t, model.DepositRecordStatusConfirmed, record.Status)
}

func TestIndexerService_GetDepositByTxHash(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	ctx := context.Background()
	txHash := "0xabc123"
	logIndex := 0

	expectedRecord := &model.DepositRecord{
		DepositID:     "dep-123",
		TxHash:        txHash,
		LogIndex:      logIndex,
		WalletAddress: "0x123",
		Token:         "USDC",
		Amount:        decimal.NewFromFloat(1000),
	}

	depositRepo.On("GetByTxHashAndLogIndex", ctx, txHash, logIndex).Return(expectedRecord, nil)

	record, err := svc.GetDepositByTxHash(ctx, txHash, logIndex)
	assert.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, txHash, record.TxHash)
}

// ======== 并发测试 ========

func TestIndexerService_ConcurrentGetCurrentBlock(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	var wg sync.WaitGroup
	concurrency := 20
	iterations := 100

	// 并发更新
	for i := 0; i < concurrency/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				svc.mu.Lock()
				svc.currentBlock = uint64(id*iterations + j)
				svc.mu.Unlock()
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < concurrency/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = svc.GetCurrentBlock()
			}
		}()
	}

	wg.Wait()
}

func TestIndexerService_ConcurrentIsRunning(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	var wg sync.WaitGroup
	concurrency := 20
	iterations := 100

	// 并发更新状态
	for i := 0; i < concurrency/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				svc.mu.Lock()
				svc.running = j%2 == 0
				svc.mu.Unlock()
			}
		}()
	}

	// 并发读取状态
	for i := 0; i < concurrency/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = svc.IsRunning()
			}
		}()
	}

	wg.Wait()
}

func TestIndexerService_GetTokenSymbol(t *testing.T) {
	checkpointRepo := &mockCheckpointRepository{}
	depositRepo := newMockDepositRepository()
	svc := createTestIndexerService(checkpointRepo, depositRepo)

	// 目前是硬编码返回 USDC
	symbol := svc.getTokenSymbol("0xtoken")
	assert.Equal(t, "USDC", symbol)
}

// TestIndexerStatus 测试索引器状态结构
func TestIndexerStatus_Fields(t *testing.T) {
	status := &IndexerStatus{
		ChainID:         31337,
		Running:         true,
		CurrentBlock:    12345,
		LatestBlock:     12350,
		LagBlocks:       5,
		CheckpointBlock: 12340,
	}

	assert.Equal(t, int64(31337), status.ChainID)
	assert.True(t, status.Running)
	assert.Equal(t, uint64(12345), status.CurrentBlock)
	assert.Equal(t, uint64(12350), status.LatestBlock)
	assert.Equal(t, int64(5), status.LagBlocks)
	assert.Equal(t, int64(12340), status.CheckpointBlock)
}
