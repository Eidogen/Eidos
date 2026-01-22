// Package app 提供应用生命周期管理
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/client"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/eidos-exchange/eidos/eidos-api/internal/router"
	"github.com/eidos-exchange/eidos/eidos-api/internal/service"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ws"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/discovery"
)

// App 应用实例
type App struct {
	cfg    *config.Config
	logger *slog.Logger

	// HTTP 服务
	httpServer *http.Server
	engine     *gin.Engine

	// 统一服务发现基础设施
	infra *discovery.Infrastructure

	// 依赖组件
	redis         *redis.Client
	tradingClient *client.TradingClient
	marketClient  *client.MarketClient
	riskClient    *client.RiskClient

	// Handlers
	healthHandler     *handler.HealthHandler
	orderHandler      *handler.OrderHandler
	balanceHandler    *handler.BalanceHandler
	depositHandler    *handler.DepositHandler
	withdrawalHandler *handler.WithdrawalHandler
	tradeHandler      *handler.TradeHandler
	marketHandler     *handler.MarketHandler

	// WebSocket
	wsHub        *ws.Hub
	wsSubscriber *ws.Subscriber

	// Middleware 组件
	replayGuard   *cache.ReplayGuard
	slidingWindow *ratelimit.SlidingWindow
}

// New 创建应用实例
func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

// Start 启动应用
func (a *App) Start(ctx context.Context) error {
	// 1. 初始化依赖
	if err := a.initDependencies(ctx); err != nil {
		return fmt.Errorf("init dependencies: %w", err)
	}

	// 2. 初始化 HTTP 服务
	a.initHTTPServer()

	// 3. 设置就绪状态
	a.healthHandler.SetReady(true)

	// 4. 启动 HTTP 服务
	go func() {
		addr := fmt.Sprintf(":%d", a.cfg.Service.HTTPPort)
		a.logger.Info("starting HTTP server", "addr", addr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	// 5. 启动 WebSocket Hub
	go a.wsHub.Run()

	// 6. 启动 Redis Pub/Sub 订阅器（接收 eidos-market 推送的行情数据）
	if err := a.wsSubscriber.Start(ctx); err != nil {
		a.logger.Warn("failed to start ws subscriber", "error", err)
		// 订阅失败不影响服务启动，WebSocket 仍可工作但不会收到实时推送
	}

	return nil
}

// Stop 停止应用
func (a *App) Stop(ctx context.Context) error {
	a.logger.Info("stopping application")

	// 设置不就绪
	if a.healthHandler != nil {
		a.healthHandler.SetReady(false)
	}

	// 停止 Redis Pub/Sub 订阅器
	if a.wsSubscriber != nil {
		a.wsSubscriber.Stop()
	}

	// 停止 WebSocket Hub
	if a.wsHub != nil {
		a.wsHub.Stop()
	}

	// 停止 HTTP 服务
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.logger.Error("HTTP server shutdown error", "error", err)
		}
	}

	// 关闭服务发现基础设施（会自动注销 Nacos 服务和关闭 gRPC 连接）
	if a.infra != nil {
		if err := a.infra.Close(); err != nil {
			a.logger.Error("close infrastructure failed", "error", err)
		}
	}

	// 关闭 Redis
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("Redis close error", "error", err)
		}
	}

	a.logger.Info("application stopped")
	return nil
}

// WaitForShutdown 等待关闭信号
func (a *App) WaitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	a.logger.Info("received shutdown signal", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.Stop(ctx); err != nil {
		a.logger.Error("application stop error", "error", err)
	}
}

