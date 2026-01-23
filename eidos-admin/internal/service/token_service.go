package service

import (
	"context"
	"errors"
	"strings"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// TokenService 代币配置服务
type TokenService struct {
	tokenRepo  *repository.TokenRepository
	configRepo *repository.SystemConfigRepository
	auditRepo  *repository.AuditLogRepository
}

// NewTokenService 创建代币配置服务
func NewTokenService(
	tokenRepo *repository.TokenRepository,
	configRepo *repository.SystemConfigRepository,
	auditRepo *repository.AuditLogRepository,
) *TokenService {
	return &TokenService{
		tokenRepo:  tokenRepo,
		configRepo: configRepo,
		auditRepo:  auditRepo,
	}
}

// CreateTokenRequest 创建代币请求
type CreateTokenRequest struct {
	Symbol          string `json:"symbol" binding:"required"`
	Name            string `json:"name" binding:"required"`
	ContractAddress string `json:"contract_address"`
	Decimals        int    `json:"decimals" binding:"min=0,max=18"`
	ChainID         int64  `json:"chain_id"`
	MinDeposit      string `json:"min_deposit"`
	MinWithdraw     string `json:"min_withdraw"`
	MaxWithdraw     string `json:"max_withdraw"`
	WithdrawFee     string `json:"withdraw_fee"`
	Confirmations   int    `json:"confirmations"`
	DisplayOrder    int    `json:"display_order"`
	IconURL         string `json:"icon_url"`
	Description     string `json:"description"`
	OperatorID      int64  `json:"-"`
}

// Create 创建代币配置
func (s *TokenService) Create(ctx context.Context, req *CreateTokenRequest) (*model.TokenConfig, error) {
	// 符号标准化为大写
	req.Symbol = strings.ToUpper(req.Symbol)

	// 检查是否已存在
	exists, err := s.tokenRepo.ExistsBySymbol(ctx, req.Symbol)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("代币符号已存在")
	}

	// 设置默认值
	if req.ChainID == 0 {
		req.ChainID = 42161 // 默认 Arbitrum
	}
	if req.Decimals == 0 {
		req.Decimals = 18
	}
	if req.MinDeposit == "" {
		req.MinDeposit = "0"
	}
	if req.MinWithdraw == "" {
		req.MinWithdraw = "0"
	}
	if req.MaxWithdraw == "" {
		req.MaxWithdraw = "0"
	}
	if req.WithdrawFee == "" {
		req.WithdrawFee = "0"
	}

	config := &model.TokenConfig{
		Symbol:          req.Symbol,
		Name:            req.Name,
		ContractAddress: req.ContractAddress,
		Decimals:        req.Decimals,
		ChainID:         req.ChainID,
		Status:          model.TokenStatusDisabled, // 新建默认禁用
		DepositEnabled:  false,
		WithdrawEnabled: false,
		MinDeposit:      req.MinDeposit,
		MinWithdraw:     req.MinWithdraw,
		MaxWithdraw:     req.MaxWithdraw,
		WithdrawFee:     req.WithdrawFee,
		Confirmations:   req.Confirmations,
		DisplayOrder:    req.DisplayOrder,
		IconURL:         req.IconURL,
		Description:     req.Description,
		CreatedBy:       req.OperatorID,
		UpdatedBy:       req.OperatorID,
	}

	if err := s.tokenRepo.Create(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeTokenConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionCreate, req.Symbol,
		"创建代币配置", nil, s.configToJSONMap(config))

	return config, nil
}

// UpdateTokenRequest 更新代币请求
type UpdateTokenRequest struct {
	ID              int64   `json:"-"`
	Symbol          string  `json:"-"`
	Name            *string `json:"name"`
	MinDeposit      *string `json:"min_deposit"`
	MinWithdraw     *string `json:"min_withdraw"`
	MaxWithdraw     *string `json:"max_withdraw"`
	WithdrawFee     *string `json:"withdraw_fee"`
	Confirmations   *int    `json:"confirmations"`
	DisplayOrder    *int    `json:"display_order"`
	IconURL         *string `json:"icon_url"`
	Description     *string `json:"description"`
	DepositEnabled  *bool   `json:"deposit_enabled"`
	WithdrawEnabled *bool   `json:"withdraw_enabled"`
	OperatorID      int64   `json:"-"`
}

// Update 更新代币配置
func (s *TokenService) Update(ctx context.Context, req *UpdateTokenRequest) (*model.TokenConfig, error) {
	// 优先通过 ID 查询
	var config *model.TokenConfig
	var err error
	if req.ID > 0 {
		config, err = s.tokenRepo.GetByID(ctx, req.ID)
	} else {
		config, err = s.tokenRepo.GetBySymbol(ctx, req.Symbol)
	}
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errors.New("代币不存在")
	}

	oldValue := s.configToJSONMap(config)

	// 更新字段
	if req.Name != nil {
		config.Name = *req.Name
	}
	if req.MinDeposit != nil {
		config.MinDeposit = *req.MinDeposit
	}
	if req.MinWithdraw != nil {
		config.MinWithdraw = *req.MinWithdraw
	}
	if req.MaxWithdraw != nil {
		config.MaxWithdraw = *req.MaxWithdraw
	}
	if req.WithdrawFee != nil {
		config.WithdrawFee = *req.WithdrawFee
	}
	if req.Confirmations != nil {
		config.Confirmations = *req.Confirmations
	}
	if req.DisplayOrder != nil {
		config.DisplayOrder = *req.DisplayOrder
	}
	if req.IconURL != nil {
		config.IconURL = *req.IconURL
	}
	if req.Description != nil {
		config.Description = *req.Description
	}
	if req.DepositEnabled != nil {
		config.DepositEnabled = *req.DepositEnabled
	}
	if req.WithdrawEnabled != nil {
		config.WithdrawEnabled = *req.WithdrawEnabled
	}
	config.UpdatedBy = req.OperatorID

	if err := s.tokenRepo.Update(ctx, config); err != nil {
		return nil, err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeTokenConfigs, req.OperatorID)

	// 记录审计日志
	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, config.Symbol,
		"更新代币配置", oldValue, s.configToJSONMap(config))

	return config, nil
}

