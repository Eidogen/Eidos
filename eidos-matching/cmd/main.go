// Package main 撮合引擎入口
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

	"github.com/eidos-exchange/eidos/eidos-matching/internal/app"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	configPath = flag.String("config", "config/config.yaml", "配置文件路径")
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger := initLogger(cfg.Log.Level, cfg.Log.Format)
	defer logger.Sync()

	logger.Info("starting service",
		zap.String("service", cfg.Service.Name),
		zap.String("node_id", cfg.Service.NodeID),
		zap.String("env", cfg.Service.Env),
	)

	// 创建应用
	application, err := app.NewApp(cfg)
	if err != nil {
		logger.Fatal("create app", zap.Error(err))
	}

	// 注意: gRPC 服务器在 app.Start() 中启动，包含:
	// - MatchingService 注册 (GetOrderbook, GetDepth, HealthCheck)
	// - gRPC 监听在 cfg.Service.GRPCPort

	// 启动 HTTP 服务器 (健康检查 + Metrics)
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	httpMux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		stats := application.GetStats()
		if running, ok := stats["running"].(bool); ok && running {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("NOT READY"))
		}
	})
	httpMux.Handle("/metrics", promhttp.Handler())
	httpMux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := application.GetStats()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "%+v", stats)
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Service.HTTPPort),
		Handler: httpMux,
	}

	go func() {
		logger.Info("HTTP server listening", zap.Int("port", cfg.Service.HTTPPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve", zap.Error(err))
		}
	}()

	// 启动应用 (包含 gRPC 服务器启动和健康状态设置)
	if err := application.Start(); err != nil {
		logger.Fatal("start app", zap.Error(err))
	}

	logger.Info("service ready",
		zap.String("service", cfg.Service.Name),
		zap.Int("grpc_port", cfg.Service.GRPCPort),
		zap.Int("http_port", cfg.Service.HTTPPort),
	)

	// 等待停止信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received signal", zap.String("signal", sig.String()))
	case <-ctx.Done():
	}

	// 优雅关闭
	logger.Info("shutting down...")

	// 停止应用 (包含 gRPC 服务器关闭)
	if err := application.Stop(); err != nil {
		logger.Error("stop app", zap.Error(err))
	}

	// 停止 HTTP 服务器
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown http", zap.Error(err))
	}

	logger.Info("service stopped")
}

// initLogger 初始化日志
func initLogger(level, format string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	var config zap.Config
	if format == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}
	config.Level = zap.NewAtomicLevelAt(zapLevel)

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(logger)
	return logger
}
