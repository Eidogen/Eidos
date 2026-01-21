package jobs

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
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
				"table", tableName,
				"error", err)
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
		"processed", result.ProcessedCount,
		"archived", result.AffectedCount,
		"errors", result.ErrorCount)

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
		logger.Warn("failed to mark archive start", "error", err)
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
		"table", tableName,
		"last_id", lastID,
		"before_time", beforeTime,
		"retention_days", retentionDays)

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
					"table", tableName,
					"error", err)
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
		logger.Warn("failed to mark archive finish", "error", err)
	}

	result.LastID = lastID

	logger.Info("archive completed for table",
		"table", tableName,
		"processed", result.Processed,
		"archived", result.Archived,
		"last_id", lastID)

	return result, nil
}

// MockArchiveDataProvider 模拟归档数据提供者 (用于测试)
type MockArchiveDataProvider struct{}

func (p *MockArchiveDataProvider) GetArchivableRecords(ctx context.Context, tableName string, beforeTime int64, lastID int64, limit int) ([]int64, error) {
	return nil, nil
}

func (p *MockArchiveDataProvider) ArchiveRecords(ctx context.Context, tableName string, ids []int64) error {
	return nil
}

func (p *MockArchiveDataProvider) GetMaxID(ctx context.Context, tableName string, beforeTime int64) (int64, error) {
	return 0, nil
}

// DBArchiveDataProvider 数据库归档数据提供者
type DBArchiveDataProvider struct {
	db *gorm.DB
}

// NewDBArchiveDataProvider 创建数据库归档数据提供者
func NewDBArchiveDataProvider(db *gorm.DB) *DBArchiveDataProvider {
	return &DBArchiveDataProvider{db: db}
}

// GetArchivableRecords 获取可归档的记录 ID
// 查询指定表中 created_at < beforeTime 且 id > lastID 的记录
func (p *DBArchiveDataProvider) GetArchivableRecords(ctx context.Context, tableName string, beforeTime int64, lastID int64, limit int) ([]int64, error) {
	var ids []int64

	// 使用原始 SQL 查询，支持动态表名
	// 注意: tableName 已在调用方验证过，这里是安全的
	query := fmt.Sprintf(
		"SELECT id FROM %s WHERE created_at < ? AND id > ? ORDER BY id ASC LIMIT ?",
		tableName,
	)

	rows, err := p.db.WithContext(ctx).Raw(query, beforeTime, lastID, limit).Rows()
	if err != nil {
		return nil, fmt.Errorf("query archivable records failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan record id failed: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows failed: %w", err)
	}

	return ids, nil
}

// ArchiveRecords 归档记录
// 将记录移动到归档表 (tableName_archive) 并从原表删除
func (p *DBArchiveDataProvider) ArchiveRecords(ctx context.Context, tableName string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	archiveTableName := tableName + "_archive"

	// 使用事务确保原子性
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 将记录复制到归档表
		insertQuery := fmt.Sprintf(
			"INSERT INTO %s SELECT * FROM %s WHERE id IN ?",
			archiveTableName, tableName,
		)
		if err := tx.Exec(insertQuery, ids).Error; err != nil {
			return fmt.Errorf("copy to archive table failed: %w", err)
		}

		// 2. 从原表删除记录
		deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE id IN ?", tableName)
		if err := tx.Exec(deleteQuery, ids).Error; err != nil {
			return fmt.Errorf("delete from source table failed: %w", err)
		}

		return nil
	})
}

// GetMaxID 获取表中满足条件的最大 ID
func (p *DBArchiveDataProvider) GetMaxID(ctx context.Context, tableName string, beforeTime int64) (int64, error) {
	var maxID *int64

	query := fmt.Sprintf("SELECT MAX(id) FROM %s WHERE created_at < ?", tableName)

	if err := p.db.WithContext(ctx).Raw(query, beforeTime).Scan(&maxID).Error; err != nil {
		return 0, fmt.Errorf("query max id failed: %w", err)
	}

	if maxID == nil {
		return 0, nil
	}

	return *maxID, nil
}
