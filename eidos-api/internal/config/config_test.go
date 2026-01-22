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

func TestConfig_RedisAddr(t *testing.T) {
	cfg := &Config{
		Redis: commonConfig.RedisConfig{
			Addresses: []string{"localhost:6379"},
		},
	}
	assert.Equal(t, "localhost:6379", cfg.RedisAddr())
}

func TestConfig_RedisAddr_Empty(t *testing.T) {
	cfg := &Config{
		Redis: commonConfig.RedisConfig{
			Addresses: []string{},
		},
	}
	// 空地址应该返回默认值
	assert.Equal(t, "localhost:6379", cfg.RedisAddr())
}

func TestWebSocketConfig_PingInterval(t *testing.T) {
	cfg := &WebSocketConfig{
		PingIntervalSec: 30,
	}
	assert.Equal(t, 30*time.Second, cfg.PingInterval())
}

func TestWebSocketConfig_PongTimeout(t *testing.T) {
	cfg := &WebSocketConfig{
		PongTimeoutSec: 10,
	}
	assert.Equal(t, 10*time.Second, cfg.PongTimeout())
}

func TestWebSocketConfig_WriteWait(t *testing.T) {
	cfg := &WebSocketConfig{
		WriteWaitSec: 5,
	}
	assert.Equal(t, 5*time.Second, cfg.WriteWait())
}

func TestLoad_DefaultConfig(t *testing.T) {
	// 测试加载空路径时使用默认配置
	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// 验证默认值
	assert.Equal(t, "eidos-api", cfg.Service.Name)
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.Equal(t, "dev", cfg.Service.Env)
	assert.Contains(t, cfg.Redis.Addresses, "localhost:6379")
	assert.True(t, cfg.EIP712.MockMode)
}

func TestLoad_FromFile(t *testing.T) {
	// 创建临时配置文件（使用新的 addresses 格式）
	content := `
service:
  name: test-api
  http_port: 9090
  env: test
redis:
  addresses:
    - redis.local:6380
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "test-api", cfg.Service.Name)
	assert.Equal(t, 9090, cfg.Service.HTTPPort)
	assert.Equal(t, "test", cfg.Service.Env)
	assert.Contains(t, cfg.Redis.Addresses, "redis.local:6380")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read config file")
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)
	require.NoError(t, err)

	_, err = Load(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse config file")
}

func TestLoad_EnvOverride(t *testing.T) {
	// 设置环境变量（注意：REDIS_ADDR 是正确的环境变量名）
	os.Setenv("SERVICE_NAME", "env-api")
	os.Setenv("HTTP_PORT", "9999")
	os.Setenv("ENV", "production")
	os.Setenv("REDIS_ADDR", "redis.env.local:6381")
	os.Setenv("REDIS_PASSWORD", "secret123")
	os.Setenv("GRPC_TRADING_ADDR", "trading.env:50051")
	os.Setenv("GRPC_MARKET_ADDR", "market.env:50053")
	os.Setenv("NACOS_ENABLED", "true")
	os.Setenv("NACOS_SERVER_ADDR", "nacos.env:8848")
	os.Setenv("NACOS_NAMESPACE", "test-ns")
	os.Setenv("NACOS_GROUP", "TEST_GROUP")
	os.Setenv("EIP712_MOCK_MODE", "false")
	os.Setenv("EIP712_CHAIN_ID", "42161")
	os.Setenv("RATE_LIMIT_ENABLED", "false")

	defer func() {
		os.Unsetenv("SERVICE_NAME")
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("ENV")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_PASSWORD")
		os.Unsetenv("GRPC_TRADING_ADDR")
		os.Unsetenv("GRPC_MARKET_ADDR")
		os.Unsetenv("NACOS_ENABLED")
		os.Unsetenv("NACOS_SERVER_ADDR")
		os.Unsetenv("NACOS_NAMESPACE")
		os.Unsetenv("NACOS_GROUP")
		os.Unsetenv("EIP712_MOCK_MODE")
		os.Unsetenv("EIP712_CHAIN_ID")
		os.Unsetenv("RATE_LIMIT_ENABLED")
	}()

	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "env-api", cfg.Service.Name)
	assert.Equal(t, 9999, cfg.Service.HTTPPort)
	assert.Equal(t, "production", cfg.Service.Env)
	assert.Contains(t, cfg.Redis.Addresses, "redis.env.local:6381")
	assert.Equal(t, "secret123", cfg.Redis.Password)
	assert.Equal(t, "trading.env:50051", cfg.GRPCClients.Trading.Addr)
	assert.Equal(t, "market.env:50053", cfg.GRPCClients.Market.Addr)
	assert.True(t, cfg.Nacos.Enabled)
	assert.Equal(t, "nacos.env:8848", cfg.Nacos.ServerAddr)
	assert.Equal(t, "test-ns", cfg.Nacos.Namespace)
	assert.Equal(t, "TEST_GROUP", cfg.Nacos.Group)
	assert.False(t, cfg.EIP712.MockMode)
	assert.Equal(t, int64(42161), cfg.EIP712.Domain.ChainID)
	assert.False(t, cfg.RateLimit.Enabled)
}

func TestLoad_EnvOverride_InvalidPort(t *testing.T) {
	// 设置无效端口号
	os.Setenv("HTTP_PORT", "invalid")
	os.Setenv("EIP712_CHAIN_ID", "abc")

	defer func() {
		os.Unsetenv("HTTP_PORT")
		os.Unsetenv("EIP712_CHAIN_ID")
	}()

	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// 无效值应该保持默认值
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.Equal(t, int64(31337), cfg.EIP712.Domain.ChainID)
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	require.NotNil(t, cfg)

	// 验证默认值完整性
	assert.Equal(t, "eidos-api", cfg.Service.Name)
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.Equal(t, "dev", cfg.Service.Env)

	// Nacos 默认配置来自 commonConfig.DefaultNacosConfig()
	// 默认 Enabled=true, Namespace="eidos", Group="DEFAULT_GROUP"
	assert.True(t, cfg.Nacos.Enabled)
	assert.Equal(t, "eidos", cfg.Nacos.Namespace)
	assert.Equal(t, "DEFAULT_GROUP", cfg.Nacos.Group)

	assert.Contains(t, cfg.Redis.Addresses, "localhost:6379")
	assert.Equal(t, 100, cfg.Redis.PoolSize)

	assert.Equal(t, "localhost:50051", cfg.GRPCClients.Trading.Addr)
	assert.Equal(t, "localhost:50053", cfg.GRPCClients.Market.Addr)

	assert.Equal(t, "EidosExchange", cfg.EIP712.Domain.Name)
	assert.Equal(t, "1", cfg.EIP712.Domain.Version)
	assert.Equal(t, int64(31337), cfg.EIP712.Domain.ChainID)
	assert.True(t, cfg.EIP712.MockMode)
	assert.Equal(t, int64(300000), cfg.EIP712.TimestampToleranceMs)

	assert.True(t, cfg.RateLimit.Enabled)
	assert.Equal(t, 10, cfg.RateLimit.PerWallet.Orders)
	assert.Equal(t, 100, cfg.RateLimit.PerWallet.Queries)

	assert.Equal(t, 1024, cfg.WebSocket.ReadBufferSize)
	assert.Equal(t, 30, cfg.WebSocket.PingIntervalSec)
	assert.Equal(t, 100000, cfg.WebSocket.MaxConnections)

	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}
