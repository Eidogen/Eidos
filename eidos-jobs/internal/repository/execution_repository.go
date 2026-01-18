package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
)

// ExecutionRepository 任务执行记录仓储
type ExecutionRepository struct {
	db *gorm.DB
}

// NewExecutionRepository 创建任务执行记录仓储
func NewExecutionRepository(db *gorm.DB) *ExecutionRepository {
	return &ExecutionRepository{db: db}
}

// Create 创建执行记录
func (r *ExecutionRepository) Create(ctx context.Context, exec *model.JobExecution) error {
	now := time.Now().UnixMilli()
	exec.CreatedAt = now
	return r.db.WithContext(ctx).Create(exec).Error
}

// Update 更新执行记录
func (r *ExecutionRepository) Update(ctx context.Context, exec *model.JobExecution) error {
	return r.db.WithContext(ctx).Save(exec).Error
}

// GetByID 根据ID查询执行记录
func (r *ExecutionRepository) GetByID(ctx context.Context, id int64) (*model.JobExecution, error) {
	var exec model.JobExecution
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&exec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &exec, nil
}

// GetLatestByJobName 获取任务最新执行记录
func (r *ExecutionRepository) GetLatestByJobName(ctx context.Context, jobName string) (*model.JobExecution, error) {
	var exec model.JobExecution
	err := r.db.WithContext(ctx).
		Where("job_name = ?", jobName).
		Order("started_at DESC").
		First(&exec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &exec, nil
}

// GetRunningByJobName 获取正在运行的任务
func (r *ExecutionRepository) GetRunningByJobName(ctx context.Context, jobName string) (*model.JobExecution, error) {
	var exec model.JobExecution
	err := r.db.WithContext(ctx).
		Where("job_name = ? AND status = ?", jobName, model.JobStatusRunning).
		First(&exec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &exec, nil
}

// ListByJobName 查询任务执行历史
func (r *ExecutionRepository) ListByJobName(ctx context.Context, jobName string, limit int) ([]*model.JobExecution, error) {
	var execs []*model.JobExecution
	err := r.db.WithContext(ctx).
		Where("job_name = ?", jobName).
		Order("started_at DESC").
		Limit(limit).
		Find(&execs).Error
	return execs, err
}

// ListByStatus 按状态查询执行记录
func (r *ExecutionRepository) ListByStatus(ctx context.Context, status model.JobStatus, limit int) ([]*model.JobExecution, error) {
	var execs []*model.JobExecution
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("started_at DESC").
		Limit(limit).
		Find(&execs).Error
	return execs, err
}

// ListByTimeRange 按时间范围查询执行记录
func (r *ExecutionRepository) ListByTimeRange(ctx context.Context, startTime, endTime int64, limit int) ([]*model.JobExecution, error) {
	var execs []*model.JobExecution
	err := r.db.WithContext(ctx).
		Where("started_at >= ? AND started_at < ?", startTime, endTime).
		Order("started_at DESC").
		Limit(limit).
		Find(&execs).Error
	return execs, err
}

// CountByJobNameAndStatus 统计任务执行次数
func (r *ExecutionRepository) CountByJobNameAndStatus(ctx context.Context, jobName string, status model.JobStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.JobExecution{}).
		Where("job_name = ? AND status = ?", jobName, status).
		Count(&count).Error
	return count, err
}

// GetLastSuccessTime 获取任务上次成功执行时间
func (r *ExecutionRepository) GetLastSuccessTime(ctx context.Context, jobName string) (int64, error) {
	var exec model.JobExecution
	err := r.db.WithContext(ctx).
		Where("job_name = ? AND status = ?", jobName, model.JobStatusSuccess).
		Order("finished_at DESC").
		First(&exec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	if exec.FinishedAt != nil {
		return *exec.FinishedAt, nil
	}
	return exec.StartedAt, nil
}

// CleanupOldRecords 清理旧的执行记录
func (r *ExecutionRepository) CleanupOldRecords(ctx context.Context, beforeTime int64, batchSize int) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("started_at < ?", beforeTime).
		Limit(batchSize).
		Delete(&model.JobExecution{})
	return result.RowsAffected, result.Error
}

// MarkStaleRunningAsFailed 标记卡住的任务为失败
func (r *ExecutionRepository) MarkStaleRunningAsFailed(ctx context.Context, threshold time.Duration) (int64, error) {
	staleTime := time.Now().Add(-threshold).UnixMilli()
	errorMsg := "task timed out (marked as failed by cleanup job)"

	result := r.db.WithContext(ctx).
		Model(&model.JobExecution{}).
		Where("status = ? AND started_at < ?", model.JobStatusRunning, staleTime).
		Updates(map[string]interface{}{
			"status":        model.JobStatusFailed,
			"finished_at":   time.Now().UnixMilli(),
			"error_message": errorMsg,
		})
	return result.RowsAffected, result.Error
}
