package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Service         ServiceConfig                  `yaml:"service" json:"service"`
	Nacos           commonConfig.NacosConfig       `yaml:"nacos" json:"nacos"`
	Postgres        commonConfig.PostgresConfig    `yaml:"postgres" json:"postgres"`
	Redis           commonConfig.RedisConfig       `yaml:"redis" json:"redis"`
	Kafka           commonConfig.KafkaConfig       `yaml:"kafka" json:"kafka"`
	GRPCClients     commonConfig.GRPCClientsConfig `yaml:"grpc_clients" json:"grpc_clients"`
	HealthEndpoints HealthEndpointsConfig          `yaml:"health_endpoints" json:"health_endpoints"`
	Jobs            JobsConfig                     `yaml:"jobs" json:"jobs"`
	Scheduler       SchedulerConfig                `yaml:"scheduler" json:"scheduler"`
	Log             commonConfig.LogConfig         `yaml:"log" json:"log"`
}

// HealthEndpointConfig 单个服务健康检查端点配置
type HealthEndpointConfig struct {
	URL        string `yaml:"url" json:"url"`                 // HTTP URL (用于 HTTP 检查)
	GRPCAddr   string `yaml:"grpc_addr" json:"grpc_addr"`     // gRPC 地址 (用于 gRPC 检查)
	CheckType  string `yaml:"check_type" json:"check_type"`   // 检查类型: http 或 grpc
	TimeoutSec int    `yaml:"timeout_sec" json:"timeout_sec"` // 超时时间(秒)
}

// HealthEndpointsConfig 所有服务的健康检查端点配置
type HealthEndpointsConfig struct {
	Trading  HealthEndpointConfig `yaml:"trading" json:"trading"`
	Matching HealthEndpointConfig `yaml:"matching" json:"matching"`
	Market   HealthEndpointConfig `yaml:"market" json:"market"`
	Chain    HealthEndpointConfig `yaml:"chain" json:"chain"`
	Risk     HealthEndpointConfig `yaml:"risk" json:"risk"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type JobsConfig struct {
	Reconciliation    JobConfig            `yaml:"reconciliation" json:"reconciliation"`
	CleanupOrders     JobConfig            `yaml:"cleanup_orders" json:"cleanup_orders"`
	Archive           JobConfig            `yaml:"archive" json:"archive"`
	StatsAgg          JobConfig            `yaml:"stats_agg" json:"stats_agg"`
	KlineAgg          JobConfig            `yaml:"kline_agg" json:"kline_agg"`
	HealthMonitor     JobConfig            `yaml:"health_monitor" json:"health_monitor"`
	PartitionMgmt     JobConfig            `yaml:"partition_mgmt" json:"partition_mgmt"`
	DataCleanup       JobConfig            `yaml:"data_cleanup" json:"data_cleanup"`
	BalanceScan       BalanceScanJobConfig `yaml:"balance_scan" json:"balance_scan"`
	SettlementTrigger SettlementJobConfig  `yaml:"settlement_trigger" json:"settlement_trigger"`
}

type JobConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Cron          string `yaml:"cron" json:"cron"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

// BalanceScanJobConfig 余额扫描任务配置
type BalanceScanJobConfig struct {
	Enabled                bool    `yaml:"enabled" json:"enabled"`
	Cron                   string  `yaml:"cron" json:"cron"`
	BatchSize              int     `yaml:"batch_size" json:"batch_size"`
	CancelThresholdPercent float64 `yaml:"cancel_threshold_percent" json:"cancel_threshold_percent"`
	MaxConcurrentQueries   int     `yaml:"max_concurrent_queries" json:"max_concurrent_queries"`
	EnableNotification     bool    `yaml:"enable_notification" json:"enable_notification"`
}

// SettlementJobConfig 结算触发任务配置
type SettlementJobConfig struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	Cron              string `yaml:"cron" json:"cron"`
	BatchSize         int    `yaml:"batch_size" json:"batch_size"`
	MinBatchSize      int    `yaml:"min_batch_size" json:"min_batch_size"`
	MaxWaitTimeMs     int    `yaml:"max_wait_time_ms" json:"max_wait_time_ms"`
	RetryTimeoutMs    int    `yaml:"retry_timeout_ms" json:"retry_timeout_ms"`
	MaxRetries        int    `yaml:"max_retries" json:"max_retries"`
	ConcurrentBatches int    `yaml:"concurrent_batches" json:"concurrent_batches"`
}

