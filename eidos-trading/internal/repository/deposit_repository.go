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
	ErrDepositNotFound      = errors.New("deposit not found")
	ErrDepositAlreadyExists = errors.New("deposit already exists")
)

// DepositRepository 充值仓储接口
type DepositRepository interface {
	// Create 创建充值记录
	Create(ctx context.Context, deposit *model.Deposit) error

	// GetByID 根据 ID 查询
	GetByID(ctx context.Context, id int64) (*model.Deposit, error)

	// GetByDepositID 根据充值 ID 查询
	GetByDepositID(ctx context.Context, depositID string) (*model.Deposit, error)

	// GetByTxHashLogIndex 根据交易哈希和日志索引查询 (幂等键)
	GetByTxHashLogIndex(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error)

	// ListByWallet 查询用户充值列表
	ListByWallet(ctx context.Context, wallet string, filter *DepositFilter, page *Pagination) ([]*model.Deposit, error)

	// ListPending 查询待确认充值
	ListPending(ctx context.Context, limit int) ([]*model.Deposit, error)

	// ListConfirmed 查询已确认待入账充值
	ListConfirmed(ctx context.Context, limit int) ([]*model.Deposit, error)

	// UpdateStatus 更新充值状态
	UpdateStatus(ctx context.Context, depositID string, oldStatus, newStatus model.DepositStatus) error

	// MarkConfirmed 标记为已确认
	MarkConfirmed(ctx context.Context, depositID string) error

	// MarkCredited 标记为已入账
	MarkCredited(ctx context.Context, depositID string) error

	// CountByWallet 统计用户充值数
	CountByWallet(ctx context.Context, wallet string, filter *DepositFilter) (int64, error)
}

// DepositFilter 充值查询过滤条件
type DepositFilter struct {
	Token     string               // 代币
	Status    *model.DepositStatus // 充值状态
	TimeRange *TimeRange           // 时间范围
}

// depositRepository 充值仓储实现
type depositRepository struct {
	*Repository
}

// NewDepositRepository 创建充值仓储
func NewDepositRepository(db *gorm.DB) DepositRepository {
	return &depositRepository{
		Repository: NewRepository(db),
	}
}

// Create 创建充值记录
func (r *depositRepository) Create(ctx context.Context, deposit *model.Deposit) error {
	result := r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tx_hash"}, {Name: "log_index"}},
		DoNothing: true,
	}).Create(deposit)

	if result.Error != nil {
		return fmt.Errorf("create deposit failed: %w", result.Error)
	}

	// 如果是重复插入，返回已存在错误
	if result.RowsAffected == 0 {
		return ErrDepositAlreadyExists
	}
	return nil
}

// GetByID 根据 ID 查询
func (r *depositRepository) GetByID(ctx context.Context, id int64) (*model.Deposit, error) {
	var deposit model.Deposit
	result := r.DB(ctx).Where("id = ?", id).First(&deposit)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrDepositNotFound
		}
		return nil, fmt.Errorf("get deposit by id failed: %w", result.Error)
	}
	return &deposit, nil
}

// GetByDepositID 根据充值 ID 查询
func (r *depositRepository) GetByDepositID(ctx context.Context, depositID string) (*model.Deposit, error) {
	var deposit model.Deposit
	result := r.DB(ctx).Where("deposit_id = ?", depositID).First(&deposit)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrDepositNotFound
		}
		return nil, fmt.Errorf("get deposit by deposit_id failed: %w", result.Error)
	}
	return &deposit, nil
}

// GetByTxHashLogIndex 根据交易哈希和日志索引查询 (幂等键)
func (r *depositRepository) GetByTxHashLogIndex(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error) {
	var deposit model.Deposit
	result := r.DB(ctx).Where("tx_hash = ? AND log_index = ?", txHash, logIndex).First(&deposit)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrDepositNotFound
		}
		return nil, fmt.Errorf("get deposit by tx_hash_log_index failed: %w", result.Error)
	}
	return &deposit, nil
}

// ListByWallet 查询用户充值列表
func (r *depositRepository) ListByWallet(ctx context.Context, wallet string, filter *DepositFilter, page *Pagination) ([]*model.Deposit, error) {
	db := r.DB(ctx).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Deposit{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count deposits failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var deposits []*model.Deposit
	db = db.Order("created_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&deposits).Error; err != nil {
		return nil, fmt.Errorf("list deposits by wallet failed: %w", err)
	}
	return deposits, nil
}

// ListPending 查询待确认充值
func (r *depositRepository) ListPending(ctx context.Context, limit int) ([]*model.Deposit, error) {
	var deposits []*model.Deposit
	result := r.DB(ctx).
		Where("status = ?", model.DepositStatusPending).
		Order("detected_at ASC").
		Limit(limit).
		Find(&deposits)

	if result.Error != nil {
		return nil, fmt.Errorf("list pending deposits failed: %w", result.Error)
	}
	return deposits, nil
}

// ListConfirmed 查询已确认待入账充值
func (r *depositRepository) ListConfirmed(ctx context.Context, limit int) ([]*model.Deposit, error) {
	var deposits []*model.Deposit
	result := r.DB(ctx).
		Where("status = ?", model.DepositStatusConfirmed).
		Order("confirmed_at ASC").
		Limit(limit).
		Find(&deposits)

	if result.Error != nil {
		return nil, fmt.Errorf("list confirmed deposits failed: %w", result.Error)
	}
	return deposits, nil
}

// UpdateStatus 更新充值状态
func (r *depositRepository) UpdateStatus(ctx context.Context, depositID string, oldStatus, newStatus model.DepositStatus) error {
	result := r.DB(ctx).Model(&model.Deposit{}).
		Where("deposit_id = ? AND status = ?", depositID, oldStatus).
		Update("status", newStatus)

	if result.Error != nil {
		return fmt.Errorf("update deposit status failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkConfirmed 标记为已确认
func (r *depositRepository) MarkConfirmed(ctx context.Context, depositID string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Deposit{}).
		Where("deposit_id = ? AND status = ?", depositID, model.DepositStatusPending).
		Updates(map[string]interface{}{
			"status":       model.DepositStatusConfirmed,
			"confirmed_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark deposit confirmed failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// MarkCredited 标记为已入账
func (r *depositRepository) MarkCredited(ctx context.Context, depositID string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Deposit{}).
		Where("deposit_id = ? AND status = ?", depositID, model.DepositStatusConfirmed).
		Updates(map[string]interface{}{
			"status":      model.DepositStatusCredited,
			"credited_at": now,
			"updated_at":  now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark deposit credited failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// CountByWallet 统计用户充值数
func (r *depositRepository) CountByWallet(ctx context.Context, wallet string, filter *DepositFilter) (int64, error) {
	db := r.DB(ctx).Model(&model.Deposit{}).Where("wallet = ?", wallet)
	db = r.applyFilter(db, filter)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count deposits failed: %w", err)
	}
	return count, nil
}

// applyFilter 应用过滤条件
func (r *depositRepository) applyFilter(db *gorm.DB, filter *DepositFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	if filter.Token != "" {
		db = db.Where("token = ?", filter.Token)
	}
	if filter.Status != nil {
		db = db.Where("status = ?", *filter.Status)
	}
	if filter.TimeRange != nil && filter.TimeRange.IsValid() {
		db = db.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
	}

	return db
}
