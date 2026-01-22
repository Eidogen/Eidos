package discovery

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
)

// InitOptions 初始化选项
type InitOptions struct {
	// 服务名称 (用于注册和日志)
	ServiceName string

	// Nacos 配置 (必须启用)
	NacosEnabled    bool
	NacosServerAddr string
	NacosNamespace  string
	NacosGroup      string
	NacosUsername   string
	NacosPassword   string
	NacosLogDir     string // Nacos 日志目录
	NacosCacheDir   string // Nacos 缓存目录

	// 当前服务端口 (用于注册)
	GRPCPort int
	HTTPPort int

	// 日志器
	Logger *slog.Logger

	// 服务元数据
	Metadata map[string]string
}

// InitOptionsFromNacosConfig 从 Nacos 配置创建选项
func InitOptionsFromNacosConfig(cfg *config.NacosConfig, serviceName string, grpcPort, httpPort int) *InitOptions {
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "/tmp/nacos/log"
	}
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = "/tmp/nacos/cache"
	}

	return &InitOptions{
		ServiceName:     serviceName,
		NacosEnabled:    cfg.Enabled,
		NacosServerAddr: cfg.ServerAddr,
		NacosNamespace:  cfg.Namespace,
		NacosGroup:      cfg.Group,
		NacosUsername:   cfg.Username,
		NacosPassword:   cfg.Password,
		NacosLogDir:     logDir,
		NacosCacheDir:   cacheDir,
		GRPCPort:        grpcPort,
		HTTPPort:        httpPort,
		Logger:          slog.Default(),
		Metadata:        make(map[string]string),
	}
}

// WithMetadata 设置元数据
func (o *InitOptions) WithMetadata(key, value string) *InitOptions {
	o.Metadata[key] = value
	return o
}

// WithDependencies 设置服务依赖
func (o *InitOptions) WithDependencies(deps ...string) *InitOptions {
	if len(deps) > 0 {
		o.Metadata["dependencies"] = joinStrings(deps, ",")
	}
	return o
}

// WithKafkaTopics 设置 Kafka 主题（生产和消费）
func (o *InitOptions) WithKafkaTopics(produce, consume []string) *InitOptions {
	if len(produce) > 0 {
		o.Metadata["kafka_produce"] = joinStrings(produce, ",")
	}
	if len(consume) > 0 {
		o.Metadata["kafka_consume"] = joinStrings(consume, ",")
	}
	return o
}

// joinStrings 用分隔符连接字符串
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// Infrastructure 基础设施
type Infrastructure struct {
	ServiceDiscovery *ServiceDiscovery
	ConfigCenter     *nacos.ConfigCenter
	ConfigLoader     *ConfigLoader
	Logger           *slog.Logger

	serviceName string
	grpcPort    uint64
	metadata    map[string]string
	registered  bool
}

// NewInfrastructure 创建基础设施
func NewInfrastructure(opts *InitOptions) (*Infrastructure, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	infra := &Infrastructure{
		Logger:      opts.Logger,
		serviceName: opts.ServiceName,
		grpcPort:    uint64(opts.GRPCPort),
		metadata:    opts.Metadata,
	}

	// 创建服务发现配置 (Nacos 必须启用)
	logDir := opts.NacosLogDir
	if logDir == "" {
		logDir = "/tmp/nacos/log"
	}
	cacheDir := opts.NacosCacheDir
	if cacheDir == "" {
		cacheDir = "/tmp/nacos/cache"
	}

	sdConfig := &Config{
		NacosEnabled:        opts.NacosEnabled,
		NacosServerAddr:     opts.NacosServerAddr,
		NacosNamespace:      opts.NacosNamespace,
		NacosGroup:          opts.NacosGroup,
		NacosUsername:       opts.NacosUsername,
		NacosPassword:       opts.NacosPassword,
		NacosLogDir:         logDir,
		NacosCacheDir:       cacheDir,
		LoadBalancingPolicy: "round_robin",
	}

	// 创建服务发现
	sd, err := NewServiceDiscovery(sdConfig, opts.Logger)
	if err != nil {
		return nil, fmt.Errorf("create service discovery: %w", err)
	}
	infra.ServiceDiscovery = sd

	// 创建配置中心
	configCenter, err := CreateConfigCenter(
		opts.NacosServerAddr,
		opts.NacosNamespace,
		opts.NacosGroup,
		opts.NacosUsername,
		opts.NacosPassword,
	)
	if err != nil {
		opts.Logger.Warn("failed to create config center, config hot reload disabled",
			"error", err,
		)
	} else {
		infra.ConfigCenter = configCenter
		infra.ConfigLoader = NewConfigLoader(configCenter, opts.ServiceName, WithGroup(opts.NacosGroup), WithLogger(opts.Logger))
	}

	return infra, nil
}