// TokenUpdateStatusRequest 更新代币状态请求
type TokenUpdateStatusRequest struct {
	Symbol     string            `json:"-"`
	Status     model.TokenStatus `json:"status" binding:"required"`
	OperatorID int64             `json:"-"`
}

// UpdateStatus 更新代币状态
func (s *TokenService) UpdateStatus(ctx context.Context, req *TokenUpdateStatusRequest) error {
	config, err := s.tokenRepo.GetBySymbol(ctx, req.Symbol)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("代币不存在")
	}

	oldValue := model.JSONMap{
		"status": config.Status,
	}

	if err := s.tokenRepo.UpdateStatus(ctx, req.Symbol, req.Status, req.OperatorID); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeTokenConfigs, req.OperatorID)

	// 记录审计日志
	newValue := model.JSONMap{
		"status": req.Status,
	}
	s.recordAudit(ctx, req.OperatorID, model.AuditActionStatusChange, req.Symbol,
		"更新代币状态", oldValue, newValue)

	return nil
}

// GetBySymbol 根据符号获取代币配置
func (s *TokenService) GetBySymbol(ctx context.Context, symbol string) (*model.TokenConfig, error) {
	return s.tokenRepo.GetBySymbol(ctx, strings.ToUpper(symbol))
}

// GetByID 根据ID获取代币配置
func (s *TokenService) GetByID(ctx context.Context, id int64) (*model.TokenConfig, error) {
	return s.tokenRepo.GetByID(ctx, id)
}

// List 获取代币配置列表
func (s *TokenService) List(ctx context.Context, page *model.Pagination, status *model.TokenStatus) ([]*model.TokenConfig, error) {
	return s.tokenRepo.List(ctx, page, status)
}

// ListAll 获取所有代币配置
func (s *TokenService) ListAll(ctx context.Context) ([]*model.TokenConfig, error) {
	return s.tokenRepo.ListAll(ctx)
}

// ListActive 获取所有活跃代币
func (s *TokenService) ListActive(ctx context.Context) ([]*model.TokenConfig, error) {
	return s.tokenRepo.ListActive(ctx)
}

// ListDepositEnabled 获取可充值的代币
func (s *TokenService) ListDepositEnabled(ctx context.Context) ([]*model.TokenConfig, error) {
	return s.tokenRepo.ListDepositEnabled(ctx)
}

// ListWithdrawEnabled 获取可提现的代币
func (s *TokenService) ListWithdrawEnabled(ctx context.Context) ([]*model.TokenConfig, error) {
	return s.tokenRepo.ListWithdrawEnabled(ctx)
}

// Delete 删除代币配置
func (s *TokenService) Delete(ctx context.Context, id int64, operatorID int64) error {
	config, err := s.tokenRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if config == nil {
		return errors.New("代币不存在")
	}

	// 不能删除活跃的代币
	if config.Status == model.TokenStatusActive {
		return errors.New("不能删除活跃状态的代币，请先禁用")
	}

	oldValue := s.configToJSONMap(config)

	if err := s.tokenRepo.Delete(ctx, id); err != nil {
		return err
	}

	// 增加配置版本号
	s.configRepo.BumpVersion(ctx, model.ConfigScopeTokenConfigs, operatorID)

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionDelete, config.Symbol,
		"删除代币配置", oldValue, nil)

	return nil
}

// GetConfigVersion 获取配置版本
func (s *TokenService) GetConfigVersion(ctx context.Context) (int64, error) {
	version, err := s.configRepo.GetVersion(ctx, model.ConfigScopeTokenConfigs)
	if err != nil {
		return 0, err
	}
	if version == nil {
		return 0, nil
	}
	return version.Version, nil
}

// configToJSONMap 配置转 JSONMap
func (s *TokenService) configToJSONMap(config *model.TokenConfig) model.JSONMap {
	return model.JSONMap{
		"symbol":           config.Symbol,
		"name":             config.Name,
		"contract_address": config.ContractAddress,
		"decimals":         config.Decimals,
		"chain_id":         config.ChainID,
		"status":           config.Status,
		"deposit_enabled":  config.DepositEnabled,
		"withdraw_enabled": config.WithdrawEnabled,
		"min_deposit":      config.MinDeposit,
		"min_withdraw":     config.MinWithdraw,
		"max_withdraw":     config.MaxWithdraw,
		"withdraw_fee":     config.WithdrawFee,
	}
}

// recordAudit 记录审计日志
func (s *TokenService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceID, description string, oldValue, newValue model.JSONMap) {

	auditLog := &model.AuditLog{
		AdminID:      operatorID,
		Action:       action,
		ResourceType: model.ResourceTypeToken,
		ResourceID:   resourceID,
		Description:  description,
		OldValue:     oldValue,
		NewValue:     newValue,
		Status:       model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
