package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrWalletNonceNotFound = errors.New("wallet nonce not found")
	ErrPendingTxNotFound   = errors.New("pending tx not found")
)

// NonceRepository Nonce 管理仓储接口
type NonceRepository interface {
	// 钱包 Nonce
	GetWalletNonce(ctx context.Context, wallet string, chainID int64) (*model.WalletNonce, error)
	UpsertWalletNonce(ctx context.Context, nonce *model.WalletNonce) error
	IncrementNonce(ctx context.Context, wallet string, chainID int64) (int64, error)

	// 待确认交易
	CreatePendingTx(ctx context.Context, tx *model.PendingTx) error
	GetPendingTxByHash(ctx context.Context, txHash string) (*model.PendingTx, error)
	GetPendingTxByRef(ctx context.Context, txType model.PendingTxType, refID string) (*model.PendingTx, error)
	UpdatePendingTxStatus(ctx context.Context, txHash string, status model.PendingTxStatus) error
	ListPendingTxs(ctx context.Context, wallet string, chainID int64) ([]*model.PendingTx, error)
	ListTimedOutTxs(ctx context.Context, chainID int64, limit int) ([]*model.PendingTx, error)
	DeleteConfirmedTxs(ctx context.Context, olderThan int64) (int64, error)
}

// nonceRepository Nonce 管理仓储实现
type nonceRepository struct {
	*Repository
}

// NewNonceRepository 创建 Nonce 管理仓储
func NewNonceRepository(db *gorm.DB) NonceRepository {
	return &nonceRepository{
		Repository: NewRepository(db),
	}
}

func (r *nonceRepository) GetWalletNonce(ctx context.Context, wallet string, chainID int64) (*model.WalletNonce, error) {
	var nonce model.WalletNonce
	err := r.DB(ctx).
		Where("wallet_address = ? AND chain_id = ?", wallet, chainID).
		First(&nonce).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWalletNonceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &nonce, nil
}

func (r *nonceRepository) UpsertWalletNonce(ctx context.Context, nonce *model.WalletNonce) error {
	now := time.Now().UnixMilli()
	nonce.SyncedAt = now
	nonce.UpdatedAt = now
	if nonce.CreatedAt == 0 {
		nonce.CreatedAt = now
	}

	return r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet_address"}, {Name: "chain_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"current_nonce", "synced_at", "updated_at"}),
	}).Create(nonce).Error
}

func (r *nonceRepository) IncrementNonce(ctx context.Context, wallet string, chainID int64) (int64, error) {
	now := time.Now().UnixMilli()

	// 使用 SELECT FOR UPDATE 锁定行
	var nonce model.WalletNonce
	err := r.DB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("wallet_address = ? AND chain_id = ?", wallet, chainID).
		First(&nonce).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 不存在则创建
		newNonce := &model.WalletNonce{
			WalletAddress: wallet,
			ChainID:       chainID,
			CurrentNonce:  0,
			SyncedAt:      now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := r.DB(ctx).Create(newNonce).Error; err != nil {
			return 0, err
		}
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	// 递增 nonce
	newNonceValue := nonce.CurrentNonce + 1
	err = r.DB(ctx).Model(&model.WalletNonce{}).
		Where("wallet_address = ? AND chain_id = ?", wallet, chainID).
		Updates(map[string]interface{}{
			"current_nonce": newNonceValue,
			"updated_at":    now,
		}).Error
	if err != nil {
		return 0, err
	}

	return nonce.CurrentNonce, nil
}

func (r *nonceRepository) CreatePendingTx(ctx context.Context, tx *model.PendingTx) error {
	now := time.Now().UnixMilli()
	tx.CreatedAt = now
	tx.UpdatedAt = now
	return r.DB(ctx).Create(tx).Error
}

func (r *nonceRepository) GetPendingTxByHash(ctx context.Context, txHash string) (*model.PendingTx, error) {
	var tx model.PendingTx
	err := r.DB(ctx).Where("tx_hash = ?", txHash).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPendingTxNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *nonceRepository) GetPendingTxByRef(ctx context.Context, txType model.PendingTxType, refID string) (*model.PendingTx, error) {
	var tx model.PendingTx
	err := r.DB(ctx).
		Where("tx_type = ? AND ref_id = ? AND status = ?", txType, refID, model.PendingTxStatusPending).
		First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPendingTxNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *nonceRepository) UpdatePendingTxStatus(ctx context.Context, txHash string, status model.PendingTxStatus) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.PendingTx{}).
		Where("tx_hash = ?", txHash).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPendingTxNotFound
	}
	return nil
}

func (r *nonceRepository) ListPendingTxs(ctx context.Context, wallet string, chainID int64) ([]*model.PendingTx, error) {
	var txs []*model.PendingTx
	err := r.DB(ctx).
		Where("wallet_address = ? AND chain_id = ? AND status = ?", wallet, chainID, model.PendingTxStatusPending).
		Order("nonce ASC").
		Find(&txs).Error
	return txs, err
}

func (r *nonceRepository) ListTimedOutTxs(ctx context.Context, chainID int64, limit int) ([]*model.PendingTx, error) {
	now := time.Now().UnixMilli()
	var txs []*model.PendingTx
	err := r.DB(ctx).
		Where("chain_id = ? AND status = ? AND timeout_at < ?", chainID, model.PendingTxStatusPending, now).
		Order("timeout_at ASC").
		Limit(limit).
		Find(&txs).Error
	return txs, err
}

func (r *nonceRepository) DeleteConfirmedTxs(ctx context.Context, olderThan int64) (int64, error) {
	result := r.DB(ctx).
		Where("status IN ? AND created_at < ?",
			[]model.PendingTxStatus{model.PendingTxStatusConfirmed, model.PendingTxStatusFailed, model.PendingTxStatusReplaced},
			olderThan).
		Delete(&model.PendingTx{})
	return result.RowsAffected, result.Error
}
