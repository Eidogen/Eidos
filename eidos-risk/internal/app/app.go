// Package app 提供风控服务的应用入口
//
// ========================================
// eidos-risk 服务对接总览
// ========================================
//
// ## 服务信息
// - 服务名: eidos-risk
// - gRPC 端口: 50055
// - 数据库: eidos_risk (PostgreSQL)
//
// ## 依赖服务
// - PostgreSQL: 数据持久化
// - Redis: 缓存 (黑名单、频率限制、金额统计)
// - Kafka: 消息队列 (订单更新、成交结果、余额更新、提现)
// - Nacos: 服务注册与发现
//
// ## Kafka 主题
// - 消费: order-updates, trade-results, balance-updates, withdrawals
// - 生产: risk-alerts
//
// ## 上游对接 (eidos-api)
// eidos-api 需要在以下时机调用本服务:
//
// 1. 下单前调用 CheckOrder
//   - 时机: 签名验证通过后，发送到 matching 之前
//   - 调用: riskv1.CheckOrder(wallet, market, side, type, price, amount)
//   - 处理: approved=false 时返回 reject_code 给客户端
//
// 2. 提现前调用 CheckWithdraw
//   - 时机: 签名验证通过后，创建提现记录之前
//   - 调用: riskv1.CheckWithdraw(wallet, token, amount, to_address)
//   - 处理: require_manual_review=true 时标记提现为待审核
//
// ## 上游对接 (eidos-trading)
// eidos-trading 需要发送以下 Kafka 消息:
//
// 1. 订单状态变更 -> order-updates
//   - OPEN/PARTIAL: 订单进入订单簿
//   - FILLED/CANCELLED: 订单从订单簿移除
//
// 2. 成交通知 -> trade-results
//   - 每笔成交后发送，用于更新待结算金额
//
// 3. 余额变更 -> balance-updates
//   - 结算完成后发送 SETTLE 类型
//
// ## 上游对接 (eidos-chain)
// 1. 提现确认 -> withdrawals
//   - 链上提现确认后发送 CONFIRMED 状态
//
// ## 下游对接 (监控告警)
// 1. 消费 risk-alerts 主题
//   - 接入告警系统 (钉钉/Slack/PagerDuty)
//   - severity: info/warning/critical
//
// ========================================
package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/discovery"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/migrate"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/tracing"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/config"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/infra"
)

// App 风控服务应用
type App struct {
	cfg *config.Config

	// 基础设施
	infra       *discovery.Infrastructure // 统一服务发现基础设施
	db          *gorm.DB
	redisClient redis.UniversalClient
	grpcServer  *grpc.Server
	httpServer  *http.Server // HTTP 服务 (metrics + health)

	// Kafka
	kafkaProducer *kafka.Producer
	kafkaConsumer *kafka.Consumer

	// 缓存层
	rateLimitCache *cache.RateLimitCache

	// 服务层
	riskSvc            *service.RiskService
	blacklistSvc       *service.BlacklistService
	ruleSvc            *service.RuleService
	eventSvc           *service.EventService
	withdrawReviewSvc  *service.WithdrawalReviewService
	whitelistSvc       *service.WhitelistService
	alertSvc           *service.AlertService
	tradeMonitorSvc    *service.TradeMonitorService   // 交易监控服务 (后置风控)
	degradationSvc     *service.DegradationService    // 服务降级管理

	// 链路追踪
	tracingShutdown func(context.Context) error

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
	// 0. 初始化链路追踪
	if err := a.initTracing(); err != nil {
		logger.Warn("init tracing failed, tracing disabled", "error", err)
	}

	// 1. 初始化服务发现基础设施
	if err := a.initInfra(); err != nil {
		return fmt.Errorf("failed to init infrastructure: %w", err)
	}

	// 2. 确保数据库存在
	if err := migrate.EnsureDatabase(a.cfg.Postgres.Host, a.cfg.Postgres.Port, a.cfg.Postgres.User, a.cfg.Postgres.Password, a.cfg.Postgres.Database); err != nil {
		return fmt.Errorf("failed to ensure database: %w", err)
	}

	// 3. 初始化数据库
	if err := a.initDB(); err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	// 4. 初始化 Redis
	if err := a.initRedis(); err != nil {
		return fmt.Errorf("failed to init redis: %w", err)
	}

	// 5. 初始化 Kafka
	if err := a.initKafka(); err != nil {
		logger.Warn("failed to init kafka, running without kafka", "error", err)
	}

	// 6. 初始化服务层
	a.initServices()

	// 7. 初始化缓存 (从数据库加载)
	a.warmupCache()

	// 8. 启动 gRPC 服务
	if err := a.startGRPC(); err != nil {
		return fmt.Errorf("failed to start gRPC: %w", err)
	}

	return nil
}

