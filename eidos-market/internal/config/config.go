package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service  ServiceConfig         `yaml:"service" json:"service"`
	Nacos    config.NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres config.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis    config.RedisConfig    `yaml:"redis" json:"redis"`
	Kafka    config.KafkaConfig    `yaml:"kafka" json:"kafka"`
	Kline    KlineConfig           `yaml:"kline" json:"kline"`
	Log      LogConfig             `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type KlineConfig struct {
	Intervals []string `yaml:"intervals" json:"intervals"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-market",
			GRPCPort: 50053,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
