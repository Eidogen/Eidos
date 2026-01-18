package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// 测试用数据库设置
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 自动迁移
	err = db.AutoMigrate(&model.Admin{}, &model.AuditLog{})
	require.NoError(t, err)

	return db
}

func createTestAdmin(t *testing.T, db *gorm.DB, username, password string, role model.Role) *model.Admin {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)

	admin := &model.Admin{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     "Test Admin",
		Email:        username + "@test.com",
		Role:         role,
		Status:       model.AdminStatusActive,
		CreatedBy:    1,
		UpdatedBy:    1,
	}

	err = db.Create(admin).Error
	require.NoError(t, err)

	return admin
}

func TestAuthService_Login_Success(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	// 创建测试管理员
	createTestAdmin(t, db, "admin", "password123", model.RoleSuperAdmin)

	// 测试登录
	ctx := context.Background()
	req := &LoginRequest{
		Username: "admin",
		Password: "password123",
	}

	resp, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "admin", resp.Admin.Username)
	assert.Empty(t, resp.Admin.PasswordHash) // 密码应被清除
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	createTestAdmin(t, db, "admin", "password123", model.RoleSuperAdmin)

	ctx := context.Background()
	req := &LoginRequest{
		Username: "admin",
		Password: "wrongpassword",
	}

	resp, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "用户名或密码错误")
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	ctx := context.Background()
	req := &LoginRequest{
		Username: "nonexistent",
		Password: "password123",
	}

	resp, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "用户名或密码错误")
}

func TestAuthService_Login_DisabledAccount(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	admin := createTestAdmin(t, db, "admin", "password123", model.RoleSuperAdmin)
	admin.Status = model.AdminStatusDisabled
	db.Save(admin)

	ctx := context.Background()
	req := &LoginRequest{
		Username: "admin",
		Password: "password123",
	}

	resp, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "账户已禁用")
}

func TestAuthService_Login_LockedAccount(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	admin := createTestAdmin(t, db, "admin", "password123", model.RoleSuperAdmin)
	lockedUntil := time.Now().Add(1 * time.Hour).UnixMilli()
	admin.LockedUntil = &lockedUntil
	db.Save(admin)

	ctx := context.Background()
	req := &LoginRequest{
		Username: "admin",
		Password: "password123",
	}

	resp, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "账户已锁定")
}

func TestAuthService_ValidateToken_Success(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	createTestAdmin(t, db, "admin", "password123", model.RoleSuperAdmin)

	ctx := context.Background()
	loginReq := &LoginRequest{
		Username: "admin",
		Password: "password123",
	}

	loginResp, err := svc.Login(ctx, loginReq, "127.0.0.1", "TestAgent")
	require.NoError(t, err)

	// 验证 token
	claims, err := svc.ValidateToken(loginResp.Token)
	require.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, "admin", claims.Username)
	assert.Equal(t, model.RoleSuperAdmin, claims.Role)
}

func TestAuthService_ValidateToken_Invalid(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	claims, err := svc.ValidateToken("invalid-token")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestAuthService_ChangePassword_Success(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	admin := createTestAdmin(t, db, "admin", "oldpassword", model.RoleSuperAdmin)

	ctx := context.Background()
	err := svc.ChangePassword(ctx, admin.ID, "oldpassword", "newpassword")
	require.NoError(t, err)

	// 验证新密码可以登录
	loginReq := &LoginRequest{
		Username: "admin",
		Password: "newpassword",
	}

	resp, err := svc.Login(ctx, loginReq, "127.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAuthService_ChangePassword_WrongOldPassword(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	admin := createTestAdmin(t, db, "admin", "oldpassword", model.RoleSuperAdmin)

	ctx := context.Background()
	err := svc.ChangePassword(ctx, admin.ID, "wrongoldpassword", "newpassword")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "原密码错误")
}

func TestAuthService_HashPassword(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	password := "testpassword"
	hash, err := svc.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	// 验证哈希
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	assert.NoError(t, err)
}

func TestAuthService_Logout(t *testing.T) {
	db := setupTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	ctx := context.Background()
	err := svc.Logout(ctx, 1, "admin", "127.0.0.1", "TestAgent")
	assert.NoError(t, err)

	// 验证审计日志
	var auditLogs []model.AuditLog
	db.Find(&auditLogs)
	assert.Len(t, auditLogs, 1)
	assert.Equal(t, model.AuditActionLogout, auditLogs[0].Action)
}

func TestClaims_Permissions(t *testing.T) {
	claims := &Claims{
		AdminID:     1,
		Username:    "admin",
		Role:        model.RoleSuperAdmin,
		Permissions: model.RolePermissions[model.RoleSuperAdmin],
	}

	assert.Contains(t, claims.Permissions, model.PermMarketRead)
	assert.Contains(t, claims.Permissions, model.PermMarketWrite)
	assert.Contains(t, claims.Permissions, model.PermAdminWrite)
}

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		role     model.Role
		perm     string
		expected bool
	}{
		{model.RoleSuperAdmin, model.PermMarketWrite, true},
		{model.RoleSuperAdmin, model.PermAdminWrite, true},
		{model.RoleOperator, model.PermMarketWrite, true},
		{model.RoleOperator, model.PermAdminWrite, false},
		{model.RoleSupport, model.PermUserRead, true},
		{model.RoleSupport, model.PermMarketWrite, false},
		{model.RoleViewer, model.PermMarketRead, true},
		{model.RoleViewer, model.PermMarketWrite, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_"+tt.perm, func(t *testing.T) {
			result := tt.role.HasPermission(tt.perm)
			assert.Equal(t, tt.expected, result)
		})
	}
}
