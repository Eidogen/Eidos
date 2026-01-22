package nacos

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ServiceHelper 服务助手，封装常用操作
type ServiceHelper struct {
	client    *Client
	registry  *Registry
	discovery *Discovery
	config    *Config
}

// NewServiceHelper 创建服务助手
func NewServiceHelper(cfg *Config) (*ServiceHelper, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create nacos client failed: %w", err)
	}

	return &ServiceHelper{
		client:    client,
		registry:  NewRegistry(client),
		discovery: NewDiscovery(client),
		config:    cfg,
	}, nil
}

// Client 返回 Nacos 客户端
func (h *ServiceHelper) Client() *Client {
	return h.client
}

// Registry 返回服务注册器
func (h *ServiceHelper) Registry() *Registry {
	return h.registry
}

// Discovery 返回服务发现器
func (h *ServiceHelper) Discovery() *Discovery {
	return h.discovery
}

// RegisterService 注册服务
func (h *ServiceHelper) RegisterService(serviceName string, port uint64, metadata map[string]string) error {
	instance := DefaultServiceInstance(serviceName, port)
	instance.GroupName = h.config.Group
	if metadata != nil {
		instance.Metadata = metadata
	}

	// 添加重试逻辑，解决 Nacos 启动瞬间客户端状态可能为 STARTING 的问题
	var err error
	for i := 0; i < 5; i++ {
		err = h.registry.Register(instance)
		if err == nil {
			return nil
		}
		if i < 4 {
			time.Sleep(1 * time.Second)
		}
	}
	return fmt.Errorf("register service failed after 5 retries: %w", err)
}

// DeregisterAll 注销所有服务
func (h *ServiceHelper) DeregisterAll() error {
	return h.registry.DeregisterAll()
}

// GetServiceAddress 获取服务地址 (随机负载均衡)
func (h *ServiceHelper) GetServiceAddress(serviceName string) (string, error) {
	inst, err := h.discovery.GetOneHealthy(serviceName)
	if err != nil {
		return "", err
	}
	return inst.Address(), nil
}

// GetServiceAddresses 获取服务所有健康地址
func (h *ServiceHelper) GetServiceAddresses(serviceName string) ([]string, error) {
	instances, err := h.discovery.GetService(serviceName)
	if err != nil {
		return nil, err
	}

	addrs := make([]string, 0, len(instances))
	for _, inst := range instances {
		if inst.Healthy && inst.Enable {
			addrs = append(addrs, inst.Address())
		}
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no healthy instance for service %s", serviceName)
	}

	return addrs, nil
}

// DialGRPC 直接拨号到服务 (使用服务发现)
func (h *ServiceHelper) DialGRPC(ctx context.Context, serviceName string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	addr, err := h.GetServiceAddress(serviceName)
	if err != nil {
		return nil, err
	}

	// 默认使用不安全连接
	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	opts = append(defaultOpts, opts...)

	return grpc.DialContext(ctx, addr, opts...)
}

// DialGRPCWithResolver 使用 Nacos resolver 拨号 (支持客户端负载均衡)
func (h *ServiceHelper) DialGRPCWithResolver(ctx context.Context, serviceName string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// 注册解析器
	RegisterResolver(h.discovery, h.config.Group)

	// 构建目标地址
	target := BuildTarget(serviceName)

	// 默认使用不安全连接和 round_robin 负载均衡
	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	}
	opts = append(defaultOpts, opts...)

	return grpc.DialContext(ctx, target, opts...)
}

// Subscribe 订阅服务变更
func (h *ServiceHelper) Subscribe(serviceName string, callback func(addrs []string)) error {
	return h.discovery.Subscribe(serviceName, func(instances []*Instance) {
		addrs := make([]string, 0, len(instances))
		for _, inst := range instances {
			if inst.Healthy && inst.Enable {
				addrs = append(addrs, inst.Address())
			}
		}
		callback(addrs)
	})
}

// Close 关闭助手
func (h *ServiceHelper) Close() error {
	var lastErr error

	// 注销所有服务
	if err := h.registry.DeregisterAll(); err != nil {
		lastErr = err
	}

	// 取消所有订阅
	if err := h.discovery.UnsubscribeAll(); err != nil {
		lastErr = err
	}

	// 关闭客户端
	h.client.Close()

	return lastErr
}

// WaitForService 等待服务可用
func (h *ServiceHelper) WaitForService(ctx context.Context, serviceName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for service %s", serviceName)
		case <-ticker.C:
			if _, err := h.discovery.GetOneHealthy(serviceName); err == nil {
				return nil
			}
		}
	}
}

// HealthCheck 健康检查
func (h *ServiceHelper) HealthCheck(ctx context.Context) error {
	// 尝试获取一个服务实例来验证连接
	_, err := h.discovery.GetService("nacos.naming.public")
	// 忽略 "no instance" 错误，只要能连接就行
	if err != nil && err.Error() != "no healthy instance for service nacos.naming.public" {
		return err
	}
	return nil
}
