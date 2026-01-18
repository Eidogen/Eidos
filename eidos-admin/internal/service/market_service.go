package service

import (
	"context"
	"errors"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// MarketService 交易对服务
type MarketService struct {
	marketRepo *repository.MarketConfigRepository
	configRepo *repository.SystemConfigRepository
	auditRepo  *repository.AuditLogRepository
}

// NewMarketService 创建交易对服务
func NewMarketService(
	marketRepo *repository.MarketConfigRepository,
	configRepo *repository.SystemConfigRepository,
	auditRepo *repository.AuditLogRepository,
) *MarketService {
	return &MarketService{
		marketRepo: marketRepo,
		configRepo: configRepo,
		auditRepo:  auditRepo,
	}
}

// CreateMarketRequest 创建交易对请求
type CreateMarketRequest struct {
	Symbol         string `json:"symbol" binding:"required"`
	BaseToken      string `json:"base_token" binding:"required"`
	QuoteToken     string `json:"quote_token" binding:"required"`
	BaseTokenAddr  string `json:"base_token_addr" binding:"required"`
	QuoteTokenAddr string `json:"quote_token_addr" binding:"required"`
	PriceDecimals  int    `json:"price_decimals" binding:"min=0,max=18"`
	SizeDecimals   int    `json:"size_decimals" binding:"min=0,max=18"`
	MinSize        string `json:"min_size" binding:"required"`
	MaxSize        string `json:"max_size" binding:"required"`
	MinNotional    string `json:"min_notional" binding:"required"`
	TickSize       string `json:"tick_size" binding:"required"`
	MakerFee       string `json:"maker_fee" binding:"required"`
	TakerFee       string `json:"taker_fee" binding:"required"`
	DisplayOrder   int    `json:"display_order"`
	Description    string `json:"description"`
	OperatorID     int64  `json:"-"`
}

// Create 创建交易对
func (s *MarketService) Create(ctx context.Context, req *CreateMarketRequest) (*model.MarketConfig, error) {
	// 检查是否已存在
	existing, err := s.marketRepo.GetBySymbol(ctx, req.Symbol)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("交易对已存在")
	}

	config := &model.MarketConfig{
		Symbol:         req.Symbol,
		BaseToken:      req.BaseToken,
		QuoteToken:     req.QuoteToken,
		BaseTokenAddr:  req.BaseTokenAddr,
		QuoteTokenAddr: req.QuoteTokenAddr,
		PriceDecimals:  req.PriceDecimals,
		SizeDecimals:   req.SizeDecimals,
		MinSize:        req.MinSize,
		MaxSize:        req.MaxSize,
		MinNotional:    req.MinNotional,
		TickSize:       req.TickSize,
		MakerFee:       req.MakerFee,
		TakerFee:       req.TakerFee,
		Status:         model.MarketStatusSuspended, // 新建默认暂停
		TradingEnabled: false,
		DisplayOrder:   req.DisplayOrder,
		Description:    req.Description,
		CreatedBy:      req.OperatorID,
		UpdatedBy:      req.OperatorID,
	}

	if err := s.marketRepo.Create(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeMarketConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionCreate, req.Symbol,
		"创建交易对", nil, s.configToJSONMap(config))

	return config, nil
}

// UpdateMarketRequest 更新交易对请求
type UpdateMarketRequest struct {
	ID           int64   `json:"-"`
	Symbol       string  `json:"-"`
	MinSize      *string `json:"min_size"`
	MaxSize      *string `json:"max_size"`
	MinNotional  *string `json:"min_notional"`
	MakerFee     *string `json:"maker_fee"`
	TakerFee     *string `json:"taker_fee"`
	DisplayOrder *int    `json:"display_order"`
	Description  *string `json:"description"`
	OperatorID   int64   `json:"-"`
}

// Update 更新交易对
func (s *MarketService) Update(ctx context.Context, req *UpdateMarketRequest) (*model.MarketConfig, error) {
	// 优先通过 ID 查询
	var config *model.MarketConfig
	var err error
	if req.ID > 0 {
		config, err = s.marketRepo.GetByID(ctx, req.ID)
	} else {
		config, err = s.marketRepo.GetBySymbol(ctx, req.Symbol)
	}
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errors.New("交易对不存在")
	}

	oldValue := s.configToJSONMap(config)

	// 更新字段
	if req.MinSize != nil {
		config.MinSize = *req.MinSize
	}
	if req.MaxSize != nil {
		config.MaxSize = *req.MaxSize
	}
	if req.MinNotional != nil {
		config.MinNotional = *req.MinNotional
	}
	if req.MakerFee != nil {
		config.MakerFee = *req.MakerFee
	}
	if req.TakerFee != nil {
		config.TakerFee = *req.TakerFee
	}
	if req.DisplayOrder != nil {
		config.DisplayOrder = *req.DisplayOrder
	}
	if req.Description != nil {
		config.Description = *req.Description
	}
	config.UpdatedBy = req.OperatorID

	if err := s.marketRepo.Update(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeMarketConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, req.Symbol,
		"更新交易对配置", oldValue, s.configToJSONMap(config))

	return config, nil
}

