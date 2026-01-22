// Package config 配置管理
package config

import (
	"fmt"
	"os"
	"time"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config 服务配置
type Config struct {
	Service     ServiceConfig                  `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig       `yaml:"nacos" json:"nacos"`
	Redis       commonConfig.RedisConfig       `yaml:"redis" json:"redis"`
	Kafka       commonConfig.KafkaConfig       `yaml:"kafka" json:"kafka"`
	Risk        commonConfig.ClientConfig      `yaml:"risk" json:"risk"`
	Markets     []MarketConfig                 `yaml:"markets" json:"markets"`
	Snapshot    SnapshotConfig                 `yaml:"snapshot" json:"snapshot"`
	HA          HAConfig                       `yaml:"ha" json:"ha"`
	IndexPrice  IndexPriceConfig               `yaml:"index_price" json:"index_price"`
	Log         commonConfig.LogConfig         `yaml:"log" json:"log"`
	Tracing     commonConfig.TracingConfig     `yaml:"tracing" json:"tracing"`
	GRPCClients commonConfig.GRPCClientsConfig `yaml:"grpc_clients" json:"grpc_clients"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	NodeID   string `yaml:"node_id" json:"node_id"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
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

// IndexPriceConfig 指数价格配置
type IndexPriceConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	UpdateInterval     time.Duration `yaml:"update_interval" json:"update_interval"`
	StaleThreshold     time.Duration `yaml:"stale_threshold" json:"stale_threshold"`
	AggregationMethod  string        `yaml:"aggregation_method" json:"aggregation_method"` // median, weighted_avg, best_source
	MinValidSources    int           `yaml:"min_valid_sources" json:"min_valid_sources"`
	DeviationThreshold string        `yaml:"deviation_threshold" json:"deviation_threshold"`
}

// Load 加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// 环境变量替换
	data = []byte(commonConfig.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 设置默认值
	cfg.setDefaults()

	// 环境变量覆盖
	cfg.applyEnvOverrides()

	return &cfg, nil
}

// applyEnvOverrides 应用环境变量覆盖
func (c *Config) applyEnvOverrides() {
	// Nacos
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		c.Nacos.Enabled = v == "true"
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

	// 数据库
	if v := os.Getenv("DB_HOST"); v != "" {
		// Matching doesn't use Postgres directly, but for consistency if added later
	}

	// Kafka
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		c.Kafka.Brokers = []string{v}
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

	// Nacos 默认配置
	if c.Nacos.ServerAddr == "" {
		defaultNacos := commonConfig.DefaultNacosConfig()
		c.Nacos = defaultNacos
	}

	if c.Kafka.Consumer.MaxPollRecords == 0 {
		c.Kafka.Consumer.MaxPollRecords = 100
	}
	if c.Kafka.Producer.LingerMs == 0 {
		c.Kafka.Producer.LingerMs = 5
	}
	if c.Kafka.Producer.BatchSize == 0 {
		c.Kafka.Producer.BatchSize = 1000
	}
	if c.Kafka.Producer.Compression == "" {
		c.Kafka.Producer.Compression = "lz4"
	}
	if c.Kafka.Producer.RequiredAcks == 0 {
		// all
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

	// Tracing 默认配置
	if c.Tracing.Endpoint == "" {
		c.Tracing = commonConfig.DefaultTracingConfig()
	}

	// Risk 默认配置
	if c.Risk.TimeoutMs == 0 {
		c.Risk.TimeoutMs = 100 // 默认 100ms 超时，风控检查需要快速响应
	}

	// IndexPrice 默认配置
	if c.IndexPrice.UpdateInterval == 0 {
		c.IndexPrice.UpdateInterval = time.Second
	}
	if c.IndexPrice.StaleThreshold == 0 {
		c.IndexPrice.StaleThreshold = 30 * time.Second
	}
	if c.IndexPrice.AggregationMethod == "" {
		c.IndexPrice.AggregationMethod = "median"
	}
	if c.IndexPrice.MinValidSources == 0 {
		c.IndexPrice.MinValidSources = 1
	}
	if c.IndexPrice.DeviationThreshold == "" {
		c.IndexPrice.DeviationThreshold = "0.1" // 10%
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
