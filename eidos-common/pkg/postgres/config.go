package postgres

import (
	"fmt"
	"time"
)

// Config PostgreSQL 连接配置
type Config struct {
	Host     string `yaml:"host" json:"host"`         // 数据库主机
	Port     int    `yaml:"port" json:"port"`         // 数据库端口
	Database string `yaml:"database" json:"database"` // 数据库名
	User     string `yaml:"user" json:"user"`         // 用户名
	Password string `yaml:"password" json:"password"` // 密码
	SSLMode  string `yaml:"ssl_mode" json:"ssl_mode"` // SSL 模式

	// 连接池配置
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`       // 最大打开连接数
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`       // 最大空闲连接数
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"` // 连接最大存活时间
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"` // 连接最大空闲时间

	// 健康检查配置
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"` // 健康检查间隔
	HealthCheckTimeout  time.Duration `yaml:"health_check_timeout" json:"health_check_timeout"`   // 健康检查超时

	// 重试配置
	MaxRetries     int           `yaml:"max_retries" json:"max_retries"`           // 最大重试次数
	RetryInterval  time.Duration `yaml:"retry_interval" json:"retry_interval"`     // 重试间隔
	ConnectTimeout time.Duration `yaml:"connect_timeout" json:"connect_timeout"`   // 连接超时

	// 日志配置
	LogLevel         string `yaml:"log_level" json:"log_level"`                   // 日志级别: silent, error, warn, info
	SlowQueryThreshold time.Duration `yaml:"slow_query_threshold" json:"slow_query_threshold"` // 慢查询阈值
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Host:                "localhost",
		Port:                5432,
		Database:            "postgres",
		User:                "postgres",
		Password:            "",
		SSLMode:             "disable",
		MaxOpenConns:        100,
		MaxIdleConns:        10,
		ConnMaxLifetime:     30 * time.Minute,
		ConnMaxIdleTime:     5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
		MaxRetries:          3,
		RetryInterval:       1 * time.Second,
		ConnectTimeout:      10 * time.Second,
		LogLevel:            "info",
		SlowQueryThreshold:  200 * time.Millisecond,
	}
}

// DSN 返回数据库连接字符串
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode, int(c.ConnectTimeout.Seconds()),
	)
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("postgres host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("postgres port must be between 1 and 65535")
	}
	if c.Database == "" {
		return fmt.Errorf("postgres database is required")
	}
	if c.User == "" {
		return fmt.Errorf("postgres user is required")
	}
	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("max_open_conns must be positive")
	}
	if c.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns must be non-negative")
	}
	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max_idle_conns cannot be greater than max_open_conns")
	}
	return nil
}

// ReadReplicaConfig 读副本配置
type ReadReplicaConfig struct {
	Config
	Weight int `yaml:"weight" json:"weight"` // 权重 (用于负载均衡)
}

// ClusterConfig 集群配置
type ClusterConfig struct {
	Master       *Config              `yaml:"master" json:"master"`               // 主库配置
	ReadReplicas []*ReadReplicaConfig `yaml:"read_replicas" json:"read_replicas"` // 读副本配置

	// 读写分离配置
	ReadWriteSplit bool `yaml:"read_write_split" json:"read_write_split"` // 是否启用读写分离
}
