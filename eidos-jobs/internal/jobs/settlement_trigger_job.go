// Package jobs 结算触发任务
// 定时触发结算批次提交和超时结算重试
package jobs

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
)

// SettlementConfig 结算配置
type SettlementConfig struct {
	// BatchSize 每次提交的最大交易数
	BatchSize int
	// MinBatchSize 最小批次大小（达到此数量才触发提交）
	MinBatchSize int
	// MaxWaitTime 最大等待时间（超过此时间即使未达到 MinBatchSize 也提交）
	MaxWaitTime time.Duration
	// RetryTimeout 重试超时时间（超过此时间的待处理结算需要重试）
	RetryTimeout time.Duration
	// MaxRetries 最大重试次数
	MaxRetries int
	// ConcurrentBatches 并发处理的批次数
	ConcurrentBatches int
}

// DefaultSettlementConfig 默认配置
var DefaultSettlementConfig = SettlementConfig{
	BatchSize:         100,
	MinBatchSize:      10,
	MaxWaitTime:       5 * time.Minute,
	RetryTimeout:      10 * time.Minute,
	MaxRetries:        3,
	ConcurrentBatches: 2,
}

// SettlementBatch 结算批次
type SettlementBatch struct {
	BatchID     string
	Market      string
	TradeIDs    []string
	TotalAmount string
	CreatedAt   int64
	Status      string
	RetryCount  int
}

// SettlementStatus 结算状态
type SettlementStatus string

const (
	SettlementStatusPending   SettlementStatus = "pending"
	SettlementStatusSubmitted SettlementStatus = "submitted"
	SettlementStatusConfirmed SettlementStatus = "confirmed"
	SettlementStatusFailed    SettlementStatus = "failed"
	SettlementStatusTimeout   SettlementStatus = "timeout"
)

// SettlementDataProvider 结算数据提供者接口
type SettlementDataProvider interface {
	// GetPendingTrades 获取待结算的交易
	GetPendingTrades(ctx context.Context, limit int) ([]*PendingTrade, error)
	// CreateSettlementBatch 创建结算批次
	CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*SettlementBatch, error)
	// SubmitSettlementBatch 提交结算批次到链上
	SubmitSettlementBatch(ctx context.Context, batchID string) error
	// GetTimedOutBatches 获取超时的批次
	GetTimedOutBatches(ctx context.Context, timeout time.Duration) ([]*SettlementBatch, error)
	// RetrySettlementBatch 重试结算批次
	RetrySettlementBatch(ctx context.Context, batchID string) error
	// MarkBatchFailed 标记批次失败
	MarkBatchFailed(ctx context.Context, batchID string, reason string) error
	// GetLastBatchTime 获取上次批次创建时间
	GetLastBatchTime(ctx context.Context) (time.Time, error)
	// GetPendingTradeCount 获取待结算交易数量
	GetPendingTradeCount(ctx context.Context) (int, error)
}

// PendingTrade 待结算交易
type PendingTrade struct {
	TradeID     string
	Market      string
	MakerWallet string
	TakerWallet string
	Amount      string
	Price       string
	CreatedAt   int64
}

// SettlementTriggerJob 结算触发任务
type SettlementTriggerJob struct {
	scheduler.BaseJob
	dataProvider SettlementDataProvider
	config       SettlementConfig
	alertFunc    func(ctx context.Context, alert *SettlementAlert) error
}

// SettlementAlert 结算告警
type SettlementAlert struct {
	AlertType   string // "batch_failed", "timeout_retry", "high_pending"
	BatchID     string
	Message     string
	Details     map[string]interface{}
	Timestamp   int64
}

// NewSettlementTriggerJob 创建结算触发任务
func NewSettlementTriggerJob(
	dataProvider SettlementDataProvider,
	config *SettlementConfig,
) *SettlementTriggerJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameSettlementTrigger]

	jobConfig := DefaultSettlementConfig
	if config != nil {
		jobConfig = *config
	}

	return &SettlementTriggerJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameSettlementTrigger,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		dataProvider: dataProvider,
		config:       jobConfig,
	}
}

// SetAlertFunc 设置告警回调
func (j *SettlementTriggerJob) SetAlertFunc(f func(ctx context.Context, alert *SettlementAlert) error) {
	j.alertFunc = f
}

