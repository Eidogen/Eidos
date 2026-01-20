package config

import (
	"os"
	"strconv"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config 服务配置
type Config struct {
	Service     ServiceConfig     `yaml:"service" json:"service"`
	Nacos       NacosConfig       `yaml:"nacos" json:"nacos"`
	Database    DatabaseConfig    `yaml:"database" json:"database"`
	Redis       RedisConfig       `yaml:"redis" json:"redis"`
	Kafka       KafkaConfig       `yaml:"kafka" json:"kafka"`
	Outbox      OutboxConfig      `yaml:"outbox" json:"outbox"`
	Node        NodeConfig        `yaml:"node" json:"node"`
	EIP712      EIP712Config      `yaml:"eip712" json:"eip712"`
	Log         LogConfig         `yaml:"log" json:"log"`
	Markets     []MarketConfig    `yaml:"markets" json:"markets"`
	Tokens      []TokenConfig     `yaml:"tokens" json:"tokens"`
	RiskControl RiskControlConfig `yaml:"risk_control" json:"risk_control"`
	Worker      WorkerConfig      `yaml:"worker" json:"worker"`
	Matching    MatchingConfig    `yaml:"matching" json:"matching"`
	Risk        RiskConfig        `yaml:"risk" json:"risk"`
}

// MatchingConfig eidos-matching 服务配置
type MatchingConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"` // gRPC 地址 (host:port)
}

// RiskConfig eidos-risk 服务配置
type RiskConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Addr    string `yaml:"addr" json:"addr"` // gRPC 地址 (host:port)
}

// RiskControlConfig 风控配置
type RiskControlConfig struct {
	// UserPendingLimit 用户待结算限额 (USDT)
	// 超过此限额用户无法下新单
	UserPendingLimit decimal.Decimal `yaml:"user_pending_limit" json:"user_pending_limit"`

	// GlobalPendingLimit 系统全局待结算限额 (USDT)
	// 超过此限额系统停止接单
	GlobalPendingLimit decimal.Decimal `yaml:"global_pending_limit" json:"global_pending_limit"`

	// MaxOpenOrdersPerUser 用户最大活跃订单数
	MaxOpenOrdersPerUser int `yaml:"max_open_orders_per_user" json:"max_open_orders_per_user"`
}

// WorkerConfig Worker 配置
type WorkerConfig struct {
	// OrderExpiry 订单过期 Worker 配置
	OrderExpiry OrderExpiryConfig `yaml:"order_expiry" json:"order_expiry"`

	// Reconciliation 对账 Worker 配置
	Reconciliation ReconciliationConfig `yaml:"reconciliation" json:"reconciliation"`

	// AsyncDBTimeout 异步 DB 写入超时 (秒)
	AsyncDBTimeoutSec int `yaml:"async_db_timeout_sec" json:"async_db_timeout_sec"`
}

// OrderExpiryConfig 订单过期配置
type OrderExpiryConfig struct {
	Enabled          bool `yaml:"enabled" json:"enabled"`
	CheckIntervalSec int  `yaml:"check_interval_sec" json:"check_interval_sec"`
	BatchSize        int  `yaml:"batch_size" json:"batch_size"`
}

// ReconciliationConfig 对账配置
type ReconciliationConfig struct {
	Enabled          bool `yaml:"enabled" json:"enabled"`
	CheckIntervalSec int  `yaml:"check_interval_sec" json:"check_interval_sec"`
	BatchSize        int  `yaml:"batch_size" json:"batch_size"`
}

// NacosConfig Nacos 配置
type NacosConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	ServerAddr string `yaml:"server_addr" json:"server_addr"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Group      string `yaml:"group" json:"group"`
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	LogDir     string `yaml:"log_dir" json:"log_dir"`
	CacheDir   string `yaml:"cache_dir" json:"cache_dir"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	GRPCPort int    `yaml:"grpc_port" json:"grpc_port"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	User                   string `yaml:"user" json:"user"`
	Password               string `yaml:"password" json:"password"`
	Database               string `yaml:"database" json:"database"`
	MaxIdleConns           int    `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxOpenConns           int    `yaml:"max_open_conns" json:"max_open_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes" json:"conn_max_lifetime_minutes"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Password string `yaml:"password" json:"password"`
	DB       int    `yaml:"db" json:"db"`
	PoolSize int    `yaml:"pool_size" json:"pool_size"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Enabled  bool           `yaml:"enabled" json:"enabled"`
	Brokers  []string       `yaml:"brokers" json:"brokers"`
	GroupID  string         `yaml:"group_id" json:"group_id"`
	Producer ProducerConfig `yaml:"producer" json:"producer"`
	Consumer ConsumerConfig `yaml:"consumer" json:"consumer"`
}

