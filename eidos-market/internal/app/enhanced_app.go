// Package app provides the application bootstrap.
package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
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
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
	msync "github.com/eidos-exchange/eidos/eidos-market/internal/sync"
)

// EnhancedApp high-throughput application with full caching support
type EnhancedApp struct {
	cfg    *config.Config
	logger *zap.Logger

	// Infrastructure
	db                *gorm.DB
	redisClient       redis.UniversalClient
	batchConsumer     *kafka.BatchConsumer
	nacosHelper       *nacos.ServiceHelper
	nacosConfigCenter *nacos.ConfigCenter
	matchingClient    *client.MatchingClient

	// Cache layer
	cacheManager *cache.CacheManager

	// Repositories
	klineRepo  repository.KlineRepository
	marketRepo repository.MarketRepository
	tradeRepo  repository.TradeRepository

	// Services
	marketService   *service.EnhancedMarketService
	marketConfigSync *msync.MarketConfigSync

	// Servers
	grpcServer *grpc.Server
	httpServer *http.Server
}

// NewEnhancedApp creates a new enhanced application
func NewEnhancedApp(cfg *config.Config, logger *zap.Logger) *EnhancedApp {
	return &EnhancedApp{
		cfg:    cfg,
		logger: logger,
	}
}

// Run runs the enhanced application
func (a *EnhancedApp) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize infrastructure
	if err := a.initInfrastructure(ctx); err != nil {
		return fmt.Errorf("failed to init infrastructure: %w", err)
	}

	// Initialize repositories
	a.initRepositories()

	// Initialize cache manager
	a.initCacheManager()

	// Initialize services
	if err := a.initServices(ctx); err != nil {
		return fmt.Errorf("failed to init services: %w", err)
	}

	// Initialize event handlers
	a.initEventHandlers()

	// Start Kafka consumer
	if err := a.startKafkaConsumer(); err != nil {
		return fmt.Errorf("failed to start kafka consumer: %w", err)
	}

	// Start gRPC server
	if err := a.startGRPCServer(); err != nil {
		return fmt.Errorf("failed to start grpc server: %w", err)
	}

	// Start HTTP server
	if err := a.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start http server: %w", err)
	}

	a.logger.Info("enhanced application started",
		zap.String("service", a.cfg.Service.Name),
		zap.Int("grpc_port", a.cfg.Service.GRPCPort),
		zap.Int("http_port", a.cfg.Service.HTTPPort))

	// Wait for signal
	a.waitForSignal(cancel)

	// Graceful shutdown
	return a.shutdown(ctx)
}

// initInfrastructure initializes infrastructure components
func (a *EnhancedApp) initInfrastructure(ctx context.Context) error {
	// Initialize Nacos
	if a.cfg.Nacos.Enabled {
		if err := a.initNacos(); err != nil {
			return fmt.Errorf("failed to init nacos: %w", err)
		}
	}

	// Initialize PostgreSQL
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

	// Auto migrate
	if err := AutoMigrate(a.db, a.logger); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	a.logger.Info("database migrated")

	// Initialize Redis
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

	// Initialize batch Kafka consumer for high throughput
	batchConfig := kafka.DefaultBatchConsumerConfig()
	batchConfig.Brokers = a.cfg.Kafka.Brokers
	batchConfig.GroupID = a.cfg.Kafka.GroupID
	batchConsumer, err := kafka.NewBatchConsumer(batchConfig, a.logger)
	if err != nil {
		return fmt.Errorf("failed to create batch kafka consumer: %w", err)
	}
	a.batchConsumer = batchConsumer
	a.logger.Info("batch kafka consumer created")

	// Initialize Matching client
	if a.cfg.Matching.Enabled {
		if err := a.initMatchingClient(); err != nil {
			a.logger.Warn("failed to init matching client, depth snapshot recovery disabled",
				zap.Error(err))
		}
	}

	return nil
}

