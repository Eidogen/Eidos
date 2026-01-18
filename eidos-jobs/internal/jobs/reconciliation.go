package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"go.uber.org/zap"
)

// ReconciliationDataProvider 对账数据提供者接口
type ReconciliationDataProvider interface {
	// GetOffchainBalances 获取链下余额
	GetOffchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error)
	// GetOnchainBalances 获取链上余额
	GetOnchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error)
	// GetChangedWallets 获取在指定时间范围内有变动的钱包
	GetChangedWallets(ctx context.Context, startTime, endTime int64) ([]string, error)
	// GetAllWallets 获取所有有余额的钱包
	GetAllWallets(ctx context.Context) ([]string, error)
	// GetLatestBlockNumber 获取最新区块号
	GetLatestBlockNumber(ctx context.Context) (int64, error)
}

// ReconciliationJob 对账任务
type ReconciliationJob struct {
	scheduler.BaseJob
	reconRepo    *repository.ReconciliationRepository
	dataProvider ReconciliationDataProvider
	alertFunc    func(ctx context.Context, alert *ReconciliationAlert) error
	batchSize    int
}

// ReconciliationAlert 对账告警
type ReconciliationAlert struct {
	JobType     string
	Severity    string // "warning", "critical"
	WalletCount int
	TotalDiff   decimal.Decimal
	Details     map[string]interface{}
	Timestamp   int64
}

// NewReconciliationJob 创建对账任务
func NewReconciliationJob(
	reconRepo *repository.ReconciliationRepository,
	dataProvider ReconciliationDataProvider,
) *ReconciliationJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameReconciliation]

	return &ReconciliationJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameReconciliation,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		reconRepo:    reconRepo,
		dataProvider: dataProvider,
		batchSize:    100,
	}
}

// SetAlertFunc 设置告警回调
func (j *ReconciliationJob) SetAlertFunc(f func(ctx context.Context, alert *ReconciliationAlert) error) {
	j.alertFunc = f
}

// Execute 执行对账
func (j *ReconciliationJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	now := time.Now()

	// 获取检查点
	checkpoint, err := j.reconRepo.GetCheckpoint(ctx, string(model.ReconciliationJobBalance))
	if err != nil {
		return result, fmt.Errorf("failed to get checkpoint: %w", err)
	}

	// 判断是增量对账还是全量对账
	reconcileType := model.ReconciliationTypeIncremental
	var wallets []string

	if checkpoint == nil || (now.Hour() == 2 && now.Minute() < 60) {
		// 每天凌晨 2 点进行全量对账，或者首次运行
		reconcileType = model.ReconciliationTypeFull
		wallets, err = j.dataProvider.GetAllWallets(ctx)
		if err != nil {
			return result, fmt.Errorf("failed to get all wallets: %w", err)
		}
		logger.Info("starting full reconciliation", zap.Int("wallet_count", len(wallets)))
	} else {
		// 增量对账：只对账上一小时有变动的钱包
		startTime := checkpoint.LastTime
		endTime := now.UnixMilli()
		wallets, err = j.dataProvider.GetChangedWallets(ctx, startTime, endTime)
		if err != nil {
			return result, fmt.Errorf("failed to get changed wallets: %w", err)
		}
		logger.Info("starting incremental reconciliation",
			zap.Int("wallet_count", len(wallets)),
			zap.Int64("start_time", startTime),
			zap.Int64("end_time", endTime))
	}

	if len(wallets) == 0 {
		logger.Info("no wallets to reconcile")
		result.Details["message"] = "no wallets to reconcile"
		return result, nil
	}

	// 分批对账
	matchedCount := 0
	mismatchedCount := 0
	totalDiff := decimal.Zero
	records := make([]*model.ReconciliationRecord, 0)

	for i := 0; i < len(wallets); i += j.batchSize {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		end := i + j.batchSize
		if end > len(wallets) {
			end = len(wallets)
		}
		batch := wallets[i:end]

		// 获取链下余额
		offchainBalances, err := j.dataProvider.GetOffchainBalances(ctx, batch)
		if err != nil {
			logger.Error("failed to get offchain balances", zap.Error(err))
			result.ErrorCount++
			continue
		}

		// 获取链上余额
		onchainBalances, err := j.dataProvider.GetOnchainBalances(ctx, batch)
		if err != nil {
			logger.Error("failed to get onchain balances", zap.Error(err))
			result.ErrorCount++
			continue
		}

		// 对比余额
		for _, wallet := range batch {
			offchain := offchainBalances[wallet]
			onchain := onchainBalances[wallet]

			// 合并所有代币
			tokens := make(map[string]bool)
			for token := range offchain {
				tokens[token] = true
			}
			for token := range onchain {
				tokens[token] = true
			}

			for token := range tokens {
				offchainVal := offchain[token]
				onchainVal := onchain[token]
				diff := offchainVal.Sub(onchainVal).Abs()

				status := string(model.ReconciliationStatusMatched)
				if !diff.IsZero() {
					status = string(model.ReconciliationStatusMismatched)
					mismatchedCount++
					totalDiff = totalDiff.Add(diff)
				} else {
					matchedCount++
				}

				offchainStr := offchainVal.String()
				onchainStr := onchainVal.String()
				diffStr := diff.String()

				records = append(records, &model.ReconciliationRecord{
					JobType:       string(model.ReconciliationJobBalance),
					ReconcileType: string(reconcileType),
					Status:        status,
					WalletAddress: &wallet,
					Token:         &token,
					OffchainValue: &offchainStr,
					OnchainValue:  &onchainStr,
					DiffValue:     &diffStr,
				})
			}
		}

		result.ProcessedCount += len(batch)
	}

	// 保存对账记录
	if len(records) > 0 {
		if err := j.reconRepo.BatchCreate(ctx, records); err != nil {
			logger.Error("failed to save reconciliation records", zap.Error(err))
			result.ErrorCount++
		}
	}

	// 更新检查点
	newCheckpoint := &model.ReconciliationCheckpoint{
		JobType:  string(model.ReconciliationJobBalance),
		LastTime: now.UnixMilli(),
	}
	if err := j.reconRepo.UpsertCheckpoint(ctx, newCheckpoint); err != nil {
		logger.Error("failed to update checkpoint", zap.Error(err))
	}

	// 发送告警
	if mismatchedCount > 0 {
		j.sendAlert(ctx, totalDiff, mismatchedCount)
	}

	result.AffectedCount = mismatchedCount
	result.Details["matched_count"] = matchedCount
	result.Details["mismatched_count"] = mismatchedCount
	result.Details["total_diff"] = totalDiff.String()
	result.Details["reconcile_type"] = string(reconcileType)

	logger.Info("reconciliation completed",
		zap.Int("processed", result.ProcessedCount),
		zap.Int("matched", matchedCount),
		zap.Int("mismatched", mismatchedCount),
		zap.String("total_diff", totalDiff.String()))

	return result, nil
}

