package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
)

var (
	ErrBlacklistNotFound  = errors.New("blacklist entry not found")
	ErrBlacklistDuplicate = errors.New("blacklist entry already exists")
)

// BlacklistRepository 黑名单仓储
type BlacklistRepository struct {
	db *gorm.DB
}

// NewBlacklistRepository 创建黑名单仓储
func NewBlacklistRepository(db *gorm.DB) *BlacklistRepository {
	return &BlacklistRepository{db: db}
}

// Create 创建黑名单条目
func (r *BlacklistRepository) Create(ctx context.Context, entry *model.BlacklistEntry) error {
	now := time.Now().UnixMilli()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	result := r.db.WithContext(ctx).Create(entry)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrBlacklistDuplicate
		}
		return result.Error
	}
	return nil
}

// Update 更新黑名单条目
func (r *BlacklistRepository) Update(ctx context.Context, entry *model.BlacklistEntry) error {
	entry.UpdatedAt = time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("id = ?", entry.ID).
		Updates(map[string]interface{}{
			"reason":          entry.Reason,
			"effective_until": entry.EffectiveUntil,
			"status":          entry.Status,
			"updated_by":      entry.UpdatedBy,
			"updated_at":      entry.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBlacklistNotFound
	}
	return nil
}

// GetByWallet 根据钱包地址获取黑名单条目
func (r *BlacklistRepository) GetByWallet(ctx context.Context, walletAddress string) ([]*model.BlacklistEntry, error) {
	var entries []*model.BlacklistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Where("status = ?", model.BlacklistStatusActive).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetByWalletAndType 根据钱包地址和类型获取
func (r *BlacklistRepository) GetByWalletAndType(ctx context.Context, walletAddress string, listType model.BlacklistType) (*model.BlacklistEntry, error) {
	var entry model.BlacklistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Where("list_type = ?", listType).
		Where("status = ?", model.BlacklistStatusActive).
		First(&entry).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 不在黑名单中返回nil
		}
		return nil, err
	}
	return &entry, nil
}

// CheckBlacklisted 检查钱包是否在黑名单中
func (r *BlacklistRepository) CheckBlacklisted(ctx context.Context, walletAddress string) (bool, string, int64, error) {
	now := time.Now().UnixMilli()

	var entry model.BlacklistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Where("status = ?", model.BlacklistStatusActive).
		Where("effective_from <= ?", now).
		Where("(effective_until IS NULL OR effective_until = 0 OR effective_until > ?)", now).
		First(&entry).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", 0, nil
		}
		return false, "", 0, err
	}
	return true, entry.Reason, entry.EffectiveUntil, nil
}

// CheckBlacklistedByType 检查钱包是否在特定类型黑名单中
func (r *BlacklistRepository) CheckBlacklistedByType(ctx context.Context, walletAddress string, listType model.BlacklistType) (bool, *model.BlacklistEntry, error) {
	now := time.Now().UnixMilli()

	var entry model.BlacklistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", walletAddress).
		Where("status = ?", model.BlacklistStatusActive).
		Where("(list_type = ? OR list_type = ?)", listType, model.BlacklistTypeFull).
		Where("effective_from <= ?", now).
		Where("(effective_until IS NULL OR effective_until = 0 OR effective_until > ?)", now).
		First(&entry).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, &entry, nil
}

// Remove 移除黑名单条目 (软删除)
func (r *BlacklistRepository) Remove(ctx context.Context, walletAddress string, updatedBy string) error {
	result := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("wallet_address = ?", walletAddress).
		Where("status = ?", model.BlacklistStatusActive).
		Updates(map[string]interface{}{
			"status":     model.BlacklistStatusRemoved,
			"updated_by": updatedBy,
			"updated_at": time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBlacklistNotFound
	}
	return nil
}

// RemoveByType 移除特定类型的黑名单条目 (软删除)
func (r *BlacklistRepository) RemoveByType(ctx context.Context, walletAddress string, listType model.BlacklistType, updatedBy string) error {
	result := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("wallet_address = ?", walletAddress).
		Where("list_type = ?", listType).
		Where("status = ?", model.BlacklistStatusActive).
		Updates(map[string]interface{}{
			"status":     model.BlacklistStatusRemoved,
			"updated_by": updatedBy,
			"updated_at": time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBlacklistNotFound
	}
	return nil
}

// ListActive 查询所有活跃黑名单
func (r *BlacklistRepository) ListActive(ctx context.Context, pagination *Pagination) ([]*model.BlacklistEntry, int64, error) {
	var entries []*model.BlacklistEntry
	var total int64

	now := time.Now().UnixMilli()
	query := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("status = ?", model.BlacklistStatusActive).
		Where("effective_from <= ?", now).
		Where("(effective_until IS NULL OR effective_until = 0 OR effective_until > ?)", now)

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// ListByType 根据类型查询黑名单
func (r *BlacklistRepository) ListByType(ctx context.Context, listType model.BlacklistType, pagination *Pagination) ([]*model.BlacklistEntry, int64, error) {
	var entries []*model.BlacklistEntry
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("list_type = ?", listType).
		Where("status = ?", model.BlacklistStatusActive)

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// GetAllActiveWallets 获取所有活跃黑名单钱包地址 (用于缓存刷新)
func (r *BlacklistRepository) GetAllActiveWallets(ctx context.Context) ([]*model.BlacklistEntry, error) {
	var entries []*model.BlacklistEntry
	now := time.Now().UnixMilli()

	err := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("status = ?", model.BlacklistStatusActive).
		Where("effective_from <= ?", now).
		Where("(effective_until IS NULL OR effective_until = 0 OR effective_until > ?)", now).
		Find(&entries).Error

	if err != nil {
		return nil, err
	}
	return entries, nil
}

// CleanupExpired 清理过期的黑名单条目
func (r *BlacklistRepository) CleanupExpired(ctx context.Context) (int64, error) {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.BlacklistEntry{}).
		Where("status = ?", model.BlacklistStatusActive).
		Where("effective_until > 0").
		Where("effective_until < ?", now).
		Updates(map[string]interface{}{
			"status":     model.BlacklistStatusExpired,
			"updated_at": now,
		})

	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
