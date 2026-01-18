// Package config 提供风控服务配置管理
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Config 风控服务配置
type Config struct {
	Service  ServiceConfig  `yaml:"service" json:"service"`
	Nacos    NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis    RedisConfig    `yaml:"redis" json:"redis"`
	Kafka    KafkaConfig    `yaml:"kafka" json:"kafka"`
	Risk     RiskConfig     `yaml:"risk" json:"risk"`
	Log      LogConfig      `yaml:"log" json:"log"`
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
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	ServerAddr string `yaml:"server_addr" json:"server_addr"`
	Namespace  string `yaml:"namespace" json:"namespace"`
	Group      string `yaml:"group" json:"group"`
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	LogDir     string `yaml:"log_dir" json:"log_dir"`
	CacheDir   string `yaml:"cache_dir" json:"cache_dir"`
}

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	Database               string `yaml:"database" json:"database"`
	User                   string `yaml:"user" json:"user"`
	Password               string `yaml:"password" json:"password"`
	MaxConnections         int    `yaml:"max_connections" json:"max_connections"`
	MaxIdleConns           int    `yaml:"max_idle_conns" json:"max_idle_conns"`
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
	Enabled  bool     `yaml:"enabled" json:"enabled"`
	Brokers  []string `yaml:"brokers" json:"brokers"`
	GroupID  string   `yaml:"group_id" json:"group_id"`
	ClientID string   `yaml:"client_id" json:"client_id"`
}

// RiskConfig 风控配置
type RiskConfig struct {
	Limits     LimitsConfig     `yaml:"limits" json:"limits"`
	RateLimits RateLimitsConfig `yaml:"rate_limits" json:"rate_limits"`
	Monitoring MonitoringConfig `yaml:"monitoring" json:"monitoring"`
}

// LimitsConfig 限额配置
type LimitsConfig struct {
	DailyWithdraw       string `yaml:"daily_withdraw" json:"daily_withdraw"`             // 单用户每日提现上限
	SingleOrder         string `yaml:"single_order" json:"single_order"`                 // 单笔订单上限
	PendingSettle       string `yaml:"pending_settle" json:"pending_settle"`             // 用户待结算余额上限
	SystemPendingSettle string `yaml:"system_pending_settle" json:"system_pending_settle"` // 系统待结算上限
	MaxOpenOrders       int    `yaml:"max_open_orders" json:"max_open_orders"`           // 单用户最大挂单数
	MaxDailyTrades      int    `yaml:"max_daily_trades" json:"max_daily_trades"`         // 单用户每日成交笔数上限
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
	content = expandEnvVars(content)

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaults(&cfg)

	return &cfg, nil
}

// expandEnvVars 展开环境变量 ${VAR:default}
func expandEnvVars(s string) string {
	result := s
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		expr := result[start+2 : end]
		parts := strings.SplitN(expr, ":", 2)
		varName := parts[0]
		defaultVal := ""
		if len(parts) > 1 {
			defaultVal = parts[1]
		}

		value := os.Getenv(varName)
		if value == "" {
			value = defaultVal
		}

		result = result[:start] + value + result[end+1:]
	}
	return result
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

	if cfg.Postgres.Port == 0 {
		cfg.Postgres.Port = 5432
	}
	if cfg.Postgres.MaxConnections == 0 {
		cfg.Postgres.MaxConnections = 30
	}
	if cfg.Postgres.MaxIdleConns == 0 {
		cfg.Postgres.MaxIdleConns = 10
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes == 0 {
		cfg.Postgres.ConnMaxLifetimeMinutes = 60
	}

	if cfg.Redis.Port == 0 {
		cfg.Redis.Port = 6379
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
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
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

// GetEnvInt 获取环境变量整数值
func GetEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// GetEnvString 获取环境变量字符串值
func GetEnvString(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