type SchedulerConfig struct {
	MaxConcurrentJobs int `yaml:"max_concurrent_jobs" json:"max_concurrent_jobs"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}

	// 尝试从配置文件加载
	configPath := getConfigPath()
	data, err := os.ReadFile(configPath)
	if err == nil {
		// 先解析为临时结构以支持环境变量替换
		content := commonConfig.ExpandEnv(string(data))
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

	// Nacos 默认配置
	if cfg.Nacos.ServerAddr == "" {
		cfg.Nacos = commonConfig.DefaultNacosConfig()
	}

	// Postgres defaults
	if cfg.Postgres.Host == "" {
		cfg.Postgres.Host = "postgres"
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
		cfg.Postgres.MaxConnections = 20
	}
	if cfg.Postgres.MaxIdleConns == 0 {
		cfg.Postgres.MaxIdleConns = 10
	}
	if cfg.Postgres.ConnMaxLifetime == 0 {
		cfg.Postgres.ConnMaxLifetime = 3600
	}

	// Redis defaults
	if len(cfg.Redis.Addresses) == 0 {
		cfg.Redis.Addresses = []string{"redis:6379"}
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 20
	}

	// Scheduler defaults
	if cfg.Scheduler.MaxConcurrentJobs == 0 {
		cfg.Scheduler.MaxConcurrentJobs = 3
	}

	// gRPC clients defaults
	if cfg.GRPCClients.Trading.Addr == "" {
		cfg.GRPCClients.Trading.Addr = "eidos-trading:50051"
	}
	if cfg.GRPCClients.Matching.Addr == "" {
		cfg.GRPCClients.Matching.Addr = "eidos-matching:50052"
	}
	if cfg.GRPCClients.Market.Addr == "" {
		cfg.GRPCClients.Market.Addr = "eidos-market:50053"
	}
	if cfg.GRPCClients.Chain.Addr == "" {
		cfg.GRPCClients.Chain.Addr = "eidos-chain:50054"
	}
	if cfg.GRPCClients.Risk.Addr == "" {
		cfg.GRPCClients.Risk.Addr = "eidos-risk:50055"
	}

	// Health endpoints defaults - 全部使用 gRPC 健康检查
	if cfg.HealthEndpoints.Trading.CheckType == "" {
		cfg.HealthEndpoints.Trading.CheckType = "grpc"
	}
	if cfg.HealthEndpoints.Trading.GRPCAddr == "" {
		cfg.HealthEndpoints.Trading.GRPCAddr = "eidos-trading:50051"
	}
	if cfg.HealthEndpoints.Trading.TimeoutSec == 0 {
		cfg.HealthEndpoints.Trading.TimeoutSec = 5
	}

	if cfg.HealthEndpoints.Matching.CheckType == "" {
		cfg.HealthEndpoints.Matching.CheckType = "grpc"
	}
	if cfg.HealthEndpoints.Matching.GRPCAddr == "" {
		cfg.HealthEndpoints.Matching.GRPCAddr = "eidos-matching:50052"
	}
	if cfg.HealthEndpoints.Matching.TimeoutSec == 0 {
		cfg.HealthEndpoints.Matching.TimeoutSec = 5
	}

	if cfg.HealthEndpoints.Market.CheckType == "" {
		cfg.HealthEndpoints.Market.CheckType = "grpc"
	}
	if cfg.HealthEndpoints.Market.GRPCAddr == "" {
		cfg.HealthEndpoints.Market.GRPCAddr = "eidos-market:50053"
	}
	if cfg.HealthEndpoints.Market.TimeoutSec == 0 {
		cfg.HealthEndpoints.Market.TimeoutSec = 5
	}

	if cfg.HealthEndpoints.Chain.CheckType == "" {
		cfg.HealthEndpoints.Chain.CheckType = "grpc"
	}
	if cfg.HealthEndpoints.Chain.GRPCAddr == "" {
		cfg.HealthEndpoints.Chain.GRPCAddr = "eidos-chain:50054"
	}
	if cfg.HealthEndpoints.Chain.TimeoutSec == 0 {
		cfg.HealthEndpoints.Chain.TimeoutSec = 5
	}

	if cfg.HealthEndpoints.Risk.CheckType == "" {
		cfg.HealthEndpoints.Risk.CheckType = "grpc"
	}
	if cfg.HealthEndpoints.Risk.GRPCAddr == "" {
		cfg.HealthEndpoints.Risk.GRPCAddr = "eidos-risk:50055"
	}
	if cfg.HealthEndpoints.Risk.TimeoutSec == 0 {
		cfg.HealthEndpoints.Risk.TimeoutSec = 5
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
	if v := os.Getenv("DB_DATABASE"); v != "" {
		cfg.Postgres.Database = v
	}

	// Redis
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addresses = []string{v}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}

	// Nacos
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		cfg.Nacos.Enabled = v == "true"
	}
	if v := os.Getenv("NACOS_SERVER_ADDR"); v != "" {
		cfg.Nacos.ServerAddr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		cfg.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_GROUP"); v != "" {
		cfg.Nacos.Group = v
	}

	// Log
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}
