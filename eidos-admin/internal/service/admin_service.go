package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// AdminService 管理员服务
type AdminService struct {
	adminRepo *repository.AdminRepository
	auditRepo *repository.AuditLogRepository
}

// NewAdminService 创建管理员服务
func NewAdminService(adminRepo *repository.AdminRepository, auditRepo *repository.AuditLogRepository) *AdminService {
	return &AdminService{
		adminRepo: adminRepo,
		auditRepo: auditRepo,
	}
}

// CreateAdminRequest 创建管理员请求
type CreateAdminRequest struct {
	Username  string     `json:"username" binding:"required,min=3,max=50"`
	Password  string     `json:"password" binding:"required,min=8"`
	Nickname  string     `json:"nickname" binding:"max=50"`
	Email     string     `json:"email" binding:"email"`
	Phone     string     `json:"phone"`
	Role      model.Role `json:"role" binding:"required"`
	OperatorID int64     `json:"-"`
}

// Create 创建管理员
func (s *AdminService) Create(ctx context.Context, req *CreateAdminRequest) (*model.Admin, error) {
	// 检查用户名是否已存在
	existing, err := s.adminRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("用户名已存在")
	}

	// 密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	admin := &model.Admin{
		Username:     req.Username,
		PasswordHash: string(hash),
		Nickname:     req.Nickname,
		Email:        req.Email,
		Phone:        req.Phone,
		Role:         req.Role,
		Status:       model.AdminStatusActive,
		CreatedBy:    req.OperatorID,
		UpdatedBy:    req.OperatorID,
	}

	if err := s.adminRepo.Create(ctx, admin); err != nil {
		return nil, err
	}

	// 清除密码
	admin.PasswordHash = ""
	return admin, nil
}

// UpdateAdminRequest 更新管理员请求
type UpdateAdminRequest struct {
	ID         int64       `json:"-"`
	Nickname   string      `json:"nickname"`
	Email      string      `json:"email"`
	Phone      string      `json:"phone"`
	Role       model.Role  `json:"role"`
	Status     *model.AdminStatus `json:"status"`
	OperatorID int64       `json:"-"`
}

// Update 更新管理员
func (s *AdminService) Update(ctx context.Context, req *UpdateAdminRequest) (*model.Admin, error) {
	admin, err := s.adminRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, errors.New("管理员不存在")
	}

	// 保存旧值用于审计
	oldValue := model.JSONMap{
		"nickname": admin.Nickname,
		"email":    admin.Email,
		"phone":    admin.Phone,
		"role":     admin.Role,
		"status":   admin.Status,
	}

	// 更新字段
	if req.Nickname != "" {
		admin.Nickname = req.Nickname
	}
	if req.Email != "" {
		admin.Email = req.Email
	}
	if req.Phone != "" {
		admin.Phone = req.Phone
	}
	if req.Role != "" {
		admin.Role = req.Role
	}
	if req.Status != nil {
		admin.Status = *req.Status
	}
	admin.UpdatedBy = req.OperatorID

	if err := s.adminRepo.Update(ctx, admin); err != nil {
		return nil, err
	}

	// 记录审计日志
	newValue := model.JSONMap{
		"nickname": admin.Nickname,
		"email":    admin.Email,
		"phone":    admin.Phone,
		"role":     admin.Role,
		"status":   admin.Status,
	}

	s.recordAudit(ctx, req.OperatorID, model.AuditActionUpdate, model.ResourceTypeAdmin,
		admin.Username, "更新管理员信息", oldValue, newValue)

	// 清除密码
	admin.PasswordHash = ""
	return admin, nil
}

// Delete 删除管理员
func (s *AdminService) Delete(ctx context.Context, id, operatorID int64) error {
	admin, err := s.adminRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if admin == nil {
		return errors.New("管理员不存在")
	}

	// 不能删除自己
	if id == operatorID {
		return errors.New("不能删除自己")
	}

	if err := s.adminRepo.Delete(ctx, id); err != nil {
		return err
	}

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionDelete, model.ResourceTypeAdmin,
		admin.Username, "删除管理员", nil, nil)

	return nil
}

// GetByID 根据 ID 获取管理员
func (s *AdminService) GetByID(ctx context.Context, id int64) (*model.Admin, error) {
	admin, err := s.adminRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if admin != nil {
		admin.PasswordHash = ""
	}
	return admin, nil
}

// List 获取管理员列表
func (s *AdminService) List(ctx context.Context, page *model.Pagination) ([]*model.Admin, error) {
	admins, err := s.adminRepo.List(ctx, page)
	if err != nil {
		return nil, err
	}

	// 清除密码
	for _, admin := range admins {
		admin.PasswordHash = ""
	}
	return admins, nil
}

// ResetPassword 重置密码
func (s *AdminService) ResetPassword(ctx context.Context, id int64, newPassword string, operatorID int64) error {
	admin, err := s.adminRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if admin == nil {
		return errors.New("管理员不存在")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin.PasswordHash = string(hash)
	admin.UpdatedBy = operatorID

	if err := s.adminRepo.Update(ctx, admin); err != nil {
		return err
	}

	// 记录审计日志
	s.recordAudit(ctx, operatorID, model.AuditActionUpdate, model.ResourceTypeAdmin,
		admin.Username, "重置管理员密码", nil, nil)

	return nil
}

// recordAudit 记录审计日志
func (s *AdminService) recordAudit(ctx context.Context, operatorID int64, action model.AuditAction,
	resourceType model.ResourceType, resourceID, description string, oldValue, newValue model.JSONMap) {

	// 获取操作者信息
	operator, _ := s.adminRepo.GetByID(ctx, operatorID)
	username := ""
	if operator != nil {
		username = operator.Username
	}

	auditLog := &model.AuditLog{
		AdminID:       operatorID,
		AdminUsername: username,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Description:   description,
		OldValue:      oldValue,
		NewValue:      newValue,
		Status:        model.AuditStatusSuccess,
	}
	s.auditRepo.Create(ctx, auditLog)
}
