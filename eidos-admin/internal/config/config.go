package config

import (
	"os"
	"strconv"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig                `yaml:"server" json:"server"`
	Service     ServiceConfig               `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig    `yaml:"nacos" json:"nacos"`
	Postgres    commonConfig.PostgresConfig `yaml:"postgres" json:"postgres"`
	Redis       commonConfig.RedisConfig    `yaml:"redis" json:"redis"`
	GRPCClients GRPCClientsConfig           `yaml:"grpc_clients" json:"grpc_clients"`
	JWT         JWTConfig                   `yaml:"jwt" json:"jwt"`
	Log         LogConfig                   `yaml:"log" json:"log"`
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
	Jobs     string `yaml:"jobs" json:"jobs"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret" json:"secret"`
	ExpireHours int    `yaml:"expire_hours" json:"expire_hours"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	// 从文件加载
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			// 展开环境变量: ${VAR:DEFAULT}
			expanded := commonConfig.ExpandEnv(string(data))
			if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
				return nil, err
			}
		}
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: GetEnvInt("HTTP_PORT", 8088),
			Mode: GetEnv("GIN_MODE", "debug"),
		},
		Service: ServiceConfig{
			Name:     "eidos-admin",
			HTTPPort: GetEnvInt("HTTP_PORT", 8088),
			Env:      GetEnv("ENV", "dev"),
		},
		Postgres: commonConfig.PostgresConfig{
			Host:            GetEnv("POSTGRES_HOST", "localhost"),
			Port:            GetEnvInt("POSTGRES_PORT", 5432),
			Database:        GetEnv("POSTGRES_DATABASE", "eidos_admin"),
			User:            GetEnv("POSTGRES_USER", "eidos"),
			Password:        GetEnv("POSTGRES_PASSWORD", "eidos123"),
			MaxConnections:  GetEnvInt("POSTGRES_MAX_CONNS", 30),
			MaxIdleConns:    GetEnvInt("POSTGRES_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: GetEnvInt("POSTGRES_CONN_MAX_LIFETIME", 3600),
		},
		Redis: commonConfig.RedisConfig{
			Addresses: []string{GetEnv("REDIS_ADDR", "localhost:6379")},
			Password:  GetEnv("REDIS_PASSWORD", ""),
			DB:        GetEnvInt("REDIS_DB", 0),
			PoolSize:  GetEnvInt("REDIS_POOL_SIZE", 20),
		},
		JWT: JWTConfig{
			Secret:      GetEnv("JWT_SECRET", "your-jwt-secret-key-here"),
			ExpireHours: GetEnvInt("JWT_EXPIRE_HOURS", 8),
		},
		Log: LogConfig{
			Level:  GetEnv("LOG_LEVEL", "info"),
			Format: GetEnv("LOG_FORMAT", "json"),
		},
	}
}

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
