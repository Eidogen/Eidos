package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/app"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/config"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
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
		"grpc_port", cfg.Service.GRPCPort)

	// 创建并启动应用
	application := app.New(cfg)
	if err := application.Run(); err != nil {
		logger.Fatal("failed to start application", "error", err)
	}

	// 等待关闭信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("received shutdown signal", "signal", sig.String())

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := application.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	logger.Info("service stopped")
}
