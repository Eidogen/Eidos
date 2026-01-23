package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// TokenRepository 代币配置仓储
type TokenRepository struct {
	db *gorm.DB
}

// NewTokenRepository 创建代币配置仓储
func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

// Create 创建代币配置
func (r *TokenRepository) Create(ctx context.Context, config *model.TokenConfig) error {
	return r.db.WithContext(ctx).Create(config).Error
}

// Update 更新代币配置
func (r *TokenRepository) Update(ctx context.Context, config *model.TokenConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

// Delete 删除代币配置
func (r *TokenRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.TokenConfig{}, id).Error
}

// GetByID 根据 ID 获取代币配置
func (r *TokenRepository) GetByID(ctx context.Context, id int64) (*model.TokenConfig, error) {
	var config model.TokenConfig
	err := r.db.WithContext(ctx).First(&config, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// GetBySymbol 根据符号获取代币配置
func (r *TokenRepository) GetBySymbol(ctx context.Context, symbol string) (*model.TokenConfig, error) {
	var config model.TokenConfig
	err := r.db.WithContext(ctx).Where("symbol = ?", symbol).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// GetByContractAddress 根据合约地址获取代币配置
func (r *TokenRepository) GetByContractAddress(ctx context.Context, address string) (*model.TokenConfig, error) {
	var config model.TokenConfig
	err := r.db.WithContext(ctx).Where("contract_address = ?", address).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// List 获取代币配置列表
func (r *TokenRepository) List(ctx context.Context, page *model.Pagination, status *model.TokenStatus) ([]*model.TokenConfig, error) {
	var configs []*model.TokenConfig

	query := r.db.WithContext(ctx).Model(&model.TokenConfig{})

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 计算总数
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	err := query.Order("display_order ASC, id ASC").
		Offset(page.GetOffset()).
		Limit(page.GetLimit()).
		Find(&configs).Error

	return configs, err
}

// ListAll 获取所有代币配置
func (r *TokenRepository) ListAll(ctx context.Context) ([]*model.TokenConfig, error) {
	var configs []*model.TokenConfig
	err := r.db.WithContext(ctx).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// ListActive 获取活跃的代币配置
func (r *TokenRepository) ListActive(ctx context.Context) ([]*model.TokenConfig, error) {
	var configs []*model.TokenConfig
	err := r.db.WithContext(ctx).
		Where("status = ?", model.TokenStatusActive).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// ListDepositEnabled 获取可充值的代币
func (r *TokenRepository) ListDepositEnabled(ctx context.Context) ([]*model.TokenConfig, error) {
	var configs []*model.TokenConfig
	err := r.db.WithContext(ctx).
		Where("status = ? AND deposit_enabled = ?", model.TokenStatusActive, true).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// ListWithdrawEnabled 获取可提现的代币
func (r *TokenRepository) ListWithdrawEnabled(ctx context.Context) ([]*model.TokenConfig, error) {
	var configs []*model.TokenConfig
	err := r.db.WithContext(ctx).
		Where("status = ? AND withdraw_enabled = ?", model.TokenStatusActive, true).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// UpdateStatus 更新代币状态
func (r *TokenRepository) UpdateStatus(ctx context.Context, symbol string, status model.TokenStatus, updatedBy int64) error {
	return r.db.WithContext(ctx).Model(&model.TokenConfig{}).
		Where("symbol = ?", symbol).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_by": updatedBy,
		}).Error
}

// UpdateDepositEnabled 更新充值状态
func (r *TokenRepository) UpdateDepositEnabled(ctx context.Context, symbol string, enabled bool, updatedBy int64) error {
	return r.db.WithContext(ctx).Model(&model.TokenConfig{}).
		Where("symbol = ?", symbol).
		Updates(map[string]interface{}{
			"deposit_enabled": enabled,
			"updated_by":      updatedBy,
		}).Error
}

// UpdateWithdrawEnabled 更新提现状态
func (r *TokenRepository) UpdateWithdrawEnabled(ctx context.Context, symbol string, enabled bool, updatedBy int64) error {
	return r.db.WithContext(ctx).Model(&model.TokenConfig{}).
		Where("symbol = ?", symbol).
		Updates(map[string]interface{}{
			"withdraw_enabled": enabled,
			"updated_by":       updatedBy,
		}).Error
}

// Count 统计代币数量
func (r *TokenRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TokenConfig{}).Count(&count).Error
	return count, err
}

// CountByStatus 按状态统计代币数量
func (r *TokenRepository) CountByStatus(ctx context.Context, status model.TokenStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TokenConfig{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}

// ExistsBySymbol 检查符号是否已存在
func (r *TokenRepository) ExistsBySymbol(ctx context.Context, symbol string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.TokenConfig{}).
		Where("symbol = ?", symbol).
		Count(&count).Error
	return count > 0, err
}