// Execute 执行结算触发
func (j *SettlementTriggerJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	startTime := time.Now()
	logger.Info("starting settlement trigger job")

	// 1. 处理超时批次重试
	retryCount, retryErrors := j.processTimeoutBatches(ctx)
	result.Details["retry_count"] = retryCount
	result.Details["retry_errors"] = retryErrors
	result.ErrorCount += retryErrors

	// 2. 检查是否需要创建新批次
	batchCreated, batchSubmitted, batchErrors := j.processPendingTrades(ctx)
	result.Details["batches_created"] = batchCreated
	result.Details["batches_submitted"] = batchSubmitted
	result.ErrorCount += batchErrors

	result.ProcessedCount = retryCount + batchCreated
	result.AffectedCount = batchSubmitted

	duration := time.Since(startTime)
	result.Details["duration_ms"] = duration.Milliseconds()

	logger.Info("settlement trigger job completed",
		zap.Int("retry_count", retryCount),
		zap.Int("batches_created", batchCreated),
		zap.Int("batches_submitted", batchSubmitted),
		zap.Int("errors", result.ErrorCount),
		zap.Duration("duration", duration))

	return result, nil
}

// processTimeoutBatches 处理超时的批次
func (j *SettlementTriggerJob) processTimeoutBatches(ctx context.Context) (int, int) {
	timedOutBatches, err := j.dataProvider.GetTimedOutBatches(ctx, j.config.RetryTimeout)
	if err != nil {
		logger.Error("failed to get timed out batches", zap.Error(err))
		return 0, 1
	}

	if len(timedOutBatches) == 0 {
		return 0, 0
	}

	logger.Info("found timed out settlement batches", zap.Int("count", len(timedOutBatches)))

	retryCount := 0
	errorCount := 0

	for _, batch := range timedOutBatches {
		select {
		case <-ctx.Done():
			return retryCount, errorCount
		default:
		}

		// 检查重试次数
		if batch.RetryCount >= j.config.MaxRetries {
			logger.Error("batch exceeded max retries, marking as failed",
				zap.String("batch_id", batch.BatchID),
				zap.Int("retry_count", batch.RetryCount))

			if err := j.dataProvider.MarkBatchFailed(ctx, batch.BatchID, "exceeded max retries"); err != nil {
				logger.Error("failed to mark batch as failed",
					zap.String("batch_id", batch.BatchID),
					zap.Error(err))
				errorCount++
			}

			// 发送告警
			j.sendAlert(ctx, "batch_failed", batch.BatchID,
				fmt.Sprintf("Settlement batch %s failed after %d retries", batch.BatchID, batch.RetryCount),
				map[string]interface{}{
					"market":      batch.Market,
					"trade_count": len(batch.TradeIDs),
				})

			continue
		}

		// 重试批次
		logger.Info("retrying timed out batch",
			zap.String("batch_id", batch.BatchID),
			zap.Int("retry_count", batch.RetryCount))

		if err := j.dataProvider.RetrySettlementBatch(ctx, batch.BatchID); err != nil {
			logger.Error("failed to retry settlement batch",
				zap.String("batch_id", batch.BatchID),
				zap.Error(err))
			errorCount++

			j.sendAlert(ctx, "timeout_retry", batch.BatchID,
				fmt.Sprintf("Failed to retry settlement batch %s: %v", batch.BatchID, err),
				map[string]interface{}{
					"retry_count": batch.RetryCount,
				})
			continue
		}

		retryCount++
	}

	return retryCount, errorCount
}

