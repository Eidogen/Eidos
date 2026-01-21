package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// ServerConfig gRPC 服务器配置
type ServerConfig struct {
	// 监听配置
	Host string `yaml:"host" json:"host"` // 监听地址
	Port int    `yaml:"port" json:"port"` // 监听端口

	// 保活配置
	MaxConnectionIdle     time.Duration `yaml:"max_connection_idle" json:"max_connection_idle"`         // 最大空闲时间
	MaxConnectionAge      time.Duration `yaml:"max_connection_age" json:"max_connection_age"`           // 最大连接存活时间
	MaxConnectionAgeGrace time.Duration `yaml:"max_connection_age_grace" json:"max_connection_age_grace"` // 优雅关闭时间
	KeepAliveTime         time.Duration `yaml:"keep_alive_time" json:"keep_alive_time"`                 // 保活间隔
	KeepAliveTimeout      time.Duration `yaml:"keep_alive_timeout" json:"keep_alive_timeout"`           // 保活超时

	// 消息大小限制
	MaxRecvMsgSize int `yaml:"max_recv_msg_size" json:"max_recv_msg_size"` // 最大接收消息大小
	MaxSendMsgSize int `yaml:"max_send_msg_size" json:"max_send_msg_size"` // 最大发送消息大小

	// 并发限制
	MaxConcurrentStreams uint32 `yaml:"max_concurrent_streams" json:"max_concurrent_streams"` // 最大并发流

	// 功能开关
	EnableReflection bool `yaml:"enable_reflection" json:"enable_reflection"` // 启用反射
	EnableHealthCheck bool `yaml:"enable_health_check" json:"enable_health_check"` // 启用健康检查
	EnableMetrics    bool `yaml:"enable_metrics" json:"enable_metrics"`       // 启用指标收集
	EnableLogging    bool `yaml:"enable_logging" json:"enable_logging"`       // 启用日志
	EnableRecovery   bool `yaml:"enable_recovery" json:"enable_recovery"`     // 启用 panic 恢复
}

// DefaultServerConfig 返回默认服务器配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:                  "0.0.0.0",
		Port:                  9000,
		MaxConnectionIdle:     15 * time.Minute,
		MaxConnectionAge:      30 * time.Minute,
		MaxConnectionAgeGrace: 5 * time.Second,
		KeepAliveTime:         5 * time.Minute,
		KeepAliveTimeout:      20 * time.Second,
		MaxRecvMsgSize:        16 * 1024 * 1024, // 16MB
		MaxSendMsgSize:        16 * 1024 * 1024, // 16MB
		MaxConcurrentStreams:  1000,
		EnableReflection:      true,
		EnableHealthCheck:     true,
		EnableMetrics:         true,
		EnableLogging:         true,
		EnableRecovery:        true,
	}
}

// Server gRPC 服务器封装
type Server struct {
	config       *ServerConfig
	server       *grpc.Server
	healthServer *health.Server
	listener     net.Listener
}

// NewServer 创建 gRPC 服务器
func NewServer(config *ServerConfig) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	opts := buildServerOptions(config)
	server := grpc.NewServer(opts...)

	s := &Server{
		config: config,
		server: server,
	}

	// 启用健康检查
	if config.EnableHealthCheck {
		s.healthServer = health.NewServer()
		grpc_health_v1.RegisterHealthServer(server, s.healthServer)
	}

	// 启用反射
	if config.EnableReflection {
		reflection.Register(server)
	}

	return s, nil
}

