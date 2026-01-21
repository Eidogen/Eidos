// Package config 提供配置加载
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Service     ServiceConfig     `yaml:"service" json:"service"`
	Nacos       NacosConfig       `yaml:"nacos" json:"nacos"`
	Redis       RedisConfig       `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig `yaml:"grpc_clients" json:"grpc_clients"`
	EIP712      EIP712Config      `yaml:"eip712" json:"eip712"`
	RateLimit   RateLimitConfig   `yaml:"rate_limit" json:"rate_limit"`
	WebSocket   WebSocketConfig   `yaml:"websocket" json:"websocket"`
	Risk        RiskConfig        `yaml:"risk" json:"risk"`
	Log         LogConfig         `yaml:"log" json:"log"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

// NacosConfig Nacos 配置
type NacosConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	ServerAddr string `yaml:"server_addr" json:"server_addr"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Group      string `yaml:"group" json:"group"`
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	LogDir     string `yaml:"log_dir" json:"log_dir"`
	CacheDir   string `yaml:"cache_dir" json:"cache_dir"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`
	PoolSize int    `yaml:"pool_size" json:"pool_size"`
}

// Addr 返回 Redis 地址
func (c *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GRPCClientsConfig gRPC 客户端配置
type GRPCClientsConfig struct {
	Trading  string `yaml:"trading" json:"trading"`
	Matching string `yaml:"matching" json:"matching"`
	Market   string `yaml:"market" json:"market"`
	Risk     string `yaml:"risk" json:"risk"`
}

// EIP712Config EIP-712 签名配置
type EIP712Config struct {
	Domain               EIP712DomainConfig `yaml:"domain" json:"domain"`
	MockMode             bool               `yaml:"mock_mode" json:"mock_mode"`
	TimestampToleranceMs int64              `yaml:"timestamp_tolerance_ms" json:"timestamp_tolerance_ms"`
}

// EIP712DomainConfig EIP-712 域配置
type EIP712DomainConfig struct {
	Name              string `yaml:"name" json:"name"`
	Version           string `yaml:"version" json:"version"`
	ChainID           int64  `yaml:"chain_id" json:"chain_id"`
	VerifyingContract string `yaml:"verifying_contract" json:"verifying_contract"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled   bool            `yaml:"enabled" json:"enabled"`
	PerWallet WalletRateLimit `yaml:"per_wallet" json:"per_wallet"`
	PerIP     IPRateLimit     `yaml:"per_ip" json:"per_ip"`
	Global    GlobalRateLimit `yaml:"global" json:"global"`
}

// WalletRateLimit 按钱包限流
type WalletRateLimit struct {
	Orders      int `yaml:"orders" json:"orders"`           // 下单 次/秒
	Queries     int `yaml:"queries" json:"queries"`         // 查询 次/秒
	Withdrawals int `yaml:"withdrawals" json:"withdrawals"` // 提现 次/秒
	Cancels     int `yaml:"cancels" json:"cancels"`         // 取消 次/秒
}

// IPRateLimit 按 IP 限流
type IPRateLimit struct {
	Total     int `yaml:"total" json:"total"`           // 总请求 次/分钟
	WebSocket int `yaml:"websocket" json:"websocket"`   // WebSocket 连接 次/分钟
	PublicAPI int `yaml:"public_api" json:"public_api"` // 公开 API 次/秒
}

// GlobalRateLimit 全局限流
type GlobalRateLimit struct {
	Orders int `yaml:"orders" json:"orders"` // 全局下单 次/秒
}

// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
	ReadBufferSize    int      `yaml:"read_buffer_size" json:"read_buffer_size"`
	WriteBufferSize   int      `yaml:"write_buffer_size" json:"write_buffer_size"`
	PingIntervalSec   int      `yaml:"ping_interval" json:"ping_interval"`
	PongTimeoutSec    int      `yaml:"pong_timeout" json:"pong_timeout"`
	MaxConnections    int      `yaml:"max_connections" json:"max_connections"`
	MaxSubscriptions  int      `yaml:"max_subscriptions" json:"max_subscriptions"`
	WriteWaitSec      int      `yaml:"write_wait" json:"write_wait"`
	MaxMessageSize    int      `yaml:"max_message_size" json:"max_message_size"`
	AllowedOrigins    []string `yaml:"allowed_origins" json:"allowed_origins"`
	AllowAllOrigins   bool     `yaml:"allow_all_origins" json:"allow_all_origins"`
	AuthTimeoutSec    int      `yaml:"auth_timeout" json:"auth_timeout"`
	EnablePrivateAuth bool     `yaml:"enable_private_auth" json:"enable_private_auth"`
}

