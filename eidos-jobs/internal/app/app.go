// Package app 提供定时任务服务的应用入口
//
// ========================================
// eidos-jobs 服务对接总览
// ========================================
//
// ## 服务信息
// - 服务名: eidos-jobs
// - gRPC 端口: 50056
// - 数据库: eidos_jobs (PostgreSQL)
//
// ## 依赖服务
// - PostgreSQL: 数据持久化 (任务执行记录、统计数据、对账记录)
// - Redis: 分布式锁、缓存
// - Nacos: 服务注册与发现
//
// ## 任务列表
// 1. health-monitor: 健康监控 (每30秒)
// 2. cleanup-orders: 清理过期订单 (每2分钟)
// 3. stats-agg: 统计汇总 (每小时)
// 4. kline-agg: K线聚合 (每小时)
// 5. reconciliation: 链上链下对账 (每小时)
// 6. archive-data: 数据归档 (每日凌晨3点)
// 7. data-cleanup: 数据清理 (每日凌晨4点)
// 8. partition-manage: 分区管理 (每月1日)
//
// ## TODO: 上游对接 (eidos-trading)
// 需要实现 TradingClient 接口:
// - GetExpiredOrders: 获取需要过期的订单
//
// 需要实现 StatsDataProvider 接口:
// - GetHourlyTradeStats: 获取小时交易统计
// - GetDailyTradeStats: 获取日交易统计
// - GetActiveUserCount: 获取活跃用户数
// - GetNewUserCount: 获取新增用户数
//
// ## TODO: 上游对接 (eidos-matching)
// 需要实现 MatchingClient 接口:
// - ExpireOrders: 发送过期订单请求到撮合引擎
//
// ## TODO: 上游对接 (eidos-chain)
// 需要实现 ReconciliationDataProvider 接口:
// - GetOnchainBalances: 获取链上余额
// - GetChangedWallets: 获取有变动的钱包
// - GetLatestBlockNumber: 获取最新区块号
//
// ## TODO: 上游对接 (eidos-market)
// 需要实现 KlineDataProvider 接口:
// - GetMinuteKlines: 获取分钟K线数据
// - UpsertKlines: 批量保存K线数据
//
// ## TODO: 下游对接 (监控系统)
// 健康监控任务会检查以下组件:
// - 数据库连接
// - Redis 连接
// - 各服务 gRPC 健康端点
// 需要配置告警回调处理健康告警
//
// ========================================
package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/client"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/config"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	jobsv1 "github.com/eidos-exchange/eidos/proto/jobs/v1"
)

// App 定时任务服务应用
type App struct {
	cfg *config.Config

	// 基础设施
	db          *gorm.DB
	redisClient redis.UniversalClient
	grpcServer  *grpc.Server

	// 调度器
	scheduler *scheduler.Scheduler

	// 仓储层
	execRepo    *repository.ExecutionRepository
	statsRepo   *repository.StatisticsRepository
	reconRepo   *repository.ReconciliationRepository
	archiveRepo *repository.ArchiveRepository

	// gRPC 客户端
	tradingClient  *client.TradingClient
	matchingClient *client.MatchingClient
	marketClient   *client.MarketClient
	chainClient    *client.ChainClient

	// 上下文
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
	// 1. 初始化数据库
	if err := a.initDB(); err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	// 2. 初始化 Redis
	if err := a.initRedis(); err != nil {
		return fmt.Errorf("failed to init redis: %w", err)
	}

	// 3. 初始化仓储层
	a.initRepositories()

	// 4. 初始化 gRPC 客户端 (可选，失败不阻止启动)
	a.initClients()

	// 5. 初始化调度器
	a.initScheduler()

	// 6. 注册任务
	a.registerJobs()

	// 7. 启动调度器
	a.scheduler.Start()

	// 8. 启动 gRPC 服务
	if err := a.startGRPC(); err != nil {
		return fmt.Errorf("failed to start gRPC: %w", err)
	}

	return nil
}

