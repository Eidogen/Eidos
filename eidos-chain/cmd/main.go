package main

import (
	"flag"
	"os"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/app"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
)

const serviceName = "eidos-chain"

func main() {
	// 命令行参数
	configPath := flag.String("config", "config/config.yaml", "config file path")
	flag.Parse()

	// 初始化日志
	if err := logger.Init(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: serviceName,
	}); err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Info("starting service", zap.String("service", serviceName))

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// 根据配置更新日志级别
	if cfg.Log.Level != "" {
		logger.SetLevel(cfg.Log.Level)
	}

	// 创建应用
	application, err := app.NewApp(cfg)
	if err != nil {
		logger.Fatal("failed to create app", zap.Error(err))
	}

	// 运行应用
	if err := application.Run(); err != nil {
		logger.Fatal("app run error", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("service stopped")
}
