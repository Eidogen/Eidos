// Package app 提供 eidos-chain 服务的应用生命周期管理
//
// ========================================
// eidos-chain 服务对接说明
// ========================================
//
// ## 服务职责
// eidos-chain 是链上服务，负责:
// 1. 链上结算 (Settlement): 批量收集成交，提交到区块链
// 2. 链上提现 (Withdrawal): 处理用户提现请求，调用合约
// 3. 链上索引 (Indexer): 监听链上事件，检测充值
//
// ## Kafka 对接 (参见 internal/kafka/consumer.go 和 producer.go)
//
// ### 消费的 Topic (来自 eidos-trading)
// - settlements: 成交记录，需要上链结算
// - withdrawals: 提现请求，需要上链执行
//
// ### 生产的 Topic (发送给 eidos-trading)
// - deposits: 链上检测到的充值事件
// - settlement-confirmed: 结算交易上链确认
// - withdrawal-confirmed: 提现交易上链确认
//
// ## gRPC 对接
// - 端口: 50054
// - Proto: proto/chain/v1/chain.proto
// - TODO: 生成 proto 并注册 gRPC 服务
//
// ## 智能合约对接
// - TODO: 部署 Exchange 合约并更新 contract_address 配置
// - TODO: 实现合约 ABI 绑定 (internal/blockchain/contracts/)
// - 当前为 Mock 模式 (verifyingContract = 0x0)
//
// ## 数据库
// - 数据库名: eidos_chain
// - 迁移文件: migrations/
//
// ========================================
package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/config"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	chainv1 "github.com/eidos-exchange/eidos/proto/chain/v1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// App 应用
type App struct {
	cfg *config.Config

	// 基础设施
	db    *gorm.DB
	redis *redis.Client

	// 区块链
	blockchainClient *blockchain.Client
	nonceManager     *blockchain.NonceManager

	// 仓储
	settlementRepo     repository.SettlementRepository
	withdrawalRepo     repository.WithdrawalRepository
	depositRepo        repository.DepositRepository
	checkpointRepo     repository.CheckpointRepository
	nonceRepo          repository.NonceRepository
	reconciliationRepo repository.ReconciliationRepository

	// 服务
	settlementSvc     *service.SettlementService
	withdrawalSvc     *service.WithdrawalService
	indexerSvc        *service.IndexerService
	reconciliationSvc *service.ReconciliationService

	// Kafka
	kafkaConsumer *kafka.Consumer
	kafkaProducer *kafka.Producer
	eventPublisher *kafka.KafkaEventPublisher

	// gRPC
	grpcServer    *grpc.Server
	healthServer  *health.Server
	chainHandler  *handler.ChainHandler

	// 运行控制
	stopCh chan struct{}
}

// NewApp 创建应用
func NewApp(cfg *config.Config) (*App, error) {
	app := &App{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}

	if err := app.initInfrastructure(); err != nil {
		return nil, fmt.Errorf("failed to init infrastructure: %w", err)
	}

	if err := app.initBlockchain(); err != nil {
		return nil, fmt.Errorf("failed to init blockchain: %w", err)
	}

	app.initRepositories()

	if err := app.initServices(); err != nil {
		return nil, fmt.Errorf("failed to init services: %w", err)
	}

	if err := app.initKafka(); err != nil {
		return nil, fmt.Errorf("failed to init kafka: %w", err)
	}

	app.initGRPC()

	return app, nil
}

// initInfrastructure 初始化基础设施
func (a *App) initInfrastructure() error {
	// PostgreSQL
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		a.cfg.Postgres.Host,
		a.cfg.Postgres.Port,
		a.cfg.Postgres.User,
		a.cfg.Postgres.Password,
		a.cfg.Postgres.Database,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(a.cfg.Postgres.MaxConnections)
	sqlDB.SetMaxIdleConns(a.cfg.Postgres.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(a.cfg.Postgres.ConnMaxLifetime) * time.Second)

	a.db = db
	logger.Info("database connected", zap.String("host", a.cfg.Postgres.Host))

	// 自动迁移
	if err := AutoMigrate(a.db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	logger.Info("database migrated")

	// Redis
	redisAddr := "localhost:6379"
	if len(a.cfg.Redis.Addresses) > 0 {
		redisAddr = a.cfg.Redis.Addresses[0]
	}

	a.redis = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: a.cfg.Redis.Password,
		DB:       a.cfg.Redis.DB,
		PoolSize: a.cfg.Redis.PoolSize,
	})

	if err := a.redis.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("failed to connect redis: %w", err)
	}

	logger.Info("redis connected", zap.String("addr", redisAddr))

	return nil
}

