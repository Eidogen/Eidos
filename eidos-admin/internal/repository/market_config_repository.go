package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// MarketConfigRepository 交易对配置仓储
type MarketConfigRepository struct {
	db *gorm.DB
}

// NewMarketConfigRepository 创建交易对配置仓储
func NewMarketConfigRepository(db *gorm.DB) *MarketConfigRepository {
	return &MarketConfigRepository{db: db}
}

// Create 创建交易对配置
func (r *MarketConfigRepository) Create(ctx context.Context, config *model.MarketConfig) error {
	return r.db.WithContext(ctx).Create(config).Error
}

// Update 更新交易对配置
func (r *MarketConfigRepository) Update(ctx context.Context, config *model.MarketConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

// Delete 删除交易对配置
func (r *MarketConfigRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.MarketConfig{}, id).Error
}

// GetByID 根据 ID 获取交易对配置
func (r *MarketConfigRepository) GetByID(ctx context.Context, id int64) (*model.MarketConfig, error) {
	var config model.MarketConfig
	err := r.db.WithContext(ctx).First(&config, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// GetBySymbol 根据交易对符号获取配置
func (r *MarketConfigRepository) GetBySymbol(ctx context.Context, symbol string) (*model.MarketConfig, error) {
	var config model.MarketConfig
	err := r.db.WithContext(ctx).Where("symbol = ?", symbol).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// List 获取交易对配置列表
func (r *MarketConfigRepository) List(ctx context.Context, page *model.Pagination, status *model.MarketStatus) ([]*model.MarketConfig, error) {
	var configs []*model.MarketConfig

	query := r.db.WithContext(ctx).Model(&model.MarketConfig{})

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

// ListAll 获取所有交易对配置
func (r *MarketConfigRepository) ListAll(ctx context.Context) ([]*model.MarketConfig, error) {
	var configs []*model.MarketConfig
	err := r.db.WithContext(ctx).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// ListActive 获取活跃的交易对配置
func (r *MarketConfigRepository) ListActive(ctx context.Context) ([]*model.MarketConfig, error) {
	var configs []*model.MarketConfig
	err := r.db.WithContext(ctx).
		Where("status = ?", model.MarketStatusActive).
		Order("display_order ASC, id ASC").
		Find(&configs).Error
	return configs, err
}

// UpdateStatus 更新交易对状态
func (r *MarketConfigRepository) UpdateStatus(ctx context.Context, symbol string, status model.MarketStatus, tradingEnabled bool, updatedBy int64) error {
	return r.db.WithContext(ctx).Model(&model.MarketConfig{}).
		Where("symbol = ?", symbol).
		Updates(map[string]interface{}{
			"status":          status,
			"trading_enabled": tradingEnabled,
			"updated_by":      updatedBy,
		}).Error
}

// Count 统计交易对数量
func (r *MarketConfigRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.MarketConfig{}).Count(&count).Error
	return count, err
}

// CountByStatus 按状态统计交易对数量
func (r *MarketConfigRepository) CountByStatus(ctx context.Context, status model.MarketStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.MarketConfig{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}
