package main

import (
	"flag"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-market/internal/app"
	"github.com/eidos-exchange/eidos/eidos-market/internal/config"
	"go.uber.org/zap"
)

const serviceName = "eidos-market"

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "配置文件路径")
	enhanced := flag.Bool("enhanced", true, "启用增强模式 (高吞吐量设计)")
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
	}); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	defer logger.Sync()

	log := logger.L()
	log.Info("starting service",
		zap.String("service", serviceName),
		zap.String("config", *configPath),
		zap.String("env", cfg.Service.Env),
		zap.Bool("enhanced", *enhanced),
	)

	// 创建并运行应用
	if *enhanced {
		// 增强模式：高吞吐量设计
		// - 批量 Kafka 消费
		// - 多级缓存策略
		// - 写后缓存
		// - 配置热更新
		application := app.NewEnhancedApp(cfg, log)
		if err := application.Run(); err != nil {
			log.Fatal("application error", zap.Error(err))
		}
	} else {
		// 基础模式：简单实现
		application := app.New(cfg, log)
		if err := application.Run(); err != nil {
			log.Fatal("application error", zap.Error(err))
		}
	}
}
