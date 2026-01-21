package main

import (
	"flag"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/app"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

const serviceName = "eidos-chain"

func main() {
	// 命令行参数
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

	// 创建应用
	application, err := app.NewApp(cfg)
	if err != nil {
		logger.Fatal("failed to create app", "error", err)
	}

	// 运行应用
	if err := application.Run(); err != nil {
		logger.Fatal("app run error", "error", err)
	}

	logger.Info("service stopped")
}