// processPendingTrades 处理待结算的交易
func (j *SettlementTriggerJob) processPendingTrades(ctx context.Context) (int, int, int) {
	// 获取待结算交易数量
	pendingCount, err := j.dataProvider.GetPendingTradeCount(ctx)
	if err != nil {
		logger.Error("failed to get pending trade count", zap.Error(err))
		return 0, 0, 1
	}

	if pendingCount == 0 {
		logger.Debug("no pending trades")
		return 0, 0, 0
	}

	// 检查是否需要创建批次
	lastBatchTime, err := j.dataProvider.GetLastBatchTime(ctx)
	if err != nil {
		logger.Warn("failed to get last batch time, assuming should create batch", zap.Error(err))
		lastBatchTime = time.Time{} // 如果获取失败，假设很久以前
	}

	timeSinceLastBatch := time.Since(lastBatchTime)
	shouldCreateBatch := pendingCount >= j.config.MinBatchSize ||
		(pendingCount > 0 && timeSinceLastBatch > j.config.MaxWaitTime)

	if !shouldCreateBatch {
		logger.Debug("not enough pending trades and max wait time not reached",
			zap.Int("pending_count", pendingCount),
			zap.Int("min_batch_size", j.config.MinBatchSize),
			zap.Duration("time_since_last_batch", timeSinceLastBatch),
			zap.Duration("max_wait_time", j.config.MaxWaitTime))
		return 0, 0, 0
	}

	// 如果积压较多，发送告警
	if pendingCount > j.config.BatchSize*3 {
		j.sendAlert(ctx, "high_pending", "",
			fmt.Sprintf("High pending settlement count: %d", pendingCount),
			map[string]interface{}{
				"pending_count": pendingCount,
				"threshold":     j.config.BatchSize * 3,
			})
	}

	batchCreated := 0
	batchSubmitted := 0
	errorCount := 0

	// 创建并提交批次
	for pendingCount > 0 {
		select {
		case <-ctx.Done():
			return batchCreated, batchSubmitted, errorCount
		default:
		}

		// 获取待结算交易
		trades, err := j.dataProvider.GetPendingTrades(ctx, j.config.BatchSize)
		if err != nil {
			logger.Error("failed to get pending trades", zap.Error(err))
			errorCount++
			break
		}

		if len(trades) == 0 {
			break
		}

		// 提取交易ID
		tradeIDs := make([]string, len(trades))
		for i, trade := range trades {
			tradeIDs[i] = trade.TradeID
		}

		// 创建批次
		batch, err := j.dataProvider.CreateSettlementBatch(ctx, tradeIDs)
		if err != nil {
			logger.Error("failed to create settlement batch", zap.Error(err))
			errorCount++
			break
		}

		batchCreated++
		logger.Info("created settlement batch",
			zap.String("batch_id", batch.BatchID),
			zap.Int("trade_count", len(tradeIDs)))

		// 提交批次
		if err := j.dataProvider.SubmitSettlementBatch(ctx, batch.BatchID); err != nil {
			logger.Error("failed to submit settlement batch",
				zap.String("batch_id", batch.BatchID),
				zap.Error(err))
			errorCount++
			continue
		}

		batchSubmitted++
		logger.Info("submitted settlement batch",
			zap.String("batch_id", batch.BatchID))

		pendingCount -= len(trades)

		// 如果处理的数量少于批次大小，说明已处理完毕
		if len(trades) < j.config.BatchSize {
			break
		}
	}

	return batchCreated, batchSubmitted, errorCount
}

// sendAlert 发送告警
func (j *SettlementTriggerJob) sendAlert(ctx context.Context, alertType, batchID, message string, details map[string]interface{}) {
	if j.alertFunc == nil {
		return
	}

	alert := &SettlementAlert{
		AlertType: alertType,
		BatchID:   batchID,
		Message:   message,
		Details:   details,
		Timestamp: time.Now().UnixMilli(),
	}

	if err := j.alertFunc(ctx, alert); err != nil {
		logger.Warn("failed to send settlement alert",
			zap.String("alert_type", alertType),
			zap.Error(err))
	}
}

// MockSettlementDataProvider 模拟数据提供者（用于测试）
type MockSettlementDataProvider struct{}

func (p *MockSettlementDataProvider) GetPendingTrades(ctx context.Context, limit int) ([]*PendingTrade, error) {
	return nil, nil
}

func (p *MockSettlementDataProvider) CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*SettlementBatch, error) {
	return &SettlementBatch{
		BatchID: fmt.Sprintf("batch-%d", time.Now().UnixNano()),
	}, nil
}

func (p *MockSettlementDataProvider) SubmitSettlementBatch(ctx context.Context, batchID string) error {
	return nil
}

func (p *MockSettlementDataProvider) GetTimedOutBatches(ctx context.Context, timeout time.Duration) ([]*SettlementBatch, error) {
	return nil, nil
}

func (p *MockSettlementDataProvider) RetrySettlementBatch(ctx context.Context, batchID string) error {
	return nil
}

func (p *MockSettlementDataProvider) MarkBatchFailed(ctx context.Context, batchID string, reason string) error {
	return nil
}

func (p *MockSettlementDataProvider) GetLastBatchTime(ctx context.Context) (time.Time, error) {
	return time.Now(), nil
}

func (p *MockSettlementDataProvider) GetPendingTradeCount(ctx context.Context) (int, error) {
	return 0, nil
}
