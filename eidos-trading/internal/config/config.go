package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

// Config 服务配置
type Config struct {
	Service  ServiceConfig         `yaml:"service" json:"service"`
	Nacos    config.NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres config.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis    config.RedisConfig    `yaml:"redis" json:"redis"`
	Kafka    config.KafkaConfig    `yaml:"kafka" json:"kafka"`
	EIP712   EIP712Config          `yaml:"eip712" json:"eip712"`
	Log      LogConfig             `yaml:"log" json:"log"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

// EIP712Config EIP-712 配置
type EIP712Config struct {
	Domain EIP712Domain `yaml:"domain" json:"domain"`
}

// EIP712Domain EIP-712 域
type EIP712Domain struct {
	Name              string `yaml:"name" json:"name"`
	Version           string `yaml:"version" json:"version"`
	ChainID           int64  `yaml:"chain_id" json:"chain_id"`
	VerifyingContract string `yaml:"verifying_contract" json:"verifying_contract"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// Load 加载配置
func Load() (*Config, error) {
	// TODO: 从配置文件和环境变量加载
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-trading",
			GRPCPort: 50051,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
