package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/client"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/config"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/router"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/discovery"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/tracing"
)

// App 应用
type App struct {
	cfg           *config.Config
	db            *gorm.DB
	redisClient   redis.UniversalClient
	httpServer    *http.Server
	engine        *gin.Engine
	infra         *discovery.Infrastructure // 统一服务发现基础设施
	clientManager *client.ClientManager
	repos         *repositories

	// 链路追踪
	tracingShutdown func(context.Context) error
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
	// 初始化链路追踪
	if err := a.initTracing(); err != nil {
		logger.Warn("init tracing failed, tracing disabled", "error", err)
	}

	// 设置 Gin 模式
	gin.SetMode(a.cfg.Service.Env)

	// 创建 Gin 引擎
	a.engine = gin.New()

	// 如果启用了链路追踪，添加 tracing 中间件（放在最前面）
	if a.cfg.Tracing.Enabled {
		a.engine.Use(tracing.GinMiddleware(a.cfg.Service.Name))
	}

	// 添加中间件
	a.engine.Use(gin.Recovery())
	a.engine.Use(middleware.Logger())
	a.engine.Use(middleware.CORS())
	a.engine.Use(middleware.MetricsMiddleware())

	// 初始化服务发现基础设施
	if err := a.initInfra(); err != nil {
		return fmt.Errorf("init infrastructure: %w", err)
	}

	// 初始化 gRPC 客户端管理器（使用服务发现）
	a.clientManager = client.NewClientManagerWithDiscovery(&a.cfg.GRPCClients, a.infra)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer connectCancel()
	if err := a.clientManager.Connect(connectCtx); err != nil {
		logger.Warn("failed to connect to some gRPC services", "error", err)
	}

	// 连接 Jobs 服务（可选）
	if a.cfg.GRPCClients.Jobs.Addr != "" {
		jobsCtx, jobsCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer jobsCancel()
		if err := a.clientManager.ConnectJobs(jobsCtx, a.cfg.GRPCClients.Jobs.Addr); err != nil {
			logger.Warn("failed to connect to jobs service", "error", err)
		}
	}

	// 初始化存储层
	a.repos = a.initRepositories() // Store repos in App struct
	repos := a.repos

	// 初始化服务层
	services := a.initServices(repos)

	// 初始化处理器
	handlers := a.initHandlers(services)

	// 初始化认证中间件
	authMiddleware := middleware.NewAuthMiddleware(services.Auth)
	router.SetupRouter(a.engine, handlers, authMiddleware)

	// 设置 Prometheus 指标端点
	a.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 创建 HTTP 服务器
	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.cfg.Service.HTTPPort),
		Handler:      a.engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("app initialized",
		"port", a.cfg.Service.HTTPPort,
		"mode", a.cfg.Service.Env)

	return nil
}

// initTracing 初始化链路追踪
func (a *App) initTracing() error {
	if !a.cfg.Tracing.Enabled {
		return nil
	}

	tracingCfg := &tracing.Config{
		Enabled:     a.cfg.Tracing.Enabled,
		ServiceName: a.cfg.Service.Name,
		Endpoint:    a.cfg.Tracing.Endpoint,
		SampleRate:  a.cfg.Tracing.SampleRate,
		Environment: a.cfg.Service.Env,
		Version:     "1.0.0",
		Insecure:    a.cfg.Tracing.Insecure,
		Timeout:     time.Duration(a.cfg.Tracing.TimeoutSec) * time.Second,
	}

	shutdown, err := tracing.Init(tracingCfg)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}

	a.tracingShutdown = shutdown
	logger.Info("tracing initialized",
		"endpoint", a.cfg.Tracing.Endpoint,
		"sample_rate", a.cfg.Tracing.SampleRate)

	return nil
}

