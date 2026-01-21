package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"

	"github.com/eidos-exchange/eidos/eidos-market/internal/aggregator"
	"github.com/eidos-exchange/eidos/eidos-market/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-market/internal/client"
	"github.com/eidos-exchange/eidos/eidos-market/internal/config"
	"github.com/eidos-exchange/eidos/eidos-market/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-market/internal/handler/event"
	"github.com/eidos-exchange/eidos/eidos-market/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// App 应用程序
type App struct {
	cfg    *config.Config
	logger *slog.Logger

	// 基础设施
	db             *gorm.DB
	redisClient    redis.UniversalClient
	consumer       *kafka.Consumer
	nacosHelper    *nacos.ServiceHelper
	matchingClient *client.MatchingClient

	// 缓存和发布
	redisCache *cache.RedisCache
	pubsub     *cache.PubSub

	// 仓储
	klineRepo  repository.KlineRepository
	marketRepo repository.MarketRepository
	tradeRepo  repository.TradeRepository

	// 服务
	marketService *service.MarketService

	// gRPC
	grpcServer *grpc.Server

	// HTTP (metrics)
	httpServer *http.Server
}

// New 创建应用
func New(cfg *config.Config, logger *slog.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
	}
}

// Run 运行应用
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化基础设施
	if err := a.initInfrastructure(ctx); err != nil {
		return fmt.Errorf("failed to init infrastructure: %w", err)
	}

	// 初始化仓储
	a.initRepositories()

	// 初始化缓存和发布
	a.initCacheAndPubSub()

	// 初始化服务
	if err := a.initServices(ctx); err != nil {
		return fmt.Errorf("failed to init services: %w", err)
	}

	// 初始化事件处理器
	a.initEventHandlers()

	// 启动 Kafka 消费者
	if err := a.startKafkaConsumer(); err != nil {
		return fmt.Errorf("failed to start kafka consumer: %w", err)
	}

	// 启动 gRPC 服务器
	if err := a.startGRPCServer(); err != nil {
		return fmt.Errorf("failed to start grpc server: %w", err)
	}

	// 启动 HTTP 服务器 (metrics)
	if err := a.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start http server: %w", err)
	}

	a.logger.Info("application started",
		"service", a.cfg.Service.Name,
		"grpc_port", a.cfg.Service.GRPCPort,
		"http_port", a.cfg.Service.HTTPPort)

	// 等待信号
	a.waitForSignal(cancel)

	// 优雅关闭
	return a.shutdown(ctx)
}

// initInfrastructure 初始化基础设施
func (a *App) initInfrastructure(ctx context.Context) error {
	// 初始化 Nacos (服务注册)
	if a.cfg.Nacos.Enabled {
		if err := a.initNacos(); err != nil {
			return fmt.Errorf("failed to init nacos: %w", err)
		}
	}

	// 初始化 PostgreSQL
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		a.cfg.Postgres.Host,
		a.cfg.Postgres.Port,
		a.cfg.Postgres.User,
		a.cfg.Postgres.Password,
		a.cfg.Postgres.Database,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(a.cfg.Postgres.MaxConnections)
	sqlDB.SetMaxIdleConns(a.cfg.Postgres.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(a.cfg.Postgres.ConnMaxLifetime) * time.Second)

	a.db = db
	a.logger.Info("postgres connected")

	// 自动迁移
	if err := AutoMigrate(a.db, a.logger); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	a.logger.Info("database migrated")

	// 初始化 Redis
	if len(a.cfg.Redis.Addresses) == 1 {
		a.redisClient = redis.NewClient(&redis.Options{
			Addr:     a.cfg.Redis.Addresses[0],
			Password: a.cfg.Redis.Password,
			DB:       a.cfg.Redis.DB,
			PoolSize: a.cfg.Redis.PoolSize,
		})
	} else {
		a.redisClient = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    a.cfg.Redis.Addresses,
			Password: a.cfg.Redis.Password,
			PoolSize: a.cfg.Redis.PoolSize,
		})
	}

	if err := a.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}
	a.logger.Info("redis connected")

	// 初始化 Kafka 消费者
	consumer, err := kafka.NewConsumer(kafka.ConsumerConfig{
		Brokers:  a.cfg.Kafka.Brokers,
		GroupID:  a.cfg.Kafka.GroupID,
		ClientID: a.cfg.Kafka.ClientID,
	}, a.logger)
	if err != nil {
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}
	a.consumer = consumer
	a.logger.Info("kafka consumer created")

	// 初始化 Matching 客户端（用于深度快照恢复）
	if a.cfg.Matching.Enabled {
		if err := a.initMatchingClient(); err != nil {
			// 非致命错误，记录警告继续运行（深度数据将只能通过增量更新维护）
			a.logger.Warn("failed to init matching client, depth snapshot recovery disabled",
				"error", err)
		}
	}

	return nil
}

