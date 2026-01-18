package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// 集成测试环境
type testEnv struct {
	db      *gorm.DB
	engine  *gin.Engine
	token   string
	adminID int64
}

func setupTestEnv(t *testing.T) *testEnv {
	gin.SetMode(gin.TestMode)

	// 创建内存数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 迁移
	err = db.AutoMigrate(
		&model.Admin{},
		&model.AuditLog{},
		&model.MarketConfig{},
		&model.SystemConfig{},
		&model.ConfigVersion{},
		&model.DailyStats{},
	)
	require.NoError(t, err)

	// 创建测试管理员
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	admin := &model.Admin{
		Username:     "admin",
		PasswordHash: string(hash),
		Nickname:     "Test Admin",
		Email:        "admin@test.com",
		Role:         model.RoleSuperAdmin,
		Status:       model.AdminStatusActive,
		CreatedBy:    1,
		UpdatedBy:    1,
	}
	db.Create(admin)

	// 初始化仓储
	adminRepo := repository.NewAdminRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	statsRepo := repository.NewStatsRepository(db)

	// 初始化服务
	authCfg := &service.AuthServiceConfig{
		JWTSecret:      "test-secret-key",
		JWTExpireHours: 24,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}
	authSvc := service.NewAuthService(adminRepo, auditRepo, authCfg)
	adminSvc := service.NewAdminService(adminRepo, auditRepo)
	marketSvc := service.NewMarketService(marketRepo, configRepo, auditRepo)
	statsSvc := service.NewStatsService(statsRepo, adminRepo, marketRepo)
	configSvc := service.NewConfigService(configRepo, auditRepo)
	auditSvc := service.NewAuditService(auditRepo)

	// 初始化处理器
	authHandler := handler.NewAuthHandler(authSvc)
	adminHandler := handler.NewAdminHandler(adminSvc)
	marketHandler := handler.NewMarketHandler(marketSvc)
	statsHandler := handler.NewStatsHandler(statsSvc)
	configHandler := handler.NewConfigHandler(configSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)

	// 初始化中间件
	authMiddleware := middleware.NewAuthMiddleware(authSvc)

	// 创建路由 (inline setup to avoid import cycle)
	engine := gin.New()

	// 健康检查
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API v1
	v1 := engine.Group("/admin/v1")
	{
		// 认证相关 (无需登录)
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
		}

		// 需要登录的接口
		authenticated := v1.Group("")
		authenticated.Use(authMiddleware.Required())
		{
			// 认证相关 (需要登录)
			authGroup := authenticated.Group("/auth")
			{
				authGroup.POST("/logout", authHandler.Logout)
				authGroup.PUT("/password", authHandler.ChangePassword)
				authGroup.GET("/profile", authHandler.GetProfile)
			}

			// 管理员管理
			admins := authenticated.Group("/admins")
			{
				admins.GET("", middleware.RequirePermission("admin:read"), adminHandler.List)
				admins.GET("/:id", middleware.RequirePermission("admin:read"), adminHandler.Get)
				admins.POST("", middleware.RequirePermission("admin:write"), adminHandler.Create)
				admins.PUT("/:id", middleware.RequirePermission("admin:write"), adminHandler.Update)
				admins.DELETE("/:id", middleware.RequirePermission("admin:write"), adminHandler.Delete)
				admins.PUT("/:id/password", middleware.RequirePermission("admin:write"), adminHandler.ResetPassword)
			}

			// 市场配置管理
			markets := authenticated.Group("/markets")
			{
				markets.GET("", middleware.RequirePermission("market:read"), marketHandler.List)
				markets.GET("/active", middleware.RequirePermission("market:read"), marketHandler.GetAllActive)
				markets.GET("/:id", middleware.RequirePermission("market:read"), marketHandler.Get)
				markets.GET("/symbol/:symbol", middleware.RequirePermission("market:read"), marketHandler.GetBySymbol)
				markets.POST("", middleware.RequirePermission("market:write"), marketHandler.Create)
				markets.PUT("/:id", middleware.RequirePermission("market:write"), marketHandler.Update)
				markets.PUT("/:id/status", middleware.RequirePermission("market:write"), marketHandler.UpdateStatus)
				markets.DELETE("/:id", middleware.RequirePermission("market:write"), marketHandler.Delete)
			}

			// 统计查询
			stats := authenticated.Group("/stats")
			stats.Use(middleware.RequirePermission("stats:read"))
			{
				stats.GET("/overview", statsHandler.GetOverview)
				stats.GET("/daily", statsHandler.GetDailyStats)
				stats.GET("/daily/list", statsHandler.GetDailyStatsList)
				stats.GET("/trading", statsHandler.GetTradingStats)
				stats.GET("/users", statsHandler.GetUserStats)
				stats.GET("/settlements", statsHandler.GetSettlementStats)
			}

			// 系统配置管理
			configs := authenticated.Group("/configs")
			{
				configs.GET("", middleware.RequirePermission("config:read"), configHandler.List)
				configs.GET("/version", middleware.RequirePermission("config:read"), configHandler.GetCurrentVersion)
				configs.GET("/versions", middleware.RequirePermission("config:read"), configHandler.GetVersionHistory)
				configs.GET("/category/:category", middleware.RequirePermission("config:read"), configHandler.GetByCategory)
				configs.GET("/key/:key", middleware.RequirePermission("config:read"), configHandler.GetByKey)
				configs.GET("/:id", middleware.RequirePermission("config:read"), configHandler.Get)
				configs.POST("", middleware.RequirePermission("config:write"), configHandler.Create)
				configs.PUT("/batch", middleware.RequirePermission("config:write"), configHandler.BatchUpdate)
				configs.PUT("/:id", middleware.RequirePermission("config:write"), configHandler.Update)
				configs.DELETE("/:id", middleware.RequirePermission("config:write"), configHandler.Delete)
			}

			// 审计日志查询
			audits := authenticated.Group("/audits")
			audits.Use(middleware.RequirePermission("audit:read"))
			{
				audits.GET("", auditHandler.List)
				audits.GET("/actions", auditHandler.GetActions)
				audits.GET("/resource-types", auditHandler.GetResourceTypes)
				audits.GET("/export", auditHandler.Export)
				audits.GET("/admin/:admin_id", auditHandler.GetByAdmin)
				audits.GET("/resource/:resource_type/:resource_id", auditHandler.GetByResource)
				audits.GET("/:id", auditHandler.Get)
			}
		}
	}

	// 登录获取 token
	ctx := context.Background()
	loginResp, err := authSvc.Login(ctx, &service.LoginRequest{
		Username: "admin",
		Password: "password123",
	}, "127.0.0.1", "TestAgent")
	require.NoError(t, err)

	return &testEnv{
		db:      db,
		engine:  engine,
		token:   loginResp.Token,
		adminID: loginResp.Admin.ID,
	}
}