// Shutdown 优雅关闭
func (a *App) Shutdown(ctx context.Context) error {
	logger.Info("shutting down risk service...")

	// 关闭顺序：服务发现注销 -> 服务端 -> 消息队列 -> 数据库 -> 缓存
	// 1. 先关闭服务发现（会从 Nacos 注销，停止新流量进入）
	if a.infra != nil {
		if err := a.infra.Close(); err != nil {
			logger.Warn("close infrastructure failed", "error", err)
		}
	}

	// 2. 停止 HTTP 服务器
	if err := infra.ShutdownHTTPServer(a.httpServer, 5*time.Second); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}

	// 3. 停止 gRPC 服务器（等待现有请求完成）
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 4. 停止 Kafka 消费者
	if a.kafkaConsumer != nil {
		a.kafkaConsumer.Stop()
	}

	// 4.5 停止服务降级监控
	if a.degradationSvc != nil {
		a.degradationSvc.Stop()
	}

	// 5. 关闭 Kafka 生产者
	if a.kafkaProducer != nil {
		a.kafkaProducer.Close()
	}

	// 6. 关闭数据库
	if a.db != nil {
		sqlDB, _ := a.db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	// 7. 关闭 Redis（最后关闭缓存层）
	if a.redisClient != nil {
		a.redisClient.Close()
	}

	// 8. 关闭链路追踪
	if a.tracingShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.tracingShutdown(shutdownCtx); err != nil {
			logger.Error("shutdown tracing failed", "error", err)
		}
	}

	a.cancel()
	logger.Info("risk service stopped")
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
	opts := discovery.InitOptionsFromNacosConfig(&a.cfg.Nacos, a.cfg.Service.Name, a.cfg.Service.GRPCPort, a.cfg.Service.HTTPPort)
	opts.WithMetadata("version", "1.0.0")
	opts.WithMetadata("env", a.cfg.Service.Env)
	// 风控服务是被依赖方，不主动依赖其他服务
	opts.WithKafkaTopics(
		[]string{"risk-alerts"},
		[]string{"trade-results", "order-updates", "balance-updates"},
	)

	var err error
	a.infra, err = discovery.NewInfrastructure(opts)
	if err != nil {
		return fmt.Errorf("create infrastructure: %w", err)
	}

	// 注册服务到 Nacos
	if err := a.infra.RegisterService(nil); err != nil {
		return fmt.Errorf("register service: %w", err)
	}

	// 发布配置到 Nacos 配置中心
	if err := a.infra.PublishServiceConfig(a.cfg); err != nil {
		logger.Warn("failed to publish config to nacos", "error", err)
	}

	logger.Info("service registered to nacos")
	return nil
}