// initBlockchain 初始化区块链客户端
func (a *App) initBlockchain() error {
	// 创建区块链客户端
	rpcURLs := []string{a.cfg.Blockchain.RPCURL}

	client, err := blockchain.NewClient(&blockchain.ClientConfig{
		ChainID:         a.cfg.Blockchain.ChainID,
		PrivateKey:      a.cfg.Blockchain.PrivateKey,
		RPCURLs:         rpcURLs,
		MaxRetries:      3,
		RetryInterval:   time.Second,
		HealthCheckFreq: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create blockchain client: %w", err)
	}

	a.blockchainClient = client

	// 创建 Nonce 管理器
	a.nonceManager = blockchain.NewNonceManager(client, a.redis, &blockchain.NonceManagerConfig{
		Wallet:       client.Address(),
		ChainID:      a.cfg.Blockchain.ChainID,
		LockTimeout:  30 * time.Second,
		SyncInterval: 5 * time.Minute,
		MaxPending:   100,
	})

	logger.Info("blockchain client initialized",
		zap.Int64("chain_id", a.cfg.Blockchain.ChainID),
		zap.String("wallet", client.Address().Hex()))

	return nil
}

// initRepositories 初始化仓储
func (a *App) initRepositories() {
	a.settlementRepo = repository.NewSettlementRepository(a.db)
	a.withdrawalRepo = repository.NewWithdrawalRepository(a.db)
	a.depositRepo = repository.NewDepositRepository(a.db)
	a.checkpointRepo = repository.NewCheckpointRepository(a.db)
	a.nonceRepo = repository.NewNonceRepository(a.db)
	a.reconciliationRepo = repository.NewReconciliationRepository(a.db)

	logger.Info("repositories initialized")
}

// initServices 初始化服务
func (a *App) initServices() error {
	// 结算服务
	a.settlementSvc = service.NewSettlementService(
		a.settlementRepo,
		a.nonceRepo,
		a.blockchainClient,
		a.nonceManager,
		&service.SettlementServiceConfig{
			BatchSize:        a.cfg.Settlement.BatchSize,
			BatchInterval:    time.Duration(a.cfg.Settlement.BatchInterval) * time.Second,
			MaxRetries:       a.cfg.Settlement.MaxRetries,
			RetryBackoff:     time.Duration(a.cfg.Settlement.RetryBackoff) * time.Second,
			ChainID:          a.cfg.Blockchain.ChainID,
			ExchangeContract: common.HexToAddress(a.cfg.Blockchain.ContractAddress),
		},
	)

	// 提现服务
	a.withdrawalSvc = service.NewWithdrawalService(
		a.withdrawalRepo,
		a.nonceRepo,
		a.blockchainClient,
		a.nonceManager,
		&service.WithdrawalServiceConfig{
			MaxRetries:    a.cfg.Settlement.MaxRetries,
			RetryBackoff:  time.Duration(a.cfg.Settlement.RetryBackoff) * time.Second,
			ChainID:       a.cfg.Blockchain.ChainID,
			VaultContract: common.HexToAddress(a.cfg.Blockchain.ContractAddress),
		},
	)

	// 索引服务
	a.indexerSvc = service.NewIndexerService(
		a.blockchainClient,
		a.checkpointRepo,
		a.depositRepo,
		&service.IndexerServiceConfig{
			ChainID:              a.cfg.Blockchain.ChainID,
			PollInterval:         time.Second,
			CheckpointInterval:   10,
			RequiredConfirms:     a.cfg.Blockchain.Confirmations.Deposit,
			VaultContractAddress: common.HexToAddress(a.cfg.Blockchain.ContractAddress),
		},
	)

	logger.Info("services initialized")
	return nil
}

// initKafka 初始化 Kafka
func (a *App) initKafka() error {
	// 生产者
	producer, err := kafka.NewProducer(&kafka.ProducerConfig{
		Brokers:  a.cfg.Kafka.Brokers,
		ClientID: a.cfg.Kafka.ClientID,
	})
	if err != nil {
		return fmt.Errorf("failed to create kafka producer: %w", err)
	}
	a.kafkaProducer = producer
	a.eventPublisher = kafka.NewKafkaEventPublisher(producer)

	// 设置事件回调
	a.settlementSvc.SetOnSettlementConfirmed(a.eventPublisher.PublishSettlementConfirmation)
	a.withdrawalSvc.SetOnWithdrawalConfirmed(a.eventPublisher.PublishWithdrawalConfirmation)
	a.indexerSvc.SetOnDepositDetected(a.eventPublisher.PublishDeposit)

	// 消费者
	consumer, err := kafka.NewConsumer(&kafka.ConsumerConfig{
		Brokers:           a.cfg.Kafka.Brokers,
		GroupID:           a.cfg.Kafka.GroupID,
		SettlementService: a.settlementSvc,
		WithdrawalService: a.withdrawalSvc,
	})
	if err != nil {
		return fmt.Errorf("failed to create kafka consumer: %w", err)
	}
	a.kafkaConsumer = consumer

	logger.Info("kafka initialized", zap.Strings("brokers", a.cfg.Kafka.Brokers))
	return nil
}

// initGRPC 初始化 gRPC
func (a *App) initGRPC() {
	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor(),
			middleware.UnaryServerInterceptor(),
		),
	)

	a.healthServer = health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, a.healthServer)

	a.chainHandler = handler.NewChainHandler(
		a.settlementSvc,
		a.withdrawalSvc,
		a.indexerSvc,
		a.settlementRepo,
		a.withdrawalRepo,
		a.depositRepo,
		a.reconciliationRepo,
		a.blockchainClient,
		a.nonceManager,
	)

	// 创建对账服务
	a.reconciliationSvc = service.NewReconciliationService(
		a.blockchainClient,
		a.reconciliationRepo,
		nil, // TODO: 配置 BalanceProvider (需要连接 eidos-trading)
		nil, // 使用默认配置
	)

	// 创建 gRPC Handler 并注册服务
	grpcHandler := handler.NewGRPCHandler(
		a.settlementSvc,
		a.withdrawalSvc,
		a.indexerSvc,
		a.reconciliationSvc,
		a.settlementRepo,
		a.withdrawalRepo,
		a.depositRepo,
		a.reconciliationRepo,
		a.blockchainClient,
		a.nonceManager,
	)
	chainv1.RegisterChainServiceServer(a.grpcServer, grpcHandler)

	logger.Info("grpc server initialized with ChainService")
}

