package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
)

var (
	ErrDepositRecordNotFound = errors.New("deposit record not found")
	ErrDuplicateDeposit      = errors.New("duplicate deposit")
)

// DepositRepository 充值记录仓储接口
type DepositRepository interface {
	Create(ctx context.Context, record *model.DepositRecord) error
	GetByDepositID(ctx context.Context, depositID string) (*model.DepositRecord, error)
	GetByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.DepositRecord, error)
	Update(ctx context.Context, record *model.DepositRecord) error
	UpdateStatus(ctx context.Context, depositID string, status model.DepositRecordStatus) error
	UpdateConfirmations(ctx context.Context, depositID string, confirmations int) error

	ListPendingConfirmation(ctx context.Context, limit int) ([]*model.DepositRecord, error)
	ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.DepositRecord, error)
	ListByStatus(ctx context.Context, status model.DepositRecordStatus, page *Pagination) ([]*model.DepositRecord, error)
	ListByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.DepositRecord, error)

	// 幂等检查
	ExistsByTxHash(ctx context.Context, txHash string) (bool, error)
}

// depositRepository 充值记录仓储实现
type depositRepository struct {
	*Repository
}

// NewDepositRepository 创建充值记录仓储
func NewDepositRepository(db *gorm.DB) DepositRepository {
	return &depositRepository{
		Repository: NewRepository(db),
	}
}

func (r *depositRepository) Create(ctx context.Context, record *model.DepositRecord) error {
	now := time.Now().UnixMilli()
	record.CreatedAt = now
	record.UpdatedAt = now

	err := r.DB(ctx).Create(record).Error
	if err != nil && isDuplicateKeyError(err) {
		return ErrDuplicateDeposit
	}
	return err
}

func (r *depositRepository) GetByDepositID(ctx context.Context, depositID string) (*model.DepositRecord, error) {
	var record model.DepositRecord
	err := r.DB(ctx).Where("deposit_id = ?", depositID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDepositRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *depositRepository) GetByTxHashAndLogIndex(ctx context.Context, txHash string, logIndex int) (*model.DepositRecord, error) {
	var record model.DepositRecord
	err := r.DB(ctx).
		Where("tx_hash = ? AND log_index = ?", txHash, logIndex).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDepositRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *depositRepository) Update(ctx context.Context, record *model.DepositRecord) error {
	record.UpdatedAt = time.Now().UnixMilli()
	return r.DB(ctx).Save(record).Error
}

func (r *depositRepository) UpdateStatus(ctx context.Context, depositID string, status model.DepositRecordStatus) error {
	now := time.Now().UnixMilli()
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}
	if status == model.DepositRecordStatusCredited {
		updates["credited_at"] = now
	}

	result := r.DB(ctx).Model(&model.DepositRecord{}).
		Where("deposit_id = ?", depositID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDepositRecordNotFound
	}
	return nil
}

func (r *depositRepository) UpdateConfirmations(ctx context.Context, depositID string, confirmations int) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.DepositRecord{}).
		Where("deposit_id = ?", depositID).
		Updates(map[string]interface{}{
			"confirmations": confirmations,
			"updated_at":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDepositRecordNotFound
	}
	return nil
}

func (r *depositRepository) ListPendingConfirmation(ctx context.Context, limit int) ([]*model.DepositRecord, error) {
	var records []*model.DepositRecord
	err := r.DB(ctx).
		Where("status < ?", model.DepositRecordStatusCredited).
		Order("block_number ASC, log_index ASC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func (r *depositRepository) ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.DepositRecord, error) {
	var records []*model.DepositRecord

	query := r.DB(ctx).Model(&model.DepositRecord{}).Where("wallet_address = ?", wallet)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *depositRepository) ListByStatus(ctx context.Context, status model.DepositRecordStatus, page *Pagination) ([]*model.DepositRecord, error) {
	var records []*model.DepositRecord

	query := r.DB(ctx).Model(&model.DepositRecord{}).Where("status = ?", status)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&records).Error
	return records, err
}

func (r *depositRepository) ListByBlockRange(ctx context.Context, chainID int64, startBlock, endBlock int64) ([]*model.DepositRecord, error) {
	var records []*model.DepositRecord
	err := r.DB(ctx).
		Where("chain_id = ? AND block_number >= ? AND block_number <= ?", chainID, startBlock, endBlock).
		Order("block_number ASC, log_index ASC").
		Find(&records).Error
	return records, err
}

func (r *depositRepository) ExistsByTxHash(ctx context.Context, txHash string) (bool, error) {
	var count int64
	err := r.DB(ctx).Model(&model.DepositRecord{}).
		Where("tx_hash = ?", txHash).
		Count(&count).Error
	return count > 0, err
}

// isDuplicateKeyError 判断是否为重复键错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL duplicate key error code: 23505
	errStr := err.Error()
	return contains(errStr, "duplicate key") || contains(errStr, "23505")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
