package config

import (
	"os"
	"strconv"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
)

type Config struct {
	Server      ServerConfig              `yaml:"server" json:"server"`
	Service     ServiceConfig             `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig  `yaml:"nacos" json:"nacos"`
	Postgres    commonConfig.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis       commonConfig.RedisConfig  `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig         `yaml:"grpc_clients" json:"grpc_clients"`
	JWT         JWTConfig                 `yaml:"jwt" json:"jwt"`
	Log         LogConfig                 `yaml:"log" json:"log"`
}

type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Mode string `yaml:"mode" json:"mode"` // debug, release
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type GRPCClientsConfig struct {
	Trading  string `yaml:"trading" json:"trading"`
	Matching string `yaml:"matching" json:"matching"`
	Market   string `yaml:"market" json:"market"`
	Chain    string `yaml:"chain" json:"chain"`
	Risk     string `yaml:"risk" json:"risk"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret" json:"secret"`
	ExpireHours int    `yaml:"expire_hours" json:"expire_hours"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("HTTP_PORT", 8088),
			Mode: getEnv("GIN_MODE", "debug"),
		},
		Service: ServiceConfig{
			Name:     "eidos-admin",
			HTTPPort: getEnvInt("HTTP_PORT", 8088),
			Env:      getEnv("ENV", "dev"),
		},
		Postgres: commonConfig.PostgresConfig{
			Host:            getEnv("POSTGRES_HOST", "localhost"),
			Port:            getEnvInt("POSTGRES_PORT", 5432),
			Database:        getEnv("POSTGRES_DATABASE", "eidos_admin"),
			User:            getEnv("POSTGRES_USER", "eidos"),
			Password:        getEnv("POSTGRES_PASSWORD", "eidos123"),
			MaxConnections:  getEnvInt("POSTGRES_MAX_CONNS", 30),
			MaxIdleConns:    getEnvInt("POSTGRES_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvInt("POSTGRES_CONN_MAX_LIFETIME", 3600),
		},
		Redis: commonConfig.RedisConfig{
			Addresses: []string{getEnv("REDIS_ADDR", "localhost:6379")},
			Password:  getEnv("REDIS_PASSWORD", ""),
			DB:        getEnvInt("REDIS_DB", 0),
			PoolSize:  getEnvInt("REDIS_POOL_SIZE", 20),
		},
		JWT: JWTConfig{
			Secret:      getEnv("JWT_SECRET", "your-jwt-secret-key-here"),
			ExpireHours: getEnvInt("JWT_EXPIRE_HOURS", 8),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
