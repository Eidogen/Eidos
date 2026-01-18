package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
)

// ReconciliationRepository 对账记录仓储
type ReconciliationRepository struct {
	db *gorm.DB
}

// NewReconciliationRepository 创建对账记录仓储
func NewReconciliationRepository(db *gorm.DB) *ReconciliationRepository {
	return &ReconciliationRepository{db: db}
}

// Create 创建对账记录
func (r *ReconciliationRepository) Create(ctx context.Context, record *model.ReconciliationRecord) error {
	record.CreatedAt = time.Now().UnixMilli()
	return r.db.WithContext(ctx).Create(record).Error
}

// BatchCreate 批量创建对账记录
func (r *ReconciliationRepository) BatchCreate(ctx context.Context, records []*model.ReconciliationRecord) error {
	if len(records) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	for _, rec := range records {
		rec.CreatedAt = now
	}

	return r.db.WithContext(ctx).CreateInBatches(records, 100).Error
}

// GetByID 根据ID查询对账记录
func (r *ReconciliationRepository) GetByID(ctx context.Context, id int64) (*model.ReconciliationRecord, error) {
	var record model.ReconciliationRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ListByJobType 根据任务类型查询对账记录
func (r *ReconciliationRepository) ListByJobType(ctx context.Context, jobType string, limit int) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord
	err := r.db.WithContext(ctx).
		Where("job_type = ?", jobType).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// ListMismatched 查询不匹配的对账记录
func (r *ReconciliationRepository) ListMismatched(ctx context.Context, limit int) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord
	err := r.db.WithContext(ctx).
		Where("status = ?", model.ReconciliationStatusMismatched).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// ListByTimeRange 根据时间范围查询对账记录
func (r *ReconciliationRepository) ListByTimeRange(ctx context.Context, startTime, endTime int64, limit int) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord
	err := r.db.WithContext(ctx).
		Where("created_at >= ? AND created_at < ?", startTime, endTime).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// ListByWallet 根据钱包地址查询对账记录
func (r *ReconciliationRepository) ListByWallet(ctx context.Context, walletAddress string, limit int) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// CountMismatchedByJobType 统计不匹配的对账记录数量
func (r *ReconciliationRepository) CountMismatchedByJobType(ctx context.Context, jobType string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ReconciliationRecord{}).
		Where("job_type = ? AND status = ?", jobType, model.ReconciliationStatusMismatched).
		Count(&count).Error
	return count, err
}

// SumDiffByJobType 汇总差异金额
func (r *ReconciliationRepository) SumDiffByJobType(ctx context.Context, jobType string) (string, error) {
	var sum string
	err := r.db.WithContext(ctx).
		Model(&model.ReconciliationRecord{}).
		Select("COALESCE(SUM(ABS(diff_value::numeric)), 0)::text").
		Where("job_type = ? AND status = ?", jobType, model.ReconciliationStatusMismatched).
		Scan(&sum).Error
	return sum, err
}

// CleanupOldRecords 清理旧的对账记录
func (r *ReconciliationRepository) CleanupOldRecords(ctx context.Context, beforeTime int64, batchSize int) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("created_at < ?", beforeTime).
		Limit(batchSize).
		Delete(&model.ReconciliationRecord{})
	return result.RowsAffected, result.Error
}

// ============ 检查点操作 ============

// GetCheckpoint 获取检查点
func (r *ReconciliationRepository) GetCheckpoint(ctx context.Context, jobType string) (*model.ReconciliationCheckpoint, error) {
	var checkpoint model.ReconciliationCheckpoint
	err := r.db.WithContext(ctx).
		Where("job_type = ?", jobType).
		First(&checkpoint).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &checkpoint, nil
}

// UpsertCheckpoint 更新或创建检查点
func (r *ReconciliationRepository) UpsertCheckpoint(ctx context.Context, checkpoint *model.ReconciliationCheckpoint) error {
	checkpoint.UpdatedAt = time.Now().UnixMilli()

	return r.db.WithContext(ctx).
		Where("job_type = ?", checkpoint.JobType).
		Assign(model.ReconciliationCheckpoint{
			LastTime:  checkpoint.LastTime,
			LastBlock: checkpoint.LastBlock,
			UpdatedAt: checkpoint.UpdatedAt,
		}).
		FirstOrCreate(checkpoint).Error
}

// UpdateCheckpointTime 更新检查点时间
func (r *ReconciliationRepository) UpdateCheckpointTime(ctx context.Context, jobType string, lastTime int64) error {
	return r.db.WithContext(ctx).
		Model(&model.ReconciliationCheckpoint{}).
		Where("job_type = ?", jobType).
		Updates(map[string]interface{}{
			"last_time":  lastTime,
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// UpdateCheckpointBlock 更新检查点区块号
func (r *ReconciliationRepository) UpdateCheckpointBlock(ctx context.Context, jobType string, lastBlock int64) error {
	return r.db.WithContext(ctx).
		Model(&model.ReconciliationCheckpoint{}).
		Where("job_type = ?", jobType).
		Updates(map[string]interface{}{
			"last_block": lastBlock,
			"updated_at": time.Now().UnixMilli(),
		}).Error
}