// Run 运行应用
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动 Kafka 消费者
	if err := a.kafkaConsumer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start kafka consumer: %w", err)
	}

	// 启动索引器
	if err := a.indexerSvc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start indexer: %w", err)
	}

	// 启动后台任务
	go a.runBackgroundTasks(ctx)

	// 启动 gRPC 服务器
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Service.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		logger.Info("gRPC server listening", zap.Int("port", a.cfg.Service.GRPCPort))
		if err := a.grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info("received shutdown signal")
	case <-a.stopCh:
		logger.Info("shutdown requested")
	}

	return a.shutdown()
}

// runBackgroundTasks 运行后台任务
func (a *App) runBackgroundTasks(ctx context.Context) {
	// 定时刷新批次
	flushTicker := time.NewTicker(time.Second)
	defer flushTicker.Stop()

	// 定时处理待提交批次
	processTicker := time.NewTicker(5 * time.Second)
	defer processTicker.Stop()

	// 定时更新确认数
	confirmTicker := time.NewTicker(10 * time.Second)
	defer confirmTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-flushTicker.C:
			if err := a.settlementSvc.FlushIfNeeded(ctx); err != nil {
				logger.Error("failed to flush settlement batch", zap.Error(err))
			}
		case <-processTicker.C:
			if err := a.settlementSvc.ProcessPendingBatches(ctx); err != nil {
				logger.Error("failed to process pending batches", zap.Error(err))
			}
			if err := a.withdrawalSvc.ProcessPendingWithdrawals(ctx); err != nil {
				logger.Error("failed to process pending withdrawals", zap.Error(err))
			}
		case <-confirmTicker.C:
			if err := a.indexerSvc.UpdateConfirmations(ctx); err != nil {
				logger.Error("failed to update confirmations", zap.Error(err))
			}
		}
	}
}

// shutdown 关闭应用
func (a *App) shutdown() error {
	logger.Info("shutting down...")

	a.healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// 停止 Kafka 消费者
	if a.kafkaConsumer != nil {
		a.kafkaConsumer.Stop()
	}

	// 停止索引器
	if a.indexerSvc != nil {
		a.indexerSvc.Stop()
	}

	// 关闭 gRPC 服务器
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 关闭 Kafka 生产者
	if a.kafkaProducer != nil {
		a.kafkaProducer.Close()
	}

	// 关闭区块链客户端
	if a.blockchainClient != nil {
		a.blockchainClient.Close()
	}

	// 关闭 Redis
	if a.redis != nil {
		a.redis.Close()
	}

	// 关闭数据库
	if a.db != nil {
		sqlDB, _ := a.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	logger.Info("shutdown complete")
	return nil
}

// Stop 停止应用
func (a *App) Stop() {
	close(a.stopCh)
}