// UpdateStatusRequest 更新交易对状态请求
type UpdateStatusRequest struct {
	Symbol         string             `json:"-"`
	Status         model.MarketStatus `json:"status" binding:"required"`
	TradingEnabled bool               `json:"trading_enabled"`
	OperatorID     int64              `json:"-"`
}

// UpdateStatus 更新交易对状态
func (s *MarketService) UpdateStatus(ctx context.Context, req *UpdateStatusRequest) error {
	config, err := s.marketRepo.GetBySymbol(ctx, req.Symbol)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("交易对不存在")
	}

	oldValue := model.JSONMap{
		"status":          config.Status,
		"trading_enabled": config.TradingEnabled,
	}

	if err := s.marketRepo.UpdateStatus(ctx, req.Symbol, req.Status, req.TradingEnabled, req.OperatorID); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeMarketConfigs, req.OperatorID)

	// 记录审计日志
	newValue := model.JSONMap{
		"status":          req.Status,
		"trading_enabled": req.TradingEnabled,
	}
	s.recordAudit(ctx, req.OperatorID, model.AuditActionStatusChange, req.Symbol,
		"更新交易对状态", oldValue, newValue)

	return nil
}

// GetBySymbol 根据符号获取交易对
func (s *MarketService) GetBySymbol(ctx context.Context, symbol string) (*model.MarketConfig, error) {
	return s.marketRepo.GetBySymbol(ctx, symbol)
}

// List 获取交易对列表
func (s *MarketService) List(ctx context.Context, page *model.Pagination, status *model.MarketStatus) ([]*model.MarketConfig, error) {
	return s.marketRepo.List(ctx, page, status)
}

// ListAll 获取所有交易对
func (s *MarketService) ListAll(ctx context.Context) ([]*model.MarketConfig, error) {
	return s.marketRepo.ListAll(ctx)
}

// GetByID 根据ID获取交易对
func (s *MarketService) GetByID(ctx context.Context, id int64) (*model.MarketConfig, error) {
	return s.marketRepo.GetByID(ctx, id)
}

// GetAllActive 获取所有活跃交易对
func (s *MarketService) GetAllActive(ctx context.Context) ([]*model.MarketConfig, error) {
	return s.marketRepo.ListActive(ctx)
}

// Delete 删除交易对
func (s *MarketService) Delete(ctx context.Context, id int64, operatorID int64) error {
	config, err := s.marketRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("交易对不存在")
	}

	oldValue := s.configToJSONMap(config)

	if err := s.marketRepo.Delete(ctx, id); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeMarketConfigs, operatorID)

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionDelete, config.Symbol,
		"删除交易对", oldValue, nil)

	return nil
}

// GetConfigVersion 获取配置版本
func (s *MarketService) GetConfigVersion(ctx context.Context) (int64, error) {
	version, err := s.configRepo.GetVersion(ctx, model.ConfigScopeMarketConfigs)
	if err != nil {
		return 0, err
	}
	if version == nil {
		return 0, nil
	}
	return version.Version, nil
}

// configToJSONMap 配置转 JSONMap
func (s *MarketService) configToJSONMap(config *model.MarketConfig) model.JSONMap {
	return model.JSONMap{
		"symbol":          config.Symbol,
		"base_token":      config.BaseToken,
		"quote_token":     config.QuoteToken,
		"price_decimals":  config.PriceDecimals,
		"size_decimals":   config.SizeDecimals,
		"min_size":        config.MinSize,
		"max_size":        config.MaxSize,
		"min_notional":    config.MinNotional,
		"tick_size":       config.TickSize,
		"maker_fee":       config.MakerFee,
		"taker_fee":       config.TakerFee,
		"status":          config.Status,
		"trading_enabled": config.TradingEnabled,
	}
}

// recordAudit 记录审计日志
func (s *MarketService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceID, description string, oldValue, newValue model.JSONMap) {

	auditLog := &model.AuditLog{
		AdminID:      operatorID,
		Action:       action,
		ResourceType: model.ResourceTypeMarket,
		ResourceID:   resourceID,
		Description:  description,
		OldValue:     oldValue,
		NewValue:     newValue,
		Status:       model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