// Shutdown 优雅关闭
func (a *App) Shutdown(ctx context.Context) error {
	logger.Info("shutting down jobs service...")

	// 停止接收新请求
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 停止调度器
	if a.scheduler != nil {
		a.scheduler.Stop()
	}

	// 关闭 gRPC 客户端
	if a.tradingClient != nil {
		a.tradingClient.Close()
	}
	if a.matchingClient != nil {
		a.matchingClient.Close()
	}
	if a.marketClient != nil {
		a.marketClient.Close()
	}
	if a.chainClient != nil {
		a.chainClient.Close()
	}

	// 关闭 Redis
	if a.redisClient != nil {
		a.redisClient.Close()
	}

	// 关闭数据库
	if a.db != nil {
		sqlDB, _ := a.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	a.cancel()
	logger.Info("jobs service stopped")
	return nil
}

// initDB 初始化数据库
func (a *App) initDB() error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		a.cfg.Postgres.Host,
		a.cfg.Postgres.Port,
		a.cfg.Postgres.User,
		a.cfg.Postgres.Password,
		a.cfg.Postgres.Database,
	)

	gormConfig := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxOpenConns(a.cfg.Postgres.MaxConnections)
	sqlDB.SetMaxIdleConns(a.cfg.Postgres.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(a.cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)

	a.db = db
	logger.Info("database connected",
		zap.String("host", a.cfg.Postgres.Host),
		zap.String("database", a.cfg.Postgres.Database))

	// 自动迁移
	if err := AutoMigrate(a.db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	logger.Info("database migrated")

	return nil
}

// initRedis 初始化 Redis
func (a *App) initRedis() error {
	a.redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", a.cfg.Redis.Host, a.cfg.Redis.Port),
		Password: a.cfg.Redis.Password,
		DB:       a.cfg.Redis.DB,
		PoolSize: a.cfg.Redis.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.redisClient.Ping(ctx).Err(); err != nil {
		return err
	}

	logger.Info("redis connected",
		zap.String("host", a.cfg.Redis.Host),
		zap.Int("db", a.cfg.Redis.DB))

	return nil
}

// initRepositories 初始化仓储层
func (a *App) initRepositories() {
	a.execRepo = repository.NewExecutionRepository(a.db)
	a.statsRepo = repository.NewStatisticsRepository(a.db)
	a.reconRepo = repository.NewReconciliationRepository(a.db)
	a.archiveRepo = repository.NewArchiveRepository(a.db)

	logger.Info("repositories initialized")
}

// initClients 初始化 gRPC 客户端
// 客户端初始化失败不阻止服务启动，任务会使用 Mock 实现
func (a *App) initClients() {
	var err error

	// Trading 客户端
	if a.cfg.GRPCClients.Trading != "" {
		a.tradingClient, err = client.NewTradingClient(a.cfg.GRPCClients.Trading)
		if err != nil {
			logger.Warn("failed to connect to trading service, using mock",
				zap.String("addr", a.cfg.GRPCClients.Trading),
				zap.Error(err))
		}
	}

	// Matching 客户端
	if a.cfg.GRPCClients.Matching != "" {
		a.matchingClient, err = client.NewMatchingClient(a.cfg.GRPCClients.Matching, nil) // TODO: pass Kafka producer
		if err != nil {
			logger.Warn("failed to connect to matching service, using mock",
				zap.String("addr", a.cfg.GRPCClients.Matching),
				zap.Error(err))
		}
	}

	// Market 客户端
	if a.cfg.GRPCClients.Market != "" {
		a.marketClient, err = client.NewMarketClient(a.cfg.GRPCClients.Market)
		if err != nil {
			logger.Warn("failed to connect to market service, using mock",
				zap.String("addr", a.cfg.GRPCClients.Market),
				zap.Error(err))
		}
	}

	// Chain 客户端
	if a.cfg.GRPCClients.Chain != "" {
		a.chainClient, err = client.NewChainClient(a.cfg.GRPCClients.Chain)
		if err != nil {
			logger.Warn("failed to connect to chain service, using mock",
				zap.String("addr", a.cfg.GRPCClients.Chain),
				zap.Error(err))
		}
	}

	logger.Info("gRPC clients initialized")
}

// initScheduler 初始化调度器
func (a *App) initScheduler() {
	maxConcurrent := a.cfg.Scheduler.MaxConcurrentJobs
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	a.scheduler = scheduler.NewScheduler(
		&scheduler.SchedulerConfig{
			MaxConcurrentJobs: maxConcurrent,
			RedisClient:       a.redisClient,
		},
		a.execRepo,
	)

	logger.Info("scheduler initialized",
		zap.Int("max_concurrent_jobs", maxConcurrent))
}

// registerJobs 注册任务
func (a *App) registerJobs() {
	// 1. 健康监控任务 (不需要分布式锁)
	if a.cfg.Jobs.HealthMonitor.Enabled {
		healthJob := jobs.NewHealthMonitorJob(
			a.db,
			a.redisClient,
			a.buildServiceEndpoints(),
		)
		a.scheduler.RegisterJob(healthJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameHealthMonitor, a.cfg.Jobs.HealthMonitor.Cron),
			Enabled: true,
		})
	}

	// 2. 清理过期订单任务
	if a.cfg.Jobs.CleanupOrders.Enabled {
		var tradingClient jobs.TradingClient = &jobs.MockTradingClient{}
		var matchingClient jobs.MatchingClient = &jobs.MockMatchingClient{}
		if a.tradingClient != nil {
			tradingClient = a.tradingClient
		}
		if a.matchingClient != nil {
			matchingClient = a.matchingClient
		}
		cleanupJob := jobs.NewCleanupOrdersJob(tradingClient, matchingClient)
		a.scheduler.RegisterJob(cleanupJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameCleanupOrders, a.cfg.Jobs.CleanupOrders.Cron),
			Enabled: true,
		})
	}

	// 3. 统计汇总任务
	if a.cfg.Jobs.StatsAgg.Enabled {
		var statsProvider jobs.StatsDataProvider = &jobs.MockStatsDataProvider{}
		if a.tradingClient != nil {
			statsProvider = client.NewStatsDataProvider(a.tradingClient)
		}
		statsJob := jobs.NewStatsAggJob(a.statsRepo, statsProvider)
		a.scheduler.RegisterJob(statsJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameStatsAgg, a.cfg.Jobs.StatsAgg.Cron),
			Enabled: true,
		})
	}

	// 4. K线聚合任务
	if a.cfg.Jobs.KlineAgg.Enabled {
		var klineProvider jobs.KlineDataProvider = &jobs.MockKlineDataProvider{}
		if a.marketClient != nil {
			klineProvider = a.marketClient
		}
		klineJob := jobs.NewKlineAggJob(klineProvider)
		a.scheduler.RegisterJob(klineJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameKlineAgg, a.cfg.Jobs.KlineAgg.Cron),
			Enabled: true,
		})
	}

	// 5. 对账任务
	if a.cfg.Jobs.Reconciliation.Enabled {
		var reconProvider jobs.ReconciliationDataProvider = &jobs.MockReconciliationDataProvider{}
		if a.chainClient != nil && a.tradingClient != nil {
			reconProvider = client.NewReconciliationDataProvider(a.chainClient, a.tradingClient)
		}
		reconJob := jobs.NewReconciliationJob(a.reconRepo, reconProvider)
		a.scheduler.RegisterJob(reconJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameReconciliation, a.cfg.Jobs.Reconciliation.Cron),
			Enabled: true,
		})
	}

	// 6. 数据归档任务
	if a.cfg.Jobs.Archive.Enabled {
		archiveProvider := client.NewArchiveDataProvider(a.db, nil)
		archiveJob := jobs.NewArchiveDataJob(
			a.archiveRepo,
			archiveProvider,
			model.ArchivableTables,
		)
		a.scheduler.RegisterJob(archiveJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameArchiveData, a.cfg.Jobs.Archive.Cron),
			Enabled: true,
		})
	}

	// 7. 数据清理任务
	if a.cfg.Jobs.DataCleanup.Enabled {
		cleanupJob := jobs.NewDataCleanupJob(
			a.db,
			jobs.DefaultDataCleanupConfig,
		)
		a.scheduler.RegisterJob(cleanupJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameDataCleanup, a.cfg.Jobs.DataCleanup.Cron),
			Enabled: true,
		})
	}

	// 8. 分区管理任务
	if a.cfg.Jobs.PartitionMgmt.Enabled {
		partitionJob := jobs.NewPartitionManageJob(a.db, jobs.DefaultPartitionTables)
		a.scheduler.RegisterJob(partitionJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNamePartitionManage, a.cfg.Jobs.PartitionMgmt.Cron),
			Enabled: true,
		})
	}

	// 9. 余额扫描取消订单任务
	if a.cfg.Jobs.BalanceScan.Enabled {
		var balanceScanProvider jobs.BalanceScanDataProvider = &jobs.MockBalanceScanDataProvider{}
		if a.chainClient != nil && a.tradingClient != nil {
			balanceScanProvider = client.NewBalanceScanDataProvider(a.chainClient, a.tradingClient)
		}
		balanceScanConfig := &jobs.BalanceScanConfig{
			BatchSize:              a.cfg.Jobs.BalanceScan.BatchSize,
			CancelThresholdPercent: a.cfg.Jobs.BalanceScan.CancelThresholdPercent,
			MaxConcurrentQueries:   a.cfg.Jobs.BalanceScan.MaxConcurrentQueries,
			EnableNotification:     a.cfg.Jobs.BalanceScan.EnableNotification,
		}
		if balanceScanConfig.BatchSize <= 0 {
			balanceScanConfig.BatchSize = jobs.DefaultBalanceScanConfig.BatchSize
		}
		if balanceScanConfig.MaxConcurrentQueries <= 0 {
			balanceScanConfig.MaxConcurrentQueries = jobs.DefaultBalanceScanConfig.MaxConcurrentQueries
		}
		balanceScanJob := jobs.NewBalanceScanJob(balanceScanProvider, balanceScanConfig)
		a.scheduler.RegisterJob(balanceScanJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameBalanceScan, a.cfg.Jobs.BalanceScan.Cron),
			Enabled: true,
		})
	}

	// 10. 结算触发任务
	if a.cfg.Jobs.SettlementTrigger.Enabled {
		var settlementProvider jobs.SettlementDataProvider = &jobs.MockSettlementDataProvider{}
		if a.chainClient != nil && a.tradingClient != nil {
			settlementProvider = client.NewSettlementDataProvider(a.chainClient, a.tradingClient)
		}
		settlementConfig := &jobs.SettlementConfig{
			BatchSize:         a.cfg.Jobs.SettlementTrigger.BatchSize,
			MinBatchSize:      a.cfg.Jobs.SettlementTrigger.MinBatchSize,
			MaxWaitTime:       time.Duration(a.cfg.Jobs.SettlementTrigger.MaxWaitTimeMs) * time.Millisecond,
			RetryTimeout:      time.Duration(a.cfg.Jobs.SettlementTrigger.RetryTimeoutMs) * time.Millisecond,
			MaxRetries:        a.cfg.Jobs.SettlementTrigger.MaxRetries,
			ConcurrentBatches: a.cfg.Jobs.SettlementTrigger.ConcurrentBatches,
		}
		if settlementConfig.BatchSize <= 0 {
			settlementConfig.BatchSize = jobs.DefaultSettlementConfig.BatchSize
		}
		if settlementConfig.MinBatchSize <= 0 {
			settlementConfig.MinBatchSize = jobs.DefaultSettlementConfig.MinBatchSize
		}
		if settlementConfig.MaxWaitTime <= 0 {
			settlementConfig.MaxWaitTime = jobs.DefaultSettlementConfig.MaxWaitTime
		}
		if settlementConfig.RetryTimeout <= 0 {
			settlementConfig.RetryTimeout = jobs.DefaultSettlementConfig.RetryTimeout
		}
		if settlementConfig.MaxRetries <= 0 {
			settlementConfig.MaxRetries = jobs.DefaultSettlementConfig.MaxRetries
		}
		settlementJob := jobs.NewSettlementTriggerJob(settlementProvider, settlementConfig)
		a.scheduler.RegisterJob(settlementJob, scheduler.JobConfig{
			Cron:    a.getJobCron(scheduler.JobNameSettlementTrigger, a.cfg.Jobs.SettlementTrigger.Cron),
			Enabled: true,
		})
	}

	logger.Info("jobs registered")
}