// RiskConfig 风控服务配置
type RiskConfig struct {
	Enabled  bool `yaml:"enabled" json:"enabled"`
	FailOpen bool `yaml:"fail_open" json:"fail_open"` // 风控服务不可用时是否放行
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// PingInterval 返回心跳间隔
func (c *WebSocketConfig) PingInterval() time.Duration {
	return time.Duration(c.PingIntervalSec) * time.Second
}

// PongTimeout 返回 Pong 超时
func (c *WebSocketConfig) PongTimeout() time.Duration {
	return time.Duration(c.PongTimeoutSec) * time.Second
}

// WriteWait 返回写超时
func (c *WebSocketConfig) WriteWait() time.Duration {
	return time.Duration(c.WriteWaitSec) * time.Second
}

// AuthTimeout 返回认证超时
func (c *WebSocketConfig) AuthTimeout() time.Duration {
	if c.AuthTimeoutSec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.AuthTimeoutSec) * time.Second
}

// Load 加载配置
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	// 从文件加载
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		// 展开环境变量: ${VAR:DEFAULT}
		expanded := config.ExpandEnv(string(data))
		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	// 环境变量覆盖
	overrideFromEnv(cfg)

	return cfg, nil
}

// defaultConfig 默认配置
func defaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-api",
			HTTPPort: 8080,
			Env:      "dev",
		},
		Nacos: NacosConfig{
			Enabled:   false,
			Namespace: "public",
			Group:     "EIDOS_GROUP",
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			PoolSize: 100,
		},
		GRPCClients: GRPCClientsConfig{
			Trading: "localhost:50051",
			Market:  "localhost:50053",
			Risk:    "localhost:50055",
		},
		EIP712: EIP712Config{
			Domain: EIP712DomainConfig{
				Name:              "EidosExchange",
				Version:           "1",
				ChainID:           31337,
				VerifyingContract: "0x0000000000000000000000000000000000000000",
			},
			MockMode:             true,
			TimestampToleranceMs: 300000, // 5 分钟
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
			PerWallet: WalletRateLimit{
				Orders:      10,
				Queries:     100,
				Withdrawals: 1,
				Cancels:     20,
			},
			PerIP: IPRateLimit{
				Total:     1000,
				WebSocket: 5,
				PublicAPI: 100,
			},
			Global: GlobalRateLimit{
				Orders: 10000,
			},
		},
		WebSocket: WebSocketConfig{
			ReadBufferSize:    1024,
			WriteBufferSize:   1024,
			PingIntervalSec:   30,
			PongTimeoutSec:    10,
			MaxConnections:    100000,
			MaxSubscriptions:  50,
			WriteWaitSec:      10,
			MaxMessageSize:    4096,
			AllowedOrigins:    []string{},
			AllowAllOrigins:   true, // dev mode: allow all
			AuthTimeoutSec:    30,
			EnablePrivateAuth: true,
		},
		Risk: RiskConfig{
			Enabled:  true,
			FailOpen: false,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// overrideFromEnv 从环境变量覆盖配置
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.Service.Name = v
	}
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Service.HTTPPort = port
		}
	}
	if v := os.Getenv("ENV"); v != "" {
		cfg.Service.Env = v
	}

	// Redis
	if v := os.Getenv("REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("REDIS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Redis.Port = port
		}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}

	// gRPC Clients
	if v := os.Getenv("GRPC_TRADING_ADDR"); v != "" {
		cfg.GRPCClients.Trading = v
	}
	if v := os.Getenv("GRPC_MARKET_ADDR"); v != "" {
		cfg.GRPCClients.Market = v
	}

	// Nacos
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		cfg.Nacos.Enabled = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("NACOS_SERVER_ADDR"); v != "" {
		cfg.Nacos.ServerAddr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		cfg.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_GROUP"); v != "" {
		cfg.Nacos.Group = v
	}

	// EIP712
	if v := os.Getenv("EIP712_MOCK_MODE"); v != "" {
		cfg.EIP712.MockMode = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("EIP712_CHAIN_ID"); v != "" {
		if chainID, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.EIP712.Domain.ChainID = chainID
		}
	}

	// Rate Limit
	if v := os.Getenv("RATE_LIMIT_ENABLED"); v != "" {
		cfg.RateLimit.Enabled = strings.ToLower(v) == "true"
	}

	// Risk
	if v := os.Getenv("RISK_ENABLED"); v != "" {
		cfg.Risk.Enabled = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("RISK_FAIL_OPEN"); v != "" {
		cfg.Risk.FailOpen = strings.ToLower(v) == "true"
	}

	// WebSocket
	if v := os.Getenv("WS_ALLOW_ALL_ORIGINS"); v != "" {
		cfg.WebSocket.AllowAllOrigins = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("WS_ENABLE_PRIVATE_AUTH"); v != "" {
		cfg.WebSocket.EnablePrivateAuth = strings.ToLower(v) == "true"
	}
}
