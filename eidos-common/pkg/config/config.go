package config

import (
	"os"
	"strconv"
	"strings"
)

// GetEnv 获取环境变量，支持默认值
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt 获取整数环境变量
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvInt64 获取 int64 环境变量
func GetEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvBool 获取布尔环境变量
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

// GetEnvSlice 获取逗号分隔的字符串切片
func GetEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
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
	ConnMaxLifetime int    `yaml:"conn_max_lifetime" json:"conn_max_lifetime"` // 秒
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

// NacosConfig Nacos 配置
type NacosConfig struct {
	ServerAddr  string `yaml:"server_addr" json:"server_addr"`
	Namespace   string `yaml:"namespace" json:"namespace"`
	Group       string `yaml:"group" json:"group"`
	ServiceName string `yaml:"service_name" json:"service_name"`
}

// GRPCConfig gRPC 服务配置
type GRPCConfig struct {
	Port int `yaml:"port" json:"port"`
}

// HTTPConfig HTTP 服务配置
type HTTPConfig struct {
	Port int `yaml:"port" json:"port"`
}

// EIP712Config EIP-712 配置
type EIP712Config struct {
	DomainName        string `yaml:"domain_name" json:"domain_name"`
	DomainVersion     string `yaml:"domain_version" json:"domain_version"`
	ChainID           int64  `yaml:"chain_id" json:"chain_id"`
	VerifyingContract string `yaml:"verifying_contract" json:"verifying_contract"`
}
