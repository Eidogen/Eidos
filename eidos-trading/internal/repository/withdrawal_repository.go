package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrWithdrawalNotFound      = errors.New("withdrawal not found")
	ErrWithdrawalAlreadyExists = errors.New("withdrawal already exists")
)

// WithdrawalRepository 提现仓储接口
type WithdrawalRepository interface {
	// Create 创建提现记录
	Create(ctx context.Context, withdrawal *model.Withdrawal) error

	// GetByID 根据 ID 查询
	GetByID(ctx context.Context, id int64) (*model.Withdrawal, error)

	// GetByWithdrawID 根据提现 ID 查询
	GetByWithdrawID(ctx context.Context, withdrawID string) (*model.Withdrawal, error)

	// GetByWalletNonce 根据钱包和 Nonce 查询 (幂等键)
	GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Withdrawal, error)

	// ListByWallet 查询用户提现列表
	ListByWallet(ctx context.Context, wallet string, filter *WithdrawalFilter, page *Pagination) ([]*model.Withdrawal, error)

	// ListPending 查询待处理提现
	ListPending(ctx context.Context, limit int) ([]*model.Withdrawal, error)

	// ListProcessing 查询处理中提现
	ListProcessing(ctx context.Context, limit int) ([]*model.Withdrawal, error)

	// ListSubmitted 查询已提交提现
	ListSubmitted(ctx context.Context, limit int) ([]*model.Withdrawal, error)

	// UpdateStatus 更新提现状态
	UpdateStatus(ctx context.Context, withdrawID string, oldStatus, newStatus model.WithdrawStatus) error

	// MarkProcessing 标记为处理中
	MarkProcessing(ctx context.Context, withdrawID string) error

	// MarkSubmitted 标记为已提交
	MarkSubmitted(ctx context.Context, withdrawID string, txHash string) error

	// MarkConfirmed 标记为已确认
	MarkConfirmed(ctx context.Context, withdrawID string) error

	// MarkFailed 标记为失败
	MarkFailed(ctx context.Context, withdrawID string, reason string) error

	// MarkRejected 标记为拒绝
	MarkRejected(ctx context.Context, withdrawID string, reason string) error

	// MarkCancelled 标记为已取消
	MarkCancelled(ctx context.Context, withdrawID string) error

	// MarkRefunded 标记为已退回
	MarkRefunded(ctx context.Context, withdrawID string) error

	// CountByWallet 统计用户提现数
	CountByWallet(ctx context.Context, wallet string, filter *WithdrawalFilter) (int64, error)

	// CountPendingByWallet 统计用户待处理提现数
	CountPendingByWallet(ctx context.Context, wallet string) (int64, error)
}

// WithdrawalFilter 提现查询过滤条件
type WithdrawalFilter struct {
	Token     string                 // 代币
	Status    *model.WithdrawStatus  // 提现状态
	Statuses  []model.WithdrawStatus // 多个状态
	TimeRange *TimeRange             // 时间范围
}

// withdrawalRepository 提现仓储实现
type withdrawalRepository struct {
	*Repository
}

// NewWithdrawalRepository 创建提现仓储
func NewWithdrawalRepository(db *gorm.DB) WithdrawalRepository {
	return &withdrawalRepository{
		Repository: NewRepository(db),
	}
}

// Create 创建提现记录
func (r *withdrawalRepository) Create(ctx context.Context, withdrawal *model.Withdrawal) error {
	result := r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet"}, {Name: "nonce"}},
		DoNothing: true,
	}).Create(withdrawal)

	if result.Error != nil {
		return fmt.Errorf("create withdrawal failed: %w", result.Error)
	}

	// 如果是重复插入，返回已存在错误
	if result.RowsAffected == 0 {
		return ErrWithdrawalAlreadyExists
	}
	return nil
}

// GetByID 根据 ID 查询
func (r *withdrawalRepository) GetByID(ctx context.Context, id int64) (*model.Withdrawal, error) {
	var withdrawal model.Withdrawal
	result := r.DB(ctx).Where("id = ?", id).First(&withdrawal)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("get withdrawal by id failed: %w", result.Error)
	}
	return &withdrawal, nil
}

// GetByWithdrawID 根据提现 ID 查询
func (r *withdrawalRepository) GetByWithdrawID(ctx context.Context, withdrawID string) (*model.Withdrawal, error) {
	var withdrawal model.Withdrawal
	result := r.DB(ctx).Where("withdraw_id = ?", withdrawID).First(&withdrawal)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("get withdrawal by withdraw_id failed: %w", result.Error)
	}
	return &withdrawal, nil
}

// GetByWalletNonce 根据钱包和 Nonce 查询 (幂等键)
func (r *withdrawalRepository) GetByWalletNonce(ctx context.Context, wallet string, nonce uint64) (*model.Withdrawal, error) {
	var withdrawal model.Withdrawal
	result := r.DB(ctx).Where("wallet = ? AND nonce = ?", wallet, nonce).First(&withdrawal)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("get withdrawal by wallet_nonce failed: %w", result.Error)
	}
	return &withdrawal, nil
}