// initRepositories initializes repositories
func (a *EnhancedApp) initRepositories() {
	a.klineRepo = repository.NewKlineRepository(a.db)
	a.marketRepo = repository.NewMarketRepository(a.db)
	a.tradeRepo = repository.NewTradeRepository(a.db)
	a.logger.Info("repositories initialized")
}

// initCacheManager initializes cache manager
func (a *EnhancedApp) initCacheManager() {
	a.cacheManager = cache.NewCacheManager(
		a.redisClient,
		a.klineRepo,
		a.tradeRepo,
		a.logger,
	)
	a.logger.Info("cache manager initialized")
}

// initServices initializes services
func (a *EnhancedApp) initServices(ctx context.Context) error {
	// Create publisher
	publisher := NewPublisher(a.cacheManager.PubSub, a.cacheManager.RedisCache)

	// Prepare DepthSnapshotProvider
	var snapshotProvider aggregator.DepthSnapshotProvider
	if a.matchingClient != nil {
		snapshotProvider = a.matchingClient
		a.logger.Info("depth snapshot provider enabled via eidos-matching")
	} else {
		a.logger.Warn("depth snapshot provider disabled, sequence gaps cannot be recovered")
	}

	// Create service config
	svcConfig := service.EnhancedMarketServiceConfig{
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
		CacheConfig:      cache.DefaultCacheManagerConfig(),
		MarketSyncConfig: msync.DefaultMarketConfigSyncConfig(),
		EnableCacheFirst: true,
		TickerCacheTTL:   10 * time.Second,
		DepthCacheTTL:    5 * time.Second,
		KlineCacheTTL:    60 * time.Second,
	}

	// Create enhanced market service
	a.marketService = service.NewEnhancedMarketService(
		a.klineRepo,
		a.marketRepo,
		a.tradeRepo,
		a.cacheManager,
		publisher,
		snapshotProvider,
		a.logger,
		svcConfig,
	)

	// Start enhanced service
	if err := a.marketService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start market service: %w", err)
	}

	// Initialize market config sync if Nacos is enabled
	if a.cfg.Nacos.Enabled && a.nacosConfigCenter != nil {
		a.marketConfigSync = msync.NewMarketConfigSync(
			a.marketRepo,
			a.nacosConfigCenter,
			a.marketService, // Implements MarketChangeListener
			a.logger,
			svcConfig.MarketSyncConfig,
		)

		if err := a.marketConfigSync.Start(); err != nil {
			a.logger.Warn("failed to start market config sync", zap.Error(err))
		}
	}

	a.logger.Info("services initialized")
	return nil
}

// initEventHandlers initializes event handlers
func (a *EnhancedApp) initEventHandlers() {
	// Batch trade handler
	batchTradeHandler := event.NewBatchTradeHandler(a.marketService, a.logger)
	a.batchConsumer.RegisterHandler(batchTradeHandler)

	// Batch orderbook handler
	batchOrderBookHandler := event.NewBatchOrderBookHandler(a.marketService, a.logger)
	a.batchConsumer.RegisterHandler(batchOrderBookHandler)

	a.logger.Info("batch event handlers initialized")
}

// startKafkaConsumer starts the Kafka consumer
func (a *EnhancedApp) startKafkaConsumer() error {
	return a.batchConsumer.Start()
}

// startGRPCServer starts the gRPC server
func (a *EnhancedApp) startGRPCServer() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Service.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.grpcServer = grpc.NewServer()

	// Register enhanced market handler
	marketHandler := handler.NewEnhancedMarketHandler(a.marketService, a.logger)
	marketv1.RegisterMarketServiceServer(a.grpcServer, marketHandler)

	// Register health check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, healthServer)
	healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable reflection in dev environment
	if a.cfg.IsDev() {
		reflection.Register(a.grpcServer)
	}

	go func() {
		if err := a.grpcServer.Serve(lis); err != nil {
			a.logger.Error("grpc server error", zap.Error(err))
		}
	}()

	return nil
}

