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
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/config"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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

	// 服务层
	riskSvc      *service.RiskService
	blacklistSvc *service.BlacklistService
	ruleSvc      *service.RuleService
	eventSvc     *service.EventService

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

	// 停止 HTTP 服务器
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			logger.Error("http server shutdown error", "error", err)
		}
	}

	// 停止接收新请求
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 停止 Kafka 消费者
	if a.kafkaConsumer != nil {
		a.kafkaConsumer.Stop()
	}

	// 关闭 Kafka 生产者
	if a.kafkaProducer != nil {
		a.kafkaProducer.Close()
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

	// 关闭服务发现基础设施
	if a.infra != nil {
		a.infra.Close()
	}

	a.cancel()
	logger.Info("risk service stopped")
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
	sqlDB.SetConnMaxLifetime(time.Duration(a.cfg.Postgres.ConnMaxLifetime) * time.Second)

	a.db = db
	logger.Info("database connected",
		"host", a.cfg.Postgres.Host,
		"database", a.cfg.Postgres.Database)

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
		Addr:     a.cfg.Redis.Addresses[0],
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
		"addr", a.cfg.Redis.Addresses[0],
		"db", a.cfg.Redis.DB)

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
	withdrawRepo := repository.NewWithdrawalReviewRepository(a.db)

	// 创建缓存层
	blacklistCache := cache.NewBlacklistCache(a.redisClient)
	rateLimitCache := cache.NewRateLimitCache(a.redisClient)
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
		withdrawRepo,
		blacklistCache,
		rateLimitCache,
		amountCache,
		orderCache,
		marketCache,
		withdrawCache,
		a.cfg,
	)

	a.blacklistSvc = service.NewBlacklistService(blacklistRepo, blacklistCache)
	a.ruleSvc = service.NewRuleService(ruleRepo)
	a.eventSvc = service.NewEventService(eventRepo)

	// 设置 Kafka 回调
	if a.kafkaProducer != nil {
		alertCallback := a.kafkaProducer.RiskAlertCallback()
		a.riskSvc.SetOnRiskAlert(alertCallback)
		a.blacklistSvc.SetOnRiskAlert(alertCallback)
		a.ruleSvc.SetOnRiskAlert(alertCallback)
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
		)
		if err != nil {
			logger.Error("failed to create kafka consumer", "error", err)
		} else {
			a.kafkaConsumer = consumer
			go func() {
				if err := consumer.Start(a.ctx); err != nil {
					logger.Error("kafka consumer error", "error", err)
				}
			}()
		}
	}

	logger.Info("services initialized")
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

	// 创建 gRPC 服务器 (带 metrics 拦截器)
	a.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor(),
			middleware.UnaryServerInterceptor(),
			metrics.UnaryServerInterceptor(a.cfg.Service.Name),
		),
		grpc.ChainStreamInterceptor(
			metrics.StreamServerInterceptor(a.cfg.Service.Name),
		),
	)

	// 注册风控服务
	riskHandler := handler.NewRiskHandler(
		a.riskSvc,
		a.blacklistSvc,
		a.ruleSvc,
		a.eventSvc,
	)
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
	mux := http.NewServeMux()

	// Prometheus metrics 端点
	mux.Handle("/metrics", promhttp.Handler())

	// 健康检查端点 (HTTP)
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		// 检查数据库连接
		if a.db != nil {
			sqlDB, err := a.db.DB()
			if err != nil || sqlDB.Ping() != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("DB NOT READY"))
				return
			}
		}
		// 检查 Redis 连接
		if a.redisClient != nil {
			if err := a.redisClient.Ping(r.Context()).Err(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("REDIS NOT READY"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	a.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.cfg.Service.HTTPPort),
		Handler: mux,
	}

	go func() {
		logger.Info("HTTP server listening (metrics + health)",
			"port", a.cfg.Service.HTTPPort,
		)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
		}
	}()
}

// GetConfig 获取配置
func (a *App) GetConfig() *config.Config {
	return a.cfg
}
