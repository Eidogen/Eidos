package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Service
	assert.Equal(t, "eidos-market", cfg.Service.Name)
	assert.Equal(t, 50053, cfg.Service.GRPCPort)
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.Equal(t, "dev", cfg.Service.Env)

	// Nacos
	assert.Equal(t, "nacos:8848", cfg.Nacos.ServerAddr)
	assert.Equal(t, "eidos", cfg.Nacos.Namespace)
	assert.Equal(t, "DEFAULT_GROUP", cfg.Nacos.Group)

	// Postgres
	assert.Equal(t, "postgres", cfg.Postgres.Host)
	assert.Equal(t, 5432, cfg.Postgres.Port)
	assert.Equal(t, "eidos_market", cfg.Postgres.Database)
	assert.Equal(t, "eidos", cfg.Postgres.User)
	assert.Equal(t, 50, cfg.Postgres.MaxConnections)
	assert.Equal(t, 10, cfg.Postgres.MaxIdleConns)
	assert.Equal(t, 3600, cfg.Postgres.ConnMaxLifetime)

	// Redis
	assert.Equal(t, []string{"redis:6379"}, cfg.Redis.Addresses)
	assert.Equal(t, "", cfg.Redis.Password)
	assert.Equal(t, 0, cfg.Redis.DB)
	assert.Equal(t, 100, cfg.Redis.PoolSize)

	// Kafka
	assert.Equal(t, []string{"kafka:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, "eidos-market", cfg.Kafka.GroupID)
	assert.Equal(t, "", cfg.Kafka.ClientID)

	// Kline
	expectedIntervals := []string{"1m", "5m", "15m", "30m", "1h", "4h", "1d", "1w"}
	assert.Equal(t, expectedIntervals, cfg.Kline.Intervals)

	// Aggregator
	assert.Equal(t, 10, cfg.Aggregator.KlineFlushInterval)
	assert.Equal(t, 1000, cfg.Aggregator.TickerPublishInterval)
	assert.Equal(t, 100, cfg.Aggregator.DepthPublishInterval)
	assert.Equal(t, 100, cfg.Aggregator.DepthMaxLevels)

	// Log
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestLoad_DefaultsOnly(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should return defaults when no path provided
	assert.Equal(t, "eidos-market", cfg.Service.Name)
	assert.Equal(t, 50053, cfg.Service.GRPCPort)
}

func TestLoad_FromFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
service:
  name: "test-market"
  grpc_port: 50099
  http_port: 8099
  env: "test"
log:
  level: "debug"
  format: "console"
aggregator:
  kline_flush_interval: 30
  ticker_publish_interval: 2000
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "test-market", cfg.Service.Name)
	assert.Equal(t, 50099, cfg.Service.GRPCPort)
	assert.Equal(t, 8099, cfg.Service.HTTPPort)
	assert.Equal(t, "test", cfg.Service.Env)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "console", cfg.Log.Format)
	assert.Equal(t, 30, cfg.Aggregator.KlineFlushInterval)
	assert.Equal(t, 2000, cfg.Aggregator.TickerPublishInterval)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	require.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Invalid YAML content
	content := `
service:
  name: "test"
  grpc_port: [invalid
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	_, err = Load(configPath)
	require.Error(t, err)
}

func TestLoad_EnvExpansion(t *testing.T) {
	// Set environment variable
	os.Setenv("TEST_SERVICE_NAME", "env-expanded-service")
	defer os.Unsetenv("TEST_SERVICE_NAME")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
service:
  name: "${TEST_SERVICE_NAME}"
  grpc_port: 50053
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	assert.Equal(t, "env-expanded-service", cfg.Service.Name)
}

func TestApplyEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVICE_NAME", "env-service")
	os.Setenv("GRPC_PORT", "60000")
	os.Setenv("HTTP_PORT", "9000")
	os.Setenv("ENV", "staging")
	os.Setenv("NACOS_SERVER_ADDR", "nacos.env:8848")
	os.Setenv("NACOS_NAMESPACE", "env-namespace")
	os.Setenv("POSTGRES_HOST", "pg.env")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_DATABASE", "env_db")
	os.Setenv("POSTGRES_USER", "env_user")
	os.Setenv("POSTGRES_PASSWORD", "env_password")
	os.Setenv("REDIS_ADDR", "redis.env:6379")
	os.Setenv("REDIS_PASSWORD", "redis_password")
	os.Setenv("LOG_LEVEL", "debug")

	defer func() {
		os.Unsetenv("SERVICE_NAME")
		os.Unsetenv("GRPC_PORT")
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("ENV")
		os.Unsetenv("NACOS_SERVER_ADDR")
		os.Unsetenv("NACOS_NAMESPACE")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_DATABASE")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_PASSWORD")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg, err := Load("")
	require.NoError(t, err)

	// Verify overrides
	assert.Equal(t, "env-service", cfg.Service.Name)
	assert.Equal(t, 60000, cfg.Service.GRPCPort)
	assert.Equal(t, 9000, cfg.Service.HTTPPort)
	assert.Equal(t, "staging", cfg.Service.Env)
	assert.Equal(t, "nacos.env:8848", cfg.Nacos.ServerAddr)
	assert.Equal(t, "env-namespace", cfg.Nacos.Namespace)
	assert.Equal(t, "pg.env", cfg.Postgres.Host)
	assert.Equal(t, 5433, cfg.Postgres.Port)
	assert.Equal(t, "env_db", cfg.Postgres.Database)
	assert.Equal(t, "env_user", cfg.Postgres.User)
	assert.Equal(t, "env_password", cfg.Postgres.Password)
	assert.Equal(t, []string{"redis.env:6379"}, cfg.Redis.Addresses)
	assert.Equal(t, "redis_password", cfg.Redis.Password)
	assert.Equal(t, "debug", cfg.Log.Level)
}

func TestGetKlineFlushInterval(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 10*time.Second, cfg.GetKlineFlushInterval())

	cfg.Aggregator.KlineFlushInterval = 30
	assert.Equal(t, 30*time.Second, cfg.GetKlineFlushInterval())
}

func TestGetTickerPublishInterval(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 1000*time.Millisecond, cfg.GetTickerPublishInterval())

	cfg.Aggregator.TickerPublishInterval = 500
	assert.Equal(t, 500*time.Millisecond, cfg.GetTickerPublishInterval())
}

func TestGetDepthPublishInterval(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 100*time.Millisecond, cfg.GetDepthPublishInterval())

	cfg.Aggregator.DepthPublishInterval = 200
	assert.Equal(t, 200*time.Millisecond, cfg.GetDepthPublishInterval())
}

func TestIsDev(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.IsDev())

	cfg.Service.Env = "prod"
	assert.False(t, cfg.IsDev())

	cfg.Service.Env = "staging"
	assert.False(t, cfg.IsDev())
}

func TestIsProd(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.IsProd())

	cfg.Service.Env = "prod"
	assert.True(t, cfg.IsProd())

	cfg.Service.Env = "staging"
	assert.False(t, cfg.IsProd())
}

func TestServiceConfig(t *testing.T) {
	sc := ServiceConfig{
		Name:     "test-service",
		GRPCPort: 50053,
		HTTPPort: 8080,
		Env:      "test",
	}

	assert.Equal(t, "test-service", sc.Name)
	assert.Equal(t, 50053, sc.GRPCPort)
	assert.Equal(t, 8080, sc.HTTPPort)
	assert.Equal(t, "test", sc.Env)
}

func TestKlineConfig(t *testing.T) {
	kc := KlineConfig{
		Intervals: []string{"1m", "5m", "1h"},
	}

	assert.Len(t, kc.Intervals, 3)
	assert.Contains(t, kc.Intervals, "1m")
	assert.Contains(t, kc.Intervals, "5m")
	assert.Contains(t, kc.Intervals, "1h")
}

func TestAggregatorConfig(t *testing.T) {
	ac := AggregatorConfig{
		KlineFlushInterval:    15,
		TickerPublishInterval: 500,
		DepthPublishInterval:  50,
		DepthMaxLevels:        200,
	}

	assert.Equal(t, 15, ac.KlineFlushInterval)
	assert.Equal(t, 500, ac.TickerPublishInterval)
	assert.Equal(t, 50, ac.DepthPublishInterval)
	assert.Equal(t, 200, ac.DepthMaxLevels)
}

func TestLogConfig(t *testing.T) {
	lc := commonConfig.LogConfig{
		Level:  "warn",
		Format: "console",
	}

	assert.Equal(t, "warn", lc.Level)
	assert.Equal(t, "console", lc.Format)
}