// initRepositories 初始化仓储
func (a *App) initRepositories() {
	a.klineRepo = repository.NewKlineRepository(a.db)
	a.marketRepo = repository.NewMarketRepository(a.db)
	a.tradeRepo = repository.NewTradeRepository(a.db)
	a.logger.Info("repositories initialized")
}

// initCacheAndPubSub 初始化缓存和发布
func (a *App) initCacheAndPubSub() {
	a.redisCache = cache.NewRedisCache(a.redisClient, a.logger)
	a.pubsub = cache.NewPubSub(a.redisClient, a.logger)
	a.logger.Info("cache and pubsub initialized")
}

// initServices 初始化服务
func (a *App) initServices(ctx context.Context) error {
	// 创建服务配置
	svcConfig := service.MarketServiceConfig{
		KlineConfig: aggregator.KlineAggregatorConfig{
			FlushInterval: a.cfg.GetKlineFlushInterval(),
			BufferSize:    1000,
		},
		TickerConfig: aggregator.TickerCalculatorConfig{
			PublishInterval: a.cfg.GetTickerPublishInterval(),
		},
		DepthConfig: aggregator.DepthManagerConfig{
			MaxLevels:       a.cfg.Aggregator.DepthMaxLevels,
			PublishInterval: a.cfg.GetDepthPublishInterval(),
		},
	}

	// 创建发布者（实现 Publisher 接口）
	publisher := NewPublisher(a.pubsub, a.redisCache)

	// 准备 DepthSnapshotProvider
	// matchingClient 实现了 aggregator.DepthSnapshotProvider 接口
	var snapshotProvider aggregator.DepthSnapshotProvider
	if a.matchingClient != nil {
		snapshotProvider = a.matchingClient
		a.logger.Info("depth snapshot provider enabled via eidos-matching")
	} else {
		a.logger.Warn("depth snapshot provider disabled, sequence gaps cannot be recovered")
	}

	// 创建行情服务
	a.marketService = service.NewMarketService(
		a.klineRepo,
		a.marketRepo,
		a.tradeRepo,
		publisher,
		snapshotProvider,
		a.logger,
		svcConfig,
	)

	// 初始化所有市场
	if err := a.marketService.InitMarkets(ctx); err != nil {
		a.logger.Warn("failed to init markets, continuing anyway", "error", err)
	}

	a.logger.Info("services initialized")
	return nil
}

// initEventHandlers 初始化事件处理器
func (a *App) initEventHandlers() {
	// 成交事件处理器
	tradeHandler := event.NewTradeHandler(a.marketService, a.logger)
	a.consumer.RegisterHandler(tradeHandler)

	// 订单簿事件处理器
	orderbookHandler := event.NewOrderBookHandler(a.marketService, a.logger)
	a.consumer.RegisterHandler(orderbookHandler)

	a.logger.Info("event handlers initialized")
}

// startKafkaConsumer 启动 Kafka 消费者
func (a *App) startKafkaConsumer() error {
	return a.consumer.Start()
}

// startGRPCServer 启动 gRPC 服务器
func (a *App) startGRPCServer() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Service.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.grpcServer = grpc.NewServer()

	// 注册行情服务
	marketHandler := handler.NewMarketHandler(a.marketService)
	marketv1.RegisterMarketServiceServer(a.grpcServer, marketHandler)

	// 注册健康检查
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, healthServer)
	healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_SERVING)

	// 开发环境启用反射
	if a.cfg.IsDev() {
		reflection.Register(a.grpcServer)
	}

	go func() {
		if err := a.grpcServer.Serve(lis); err != nil {
			a.logger.Error("grpc server error", "error", err)
		}
	}()

	return nil
}

// startHTTPServer 启动 HTTP 服务器 (metrics endpoint)
func (a *App) startHTTPServer() error {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// 健康检查 endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 就绪检查 endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.cfg.Service.HTTPPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		a.logger.Info("http server started", "port", a.cfg.Service.HTTPPort)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("http server error", "error", err)
		}
	}()

	return nil
}

