package nacos

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// ServiceInstance 服务实例信息
type ServiceInstance struct {
	ServiceName string            // 服务名称
	IP          string            // 实例 IP
	Port        uint64            // 实例端口
	Weight      float64           // 权重 (默认 1.0)
	Enable      bool              // 是否启用
	Healthy     bool              // 是否健康
	Ephemeral   bool              // 是否临时实例
	Metadata    map[string]string // 元数据
	ClusterName string            // 集群名称
	GroupName   string            // 分组名称
}

// DefaultServiceInstance 返回默认服务实例配置
func DefaultServiceInstance(serviceName string, port uint64) *ServiceInstance {
	ip := getLocalIP()
	return &ServiceInstance{
		ServiceName: serviceName,
		IP:          ip,
		Port:        port,
		Weight:      1.0,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata:    make(map[string]string),
		ClusterName: "DEFAULT",
		GroupName:   "DEFAULT_GROUP",
	}
}

// Registry 服务注册器
type Registry struct {
	client    *Client
	instances sync.Map // map[string]*ServiceInstance 已注册的实例
}

// NewRegistry 创建服务注册器
func NewRegistry(client *Client) *Registry {
	return &Registry{
		client: client,
	}
}

// Register 注册服务实例
func (r *Registry) Register(instance *ServiceInstance) error {
	if instance.GroupName == "" {
		instance.GroupName = r.client.config.Group
	}

	success, err := r.client.namingClient.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          instance.IP,
		Port:        instance.Port,
		ServiceName: instance.ServiceName,
		Weight:      instance.Weight,
		Enable:      instance.Enable,
		Healthy:     instance.Healthy,
		Ephemeral:   instance.Ephemeral,
		Metadata:    instance.Metadata,
		ClusterName: instance.ClusterName,
		GroupName:   instance.GroupName,
	})

	if err != nil {
		return fmt.Errorf("register instance failed: %w", err)
	}

	if !success {
		return fmt.Errorf("register instance failed: unknown error")
	}

	// 保存已注册的实例
	key := fmt.Sprintf("%s:%s:%d", instance.GroupName, instance.ServiceName, instance.Port)
	r.instances.Store(key, instance)

	return nil
}

// Deregister 注销服务实例
func (r *Registry) Deregister(instance *ServiceInstance) error {
	if instance.GroupName == "" {
		instance.GroupName = r.client.config.Group
	}

	success, err := r.client.namingClient.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          instance.IP,
		Port:        instance.Port,
		ServiceName: instance.ServiceName,
		Cluster:     instance.ClusterName,
		GroupName:   instance.GroupName,
		Ephemeral:   instance.Ephemeral,
	})

	if err != nil {
		return fmt.Errorf("deregister instance failed: %w", err)
	}

	if !success {
		return fmt.Errorf("deregister instance failed: unknown error")
	}

	// 移除已注册的实例
	key := fmt.Sprintf("%s:%s:%d", instance.GroupName, instance.ServiceName, instance.Port)
	r.instances.Delete(key)

	return nil
}

// DeregisterAll 注销所有已注册的实例
func (r *Registry) DeregisterAll() error {
	var lastErr error
	r.instances.Range(func(key, value interface{}) bool {
		instance := value.(*ServiceInstance)
		if err := r.Deregister(instance); err != nil {
			lastErr = err
		}
		return true
	})
	return lastErr
}

// UpdateInstance 更新服务实例
func (r *Registry) UpdateInstance(instance *ServiceInstance) error {
	if instance.GroupName == "" {
		instance.GroupName = r.client.config.Group
	}

	success, err := r.client.namingClient.UpdateInstance(vo.UpdateInstanceParam{
		Ip:          instance.IP,
		Port:        instance.Port,
		ServiceName: instance.ServiceName,
		Weight:      instance.Weight,
		Enable:      instance.Enable,
		Ephemeral:   instance.Ephemeral,
		Metadata:    instance.Metadata,
		ClusterName: instance.ClusterName,
		GroupName:   instance.GroupName,
	})

	if err != nil {
		return fmt.Errorf("update instance failed: %w", err)
	}

	if !success {
		return fmt.Errorf("update instance failed: unknown error")
	}

	return nil
}

// SetInstanceHealth 设置实例健康状态
func (r *Registry) SetInstanceHealth(instance *ServiceInstance, healthy bool) error {
	instance.Healthy = healthy
	return r.UpdateInstance(instance)
}

// getLocalIP 获取本机 IP
func getLocalIP() string {
	// 优先从环境变量获取
	if ip := os.Getenv("POD_IP"); ip != "" {
		return ip
	}
	if ip := os.Getenv("HOST_IP"); ip != "" {
		return ip
	}

	// 获取本机网卡 IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}

// GetLocalIP 公开获取本机 IP 的方法
func GetLocalIP() string {
	return getLocalIP()
}