// ListByWallet 查询用户提现列表
func (r *withdrawalRepository) ListByWallet(ctx context.Context, wallet string, filter *WithdrawalFilter, page *Pagination) ([]*model.Withdrawal, error) {
	db := r.DB(ctx).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Withdrawal{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count withdrawals failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var withdrawals []*model.Withdrawal
	db = db.Order("created_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&withdrawals).Error; err != nil {
		return nil, fmt.Errorf("list withdrawals by wallet failed: %w", err)
	}
	return withdrawals, nil
}

// ListPending 查询待处理提现
func (r *withdrawalRepository) ListPending(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	var withdrawals []*model.Withdrawal
	result := r.DB(ctx).
		Where("status = ?", model.WithdrawStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&withdrawals)

	if result.Error != nil {
		return nil, fmt.Errorf("list pending withdrawals failed: %w", result.Error)
	}
	return withdrawals, nil
}

// ListProcessing 查询处理中提现
func (r *withdrawalRepository) ListProcessing(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	var withdrawals []*model.Withdrawal
	result := r.DB(ctx).
		Where("status = ?", model.WithdrawStatusProcessing).
		Order("created_at ASC").
		Limit(limit).
		Find(&withdrawals)

	if result.Error != nil {
		return nil, fmt.Errorf("list processing withdrawals failed: %w", result.Error)
	}
	return withdrawals, nil
}

// ListSubmitted 查询已提交提现
func (r *withdrawalRepository) ListSubmitted(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	var withdrawals []*model.Withdrawal
	result := r.DB(ctx).
		Where("status = ?", model.WithdrawStatusSubmitted).
		Order("submitted_at ASC").
		Limit(limit).
		Find(&withdrawals)

	if result.Error != nil {
		return nil, fmt.Errorf("list submitted withdrawals failed: %w", result.Error)
	}
	return withdrawals, nil
}

// UpdateStatus 更新提现状态
func (r *withdrawalRepository) UpdateStatus(ctx context.Context, withdrawID string, oldStatus, newStatus model.WithdrawStatus) error {
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, oldStatus).
		Update("status", newStatus)

	if result.Error != nil {
		return fmt.Errorf("update withdrawal status failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkProcessing 标记为处理中
func (r *withdrawalRepository) MarkProcessing(ctx context.Context, withdrawID string) error {
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusPending).
		Updates(map[string]interface{}{
			"status":     model.WithdrawStatusProcessing,
			"updated_at": time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal processing failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkSubmitted 标记为已提交
func (r *withdrawalRepository) MarkSubmitted(ctx context.Context, withdrawID string, txHash string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusProcessing).
		Updates(map[string]interface{}{
			"status":       model.WithdrawStatusSubmitted,
			"tx_hash":      txHash,
			"submitted_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal submitted failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkConfirmed 标记为已确认
func (r *withdrawalRepository) MarkConfirmed(ctx context.Context, withdrawID string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusSubmitted).
		Updates(map[string]interface{}{
			"status":       model.WithdrawStatusConfirmed,
			"confirmed_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal confirmed failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkFailed 标记为失败
func (r *withdrawalRepository) MarkFailed(ctx context.Context, withdrawID string, reason string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status IN ?", withdrawID, []model.WithdrawStatus{
			model.WithdrawStatusProcessing,
			model.WithdrawStatusSubmitted,
		}).
		Updates(map[string]interface{}{
			"status":        model.WithdrawStatusFailed,
			"reject_reason": reason,
			"updated_at":    now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkRejected 标记为拒绝
func (r *withdrawalRepository) MarkRejected(ctx context.Context, withdrawID string, reason string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusPending).
		Updates(map[string]interface{}{
			"status":        model.WithdrawStatusRejected,
			"reject_reason": reason,
			"updated_at":    now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal rejected failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkCancelled 标记为已取消
func (r *withdrawalRepository) MarkCancelled(ctx context.Context, withdrawID string) error {
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusPending).
		Updates(map[string]interface{}{
			"status":     model.WithdrawStatusCancelled,
			"updated_at": time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal cancelled failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkRefunded 标记为已退回
func (r *withdrawalRepository) MarkRefunded(ctx context.Context, withdrawID string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("withdraw_id = ? AND status = ?", withdrawID, model.WithdrawStatusFailed).
		Updates(map[string]interface{}{
			"refunded_at": now,
			"updated_at":  now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark withdrawal refunded failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// CountByWallet 统计用户提现数
func (r *withdrawalRepository) CountByWallet(ctx context.Context, wallet string, filter *WithdrawalFilter) (int64, error) {
	db := r.DB(ctx).Model(&model.Withdrawal{}).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count withdrawals failed: %w", err)
	}
	return count, nil
}

// CountPendingByWallet 统计用户待处理提现数
func (r *withdrawalRepository) CountPendingByWallet(ctx context.Context, wallet string) (int64, error) {
	var count int64
	result := r.DB(ctx).Model(&model.Withdrawal{}).
		Where("wallet = ? AND status IN ?", wallet, []model.WithdrawStatus{
			model.WithdrawStatusPending,
			model.WithdrawStatusProcessing,
			model.WithdrawStatusSubmitted,
		}).
		Count(&count)

	if result.Error != nil {
		return 0, fmt.Errorf("count pending withdrawals failed: %w", result.Error)
	}
	return count, nil
}

// applyFilter 应用过滤条件
func (r *withdrawalRepository) applyFilter(db *gorm.DB, filter *WithdrawalFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	if filter.Token != "" {
		db = db.Where("token = ?", filter.Token)
	}
	if filter.Status != nil {
		db = db.Where("status = ?", *filter.Status)
	}
	if len(filter.Statuses) > 0 {
		db = db.Where("status IN ?", filter.Statuses)
	}
	if filter.TimeRange != nil && filter.TimeRange.IsValid() {
		db = db.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
	}

	return db
}
