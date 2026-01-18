package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
)

var (
	ErrReconciliationNotFound = errors.New("reconciliation record not found")
)

// ReconciliationRepository 对账记录仓储接口
type ReconciliationRepository interface {
	Create(ctx context.Context, record *model.ReconciliationRecord) error
	GetByID(ctx context.Context, id int64) (*model.ReconciliationRecord, error)
	UpdateResolution(ctx context.Context, id int64, resolution string, resolvedBy string) error

	ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.ReconciliationRecord, error)
	ListByStatus(ctx context.Context, status model.ReconciliationStatus, page *Pagination) ([]*model.ReconciliationRecord, error)
	ListDiscrepancies(ctx context.Context, page *Pagination) ([]*model.ReconciliationRecord, error)
	ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.ReconciliationRecord, error)

	// 统计
	CountDiscrepancies(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context, status model.ReconciliationStatus) (int64, error)
}

// reconciliationRepository 对账记录仓储实现
type reconciliationRepository struct {
	*Repository
}

// NewReconciliationRepository 创建对账记录仓储
func NewReconciliationRepository(db *gorm.DB) ReconciliationRepository {
	return &reconciliationRepository{
		Repository: NewRepository(db),
	}
}

func (r *reconciliationRepository) Create(ctx context.Context, record *model.ReconciliationRecord) error {
	record.CreatedAt = time.Now().UnixMilli()
	return r.DB(ctx).Create(record).Error
}

func (r *reconciliationRepository) GetByID(ctx context.Context, id int64) (*model.ReconciliationRecord, error) {
	var record model.ReconciliationRecord
	err := r.DB(ctx).Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReconciliationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *reconciliationRepository) UpdateResolution(ctx context.Context, id int64, resolution string, resolvedBy string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.ReconciliationRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      model.ReconciliationStatusResolved,
			"resolution":  resolution,
			"resolved_by": resolvedBy,
			"resolved_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrReconciliationNotFound
	}
	return nil
}

func (r *reconciliationRepository) ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord

	query := r.DB(ctx).Model(&model.ReconciliationRecord{}).Where("wallet_address = ?", wallet)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("checked_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *reconciliationRepository) ListByStatus(ctx context.Context, status model.ReconciliationStatus, page *Pagination) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord

	query := r.DB(ctx).Model(&model.ReconciliationRecord{}).Where("status = ?", status)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("checked_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *reconciliationRepository) ListDiscrepancies(ctx context.Context, page *Pagination) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord

	query := r.DB(ctx).Model(&model.ReconciliationRecord{}).
		Where("status = ?", model.ReconciliationStatusDiscrepancy)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("checked_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *reconciliationRepository) ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.ReconciliationRecord, error) {
	var records []*model.ReconciliationRecord

	query := r.DB(ctx).Model(&model.ReconciliationRecord{}).
		Where("checked_at >= ? AND checked_at < ?", start, end)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("checked_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *reconciliationRepository) CountDiscrepancies(ctx context.Context) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&model.ReconciliationRecord{}).
		Where("status = ?", model.ReconciliationStatusDiscrepancy).
		Count(&count).Error
	return count, err
}

func (r *reconciliationRepository) CountByStatus(ctx context.Context, status model.ReconciliationStatus) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&model.ReconciliationRecord{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}