// ProducerConfig Kafka 生产者配置
type ProducerConfig struct {
	RequiredAcks  int `yaml:"required_acks" json:"required_acks"`   // 0=NoResponse, 1=WaitForLocal, -1=WaitForAll
	MaxRetry      int `yaml:"max_retry" json:"max_retry"`           // 最大重试次数
	FlushMessages int `yaml:"flush_messages" json:"flush_messages"` // 批量发送消息数
	FlushBytes    int `yaml:"flush_bytes" json:"flush_bytes"`       // 批量发送字节数
	FlushFreqMs   int `yaml:"flush_freq_ms" json:"flush_freq_ms"`   // 批量发送间隔 (毫秒)
}

// ConsumerConfig Kafka 消费者配置
type ConsumerConfig struct {
	InitialOffset string `yaml:"initial_offset" json:"initial_offset"` // newest, oldest
}

// OutboxConfig Outbox 配置
type OutboxConfig struct {
	RelayIntervalMs   int `yaml:"relay_interval_ms" json:"relay_interval_ms"`     // 轮询间隔 (毫秒)
	BatchSize         int `yaml:"batch_size" json:"batch_size"`                   // 每批处理数量
	MaxRetries        int `yaml:"max_retries" json:"max_retries"`                 // 最大重试次数
	CleanupIntervalMs int `yaml:"cleanup_interval_ms" json:"cleanup_interval_ms"` // 清理间隔 (毫秒)
	RetentionMs       int `yaml:"retention_ms" json:"retention_ms"`               // 已发送消息保留时间 (毫秒)
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID int64 `yaml:"id" json:"id"` // 节点 ID (用于 Snowflake)
}

// EIP712Config EIP-712 配置
type EIP712Config struct {
	Domain EIP712Domain `yaml:"domain" json:"domain"`
}

// EIP712Domain EIP-712 域
type EIP712Domain struct {
	Name              string `yaml:"name" json:"name"`
	Version           string `yaml:"version" json:"version"`
	ChainID           int64  `yaml:"chain_id" json:"chain_id"`
	VerifyingContract string `yaml:"verifying_contract" json:"verifying_contract"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

// MarketConfig 交易对配置
type MarketConfig struct {
	Market          string          `yaml:"market" json:"market"`
	BaseToken       string          `yaml:"base_token" json:"base_token"`
	QuoteToken      string          `yaml:"quote_token" json:"quote_token"`
	MinAmount       decimal.Decimal `yaml:"min_amount" json:"min_amount"`
	MaxAmount       decimal.Decimal `yaml:"max_amount" json:"max_amount"`
	MinPrice        decimal.Decimal `yaml:"min_price" json:"min_price"`
	MaxPrice        decimal.Decimal `yaml:"max_price" json:"max_price"`
	PricePrecision  int32           `yaml:"price_precision" json:"price_precision"`
	AmountPrecision int32           `yaml:"amount_precision" json:"amount_precision"`
	MakerFeeRate    decimal.Decimal `yaml:"maker_fee_rate" json:"maker_fee_rate"`
	TakerFeeRate    decimal.Decimal `yaml:"taker_fee_rate" json:"taker_fee_rate"`
	Status          int8            `yaml:"status" json:"status"`
}

// TokenConfig 代币配置
type TokenConfig struct {
	Symbol   string `yaml:"symbol" json:"symbol"`
	Name     string `yaml:"name" json:"name"`
	Decimals int32  `yaml:"decimals" json:"decimals"`
	Address  string `yaml:"address" json:"address"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := defaultConfig()

	// 尝试从配置文件加载
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// 从环境变量覆盖
	loadFromEnv(cfg)

	return cfg, nil
}

