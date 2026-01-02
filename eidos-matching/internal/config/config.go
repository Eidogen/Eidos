package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service  ServiceConfig      `yaml:"service" json:"service"`
	Nacos    config.NacosConfig `yaml:"nacos" json:"nacos"`
	Redis    config.RedisConfig `yaml:"redis" json:"redis"`
	Kafka    config.KafkaConfig `yaml:"kafka" json:"kafka"`
	Matching MatchingConfig     `yaml:"matching" json:"matching"`
	Log      LogConfig          `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type MatchingConfig struct {
	Markets          []string `yaml:"markets" json:"markets"`
	SnapshotInterval int      `yaml:"snapshot_interval" json:"snapshot_interval"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-matching",
			GRPCPort: 50052,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
