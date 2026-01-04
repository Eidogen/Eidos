// Package app 提供应用生命周期管理
package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/id"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/handler/event"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
)

const serviceName = "eidos-trading"

// App 应用实例
type App struct {
	cfg *config.Config

	// 基础设施
	db          *gorm.DB
	rdb         *redis.Client
	nacosHelper *nacos.ServiceHelper
	idGen       *id.Generator

	// gRPC
	grpcServer   *grpc.Server
	healthServer *health.Server

	// Kafka
	producer *kafka.Producer
	consumer *kafka.ConsumerGroup

	// Workers
	outboxRelay          *worker.OutboxRelay
	orderOutboxRelay     *worker.OrderOutboxRelay
	cancelOutboxRelay    *worker.CancelOutboxRelay
	orderExpiryWorker    *worker.OrderExpiryWorker
	reconciliationWorker *worker.ReconciliationWorker

	// 服务层
	orderSvc    service.OrderService
	balanceSvc  service.BalanceService
	tradeSvc    service.TradeService
	depositSvc  service.DepositService
	withdrawSvc service.WithdrawalService
	clearingSvc service.ClearingService

	// 仓储层
	orderRepo    repository.OrderRepository
	balanceRepo  repository.BalanceRepository
	tradeRepo    repository.TradeRepository
	depositRepo  repository.DepositRepository
	withdrawRepo repository.WithdrawalRepository
	nonceRepo    repository.NonceRepository
	outboxRepo   *repository.OutboxRepository

	// 缓存层 (Redis 作为实时资金真相)
	balanceCache cache.BalanceRedisRepository

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
}

// New 创建应用实例
func New(cfg *config.Config) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Run 启动应用
func (a *App) Run() error {
	logger.Info("starting service", zap.String("service", serviceName))

	// 1. 初始化基础设施
	if err := a.initInfra(); err != nil {
		return fmt.Errorf("init infra: %w", err)
	}

	// 2. 初始化仓储层
	a.initRepositories()

	// 3. 初始化服务层
	a.initServices()

	// 4. 初始化 Kafka
	if err := a.initKafka(); err != nil {
		return fmt.Errorf("init kafka: %w", err)
	}

	// 4.1 重新初始化需要 Kafka producer 的服务
	a.initServicesWithKafka()

	// 5. 初始化后台任务
	a.initWorkers()

	// 6. 启动 gRPC 服务器
	if err := a.startGRPCServer(); err != nil {
		return fmt.Errorf("start grpc: %w", err)
	}

	// 7. 启动后台任务
	a.startWorkers()

	// 8. 启动 Kafka 消费者
	if err := a.startConsumers(); err != nil {
		return fmt.Errorf("start consumers: %w", err)
	}

	// 9. 等待关闭信号
	a.waitForShutdown()

	return nil
}

