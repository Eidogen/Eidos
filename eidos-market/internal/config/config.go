package config

import (
	"os"
	"time"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Service     ServiceConfig                  `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig       `yaml:"nacos" json:"nacos"`
	Matching    commonConfig.ClientConfig      `yaml:"matching" json:"matching"`
	Postgres    commonConfig.PostgresConfig    `yaml:"postgres" json:"postgres"`
	Redis       commonConfig.RedisConfig       `yaml:"redis" json:"redis"`
	Kafka       commonConfig.KafkaConfig       `yaml:"kafka" json:"kafka"`
	Kline       KlineConfig                    `yaml:"kline" json:"kline"`
	Aggregator  AggregatorConfig               `yaml:"aggregator" json:"aggregator"`
	Log         commonConfig.LogConfig         `yaml:"log" json:"log"`
	Tracing     commonConfig.TracingConfig     `yaml:"tracing" json:"tracing"`
	GRPCClients commonConfig.GRPCClientsConfig `yaml:"grpc_clients" json:"grpc_clients"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

// KlineConfig K 线配置
type KlineConfig struct {
	Intervals []string `yaml:"intervals" json:"intervals"`
}

// AggregatorConfig 聚合器配置
type AggregatorConfig struct {
	KlineFlushInterval    int `yaml:"kline_flush_interval" json:"kline_flush_interval"`       // K 线刷盘间隔（秒）
	TickerPublishInterval int `yaml:"ticker_publish_interval" json:"ticker_publish_interval"` // Ticker 发布间隔（毫秒）
	DepthPublishInterval  int `yaml:"depth_publish_interval" json:"depth_publish_interval"`   // 深度发布间隔（毫秒）
	DepthMaxLevels        int `yaml:"depth_max_levels" json:"depth_max_levels"`               // 深度最大档位数
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-market",
			GRPCPort: 50053,
			HTTPPort: 8080,
			Env:      "dev",
		},
		Nacos: commonConfig.DefaultNacosConfig(),
		Matching: commonConfig.ClientConfig{
			Addr: "eidos-matching:50052",
		},
		Postgres: commonConfig.PostgresConfig{
			Host:            "postgres",
			Port:            5432,
			Database:        "eidos_market",
			User:            "eidos",
			Password:        "eidos123",
			MaxConnections:  50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 3600,
		},
		Redis: commonConfig.RedisConfig{
			Addresses: []string{"redis:6379"},
			Password:  "",
			DB:        0,
			PoolSize:  100,
		},
		Kafka: commonConfig.KafkaConfig{
			Brokers: []string{"kafka:9092"},
			GroupID: "eidos-market",
		},
		Kline: KlineConfig{
			Intervals: []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w"},
		},
		Aggregator: AggregatorConfig{
			KlineFlushInterval:    10,
			TickerPublishInterval: 1000,
			DepthPublishInterval:  100,
			DepthMaxLevels:        100,
		},
		Log: commonConfig.LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load 加载配置
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// 如果配置文件存在，从文件加载
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		// 先展开环境变量
		expandedData := commonConfig.ExpandEnv(string(data))
		if err := yaml.Unmarshal([]byte(expandedData), cfg); err != nil {
			return nil, err
		}
	}

	// 环境变量覆盖
	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides 应用环境变量覆盖
func (c *Config) applyEnvOverrides() {
	// Service
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		c.Service.Name = v
	}
	if v := commonConfig.GetEnvInt("GRPC_PORT", 0); v > 0 {
		c.Service.GRPCPort = v
	}
	if v := commonConfig.GetEnvInt("HTTP_PORT", 0); v > 0 {
		c.Service.HTTPPort = v
	}
	if v := os.Getenv("ENV"); v != "" {
		c.Service.Env = v
	}

	// Nacos
	if v := commonConfig.GetEnvBool("NACOS_ENABLED", false); v {
		c.Nacos.Enabled = v
	}
	if v := os.Getenv("NACOS_SERVER_ADDR"); v != "" {
		c.Nacos.ServerAddr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		c.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_GROUP"); v != "" {
		c.Nacos.Group = v
	}
	if v := os.Getenv("NACOS_USERNAME"); v != "" {
		c.Nacos.Username = v
	}
	if v := os.Getenv("NACOS_PASSWORD"); v != "" {
		c.Nacos.Password = v
	}

	// Matching
	if v := commonConfig.GetEnvBool("MATCHING_ENABLED", false); v {
		c.Matching.Enabled = v
	}
	if v := os.Getenv("MATCHING_ADDR"); v != "" {
		c.Matching.Addr = v
	}
	if v := commonConfig.GetEnvInt("MATCHING_TIMEOUT", 0); v > 0 {
		c.Matching.TimeoutMs = v
	}

	// Postgres
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		c.Postgres.Host = v
	}
	if v := commonConfig.GetEnvInt("POSTGRES_PORT", 0); v > 0 {
		c.Postgres.Port = v
	}
	if v := os.Getenv("POSTGRES_DATABASE"); v != "" {
		c.Postgres.Database = v
	}
	if v := os.Getenv("POSTGRES_USER"); v != "" {
		c.Postgres.User = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		c.Postgres.Password = v
	}

	// Redis
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.Redis.Addresses = []string{v}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.Redis.Password = v
	}

	// Kafka
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		c.Kafka.Brokers = commonConfig.GetEnvSlice("KAFKA_BROKERS", c.Kafka.Brokers)
	}

	// Log
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}

	// 链路追踪配置 (支持 OTEL 标准环境变量)
	if v := os.Getenv("TRACING_ENABLED"); v != "" {
		c.Tracing.Enabled = v == "true"
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		c.Tracing.Endpoint = endpoint
	} else if endpoint := os.Getenv("TRACING_ENDPOINT"); endpoint != "" {
		c.Tracing.Endpoint = endpoint
	}
}

// GetKlineFlushInterval 获取 K 线刷盘间隔
func (c *Config) GetKlineFlushInterval() time.Duration {
	return time.Duration(c.Aggregator.KlineFlushInterval) * time.Second
}

// GetTickerPublishInterval 获取 Ticker 发布间隔
func (c *Config) GetTickerPublishInterval() time.Duration {
	return time.Duration(c.Aggregator.TickerPublishInterval) * time.Millisecond
}

// GetDepthPublishInterval 获取深度发布间隔
func (c *Config) GetDepthPublishInterval() time.Duration {
	return time.Duration(c.Aggregator.DepthPublishInterval) * time.Millisecond
}

// IsDev 判断是否为开发环境
func (c *Config) IsDev() bool {
	return c.Service.Env == "dev"
}

// IsProd 判断是否为生产环境
func (c *Config) IsProd() bool {
	return c.Service.Env == "prod"
}

// GetMatchingConnectTimeout 获取 Matching 连接超时
func (c *Config) GetMatchingConnectTimeout() time.Duration {
	return time.Duration(c.Matching.TimeoutMs) * time.Millisecond
}

// GetMatchingRequestTimeout 获取 Matching 请求超时
func (c *Config) GetMatchingRequestTimeout() time.Duration {
	return time.Duration(c.Matching.TimeoutMs) * time.Millisecond
}
