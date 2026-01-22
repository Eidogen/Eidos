// Package config 提供风控服务配置管理
package config

import (
	"os"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config 风控服务配置
type Config struct {
	Service     ServiceConfig                  `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig       `yaml:"nacos" json:"nacos"`
	Postgres    commonConfig.PostgresConfig    `yaml:"postgres" json:"postgres"`
	Redis       commonConfig.RedisConfig       `yaml:"redis" json:"redis"`
	Kafka       commonConfig.KafkaConfig       `yaml:"kafka" json:"kafka"`
	Risk        RiskConfig                     `yaml:"risk" json:"risk"`
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

// RiskConfig 风控配置
type RiskConfig struct {
	Limits     LimitsConfig     `yaml:"limits" json:"limits"`
	RateLimits RateLimitsConfig `yaml:"rate_limits" json:"rate_limits"`
	Monitoring MonitoringConfig `yaml:"monitoring" json:"monitoring"`
}

// LimitsConfig 限额配置
type LimitsConfig struct {
	DailyWithdraw       string `yaml:"daily_withdraw" json:"daily_withdraw"`               // 单用户每日提现上限
	SingleOrder         string `yaml:"single_order" json:"single_order"`                   // 单笔订单上限
	PendingSettle       string `yaml:"pending_settle" json:"pending_settle"`               // 用户待结算余额上限
	SystemPendingSettle string `yaml:"system_pending_settle" json:"system_pending_settle"` // 系统待结算上限
	MaxOpenOrders       int    `yaml:"max_open_orders" json:"max_open_orders"`             // 单用户最大挂单数
	MaxDailyTrades      int    `yaml:"max_daily_trades" json:"max_daily_trades"`           // 单用户每日成交笔数上限
}

// RateLimitsConfig 频率限制配置
type RateLimitsConfig struct {
	OrdersPerSecond  int `yaml:"orders_per_second" json:"orders_per_second"`   // 每秒下单数限制
	OrdersPerMinute  int `yaml:"orders_per_minute" json:"orders_per_minute"`   // 每分钟下单数限制
	CancelsPerMinute int `yaml:"cancels_per_minute" json:"cancels_per_minute"` // 每分钟取消数限制
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	LargeTradeThreshold string `yaml:"large_trade_threshold" json:"large_trade_threshold"` // 大额交易阈值
	AlertIntervalSec    int    `yaml:"alert_interval_sec" json:"alert_interval_sec"`       // 告警间隔(秒)
}

// Load 加载配置
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// 环境变量替换
	content := string(data)
	content = commonConfig.ExpandEnv(content)

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, err
	}

	// 环境变量覆盖
	overrideFromEnv(&cfg)

	return &cfg, nil
}

// overrideFromEnv 从环境变量覆盖配置
func overrideFromEnv(cfg *Config) {
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
	if v := os.Getenv("NACOS_USERNAME"); v != "" {
		cfg.Nacos.Username = v
	}
	if v := os.Getenv("NACOS_PASSWORD"); v != "" {
		cfg.Nacos.Password = v
	}

	// 数据库
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := os.Getenv("DB_DATABASE"); v != "" {
		cfg.Postgres.Database = v
	}

	// 链路追踪配置 (支持 OTEL 标准环境变量)
	if v := os.Getenv("TRACING_ENABLED"); v != "" {
		cfg.Tracing.Enabled = v == "true"
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		cfg.Tracing.Endpoint = endpoint
	} else if endpoint := os.Getenv("TRACING_ENDPOINT"); endpoint != "" {
		cfg.Tracing.Endpoint = endpoint
	}
}

// setDefaults 设置默认值
func setDefaults(cfg *Config) {
	if cfg.Service.Name == "" {
		cfg.Service.Name = "eidos-risk"
	}
	if cfg.Service.GRPCPort == 0 {
		cfg.Service.GRPCPort = 50055
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

	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
	}
	if cfg.Postgres.MaxConnections == 0 {
		cfg.Postgres.MaxConnections = 30
	}
	if cfg.Postgres.MaxIdleConns == 0 {
		cfg.Postgres.MaxIdleConns = 10
	}
	if cfg.Postgres.ConnMaxLifetime == 0 {
		cfg.Postgres.ConnMaxLifetime = 3600 // 60 minutes in seconds
	}

	if len(cfg.Redis.Addresses) == 0 {
		cfg.Redis.Addresses = []string{"localhost:6379"}
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 50
	}

	// 风控限额默认值
	if cfg.Risk.Limits.DailyWithdraw == "" {
		cfg.Risk.Limits.DailyWithdraw = "100000"
	}
	if cfg.Risk.Limits.SingleOrder == "" {
		cfg.Risk.Limits.SingleOrder = "50000"
	}
	if cfg.Risk.Limits.PendingSettle == "" {
		cfg.Risk.Limits.PendingSettle = "10000"
	}
	if cfg.Risk.Limits.SystemPendingSettle == "" {
		cfg.Risk.Limits.SystemPendingSettle = "1000000"
	}
	if cfg.Risk.Limits.MaxOpenOrders == 0 {
		cfg.Risk.Limits.MaxOpenOrders = 100
	}
	if cfg.Risk.Limits.MaxDailyTrades == 0 {
		cfg.Risk.Limits.MaxDailyTrades = 10000
	}

	// 频率限制默认值
	if cfg.Risk.RateLimits.OrdersPerSecond == 0 {
		cfg.Risk.RateLimits.OrdersPerSecond = 10
	}
	if cfg.Risk.RateLimits.OrdersPerMinute == 0 {
		cfg.Risk.RateLimits.OrdersPerMinute = 300
	}
	if cfg.Risk.RateLimits.CancelsPerMinute == 0 {
		cfg.Risk.RateLimits.CancelsPerMinute = 100
	}

	// 监控配置默认值
	if cfg.Risk.Monitoring.LargeTradeThreshold == "" {
		cfg.Risk.Monitoring.LargeTradeThreshold = "100000"
	}
	if cfg.Risk.Monitoring.AlertIntervalSec == 0 {
		cfg.Risk.Monitoring.AlertIntervalSec = 60
	}

	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
}

// GetDailyWithdrawLimit 获取每日提现限额
func (c *LimitsConfig) GetDailyWithdrawLimit() decimal.Decimal {
	d, err := decimal.NewFromString(c.DailyWithdraw)
	if err != nil {
		return decimal.NewFromInt(100000)
	}
	return d
}

// GetSingleOrderLimit 获取单笔订单限额
func (c *LimitsConfig) GetSingleOrderLimit() decimal.Decimal {
	d, err := decimal.NewFromString(c.SingleOrder)
	if err != nil {
		return decimal.NewFromInt(50000)
	}
	return d
}

// GetPendingSettleLimit 获取待结算限额
func (c *LimitsConfig) GetPendingSettleLimit() decimal.Decimal {
	d, err := decimal.NewFromString(c.PendingSettle)
	if err != nil {
		return decimal.NewFromInt(10000)
	}
	return d
}

// GetSystemPendingSettleLimit 获取系统待结算限额
func (c *LimitsConfig) GetSystemPendingSettleLimit() decimal.Decimal {
	d, err := decimal.NewFromString(c.SystemPendingSettle)
	if err != nil {
		return decimal.NewFromInt(1000000)
	}
	return d
}

// GetLargeTradeThreshold 获取大额交易阈值
func (c *MonitoringConfig) GetLargeTradeThreshold() decimal.Decimal {
	d, err := decimal.NewFromString(c.LargeTradeThreshold)
	if err != nil {
		return decimal.NewFromInt(100000)
	}
	return d
}