// getJobCron 获取任务的 cron 表达式 (优先使用配置，否则使用默认值)
func (a *App) getJobCron(jobName string, configCron string) string {
	if configCron != "" {
		return configCron
	}
	if defaultCfg, ok := scheduler.DefaultJobConfigs[jobName]; ok {
		return defaultCfg.Cron
	}
	return ""
}

// buildServiceEndpoints 构建服务端点列表
func (a *App) buildServiceEndpoints() []jobs.ServiceEndpoint {
	endpoints := make([]jobs.ServiceEndpoint, 0)

	// 添加各服务的健康检查端点
	if a.cfg.GRPCClients.Trading != "" {
		endpoints = append(endpoints, jobs.ServiceEndpoint{
			Name:    "eidos-trading",
			URL:     fmt.Sprintf("http://%s/health", a.cfg.GRPCClients.Trading),
			Timeout: 5 * time.Second,
		})
	}
	if a.cfg.GRPCClients.Chain != "" {
		endpoints = append(endpoints, jobs.ServiceEndpoint{
			Name:    "eidos-chain",
			URL:     fmt.Sprintf("http://%s/health", a.cfg.GRPCClients.Chain),
			Timeout: 5 * time.Second,
		})
	}
	if a.cfg.GRPCClients.Market != "" {
		endpoints = append(endpoints, jobs.ServiceEndpoint{
			Name:    "eidos-market",
			URL:     fmt.Sprintf("http://%s/health", a.cfg.GRPCClients.Market),
			Timeout: 5 * time.Second,
		})
	}
	if a.cfg.GRPCClients.Matching != "" {
		endpoints = append(endpoints, jobs.ServiceEndpoint{
			Name:    "eidos-matching",
			URL:     fmt.Sprintf("http://%s/health", a.cfg.GRPCClients.Matching),
			Timeout: 5 * time.Second,
		})
	}

	return endpoints
}

// startGRPC 启动 gRPC 服务
func (a *App) startGRPC() error {
	addr := fmt.Sprintf(":%d", a.cfg.Service.GRPCPort)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// 创建 gRPC 服务器
	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor(),
			middleware.UnaryServerInterceptor(),
		),
	)

	// 注册任务服务
	jobsHandler := handler.NewJobsHandler(
		a.scheduler,
		a.execRepo,
		a.statsRepo,
		a.reconRepo,
	)
	jobsv1.RegisterJobsServiceServer(a.grpcServer, jobsHandler)

	// 注册健康检查
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, healthServer)
	healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_SERVING)

	logger.Info("starting gRPC server",
		zap.String("addr", addr),
		zap.String("service", a.cfg.Service.Name))

	go func() {
		if err := a.grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// GetConfig 获取配置
func (a *App) GetConfig() *config.Config {
	return a.cfg
}

// GetScheduler 获取调度器 (用于测试)
func (a *App) GetScheduler() *scheduler.Scheduler {
	return a.scheduler
}
