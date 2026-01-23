// Package app 提供应用生命周期管理
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

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/discovery"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/id"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/infra"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/migrate"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/tracing"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/client"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/handler/event"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/publisher"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
)

const serviceName = "eidos-trading"

// App 应用实例
type App struct {
	cfg *config.Config

	// 基础设施
	db    *gorm.DB
	rdb   *redis.Client
	infra *discovery.Infrastructure // 统一服务发现基础设施
	idGen *id.Generator

	// gRPC
	grpcServer   *grpc.Server
	healthServer *health.Server

	// HTTP (metrics + health)
	httpServer *http.Server

	// Kafka
	producer *kafka.Producer
	consumer *kafka.ConsumerGroup

	// 外部服务客户端
	matchingClient *client.MatchingClient
	riskClient     *client.RiskClient

	// 消息发布者
	orderPublisher      *publisher.OrderPublisher
	balancePublisher    *publisher.BalancePublisher
	settlementPublisher *publisher.SettlementPublisher
	withdrawalPublisher *publisher.WithdrawalPublisher

	// Workers
	outboxRelay          *worker.OutboxRelay
	orderOutboxRelay     *worker.OrderOutboxRelay
	cancelOutboxRelay    *worker.CancelOutboxRelay
	orderExpiryWorker    *worker.OrderExpiryWorker
	reconciliationWorker *worker.ReconciliationWorker

	// 服务层
	orderSvc      service.OrderService
	balanceSvc    service.BalanceService
	tradeSvc      service.TradeService
	depositSvc    service.DepositService
	withdrawSvc   service.WithdrawalService
	clearingSvc   service.ClearingService
	subAccountSvc service.SubAccountService

	// 仓储层
	orderRepo      repository.OrderRepository
	balanceRepo    repository.BalanceRepository
	tradeRepo      repository.TradeRepository
	depositRepo    repository.DepositRepository
	withdrawRepo   repository.WithdrawalRepository
	nonceRepo      repository.NonceRepository
	outboxRepo     *repository.OutboxRepository
	subAccountRepo repository.SubAccountRepository

	// 缓存层 (Redis 作为实时资金真相)
	balanceCache cache.BalanceRedisRepository

	// 链路追踪
	tracingShutdown func(context.Context) error

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
	logger.Info("starting service", "service", serviceName)

	// 0. 初始化链路追踪
	if err := a.initTracing(); err != nil {
		logger.Warn("init tracing failed, tracing disabled", "error", err)
	}

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

	// 4.1 初始化消息发布者
	a.initPublishers()

	// 4.2 重新初始化需要 Kafka producer 的服务
	a.initServicesWithKafka()

	// 4.3 初始化外部服务客户端
	if err := a.initClients(); err != nil {
		return fmt.Errorf("init clients: %w", err)
	}

	// 5. 初始化后台任务
	a.initWorkers()

	// 6. 启动 gRPC 服务器
	if err := a.startGRPCServer(); err != nil {
		return fmt.Errorf("start grpc: %w", err)
	}

	// 6.1 启动 HTTP 服务器 (metrics + health check)
	a.startHTTPServer()

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

// initInfra 初始化基础设施
func (a *App) initInfra() error {
	var err error

	// 初始化统一服务发现基础设施 (Nacos 必须启用)
	opts := discovery.InitOptionsFromNacosConfig(&a.cfg.Nacos, serviceName, a.cfg.Service.GRPCPort, a.cfg.Service.HTTPPort)

	// 设置服务元数据
	opts.WithMetadata("version", "1.0.0")
	opts.WithMetadata("env", a.cfg.Service.Env)
	opts.WithDependencies("eidos-matching", "eidos-risk")
	opts.WithKafkaTopics(
		[]string{"orders", "cancel-requests", "settlements", "withdrawals"},
		[]string{"trade-results", "order-cancelled", "deposits", "settlement-confirmed", "withdrawal-confirmed"},
	)

	a.infra, err = discovery.NewInfrastructure(opts)
	if err != nil {
		return fmt.Errorf("init infrastructure: %w", err)
	}

	// 注册服务到 Nacos
	if err := a.infra.RegisterService(nil); err != nil {
		return fmt.Errorf("register service: %w", err)
	}
	logger.Info("service registered to nacos",
		"service", serviceName,
		"grpcPort", a.cfg.Service.GRPCPort,
	)

	// 发布配置到 Nacos 配置中心
	if err := a.infra.PublishServiceConfig(a.cfg); err != nil {
		logger.Warn("failed to publish config to nacos", "error", err)
	}

	// 确保数据库存在
	if err := migrate.EnsureDatabase(a.cfg.Postgres.Host, a.cfg.Postgres.Port, a.cfg.Postgres.User, a.cfg.Postgres.Password, a.cfg.Postgres.Database); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}

	// 初始化数据库 (使用统一的 infra 包)
	a.db, err = infra.NewDatabase(&a.cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}

	// 自动迁移
	if err := AutoMigrate(a.db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	logger.Info("database migrated")

	// 初始化 Redis (使用统一的 infra 包)
	a.rdb = infra.NewRedisClient(&a.cfg.Redis)

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
	a.subAccountRepo = repository.NewSubAccountRepository(a.db)

	// 初始化 Redis 缓存层 (实时资金真相)
	a.balanceCache = cache.NewBalanceRedisRepository(a.rdb)
	logger.Info("balance cache initialized (Redis as source of truth)")
}

// initServices 初始化服务层
// 注意: orderSvc 和 balanceSvc 需要 publisher，但 publisher 在 initPublishers 中初始化
// 所以这里先传 nil，在 initPublishers 后会重新初始化
func (a *App) initServices() {
	marketCfg := NewMarketConfigProvider(a.cfg)
	tokenCfg := NewTokenConfigProvider(a.cfg)
	riskCfg := NewRiskConfigProvider(a.cfg)

	// 先用 nil producer/publisher，后续在 initServicesWithKafka 中更新
	a.orderSvc = service.NewOrderService(a.orderRepo, a.balanceRepo, a.nonceRepo, a.balanceCache, a.idGen, marketCfg, riskCfg, nil, nil)
	a.balanceSvc = service.NewBalanceService(a.balanceRepo, a.outboxRepo, a.balanceCache, tokenCfg, nil)
	a.tradeSvc = service.NewTradeService(a.tradeRepo, a.balanceRepo, a.balanceCache, a.idGen, marketCfg)
	a.depositSvc = service.NewDepositService(a.depositRepo, a.balanceRepo, a.balanceCache, a.idGen, tokenCfg)
	a.withdrawSvc = service.NewWithdrawalService(a.withdrawRepo, a.balanceRepo, a.balanceCache, a.nonceRepo, a.idGen, tokenCfg, nil)
	a.clearingSvc = service.NewClearingService(a.db, a.tradeRepo, a.orderRepo, a.balanceRepo, a.balanceCache, marketCfg, nil, nil, nil)
	a.subAccountSvc = service.NewSubAccountService(a.subAccountRepo, a.balanceRepo, tokenCfg)
}

// initServicesWithKafka 在 Kafka 和 Publisher 初始化后重新创建需要它们的服务
func (a *App) initServicesWithKafka() {
	if a.producer == nil {
		return
	}
	marketCfg := NewMarketConfigProvider(a.cfg)
	tokenCfg := NewTokenConfigProvider(a.cfg)
	riskCfg := NewRiskConfigProvider(a.cfg)

	// 重新创建需要 producer/publisher 的服务
	a.orderSvc = service.NewOrderService(a.orderRepo, a.balanceRepo, a.nonceRepo, a.balanceCache, a.idGen, marketCfg, riskCfg, a.producer, a.orderPublisher)
	a.balanceSvc = service.NewBalanceService(a.balanceRepo, a.outboxRepo, a.balanceCache, tokenCfg, a.balancePublisher)
	a.clearingSvc = service.NewClearingService(a.db, a.tradeRepo, a.orderRepo, a.balanceRepo, a.balanceCache, marketCfg, a.orderPublisher, a.balancePublisher, a.settlementPublisher)
	a.withdrawSvc = service.NewWithdrawalService(a.withdrawRepo, a.balanceRepo, a.balanceCache, a.nonceRepo, a.idGen, tokenCfg, a.withdrawalPublisher)
}

// initPublishers 初始化消息发布者
func (a *App) initPublishers() {
	if a.producer == nil {
		logger.Warn("kafka producer not available, publishers disabled")
		return
	}

	a.orderPublisher = publisher.NewOrderPublisher(a.producer)
	a.balancePublisher = publisher.NewBalancePublisher(a.producer)
	a.settlementPublisher = publisher.NewSettlementPublisher(a.producer)
	a.withdrawalPublisher = publisher.NewWithdrawalPublisher(a.producer)
	logger.Info("message publishers initialized (order-updates, balance-updates, settlements, withdrawals)")
}

// initClients 初始化外部服务客户端
func (a *App) initClients() error {
	ctx := context.Background()

	// 初始化 eidos-matching 客户端（通过服务发现）
	if a.cfg.Matching.Enabled {
		conn, err := a.infra.GetServiceConnection(ctx, discovery.ServiceMatching)
		if err != nil {
			logger.Warn("failed to get connection to eidos-matching, continuing without it",
				"service", discovery.ServiceMatching,
				"error", err,
			)
			// 不返回错误，允许在没有 matching 服务的情况下启动
		} else {
			a.matchingClient = client.NewMatchingClientFromConn(conn)
			logger.Info("matching client initialized via service discovery",
				"service", discovery.ServiceMatching,
				"mode", a.getServiceDiscoveryMode(),
			)
		}
	}

	// 初始化 eidos-risk 客户端（通过服务发现）
	if a.cfg.Risk.Enabled {
		conn, err := a.infra.GetServiceConnection(ctx, discovery.ServiceRisk)
		if err != nil {
			logger.Warn("failed to get connection to eidos-risk, continuing without it",
				"service", discovery.ServiceRisk,
				"error", err,
			)
			// 不返回错误，允许在没有 risk 服务的情况下启动
		} else {
			riskCfg := &client.RiskClientConfig{
				RequestTimeout: 3 * time.Second,
				MaxRetries:     3,
				RetryBackoff:   100 * time.Millisecond,
				FailOpen:       a.cfg.Risk.FailOpen,
			}
			a.riskClient = client.NewRiskClientFromConn(conn, riskCfg)
			logger.Info("risk client initialized via service discovery",
				"service", discovery.ServiceRisk,
				"mode", a.getServiceDiscoveryMode(),
			)
		}
	}

	return nil
}

// getServiceDiscoveryMode 返回当前服务发现模式
func (a *App) getServiceDiscoveryMode() string {
	if a.infra != nil && a.infra.IsNacosEnabled() {
		return "nacos"
	}
	return "static"
}

// initKafka 初始化 Kafka
func (a *App) initKafka() error {
	if !a.cfg.Kafka.Enabled {
		logger.Warn("kafka is disabled")
		return nil
	}

	var err error

	// 创建生产者
	a.producer, err = kafka.NewProducer(kafka.DefaultProducerConfig(a.cfg.Kafka.Brokers))
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
			kafka.TopicWithdrawalReviewResults, // 提现审核结果 (来自 eidos-risk)
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
		a.reconciliationWorker = worker.NewReconciliationWorker(reconcileConfig, a.rdb, a.balanceCache, a.balanceRepo)
	}

	// 以下 Worker 依赖 Kafka
	if !a.cfg.Kafka.Enabled || a.producer == nil {
		return
	}

	// 3. Outbox Relay: 轮询数据库投递消息到 Kafka
	a.outboxRelay = worker.NewOutboxRelay(
		&worker.OutboxRelayConfig{
			RelayInterval:    time.Duration(a.cfg.Outbox.RelayIntervalMs) * time.Millisecond,
			BatchSize:        a.cfg.Outbox.BatchSize,
			CleanupInterval:  time.Duration(a.cfg.Outbox.CleanupIntervalMs) * time.Millisecond,
			Retention:        time.Duration(a.cfg.Outbox.RetentionMs) * time.Millisecond,
			RecoveryInterval: 5 * time.Minute,
			StaleThreshold:   5 * time.Minute,
		},
		a.outboxRepo,
		a.producer,
		nil, // alerter: 可选，nil 时不发送告警通知
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
	a.cancelOutboxRelay = worker.NewCancelOutboxRelay(cancelRelayCfg, a.rdb, a.producer, nil)
}

// startHTTPServer 启动 HTTP 服务器 (metrics + health check)
// 使用统一的 infra 包，消除重复代码
func (a *App) startHTTPServer() {
	a.httpServer = infra.NewHTTPServer(&infra.HTTPServerConfig{
		Port:          a.cfg.Service.HTTPPort,
		DB:            a.db,
		Redis:         a.rdb,
		EnableMetrics: true,
		EnableHealth:  true,
	})
	infra.StartHTTPServer(a.httpServer)
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
		a.nonceRepo,
		&a.cfg.EIP712,
	)

	// 构建拦截器链
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		middleware.RecoveryUnaryServerInterceptor(),
		middleware.UnaryServerInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		middleware.RecoveryStreamServerInterceptor(),
		middleware.StreamServerInterceptor(),
	}

	// 如果启用了链路追踪，添加 tracing 拦截器
	if a.cfg.Tracing.Enabled {
		unaryInterceptors = append([]grpc.UnaryServerInterceptor{
			tracing.UnaryServerInterceptor(a.cfg.Service.Name),
		}, unaryInterceptors...)
		streamInterceptors = append([]grpc.StreamServerInterceptor{
			tracing.StreamServerInterceptor(a.cfg.Service.Name),
		}, streamInterceptors...)
	}

	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// 注册健康检查
	a.healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, a.healthServer)
	a.healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// 注册业务服务
	pb.RegisterTradingServiceServer(a.grpcServer, tradingHandler)

	// 注册子账户服务
	subAccountHandler := handler.NewSubAccountHandler(a.subAccountSvc)
	pb.RegisterSubAccountServiceServer(a.grpcServer, subAccountHandler)

	// 启动监听
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Service.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		logger.Info("gRPC server listening", "port", a.cfg.Service.GRPCPort)
		if err := a.grpcServer.Serve(lis); err != nil {
			logger.Error("grpc serve error", "error", err)
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
	// 提现审核结果处理器 (来自 eidos-risk)
	eventProcessor.RegisterHandler(
		kafka.TopicWithdrawalReviewResults,
		event.NewWithdrawalReviewResultHandler(a.withdrawSvc),
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

	// 关闭服务发现基础设施（会自动注销 Nacos 服务和关闭连接）
	if a.infra != nil {
		if err := a.infra.Close(); err != nil {
			logger.Error("close infrastructure failed", "error", err)
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

	// 关闭 matching 客户端
	if a.matchingClient != nil {
		a.matchingClient.Close()
	}

	// 关闭 risk 客户端
	if a.riskClient != nil {
		a.riskClient.Close()
	}

	// 优雅停止 gRPC 服务
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 停止 HTTP 服务器
	if err := infra.ShutdownHTTPServer(a.httpServer, 5*time.Second); err != nil {
		logger.Error("http server shutdown error", "error", err)
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

	// 关闭链路追踪
	if a.tracingShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.tracingShutdown(shutdownCtx); err != nil {
			logger.Error("shutdown tracing failed", "error", err)
		}
	}

	logger.Info("service stopped")
}

