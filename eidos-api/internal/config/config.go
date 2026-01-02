package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service     ServiceConfig       `yaml:"service" json:"service"`
	Nacos       config.NacosConfig  `yaml:"nacos" json:"nacos"`
	Redis       config.RedisConfig  `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig   `yaml:"grpc_clients" json:"grpc_clients"`
	EIP712      config.EIP712Config `yaml:"eip712" json:"eip712"`
	RateLimit   RateLimitConfig     `yaml:"rate_limit" json:"rate_limit"`
	WebSocket   WebSocketConfig     `yaml:"websocket" json:"websocket"`
	Log         LogConfig           `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type GRPCClientsConfig struct {
	Trading  string `yaml:"trading" json:"trading"`
	Matching string `yaml:"matching" json:"matching"`
	Market   string `yaml:"market" json:"market"`
	Risk     string `yaml:"risk" json:"risk"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled" json:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int  `yaml:"burst" json:"burst"`
}

type WebSocketConfig struct {
	ReadBufferSize  int `yaml:"read_buffer_size" json:"read_buffer_size"`
	WriteBufferSize int `yaml:"write_buffer_size" json:"write_buffer_size"`
	PingInterval    int `yaml:"ping_interval" json:"ping_interval"`
	PongTimeout     int `yaml:"pong_timeout" json:"pong_timeout"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-api",
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
