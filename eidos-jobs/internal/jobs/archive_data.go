package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"go.uber.org/zap"
)

// ArchiveDataProvider 归档数据提供者接口
type ArchiveDataProvider interface {
	// GetArchivableRecords 获取可归档的记录
	GetArchivableRecords(ctx context.Context, tableName string, beforeTime int64, lastID int64, limit int) ([]int64, error)
	// ArchiveRecords 归档记录 (移动到归档表或删除)
	ArchiveRecords(ctx context.Context, tableName string, ids []int64) error
	// GetMaxID 获取表中最大ID
	GetMaxID(ctx context.Context, tableName string, beforeTime int64) (int64, error)
}

// ArchiveDataJob 数据归档任务
type ArchiveDataJob struct {
	scheduler.BaseJob
	archiveRepo  *repository.ArchiveRepository
	dataProvider ArchiveDataProvider
	config       map[string]model.ArchiveConfig
	batchSize    int
	sleepMs      int
}

// NewArchiveDataJob 创建数据归档任务
func NewArchiveDataJob(
	archiveRepo *repository.ArchiveRepository,
	dataProvider ArchiveDataProvider,
	config map[string]model.ArchiveConfig,
) *ArchiveDataJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameArchiveData]

	return &ArchiveDataJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameArchiveData,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		archiveRepo:  archiveRepo,
		dataProvider: dataProvider,
		config:       config,
		batchSize:    1000,
		sleepMs:      100,
	}
}

// Execute 执行数据归档
func (j *ArchiveDataJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	tableResults := make(map[string]interface{})

	for tableName, archiveConfig := range j.config {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		tableResult, err := j.archiveTable(ctx, tableName, archiveConfig)
		if err != nil {
			logger.Error("failed to archive table",
				zap.String("table", tableName),
				zap.Error(err))
			result.ErrorCount++
			tableResults[tableName] = map[string]interface{}{
				"error": err.Error(),
			}
			continue
		}

		result.ProcessedCount += tableResult.Processed
		result.AffectedCount += tableResult.Archived
		tableResults[tableName] = tableResult
	}

	result.Details["tables"] = tableResults

	logger.Info("archive data completed",
		zap.Int("processed", result.ProcessedCount),
		zap.Int("archived", result.AffectedCount),
		zap.Int("errors", result.ErrorCount))

	return result, nil
}

// TableArchiveResult 表归档结果
type TableArchiveResult struct {
	Processed int
	Archived  int
	LastID    int64
}

// archiveTable 归档单个表
func (j *ArchiveDataJob) archiveTable(ctx context.Context, tableName string, config model.ArchiveConfig) (*TableArchiveResult, error) {
	result := &TableArchiveResult{}

	// 计算归档截止时间
	retentionDays := config.RetentionDays
	if retentionDays <= 0 {
		retentionDays = model.DefaultArchiveConfig.RetentionDays
	}
	beforeTime := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()

	// 获取上次归档进度
	progress, err := j.archiveRepo.GetProgress(ctx, tableName)
	if err != nil {
		return result, fmt.Errorf("failed to get archive progress: %w", err)
	}

	lastID := int64(0)
	if progress != nil {
		lastID = progress.LastID
	}

	// 记录归档开始
	if err := j.archiveRepo.StartArchive(ctx, tableName); err != nil {
		logger.Warn("failed to mark archive start", zap.Error(err))
	}

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = j.batchSize
	}

	sleepMs := config.SleepMs
	if sleepMs <= 0 {
		sleepMs = j.sleepMs
	}

	logger.Info("starting archive for table",
		zap.String("table", tableName),
		zap.Int64("last_id", lastID),
		zap.Int64("before_time", beforeTime),
		zap.Int("retention_days", retentionDays))

	// 分批归档
	for {
		select {
		case <-ctx.Done():
			// 保存进度
			j.archiveRepo.UpdateLastID(ctx, tableName, lastID)
			return result, ctx.Err()
		default:
		}

		// 获取可归档的记录
		ids, err := j.dataProvider.GetArchivableRecords(ctx, tableName, beforeTime, lastID, batchSize)
		if err != nil {
			return result, fmt.Errorf("failed to get archivable records: %w", err)
		}

		if len(ids) == 0 {
			break
		}

		// 归档记录
		if err := j.dataProvider.ArchiveRecords(ctx, tableName, ids); err != nil {
			return result, fmt.Errorf("failed to archive records: %w", err)
		}

		result.Processed += len(ids)
		result.Archived += len(ids)
		lastID = ids[len(ids)-1]

		// 定期保存进度
		if result.Processed%10000 == 0 {
			if err := j.archiveRepo.UpdateLastID(ctx, tableName, lastID); err != nil {
				logger.Warn("failed to update archive progress",
					zap.String("table", tableName),
					zap.Error(err))
			}
		}

		// 批次间休眠
		if sleepMs > 0 {
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
		}

		// 如果处理的数量少于批次大小，说明已经处理完了
		if len(ids) < batchSize {
			break
		}
	}

	// 记录归档完成
	if err := j.archiveRepo.FinishArchive(ctx, tableName, lastID); err != nil {
		logger.Warn("failed to mark archive finish", zap.Error(err))
	}

	result.LastID = lastID

	logger.Info("archive completed for table",
		zap.String("table", tableName),
		zap.Int("processed", result.Processed),
		zap.Int("archived", result.Archived),
		zap.Int64("last_id", lastID))

	return result, nil
}

// MockArchiveDataProvider 模拟归档数据提供者 (用于测试)
type MockArchiveDataProvider struct{}

func (p *MockArchiveDataProvider) GetArchivableRecords(ctx context.Context, tableName string, beforeTime int64, lastID int64, limit int) ([]int64, error) {
	// TODO: 实际实现通过直接查询数据库
	return nil, nil
}

func (p *MockArchiveDataProvider) ArchiveRecords(ctx context.Context, tableName string, ids []int64) error {
	// TODO: 实际实现：移动到归档表或删除
	return nil
}

func (p *MockArchiveDataProvider) GetMaxID(ctx context.Context, tableName string, beforeTime int64) (int64, error) {
	// TODO: 实际实现通过直接查询数据库
	return 0, nil
}
