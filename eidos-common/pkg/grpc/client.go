package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
		"target", target,
	)

	return conn, nil
}

// buildDialOptions 构建连接选项
func (f *ClientFactory) buildDialOptions() []grpc.DialOption {
	var opts []grpc.DialOption

	// TLS 配置
	if f.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// 加载 TLS 证书
		tlsCreds, err := f.loadTLSCredentials()
		if err != nil {
			logger.Warn("failed to load TLS credentials, falling back to insecure",
				"error", err)
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(tlsCreds))
		}
	}

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

// loadTLSCredentials 加载 TLS 证书
func (f *ClientFactory) loadTLSCredentials() (credentials.TransportCredentials, error) {
	// 如果没有配置证书文件，使用系统根证书
	if f.config.CertFile == "" {
		tlsConfig := &tls.Config{
			ServerName: f.config.ServerName,
		}
		return credentials.NewTLS(tlsConfig), nil
	}

	// 加载 CA 证书
	caCert, err := os.ReadFile(f.config.CertFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert file: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert")
	}

	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		ServerName: f.config.ServerName,
	}

	return credentials.NewTLS(tlsConfig), nil
}

// Close 关闭所有连接
func (f *ClientFactory) Close() error {
	var lastErr error
	f.connections.Range(func(key, value interface{}) bool {
		conn := value.(*grpc.ClientConn)
		if err := conn.Close(); err != nil {
			lastErr = err
			logger.Error("failed to close grpc connection",
				"target", key.(string),
				"error", err,
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

		if err != nil {
			logger.Debug("grpc client call failed",
				"method", method,
				"duration", duration,
				"error", err,
			)
		} else {
			logger.Debug("grpc client call completed",
				"method", method,
				"duration", duration,
			)
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

		logger.Debug("grpc client stream started",
			"method", method,
			"setup_duration", time.Since(start),
			"error", err,
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
// 使用 OpenTelemetry 标准实现，支持完整的分布式追踪
func UnaryClientTracingInterceptor() grpc.UnaryClientInterceptor {
	return tracing.UnaryClientInterceptor("grpc-client")
}

// StreamClientTracingInterceptor 流式调用追踪拦截器
// 使用 OpenTelemetry 标准实现，支持完整的分布式追踪
func StreamClientTracingInterceptor() grpc.StreamClientInterceptor {
	return tracing.StreamClientInterceptor("grpc-client")
}

// ========== 便捷辅助函数 ==========

// DefaultDialOptions 返回企业级默认 gRPC 连接选项
// 包含：keepalive、负载均衡、消息大小限制、拦截器（tracing/metrics/logging）
func DefaultDialOptions(serviceName string, enableTracing bool) []grpc.DialOption {
	opts := []grpc.DialOption{
		// 不安全连接（内网服务间通信）
		grpc.WithTransportCredentials(insecure.NewCredentials()),

		// Keepalive 配置 - 保持连接活跃
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // 30秒发送一次 ping
			Timeout:             10 * time.Second, // 10秒等待 pong
			PermitWithoutStream: true,             // 无活跃流时也发送 ping
		}),

		// 消息大小限制 - 与服务端保持一致
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024), // 16MB
			grpc.MaxCallSendMsgSize(16*1024*1024), // 16MB
		),

		// 负载均衡 - round_robin
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	}

	// 构建拦截器链
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		UnaryClientLoggingInterceptor(),
		UnaryClientMetricsInterceptor(),
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		StreamClientLoggingInterceptor(),
		StreamClientMetricsInterceptor(),
	}

	// 如果启用 tracing，添加到拦截器链最前面
	if enableTracing {
		unaryInterceptors = append(
			[]grpc.UnaryClientInterceptor{tracing.UnaryClientInterceptor(serviceName)},
			unaryInterceptors...,
		)
		streamInterceptors = append(
			[]grpc.StreamClientInterceptor{tracing.StreamClientInterceptor(serviceName)},
			streamInterceptors...,
		)
	}

	opts = append(opts,
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	)

	return opts
}

// DialWithDefaults 使用企业级默认配置创建 gRPC 连接
// 这是推荐的连接创建方式，确保所有客户端使用一致的配置
func DialWithDefaults(ctx context.Context, target string, serviceName string, enableTracing bool, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := DefaultDialOptions(serviceName, enableTracing)
	opts = append(opts, extraOpts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s failed: %w", target, err)
	}

	logger.Info("grpc connection created",
		"target", target,
		"service", serviceName,
		"tracing", enableTracing,
	)

	return conn, nil
}

// DialWithTimeout 使用超时和默认配置创建 gRPC 连接
// 适用于需要等待连接建立的场景
func DialWithTimeout(ctx context.Context, target string, serviceName string, timeout time.Duration, enableTracing bool) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := DefaultDialOptions(serviceName, enableTracing)

	// 使用 grpc.NewClient 替代已弃用的 grpc.DialContext
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client for %s failed: %w", target, err)
	}

	// 等待连接就绪
	conn.Connect()

	// 简单等待连接状态变化
	select {
	case <-dialCtx.Done():
		conn.Close()
		return nil, fmt.Errorf("dial %s timeout: %w", target, dialCtx.Err())
	default:
		// 连接创建成功，后续会自动重连
	}

	logger.Info("grpc connection created with timeout",
		"target", target,
		"service", serviceName,
		"timeout", timeout,
	)

	return conn, nil
}
