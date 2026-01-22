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
	return &InitOptions{
		ServiceName:     serviceName,
		NacosEnabled:    cfg.Enabled,
		NacosServerAddr: cfg.ServerAddr,
		NacosNamespace:  cfg.Namespace,
		NacosGroup:      cfg.Group,
		NacosUsername:   cfg.Username,
		NacosPassword:   cfg.Password,
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

// Infrastructure 基础设施
type Infrastructure struct {
	ServiceDiscovery *ServiceDiscovery
	ConfigCenter     *nacos.ConfigCenter
	ConfigLoader     *ConfigLoader
	Logger           *slog.Logger

	serviceName string
	grpcPort    uint64
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
	}

	// 创建服务发现配置 (Nacos 必须启用)
	sdConfig := &Config{
		NacosEnabled:        opts.NacosEnabled,
		NacosServerAddr:     opts.NacosServerAddr,
		NacosNamespace:      opts.NacosNamespace,
		NacosGroup:          opts.NacosGroup,
		NacosUsername:       opts.NacosUsername,
		NacosPassword:       opts.NacosPassword,
		NacosLogDir:         "/tmp/nacos/log",
		NacosCacheDir:       "/tmp/nacos/cache",
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
func (infra *Infrastructure) RegisterService(metadata map[string]string) error {
	if infra.registered {
		return nil
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["version"] = "1.0.0"

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
