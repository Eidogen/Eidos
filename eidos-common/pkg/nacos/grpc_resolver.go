package nacos

import (
	"fmt"
	"sync"

	"google.golang.org/grpc/resolver"
)

const (
	// NacosScheme gRPC resolver scheme
	NacosScheme = "nacos"
)

// GRPCResolver Nacos gRPC 服务解析器
type GRPCResolver struct {
	discovery   *Discovery
	serviceName string
	groupName   string
	cc          resolver.ClientConn
	wg          sync.WaitGroup
	closeCh     chan struct{}
}

// GRPCResolverBuilder Nacos gRPC 解析器构建器
type GRPCResolverBuilder struct {
	discovery *Discovery
	groupName string
}

// NewGRPCResolverBuilder 创建 gRPC 解析器构建器
func NewGRPCResolverBuilder(discovery *Discovery, groupName string) *GRPCResolverBuilder {
	return &GRPCResolverBuilder{
		discovery: discovery,
		groupName: groupName,
	}
}

// Build 构建解析器
func (b *GRPCResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()
	if serviceName == "" {
		return nil, fmt.Errorf("invalid target endpoint")
	}

	r := &GRPCResolver{
		discovery:   b.discovery,
		serviceName: serviceName,
		groupName:   b.groupName,
		cc:          cc,
		closeCh:     make(chan struct{}),
	}

	// 初始解析
	if err := r.resolve(); err != nil {
		return nil, err
	}

	// 订阅服务变更
	r.wg.Add(1)
	go r.watch()

	return r, nil
}

// Scheme 返回解析器 scheme
func (b *GRPCResolverBuilder) Scheme() string {
	return NacosScheme
}

// ResolveNow 立即解析
func (r *GRPCResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	_ = r.resolve()
}

// Close 关闭解析器
func (r *GRPCResolver) Close() {
	close(r.closeCh)
	r.wg.Wait()
	_ = r.discovery.UnsubscribeWithGroup(r.serviceName, r.groupName)
}

// resolve 解析服务地址
func (r *GRPCResolver) resolve() error {
	instances, err := r.discovery.GetServiceWithGroup(r.serviceName, r.groupName)
	if err != nil {
		return err
	}

	addrs := make([]resolver.Address, 0, len(instances))
	for _, inst := range instances {
		if inst.Healthy && inst.Enable {
			addrs = append(addrs, resolver.Address{
				Addr:       inst.Address(),
				Attributes: nil,
			})
		}
	}

	if len(addrs) == 0 {
		return fmt.Errorf("no healthy instance for service %s", r.serviceName)
	}

	err = r.cc.UpdateState(resolver.State{
		Addresses: addrs,
	})

	return err
}

// watch 监听服务变更
func (r *GRPCResolver) watch() {
	defer r.wg.Done()

	err := r.discovery.SubscribeWithGroup(r.serviceName, r.groupName, func(instances []*Instance) {
		addrs := make([]resolver.Address, 0, len(instances))
		for _, inst := range instances {
			if inst.Healthy && inst.Enable {
				addrs = append(addrs, resolver.Address{
					Addr: inst.Address(),
				})
			}
		}

		if len(addrs) > 0 {
			_ = r.cc.UpdateState(resolver.State{
				Addresses: addrs,
			})
		}
	})

	if err != nil {
		return
	}

	<-r.closeCh
}

// RegisterResolver 注册 Nacos gRPC 解析器到全局
func RegisterResolver(discovery *Discovery, groupName string) {
	builder := NewGRPCResolverBuilder(discovery, groupName)
	resolver.Register(builder)
}

// BuildTarget 构建 gRPC 目标地址
// 返回格式: nacos:///serviceName
func BuildTarget(serviceName string) string {
	return fmt.Sprintf("%s:///%s", NacosScheme, serviceName)
}
