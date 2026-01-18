package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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

// raceTestDBCounter 用于生成唯一的数据库名
var raceTestDBCounter int64

// setupRaceTestDB 创建竞态测试用的内存数据库
// 每个测试使用唯一的命名内存数据库，确保测试隔离
func setupRaceTestDB(t *testing.T) *gorm.DB {
	// 使用唯一的命名内存数据库确保测试隔离
	// 格式: file:test_{counter}?mode=memory&cache=shared
	counter := atomic.AddInt64(&raceTestDBCounter, 1)
	dsn := fmt.Sprintf("file:racetest_%d?mode=memory&cache=shared&_busy_timeout=5000", counter)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// 获取底层 SQL DB 并设置连接池
	sqlDB, err := db.DB()
	require.NoError(t, err)

	// 设置单连接模式，防止多个连接看到不同的数据库状态
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	err = db.AutoMigrate(&model.Admin{}, &model.AuditLog{})
	require.NoError(t, err)

	// 清理：测试结束时关闭数据库连接
	t.Cleanup(func() {
		sqlDB.Close()
	})

	return db
}

// createRaceTestAdmin 创建测试管理员（用于竞态测试）
func createRaceTestAdmin(t *testing.T, db *gorm.DB, username, password string, role model.Role) *model.Admin {
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

// TestAuthService_ConcurrentLogin 测试并发登录
func TestAuthService_ConcurrentLogin(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    100, // 高阈值防止锁定
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	// 创建测试管理员
	createRaceTestAdmin(t, db, "concurrent_admin", "password123", model.RoleSuperAdmin)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 20
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := &LoginRequest{
				Username: "concurrent_admin",
				Password: "password123",
			}

			resp, err := svc.Login(ctx, req, fmt.Sprintf("127.0.0.%d", id), "TestAgent")
			if err == nil && resp != nil && resp.Token != "" {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有登录应该成功
	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAuthService_ConcurrentValidateToken 测试并发验证Token
func TestAuthService_ConcurrentValidateToken(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	// 创建测试管理员并登录获取 token
	createRaceTestAdmin(t, db, "token_admin", "password123", model.RoleSuperAdmin)

	ctx := context.Background()
	loginReq := &LoginRequest{
		Username: "token_admin",
		Password: "password123",
	}

	loginResp, err := svc.Login(ctx, loginReq, "127.0.0.1", "TestAgent")
	require.NoError(t, err)

	token := loginResp.Token

	// 并发验证 token
	var wg sync.WaitGroup
	numGoroutines := 50
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			claims, err := svc.ValidateToken(token)
			if err == nil && claims != nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// 所有验证应该成功
	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAuthService_ConcurrentLoginFailure 测试并发登录失败场景
func TestAuthService_ConcurrentLoginFailure(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	cfg := &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    100, // 高阈值防止锁定
		LockDuration:   30 * time.Minute,
	}

	svc := NewAuthService(adminRepo, auditRepo, cfg)

	createRaceTestAdmin(t, db, "fail_admin", "password123", model.RoleSuperAdmin)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 20
	var failCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := &LoginRequest{
				Username: "fail_admin",
				Password: "wrongpassword",
			}

			_, err := svc.Login(ctx, req, "127.0.0.1", "TestAgent")
			if err != nil {
				atomic.AddInt64(&failCount, 1)
			}
		}()
	}

	wg.Wait()

	// 所有登录应该失败
	assert.Equal(t, int64(numGoroutines), failCount)
}

// TestAdminService_ConcurrentCreate 测试并发创建管理员
func TestAdminService_ConcurrentCreate(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 20
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := &CreateAdminRequest{
				Username:   fmt.Sprintf("admin_%d", id),
				Password:   "password123",
				Nickname:   fmt.Sprintf("Admin %d", id),
				Email:      fmt.Sprintf("admin%d@test.com", id),
				Role:       model.RoleOperator,
				OperatorID: 1,
			}

			admin, err := svc.Create(ctx, req)
			if err == nil && admin != nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有创建应该成功
	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAdminService_ConcurrentCreateSameUsername 测试并发创建相同用户名
func TestAdminService_ConcurrentCreateSameUsername(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 10
	var successCount int64
	var failCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			req := &CreateAdminRequest{
				Username:   "duplicate_admin",
				Password:   "password123",
				Nickname:   fmt.Sprintf("Admin %d", id),
				Email:      fmt.Sprintf("admin%d@test.com", id),
				Role:       model.RoleOperator,
				OperatorID: 1,
			}

			admin, err := svc.Create(ctx, req)
			if err == nil && admin != nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 只有一个创建应该成功
	assert.Equal(t, int64(1), successCount)
	assert.Equal(t, int64(numGoroutines-1), failCount)
}

// TestAdminService_ConcurrentUpdate 测试并发更新管理员
func TestAdminService_ConcurrentUpdate(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建一个管理员用于更新
	createReq := &CreateAdminRequest{
		Username:   "update_admin",
		Password:   "password123",
		Nickname:   "Update Admin",
		Email:      "update@test.com",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 20
	var updateCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			updateReq := &UpdateAdminRequest{
				ID:         created.ID,
				Nickname:   fmt.Sprintf("Updated Admin %d", id),
				Email:      fmt.Sprintf("updated%d@test.com", id),
				OperatorID: 1,
			}

			_, err := svc.Update(ctx, updateReq)
			if err == nil {
				atomic.AddInt64(&updateCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有更新应该成功
	assert.Equal(t, int64(numGoroutines), updateCount)
}

// TestAdminService_ConcurrentGetByID 测试并发读取管理员
func TestAdminService_ConcurrentGetByID(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "read_admin",
		Password:   "password123",
		Nickname:   "Read Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 50
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			admin, err := svc.GetByID(ctx, created.ID)
			if err == nil && admin != nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAdminService_ConcurrentList 测试并发列表查询
func TestAdminService_ConcurrentList(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建多个管理员
	for i := 0; i < 10; i++ {
		createReq := &CreateAdminRequest{
			Username:   fmt.Sprintf("list_admin_%d", i),
			Password:   "password123",
			Nickname:   fmt.Sprintf("List Admin %d", i),
			Role:       model.RoleOperator,
			OperatorID: 1,
		}
		_, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	numGoroutines := 30
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			page := &model.Pagination{
				Page:     (id % 3) + 1,
				PageSize: 5,
			}

			admins, err := svc.List(ctx, page)
			if err == nil && admins != nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAdminService_ConcurrentMixedOperations 测试并发混合操作
func TestAdminService_ConcurrentMixedOperations(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 预先创建一些管理员
	var adminIDs []int64
	for i := 0; i < 5; i++ {
		createReq := &CreateAdminRequest{
			Username:   fmt.Sprintf("mixed_admin_%d", i),
			Password:   "password123",
			Nickname:   fmt.Sprintf("Mixed Admin %d", i),
			Role:       model.RoleOperator,
			OperatorID: 1,
		}
		admin, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
		adminIDs = append(adminIDs, admin.ID)
	}

	var wg sync.WaitGroup
	numGoroutines := 30

	// 并发执行读取、更新、创建操作
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			switch id % 3 {
			case 0:
				// 读取操作
				adminID := adminIDs[id%len(adminIDs)]
				svc.GetByID(ctx, adminID)

			case 1:
				// 更新操作
				adminID := adminIDs[id%len(adminIDs)]
				updateReq := &UpdateAdminRequest{
					ID:         adminID,
					Nickname:   fmt.Sprintf("Updated by %d", id),
					OperatorID: 1,
				}
				svc.Update(ctx, updateReq)

			case 2:
				// 列表查询
				page := &model.Pagination{
					Page:     1,
					PageSize: 10,
				}
				svc.List(ctx, page)
			}
		}(i)
	}

	wg.Wait()
}

// TestAuthService_ConcurrentChangePassword 测试并发修改密码
func TestAuthService_ConcurrentChangePassword(t *testing.T) {
	db := setupRaceTestDB(t)
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
	admin := createRaceTestAdmin(t, db, "password_admin", "oldpassword", model.RoleSuperAdmin)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 10
	var successCount int64
	var failCount int64

	// 并发修改密码（同一用户）
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := svc.ChangePassword(ctx, admin.ID, "oldpassword", fmt.Sprintf("newpassword%d", id))
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 至少一个应该成功
	assert.GreaterOrEqual(t, successCount, int64(1))
}

// TestAuthService_ConcurrentLogout 测试并发登出
func TestAuthService_ConcurrentLogout(t *testing.T) {
	db := setupRaceTestDB(t)
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
	var wg sync.WaitGroup
	numGoroutines := 20
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := svc.Logout(ctx, int64(id+1), fmt.Sprintf("admin_%d", id), "127.0.0.1", "TestAgent")
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有登出操作应该成功
	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAdminService_ConcurrentResetPassword 测试并发重置密码
func TestAdminService_ConcurrentResetPassword(t *testing.T) {
	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewAdminService(adminRepo, auditRepo)

	ctx := context.Background()

	// 创建管理员
	createReq := &CreateAdminRequest{
		Username:   "reset_admin",
		Password:   "password123",
		Nickname:   "Reset Admin",
		Role:       model.RoleOperator,
		OperatorID: 1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			err := svc.ResetPassword(ctx, created.ID, fmt.Sprintf("newpassword%d", id), 1)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有重置操作应该成功
	assert.Equal(t, int64(numGoroutines), successCount)
}

// TestAuditLog_ConcurrentCreation 测试并发创建审计日志
func TestAuditLog_ConcurrentCreation(t *testing.T) {
	db := setupRaceTestDB(t)
	auditRepo := repository.NewAuditLogRepository(db)

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 50
	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			auditLog := &model.AuditLog{
				AdminID:       int64(id),
				AdminUsername: fmt.Sprintf("admin_%d", id),
				IPAddress:     "127.0.0.1",
				UserAgent:     "TestAgent",
				Action:        model.AuditActionLogin,
				ResourceType:  model.ResourceTypeAdmin,
				ResourceID:    fmt.Sprintf("resource_%d", id),
				Description:   fmt.Sprintf("Test operation %d", id),
				Status:        model.AuditStatusSuccess,
			}

			err := auditRepo.Create(ctx, auditLog)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 所有创建应该成功
	assert.Equal(t, int64(numGoroutines), successCount)

	// 验证记录数
	var count int64
	db.Model(&model.AuditLog{}).Count(&count)
	assert.Equal(t, int64(numGoroutines), count)
}

// TestHighConcurrencyStress 高并发压力测试
func TestHighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	db := setupRaceTestDB(t)
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	adminSvc := NewAdminService(adminRepo, auditRepo)
	authSvc := NewAuthService(adminRepo, auditRepo, &AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    1000,
		LockDuration:   30 * time.Minute,
	})

	ctx := context.Background()

	// 创建测试用户
	admin := createRaceTestAdmin(t, db, "stress_admin", "password123", model.RoleSuperAdmin)

	var wg sync.WaitGroup
	numGoroutines := 100
	operationsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				switch (goroutineID + j) % 5 {
				case 0:
					// 登录
					req := &LoginRequest{
						Username: "stress_admin",
						Password: "password123",
					}
					authSvc.Login(ctx, req, "127.0.0.1", "TestAgent")

				case 1:
					// 获取用户
					adminSvc.GetByID(ctx, admin.ID)

				case 2:
					// 列表查询
					page := &model.Pagination{
						Page:     1,
						PageSize: 10,
					}
					adminSvc.List(ctx, page)

				case 3:
					// 更新用户
					updateReq := &UpdateAdminRequest{
						ID:         admin.ID,
						Nickname:   fmt.Sprintf("Stress %d-%d", goroutineID, j),
						OperatorID: 1,
					}
					adminSvc.Update(ctx, updateReq)

				case 4:
					// 创建审计日志
					authSvc.Logout(ctx, int64(goroutineID), fmt.Sprintf("user_%d", goroutineID), "127.0.0.1", "TestAgent")
				}
			}
		}(i)
	}

	wg.Wait()
}