// waitForSignal 等待系统信号
func (a *App) waitForSignal(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	a.logger.Info("received signal", "signal", sig.String())
	cancel()
}

// initMatchingClient 初始化 Matching 服务客户端
func (a *App) initMatchingClient() error {
	cfg := &client.MatchingClientConfig{
		Addr:           a.cfg.Matching.Addr,
		ConnectTimeout: a.cfg.GetMatchingConnectTimeout(),
		RequestTimeout: a.cfg.GetMatchingRequestTimeout(),
	}

	matchingClient, err := client.NewMatchingClient(cfg, a.logger)
	if err != nil {
		return fmt.Errorf("create matching client: %w", err)
	}

	a.matchingClient = matchingClient
	a.logger.Info("matching client initialized",
		"addr", a.cfg.Matching.Addr)

	return nil
}

// initNacos 初始化 Nacos 服务注册
func (a *App) initNacos() error {
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

	helper, err := nacos.NewServiceHelper(nacosCfg)
	if err != nil {
		return fmt.Errorf("create nacos helper: %w", err)
	}
	a.nacosHelper = helper

	// 注册服务
	if err := a.nacosHelper.RegisterService(a.cfg.Service.Name, uint64(a.cfg.Service.GRPCPort), map[string]string{
		"version": "1.0.0",
		"env":     a.cfg.Service.Env,
	}); err != nil {
		return fmt.Errorf("register service: %w", err)
	}

	a.logger.Info("nacos service registered",
		"service", a.cfg.Service.Name,
		"port", a.cfg.Service.GRPCPort)

	return nil
}

// shutdown 优雅关闭
func (a *App) shutdown(ctx context.Context) error {
	a.logger.Info("shutting down...")

	// 注销 Nacos 服务
	if a.nacosHelper != nil {
		if err := a.nacosHelper.DeregisterAll(); err != nil {
			a.logger.Error("failed to deregister nacos service", "error", err)
		} else {
			a.logger.Info("nacos service deregistered")
		}
	}

	// 关闭 HTTP 服务器
	if a.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("failed to shutdown http server", "error", err)
		} else {
			a.logger.Info("http server stopped")
		}
	}

	// 关闭 gRPC 服务器
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
		a.logger.Info("grpc server stopped")
	}

	// 关闭 Kafka 消费者
	if a.consumer != nil {
		if err := a.consumer.Stop(); err != nil {
			a.logger.Error("failed to stop kafka consumer", "error", err)
		}
	}

	// 关闭行情服务
	if a.marketService != nil {
		a.marketService.Stop()
	}

	// 关闭 Matching 客户端
	if a.matchingClient != nil {
		if err := a.matchingClient.Close(); err != nil {
			a.logger.Error("failed to close matching client", "error", err)
		}
	}

	// 关闭 Redis
	if a.redisClient != nil {
		if err := a.redisClient.Close(); err != nil {
			a.logger.Error("failed to close redis", "error", err)
		}
	}

	// 关闭数据库
	if a.db != nil {
		sqlDB, _ := a.db.DB()
		if sqlDB != nil {
			if err := sqlDB.Close(); err != nil {
				a.logger.Error("failed to close database", "error", err)
			}
		}
	}

	a.logger.Info("application stopped")
	return nil
}

// Publisher 统一发布者实现
type Publisher struct {
	pubsub *cache.PubSub
	cache  *cache.RedisCache
}

// NewPublisher 创建发布者
func NewPublisher(pubsub *cache.PubSub, cache *cache.RedisCache) *Publisher {
	return &Publisher{
		pubsub: pubsub,
		cache:  cache,
	}
}

// PublishKline 实现 KlinePublisher 接口
func (p *Publisher) PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	return p.pubsub.PublishKline(ctx, market, interval, kline)
}

// PublishTicker 实现 TickerPublisher 接口
func (p *Publisher) PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error {
	return p.pubsub.PublishTicker(ctx, market, ticker)
}

// PublishDepth 实现 DepthPublisher 接口
func (p *Publisher) PublishDepth(ctx context.Context, market string, depth *model.Depth) error {
	return p.pubsub.PublishDepth(ctx, market, depth)
}

// PublishTrade 实现 Publisher 接口
func (p *Publisher) PublishTrade(ctx context.Context, market string, trade *model.Trade) error {
	return p.pubsub.PublishTrade(ctx, market, trade)
}
