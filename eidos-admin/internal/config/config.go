package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service     ServiceConfig         `yaml:"service" json:"service"`
	Nacos       config.NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres    config.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis       config.RedisConfig    `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig     `yaml:"grpc_clients" json:"grpc_clients"`
	JWT         JWTConfig             `yaml:"jwt" json:"jwt"`
	Log         LogConfig             `yaml:"log" json:"log"`
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
	Chain    string `yaml:"chain" json:"chain"`
	Risk     string `yaml:"risk" json:"risk"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret" json:"secret"`
	ExpireHours int    `yaml:"expire_hours" json:"expire_hours"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-admin",
			HTTPPort: 8088,
			Env:      "dev",
		},
	}, nil
}
