package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"gorm.io/gorm"
)

// PartitionManageJob 分区管理任务
type PartitionManageJob struct {
	scheduler.BaseJob
	db     *gorm.DB
	tables []string
}

// NewPartitionManageJob 创建分区管理任务
func NewPartitionManageJob(db *gorm.DB, tables []string) *PartitionManageJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNamePartitionManage]

	return &PartitionManageJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNamePartitionManage,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		db:     db,
		tables: tables,
	}
}

// Execute 执行分区管理
func (j *PartitionManageJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	now := time.Now()

	// 计算需要创建的分区 (当月 + 下月)
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonth := currentMonth.AddDate(0, 1, 0)

	tableResults := make(map[string]interface{})

	for _, tableName := range j.tables {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		tableResult := map[string]interface{}{}

		// 创建当月分区
		created, err := j.createMonthlyPartition(ctx, tableName, currentMonth.Year(), int(currentMonth.Month()))
		if err != nil {
			logger.Error("failed to create current month partition",
				"table", tableName,
				"error", err)
			tableResult["current_month_error"] = err.Error()
			result.ErrorCount++
		} else if created {
			tableResult["current_month"] = fmt.Sprintf("%d_%02d", currentMonth.Year(), currentMonth.Month())
			result.AffectedCount++
		}

		// 创建下月分区
		created, err = j.createMonthlyPartition(ctx, tableName, nextMonth.Year(), int(nextMonth.Month()))
		if err != nil {
			logger.Error("failed to create next month partition",
				"table", tableName,
				"error", err)
			tableResult["next_month_error"] = err.Error()
			result.ErrorCount++
		} else if created {
			tableResult["next_month"] = fmt.Sprintf("%d_%02d", nextMonth.Year(), nextMonth.Month())
			result.AffectedCount++
		}

		tableResults[tableName] = tableResult
		result.ProcessedCount++
	}

	result.Details["tables"] = tableResults

	logger.Info("partition management completed",
		"processed", result.ProcessedCount,
		"created", result.AffectedCount,
		"errors", result.ErrorCount)

	return result, nil
}

// createMonthlyPartition 创建月度分区
func (j *PartitionManageJob) createMonthlyPartition(ctx context.Context, tableName string, year, month int) (bool, error) {
	partitionName := fmt.Sprintf("%s_%d_%02d", tableName, year, month)

	// 检查分区是否已存在
	var exists bool
	checkSQL := `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables
			WHERE schemaname = 'public'
			AND tablename = $1
		)
	`
	if err := j.db.WithContext(ctx).Raw(checkSQL, partitionName).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("failed to check partition existence: %w", err)
	}

	if exists {
		logger.Debug("partition already exists",
			"partition", partitionName)
		return false, nil
	}

	// 计算分区时间范围 (毫秒时间戳)
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	startTs := startDate.UnixMilli()
	endTs := endDate.UnixMilli()

	// 创建分区
	createSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s
		PARTITION OF %s
		FOR VALUES FROM (%d) TO (%d)
	`, partitionName, tableName, startTs, endTs)

	if err := j.db.WithContext(ctx).Exec(createSQL).Error; err != nil {
		return false, fmt.Errorf("failed to create partition: %w", err)
	}

	logger.Info("partition created",
		"partition", partitionName,
		"start_ts", startTs,
		"end_ts", endTs)

	return true, nil
}

// DefaultPartitionTables 默认需要管理分区的表
var DefaultPartitionTables = []string{
	"eidos_market_trade_history",
	"eidos_chain_events",
	"eidos_chain_deposit_records",
	"eidos_risk_audit_logs",
	"eidos_risk_events",
	"eidos_jobs_executions",
}
