package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
)

var (
	ErrWithdrawalTxNotFound = errors.New("withdrawal tx not found")
)

// WithdrawalRepository 提现交易仓储接口
type WithdrawalRepository interface {
	Create(ctx context.Context, tx *model.WithdrawalTx) error
	GetByWithdrawID(ctx context.Context, withdrawID string) (*model.WithdrawalTx, error)
	GetByID(ctx context.Context, id int64) (*model.WithdrawalTx, error)
	Update(ctx context.Context, tx *model.WithdrawalTx) error
	UpdateStatus(ctx context.Context, withdrawID string, status model.WithdrawalTxStatus, txHash string, blockNumber int64) error
	UpdateFailed(ctx context.Context, withdrawID string, errMsg string) error

	ListPending(ctx context.Context, limit int) ([]*model.WithdrawalTx, error)
	ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.WithdrawalTx, error)
	ListByStatus(ctx context.Context, status model.WithdrawalTxStatus, page *Pagination) ([]*model.WithdrawalTx, error)
	ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.WithdrawalTx, error)
}

// withdrawalRepository 提现交易仓储实现
type withdrawalRepository struct {
	*Repository
}

// NewWithdrawalRepository 创建提现交易仓储
func NewWithdrawalRepository(db *gorm.DB) WithdrawalRepository {
	return &withdrawalRepository{
		Repository: NewRepository(db),
	}
}

func (r *withdrawalRepository) Create(ctx context.Context, tx *model.WithdrawalTx) error {
	now := time.Now().UnixMilli()
	tx.CreatedAt = now
	tx.UpdatedAt = now
	return r.DB(ctx).Create(tx).Error
}

func (r *withdrawalRepository) GetByWithdrawID(ctx context.Context, withdrawID string) (*model.WithdrawalTx, error) {
	var tx model.WithdrawalTx
	err := r.DB(ctx).Where("withdraw_id = ?", withdrawID).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWithdrawalTxNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *withdrawalRepository) GetByID(ctx context.Context, id int64) (*model.WithdrawalTx, error) {
	var tx model.WithdrawalTx
	err := r.DB(ctx).Where("id = ?", id).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWithdrawalTxNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *withdrawalRepository) Update(ctx context.Context, tx *model.WithdrawalTx) error {
	tx.UpdatedAt = time.Now().UnixMilli()
	return r.DB(ctx).Save(tx).Error
}

func (r *withdrawalRepository) UpdateStatus(ctx context.Context, withdrawID string, status model.WithdrawalTxStatus, txHash string, blockNumber int64) error {
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
	if status == model.WithdrawalTxStatusSubmitted {
		updates["submitted_at"] = now
	}
	if status == model.WithdrawalTxStatusConfirmed {
		updates["confirmed_at"] = now
	}

	result := r.DB(ctx).Model(&model.WithdrawalTx{}).
		Where("withdraw_id = ?", withdrawID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWithdrawalTxNotFound
	}
	return nil
}

func (r *withdrawalRepository) UpdateFailed(ctx context.Context, withdrawID string, errMsg string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.WithdrawalTx{}).
		Where("withdraw_id = ?", withdrawID).
		Updates(map[string]interface{}{
			"status":        model.WithdrawalTxStatusFailed,
			"error_message": errMsg,
			"retry_count":   gorm.Expr("retry_count + 1"),
			"updated_at":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWithdrawalTxNotFound
	}
	return nil
}

func (r *withdrawalRepository) ListPending(ctx context.Context, limit int) ([]*model.WithdrawalTx, error) {
	var txs []*model.WithdrawalTx
	err := r.DB(ctx).
		Where("status = ?", model.WithdrawalTxStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&txs).Error
	return txs, err
}

func (r *withdrawalRepository) ListByWallet(ctx context.Context, wallet string, page *Pagination) ([]*model.WithdrawalTx, error) {
	var txs []*model.WithdrawalTx

	query := r.DB(ctx).Model(&model.WithdrawalTx{}).Where("wallet_address = ?", wallet)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&txs).Error
	return txs, err
}

func (r *withdrawalRepository) ListByStatus(ctx context.Context, status model.WithdrawalTxStatus, page *Pagination) ([]*model.WithdrawalTx, error) {
	var txs []*model.WithdrawalTx

	query := r.DB(ctx).Model(&model.WithdrawalTx{}).Where("status = ?", status)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&txs).Error
	return txs, err
}

func (r *withdrawalRepository) ListByTimeRange(ctx context.Context, start, end int64, page *Pagination) ([]*model.WithdrawalTx, error) {
	var txs []*model.WithdrawalTx

	query := r.DB(ctx).Model(&model.WithdrawalTx{}).
		Where("created_at >= ? AND created_at < ?", start, end)

	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	err := query.
		Order("created_at DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&txs).Error
	return txs, err
}
