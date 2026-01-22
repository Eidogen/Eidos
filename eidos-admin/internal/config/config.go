package config

import (
	"os"
	"strconv"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"gopkg.in/yaml.v3"
)

// Config 管理后台配置
type Config struct {
	Service     ServiceConfig                  `yaml:"service" json:"service"`
	Nacos       commonConfig.NacosConfig       `yaml:"nacos" json:"nacos"`
	Postgres    commonConfig.PostgresConfig    `yaml:"postgres" json:"postgres"`
	Redis       commonConfig.RedisConfig       `yaml:"redis" json:"redis"`
	Kafka       commonConfig.KafkaConfig       `yaml:"kafka" json:"kafka"`
	GRPCClients commonConfig.GRPCClientsConfig `yaml:"grpc_clients" json:"grpc_clients"`
	Auth        AuthConfig                     `yaml:"auth" json:"auth"`
	Log         commonConfig.LogConfig         `yaml:"log" json:"log"`
}

type ServiceConfig struct {
	Name     string `yaml:"name" json:"name"`
	HTTPPort int    `yaml:"http_port" json:"http_port"`
	Env      string `yaml:"env" json:"env"`
}

type ServerConfig struct {
	Port int    `yaml:"port" json:"port"`
	Mode string `yaml:"mode" json:"mode"` // debug, release
}

// AuthConfig 认证配置
type AuthConfig struct {
	JWT JWTConfig `yaml:"jwt" json:"jwt"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret      string `yaml:"secret" json:"secret"`
	ExpireHours int    `yaml:"expire_hours" json:"expire_hours"`
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

	// 环境变量覆盖
	overrideFromEnv(cfg)

	return cfg, nil
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

	// 数据库
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Postgres.Host = v
	}
	if v := os.Getenv("DB_DATABASE"); v != "" {
		cfg.Postgres.Database = v
	}
}

func defaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:     "eidos-admin",
			HTTPPort: GetEnvInt("HTTP_PORT", 8088),
			Env:      GetEnv("ENV", "dev"),
		},
		Nacos: commonConfig.DefaultNacosConfig(),
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
		Auth: AuthConfig{
			JWT: JWTConfig{
				Secret:      GetEnv("JWT_SECRET", "your-jwt-secret-key-here"),
				ExpireHours: GetEnvInt("JWT_EXPIRE_HOURS", 8),
			},
		},
		Log: commonConfig.LogConfig{
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
