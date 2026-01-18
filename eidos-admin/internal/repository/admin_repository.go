package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// AdminRepository 管理员仓储
type AdminRepository struct {
	db *gorm.DB
}

// NewAdminRepository 创建管理员仓储
func NewAdminRepository(db *gorm.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// Create 创建管理员
func (r *AdminRepository) Create(ctx context.Context, admin *model.Admin) error {
	return r.db.WithContext(ctx).Create(admin).Error
}

// Update 更新管理员
func (r *AdminRepository) Update(ctx context.Context, admin *model.Admin) error {
	return r.db.WithContext(ctx).Save(admin).Error
}

// Delete 删除管理员
func (r *AdminRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Admin{}, id).Error
}

// GetByID 根据 ID 获取管理员
func (r *AdminRepository) GetByID(ctx context.Context, id int64) (*model.Admin, error) {
	var admin model.Admin
	err := r.db.WithContext(ctx).First(&admin, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &admin, err
}

// GetByUsername 根据用户名获取管理员
func (r *AdminRepository) GetByUsername(ctx context.Context, username string) (*model.Admin, error) {
	var admin model.Admin
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&admin).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &admin, err
}

// List 获取管理员列表
func (r *AdminRepository) List(ctx context.Context, page *model.Pagination) ([]*model.Admin, error) {
	var admins []*model.Admin

	query := r.db.WithContext(ctx).Model(&model.Admin{})

	// 计算总数
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	err := query.Order("id DESC").
		Offset(page.GetOffset()).
		Limit(page.GetLimit()).
		Find(&admins).Error

	return admins, err
}

// ListByRole 根据角色获取管理员
func (r *AdminRepository) ListByRole(ctx context.Context, role model.Role) ([]*model.Admin, error) {
	var admins []*model.Admin
	err := r.db.WithContext(ctx).
		Where("role = ?", role).
		Order("id DESC").
		Find(&admins).Error
	return admins, err
}

// UpdateLoginSuccess 更新登录成功信息
func (r *AdminRepository) UpdateLoginSuccess(ctx context.Context, id int64, ip string) error {
	now := time.Now().UnixMilli()
	return r.db.WithContext(ctx).Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_login_at":  now,
			"last_login_ip":  ip,
			"login_attempts": 0,
			"locked_until":   nil,
		}).Error
}

// UpdateLoginFailed 更新登录失败信息
func (r *AdminRepository) UpdateLoginFailed(ctx context.Context, id int64, maxAttempts int, lockDuration time.Duration) error {
	var admin model.Admin
	if err := r.db.WithContext(ctx).First(&admin, id).Error; err != nil {
		return err
	}

	updates := map[string]interface{}{
		"login_attempts": admin.LoginAttempts + 1,
	}

	// 达到最大尝试次数，锁定账户
	if admin.LoginAttempts+1 >= maxAttempts {
		lockedUntil := time.Now().Add(lockDuration).UnixMilli()
		updates["locked_until"] = lockedUntil
	}

	return r.db.WithContext(ctx).Model(&model.Admin{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// IsLocked 检查账户是否被锁定
func (r *AdminRepository) IsLocked(ctx context.Context, id int64) (bool, error) {
	var admin model.Admin
	if err := r.db.WithContext(ctx).Select("locked_until").First(&admin, id).Error; err != nil {
		return false, err
	}

	if admin.LockedUntil == nil {
		return false, nil
	}

	return *admin.LockedUntil > time.Now().UnixMilli(), nil
}

// Count 统计管理员数量
func (r *AdminRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Admin{}).Count(&count).Error
	return count, err
}
