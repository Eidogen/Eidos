package config

import (
	"os"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

// Config 配置
type Config struct {
	Service    ServiceConfig    `yaml:"service" json:"service"`
	Nacos      NacosConfig      `yaml:"nacos" json:"nacos"`
	Postgres   PostgresConfig   `yaml:"postgres" json:"postgres"`
	Redis      RedisConfig      `yaml:"redis" json:"redis"`
	Kafka      KafkaConfig      `yaml:"kafka" json:"kafka"`
	Blockchain BlockchainConfig `yaml:"blockchain" json:"blockchain"`
	Settlement SettlementConfig `yaml:"settlement" json:"settlement"`
	Log        LogConfig        `yaml:"log" json:"log"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

// NacosConfig Nacos 配置
type NacosConfig struct {
	ServerAddr string `yaml:"server_addr" json:"server_addr"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Group      string `yaml:"group" json:"group"`
}

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	Host            string `yaml:"host" json:"host"`
	Port            int    `yaml:"port" json:"port"`
	Database        string `yaml:"database" json:"database"`
	User            string `yaml:"user" json:"user"`
	Password        string `yaml:"password" json:"password"`
	MaxConnections  int    `yaml:"max_connections" json:"max_connections"`
	MaxIdleConns    int    `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addresses []string `yaml:"addresses" json:"addresses"`
	Password  string   `yaml:"password" json:"password"`
	DB        int      `yaml:"db" json:"db"`
	PoolSize  int      `yaml:"pool_size" json:"pool_size"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers  []string `yaml:"brokers" json:"brokers"`
	GroupID  string   `yaml:"group_id" json:"group_id"`
	ClientID string   `yaml:"client_id" json:"client_id"`
}

// BlockchainConfig 区块链配置
type BlockchainConfig struct {
	RPCURL          string              `yaml:"rpc_url" json:"rpc_url"`
	BackupRPCURLs   []string            `yaml:"backup_rpc_urls" json:"backup_rpc_urls"`
	ChainID         int64               `yaml:"chain_id" json:"chain_id"`
	ContractAddress string              `yaml:"contract_address" json:"contract_address"`
	PrivateKey      string              `yaml:"private_key" json:"private_key"`
	Confirmations   ConfirmationsConfig `yaml:"confirmations" json:"confirmations"`
}

// ConfirmationsConfig 确认数配置
type ConfirmationsConfig struct {
	Deposit    int `yaml:"deposit" json:"deposit"`
	Settlement int `yaml:"settlement" json:"settlement"`
}

// SettlementConfig 结算配置
type SettlementConfig struct {
	BatchSize     int `yaml:"batch_size" json:"batch_size"`
	BatchInterval int `yaml:"batch_interval" json:"batch_interval"`
	MaxRetries    int `yaml:"max_retries" json:"max_retries"`
	RetryBackoff  int `yaml:"retry_backoff" json:"retry_backoff"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// Load 加载配置
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// 环境变量替换
	content := string(data)
	content = config.ExpandEnv(content)

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaults(&cfg)

	return &cfg, nil
}

// setDefaults 设置默认值
func setDefaults(cfg *Config) {
	if cfg.Service.Name == "" {
		cfg.Service.Name = "eidos-chain"
	}
	if cfg.Service.GRPCPort == 0 {
		cfg.Service.GRPCPort = 50054
	}
	if cfg.Service.Env == "" {
		cfg.Service.Env = "dev"
	}

	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
	}
	if cfg.Postgres.MaxConnections == 0 {
		cfg.Postgres.MaxConnections = 50
	}
	if cfg.Postgres.MaxIdleConns == 0 {
		cfg.Postgres.MaxIdleConns = 10
	}
	if cfg.Postgres.ConnMaxLifetime == 0 {
		cfg.Postgres.ConnMaxLifetime = 3600
	}

	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 50
	}

	if cfg.Blockchain.ChainID == 0 {
		cfg.Blockchain.ChainID = 31337 // 本地开发
	}

	if cfg.Settlement.BatchSize == 0 {
		cfg.Settlement.BatchSize = 100
	}
	if cfg.Settlement.BatchInterval == 0 {
		cfg.Settlement.BatchInterval = 10
	}
	if cfg.Settlement.MaxRetries == 0 {
		cfg.Settlement.MaxRetries = 3
	}
	if cfg.Settlement.RetryBackoff == 0 {
		cfg.Settlement.RetryBackoff = 30
	}

	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
}
