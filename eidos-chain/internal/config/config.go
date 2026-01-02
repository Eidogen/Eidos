package config

import (
	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Service    ServiceConfig         `yaml:"service" json:"service"`
	Nacos      config.NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres   config.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis      config.RedisConfig    `yaml:"redis" json:"redis"`
	Kafka      config.KafkaConfig    `yaml:"kafka" json:"kafka"`
	Blockchain BlockchainConfig      `yaml:"blockchain" json:"blockchain"`
	Settlement SettlementConfig      `yaml:"settlement" json:"settlement"`
	Log        LogConfig             `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type BlockchainConfig struct {
	RPCURL          string              `yaml:"rpc_url" json:"rpc_url"`
	ChainID         int64               `yaml:"chain_id" json:"chain_id"`
	ContractAddress string              `yaml:"contract_address" json:"contract_address"`
	PrivateKey      string              `yaml:"private_key" json:"private_key"`
	Confirmations   ConfirmationsConfig `yaml:"confirmations" json:"confirmations"`
}

type ConfirmationsConfig struct {
	Deposit    int `yaml:"deposit" json:"deposit"`
	Settlement int `yaml:"settlement" json:"settlement"`
}

type SettlementConfig struct {
	BatchSize     int `yaml:"batch_size" json:"batch_size"`
	BatchInterval int `yaml:"batch_interval" json:"batch_interval"`
	MaxRetries    int `yaml:"max_retries" json:"max_retries"`
	RetryBackoff  int `yaml:"retry_backoff" json:"retry_backoff"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-chain",
			GRPCPort: 50054,
			HTTPPort: 8080,
			Env:      "dev",
		},
	}, nil
}
