// Package config 配置管理
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config 服务配置
type Config struct {
	Service  ServiceConfig  `yaml:"service" json:"service"`
	Nacos    NacosConfig    `yaml:"nacos" json:"nacos"`
	Redis    RedisConfig    `yaml:"redis" json:"redis"`
	Kafka    KafkaConfig    `yaml:"kafka" json:"kafka"`
	Risk     RiskConfig     `yaml:"risk" json:"risk"`
	Markets  []MarketConfig `yaml:"markets" json:"markets"`
	Snapshot SnapshotConfig `yaml:"snapshot" json:"snapshot"`
	HA       HAConfig       `yaml:"ha" json:"ha"`
	Log      LogConfig      `yaml:"log" json:"log"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	NodeID   string `yaml:"node_id" json:"node_id"`
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

// RedisConfig Redis 配置
type RedisConfig struct {
	Addresses  []string `yaml:"addresses" json:"addresses"`
	Password   string   `yaml:"password" json:"password"`
	DB         int      `yaml:"db" json:"db"`
	PoolSize   int      `yaml:"pool_size" json:"pool_size"`
	TLSEnabled bool     `yaml:"tls_enabled" json:"tls_enabled"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers  []string            `yaml:"brokers" json:"brokers"`
	Consumer KafkaConsumerConfig `yaml:"consumer" json:"consumer"`
	Producer KafkaProducerConfig `yaml:"producer" json:"producer"`
}

// KafkaConsumerConfig Kafka 消费者配置
type KafkaConsumerConfig struct {
	GroupID   string   `yaml:"group_id" json:"group_id"`
	Topics    []string `yaml:"topics" json:"topics"`
	BatchSize int      `yaml:"batch_size" json:"batch_size"`
	LingerMs  int      `yaml:"linger_ms" json:"linger_ms"`
}

// KafkaProducerConfig Kafka 生产者配置
type KafkaProducerConfig struct {
	Compression  string `yaml:"compression" json:"compression"`
	BatchSize    int    `yaml:"batch_size" json:"batch_size"`
	BatchTimeout int    `yaml:"batch_timeout_ms" json:"batch_timeout_ms"`
	RequiredAcks int    `yaml:"required_acks" json:"required_acks"`
}

// MarketConfig 市场配置
type MarketConfig struct {
	Symbol        string `yaml:"symbol" json:"symbol"`
	BaseToken     string `yaml:"base_token" json:"base_token"`
	QuoteToken    string `yaml:"quote_token" json:"quote_token"`
	PriceDecimals int    `yaml:"price_decimals" json:"price_decimals"`
	SizeDecimals  int    `yaml:"size_decimals" json:"size_decimals"`
	MinSize       string `yaml:"min_size" json:"min_size"`
	TickSize      string `yaml:"tick_size" json:"tick_size"`
	MakerFeeRate  string `yaml:"maker_fee_rate" json:"maker_fee_rate"`
	TakerFeeRate  string `yaml:"taker_fee_rate" json:"taker_fee_rate"`
	MaxSlippage   string `yaml:"max_slippage" json:"max_slippage"`
}

// SnapshotConfig 快照配置
type SnapshotConfig struct {
	Interval time.Duration `yaml:"interval" json:"interval"`
	MaxCount int           `yaml:"max_count" json:"max_count"`
	TwoPhase bool          `yaml:"two_phase" json:"two_phase"`
}

// HAConfig 高可用配置
type HAConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval"`
	FailoverTimeout   time.Duration `yaml:"failover_timeout" json:"failover_timeout"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// RiskConfig 风控服务配置
type RiskConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`       // 是否启用风控检查
	Addr    string `yaml:"addr" json:"addr"`             // eidos-risk gRPC 地址 (如 localhost:50055)
	Timeout int    `yaml:"timeout_ms" json:"timeout_ms"` // 超时时间 (毫秒)
}

// Load 加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// 环境变量替换
	data = []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 设置默认值
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	if c.Service.Name == "" {
		c.Service.Name = "eidos-matching"
	}
	if c.Service.GRPCPort == 0 {
		c.Service.GRPCPort = 50052
	}
	if c.Service.HTTPPort == 0 {
		c.Service.HTTPPort = 8080
	}
	if c.Service.NodeID == "" {
		hostname, _ := os.Hostname()
		c.Service.NodeID = fmt.Sprintf("%s-%s", c.Service.Name, hostname)
	}

	if c.Kafka.Consumer.BatchSize == 0 {
		c.Kafka.Consumer.BatchSize = 100
	}
	if c.Kafka.Consumer.LingerMs == 0 {
		c.Kafka.Consumer.LingerMs = 5
	}
	if c.Kafka.Producer.BatchSize == 0 {
		c.Kafka.Producer.BatchSize = 1000
	}
	if c.Kafka.Producer.Compression == "" {
		c.Kafka.Producer.Compression = "lz4"
	}
	if c.Kafka.Producer.RequiredAcks == 0 {
		c.Kafka.Producer.RequiredAcks = -1 // all
	}

	if c.Snapshot.Interval == 0 {
		c.Snapshot.Interval = 30 * time.Second
	}
	if c.Snapshot.MaxCount == 0 {
		c.Snapshot.MaxCount = 10
	}

	if c.HA.HeartbeatInterval == 0 {
		c.HA.HeartbeatInterval = 100 * time.Millisecond
	}
	if c.HA.FailoverTimeout == 0 {
		c.HA.FailoverTimeout = 500 * time.Millisecond
	}

	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}

	// Risk 默认配置
	if c.Risk.Timeout == 0 {
		c.Risk.Timeout = 100 // 默认 100ms 超时，风控检查需要快速响应
	}
}

// ToEngineMarketConfig 转换为引擎市场配置
func (m *MarketConfig) ToEngineMarketConfig() *EngineMarketConfig {
	minSize, _ := decimal.NewFromString(m.MinSize)
	tickSize, _ := decimal.NewFromString(m.TickSize)
	makerFeeRate, _ := decimal.NewFromString(m.MakerFeeRate)
	takerFeeRate, _ := decimal.NewFromString(m.TakerFeeRate)
	maxSlippage, _ := decimal.NewFromString(m.MaxSlippage)

	return &EngineMarketConfig{
		Symbol:        m.Symbol,
		BaseToken:     m.BaseToken,
		QuoteToken:    m.QuoteToken,
		PriceDecimals: m.PriceDecimals,
		SizeDecimals:  m.SizeDecimals,
		MinSize:       minSize,
		TickSize:      tickSize,
		MakerFeeRate:  makerFeeRate,
		TakerFeeRate:  takerFeeRate,
		MaxSlippage:   maxSlippage,
	}
}

// EngineMarketConfig 引擎使用的市场配置
type EngineMarketConfig struct {
	Symbol        string
	BaseToken     string
	QuoteToken    string
	PriceDecimals int
	SizeDecimals  int
	MinSize       decimal.Decimal
	TickSize      decimal.Decimal
	MakerFeeRate  decimal.Decimal
	TakerFeeRate  decimal.Decimal
	MaxSlippage   decimal.Decimal
}