// buildServerOptions 构建服务器选项
func buildServerOptions(config *ServerConfig) []grpc.ServerOption {
	var opts []grpc.ServerOption

	// 保活配置
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle:     config.MaxConnectionIdle,
		MaxConnectionAge:      config.MaxConnectionAge,
		MaxConnectionAgeGrace: config.MaxConnectionAgeGrace,
		Time:                  config.KeepAliveTime,
		Timeout:               config.KeepAliveTimeout,
	}))

	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		MinTime:             5 * time.Second,
		PermitWithoutStream: true,
	}))

	// 消息大小限制
	opts = append(opts, grpc.MaxRecvMsgSize(config.MaxRecvMsgSize))
	opts = append(opts, grpc.MaxSendMsgSize(config.MaxSendMsgSize))

	// 并发限制
	opts = append(opts, grpc.MaxConcurrentStreams(config.MaxConcurrentStreams))

	// 拦截器
	var unaryInterceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor

	if config.EnableRecovery {
		unaryInterceptors = append(unaryInterceptors, UnaryServerRecoveryInterceptor())
		streamInterceptors = append(streamInterceptors, StreamServerRecoveryInterceptor())
	}

	if config.EnableLogging {
		unaryInterceptors = append(unaryInterceptors, UnaryServerLoggingInterceptor())
		streamInterceptors = append(streamInterceptors, StreamServerLoggingInterceptor())
	}

	if config.EnableMetrics {
		unaryInterceptors = append(unaryInterceptors, UnaryServerMetricsInterceptor())
		streamInterceptors = append(streamInterceptors, StreamServerMetricsInterceptor())
	}

	if len(unaryInterceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(unaryInterceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	}

	return opts
}

// Server 返回底层 gRPC 服务器
func (s *Server) Server() *grpc.Server {
	return s.server
}

// HealthServer 返回健康检查服务器
func (s *Server) HealthServer() *health.Server {
	return s.healthServer
}

// SetServingStatus 设置服务状态
func (s *Server) SetServingStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	if s.healthServer != nil {
		s.healthServer.SetServingStatus(service, status)
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	logger.Info("grpc server starting",
		zap.String("address", addr),
	)

	// 设置所有服务为健康状态
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}

	return s.server.Serve(listener)
}

// StartAsync 异步启动服务器
func (s *Server) StartAsync() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	logger.Info("grpc server starting async",
		zap.String("address", addr),
	)

	// 设置所有服务为健康状态
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}

	go func() {
		if err := s.server.Serve(listener); err != nil {
			logger.Error("grpc server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop 优雅停止服务器
func (s *Server) Stop() {
	logger.Info("grpc server stopping")

	// 设置服务为不可用状态
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	s.server.GracefulStop()
	logger.Info("grpc server stopped")
}

// ForceStop 强制停止服务器
func (s *Server) ForceStop() {
	logger.Info("grpc server force stopping")
	s.server.Stop()
}

// Address 返回服务器监听地址
func (s *Server) Address() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// UnaryServerRecoveryInterceptor panic 恢复拦截器
func UnaryServerRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("grpc server panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
				)
				err = fmt.Errorf("internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamServerRecoveryInterceptor 流式 panic 恢复拦截器
func StreamServerRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("grpc stream panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
				)
				err = fmt.Errorf("internal error")
			}
		}()
		return handler(srv, ss)
	}
}

// UnaryServerLoggingInterceptor 一元调用日志拦截器
func UnaryServerLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.WithContext(ctx).Info("grpc request failed", fields...)
		} else {
			logger.WithContext(ctx).Info("grpc request completed", fields...)
		}

		return resp, err
	}
}

// StreamServerLoggingInterceptor 流式调用日志拦截器
func StreamServerLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)

		logger.Info("grpc stream completed",
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)

		return err
	}
}

// UnaryServerMetricsInterceptor 一元调用指标拦截器
func UnaryServerMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := "OK"
		if err != nil {
			code = "ERROR"
		}

		metrics.RecordRequest("grpc_server", info.FullMethod, code, duration.Seconds())

		return resp, err
	}
}

// StreamServerMetricsInterceptor 流式调用指标拦截器
func StreamServerMetricsInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)

		code := "OK"
		if err != nil {
			code = "ERROR"
		}

		metrics.RecordRequest("grpc_server_stream", info.FullMethod, code, time.Since(start).Seconds())

		return err
	}
}
