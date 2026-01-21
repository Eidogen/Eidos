// Package repository provides whitelist data access
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"gorm.io/gorm"
)

var (
	ErrWhitelistNotFound  = errors.New("whitelist entry not found")
	ErrWhitelistDuplicate = errors.New("whitelist entry already exists")
)

// WhitelistRepository handles whitelist data access
type WhitelistRepository struct {
	db *gorm.DB
}

// NewWhitelistRepository creates a new whitelist repository
func NewWhitelistRepository(db *gorm.DB) *WhitelistRepository {
	return &WhitelistRepository{db: db}
}

// Create creates a new whitelist entry
func (r *WhitelistRepository) Create(ctx context.Context, entry *model.WhitelistEntry) error {
	now := time.Now().UnixMilli()
	if entry.CreatedAt == 0 {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	result := r.db.WithContext(ctx).Create(entry)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrWhitelistDuplicate
		}
		return result.Error
	}
	return nil
}

// Update updates a whitelist entry
func (r *WhitelistRepository) Update(ctx context.Context, entry *model.WhitelistEntry) error {
	entry.UpdatedAt = time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("entry_id = ?", entry.EntryID).
		Updates(map[string]interface{}{
			"status":          entry.Status,
			"label":           entry.Label,
			"remark":          entry.Remark,
			"metadata":        entry.Metadata,
			"effective_until": entry.EffectiveUntil,
			"updated_by":      entry.UpdatedBy,
			"updated_at":      entry.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWhitelistNotFound
	}
	return nil
}

// Delete deletes a whitelist entry
func (r *WhitelistRepository) Delete(ctx context.Context, entryID string) error {
	result := r.db.WithContext(ctx).
		Where("entry_id = ?", entryID).
		Delete(&model.WhitelistEntry{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWhitelistNotFound
	}
	return nil
}

// GetByEntryID retrieves a whitelist entry by entry ID
func (r *WhitelistRepository) GetByEntryID(ctx context.Context, entryID string) (*model.WhitelistEntry, error) {
	var entry model.WhitelistEntry
	err := r.db.WithContext(ctx).
		Where("entry_id = ?", entryID).
		First(&entry).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWhitelistNotFound
		}
		return nil, err
	}
	return &entry, nil
}

