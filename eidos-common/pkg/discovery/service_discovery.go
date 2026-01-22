// Package discovery 提供统一的服务发现和 gRPC 客户端管理
//
// 基于 Nacos 服务发现:
//   - 服务注册与发现（Nacos 必须可用）
//   - 客户端负载均衡 (round_robin)
//   - 连接池管理
//   - 健康检查
//   - 优雅关闭
//
// 注意: 本模块要求 Nacos 服务必须可用，不支持静态地址回退
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/resolver"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
)

// ServiceName 服务名称常量
const (
	ServiceTrading  = "eidos-trading"
	ServiceMatching = "eidos-matching"
	ServiceMarket   = "eidos-market"
	ServiceChain    = "eidos-chain"
	ServiceRisk     = "eidos-risk"
	ServiceJobs     = "eidos-jobs"
	ServiceAdmin    = "eidos-admin"
	ServiceAPI      = "eidos-api"
)

// Config 服务发现配置
type Config struct {
	// Nacos 配置 (必须启用)
	NacosEnabled    bool   `yaml:"nacos_enabled" json:"nacos_enabled"`
	NacosServerAddr string `yaml:"nacos_server_addr" json:"nacos_server_addr"`
	NacosNamespace  string `yaml:"nacos_namespace" json:"nacos_namespace"`
	NacosGroup      string `yaml:"nacos_group" json:"nacos_group"`
	NacosUsername   string `yaml:"nacos_username" json:"nacos_username"`
	NacosPassword   string `yaml:"nacos_password" json:"nacos_password"`
	NacosLogDir     string `yaml:"nacos_log_dir" json:"nacos_log_dir"`
	NacosCacheDir   string `yaml:"nacos_cache_dir" json:"nacos_cache_dir"`

	// gRPC 客户端配置
	ConnectTimeout      time.Duration `yaml:"connect_timeout" json:"connect_timeout"`
	KeepAliveTime       time.Duration `yaml:"keep_alive_time" json:"keep_alive_time"`
	KeepAliveTimeout    time.Duration `yaml:"keep_alive_timeout" json:"keep_alive_timeout"`
	MaxRecvMsgSize      int           `yaml:"max_recv_msg_size" json:"max_recv_msg_size"`
	MaxSendMsgSize      int           `yaml:"max_send_msg_size" json:"max_send_msg_size"`
	LoadBalancingPolicy string        `yaml:"load_balancing_policy" json:"load_balancing_policy"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		NacosEnabled:        true, // Nacos 必须启用
		NacosNamespace:      "public",
		NacosGroup:          "DEFAULT_GROUP",
		NacosLogDir:         "/tmp/nacos/log",
		NacosCacheDir:       "/tmp/nacos/cache",
		ConnectTimeout:      10 * time.Second,
		KeepAliveTime:       30 * time.Second,
		KeepAliveTimeout:    10 * time.Second,
		MaxRecvMsgSize:      10 * 1024 * 1024, // 10MB
		MaxSendMsgSize:      10 * 1024 * 1024, // 10MB
		LoadBalancingPolicy: "round_robin",
	}
}

// ServiceDiscovery 服务发现管理器
type ServiceDiscovery struct {
	config      *Config
	nacosHelper *nacos.ServiceHelper
	connections sync.Map // map[string]*grpc.ClientConn
	mu          sync.Mutex
	logger      *slog.Logger
	resolverReg sync.Once
}

// NewServiceDiscovery 创建服务发现管理器
func NewServiceDiscovery(config *Config, logger *slog.Logger) (*ServiceDiscovery, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	sd := &ServiceDiscovery{
		config: config,
		logger: logger,
	}

	// Nacos 必须启用
	if !config.NacosEnabled {
		return nil, fmt.Errorf("nacos must be enabled for service discovery")
	}
	if config.NacosServerAddr == "" {
		return nil, fmt.Errorf("nacos server address is required")
	}

	nacosConfig := &nacos.Config{
		ServerAddr: config.NacosServerAddr,
		Namespace:  config.NacosNamespace,
		Group:      config.NacosGroup,
		Username:   config.NacosUsername,
		Password:   config.NacosPassword,
		LogDir:     config.NacosLogDir,
		CacheDir:   config.NacosCacheDir,
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}

	helper, err := nacos.NewServiceHelper(nacosConfig)
	if err != nil {
		return nil, fmt.Errorf("create nacos helper: %w", err)
	}

	sd.nacosHelper = helper
	logger.Info("nacos service discovery initialized",
		"server", config.NacosServerAddr,
		"namespace", config.NacosNamespace,
		"group", config.NacosGroup,
	)

	return sd, nil
}

// RegisterService 注册当前服务到 Nacos
func (sd *ServiceDiscovery) RegisterService(serviceName string, port uint64, metadata map[string]string) error {
	if sd.nacosHelper == nil {
		sd.logger.Warn("nacos not enabled, skip service registration",
			"service", serviceName,
		)
		return nil
	}

	if err := sd.nacosHelper.RegisterService(serviceName, port, metadata); err != nil {
		return fmt.Errorf("register service %s: %w", serviceName, err)
	}

	sd.logger.Info("service registered to nacos",
		"service", serviceName,
		"port", port,
	)
	return nil
}

// GetConnection 获取到指定服务的 gRPC 连接
func (sd *ServiceDiscovery) GetConnection(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	// 先从缓存获取
	if conn, ok := sd.connections.Load(serviceName); ok {
		return conn.(*grpc.ClientConn), nil
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// 双重检查
	if conn, ok := sd.connections.Load(serviceName); ok {
		return conn.(*grpc.ClientConn), nil
	}

	// 创建新连接
	conn, err := sd.createConnection(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	sd.connections.Store(serviceName, conn)
	return conn, nil
}

// createConnection 创建 gRPC 连接
func (sd *ServiceDiscovery) createConnection(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	target := sd.resolveTarget(serviceName)
	if target == "" {
		return nil, fmt.Errorf("no address found for service: %s", serviceName)
	}

	opts := sd.buildDialOptions()

	ctx, cancel := context.WithTimeout(ctx, sd.config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial service %s at %s: %w", serviceName, target, err)
	}

	sd.logger.Info("grpc connection created",
		"service", serviceName,
		"target", target,
		"mode", sd.getMode(),
	)

	return conn, nil
}

// resolveTarget 解析服务目标地址
func (sd *ServiceDiscovery) resolveTarget(serviceName string) string {
	// 注册 Nacos resolver（只执行一次）
	sd.resolverReg.Do(func() {
		nacos.RegisterResolver(sd.nacosHelper.Discovery(), sd.config.NacosGroup)
		sd.logger.Info("nacos grpc resolver registered")
	})
	return nacos.BuildTarget(serviceName)
}

// buildDialOptions 构建 gRPC 连接选项
func (sd *ServiceDiscovery) buildDialOptions() []grpc.DialOption {
	opts := []grpc.DialOption{
		// 不安全连接（内网环境）
		grpc.WithTransportCredentials(insecure.NewCredentials()),

		// 保活配置
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                sd.config.KeepAliveTime,
			Timeout:             sd.config.KeepAliveTimeout,
			PermitWithoutStream: true,
		}),

		// 消息大小限制
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(sd.config.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(sd.config.MaxSendMsgSize),
		),

		// 负载均衡策略
		grpc.WithDefaultServiceConfig(
			fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, sd.config.LoadBalancingPolicy),
		),
	}

	return opts
}

// getMode 获取当前模式
func (sd *ServiceDiscovery) getMode() string {
	if sd.nacosHelper != nil {
		return "nacos"
	}
	return "static"
}

// WaitForService 等待服务可用
func (sd *ServiceDiscovery) WaitForService(ctx context.Context, serviceName string, timeout time.Duration) error {
	return sd.nacosHelper.WaitForService(ctx, serviceName, timeout)
}

// GetServiceAddresses 获取服务的所有地址（用于调试）
func (sd *ServiceDiscovery) GetServiceAddresses(serviceName string) ([]string, error) {
	return sd.nacosHelper.GetServiceAddresses(serviceName)
}

// Close 关闭服务发现管理器
func (sd *ServiceDiscovery) Close() error {
	var lastErr error

	// 关闭所有 gRPC 连接
	sd.connections.Range(func(key, value interface{}) bool {
		conn := value.(*grpc.ClientConn)
		if err := conn.Close(); err != nil {
			lastErr = err
			sd.logger.Error("failed to close grpc connection",
				"service", key.(string),
				"error", err,
			)
		}
		return true
	})

	// 关闭 Nacos
	if err := sd.nacosHelper.Close(); err != nil {
		lastErr = err
		sd.logger.Error("failed to close nacos helper", "error", err)
	}

	return lastErr
}

// NacosHelper 返回 Nacos 助手（用于高级操作）
func (sd *ServiceDiscovery) NacosHelper() *nacos.ServiceHelper {
	return sd.nacosHelper
}

// IsNacosEnabled 检查 Nacos 是否启用
func (sd *ServiceDiscovery) IsNacosEnabled() bool {
	return sd.nacosHelper != nil
}


// HealthCheck 健康检查
func (sd *ServiceDiscovery) HealthCheck(ctx context.Context) error {
	return sd.nacosHelper.HealthCheck(ctx)
}

// MustRegisterResolver 注册 gRPC resolver（确保只注册一次）
// 这是为了在应用启动早期就注册 resolver
func (sd *ServiceDiscovery) MustRegisterResolver() {
	sd.resolverReg.Do(func() {
		nacos.RegisterResolver(sd.nacosHelper.Discovery(), sd.config.NacosGroup)
		sd.logger.Info("nacos grpc resolver registered")
	})
}

// global resolver registration guard
var globalResolverRegistered bool
var globalResolverMu sync.Mutex

// EnsureGlobalResolver 确保全局 resolver 已注册
// 某些场景下多个 ServiceDiscovery 实例可能共存
func EnsureGlobalResolver(discovery *nacos.Discovery, group string) {
	globalResolverMu.Lock()
	defer globalResolverMu.Unlock()

	if !globalResolverRegistered {
		resolver.Register(nacos.NewGRPCResolverBuilder(discovery, group))
		globalResolverRegistered = true
	}
}
