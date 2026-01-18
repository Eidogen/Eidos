package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// DataCleanupConfig 数据清理配置
type DataCleanupConfig struct {
	// PartitionedTables 分区表配置
	PartitionedTables map[string]PartitionTableConfig
	// RegularTables 普通表配置
	RegularTables map[string]RegularTableConfig
	// TimescaleDBTables TimescaleDB 表配置
	TimescaleDBTables map[string]TimescaleTableConfig
}

// PartitionTableConfig 分区表配置
type PartitionTableConfig struct {
	RetentionMonths    int
	ArchiveBeforeDrop  bool
	ArchiveFormat      string // csv/parquet
}

// RegularTableConfig 普通表配置
type RegularTableConfig struct {
	RetentionMonths int
	BatchSize       int
	SleepMs         int
}

// TimescaleTableConfig TimescaleDB 表配置
type TimescaleTableConfig struct {
	RetentionByInterval map[string]int // interval -> retention days
}

// DataCleanupJob 数据清理任务
type DataCleanupJob struct {
	scheduler.BaseJob
	db     *gorm.DB
	config *DataCleanupConfig
}

// NewDataCleanupJob 创建数据清理任务
func NewDataCleanupJob(db *gorm.DB, config *DataCleanupConfig) *DataCleanupJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameDataCleanup]

	return &DataCleanupJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameDataCleanup,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		db:     db,
		config: config,
	}
}

// Execute 执行数据清理
func (j *DataCleanupJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	// 1. 清理分区表
	partitionResults := make(map[string]interface{})
	for tableName, config := range j.config.PartitionedTables {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		count, err := j.cleanupPartitionedTable(ctx, tableName, config)
		if err != nil {
			logger.Error("failed to cleanup partitioned table",
				zap.String("table", tableName),
				zap.Error(err))
			result.ErrorCount++
			partitionResults[tableName] = map[string]interface{}{"error": err.Error()}
		} else {
			result.AffectedCount += count
			partitionResults[tableName] = map[string]interface{}{"dropped_partitions": count}
		}
		result.ProcessedCount++
	}
	result.Details["partitioned_tables"] = partitionResults

	// 2. 清理普通表
	regularResults := make(map[string]interface{})
	for tableName, config := range j.config.RegularTables {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		count, err := j.cleanupRegularTable(ctx, tableName, config)
		if err != nil {
			logger.Error("failed to cleanup regular table",
				zap.String("table", tableName),
				zap.Error(err))
			result.ErrorCount++
			regularResults[tableName] = map[string]interface{}{"error": err.Error()}
		} else {
			result.AffectedCount += int(count)
			regularResults[tableName] = map[string]interface{}{"deleted_rows": count}
		}
		result.ProcessedCount++
	}
	result.Details["regular_tables"] = regularResults

	// 3. 清理 TimescaleDB 表
	timescaleResults := make(map[string]interface{})
	for tableName, config := range j.config.TimescaleDBTables {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		count, err := j.cleanupTimescaleTable(ctx, tableName, config)
		if err != nil {
			logger.Error("failed to cleanup timescale table",
				zap.String("table", tableName),
				zap.Error(err))
			result.ErrorCount++
			timescaleResults[tableName] = map[string]interface{}{"error": err.Error()}
		} else {
			result.AffectedCount += count
			timescaleResults[tableName] = map[string]interface{}{"dropped_chunks": count}
		}
		result.ProcessedCount++
	}
	result.Details["timescale_tables"] = timescaleResults

	logger.Info("data cleanup completed",
		zap.Int("processed", result.ProcessedCount),
		zap.Int("affected", result.AffectedCount),
		zap.Int("errors", result.ErrorCount))

	return result, nil
}

// cleanupPartitionedTable 清理分区表
func (j *DataCleanupJob) cleanupPartitionedTable(ctx context.Context, tableName string, config PartitionTableConfig) (int, error) {
	// 计算要删除的分区时间
	cutoffDate := time.Now().AddDate(0, -config.RetentionMonths, 0)
	cutoffYear := cutoffDate.Year()
	cutoffMonth := int(cutoffDate.Month())

	droppedCount := 0

	// 查询需要删除的分区
	var partitions []string
	query := `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename LIKE $1
		ORDER BY tablename
	`
	if err := j.db.WithContext(ctx).Raw(query, tableName+"_%").Scan(&partitions).Error; err != nil {
		return 0, fmt.Errorf("failed to list partitions: %w", err)
	}

	for _, partition := range partitions {
		// 解析分区名称中的年月 (格式: tablename_YYYY_MM)
		var year, month int
		n, err := fmt.Sscanf(partition, tableName+"_%d_%d", &year, &month)
		if err != nil || n != 2 {
			continue
		}

		// 检查分区是否过期
		if year < cutoffYear || (year == cutoffYear && month < cutoffMonth) {
			logger.Info("dropping partition",
				zap.String("partition", partition),
				zap.Int("year", year),
				zap.Int("month", month))

			// TODO: 如果配置了 archive_before_drop，先导出数据

			// 删除分区
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", partition)
			if err := j.db.WithContext(ctx).Exec(dropSQL).Error; err != nil {
				logger.Error("failed to drop partition",
					zap.String("partition", partition),
					zap.Error(err))
				continue
			}

			droppedCount++
		}
	}

	return droppedCount, nil
}