func (e *testEnv) request(method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}

	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	return w
}

func (e *testEnv) requestNoAuth(method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	e.engine.ServeHTTP(w, req)
	return w
}

// ========== Health Check Tests ==========

func TestHealthCheck(t *testing.T) {
	env := setupTestEnv(t)

	w := env.requestNoAuth("GET", "/health", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ok", resp["status"])
}

// ========== Auth Handler Tests ==========

func TestAuthHandler_Login_Success(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]string{
		"username": "admin",
		"password": "password123",
	}

	w := env.requestNoAuth("POST", "/admin/v1/auth/login", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotNil(t, resp["data"])

	data := resp["data"].(map[string]interface{})
	assert.NotEmpty(t, data["token"])
	assert.NotNil(t, data["admin"])
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	}

	w := env.requestNoAuth("POST", "/admin/v1/auth/login", body)
	// Handler uses Error() which returns HTTP 200 with error code in body
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(401), resp["code"])
}

func TestAuthHandler_Login_UserNotFound(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]string{
		"username": "nonexistent",
		"password": "password123",
	}

	w := env.requestNoAuth("POST", "/admin/v1/auth/login", body)
	// Handler uses Error() which returns HTTP 200 with error code in body
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(401), resp["code"])
}

func TestAuthHandler_GetProfile(t *testing.T) {
	env := setupTestEnv(t)

	w := env.request("GET", "/admin/v1/auth/profile", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "admin", data["username"])
}

func TestAuthHandler_Logout(t *testing.T) {
	env := setupTestEnv(t)

	w := env.request("POST", "/admin/v1/auth/logout", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_ChangePassword(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]string{
		"old_password": "password123",
		"new_password": "newpassword456",
	}

	w := env.request("PUT", "/admin/v1/auth/password", body)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ========== Admin Handler Tests ==========

func TestAdminHandler_List(t *testing.T) {
	env := setupTestEnv(t)

	w := env.request("GET", "/admin/v1/admins", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	assert.NotEmpty(t, data)
}

func TestAdminHandler_Create(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]interface{}{
		"username": "newadmin",
		"password": "password123",
		"nickname": "New Admin",
		"email":    "newadmin@test.com",
		"role":     "operator",
	}

	w := env.request("POST", "/admin/v1/admins", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "newadmin", data["username"])
}

func TestAdminHandler_Get(t *testing.T) {
	env := setupTestEnv(t)

	w := env.request("GET", "/admin/v1/admins/1", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "admin", data["username"])
}

func TestAdminHandler_Update(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]interface{}{
		"nickname": "Updated Nickname",
		"email":    "updated@test.com",
	}

	w := env.request("PUT", "/admin/v1/admins/1", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "Updated Nickname", data["nickname"])
}

// ========== Market Handler Tests ==========

func TestMarketHandler_Create(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]interface{}{
		"symbol":           "BTC-USDT",
		"base_token":       "BTC",
		"quote_token":      "USDT",
		"base_token_addr":  "0x1234567890123456789012345678901234567890",
		"quote_token_addr": "0x0987654321098765432109876543210987654321",
		"price_decimals":   2,
		"size_decimals":    8,
		"min_size":         "0.0001",
		"max_size":         "1000",
		"min_notional":     "10",
		"tick_size":        "0.01",
		"maker_fee":        "0.001",
		"taker_fee":        "0.002",
	}

	w := env.request("POST", "/admin/v1/markets", body)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "BTC-USDT", data["symbol"])
}

