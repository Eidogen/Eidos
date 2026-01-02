package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/middleware"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	serviceName = "eidos-market"
	grpcPort    = 50053
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

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.RecoveryUnaryServerInterceptor(),
			middleware.UnaryServerInterceptor(),
		),
	)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// TODO: 注册行情服务
	// marketv1.RegisterMarketServiceServer(server, handler.NewMarketHandler(...))

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down...")
		healthServer.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		server.GracefulStop()
		cancel()
	}()

	logger.Info("gRPC server listening", zap.Int("port", grpcPort))
	if err := server.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}

	<-ctx.Done()
	logger.Info("service stopped")
}
