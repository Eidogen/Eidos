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

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/app"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	configPath = flag.String("config", "config/config.yaml", "配置文件路径")
)

func main() {
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
		"node_id", cfg.Service.NodeID,
		"env", cfg.Service.Env,
	)

	// 创建应用
	application, err := app.NewApp(cfg)
	if err != nil {
		logger.Fatal("create app", "error", err)
	}

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
		logger.Info("HTTP server listening", "port", cfg.Service.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve", "error", err)
		}
	}()

	// 启动应用
	if err := application.Start(); err != nil {
		logger.Fatal("start app", "error", err)
	}

	logger.Info("service ready",
		"service", cfg.Service.Name,
		"grpc_port", cfg.Service.GRPCPort,
		"http_port", cfg.Service.HTTPPort,
	)

	// 等待停止信号
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received signal", "signal", sig.String())
	case <-ctx.Done():
	}

	// 优雅关闭
	logger.Info("shutting down...")

	if err := application.Stop(); err != nil {
		logger.Error("stop app", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown http", "error", err)
	}

	logger.Info("service stopped")
}
