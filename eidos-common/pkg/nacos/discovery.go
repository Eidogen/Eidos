package nacos

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Instance 服务实例
type Instance struct {
	IP          string
	Port        uint64
	Weight      float64
	Healthy     bool
	Enable      bool
	Metadata    map[string]string
	ClusterName string
	ServiceName string
}

// Address 返回实例地址 host:port
func (i *Instance) Address() string {
	return fmt.Sprintf("%s:%d", i.IP, i.Port)
}

// Discovery 服务发现器
type Discovery struct {
	client      *Client
	cache       sync.Map // map[string][]*Instance 服务实例缓存
	subscribers sync.Map // map[string]bool 已订阅的服务
	rand        *rand.Rand
	mu          sync.RWMutex
}

// NewDiscovery 创建服务发现器
func NewDiscovery(client *Client) *Discovery {
	return &Discovery{
		client: client,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GetService 获取服务的所有健康实例
func (d *Discovery) GetService(serviceName string) ([]*Instance, error) {
	return d.GetServiceWithGroup(serviceName, d.client.config.Group)
}

// GetServiceWithGroup 获取指定分组的服务实例
func (d *Discovery) GetServiceWithGroup(serviceName, groupName string) ([]*Instance, error) {
	// 先从缓存获取
	cacheKey := d.cacheKey(serviceName, groupName)
	if cached, ok := d.cache.Load(cacheKey); ok {
		instances := cached.([]*Instance)
		if len(instances) > 0 {
			return instances, nil
		}
	}

	// 从 Nacos 获取
	instances, err := d.fetchInstances(serviceName, groupName)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	d.cache.Store(cacheKey, instances)

	return instances, nil
}

// GetOneHealthy 获取一个健康实例 (随机负载均衡)
func (d *Discovery) GetOneHealthy(serviceName string) (*Instance, error) {
	return d.GetOneHealthyWithGroup(serviceName, d.client.config.Group)
}

// GetOneHealthyWithGroup 获取指定分组的一个健康实例
func (d *Discovery) GetOneHealthyWithGroup(serviceName, groupName string) (*Instance, error) {
	instances, err := d.GetServiceWithGroup(serviceName, groupName)
	if err != nil {
		return nil, err
	}

	// 过滤健康实例
	healthyInstances := make([]*Instance, 0, len(instances))
	for _, inst := range instances {
		if inst.Healthy && inst.Enable {
			healthyInstances = append(healthyInstances, inst)
		}
	}

	if len(healthyInstances) == 0 {
		return nil, fmt.Errorf("no healthy instance for service %s", serviceName)
	}

	// 随机选择一个
	d.mu.RLock()
	idx := d.rand.Intn(len(healthyInstances))
	d.mu.RUnlock()

	return healthyInstances[idx], nil
}

// GetOneByWeight 根据权重获取一个实例
func (d *Discovery) GetOneByWeight(serviceName string) (*Instance, error) {
	return d.GetOneByWeightWithGroup(serviceName, d.client.config.Group)
}

// GetOneByWeightWithGroup 根据权重获取指定分组的一个实例
func (d *Discovery) GetOneByWeightWithGroup(serviceName, groupName string) (*Instance, error) {
	instances, err := d.GetServiceWithGroup(serviceName, groupName)
	if err != nil {
		return nil, err
	}

	// 过滤健康实例
	healthyInstances := make([]*Instance, 0, len(instances))
	var totalWeight float64
	for _, inst := range instances {
		if inst.Healthy && inst.Enable {
			healthyInstances = append(healthyInstances, inst)
			totalWeight += inst.Weight
		}
	}

	if len(healthyInstances) == 0 {
		return nil, fmt.Errorf("no healthy instance for service %s", serviceName)
	}

	// 根据权重随机选择
	d.mu.RLock()
	r := d.rand.Float64() * totalWeight
	d.mu.RUnlock()

	var acc float64
	for _, inst := range healthyInstances {
		acc += inst.Weight
		if r <= acc {
			return inst, nil
		}
	}

	// 兜底返回最后一个
	return healthyInstances[len(healthyInstances)-1], nil
}

// Subscribe 订阅服务变更
func (d *Discovery) Subscribe(serviceName string, callback func(instances []*Instance)) error {
	return d.SubscribeWithGroup(serviceName, d.client.config.Group, callback)
}

// SubscribeWithGroup 订阅指定分组的服务变更
func (d *Discovery) SubscribeWithGroup(serviceName, groupName string, callback func(instances []*Instance)) error {
	cacheKey := d.cacheKey(serviceName, groupName)

	// 检查是否已订阅
	if _, ok := d.subscribers.Load(cacheKey); ok {
		return nil
	}

	err := d.client.namingClient.Subscribe(&vo.SubscribeParam{
		ServiceName: serviceName,
		GroupName:   groupName,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil {
				return
			}

			instances := d.convertInstances(services)
			// 更新缓存
			d.cache.Store(cacheKey, instances)
			// 回调通知
			if callback != nil {
				callback(instances)
			}
		},
	})

	if err != nil {
		return fmt.Errorf("subscribe service failed: %w", err)
	}

	d.subscribers.Store(cacheKey, true)
	return nil
}

// Unsubscribe 取消订阅服务变更
func (d *Discovery) Unsubscribe(serviceName string) error {
	return d.UnsubscribeWithGroup(serviceName, d.client.config.Group)
}

// UnsubscribeWithGroup 取消订阅指定分组的服务变更
func (d *Discovery) UnsubscribeWithGroup(serviceName, groupName string) error {
	cacheKey := d.cacheKey(serviceName, groupName)

	if _, ok := d.subscribers.Load(cacheKey); !ok {
		return nil
	}

	err := d.client.namingClient.Unsubscribe(&vo.SubscribeParam{
		ServiceName: serviceName,
		GroupName:   groupName,
	})

	if err != nil {
		return fmt.Errorf("unsubscribe service failed: %w", err)
	}

	d.subscribers.Delete(cacheKey)
	d.cache.Delete(cacheKey)
	return nil
}

// UnsubscribeAll 取消所有订阅
func (d *Discovery) UnsubscribeAll() error {
	var lastErr error
	d.subscribers.Range(func(key, value interface{}) bool {
		cacheKey := key.(string)
		parts := d.parseCacheKey(cacheKey)
		if len(parts) == 2 {
			if err := d.UnsubscribeWithGroup(parts[0], parts[1]); err != nil {
				lastErr = err
			}
		}
		return true
	})
	return lastErr
}

// RefreshCache 刷新服务缓存
func (d *Discovery) RefreshCache(serviceName string) error {
	return d.RefreshCacheWithGroup(serviceName, d.client.config.Group)
}

// RefreshCacheWithGroup 刷新指定分组的服务缓存
func (d *Discovery) RefreshCacheWithGroup(serviceName, groupName string) error {
	instances, err := d.fetchInstances(serviceName, groupName)
	if err != nil {
		return err
	}

	cacheKey := d.cacheKey(serviceName, groupName)
	d.cache.Store(cacheKey, instances)
	return nil
}

// ClearCache 清除缓存
func (d *Discovery) ClearCache() {
	d.cache = sync.Map{}
}

// fetchInstances 从 Nacos 获取服务实例
func (d *Discovery) fetchInstances(serviceName, groupName string) ([]*Instance, error) {
	services, err := d.client.namingClient.SelectInstances(vo.SelectInstancesParam{
		ServiceName: serviceName,
		GroupName:   groupName,
		HealthyOnly: true,
	})

	if err != nil {
		return nil, fmt.Errorf("select instances failed: %w", err)
	}

	return d.convertInstances(services), nil
}

// convertInstances 转换实例格式
func (d *Discovery) convertInstances(services []model.Instance) []*Instance {
	instances := make([]*Instance, 0, len(services))
	for _, svc := range services {
		instances = append(instances, &Instance{
			IP:          svc.Ip,
			Port:        svc.Port,
			Weight:      svc.Weight,
			Healthy:     svc.Healthy,
			Enable:      svc.Enable,
			Metadata:    svc.Metadata,
			ClusterName: svc.ClusterName,
			ServiceName: svc.ServiceName,
		})
	}
	return instances
}

// cacheKey 生成缓存 key
func (d *Discovery) cacheKey(serviceName, groupName string) string {
	return serviceName + "@" + groupName
}

// parseCacheKey 解析缓存 key
func (d *Discovery) parseCacheKey(key string) []string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '@' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return nil
}