// RegisterService 注册当前服务到 Nacos
func (infra *Infrastructure) RegisterService(extraMetadata map[string]string) error {
	if infra.registered {
		return nil
	}

	// 合并 Infrastructure 保存的元数据和额外元数据
	metadata := make(map[string]string)
	for k, v := range infra.metadata {
		metadata[k] = v
	}
	for k, v := range extraMetadata {
		metadata[k] = v
	}

	if err := infra.ServiceDiscovery.RegisterService(infra.serviceName, infra.grpcPort, metadata); err != nil {
		return err
	}

	infra.registered = true
	return nil
}

// GetServiceConnection 获取服务连接
func (infra *Infrastructure) GetServiceConnection(ctx context.Context, serviceName string) (*grpc.ClientConn, error) {
	return infra.ServiceDiscovery.GetConnection(ctx, serviceName)
}

// WaitForServices 等待依赖服务可用
func (infra *Infrastructure) WaitForServices(ctx context.Context, services ...string) error {
	for _, svc := range services {
		if err := infra.ServiceDiscovery.WaitForService(ctx, svc, 30); err != nil {
			infra.Logger.Warn("service not available",
				"service", svc,
				"error", err,
			)
			// 不阻塞启动，只记录警告
		}
	}
	return nil
}

// Close 关闭基础设施
func (infra *Infrastructure) Close() error {
	var lastErr error

	if infra.ConfigLoader != nil {
		infra.ConfigLoader.Close()
	}

	if infra.ConfigCenter != nil {
		infra.ConfigCenter.Close()
	}

	if infra.ServiceDiscovery != nil {
		if err := infra.ServiceDiscovery.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// IsNacosEnabled 检查 Nacos 是否启用
func (infra *Infrastructure) IsNacosEnabled() bool {
	return infra.ServiceDiscovery != nil && infra.ServiceDiscovery.IsNacosEnabled()
}

// GetNacosHelper 获取 Nacos 助手（用于高级操作）
func (infra *Infrastructure) GetNacosHelper() *nacos.ServiceHelper {
	if infra.ServiceDiscovery != nil {
		return infra.ServiceDiscovery.NacosHelper()
	}
	return nil
}

// LoadConfig 从 Nacos 加载配置
func (infra *Infrastructure) LoadConfig(dataId string, target interface{}) error {
	if infra.ConfigLoader != nil {
		return infra.ConfigLoader.LoadConfig(dataId, target)
	}
	return nil
}

// LoadConfigWithWatch 从 Nacos 加载配置并监听变更
func (infra *Infrastructure) LoadConfigWithWatch(dataId string, target interface{}, onChange func(interface{})) error {
	if infra.ConfigLoader != nil {
		return infra.ConfigLoader.LoadConfigWithWatch(dataId, target, onChange)
	}
	return nil
}

// PublishServiceConfig 将服务配置发布到 Nacos 配置中心
func (infra *Infrastructure) PublishServiceConfig(config interface{}) error {
	if infra.ConfigLoader == nil {
		infra.Logger.Warn("config loader not available, skipping config publish")
		return nil
	}

	dataId := infra.serviceName + ".yaml"
	if err := infra.ConfigLoader.PublishConfig(dataId, config); err != nil {
		infra.Logger.Warn("failed to publish service config",
			"dataId", dataId,
			"error", err,
		)
		return err
	}

	infra.Logger.Info("service config published to nacos",
		"dataId", dataId,
	)
	return nil
}