// initInfra 初始化服务发现基础设施
func (a *App) initInfra() error {
	opts := discovery.InitOptionsFromNacosConfig(&a.cfg.Nacos, a.cfg.Service.Name, 0, a.cfg.Service.HTTPPort)

	// 设置服务元数据
	opts.WithMetadata("version", "1.0.0")
	opts.WithMetadata("env", a.cfg.Service.Env)
	opts.WithMetadata("protocol", "http")
	// 管理后台依赖所有服务进行管理
	opts.WithDependencies("eidos-trading", "eidos-matching", "eidos-market", "eidos-chain", "eidos-risk", "eidos-jobs")

	var err error
	a.infra, err = discovery.NewInfrastructure(opts)
	if err != nil {
		return fmt.Errorf("create infrastructure: %w", err)
	}

	// 注册服务到 Nacos
	if err := a.infra.RegisterService(nil); err != nil {
		return fmt.Errorf("register service: %w", err)
	}

	logger.Info("service registered to nacos",
		"service", a.cfg.Service.Name,
		"httpPort", a.cfg.Service.HTTPPort,
	)

	// 发布配置到 Nacos 配置中心
	if err := a.infra.PublishServiceConfig(a.cfg); err != nil {
		logger.Warn("failed to publish config to nacos", "error", err)
	}

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
	Auth       *service.AuthService
	Admin      *service.AdminService
	Market     *service.MarketService
	Config     *service.ConfigService
	Stats      *service.StatsService
	Audit      *service.AuditService
	User       *service.UserService
	Order      *service.OrderService
	Withdrawal *service.WithdrawalService
	Risk       *service.RiskService
}

// initServices 初始化服务层
func (a *App) initServices(repos *repositories) *services {
	// 创建审计服务 (其他服务依赖)
	auditSvc := service.NewAuditService(repos.AuditLog)

	// 创建认证服务
	authCfg := &service.AuthServiceConfig{
		JWTSecret:      a.cfg.Auth.JWT.Secret,
		JWTExpireHours: a.cfg.Auth.JWT.ExpireHours,
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

	// 创建用户管理服务
	userSvc := service.NewUserService(
		a.clientManager.Trading(),
		a.clientManager.Risk(),
		repos.AuditLog,
	)

	// 创建订单管理服务
	orderSvc := service.NewOrderService(
		a.clientManager.Trading(),
		a.clientManager.Matching(),
		repos.AuditLog,
	)

	// 创建提现管理服务
	withdrawalSvc := service.NewWithdrawalService(
		a.clientManager.Trading(),
		a.clientManager.Chain(),
		repos.AuditLog,
	)

	// 创建风控管理服务
	riskSvc := service.NewRiskService(
		a.clientManager.Risk(),
		repos.AuditLog,
	)

	return &services{
		Auth:       authSvc,
		Admin:      adminSvc,
		Market:     marketSvc,
		Config:     configSvc,
		Stats:      statsSvc,
		Audit:      auditSvc,
		User:       userSvc,
		Order:      orderSvc,
		Withdrawal: withdrawalSvc,
		Risk:       riskSvc,
	}
}

// initHandlers 初始化处理器
func (a *App) initHandlers(svcs *services) *router.Handlers {
	return &router.Handlers{
		Auth:       handler.NewAuthHandler(svcs.Auth),
		Admin:      handler.NewAdminHandler(svcs.Admin),
		Market:     handler.NewMarketHandler(svcs.Market),
		Stats:      handler.NewStatsHandler(svcs.Stats),
		Config:     handler.NewConfigHandler(svcs.Config),
		Audit:      handler.NewAuditHandler(svcs.Audit),
		User:       handler.NewUserHandler(svcs.User),
		Order:      handler.NewOrderHandler(svcs.Order),
		Withdrawal: handler.NewWithdrawalHandler(svcs.Withdrawal),
		Risk:       handler.NewRiskHandler(svcs.Risk),
	}
}

// Run 运行应用
func (a *App) Run() error {
	// 启动 HTTP 服务
	errChan := make(chan error, 1)
	go func() {
		logger.Info("starting admin http server", "port", a.cfg.Service.HTTPPort)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("received signal, shutting down", "signal", sig)
		return nil
	case err := <-errChan:
		return fmt.Errorf("http server error: %w", err)
	}
}

// Shutdown 关闭应用
func (a *App) Shutdown(ctx context.Context) error {
	logger.Info("shutting down HTTP server")

	// Close gRPC connections
	if a.clientManager != nil {
		if err := a.clientManager.Close(); err != nil {
			logger.Warn("failed to close gRPC connections", "error", err)
		}
	}

	// 关闭服务发现基础设施（会自动注销 Nacos 服务和关闭 gRPC 连接）
	if a.infra != nil {
		if err := a.infra.Close(); err != nil {
			logger.Warn("failed to close infrastructure", "error", err)
		}
	}

	// 关闭链路追踪
	if a.tracingShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.tracingShutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown tracing failed", "error", err)
		}
	}

	return a.httpServer.Shutdown(ctx)
}

// Engine 获取 Gin 引擎 (用于测试)
func (a *App) Engine() *gin.Engine {
	return a.engine
}
