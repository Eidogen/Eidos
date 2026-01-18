package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

func setupAdminTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&model.Admin{}, &model.AuditLog{})
	require.NoError(t, err)

	return db
}

func TestAdminService_Create_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()
	req := &CreateAdminRequest{
		Username:   "newadmin",
		Password:   "password123",
		Nickname:   "New Admin",
		Email:      "newadmin@test.com",
		Phone:      "1234567890",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	admin, err := svc.Create(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, admin)
	assert.Equal(t, "newadmin", admin.Username)
	assert.Equal(t, "New Admin", admin.Nickname)
	assert.Equal(t, model.RoleOperator, admin.Role)
	assert.Equal(t, model.AdminStatusActive, admin.Status)
	assert.Empty(t, admin.PasswordHash) // 密码应被清除
}

func TestAdminService_Create_DuplicateUsername(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建第一个管理员
	req1 := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	_, err := svc.Create(ctx, req1)
	require.NoError(t, err)

	// 尝试创建重复用户名
	req2 := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password456",
		Nickname:   "Admin 2",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	admin, err := svc.Create(ctx, req2)
	assert.Error(t, err)
	assert.Nil(t, admin)
	assert.Contains(t, err.Error(), "用户名已存在")
}

func TestAdminService_Update_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Email:      "admin@test.com",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 更新管理员
	updateReq := &UpdateAdminRequest{
		ID:         created.ID,
		Nickname:   "Updated Admin",
		Email:      "updated@test.com",
		Phone:      "9876543210",
		Role:       model.RoleSuperAdmin,
		OperatorID: 1,
	}

	updated, err := svc.Update(ctx, updateReq)
	require.NoError(t, err)
	assert.NotNil(t, updated)
	assert.Equal(t, "Updated Admin", updated.Nickname)
	assert.Equal(t, "updated@test.com", updated.Email)
	assert.Equal(t, "9876543210", updated.Phone)
	assert.Equal(t, model.RoleSuperAdmin, updated.Role)
}

func TestAdminService_Update_NotFound(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	updateReq := &UpdateAdminRequest{
		ID:         9999,
		Nickname:   "Updated Admin",
		OperatorID: 1,
	}

	updated, err := svc.Update(ctx, updateReq)
	assert.Error(t, err)
	assert.Nil(t, updated)
	assert.Contains(t, err.Error(), "管理员不存在")
}

func TestAdminService_Update_Status(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)
	assert.Equal(t, model.AdminStatusActive, created.Status)

	// 禁用管理员
	disabledStatus := model.AdminStatusDisabled
	updateReq := &UpdateAdminRequest{
		ID:         created.ID,
		Status:     &disabledStatus,
		OperatorID: 1,
	}

	updated, err := svc.Update(ctx, updateReq)
	require.NoError(t, err)
	assert.Equal(t, model.AdminStatusDisabled, updated.Status)
}

func TestAdminService_Delete_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 删除管理员（使用不同的 operatorID）
	err = svc.Delete(ctx, created.ID, 999)
	require.NoError(t, err)

	// 验证已删除
	deleted, err := svc.GetByID(ctx, created.ID)
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestAdminService_Delete_Self(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 尝试删除自己
	err = svc.Delete(ctx, created.ID, created.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不能删除自己")
}

func TestAdminService_Delete_NotFound(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	err := svc.Delete(ctx, 9999, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "管理员不存在")
}

func TestAdminService_GetByID_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "password123",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 获取管理员
	admin, err := svc.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, admin)
	assert.Equal(t, "admin", admin.Username)
	assert.Empty(t, admin.PasswordHash) // 密码应被清除
}

func TestAdminService_GetByID_NotFound(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	admin, err := svc.GetByID(ctx, 9999)
	assert.NoError(t, err)
	assert.Nil(t, admin)
}

func TestAdminService_List_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建多个管理员
	for i := 0; i < 5; i++ {
		createReq := &CreateAdminRequest{
			Username:   "admin" + string(rune('0'+i)),
			Password:   "password123",
			Nickname:   "Admin " + string(rune('0'+i)),
			Role:       model.RoleOperator,
			OperatorID: 1,
		}
		_, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
	}

	// 列表查询
	page := &model.Pagination{
		Page:     1,
		PageSize: 10,
	}

	admins, err := svc.List(ctx, page)
	require.NoError(t, err)
	assert.Len(t, admins, 5)
	assert.Equal(t, int64(5), page.Total)

	// 验证密码已清除
	for _, admin := range admins {
		assert.Empty(t, admin.PasswordHash)
	}
}

func TestAdminService_List_Pagination(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建 10 个管理员
	for i := 0; i < 10; i++ {
		createReq := &CreateAdminRequest{
			Username:   "admin" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
			Password:   "password123",
			Nickname:   "Admin " + string(rune('0'+i)),
			Role:       model.RoleOperator,
			OperatorID: 1,
		}
		_, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
	}

	// 第一页
	page1 := &model.Pagination{
		Page:     1,
		PageSize: 3,
	}

	admins1, err := svc.List(ctx, page1)
	require.NoError(t, err)
	assert.Len(t, admins1, 3)
	assert.Equal(t, int64(10), page1.Total)

	// 第二页
	page2 := &model.Pagination{
		Page:     2,
		PageSize: 3,
	}

	admins2, err := svc.List(ctx, page2)
	require.NoError(t, err)
	assert.Len(t, admins2, 3)
}

func TestAdminService_ResetPassword_Success(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "admin",
		Password:   "oldpassword",
		Nickname:   "Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 重置密码
	err = svc.ResetPassword(ctx, created.ID, "newpassword", 1)
	require.NoError(t, err)

	// 验证审计日志
	var auditLogs []model.AuditLog
	db.Find(&auditLogs)
	assert.NotEmpty(t, auditLogs)
}

func TestAdminService_ResetPassword_NotFound(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	err := svc.ResetPassword(ctx, 9999, "newpassword", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "管理员不存在")
}

func TestPagination_GetOffset(t *testing.T) {
	tests := []struct {
		name     string
		page     model.Pagination
		expected int
	}{
		{"page 1", model.Pagination{Page: 1, PageSize: 10}, 0},
		{"page 2", model.Pagination{Page: 2, PageSize: 10}, 10},
		{"page 3", model.Pagination{Page: 3, PageSize: 20}, 40},
		{"page 0 (invalid)", model.Pagination{Page: 0, PageSize: 10}, 0},
		{"negative page", model.Pagination{Page: -1, PageSize: 10}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.page.GetOffset()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPagination_GetLimit(t *testing.T) {
	tests := []struct {
		name     string
		page     model.Pagination
		expected int
	}{
		{"normal", model.Pagination{PageSize: 10}, 10},
		{"zero (default)", model.Pagination{PageSize: 0}, 20},
		{"negative (default)", model.Pagination{PageSize: -1}, 20},
		{"max exceeded", model.Pagination{PageSize: 200}, 100},
		{"max exactly", model.Pagination{PageSize: 100}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.page.GetLimit()
			assert.Equal(t, tt.expected, result)
		})
	}
}
