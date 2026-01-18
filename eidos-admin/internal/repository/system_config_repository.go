package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// SystemConfigRepository 系统配置仓储
type SystemConfigRepository struct {
	db *gorm.DB
}

// NewSystemConfigRepository 创建系统配置仓储
func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

// Create 创建系统配置
func (r *SystemConfigRepository) Create(ctx context.Context, config *model.SystemConfig) error {
	return r.db.WithContext(ctx).Create(config).Error
}

// Update 更新系统配置
func (r *SystemConfigRepository) Update(ctx context.Context, config *model.SystemConfig) error {
	return r.db.WithContext(ctx).Save(config).Error
}

// Delete 删除系统配置
func (r *SystemConfigRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.SystemConfig{}, id).Error
}

// GetByID 根据 ID 获取系统配置
func (r *SystemConfigRepository) GetByID(ctx context.Context, id int64) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := r.db.WithContext(ctx).First(&config, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// GetByKey 根据配置键获取系统配置
func (r *SystemConfigRepository) GetByKey(ctx context.Context, key string) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := r.db.WithContext(ctx).Where("config_key = ?", key).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &config, err
}

// List 获取系统配置列表
func (r *SystemConfigRepository) List(ctx context.Context, page *model.Pagination, scope string) ([]*model.SystemConfig, error) {
	var configs []*model.SystemConfig

	query := r.db.WithContext(ctx).Model(&model.SystemConfig{})

	if scope != "" {
		query = query.Where("scope = ?", scope)
	}

	// 计算总数
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	err := query.Order("config_key ASC").
		Offset(page.GetOffset()).
		Limit(page.GetLimit()).
		Find(&configs).Error

	return configs, err
}

// ListByScope 根据 scope 获取配置列表
func (r *SystemConfigRepository) ListByScope(ctx context.Context, scope string) ([]*model.SystemConfig, error) {
	var configs []*model.SystemConfig
	err := r.db.WithContext(ctx).
		Where("scope = ?", scope).
		Order("config_key ASC").
		Find(&configs).Error
	return configs, err
}

// ListAll 获取所有系统配置
func (r *SystemConfigRepository) ListAll(ctx context.Context) ([]*model.SystemConfig, error) {
	var configs []*model.SystemConfig
	err := r.db.WithContext(ctx).
		Order("scope ASC, config_key ASC").
		Find(&configs).Error
	return configs, err
}

// UpdateValue 更新配置值
func (r *SystemConfigRepository) UpdateValue(ctx context.Context, key string, value string, updatedBy int64) error {
	return r.db.WithContext(ctx).Model(&model.SystemConfig{}).
		Where("config_key = ?", key).
		Updates(map[string]interface{}{
			"config_value": value,
			"updated_by":   updatedBy,
		}).Error
}

// GetVersion 获取配置版本
func (r *SystemConfigRepository) GetVersion(ctx context.Context, scope string) (*model.ConfigVersion, error) {
	var version model.ConfigVersion
	err := r.db.WithContext(ctx).Where("scope = ?", scope).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &version, err
}

// BumpVersion 增加配置版本号
func (r *SystemConfigRepository) BumpVersion(ctx context.Context, scope string, updatedBy int64) (int64, error) {
	var version model.ConfigVersion
	err := r.db.WithContext(ctx).Where("scope = ?", scope).First(&version).Error

	if err == gorm.ErrRecordNotFound {
		// 创建新版本记录
		version = model.ConfigVersion{
			Scope:     scope,
			Version:   1,
			UpdatedBy: updatedBy,
		}
		if err := r.db.WithContext(ctx).Create(&version).Error; err != nil {
			return 0, err
		}
		return 1, nil
	}

	if err != nil {
		return 0, err
	}

	// 增加版本号
	newVersion := version.Version + 1
	err = r.db.WithContext(ctx).Model(&model.ConfigVersion{}).
		Where("scope = ?", scope).
		Updates(map[string]interface{}{
			"version":    newVersion,
			"updated_by": updatedBy,
		}).Error

	return newVersion, err
}

// GetAllVersions 获取所有配置版本
func (r *SystemConfigRepository) GetAllVersions(ctx context.Context) ([]*model.ConfigVersion, error) {
	var versions []*model.ConfigVersion
	err := r.db.WithContext(ctx).Find(&versions).Error
	return versions, err
}

// GetCurrentVersion 获取当前配置版本号
func (r *SystemConfigRepository) GetCurrentVersion(ctx context.Context, scope string) (int64, error) {
	version, err := r.GetVersion(ctx, scope)
	if err != nil {
		return 0, err
	}
	if version == nil {
		return 0, nil
	}
	return version.Version, nil
}

// GetVersionHistory 获取配置版本历史（分页）
func (r *SystemConfigRepository) GetVersionHistory(ctx context.Context, page *model.Pagination) ([]*model.ConfigVersion, error) {
	var versions []*model.ConfigVersion

	query := r.db.WithContext(ctx).Model(&model.ConfigVersion{})

	// 计算总数
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	err := query.Order("version DESC").
		Offset(page.GetOffset()).
		Limit(page.GetLimit()).
		Find(&versions).Error

	return versions, err
}
