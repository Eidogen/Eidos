package main

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/app"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/config"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	if err := logger.Init(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "eidos-trading",
	}); err != nil {
		panic(err)
	}
	defer logger.Sync()

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// 启动应用
	if err := app.New(cfg).Run(); err != nil {
		logger.Fatal("application error", zap.Error(err))
	}
}
