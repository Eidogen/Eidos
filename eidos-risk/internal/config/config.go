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
	Risk     RiskConfig            `yaml:"risk" json:"risk"`
	Log      LogConfig             `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type RiskConfig struct {
	Limits LimitsConfig `yaml:"limits" json:"limits"`
}

type LimitsConfig struct {
	DailyWithdraw       string `yaml:"daily_withdraw" json:"daily_withdraw"`
	SingleOrder         string `yaml:"single_order" json:"single_order"`
	PendingSettle       string `yaml:"pending_settle" json:"pending_settle"`
	SystemPendingSettle string `yaml:"system_pending_settle" json:"system_pending_settle"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-risk",
			GRPCPort: 50055,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