// GetByWallet retrieves all whitelist entries for a wallet
func (r *WhitelistRepository) GetByWallet(ctx context.Context, wallet string) ([]*model.WhitelistEntry, error) {
	var entries []*model.WhitelistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", wallet).
		Where("status = ?", model.WhitelistStatusActive).
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetByWalletAndType retrieves whitelist entries by wallet and type
func (r *WhitelistRepository) GetByWalletAndType(ctx context.Context, wallet string, listType model.WhitelistType) ([]*model.WhitelistEntry, error) {
	var entries []*model.WhitelistEntry
	err := r.db.WithContext(ctx).
		Where("wallet_address = ?", wallet).
		Where("list_type = ?", listType).
		Where("status = ?", model.WhitelistStatusActive).
		Order("created_at DESC").
		Find(&entries).Error

	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetAddressWhitelist retrieves the address whitelist for a wallet
func (r *WhitelistRepository) GetAddressWhitelist(ctx context.Context, wallet string) ([]*model.WhitelistEntry, error) {
	return r.GetByWalletAndType(ctx, wallet, model.WhitelistTypeAddress)
}

// IsAddressWhitelisted checks if an address is whitelisted for a wallet
func (r *WhitelistRepository) IsAddressWhitelisted(ctx context.Context, wallet, targetAddress string) (bool, error) {
	now := time.Now().UnixMilli()
	var count int64

	err := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("wallet_address = ?", wallet).
		Where("target_address = ?", targetAddress).
		Where("list_type = ?", model.WhitelistTypeAddress).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_from <= ?", now).
		Where("effective_until > ? OR effective_until = 0", now).
		Count(&count).Error

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsVIP checks if a wallet is a VIP user
func (r *WhitelistRepository) IsVIP(ctx context.Context, wallet string) (bool, error) {
	now := time.Now().UnixMilli()
	var count int64

	err := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("wallet_address = ?", wallet).
		Where("list_type = ?", model.WhitelistTypeVIP).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_from <= ?", now).
		Where("effective_until > ? OR effective_until = 0", now).
		Count(&count).Error

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsMarketMaker checks if a wallet is a market maker
func (r *WhitelistRepository) IsMarketMaker(ctx context.Context, wallet string) (bool, error) {
	now := time.Now().UnixMilli()
	var count int64

	err := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("wallet_address = ?", wallet).
		Where("list_type = ?", model.WhitelistTypeMarketMaker).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_from <= ?", now).
		Where("effective_until > ? OR effective_until = 0", now).
		Count(&count).Error

	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListByType lists whitelist entries by type with pagination
func (r *WhitelistRepository) ListByType(ctx context.Context, listType model.WhitelistType, pagination *Pagination) ([]*model.WhitelistEntry, int64, error) {
	var entries []*model.WhitelistEntry
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("list_type = ?", listType)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

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

// ListActive lists all active whitelist entries with pagination
func (r *WhitelistRepository) ListActive(ctx context.Context, pagination *Pagination) ([]*model.WhitelistEntry, int64, error) {
	var entries []*model.WhitelistEntry
	var total int64

	now := time.Now().UnixMilli()
	query := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_from <= ?", now).
		Where("effective_until > ? OR effective_until = 0", now)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

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

// ListPending lists pending whitelist entries
func (r *WhitelistRepository) ListPending(ctx context.Context, pagination *Pagination) ([]*model.WhitelistEntry, int64, error) {
	var entries []*model.WhitelistEntry
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("status = ?", model.WhitelistStatusPending)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at ASC").
		Find(&entries).Error

	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

// Approve approves a pending whitelist entry
func (r *WhitelistRepository) Approve(ctx context.Context, entryID string, approver string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("entry_id = ?", entryID).
		Where("status = ?", model.WhitelistStatusPending).
		Updates(map[string]interface{}{
			"status":      model.WhitelistStatusActive,
			"approved_by": approver,
			"approved_at": now,
			"updated_by":  approver,
			"updated_at":  now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWhitelistNotFound
	}
	return nil
}

// Reject rejects a pending whitelist entry
func (r *WhitelistRepository) Reject(ctx context.Context, entryID string, rejector string, reason string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("entry_id = ?", entryID).
		Where("status = ?", model.WhitelistStatusPending).
		Updates(map[string]interface{}{
			"status":     model.WhitelistStatusInactive,
			"remark":     reason,
			"updated_by": rejector,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWhitelistNotFound
	}
	return nil
}

// Deactivate deactivates a whitelist entry
func (r *WhitelistRepository) Deactivate(ctx context.Context, entryID string, operator string, reason string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("entry_id = ?", entryID).
		Where("status = ?", model.WhitelistStatusActive).
		Updates(map[string]interface{}{
			"status":     model.WhitelistStatusInactive,
			"remark":     reason,
			"updated_by": operator,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrWhitelistNotFound
	}
	return nil
}

// CleanupExpired marks expired entries as expired
func (r *WhitelistRepository) CleanupExpired(ctx context.Context) (int64, error) {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_until > 0").
		Where("effective_until < ?", now).
		Updates(map[string]interface{}{
			"status":     model.WhitelistStatusExpired,
			"updated_at": now,
		})

	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// GetAllActiveAddresses gets all active whitelisted addresses for a wallet
func (r *WhitelistRepository) GetAllActiveAddresses(ctx context.Context, wallet string) ([]string, error) {
	var addresses []string
	now := time.Now().UnixMilli()

	err := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Where("wallet_address = ?", wallet).
		Where("list_type = ?", model.WhitelistTypeAddress).
		Where("status = ?", model.WhitelistStatusActive).
		Where("effective_from <= ?", now).
		Where("effective_until > ? OR effective_until = 0", now).
		Pluck("target_address", &addresses).Error

	if err != nil {
		return nil, err
	}
	return addresses, nil
}

// CountByStatus counts entries by status
func (r *WhitelistRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type result struct {
		Status string
		Count  int64
	}
	var results []result

	err := r.db.WithContext(ctx).
		Model(&model.WhitelistEntry{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