// initInfra 初始化基础设施
func (a *App) initInfra() error {
	var err error

	// 初始化 Nacos
	if a.cfg.Nacos.Enabled {
		a.nacosHelper, err = initNacos(a.cfg)
		if err != nil {
			return fmt.Errorf("init nacos: %w", err)
		}

		if err := a.nacosHelper.RegisterService(serviceName, uint64(a.cfg.Service.GRPCPort), map[string]string{
			"version": "1.0.0",
			"env":     a.cfg.Service.Env,
		}); err != nil {
			return fmt.Errorf("register nacos: %w", err)
		}
		logger.Info("service registered to nacos")
	}

	// 初始化数据库
	a.db, err = initDatabase(a.cfg)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	logger.Info("database connected")

	// 自动迁移
	if err := AutoMigrate(a.db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	logger.Info("database migrated")

	// 初始化 Redis
	a.rdb = initRedis(a.cfg)
	logger.Info("redis connected")

	// 初始化 ID 生成器
	a.idGen, err = id.NewGenerator(a.cfg.Node.ID)
	if err != nil {
		return fmt.Errorf("init id generator: %w", err)
	}

	return nil
}

// initRepositories 初始化仓储层
func (a *App) initRepositories() {
	a.orderRepo = repository.NewOrderRepository(a.db)
	a.balanceRepo = repository.NewBalanceRepository(a.db)
	a.tradeRepo = repository.NewTradeRepository(a.db)
	a.depositRepo = repository.NewDepositRepository(a.db)
	a.withdrawRepo = repository.NewWithdrawalRepository(a.db)
	a.nonceRepo = repository.NewNonceRepository(a.db, a.rdb)
	a.outboxRepo = repository.NewOutboxRepository(a.db)

	// 初始化 Redis 缓存层 (实时资金真相)
	a.balanceCache = cache.NewBalanceRedisRepository(a.rdb)
	logger.Info("balance cache initialized (Redis as source of truth)")
}

// initServices 初始化服务层
// 注意: orderSvc 需要 producer，但 producer 在 initKafka 中初始化
// 所以这里先传 nil，在 initKafka 后会重新初始化
func (a *App) initServices() {
	marketCfg := NewMarketConfigProvider(a.cfg)
	tokenCfg := NewTokenConfigProvider(a.cfg)
	riskCfg := NewRiskConfigProvider(a.cfg)

	// orderSvc 先用 nil producer，后续在 initServicesWithKafka 中更新
	a.orderSvc = service.NewOrderService(a.orderRepo, a.balanceRepo, a.nonceRepo, a.balanceCache, a.idGen, marketCfg, riskCfg, nil)
	a.balanceSvc = service.NewBalanceService(a.balanceRepo, a.outboxRepo, a.balanceCache, tokenCfg)
	a.tradeSvc = service.NewTradeService(a.tradeRepo, a.balanceRepo, a.balanceCache, a.idGen, marketCfg)
	a.depositSvc = service.NewDepositService(a.depositRepo, a.balanceRepo, a.balanceCache, a.idGen, tokenCfg)
	a.withdrawSvc = service.NewWithdrawalService(a.withdrawRepo, a.balanceRepo, a.balanceCache, a.nonceRepo, a.idGen, tokenCfg)
	a.clearingSvc = service.NewClearingService(a.db, a.tradeRepo, a.orderRepo, a.balanceRepo, a.balanceCache, marketCfg)
}

// initServicesWithKafka 在 Kafka 初始化后重新创建需要 producer 的服务
func (a *App) initServicesWithKafka() {
	if a.producer == nil {
		return
	}
	marketCfg := NewMarketConfigProvider(a.cfg)
	riskCfg := NewRiskConfigProvider(a.cfg)
	// 重新创建 orderSvc，这次带 producer
	a.orderSvc = service.NewOrderService(a.orderRepo, a.balanceRepo, a.nonceRepo, a.balanceCache, a.idGen, marketCfg, riskCfg, a.producer)
}

// initKafka 初始化 Kafka
func (a *App) initKafka() error {
	if !a.cfg.Kafka.Enabled {
		logger.Warn("kafka is disabled")
		return nil
	}

	var err error

	// 创建生产者
	a.producer, err = kafka.NewProducer(&kafka.ProducerConfig{
		Brokers: a.cfg.Kafka.Brokers,
	})
	if err != nil {
		return fmt.Errorf("create producer: %w", err)
	}
	logger.Info("kafka producer created")

	// 创建消费者组
	a.consumer, err = kafka.NewConsumerGroup(&kafka.ConsumerConfig{
		Brokers: a.cfg.Kafka.Brokers,
		GroupID: a.cfg.Kafka.GroupID,
		Topics: []string{
			kafka.TopicTradeResults,
			kafka.TopicOrderCancelled,
			kafka.TopicDeposits,
			kafka.TopicSettlementConfirmed,
			kafka.TopicWithdrawalConfirmed,
		},
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}
	logger.Info("kafka consumer created")

	return nil
}

// initWorkers 初始化后台任务
func (a *App) initWorkers() {
	// 1. 订单过期 Worker (不依赖 Kafka，始终启动)
	// 定期扫描过期的 PENDING 订单并取消，释放冻结资金
	if a.cfg.Worker.OrderExpiry.Enabled {
		expiryConfig := &worker.OrderExpiryWorkerConfig{
			CheckInterval: time.Duration(a.cfg.Worker.OrderExpiry.CheckIntervalSec) * time.Second,
			BatchSize:     a.cfg.Worker.OrderExpiry.BatchSize,
		}
		a.orderExpiryWorker = worker.NewOrderExpiryWorker(expiryConfig, a.orderRepo, a.orderSvc)
	}

	// 2. 对账 Worker (不依赖 Kafka，始终启动)
	// 定期检查 Redis 和 DB 余额是否一致
	if a.cfg.Worker.Reconciliation.Enabled {
		reconcileConfig := &worker.ReconciliationWorkerConfig{
			CheckInterval: time.Duration(a.cfg.Worker.Reconciliation.CheckIntervalSec) * time.Second,
			BatchSize:     a.cfg.Worker.Reconciliation.BatchSize,
		}
		a.reconciliationWorker = worker.NewReconciliationWorker(reconcileConfig, a.balanceCache, a.balanceRepo)
	}

	// 以下 Worker 依赖 Kafka
	if !a.cfg.Kafka.Enabled || a.producer == nil {
		return
	}

	// 3. Outbox Relay: 轮询数据库投递消息到 Kafka
	a.outboxRelay = worker.NewOutboxRelay(
		&worker.OutboxRelayConfig{
			RelayInterval:   time.Duration(a.cfg.Outbox.RelayIntervalMs) * time.Millisecond,
			BatchSize:       a.cfg.Outbox.BatchSize,
			CleanupInterval: time.Duration(a.cfg.Outbox.CleanupIntervalMs) * time.Millisecond,
			Retention:       time.Duration(a.cfg.Outbox.RetentionMs) * time.Millisecond,
		},
		a.outboxRepo,
		a.producer,
	)

	// 4. Order Outbox Relay: 从 Redis 消费订单并发送到 Kafka
	// 支持多实例部署，使用分布式锁保证每个分片只有一个消费者
	orderRelayCfg := worker.DefaultOrderOutboxRelayConfig()
	orderRelayCfg.InstanceID = fmt.Sprintf("order-relay-%d-%d", a.cfg.Node.ID, time.Now().UnixNano())
	a.orderOutboxRelay = worker.NewOrderOutboxRelay(orderRelayCfg, a.rdb, a.producer)

	// 5. Cancel Outbox Relay: 从 Redis 消费取消请求并发送到 Kafka
	// 支持多实例部署，使用分布式锁保证每个分片只有一个消费者
	cancelRelayCfg := worker.DefaultCancelOutboxRelayConfig()
	cancelRelayCfg.InstanceID = fmt.Sprintf("cancel-relay-%d-%d", a.cfg.Node.ID, time.Now().UnixNano())
	a.cancelOutboxRelay = worker.NewCancelOutboxRelay(cancelRelayCfg, a.rdb, a.producer)
}

// startGRPCServer 启动 gRPC 服务器
func (a *App) startGRPCServer() error {
	tradingHandler := handler.NewTradingHandler(
		a.orderSvc,
		a.balanceSvc,
		a.tradeSvc,
		a.clearingSvc,
		a.depositSvc,
		a.withdrawSvc,
	)

	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor(),
			middleware.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			middleware.RecoveryStreamServerInterceptor(),
			middleware.StreamServerInterceptor(),
		),
	)

	// 注册健康检查
	a.healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, a.healthServer)
	a.healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// 注册业务服务
	pb.RegisterTradingServiceServer(a.grpcServer, tradingHandler)

	// 启动监听
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Service.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		logger.Info("gRPC server listening", zap.Int("port", a.cfg.Service.GRPCPort))
		if err := a.grpcServer.Serve(lis); err != nil {
			logger.Error("grpc serve error", zap.Error(err))
		}
	}()

	return nil
}