// initDependencies 初始化依赖
func (a *App) initDependencies(ctx context.Context) error {
	// 初始化 Redis
	a.redis = redis.NewClient(&redis.Options{
		Addr:     a.cfg.RedisAddr(),
		Password: a.cfg.Redis.Password,
		DB:       a.cfg.Redis.DB,
		PoolSize: a.cfg.Redis.PoolSize,
	})
	if err := a.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	a.logger.Info("redis connected", "addr", a.cfg.RedisAddr())

	// 初始化统一服务发现基础设施 (Nacos 必须启用)
	opts := discovery.InitOptionsFromNacosConfig(&a.cfg.Nacos, a.cfg.Service.Name, 0, a.cfg.Service.HTTPPort)

	// 设置服务元数据
	opts.WithMetadata("version", "1.0.0")
	opts.WithMetadata("env", a.cfg.Service.Env)
	opts.WithMetadata("protocol", "http")
	opts.WithDependencies("eidos-trading", "eidos-market", "eidos-risk")
	opts.WithKafkaTopics(
		[]string{"orders", "cancel-requests"},
		[]string{"order-updates", "balance-updates"},
	)
	opts.Logger = a.logger

	var err error
	a.infra, err = discovery.NewInfrastructure(opts)
	if err != nil {
		return fmt.Errorf("init infrastructure: %w", err)
	}

	// 注册服务到 Nacos
	if err := a.infra.RegisterService(nil); err != nil {
		return fmt.Errorf("register service: %w", err)
	}
	a.logger.Info("service registered to nacos",
		"service", a.cfg.Service.Name,
		"httpPort", a.cfg.Service.HTTPPort,
	)

	// 发布配置到 Nacos 配置中心
	if err := a.infra.PublishServiceConfig(a.cfg); err != nil {
		a.logger.Warn("failed to publish config to nacos", "error", err)
	}

	// 初始化 Trading Client（通过服务发现）
	tradingConn, err := a.infra.GetServiceConnection(ctx, discovery.ServiceTrading)
	if err != nil {
		return fmt.Errorf("get trading connection: %w", err)
	}
	a.tradingClient = client.NewTradingClient(tradingConn)
	a.logger.Info("trading client initialized via service discovery",
		"service", discovery.ServiceTrading,
		"mode", a.getServiceDiscoveryMode(),
	)

	// 初始化 Market Client（通过服务发现）
	marketConn, err := a.infra.GetServiceConnection(ctx, discovery.ServiceMarket)
	if err != nil {
		return fmt.Errorf("get market connection: %w", err)
	}
	a.marketClient = client.NewMarketClient(marketConn)
	a.logger.Info("market client initialized via service discovery",
		"service", discovery.ServiceMarket,
		"mode", a.getServiceDiscoveryMode(),
	)

	// 初始化 Risk Client（通过服务发现，可选）
	if a.cfg.GRPCClients.Risk.Addr != "" {
		riskConn, err := a.infra.GetServiceConnection(ctx, discovery.ServiceRisk)
		if err != nil {
			a.logger.Warn("risk connection failed, continuing without risk service", "error", err)
		} else {
			a.riskClient = client.NewRiskClient(riskConn)
			a.logger.Info("risk client initialized via service discovery",
				"service", discovery.ServiceRisk,
				"mode", a.getServiceDiscoveryMode(),
			)
		}
	}

	// 初始化限流组件
	a.slidingWindow = ratelimit.NewSlidingWindow(a.redis)
	a.replayGuard = cache.NewReplayGuard(a.redis)

	// 初始化 WebSocket Hub 和 Redis Pub/Sub 订阅器
	a.wsHub = ws.NewHub(a.logger)
	a.wsSubscriber = ws.NewSubscriber(a.wsHub, a.redis, a.logger)

	// 初始化风控服务
	var riskService *service.RiskService
	if a.cfg.Risk.Enabled && a.riskClient != nil {
		riskService = service.NewRiskService(a.riskClient, true)
		a.logger.Info("risk service enabled")
	} else {
		a.logger.Warn("risk service disabled or unavailable")
	}

	// 初始化 Services（适配层）
	orderService := service.NewOrderServiceWithRisk(a.tradingClient, riskService)
	balanceService := service.NewBalanceService(a.tradingClient)
	depositService := service.NewDepositService(a.tradingClient)
	withdrawalService := service.NewWithdrawalServiceWithRisk(a.tradingClient, riskService)
	tradeService := service.NewTradeService(a.tradingClient)
	marketService := service.NewMarketService(a.marketClient)
	tradingHealthAdapter := service.NewTradingHealthAdapter(a.tradingClient)
	marketHealthAdapter := service.NewMarketHealthAdapter(a.marketClient)

	// 初始化 Handlers
	a.healthHandler = handler.NewHealthHandler(&handler.HealthDeps{
		TradingClient: tradingHealthAdapter,
		MarketClient:  marketHealthAdapter,
		RedisClient:   &redisHealthAdapter{a.redis},
	})
	a.orderHandler = handler.NewOrderHandler(orderService)
	a.balanceHandler = handler.NewBalanceHandler(balanceService)
	a.depositHandler = handler.NewDepositHandler(depositService)
	a.withdrawalHandler = handler.NewWithdrawalHandler(withdrawalService)
	a.tradeHandler = handler.NewTradeHandler(tradeService)
	a.marketHandler = handler.NewMarketHandler(marketService)

	return nil
}

// getServiceDiscoveryMode 返回当前服务发现模式
func (a *App) getServiceDiscoveryMode() string {
	if a.infra != nil && a.infra.IsNacosEnabled() {
		return "nacos"
	}
	return "static"
}

// initHTTPServer 初始化 HTTP 服务
func (a *App) initHTTPServer() {
	// 设置 Gin 模式
	if a.cfg.Service.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	a.engine = gin.New()

	// 注册路由
	r := router.New(
		a.engine,
		a.cfg,
		a.logger,
		a.slidingWindow,
		a.replayGuard,
	)

	r.RegisterMiddleware()
	r.RegisterRoutes(
		a.healthHandler,
		a.orderHandler,
		a.balanceHandler,
		a.depositHandler,
		a.withdrawalHandler,
		a.tradeHandler,
		a.marketHandler,
		a.wsHub,
	)

	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.cfg.Service.HTTPPort),
		Handler:      a.engine,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// Engine 返回 Gin 引擎（用于测试）
func (a *App) Engine() *gin.Engine {
	return a.engine
}


// redisHealthAdapter 适配 Redis 健康检查接口
type redisHealthAdapter struct {
	client *redis.Client
}

func (r *redisHealthAdapter) Ping() error {
	return r.client.Ping(context.Background()).Err()
}