// sendAlert 发送对账告警
func (j *ReconciliationJob) sendAlert(ctx context.Context, totalDiff decimal.Decimal, mismatchedCount int) {
	if j.alertFunc == nil {
		return
	}

	// 根据差异金额确定告警级别
	severity := "info"
	smallThreshold, _ := decimal.NewFromString(model.DiffThresholdSmall)
	largeThreshold, _ := decimal.NewFromString(model.DiffThresholdLarge)

	if totalDiff.GreaterThan(largeThreshold) {
		severity = "critical"
	} else if totalDiff.GreaterThan(smallThreshold) {
		severity = "warning"
	}

	if severity == "info" {
		// 小额差异仅记录日志，不发送告警
		logger.Info("small reconciliation diff detected, skipping alert",
			zap.String("total_diff", totalDiff.String()),
			zap.Int("count", mismatchedCount))
		return
	}

	alert := &ReconciliationAlert{
		JobType:     string(model.ReconciliationJobBalance),
		Severity:    severity,
		WalletCount: mismatchedCount,
		TotalDiff:   totalDiff,
		Details: map[string]interface{}{
			"mismatched_count": mismatchedCount,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	if err := j.alertFunc(ctx, alert); err != nil {
		logger.Error("failed to send reconciliation alert", zap.Error(err))
	}
}

// MockReconciliationDataProvider 模拟对账数据提供者 (用于测试)
type MockReconciliationDataProvider struct{}

func (p *MockReconciliationDataProvider) GetOffchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	// TODO: 实际实现通过 gRPC 调用 trading 服务
	return make(map[string]map[string]decimal.Decimal), nil
}

func (p *MockReconciliationDataProvider) GetOnchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	// TODO: 实际实现通过 gRPC 调用 chain 服务
	return make(map[string]map[string]decimal.Decimal), nil
}

func (p *MockReconciliationDataProvider) GetChangedWallets(ctx context.Context, startTime, endTime int64) ([]string, error) {
	// TODO: 实际实现通过查询数据库
	return nil, nil
}

func (p *MockReconciliationDataProvider) GetAllWallets(ctx context.Context) ([]string, error) {
	// TODO: 实际实现通过查询数据库
	return nil, nil
}

func (p *MockReconciliationDataProvider) GetLatestBlockNumber(ctx context.Context) (int64, error) {
	// TODO: 实际实现通过 gRPC 调用 chain 服务
	return 0, nil
}
