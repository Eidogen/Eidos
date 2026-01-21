package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/app"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

const serviceName = "eidos-admin"

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	// 初始化日志（使用配置中的日志级别和格式）
	if err := logger.Init(&logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: serviceName,
		Environment: cfg.Service.Env,
	}); err != nil {
		panic(fmt.Sprintf("failed to init logger: %v", err))
	}
	defer logger.Sync()

	logger.Info("starting service",
		"service", serviceName,
		"port", cfg.Server.Port)

	// 确保数据库存在
	if err := app.EnsureDatabase(cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Database); err != nil {
		logger.Fatal("failed to ensure database", "error", err)
	}

	// 初始化数据库
	db, err := initDatabase(cfg)
	if err != nil {
		logger.Fatal("failed to init database", "error", err)
	}

	// 自动迁移
	if err := app.AutoMigrate(db); err != nil {
		logger.Fatal("failed to auto migrate", "error", err)
	}
	logger.Info("database migrated")

	// 初始化 Redis
	redisClient, err := initRedis(cfg)
	if err != nil {
		logger.Fatal("failed to init redis", "error", err)
	}
	defer redisClient.Close()

	// 创建并初始化应用
	application := app.New(cfg, db, redisClient)
	if err := application.Init(); err != nil {
		logger.Fatal("failed to init application", "error", err)
	}

	// 启动服务
	go func() {
		if err := application.Run(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to run application", "error", err)
		}
	}()

	// 等待终止信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := application.Shutdown(ctx); err != nil {
		logger.Error("application shutdown error", "error", err)
	}

	logger.Info("service stopped")
}

// initDatabase 初始化数据库连接
func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Database,
	)

	gormConfig := &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Warn),
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.Postgres.MaxConnections)
	sqlDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetime) * time.Second)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connected",
		"host", cfg.Postgres.Host,
		"port", cfg.Postgres.Port,
		"database", cfg.Postgres.Database)

	return db, nil
}

// initRedis 初始化 Redis 连接
func initRedis(cfg *config.Config) (redis.UniversalClient, error) {
	client := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:    cfg.Redis.Addresses,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	logger.Info("redis connected", "addresses", cfg.Redis.Addresses)

	return client, nil
}
