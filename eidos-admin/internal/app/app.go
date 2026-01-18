package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/config"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/router"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
)

// App 应用
type App struct {
	cfg         *config.Config
	db          *gorm.DB
	redisClient redis.UniversalClient
	httpServer  *http.Server
	engine      *gin.Engine
}

// New 创建应用
func New(cfg *config.Config, db *gorm.DB, redisClient redis.UniversalClient) *App {
	return &App{
		cfg:         cfg,
		db:          db,
		redisClient: redisClient,
	}
}

// Init 初始化应用
func (a *App) Init() error {
	// 设置 Gin 模式
	if a.cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建 Gin 引擎
	a.engine = gin.New()

	// 添加中间件
	a.engine.Use(gin.Recovery())
	a.engine.Use(middleware.Logger())
	a.engine.Use(middleware.CORS())

	// 初始化存储层
	repos := a.initRepositories()

	// 初始化服务层
	services := a.initServices(repos)

	// 初始化处理器
	handlers := a.initHandlers(services)

	// 初始化认证中间件
	authMiddleware := middleware.NewAuthMiddleware(services.Auth)

	// 设置路由
	router.SetupRouter(a.engine, handlers, authMiddleware)

	// 创建 HTTP 服务器
	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler:      a.engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("app initialized",
		zap.Int("port", a.cfg.Server.Port),
		zap.String("mode", a.cfg.Server.Mode))

	return nil
}

// repositories 存储层
type repositories struct {
	Admin        *repository.AdminRepository
	MarketConfig *repository.MarketConfigRepository
	SystemConfig *repository.SystemConfigRepository
	AuditLog     *repository.AuditLogRepository
	Stats        *repository.StatsRepository
}

// initRepositories 初始化存储层
func (a *App) initRepositories() *repositories {
	return &repositories{
		Admin:        repository.NewAdminRepository(a.db),
		MarketConfig: repository.NewMarketConfigRepository(a.db),
		SystemConfig: repository.NewSystemConfigRepository(a.db),
		AuditLog:     repository.NewAuditLogRepository(a.db),
		Stats:        repository.NewStatsRepository(a.db),
	}
}

// services 服务层
type services struct {
	Auth   *service.AuthService
	Admin  *service.AdminService
	Market *service.MarketService
	Config *service.ConfigService
	Stats  *service.StatsService
	Audit  *service.AuditService
}

// initServices 初始化服务层
func (a *App) initServices(repos *repositories) *services {
	// 创建审计服务 (其他服务依赖)
	auditSvc := service.NewAuditService(repos.AuditLog)

	// 创建认证服务
	authCfg := &service.AuthServiceConfig{
		JWTSecret:      a.cfg.JWT.Secret,
		JWTExpireHours: a.cfg.JWT.ExpireHours,
		MaxAttempts:    5,
		LockDuration:   30 * time.Minute,
	}
	authSvc := service.NewAuthService(repos.Admin, repos.AuditLog, authCfg)

	// 创建管理员服务
	adminSvc := service.NewAdminService(repos.Admin, repos.AuditLog)

	// 创建市场配置服务
	marketSvc := service.NewMarketService(repos.MarketConfig, repos.SystemConfig, repos.AuditLog)

	// 创建系统配置服务
	configSvc := service.NewConfigService(repos.SystemConfig, repos.AuditLog)

	// 创建统计服务
	statsSvc := service.NewStatsService(repos.Stats, repos.Admin, repos.MarketConfig)

	return &services{
		Auth:   authSvc,
		Admin:  adminSvc,
		Market: marketSvc,
		Config: configSvc,
		Stats:  statsSvc,
		Audit:  auditSvc,
	}
}

// initHandlers 初始化处理器
func (a *App) initHandlers(svcs *services) *router.Handlers {
	return &router.Handlers{
		Auth:   handler.NewAuthHandler(svcs.Auth),
		Admin:  handler.NewAdminHandler(svcs.Admin),
		Market: handler.NewMarketHandler(svcs.Market),
		Stats:  handler.NewStatsHandler(svcs.Stats),
		Config: handler.NewConfigHandler(svcs.Config),
		Audit:  handler.NewAuditHandler(svcs.Audit),
	}
}

// Run 运行应用
func (a *App) Run() error {
	logger.Info("starting HTTP server", zap.String("addr", a.httpServer.Addr))
	return a.httpServer.ListenAndServe()
}

// Shutdown 关闭应用
func (a *App) Shutdown(ctx context.Context) error {
	logger.Info("shutting down HTTP server")
	return a.httpServer.Shutdown(ctx)
}

// Engine 获取 Gin 引擎 (用于测试)
func (a *App) Engine() *gin.Engine {
	return a.engine
}
