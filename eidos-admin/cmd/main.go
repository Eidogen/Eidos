package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
)

const (
	serviceName = "eidos-admin"
	httpPort    = 8088
)

func main() {
	if err := logger.Init(&logger.Config{
		Level:       "info",
		Format:      "json",
		ServiceName: serviceName,
	}); err != nil {
		panic(err)
	}
	defer logger.Sync()

	logger.Info("starting service", zap.String("service", serviceName))

	// TODO: 初始化 Gin 路由
	// router := gin.Default()
	// handler.RegisterRoutes(router)

	// 临时使用简单 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: mux,
	}

	go func() {
		logger.Info("HTTP server listening", zap.Int("port", httpPort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to serve", zap.Error(err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}

	logger.Info("service stopped")
}