// startWorkers 启动后台任务
func (a *App) startWorkers() {
	// 启动订单过期 Worker (关键: 释放过期订单的冻结资金)
	if a.orderExpiryWorker != nil {
		a.orderExpiryWorker.Start(a.ctx)
		logger.Info("order expiry worker started")
	}

	// 启动对账 Worker (关键: 检测 Redis 和 DB 数据漂移)
	if a.reconciliationWorker != nil {
		a.reconciliationWorker.Start(a.ctx)
		logger.Info("reconciliation worker started")
	}

	// 以下 Worker 依赖 Kafka
	if a.outboxRelay != nil {
		go a.outboxRelay.Start(a.ctx)
		logger.Info("outbox relay started")
	}

	if a.orderOutboxRelay != nil {
		a.orderOutboxRelay.Start(a.ctx)
		logger.Info("order outbox relay started (Redis → Kafka)")
	}

	if a.cancelOutboxRelay != nil {
		a.cancelOutboxRelay.Start(a.ctx)
		logger.Info("cancel outbox relay started (Redis → Kafka)")
	}
}

// startConsumers 启动 Kafka 消费者
func (a *App) startConsumers() error {
	if a.consumer == nil {
		return nil
	}

	// 创建事件处理器
	eventProcessor := worker.NewEventProcessor()

	// 注册各类事件处理器
	eventProcessor.RegisterHandler(
		kafka.TopicTradeResults,
		event.NewTradeEventHandler(a.clearingSvc),
	)
	eventProcessor.RegisterHandler(
		kafka.TopicOrderCancelled,
		event.NewOrderCancelledHandler(a.orderSvc),
	)
	eventProcessor.RegisterHandler(
		kafka.TopicDeposits,
		event.NewDepositHandler(a.depositSvc),
	)
	eventProcessor.RegisterHandler(
		kafka.TopicSettlementConfirmed,
		event.NewSettlementConfirmedHandler(a.clearingSvc),
	)
	eventProcessor.RegisterHandler(
		kafka.TopicWithdrawalConfirmed,
		event.NewWithdrawalConfirmedHandler(a.withdrawSvc),
	)

	// 注册到 Kafka 消费者组
	for topic := range eventProcessor.Handlers() {
		a.consumer.RegisterHandler(topic, eventProcessor)
	}

	// 启动消费
	if err := a.consumer.Start(a.ctx); err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}
	logger.Info("kafka consumer started")

	return nil
}

