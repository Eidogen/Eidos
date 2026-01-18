package main

import (
	"context"
	"flag"
	"os"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/app"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("load config: " + err.Error())
	}

	// 初始化日志
	if err := logger.Init(&logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: cfg.Service.Name,
	}); err != nil {
		panic("init logger: " + err.Error())
	}
	defer logger.Sync()

	log := logger.L()
	log.Info("starting service",
		zap.String("service", cfg.Service.Name),
		zap.String("env", cfg.Service.Env),
		zap.Int("port", cfg.Service.HTTPPort),
	)

	// 创建应用
	application := app.New(cfg, log)

	// 启动应用
	ctx := context.Background()
	if err := application.Start(ctx); err != nil {
		log.Fatal("failed to start application", zap.Error(err))
	}

	log.Info("service started successfully",
		zap.Int("port", cfg.Service.HTTPPort),
	)

	// 等待关闭信号
	application.WaitForShutdown()

	os.Exit(0)
}