// defaultConfig 返回默认配置
func defaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-trading",
			GRPCPort: 50051,
			HTTPPort: 8080,
			Env:      "dev",
		},
		Nacos: NacosConfig{
			Enabled:    false, // 默认不启用，开发时可以不需要 Nacos
			ServerAddr: "127.0.0.1:8848",
			Namespace:  "public",
			Group:      "EIDOS_GROUP",
			LogDir:     "/tmp/nacos/log",
			CacheDir:   "/tmp/nacos/cache",
		},
		Database: DatabaseConfig{
			Host:                   "localhost",
			Port:                   5432,
			User:                   "postgres",
			Password:               "postgres",
			Database:               "eidos_trading",
			MaxIdleConns:           10,
			MaxOpenConns:           100,
			ConnMaxLifetimeMinutes: 30,
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
			PoolSize: 100,
		},
		Kafka: KafkaConfig{
			Enabled: false, // 默认不启用 Kafka
			Brokers: []string{"localhost:9092"},
			GroupID: "eidos-trading",
			Producer: ProducerConfig{
				RequiredAcks:  -1, // WaitForAll
				MaxRetry:      3,
				FlushMessages: 100,
				FlushBytes:    1024 * 1024, // 1MB
				FlushFreqMs:   10,
			},
			Consumer: ConsumerConfig{
				InitialOffset: "newest",
			},
		},
		Outbox: OutboxConfig{
			RelayIntervalMs:   100,
			BatchSize:         100,
			MaxRetries:        5,
			CleanupIntervalMs: 3600000,  // 1 hour
			RetentionMs:       86400000, // 24 hours
		},
		Node: NodeConfig{
			ID: 1,
		},
		EIP712: EIP712Config{
			Domain: EIP712Domain{
				Name:              "EidosExchange",
				Version:           "1",
				ChainID:           31337,
				VerifyingContract: "0x0000000000000000000000000000000000000000",
			},
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		RiskControl: RiskControlConfig{
			UserPendingLimit:     decimal.NewFromFloat(100000),   // 单用户待结算限额: 10万 USDT
			GlobalPendingLimit:   decimal.NewFromFloat(10000000), // 全局待结算限额: 1000万 USDT
			MaxOpenOrdersPerUser: 100,                            // 单用户最多100个活跃订单
		},
		Worker: WorkerConfig{
			OrderExpiry: OrderExpiryConfig{
				Enabled:          true,
				CheckIntervalSec: 30,
				BatchSize:        100,
			},
			Reconciliation: ReconciliationConfig{
				Enabled:          true,
				CheckIntervalSec: 300, // 5分钟
				BatchSize:        100,
			},
			AsyncDBTimeoutSec: 30, // 异步 DB 写入超时 30 秒
		},
		Matching: MatchingConfig{
			Enabled: false,             // 默认不启用，开发时可选
			Addr:    "localhost:50052", // 默认撮合服务地址
		},
		Risk: RiskConfig{
			Enabled: false,             // 默认不启用，开发时可选
			Addr:    "localhost:50055", // 默认风控服务地址
		},
		Markets: []MarketConfig{
			{
				Market:          "ETH-USDT",
				BaseToken:       "ETH",
				QuoteToken:      "USDT",
				MinAmount:       decimal.NewFromFloat(0.001),
				MaxAmount:       decimal.NewFromFloat(1000),
				MinPrice:        decimal.NewFromFloat(1),
				MaxPrice:        decimal.NewFromFloat(100000),
				PricePrecision:  2,
				AmountPrecision: 6,
				MakerFeeRate:    decimal.NewFromFloat(0.001),
				TakerFeeRate:    decimal.NewFromFloat(0.002),
				Status:          1,
			},
		},
		Tokens: []TokenConfig{
			{Symbol: "ETH", Name: "Ethereum", Decimals: 18, Address: "0x0000000000000000000000000000000000000000"},
			{Symbol: "USDT", Name: "Tether USD", Decimals: 6, Address: "0x0000000000000000000000000000000000000001"},
		},
	}
}

// loadFromEnv 从环境变量加载配置
func loadFromEnv(cfg *Config) {
	// Nacos 配置
	if enabled := os.Getenv("NACOS_ENABLED"); enabled == "true" {
		cfg.Nacos.Enabled = true
	}
	if addr := os.Getenv("NACOS_SERVER_ADDR"); addr != "" {
		cfg.Nacos.ServerAddr = addr
	}
	if namespace := os.Getenv("NACOS_NAMESPACE"); namespace != "" {
		cfg.Nacos.Namespace = namespace
	}
	if group := os.Getenv("NACOS_GROUP"); group != "" {
		cfg.Nacos.Group = group
	}

	// 数据库配置
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.Database.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		cfg.Database.Password = password
	}
	if database := os.Getenv("DB_DATABASE"); database != "" {
		cfg.Database.Database = database
	}

	// Redis 配置
	if host := os.Getenv("REDIS_HOST"); host != "" {
		cfg.Redis.Host = host
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		cfg.Redis.Password = password
	}

	// Kafka 配置
	if enabled := os.Getenv("KAFKA_ENABLED"); enabled == "true" {
		cfg.Kafka.Enabled = true
	}
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Kafka.Brokers = []string{brokers}
	}
	if groupID := os.Getenv("KAFKA_GROUP_ID"); groupID != "" {
		cfg.Kafka.GroupID = groupID
	}

	// 节点配置 (用于 Snowflake ID 生成，集群部署时每个实例需要不同的 NODE_ID)
	if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
		if id, err := strconv.ParseInt(nodeID, 10, 64); err == nil {
			cfg.Node.ID = id
		}
	}
}