// initDB 初始化数据库
func (a *App) initDB() error {
	// 使用统一基础设施初始化
	db, err := infra.NewDatabase(&a.cfg.Postgres)
	if err != nil {
		return err
	}
	a.db = db

	// 自动迁移
	if err := AutoMigrate(a.db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	logger.Info("database migrated")

	return nil
}

// initRedis 初始化 Redis
func (a *App) initRedis() error {
	// 使用统一基础设施初始化
	a.redisClient = infra.NewRedisUniversalClient(&a.cfg.Redis)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.redisClient.Ping(ctx).Err(); err != nil {
		return err
	}

	return nil
}

// initKafka 初始化 Kafka
func (a *App) initKafka() error {
	if !a.cfg.Kafka.Enabled || len(a.cfg.Kafka.Brokers) == 0 {
		logger.Info("kafka disabled")
		return nil
	}

	// 创建生产者
	producer, err := kafka.NewProducer(a.cfg.Kafka.Brokers, a.cfg.Kafka.ClientID)
	if err != nil {
		return err
	}
	a.kafkaProducer = producer

	logger.Info("kafka producer initialized",
		"brokers", a.cfg.Kafka.Brokers)

	return nil
}

// initServices 初始化服务层
func (a *App) initServices() {
	// 创建仓储层
	ruleRepo := repository.NewRiskRuleRepository(a.db)
	eventRepo := repository.NewRiskEventRepository(a.db)
	blacklistRepo := repository.NewBlacklistRepository(a.db)
	auditRepo := repository.NewAuditLogRepository(a.db)
	withdrawReviewRepo := repository.NewWithdrawalReviewRepository(a.db)
	whitelistRepo := repository.NewWhitelistRepository(a.db)

	// 创建缓存层
	blacklistCache := cache.NewBlacklistCache(a.redisClient)
	a.rateLimitCache = cache.NewRateLimitCache(a.redisClient)
	amountCache := cache.NewAmountCache(a.redisClient)
	orderCache := cache.NewOrderCache(a.redisClient)
	marketCache := cache.NewMarketCache(a.redisClient)
	withdrawCache := cache.NewWithdrawCache(a.redisClient)

	// 创建服务层
	a.riskSvc = service.NewRiskService(
		ruleRepo,
		eventRepo,
		blacklistRepo,
		auditRepo,
		withdrawReviewRepo,
		blacklistCache,
		a.rateLimitCache,
		amountCache,
		orderCache,
		marketCache,
		withdrawCache,
		a.cfg,
	)

	a.blacklistSvc = service.NewBlacklistService(blacklistRepo, blacklistCache)
	a.ruleSvc = service.NewRuleService(ruleRepo)
	a.eventSvc = service.NewEventService(eventRepo)

	// 初始化白名单服务
	a.whitelistSvc = service.NewWhitelistService(whitelistRepo, withdrawCache)

	// 初始化告警服务
	a.alertSvc = service.NewAlertService(a.redisClient)

	// 初始化服务降级管理
	a.degradationSvc = service.NewDegradationService(a.redisClient, a.db, nil)
	a.degradationSvc.Start(a.ctx)
	// 将降级服务注入到风控服务
	a.riskSvc.SetDegradationService(a.degradationSvc)

	// 初始化提现审核服务
	a.withdrawReviewSvc = service.NewWithdrawalReviewService(withdrawReviewRepo)
	// 配置审核阈值（可从配置文件读取）
	a.withdrawReviewSvc.SetConfig(
		a.cfg.Risk.Withdrawal.AutoApproveThreshold, // 低于此分数自动通过（默认30）
		a.cfg.Risk.Withdrawal.AutoRejectThreshold,  // 高于此分数自动拒绝（默认80）
		time.Duration(a.cfg.Risk.Withdrawal.ReviewTimeoutHours)*time.Hour, // 审核超时时间（默认24小时）
	)

	// 设置 Kafka 回调
	if a.kafkaProducer != nil {
		alertCallback := a.kafkaProducer.RiskAlertCallback()
		a.riskSvc.SetOnRiskAlert(alertCallback)
		a.blacklistSvc.SetOnRiskAlert(alertCallback)
		a.ruleSvc.SetOnRiskAlert(alertCallback)

		// 设置提现审核回调 - 审核结果发送到 Kafka 通知 eidos-trading
		a.withdrawReviewSvc.SetOnApproved(a.kafkaProducer.WithdrawalApprovedCallback())
		a.withdrawReviewSvc.SetOnRejected(a.kafkaProducer.WithdrawalRejectedCallback())
		a.withdrawReviewSvc.SetOnRiskAlert(alertCallback)
	}

	// 启动提现审核后台任务（处理过期审核）
	a.withdrawReviewSvc.StartBackgroundTasks(a.ctx)

	// 创建交易监控服务 (后置风控)
	a.tradeMonitorSvc = service.NewTradeMonitorService(eventRepo, orderCache, marketCache)
	// 设置 Kafka 告警回调
	if a.kafkaProducer != nil {
		a.tradeMonitorSvc.SetAlertCallback(a.kafkaProducer.RiskAlertCallback())
	}

	// 启动 Kafka 消费者
	if a.cfg.Kafka.Enabled && len(a.cfg.Kafka.Brokers) > 0 {
		consumer, err := kafka.NewConsumer(
			&kafka.ConsumerConfig{
				Brokers: a.cfg.Kafka.Brokers,
				GroupID: a.cfg.Kafka.GroupID,
			},
			orderCache,
			amountCache,
			marketCache,
			withdrawCache,
			a.tradeMonitorSvc.TradeCallback(), // 传入交易监控回调
		)
		if err != nil {
			logger.Error("创建 Kafka consumer 失败", "error", err)
		} else {
			a.kafkaConsumer = consumer
			go func() {
				if err := consumer.Start(a.ctx); err != nil {
					logger.Error("Kafka consumer 错误", "error", err)
				}
			}()
		}
	}

	logger.Info("服务层初始化完成")
}

// warmupCache 预热缓存
func (a *App) warmupCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 加载黑名单到缓存
	if err := a.blacklistSvc.SyncCacheFromDB(ctx); err != nil {
		logger.Error("failed to sync blacklist cache", "error", err)
	}

	logger.Info("cache warmup completed")
}

