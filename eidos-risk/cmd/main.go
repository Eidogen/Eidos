package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/app"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/config"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "config file path")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// 初始化日志
	if err := logger.Init(&logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: cfg.Service.Name,
		Environment: cfg.Service.Env,
	}); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("starting service",
		"service", cfg.Service.Name,
		"env", cfg.Service.Env,
		"grpc_port", cfg.Service.GRPCPort)

	// 创建应用实例
	application := app.New(cfg)

	// 启动应用
	if err := application.Run(); err != nil {
		logger.Fatal("failed to start application", "error", err)
	}

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down service...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := application.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown gracefully", "error", err)
	}

	logger.Info("service stopped")
}
