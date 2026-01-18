package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Service     ServiceConfig      `yaml:"service" json:"service"`
	Nacos       NacosConfig        `yaml:"nacos" json:"nacos"`
	Postgres    PostgresConfig     `yaml:"postgres" json:"postgres"`
	Redis       RedisConfig        `yaml:"redis" json:"redis"`
	Kafka       KafkaConfig        `yaml:"kafka" json:"kafka"`
	GRPCClients GRPCClientsConfig  `yaml:"grpc_clients" json:"grpc_clients"`
	Jobs        JobsConfig         `yaml:"jobs" json:"jobs"`
	Scheduler   SchedulerConfig    `yaml:"scheduler" json:"scheduler"`
	Log         LogConfig          `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type NacosConfig struct {
	ServerAddr string `yaml:"server_addr" json:"server_addr"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Group      string `yaml:"group" json:"group"`
}

type PostgresConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	User                   string `yaml:"user" json:"user"`
	Password               string `yaml:"password" json:"password"`
	Database               string `yaml:"database" json:"database"`
	MaxConnections         int    `yaml:"max_connections" json:"max_connections"`
	MaxIdleConns           int    `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes" json:"conn_max_lifetime_minutes"`
}

type RedisConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`
	PoolSize int    `yaml:"pool_size" json:"pool_size"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers" json:"brokers"`
	GroupID string   `yaml:"group_id" json:"group_id"`
}

type GRPCClientsConfig struct {
	Trading  string `yaml:"trading" json:"trading"`
	Chain    string `yaml:"chain" json:"chain"`
	Market   string `yaml:"market" json:"market"`
	Matching string `yaml:"matching" json:"matching"`
}

type JobsConfig struct {
	Reconciliation JobConfig `yaml:"reconciliation" json:"reconciliation"`
	CleanupOrders  JobConfig `yaml:"cleanup_orders" json:"cleanup_orders"`
	Archive        JobConfig `yaml:"archive" json:"archive"`
	StatsAgg       JobConfig `yaml:"stats_agg" json:"stats_agg"`
	KlineAgg       JobConfig `yaml:"kline_agg" json:"kline_agg"`
	HealthMonitor  JobConfig `yaml:"health_monitor" json:"health_monitor"`
	PartitionMgmt  JobConfig `yaml:"partition_mgmt" json:"partition_mgmt"`
	DataCleanup    JobConfig `yaml:"data_cleanup" json:"data_cleanup"`
}

type JobConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Cron          string `yaml:"cron" json:"cron"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

type SchedulerConfig struct {
	MaxConcurrentJobs int `yaml:"max_concurrent_jobs" json:"max_concurrent_jobs"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}

	// 尝试从配置文件加载
	configPath := getConfigPath()
	data, err := os.ReadFile(configPath)
	if err == nil {
		// 先解析为临时结构以支持环境变量替换
		content := os.ExpandEnv(string(data))
		if err := yaml.Unmarshal([]byte(content), cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// 设置默认值
	applyDefaults(cfg)

	// 从环境变量覆盖
	applyEnvOverrides(cfg)

	return cfg, nil
}

// getConfigPath 获取配置文件路径
func getConfigPath() string {
	// 1. 环境变量
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}

	// 2. 当前目录
	if _, err := os.Stat("config/config.yaml"); err == nil {
		return "config/config.yaml"
	}

	// 3. 可执行文件目录
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), "config", "config.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return "config/config.yaml"
}

// applyDefaults 应用默认配置
func applyDefaults(cfg *Config) {
	if cfg.Service.Name == "" {
		cfg.Service.Name = "eidos-jobs"
	}
	if cfg.Service.GRPCPort == 0 {
		cfg.Service.GRPCPort = 50056
	}
	if cfg.Service.HTTPPort == 0 {
		cfg.Service.HTTPPort = 8080
	}
	if cfg.Service.Env == "" {
		cfg.Service.Env = "dev"
	}

	// Postgres defaults
	if cfg.Postgres.Host == "" {
		cfg.Postgres.Host = "localhost"
	}
	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
	}
	if cfg.Postgres.User == "" {
		cfg.Postgres.User = "eidos"
	}
	if cfg.Postgres.Database == "" {
		cfg.Postgres.Database = "eidos_jobs"
	}
	if cfg.Postgres.MaxConnections == 0 {
		cfg.Postgres.MaxConnections = 10
	}
	if cfg.Postgres.MaxIdleConns == 0 {
		cfg.Postgres.MaxIdleConns = 5
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes == 0 {
		cfg.Postgres.ConnMaxLifetimeMinutes = 30
	}

	// Redis defaults
	if cfg.Redis.Host == "" {
		cfg.Redis.Host = "localhost"
	}
	if cfg.Redis.Port == 0 {
		cfg.Redis.Port = 6379
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 20
	}

	// Scheduler defaults
	if cfg.Scheduler.MaxConcurrentJobs == 0 {
		cfg.Scheduler.MaxConcurrentJobs = 3
	}

	// Log defaults
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
}

// applyEnvOverrides 从环境变量覆盖配置
func applyEnvOverrides(cfg *Config) {
	// Service
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		cfg.Service.Name = v
	}
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Service.GRPCPort = port
		}
	}
	if v := os.Getenv("ENV"); v != "" {
		cfg.Service.Env = v
	}

	// Postgres
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := os.Getenv("POSTGRES_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Postgres.Port = port
		}
	}
	if v := os.Getenv("POSTGRES_USER"); v != "" {
		cfg.Postgres.User = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		cfg.Postgres.Password = v
	}
	if v := os.Getenv("POSTGRES_DATABASE"); v != "" {
		cfg.Postgres.Database = v
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

	// Log
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}
