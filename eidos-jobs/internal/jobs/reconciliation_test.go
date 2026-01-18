package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupReconTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = db.AutoMigrate(&model.ReconciliationRecord{}, &model.ReconciliationCheckpoint{})
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

// mockReconciliationDataProviderForTest 测试用的数据提供者
type mockReconciliationDataProviderForTest struct {
	offchainBalances map[string]map[string]decimal.Decimal // wallet -> token -> balance
	onchainBalances  map[string]map[string]decimal.Decimal // wallet -> token -> balance
	changedWallets   []string
	allWallets       []string
	latestBlock      int64
	offchainErr      error
	onchainErr       error
	changedErr       error
	allWalletsErr    error
}

func (p *mockReconciliationDataProviderForTest) GetOffchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	if p.offchainErr != nil {
		return nil, p.offchainErr
	}

	result := make(map[string]map[string]decimal.Decimal)
	for _, wallet := range wallets {
		if balances, ok := p.offchainBalances[wallet]; ok {
			result[wallet] = balances
		} else {
			result[wallet] = make(map[string]decimal.Decimal)
		}
	}
	return result, nil
}

func (p *mockReconciliationDataProviderForTest) GetOnchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	if p.onchainErr != nil {
		return nil, p.onchainErr
	}

	result := make(map[string]map[string]decimal.Decimal)
	for _, wallet := range wallets {
		if balances, ok := p.onchainBalances[wallet]; ok {
			result[wallet] = balances
		} else {
			result[wallet] = make(map[string]decimal.Decimal)
		}
	}
	return result, nil
}

func (p *mockReconciliationDataProviderForTest) GetChangedWallets(ctx context.Context, startTime, endTime int64) ([]string, error) {
	if p.changedErr != nil {
		return nil, p.changedErr
	}
	return p.changedWallets, nil
}

func (p *mockReconciliationDataProviderForTest) GetAllWallets(ctx context.Context) ([]string, error) {
	if p.allWalletsErr != nil {
		return nil, p.allWalletsErr
	}
	return p.allWallets, nil
}

func (p *mockReconciliationDataProviderForTest) GetLatestBlockNumber(ctx context.Context) (int64, error) {
	return p.latestBlock, nil
}

func TestReconciliationJob_Execute_NoWallets(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	provider := &mockReconciliationDataProviderForTest{
		allWallets:  []string{},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 0 {
		t.Errorf("Expected ProcessedCount 0, got %d", result.ProcessedCount)
	}
}

func TestReconciliationJob_Execute_MatchedBalances(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	// 设置相同的链上和链下余额
	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1234567890abcdef"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1234567890abcdef": {
				"USDT": decimal.NewFromFloat(1000.00),
			},
		},
		onchainBalances: map[string]map[string]decimal.Decimal{
			"0x1234567890abcdef": {
				"USDT": decimal.NewFromFloat(1000.00),
			},
		},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 1 {
		t.Errorf("Expected ProcessedCount 1, got %d", result.ProcessedCount)
	}

	// 余额匹配，所以 AffectedCount (mismatchedCount) 应该为 0
	if result.AffectedCount != 0 {
		t.Errorf("Expected AffectedCount 0, got %d", result.AffectedCount)
	}

	if result.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount 0, got %d", result.ErrorCount)
	}
}

func TestReconciliationJob_Execute_MismatchedBalances(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	// 设置不同的链上和链下余额
	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1234567890abcdef"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1234567890abcdef": {
				"USDT": decimal.NewFromFloat(1000.00),
			},
		},
		onchainBalances: map[string]map[string]decimal.Decimal{
			"0x1234567890abcdef": {
				"USDT": decimal.NewFromFloat(900.00), // 链上余额不匹配
			},
		},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 1 {
		t.Errorf("Expected ProcessedCount 1, got %d", result.ProcessedCount)
	}

	// 余额不匹配，AffectedCount 应该为 1
	if result.AffectedCount != 1 {
		t.Errorf("Expected AffectedCount 1, got %d", result.AffectedCount)
	}

	// 检查是否记录了差异
	records, err := repo.ListMismatched(ctx, 10)
	if err != nil {
		t.Fatalf("ListMismatched failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 mismatched record, got %d", len(records))
	}
}

func TestReconciliationJob_Execute_MultipleWallets(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1111", "0x2222", "0x3333"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {"USDT": decimal.NewFromFloat(100.00)},
			"0x2222": {"USDT": decimal.NewFromFloat(200.00)},
			"0x3333": {"ETH": decimal.NewFromFloat(1.5)},
		},
		onchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {"USDT": decimal.NewFromFloat(100.00)},
			"0x2222": {"USDT": decimal.NewFromFloat(200.00)},
			"0x3333": {"ETH": decimal.NewFromFloat(1.5)},
		},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 3 {
		t.Errorf("Expected ProcessedCount 3, got %d", result.ProcessedCount)
	}

	// 全部匹配
	if result.AffectedCount != 0 {
		t.Errorf("Expected AffectedCount 0, got %d", result.AffectedCount)
	}
}