// cleanupRegularTable 清理普通表
func (j *DataCleanupJob) cleanupRegularTable(ctx context.Context, tableName string, config RegularTableConfig) (int64, error) {
	// 计算清理截止时间
	cutoffTime := time.Now().AddDate(0, -config.RetentionMonths, 0).UnixMilli()

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	sleepMs := config.SleepMs
	if sleepMs <= 0 {
		sleepMs = 100
	}

	totalDeleted := int64(0)

	// 分批删除
	for {
		select {
		case <-ctx.Done():
			return totalDeleted, ctx.Err()
		default:
		}

		// 执行删除
		deleteSQL := fmt.Sprintf(
			"DELETE FROM %s WHERE created_at < $1 AND id IN (SELECT id FROM %s WHERE created_at < $1 LIMIT $2)",
			tableName, tableName,
		)
		result := j.db.WithContext(ctx).Exec(deleteSQL, cutoffTime, batchSize)
		if result.Error != nil {
			return totalDeleted, fmt.Errorf("failed to delete records: %w", result.Error)
		}

		totalDeleted += result.RowsAffected

		if result.RowsAffected == 0 || result.RowsAffected < int64(batchSize) {
			break
		}

		// 批次间休眠
		if sleepMs > 0 {
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
		}
	}

	return totalDeleted, nil
}

// cleanupTimescaleTable 清理 TimescaleDB 表
func (j *DataCleanupJob) cleanupTimescaleTable(ctx context.Context, tableName string, config TimescaleTableConfig) (int, error) {
	droppedCount := 0

	for interval, retentionDays := range config.RetentionByInterval {
		if retentionDays <= 0 {
			// 0 表示永久保留
			continue
		}

		// 使用 drop_chunks 函数删除过期 chunk
		cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

		dropSQL := `
			SELECT drop_chunks($1, older_than => $2, cascade_to_materializations => FALSE)
		`

		var droppedChunks int
		err := j.db.WithContext(ctx).
			Raw(dropSQL, tableName, cutoffTime).
			Where("interval = ?", interval).
			Scan(&droppedChunks).Error

		if err != nil {
			logger.Warn("failed to drop chunks",
				zap.String("table", tableName),
				zap.String("interval", interval),
				zap.Error(err))
			continue
		}

		droppedCount += droppedChunks
	}

	return droppedCount, nil
}

// DefaultDataCleanupConfig 默认数据清理配置
var DefaultDataCleanupConfig = &DataCleanupConfig{
	PartitionedTables: map[string]PartitionTableConfig{
		"eidos_market_trade_history": {RetentionMonths: 12, ArchiveBeforeDrop: true},
		"eidos_chain_events":         {RetentionMonths: 6, ArchiveBeforeDrop: false},
		"eidos_chain_deposit_records": {RetentionMonths: 24, ArchiveBeforeDrop: true},
		"eidos_risk_audit_logs":      {RetentionMonths: 6, ArchiveBeforeDrop: false},
		"eidos_risk_events":          {RetentionMonths: 12, ArchiveBeforeDrop: true},
		"eidos_jobs_executions":      {RetentionMonths: 3, ArchiveBeforeDrop: false},
	},
	RegularTables: map[string]RegularTableConfig{
		"eidos_chain_withdrawal_txs":       {RetentionMonths: 24, BatchSize: 1000, SleepMs: 100},
		"eidos_chain_settlement_batches":   {RetentionMonths: 24, BatchSize: 500, SleepMs: 100},
		"eidos_risk_withdrawal_reviews":    {RetentionMonths: 24, BatchSize: 1000, SleepMs: 100},
		"eidos_jobs_reconciliation_records": {RetentionMonths: 6, BatchSize: 1000, SleepMs: 100},
		"eidos_admin_audit_logs":           {RetentionMonths: 24, BatchSize: 1000, SleepMs: 100},
	},
	TimescaleDBTables: map[string]TimescaleTableConfig{
		"eidos_market_klines": {
			RetentionByInterval: map[string]int{
				"1m":  7,
				"5m":  30,
				"15m": 90,
				"30m": 180,
				"1h":  365,
				"4h":  730,
				"1d":  0, // 永久保留
				"1w":  0, // 永久保留
			},
		},
	},
}
