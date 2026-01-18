package service

import (
	"context"
	"errors"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// ConfigService 配置服务
type ConfigService struct {
	configRepo *repository.SystemConfigRepository
	auditRepo  *repository.AuditLogRepository
}

// NewConfigService 创建配置服务
func NewConfigService(configRepo *repository.SystemConfigRepository, auditRepo *repository.AuditLogRepository) *ConfigService {
	return &ConfigService{
		configRepo: configRepo,
		auditRepo:  auditRepo,
	}
}

// CreateConfigRequest 创建配置请求
type CreateConfigRequest struct {
	ConfigKey   string           `json:"config_key" binding:"required"`
	ConfigValue string           `json:"config_value" binding:"required"`
	ConfigType  model.ConfigType `json:"config_type"`
	Description string           `json:"description"`
	IsSecret    bool             `json:"is_secret"`
	Scope       string           `json:"scope"`
	OperatorID  int64            `json:"-"`
}

// Create 创建配置
func (s *ConfigService) Create(ctx context.Context, req *CreateConfigRequest) (*model.SystemConfig, error) {
	// 检查是否已存在
	existing, err := s.configRepo.GetByKey(ctx, req.ConfigKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("配置键已存在")
	}

	configType := req.ConfigType
	if configType == "" {
		configType = model.ConfigTypeString
	}

	scope := req.Scope
	if scope == "" {
		scope = model.ConfigScopeGlobal
	}

	config := &model.SystemConfig{
		ConfigKey:   req.ConfigKey,
		ConfigValue: req.ConfigValue,
		ConfigType:  configType,
		Description: req.Description,
		IsSecret:    req.IsSecret,
		Scope:       scope,
		CreatedBy:   req.OperatorID,
		UpdatedBy:   req.OperatorID,
	}

	if err := s.configRepo.Create(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeSystemConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionCreate, req.ConfigKey,
		"创建系统配置", nil, s.configToJSONMap(config))

	return config, nil
}

// UpdateConfigRequest 更新配置请求
type UpdateConfigRequest struct {
	ID          int64  `json:"-"`
	ConfigKey   string `json:"-"`
	ConfigValue string `json:"config_value" binding:"required"`
	Description string `json:"description"`
	OperatorID  int64  `json:"-"`
}

// Update 更新配置
func (s *ConfigService) Update(ctx context.Context, req *UpdateConfigRequest) (*model.SystemConfig, error) {
	var config *model.SystemConfig
	var err error

	// 支持通过 ID 或 Key 获取配置
	if req.ID > 0 {
		config, err = s.configRepo.GetByID(ctx, req.ID)
	} else if req.ConfigKey != "" {
		config, err = s.configRepo.GetByKey(ctx, req.ConfigKey)
	} else {
		return nil, errors.New("必须提供配置ID或配置键")
	}

	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errors.New("配置不存在")
	}

	oldValue := s.configToJSONMap(config)

	// 更新字段
	config.ConfigValue = req.ConfigValue
	if req.Description != "" {
		config.Description = req.Description
	}
	config.UpdatedBy = req.OperatorID

	if err := s.configRepo.Update(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeSystemConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, config.ConfigKey,
		"更新系统配置", oldValue, s.configToJSONMap(config))

	return config, nil
}

// Delete 删除配置
func (s *ConfigService) Delete(ctx context.Context, key string, operatorID int64) error {
	config, err := s.configRepo.GetByKey(ctx, key)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("配置不存在")
	}

	if err := s.configRepo.Delete(ctx, config.ID); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeSystemConfigs, operatorID)

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionDelete, key,
		"删除系统配置", s.configToJSONMap(config), nil)

	return nil
}

// GetByKey 根据键获取配置
func (s *ConfigService) GetByKey(ctx context.Context, key string) (*model.SystemConfig, error) {
	config, err := s.configRepo.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	// 隐藏敏感值
	if config != nil && config.IsSecret {
		config.ConfigValue = "******"
	}

	return config, nil
}

// List 获取配置列表
func (s *ConfigService) List(ctx context.Context, page *model.Pagination, scope string) ([]*model.SystemConfig, error) {
	configs, err := s.configRepo.List(ctx, page, scope)
	if err != nil {
		return nil, err
	}

	// 隐藏敏感值
	for _, config := range configs {
		if config.IsSecret {
			config.ConfigValue = "******"
		}
	}

	return configs, nil
}

// GetAllVersions 获取所有配置版本
func (s *ConfigService) GetAllVersions(ctx context.Context) ([]*model.ConfigVersion, error) {
	return s.configRepo.GetAllVersions(ctx)
}

// GetByID 根据ID获取配置
func (s *ConfigService) GetByID(ctx context.Context, id int64) (*model.SystemConfig, error) {
	config, err := s.configRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 隐藏敏感值
	if config != nil && config.IsSecret {
		config.ConfigValue = "******"
	}

	return config, nil
}

// GetByCategory 根据分类获取配置
func (s *ConfigService) GetByCategory(ctx context.Context, category string) ([]*model.SystemConfig, error) {
	configs, err := s.configRepo.ListByScope(ctx, category)
	if err != nil {
		return nil, err
	}

	// 隐藏敏感值
	for _, config := range configs {
		if config.IsSecret {
			config.ConfigValue = "******"
		}
	}

	return configs, nil
}

// GetCurrentVersion 获取当前配置版本
func (s *ConfigService) GetCurrentVersion(ctx context.Context) (int64, error) {
	return s.configRepo.GetCurrentVersion(ctx, model.ConfigScopeSystemConfigs)
}

// GetVersionHistory 获取配置版本历史
func (s *ConfigService) GetVersionHistory(ctx context.Context, page *model.Pagination) ([]*model.ConfigVersion, error) {
	return s.configRepo.GetVersionHistory(ctx, page)
}

// BatchUpdate 批量更新配置
func (s *ConfigService) BatchUpdate(ctx context.Context, configs []struct {
	Key   string
	Value string
}, operatorID int64) error {
	for _, item := range configs {
		req := &UpdateConfigRequest{
			ConfigKey:   item.Key,
			ConfigValue: item.Value,
			OperatorID:  operatorID,
		}
		if _, err := s.Update(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByID 根据ID删除配置
func (s *ConfigService) DeleteByID(ctx context.Context, id int64, operatorID int64) error {
	config, err := s.configRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("配置不存在")
	}

	if err := s.configRepo.Delete(ctx, config.ID); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeSystemConfigs, operatorID)

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionDelete, config.ConfigKey,
		"删除系统配置", s.configToJSONMap(config), nil)

	return nil
}

// configToJSONMap 配置转 JSONMap
func (s *ConfigService) configToJSONMap(config *model.SystemConfig) model.JSONMap {
	value := config.ConfigValue
	if config.IsSecret {
		value = "******"
	}
	return model.JSONMap{
		"config_key":   config.ConfigKey,
		"config_value": value,
		"config_type":  config.ConfigType,
		"scope":        config.Scope,
	}
}

// recordAudit 记录审计日志
func (s *ConfigService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceID, description string, oldValue, newValue model.JSONMap) {

	auditLog := &model.AuditLog{
		AdminID:      operatorID,
		Action:       action,
		ResourceType: model.ResourceTypeSystemConfig,
		ResourceID:   resourceID,
		Description:  description,
		OldValue:     oldValue,
		NewValue:     newValue,
		Status:       model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