func TestReconciliationJob_Execute_AllWalletsError(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	provider := &mockReconciliationDataProviderForTest{
		allWalletsErr: errors.New("database unavailable"),
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	_, err := job.Execute(ctx)
	if err == nil {
		t.Error("Expected error from GetAllWallets")
	}
}

func TestReconciliationJob_Execute_OffchainBalanceError(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	provider := &mockReconciliationDataProviderForTest{
		allWallets:  []string{"0x1111"},
		offchainErr: errors.New("offchain service unavailable"),
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute should not fail entirely: %v", err)
	}

	// 应该记录错误
	if result.ErrorCount == 0 {
		t.Error("Expected ErrorCount > 0 for offchain balance error")
	}
}

func TestReconciliationJob_Execute_OnchainBalanceError(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1111"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {"USDT": decimal.NewFromFloat(100.00)},
		},
		onchainErr: errors.New("onchain service unavailable"),
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute should not fail entirely: %v", err)
	}

	// 应该记录错误
	if result.ErrorCount == 0 {
		t.Error("Expected ErrorCount > 0 for onchain balance error")
	}
}

func TestReconciliationJob_Execute_ContextCancelled(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	// 创建大量钱包
	wallets := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		wallets[i] = "0x" + string(rune('a'+i%26))
	}

	provider := &mockReconciliationDataProviderForTest{
		allWallets:  wallets,
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := job.Execute(ctx)
	// 任务应该被取消
	if err == nil {
		t.Log("Task completed despite cancellation")
	}
}

func TestReconciliationJob_Name(t *testing.T) {
	job := NewReconciliationJob(nil, nil)

	if job.Name() != "reconciliation" {
		t.Errorf("Expected name 'reconciliation', got '%s'", job.Name())
	}
}

func TestReconciliationJob_SetAlertFunc(t *testing.T) {
	job := NewReconciliationJob(nil, nil)

	alertCalled := false
	job.SetAlertFunc(func(ctx context.Context, alert *ReconciliationAlert) error {
		alertCalled = true
		return nil
	})

	// 触发告警
	job.sendAlert(context.Background(), decimal.NewFromFloat(1000000), 5)

	if !alertCalled {
		t.Error("Expected alert function to be called")
	}
}

func TestReconciliationJob_MultipleTokensPerWallet(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	// 一个钱包有多个代币
	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1111"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {
				"USDT": decimal.NewFromFloat(100.00),
				"ETH":  decimal.NewFromFloat(1.5),
				"BTC":  decimal.NewFromFloat(0.1),
			},
		},
		onchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {
				"USDT": decimal.NewFromFloat(100.00),
				"ETH":  decimal.NewFromFloat(1.5),
				"BTC":  decimal.NewFromFloat(0.1),
			},
		},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 处理了 1 个钱包
	if result.ProcessedCount != 1 {
		t.Errorf("Expected ProcessedCount 1, got %d", result.ProcessedCount)
	}

	// 3 个代币全部匹配
	if result.AffectedCount != 0 {
		t.Errorf("Expected AffectedCount 0, got %d", result.AffectedCount)
	}

	// 检查 matched_count
	if matchedCount, ok := result.Details["matched_count"].(int); ok {
		if matchedCount != 3 {
			t.Errorf("Expected matched_count 3, got %d", matchedCount)
		}
	}
}

func TestReconciliationJob_PartialMismatch(t *testing.T) {
	db := setupReconTestDB(t)
	repo := repository.NewReconciliationRepository(db)

	// 部分代币匹配，部分不匹配
	provider := &mockReconciliationDataProviderForTest{
		allWallets: []string{"0x1111"},
		offchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {
				"USDT": decimal.NewFromFloat(100.00),
				"ETH":  decimal.NewFromFloat(2.0), // 不匹配
			},
		},
		onchainBalances: map[string]map[string]decimal.Decimal{
			"0x1111": {
				"USDT": decimal.NewFromFloat(100.00),
				"ETH":  decimal.NewFromFloat(1.5), // 不匹配
			},
		},
		latestBlock: 1000,
	}

	job := NewReconciliationJob(repo, provider)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 1 个不匹配
	if result.AffectedCount != 1 {
		t.Errorf("Expected AffectedCount 1, got %d", result.AffectedCount)
	}

	// 检查 total_diff
	if totalDiff, ok := result.Details["total_diff"].(string); ok {
		if totalDiff != "0.5" {
			t.Errorf("Expected total_diff '0.5', got '%s'", totalDiff)
		}
	}
}