// waitForShutdown 等待关闭信号
func (a *App) waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	a.shutdown()
}

// shutdown 优雅关闭
func (a *App) shutdown() {
	// 取消 context，通知所有 goroutine 退出
	a.cancel()

	// 标记健康检查为不可用
	if a.healthServer != nil {
		a.healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	// 注销 Nacos 服务
	if a.nacosHelper != nil {
		if err := a.nacosHelper.DeregisterAll(); err != nil {
			logger.Error("deregister nacos failed", zap.Error(err))
		}
	}

	// 停止订单过期 Worker
	if a.orderExpiryWorker != nil {
		a.orderExpiryWorker.Stop()
	}

	// 停止对账 Worker
	if a.reconciliationWorker != nil {
		a.reconciliationWorker.Stop()
	}

	// 停止 Outbox Relay
	if a.outboxRelay != nil {
		a.outboxRelay.Stop()
	}

	// 停止 Order Outbox Relay
	if a.orderOutboxRelay != nil {
		a.orderOutboxRelay.Stop()
	}

	// 停止 Cancel Outbox Relay
	if a.cancelOutboxRelay != nil {
		a.cancelOutboxRelay.Stop()
	}

	// 停止 Kafka 消费者
	if a.consumer != nil {
		a.consumer.Stop()
	}

	// 停止 Kafka 生产者
	if a.producer != nil {
		a.producer.Close()
	}

	// 优雅停止 gRPC 服务
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 关闭 Redis
	if a.rdb != nil {
		a.rdb.Close()
	}

	// 关闭数据库
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil {
			sqlDB.Close()
		}
	}

	logger.Info("service stopped")
}

// ========== Infrastructure Init ==========

func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
	)

	gormConfig := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)

	return db, nil
}

func initRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})
}

func initNacos(cfg *config.Config) (*nacos.ServiceHelper, error) {
	nacosCfg := &nacos.Config{
		ServerAddr: cfg.Nacos.ServerAddr,
		Namespace:  cfg.Nacos.Namespace,
		Group:      cfg.Nacos.Group,
		Username:   cfg.Nacos.Username,
		Password:   cfg.Nacos.Password,
		LogDir:     cfg.Nacos.LogDir,
		CacheDir:   cfg.Nacos.CacheDir,
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}
	return nacos.NewServiceHelper(nacosCfg)
}
