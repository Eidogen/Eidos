package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service     ServiceConfig      `yaml:"service" json:"service"`
	Nacos       config.NacosConfig `yaml:"nacos" json:"nacos"`
	Redis       config.RedisConfig `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig  `yaml:"grpc_clients" json:"grpc_clients"`
	Jobs        JobsConfig         `yaml:"jobs" json:"jobs"`
	Log         LogConfig          `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type GRPCClientsConfig struct {
	Trading string `yaml:"trading" json:"trading"`
	Chain   string `yaml:"chain" json:"chain"`
	Market  string `yaml:"market" json:"market"`
}

type JobsConfig struct {
	Reconciliation JobConfig `yaml:"reconciliation" json:"reconciliation"`
	Archive        JobConfig `yaml:"archive" json:"archive"`
	Stats          JobConfig `yaml:"stats" json:"stats"`
	ExpireOrders   JobConfig `yaml:"expire_orders" json:"expire_orders"`
}

type JobConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Cron          string `yaml:"cron" json:"cron"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-jobs",
			GRPCPort: 50056,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
