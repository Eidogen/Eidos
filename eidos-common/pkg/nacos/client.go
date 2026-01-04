package nacos

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Config Nacos 配置
type Config struct {
	ServerAddr   string `yaml:"server_addr" json:"server_addr"`       // Nacos 服务器地址 host:port
	Namespace    string `yaml:"namespace" json:"namespace"`           // 命名空间 ID
	Group        string `yaml:"group" json:"group"`                   // 服务分组
	Username     string `yaml:"username" json:"username"`             // 用户名 (可选)
	Password     string `yaml:"password" json:"password"`             // 密码 (可选)
	LogDir       string `yaml:"log_dir" json:"log_dir"`               // 日志目录
	CacheDir     string `yaml:"cache_dir" json:"cache_dir"`           // 缓存目录
	LogLevel     string `yaml:"log_level" json:"log_level"`           // 日志级别
	TimeoutMs    uint64 `yaml:"timeout_ms" json:"timeout_ms"`         // 超时时间 (毫秒)
	NotLoadCache bool   `yaml:"not_load_cache" json:"not_load_cache"` // 是否不加载缓存
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ServerAddr:   "127.0.0.1:8848",
		Namespace:    "public",
		Group:        "DEFAULT_GROUP",
		LogDir:       "/tmp/nacos/log",
		CacheDir:     "/tmp/nacos/cache",
		LogLevel:     "warn",
		TimeoutMs:    5000,
		NotLoadCache: true,
	}
}

// Client Nacos 客户端
type Client struct {
	namingClient naming_client.INamingClient
	config       *Config
}

// NewClient 创建 Nacos 客户端
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 解析服务器地址
	serverConfigs, err := parseServerAddr(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("parse server addr failed: %w", err)
	}

	// 客户端配置
	clientConfig := constant.ClientConfig{
		NamespaceId:         cfg.Namespace,
		TimeoutMs:           cfg.TimeoutMs,
		NotLoadCacheAtStart: cfg.NotLoadCache,
		LogDir:              cfg.LogDir,
		CacheDir:            cfg.CacheDir,
		LogLevel:            cfg.LogLevel,
	}

	if cfg.Username != "" {
		clientConfig.Username = cfg.Username
		clientConfig.Password = cfg.Password
	}

	// 创建命名客户端
	namingClient, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create naming client failed: %w", err)
	}

	return &Client{
		namingClient: namingClient,
		config:       cfg,
	}, nil
}

// parseServerAddr 解析服务器地址 (支持多个地址，逗号分隔)
func parseServerAddr(addr string) ([]constant.ServerConfig, error) {
	addrs := strings.Split(addr, ",")
	configs := make([]constant.ServerConfig, 0, len(addrs))

	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}

		host, portStr, err := net.SplitHostPort(a)
		if err != nil {
			// 可能只有 host，没有端口
			host = a
			portStr = "8848"
		}

		port, err := strconv.ParseUint(portStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid port %s: %w", portStr, err)
		}

		configs = append(configs, constant.ServerConfig{
			IpAddr: host,
			Port:   port,
		})
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid server address")
	}

	return configs, nil
}

// Close 关闭客户端
func (c *Client) Close() {
	if c.namingClient != nil {
		c.namingClient.CloseClient()
	}
}

// NamingClient 返回底层的 naming client
func (c *Client) NamingClient() naming_client.INamingClient {
	return c.namingClient
}

// Config 返回配置
func (c *Client) Config() *Config {
	return c.config
}