// startHTTPServer starts the HTTP server
func (a *EnhancedApp) startHTTPServer() error {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Ready check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check cache manager health
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := a.cacheManager.HealthCheck(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Redis not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Stats endpoint
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// TODO: Add stats JSON response
		w.Write([]byte(`{"status":"ok"}`))
	})

	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.cfg.Service.HTTPPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		a.logger.Info("http server started", zap.Int("port", a.cfg.Service.HTTPPort))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("http server error", zap.Error(err))
		}
	}()

	return nil
}

// waitForSignal waits for termination signal
func (a *EnhancedApp) waitForSignal(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	a.logger.Info("received signal", zap.String("signal", sig.String()))
	cancel()
}

// initMatchingClient initializes the Matching service client
func (a *EnhancedApp) initMatchingClient() error {
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
		zap.String("addr", a.cfg.Matching.Addr))

	return nil
}

// initNacos initializes Nacos service registration
func (a *EnhancedApp) initNacos() error {
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

	// Create ConfigCenter for config listening
	configCenterCfg := &nacos.ConfigCenterConfig{
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
	configCenter, err := nacos.NewConfigCenter(configCenterCfg)
	if err != nil {
		a.logger.Warn("failed to create nacos config center, config sync will be disabled",
			zap.Error(err))
	} else {
		a.nacosConfigCenter = configCenter
	}

	// Register service
	if err := a.nacosHelper.RegisterService(a.cfg.Service.Name, uint64(a.cfg.Service.GRPCPort), map[string]string{
		"version": "2.0.0", // Enhanced version
		"env":     a.cfg.Service.Env,
	}); err != nil {
		return fmt.Errorf("register service: %w", err)
	}

	a.logger.Info("nacos service registered",
		zap.String("service", a.cfg.Service.Name),
		zap.Int("port", a.cfg.Service.GRPCPort))

	return nil
}

// shutdown gracefully shuts down the application
func (a *EnhancedApp) shutdown(ctx context.Context) error {
	a.logger.Info("shutting down...")

	// Deregister Nacos service
	if a.nacosHelper != nil {
		if err := a.nacosHelper.DeregisterAll(); err != nil {
			a.logger.Error("failed to deregister nacos service", zap.Error(err))
		} else {
			a.logger.Info("nacos service deregistered")
		}
	}

	// Close Nacos config center
	if a.nacosConfigCenter != nil {
		a.nacosConfigCenter.Close()
	}

	// Stop market config sync
	if a.marketConfigSync != nil {
		a.marketConfigSync.Stop()
	}

	// Stop HTTP server
	if a.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("failed to shutdown http server", zap.Error(err))
		} else {
			a.logger.Info("http server stopped")
		}
	}

	// Stop gRPC server
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
		a.logger.Info("grpc server stopped")
	}

	// Stop Kafka consumer
	if a.batchConsumer != nil {
		if err := a.batchConsumer.Stop(); err != nil {
			a.logger.Error("failed to stop kafka consumer", zap.Error(err))
		}
	}

	// Stop market service
	if a.marketService != nil {
		a.marketService.Stop()
	}

	// Stop cache manager
	if a.cacheManager != nil {
		a.cacheManager.Stop()
	}

	// Close Matching client
	if a.matchingClient != nil {
		if err := a.matchingClient.Close(); err != nil {
			a.logger.Error("failed to close matching client", zap.Error(err))
		}
	}

	// Close Redis
	if a.redisClient != nil {
		if err := a.redisClient.Close(); err != nil {
			a.logger.Error("failed to close redis", zap.Error(err))
		}
	}

	// Close database
	if a.db != nil {
		sqlDB, _ := a.db.DB()
		if sqlDB != nil {
			if err := sqlDB.Close(); err != nil {
				a.logger.Error("failed to close database", zap.Error(err))
			}
		}
	}

	a.logger.Info("application stopped")
	return nil
}