// startGRPC 启动 gRPC 服务
func (a *App) startGRPC() error {
	addr := fmt.Sprintf(":%d", a.cfg.Service.GRPCPort)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// 构建拦截器链
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		middleware.RecoveryUnaryServerInterceptor(),
		middleware.UnaryServerInterceptor(),
		metrics.UnaryServerInterceptor(a.cfg.Service.Name),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		metrics.StreamServerInterceptor(a.cfg.Service.Name),
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

	// 创建 gRPC 服务器 (带 metrics 和 tracing 拦截器)
	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// 注册风控服务
	riskHandler := handler.NewRiskHandler(
		a.riskSvc,
		a.blacklistSvc,
		a.ruleSvc,
		a.eventSvc,
	)
	riskHandler.SetRateLimitCache(a.rateLimitCache)
	riskHandler.SetWithdrawReviewService(a.withdrawReviewSvc)
	riskHandler.SetWhitelistService(a.whitelistSvc)
	riskHandler.SetAlertService(a.alertSvc)
	riskv1.RegisterRiskServiceServer(a.grpcServer, riskHandler)

	// 注册健康检查
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(a.grpcServer, healthServer)
	healthServer.SetServingStatus(a.cfg.Service.Name, grpc_health_v1.HealthCheckResponse_SERVING)

	logger.Info("starting gRPC server",
		"addr", addr,
		"service", a.cfg.Service.Name)

	go func() {
		if err := a.grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
		}
	}()

	// 启动 HTTP 服务 (metrics + health)
	a.startHTTPServer()

	return nil
}

// startHTTPServer 启动 HTTP 服务器 (metrics + health check)
func (a *App) startHTTPServer() {
	a.httpServer = infra.NewHTTPServer(&infra.HTTPServerConfig{
		Port:           a.cfg.Service.HTTPPort,
		DB:             a.db,
		RedisUniversal: a.redisClient,
		EnableMetrics:  true,
		EnableHealth:   true,
	})
	infra.StartHTTPServer(a.httpServer)
}

// GetConfig 获取配置
func (a *App) GetConfig() *config.Config {
	return a.cfg
}
