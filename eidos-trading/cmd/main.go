package main

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/app"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
)

const serviceName = "eidos-trading"

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
		ServiceName: serviceName,
		Environment: cfg.Service.Env,
	}); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("starting service",
		"service", serviceName,
		"env", cfg.Service.Env,
		"grpc_port", cfg.Service.GRPCPort,
	)

	// 启动应用
	if err := app.New(cfg).Run(); err != nil {
		logger.Fatal("application error", "error", err)
	}
}
