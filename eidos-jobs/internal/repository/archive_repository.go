package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
)

// ArchiveRepository 归档进度仓储
type ArchiveRepository struct {
	db *gorm.DB
}

// NewArchiveRepository 创建归档进度仓储
func NewArchiveRepository(db *gorm.DB) *ArchiveRepository {
	return &ArchiveRepository{db: db}
}

// GetProgress 获取归档进度
func (r *ArchiveRepository) GetProgress(ctx context.Context, tableName string) (*model.ArchiveProgress, error) {
	var progress model.ArchiveProgress
	err := r.db.WithContext(ctx).
		Where("table_name = ?", tableName).
		First(&progress).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &progress, nil
}

// UpsertProgress 更新或创建归档进度
func (r *ArchiveRepository) UpsertProgress(ctx context.Context, progress *model.ArchiveProgress) error {
	progress.UpdatedAt = time.Now().UnixMilli()

	return r.db.WithContext(ctx).
		Where("table_name = ?", progress.SourceTable).
		Assign(model.ArchiveProgress{
			LastID:    progress.LastID,
			StartedAt: progress.StartedAt,
			UpdatedAt: progress.UpdatedAt,
		}).
		FirstOrCreate(progress).Error
}

// UpdateLastID 更新最后处理的ID
func (r *ArchiveRepository) UpdateLastID(ctx context.Context, tableName string, lastID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.ArchiveProgress{}).
		Where("table_name = ?", tableName).
		Updates(map[string]interface{}{
			"last_id":    lastID,
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// StartArchive 开始归档 (记录开始时间)
func (r *ArchiveRepository) StartArchive(ctx context.Context, tableName string) error {
	now := time.Now().UnixMilli()
	return r.db.WithContext(ctx).
		Where("table_name = ?", tableName).
		Assign(model.ArchiveProgress{
			SourceTable: tableName,
			StartedAt:   &now,
			UpdatedAt:   now,
		}).
		FirstOrCreate(&model.ArchiveProgress{}).Error
}

// FinishArchive 完成归档 (清除开始时间)
func (r *ArchiveRepository) FinishArchive(ctx context.Context, tableName string, lastID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.ArchiveProgress{}).
		Where("table_name = ?", tableName).
		Updates(map[string]interface{}{
			"last_id":    lastID,
			"started_at": nil,
			"updated_at": time.Now().UnixMilli(),
		}).Error
}

// ListAllProgress 列出所有归档进度
func (r *ArchiveRepository) ListAllProgress(ctx context.Context) ([]*model.ArchiveProgress, error) {
	var progresses []*model.ArchiveProgress
	err := r.db.WithContext(ctx).Find(&progresses).Error
	return progresses, err
}

// ListInProgress 列出正在进行的归档任务
func (r *ArchiveRepository) ListInProgress(ctx context.Context) ([]*model.ArchiveProgress, error) {
	var progresses []*model.ArchiveProgress
	err := r.db.WithContext(ctx).
		Where("started_at IS NOT NULL").
		Find(&progresses).Error
	return progresses, err
}

// Delete 删除归档进度记录
func (r *ArchiveRepository) Delete(ctx context.Context, tableName string) error {
	return r.db.WithContext(ctx).
		Where("table_name = ?", tableName).
		Delete(&model.ArchiveProgress{}).Error
}