func TestMarketHandler_List(t *testing.T) {
	env := setupTestEnv(t)

	// 先创建交易对
	body := map[string]interface{}{
		"symbol":           "ETH-USDT",
		"base_token":       "ETH",
		"quote_token":      "USDT",
		"base_token_addr":  "0x1234567890123456789012345678901234567890",
		"quote_token_addr": "0x0987654321098765432109876543210987654321",
		"min_size":         "0.01",
		"max_size":         "100",
		"min_notional":     "10",
		"tick_size":        "0.01",
		"maker_fee":        "0.001",
		"taker_fee":        "0.002",
	}
	env.request("POST", "/admin/v1/markets", body)

	// 列表查询
	w := env.request("GET", "/admin/v1/markets", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMarketHandler_GetBySymbol(t *testing.T) {
	env := setupTestEnv(t)

	// 先创建交易对
	body := map[string]interface{}{
		"symbol":           "SOL-USDT",
		"base_token":       "SOL",
		"quote_token":      "USDT",
		"base_token_addr":  "0x1234567890123456789012345678901234567890",
		"quote_token_addr": "0x0987654321098765432109876543210987654321",
		"min_size":         "0.1",
		"max_size":         "1000",
		"min_notional":     "10",
		"tick_size":        "0.01",
		"maker_fee":        "0.001",
		"taker_fee":        "0.002",
	}
	env.request("POST", "/admin/v1/markets", body)

	// 根据符号查询
	w := env.request("GET", "/admin/v1/markets/symbol/SOL-USDT", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "SOL-USDT", data["symbol"])
}

func TestMarketHandler_UpdateStatus(t *testing.T) {
	env := setupTestEnv(t)

	// 先创建交易对
	createBody := map[string]interface{}{
		"symbol":           "ARB-USDT",
		"base_token":       "ARB",
		"quote_token":      "USDT",
		"base_token_addr":  "0x1234567890123456789012345678901234567890",
		"quote_token_addr": "0x0987654321098765432109876543210987654321",
		"min_size":         "1",
		"max_size":         "10000",
		"min_notional":     "10",
		"tick_size":        "0.0001",
		"maker_fee":        "0.001",
		"taker_fee":        "0.002",
	}
	createResp := env.request("POST", "/admin/v1/markets", createBody)
	require.Equal(t, http.StatusOK, createResp.Code)

	var created map[string]interface{}
	json.Unmarshal(createResp.Body.Bytes(), &created)
	data := created["data"].(map[string]interface{})
	id := int(data["id"].(float64))

	// 更新状态
	statusBody := map[string]interface{}{
		"status":          "active",
		"trading_enabled": true,
	}

	w := env.request("PUT", "/admin/v1/markets/"+string(rune('0'+id))+"/status", statusBody)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ========== Stats Handler Tests ==========

func TestStatsHandler_GetOverview(t *testing.T) {
	// Skip: uses SumByDateRange which requires PostgreSQL-specific SQL
	t.Skip("Skipping: requires PostgreSQL-specific SQL syntax")
}

func TestStatsHandler_GetDailyStats(t *testing.T) {
	env := setupTestEnv(t)

	// 创建测试数据
	stats := &model.DailyStats{
		StatDate:      time.Now().Format("2006-01-02"),
		TradeCount:    100,
		TradeVolume:   "1000000",
		OrderCount:    500,
		MakerFeeTotal: "100",
		TakerFeeTotal: "200",
	}
	env.db.Create(stats)

	w := env.request("GET", "/admin/v1/stats/daily?date="+time.Now().Format("2006-01-02"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestStatsHandler_GetTradingStats(t *testing.T) {
	// Skip: uses SumByDateRange which requires PostgreSQL-specific SQL
	t.Skip("Skipping: requires PostgreSQL-specific SQL syntax")
}

// ========== Authorization Tests ==========

func TestUnauthorizedAccess(t *testing.T) {
	env := setupTestEnv(t)

	// 不带 token 访问需要认证的接口
	w := env.requestNoAuth("GET", "/admin/v1/admins", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestInvalidToken(t *testing.T) {
	env := setupTestEnv(t)
	env.token = "invalid-token"

	w := env.request("GET", "/admin/v1/admins", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ========== Concurrent Access Tests ==========

func TestConcurrentRequests(t *testing.T) {
	env := setupTestEnv(t)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			w := env.request("GET", "/admin/v1/auth/profile", nil)
			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
