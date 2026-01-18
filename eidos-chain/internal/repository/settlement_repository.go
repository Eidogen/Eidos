package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
)

var (
	ErrSettlementBatchNotFound = errors.New("settlement batch not found")
	ErrRollbackLogNotFound     = errors.New("rollback log not found")
)

// SettlementRepository 结算批次仓储接口
type SettlementRepository interface {
	// 批次操作
	Create(ctx context.Context, batch *model.SettlementBatch) error
	GetByBatchID(ctx context.Context, batchID string) (*model.SettlementBatch, error)
	GetByID(ctx context.Context, id int64) (*model.SettlementBatch, error)
	Update(ctx context.Context, batch *model.SettlementBatch) error
	UpdateStatus(ctx context.Context, batchID string, status model.SettlementBatchStatus, txHash string, blockNumber int64) error
	UpdateFailed(ctx context.Context, batchID string, errMsg string) error

	// 查询
	ListPending(ctx context.Context, limit int) ([]*model.SettlementBatch, error)
	ListByStatus(ctx context.Context, status model.SettlementBatchStatus, page *Pagination) ([]*model.SettlementBatch, error)
	ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.SettlementBatch, error)

	// 回滚
	CreateRollbackLog(ctx context.Context, log *model.SettlementRollbackLog) error
	GetRollbackLogByBatchID(ctx context.Context, batchID string) (*model.SettlementRollbackLog, error)
	ListRollbackLogs(ctx context.Context, page *Pagination) ([]*model.SettlementRollbackLog, error)
}

// settlementRepository 结算批次仓储实现
type settlementRepository struct {
	*Repository
}

// NewSettlementRepository 创建结算批次仓储
func NewSettlementRepository(db *gorm.DB) SettlementRepository {
	return &settlementRepository{
		Repository: NewRepository(db),
	}
}

func (r *settlementRepository) Create(ctx context.Context, batch *model.SettlementBatch) error {
	now := time.Now().UnixMilli()
	batch.CreatedAt = now
	batch.UpdatedAt = now
	return r.DB(ctx).Create(batch).Error
}

func (r *settlementRepository) GetByBatchID(ctx context.Context, batchID string) (*model.SettlementBatch, error) {
	var batch model.SettlementBatch
	err := r.DB(ctx).Where("batch_id = ?", batchID).First(&batch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSettlementBatchNotFound
	}
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *settlementRepository) GetByID(ctx context.Context, id int64) (*model.SettlementBatch, error) {
	var batch model.SettlementBatch
	err := r.DB(ctx).Where("id = ?", id).First(&batch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSettlementBatchNotFound
	}
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *settlementRepository) Update(ctx context.Context, batch *model.SettlementBatch) error {
	batch.UpdatedAt = time.Now().UnixMilli()
	return r.DB(ctx).Save(batch).Error
}

func (r *settlementRepository) UpdateStatus(ctx context.Context, batchID string, status model.SettlementBatchStatus, txHash string, blockNumber int64) error {
	now := time.Now().UnixMilli()
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}
	if txHash != "" {
		updates["tx_hash"] = txHash
	}
	if blockNumber > 0 {
		updates["block_number"] = blockNumber
	}
	if status == model.SettlementBatchStatusSubmitted {
		updates["submitted_at"] = now
	}
	if status == model.SettlementBatchStatusConfirmed {
		updates["confirmed_at"] = now
	}

	result := r.DB(ctx).Model(&model.SettlementBatch{}).
		Where("batch_id = ?", batchID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSettlementBatchNotFound
	}
	return nil
}

func (r *settlementRepository) UpdateFailed(ctx context.Context, batchID string, errMsg string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.SettlementBatch{}).
		Where("batch_id = ?", batchID).
		Updates(map[string]interface{}{
			"status":        model.SettlementBatchStatusFailed,
			"error_message": errMsg,
			"retry_count":   gorm.Expr("retry_count + 1"),
			"updated_at":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSettlementBatchNotFound
	}
	return nil
}

func (r *settlementRepository) ListPending(ctx context.Context, limit int) ([]*model.SettlementBatch, error) {
	var batches []*model.SettlementBatch
	err := r.DB(ctx).
		Where("status = ?", model.SettlementBatchStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&batches).Error
	return batches, err
}

func (r *settlementRepository) ListByStatus(ctx context.Context, status model.SettlementBatchStatus, page *Pagination) ([]*model.SettlementBatch, error) {
	var batches []*model.SettlementBatch

	query := r.DB(ctx).Model(&model.SettlementBatch{}).Where("status = ?", status)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&batches).Error
	return batches, err
}

func (r *settlementRepository) ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.SettlementBatch, error) {
	var batches []*model.SettlementBatch

	query := r.DB(ctx).Model(&model.SettlementBatch{}).
		Where("created_at >= ? AND created_at < ?", start, end)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&batches).Error
	return batches, err
}

func (r *settlementRepository) CreateRollbackLog(ctx context.Context, log *model.SettlementRollbackLog) error {
	log.CreatedAt = time.Now().UnixMilli()
	return r.DB(ctx).Create(log).Error
}

func (r *settlementRepository) GetRollbackLogByBatchID(ctx context.Context, batchID string) (*model.SettlementRollbackLog, error) {
	var log model.SettlementRollbackLog
	err := r.DB(ctx).Where("batch_id = ?", batchID).First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRollbackLogNotFound
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *settlementRepository) ListRollbackLogs(ctx context.Context, page *Pagination) ([]*model.SettlementRollbackLog, error) {
	var logs []*model.SettlementRollbackLog

	query := r.DB(ctx).Model(&model.SettlementRollbackLog{})

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("rollback_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&logs).Error
	return logs, err
}
