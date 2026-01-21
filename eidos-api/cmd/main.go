package main

import (
	"context"
	"flag"
	"os"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"

	"github.com/eidos-exchange/eidos/eidos-api/internal/app"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "配置文件路径")
	flag.Parse()

	os.Stderr.WriteString("DEBUG: main starting, loading config from " + *configPath + "\n")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		panic("load config: " + err.Error())
	}

	// 初始化日志（使用配置中的日志级别和格式）
	if err := logger.Init(&logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		ServiceName: cfg.Service.Name,
		Environment: cfg.Service.Env,
	}); err != nil {
		panic("init logger: " + err.Error())
	}
	defer logger.Sync()

	log := logger.L()
	log.Info("starting service",
		"service", cfg.Service.Name,
		"env", cfg.Service.Env,
		"port", cfg.Service.HTTPPort,
	)

	// 创建应用
	application := app.New(cfg, log)

	// 启动应用
	ctx := context.Background()
	if err := application.Start(ctx); err != nil {
		logger.Fatal("failed to start application", "error", err)
	}

	logger.Info("service started successfully",
		"port", cfg.Service.HTTPPort,
	)

	// 等待关闭信号
	application.WaitForShutdown()

	os.Exit(0)
}
