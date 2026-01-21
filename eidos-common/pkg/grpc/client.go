package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// ClientConfig gRPC 客户端配置
type ClientConfig struct {
	// 连接配置
	Target            string        `yaml:"target" json:"target"`                           // 目标地址
	Timeout           time.Duration `yaml:"timeout" json:"timeout"`                         // 连接超时
	KeepAliveTime     time.Duration `yaml:"keep_alive_time" json:"keep_alive_time"`         // 保活间隔
	KeepAliveTimeout  time.Duration `yaml:"keep_alive_timeout" json:"keep_alive_timeout"`   // 保活超时
	PermitWithoutStream bool          `yaml:"permit_without_stream" json:"permit_without_stream"` // 允许无流保活

	// TLS 配置
	Insecure   bool   `yaml:"insecure" json:"insecure"`       // 是否使用不安全连接
	CertFile   string `yaml:"cert_file" json:"cert_file"`     // 证书文件
	ServerName string `yaml:"server_name" json:"server_name"` // 服务器名称

	// 负载均衡
	LoadBalancingPolicy string `yaml:"load_balancing_policy" json:"load_balancing_policy"` // 负载均衡策略

	// 重试配置
	MaxRetryAttempts int `yaml:"max_retry_attempts" json:"max_retry_attempts"` // 最大重试次数

	// 拦截器配置
	EnableMetrics bool `yaml:"enable_metrics" json:"enable_metrics"` // 启用指标收集
	EnableLogging bool `yaml:"enable_logging" json:"enable_logging"` // 启用日志
	EnableTracing bool `yaml:"enable_tracing" json:"enable_tracing"` // 启用链路追踪
}

// DefaultClientConfig 返回默认客户端配置
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:             10 * time.Second,
		KeepAliveTime:       30 * time.Second,
		KeepAliveTimeout:    10 * time.Second,
		PermitWithoutStream: true,
		Insecure:            true,
		LoadBalancingPolicy: "round_robin",
		MaxRetryAttempts:    3,
		EnableMetrics:       true,
		EnableLogging:       true,
		EnableTracing:       true,
	}
}

// ClientFactory gRPC 客户端工厂
type ClientFactory struct {
	config      *ClientConfig
	connections sync.Map // map[string]*grpc.ClientConn
	mu          sync.Mutex
}

// NewClientFactory 创建客户端工厂
func NewClientFactory(config *ClientConfig) *ClientFactory {
	if config == nil {
		config = DefaultClientConfig()
	}
	return &ClientFactory{
		config: config,
	}
}

// GetConnection 获取或创建连接
func (f *ClientFactory) GetConnection(target string) (*grpc.ClientConn, error) {
	// 先从缓存获取
	if conn, ok := f.connections.Load(target); ok {
		return conn.(*grpc.ClientConn), nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// 双重检查
	if conn, ok := f.connections.Load(target); ok {
		return conn.(*grpc.ClientConn), nil
	}

	// 创建新连接
	conn, err := f.createConnection(target)
	if err != nil {
		return nil, err
	}

	f.connections.Store(target, conn)
	return conn, nil
}

// createConnection 创建 gRPC 连接
func (f *ClientFactory) createConnection(target string) (*grpc.ClientConn, error) {
	opts := f.buildDialOptions()

	ctx, cancel := context.WithTimeout(context.Background(), f.config.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s failed: %w", target, err)
	}

	logger.Info("grpc connection created",
		zap.String("target", target),
	)

	return conn, nil
}

// buildDialOptions 构建连接选项
func (f *ClientFactory) buildDialOptions() []grpc.DialOption {
	var opts []grpc.DialOption

	// TLS 配置
	if f.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	// TODO: 添加 TLS 支持

	// 保活配置
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                f.config.KeepAliveTime,
		Timeout:             f.config.KeepAliveTimeout,
		PermitWithoutStream: f.config.PermitWithoutStream,
	}))

	// 负载均衡
	if f.config.LoadBalancingPolicy != "" {
		opts = append(opts, grpc.WithDefaultServiceConfig(
			fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, f.config.LoadBalancingPolicy),
		))
	}

	// 拦截器
	var unaryInterceptors []grpc.UnaryClientInterceptor
	var streamInterceptors []grpc.StreamClientInterceptor

	if f.config.EnableLogging {
		unaryInterceptors = append(unaryInterceptors, UnaryClientLoggingInterceptor())
		streamInterceptors = append(streamInterceptors, StreamClientLoggingInterceptor())
	}

	if f.config.EnableMetrics {
		unaryInterceptors = append(unaryInterceptors, UnaryClientMetricsInterceptor())
		streamInterceptors = append(streamInterceptors, StreamClientMetricsInterceptor())
	}

	if f.config.EnableTracing {
		unaryInterceptors = append(unaryInterceptors, UnaryClientTracingInterceptor())
		streamInterceptors = append(streamInterceptors, StreamClientTracingInterceptor())
	}

	if len(unaryInterceptors) > 0 {
		opts = append(opts, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.WithChainStreamInterceptor(streamInterceptors...))
	}

	return opts
}

// Close 关闭所有连接
func (f *ClientFactory) Close() error {
	var lastErr error
	f.connections.Range(func(key, value interface{}) bool {
		conn := value.(*grpc.ClientConn)
		if err := conn.Close(); err != nil {
			lastErr = err
			logger.Error("failed to close grpc connection",
				zap.String("target", key.(string)),
				zap.Error(err),
			)
		}
		return true
	})
	return lastErr
}

// CloseConnection 关闭指定连接
func (f *ClientFactory) CloseConnection(target string) error {
	if conn, ok := f.connections.LoadAndDelete(target); ok {
		return conn.(*grpc.ClientConn).Close()
	}
	return nil
}

// UnaryClientLoggingInterceptor 一元调用日志拦截器
func UnaryClientLoggingInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("method", method),
			zap.Duration("duration", duration),
		}

		if err != nil {
			fields = append(fields, zap.Error(err))
			logger.WithContext(ctx).Debug("grpc client call failed", fields...)
		} else {
			logger.WithContext(ctx).Debug("grpc client call completed", fields...)
		}

		return err
	}
}

// StreamClientLoggingInterceptor 流式调用日志拦截器
func StreamClientLoggingInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, opts...)

		logger.WithContext(ctx).Debug("grpc client stream started",
			zap.String("method", method),
			zap.Duration("setup_duration", time.Since(start)),
			zap.Error(err),
		)

		return stream, err
	}
}

// UnaryClientMetricsInterceptor 一元调用指标拦截器
func UnaryClientMetricsInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		code := "OK"
		if err != nil {
			code = "ERROR"
		}

		metrics.RecordRequest("grpc_client", method, code, duration.Seconds())

		return err
	}
}

// StreamClientMetricsInterceptor 流式调用指标拦截器
func StreamClientMetricsInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, opts...)

		code := "OK"
		if err != nil {
			code = "ERROR"
		}

		metrics.RecordRequest("grpc_client_stream", method, code, time.Since(start).Seconds())

		return stream, err
	}
}

// UnaryClientTracingInterceptor 一元调用追踪拦截器
func UnaryClientTracingInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 传递 trace_id
		ctx = propagateTraceID(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamClientTracingInterceptor 流式调用追踪拦截器
func StreamClientTracingInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = propagateTraceID(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}
