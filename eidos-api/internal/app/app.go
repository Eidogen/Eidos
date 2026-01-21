// Package app 提供应用生命周期管理
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
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/client"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/eidos-exchange/eidos/eidos-api/internal/router"
	"github.com/eidos-exchange/eidos/eidos-api/internal/service"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ws"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
)

// App 应用实例
type App struct {
	cfg    *config.Config
	logger *zap.Logger

	// HTTP 服务
	httpServer *http.Server
	engine     *gin.Engine

	// Nacos 组件
	nacosClient   *nacos.Client
	nacosRegistry *nacos.Registry
	nacosDiscovery *nacos.Discovery
	serviceInstance *nacos.ServiceInstance

	// 依赖组件
	redis         *redis.Client
	tradingClient *client.TradingClient
	tradingConn   *grpc.ClientConn
	marketClient  *client.MarketClient
	marketConn    *grpc.ClientConn
	riskClient    *client.RiskClient
	riskConn      *grpc.ClientConn

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
func New(cfg *config.Config, logger *zap.Logger) *App {
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
		a.logger.Info("starting HTTP server", zap.String("addr", addr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// 5. 启动 WebSocket Hub
	go a.wsHub.Run()

	// 6. 启动 Redis Pub/Sub 订阅器（接收 eidos-market 推送的行情数据）
	if err := a.wsSubscriber.Start(ctx); err != nil {
		a.logger.Warn("failed to start ws subscriber", zap.Error(err))
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

	// 从 Nacos 注销服务
	a.stopNacos()

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
			a.logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// 关闭 gRPC 连接
	if a.tradingConn != nil {
		if err := a.tradingConn.Close(); err != nil {
			a.logger.Error("trading gRPC connection close error", zap.Error(err))
		}
	}
	if a.marketConn != nil {
		if err := a.marketConn.Close(); err != nil {
			a.logger.Error("market gRPC connection close error", zap.Error(err))
		}
	}
	if a.riskConn != nil {
		if err := a.riskConn.Close(); err != nil {
			a.logger.Error("risk gRPC connection close error", zap.Error(err))
		}
	}

	// 关闭 Redis
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("Redis close error", zap.Error(err))
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
	a.logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.Stop(ctx); err != nil {
		a.logger.Error("application stop error", zap.Error(err))
	}
}

// initDependencies 初始化依赖
func (a *App) initDependencies(ctx context.Context) error {
	// 初始化 Redis
	a.redis = redis.NewClient(&redis.Options{
		Addr:     a.cfg.Redis.Addr(),
		Password: a.cfg.Redis.Password,
		DB:       a.cfg.Redis.DB,
		PoolSize: a.cfg.Redis.PoolSize,
	})
	if err := a.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	a.logger.Info("redis connected", zap.String("addr", a.cfg.Redis.Addr()))

	// 初始化 Nacos (如果启用)
	var err error
	if a.cfg.Nacos.Enabled {
		if err := a.initNacos(); err != nil {
			return fmt.Errorf("init nacos: %w", err)
		}
	}

	// 初始化 gRPC 连接
	tradingAddr := a.cfg.GRPCClients.Trading
	marketAddr := a.cfg.GRPCClients.Market
	riskAddr := a.cfg.GRPCClients.Risk

	// 如果启用 Nacos，使用 Nacos 服务发现
	if a.cfg.Nacos.Enabled && a.nacosDiscovery != nil {
		tradingAddr = nacos.BuildTarget("eidos-trading")
		marketAddr = nacos.BuildTarget("eidos-market")
		riskAddr = nacos.BuildTarget("eidos-risk")
		a.logger.Info("using nacos service discovery",
			zap.String("trading", tradingAddr),
			zap.String("market", marketAddr),
			zap.String("risk", riskAddr))
	}

	a.tradingConn, err = grpc.NewClient(
		tradingAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		return fmt.Errorf("grpc dial trading: %w", err)
	}
	a.logger.Info("trading gRPC connected", zap.String("addr", tradingAddr))

	// 初始化 Trading Client
	a.tradingClient = client.NewTradingClient(a.tradingConn)

	// 初始化 Market gRPC 连接
	a.marketConn, err = grpc.NewClient(
		marketAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		return fmt.Errorf("grpc dial market: %w", err)
	}
	a.logger.Info("market gRPC connected", zap.String("addr", marketAddr))

	// 初始化 Market Client
	a.marketClient = client.NewMarketClient(a.marketConn)

	// 初始化 Risk gRPC 连接 (可选，风控服务不可用时继续运行)
	if riskAddr != "" {
		a.riskConn, err = grpc.NewClient(
			riskAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		)
		if err != nil {
			a.logger.Warn("risk gRPC connection failed, continuing without risk service", zap.Error(err))
		} else {
			a.logger.Info("risk gRPC connected", zap.String("addr", riskAddr))
			a.riskClient = client.NewRiskClient(a.riskConn)
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

// initNacos 初始化 Nacos 客户端、服务注册和服务发现
func (a *App) initNacos() error {
	// 创建 Nacos 客户端
	nacosCfg := &nacos.Config{
		ServerAddr: a.cfg.Nacos.ServerAddr,
		Namespace:  a.cfg.Nacos.Namespace,
		Group:      a.cfg.Nacos.Group,
		Username:   a.cfg.Nacos.Username,
		Password:   a.cfg.Nacos.Password,
		LogDir:     a.cfg.Nacos.LogDir,
		CacheDir:   a.cfg.Nacos.CacheDir,
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}

	var err error
	a.nacosClient, err = nacos.NewClient(nacosCfg)
	if err != nil {
		return fmt.Errorf("create nacos client: %w", err)
	}
	a.logger.Info("nacos client created", zap.String("server", a.cfg.Nacos.ServerAddr))

	// 创建服务发现器
	a.nacosDiscovery = nacos.NewDiscovery(a.nacosClient)

	// 注册 gRPC Resolver (用于服务发现)
	nacos.RegisterResolver(a.nacosDiscovery, a.cfg.Nacos.Group)
	a.logger.Info("nacos gRPC resolver registered")

	// 创建服务注册器
	a.nacosRegistry = nacos.NewRegistry(a.nacosClient)

	// 注册当前服务实例
	a.serviceInstance = nacos.DefaultServiceInstance(
		a.cfg.Service.Name,
		uint64(a.cfg.Service.HTTPPort),
	)
	a.serviceInstance.GroupName = a.cfg.Nacos.Group
	a.serviceInstance.Metadata = map[string]string{
		"env":      a.cfg.Service.Env,
		"protocol": "http",
	}

	if err := a.nacosRegistry.Register(a.serviceInstance); err != nil {
		return fmt.Errorf("register service: %w", err)
	}
	a.logger.Info("service registered to nacos",
		zap.String("name", a.serviceInstance.ServiceName),
		zap.String("ip", a.serviceInstance.IP),
		zap.Uint64("port", a.serviceInstance.Port))

	return nil
}

// stopNacos 停止 Nacos 相关组件
func (a *App) stopNacos() {
	// 注销服务
	if a.nacosRegistry != nil && a.serviceInstance != nil {
		if err := a.nacosRegistry.Deregister(a.serviceInstance); err != nil {
			a.logger.Error("deregister service from nacos failed", zap.Error(err))
		} else {
			a.logger.Info("service deregistered from nacos")
		}
	}

	// 取消所有订阅
	if a.nacosDiscovery != nil {
		if err := a.nacosDiscovery.UnsubscribeAll(); err != nil {
			a.logger.Error("unsubscribe all services failed", zap.Error(err))
		}
	}

	// 关闭 Nacos 客户端
	if a.nacosClient != nil {
		a.nacosClient.Close()
		a.logger.Info("nacos client closed")
	}
}

// redisHealthAdapter 适配 Redis 健康检查接口
type redisHealthAdapter struct {
	client *redis.Client
}

func (r *redisHealthAdapter) Ping() error {
	return r.client.Ping(context.Background()).Err()
}
