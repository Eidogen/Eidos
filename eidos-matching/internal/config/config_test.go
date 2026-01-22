// Package config 配置管理单元测试
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	// 创建临时配置文件
	configContent := `
service:
  name: eidos-matching
  node_id: node-1
  grpc_port: 50052
  http_port: 8080
  env: test

nacos:
  server_addr: localhost:8848
  namespace: test

redis:
  addresses:
    - localhost:6379
  password: ""
  db: 0
  pool_size: 100

kafka:
  brokers:
    - localhost:9092
  consumer:
    max_poll_records: 100
  producer:
    compression: lz4

markets:
  - symbol: BTC-USDC
    base_token: BTC
    quote_token: USDC
    price_decimals: 2
    size_decimals: 4
    min_size: "0.0001"
    tick_size: "0.01"
    maker_fee_rate: "0.001"
    taker_fee_rate: "0.002"
    max_slippage: "0.05"

snapshot:
  interval: 30s
  max_count: 10
  two_phase: true

log:
  level: info
  format: json
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "eidos-matching", cfg.Service.Name)
	assert.Equal(t, "node-1", cfg.Service.NodeID)
	assert.Equal(t, 50052, cfg.Service.GRPCPort)
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.Equal(t, []string{"localhost:6379"}, cfg.Redis.Addresses)
	assert.Equal(t, []string{"localhost:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, 1, len(cfg.Markets))
	assert.Equal(t, "BTC-USDC", cfg.Markets[0].Symbol)
	assert.Equal(t, 30*time.Second, cfg.Snapshot.Interval)
	assert.True(t, cfg.Snapshot.TwoPhase)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
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
	assert.Contains(t, err.Error(), "unmarshal config")
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	os.Setenv("TEST_REDIS_PASSWORD", "secret123")
	defer os.Unsetenv("TEST_REDIS_PASSWORD")

	configContent := `
redis:
  addresses:
    - localhost:6379
  password: $TEST_REDIS_PASSWORD
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, "secret123", cfg.Redis.Password)
}

func TestConfig_SetDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.setDefaults()

	// Service defaults
	assert.Equal(t, "eidos-matching", cfg.Service.Name)
	assert.Equal(t, 50052, cfg.Service.GRPCPort)
	assert.Equal(t, 8080, cfg.Service.HTTPPort)
	assert.NotEmpty(t, cfg.Service.NodeID)

	// Kafka defaults - 使用新的字段名称
	assert.Equal(t, 100, cfg.Kafka.Consumer.MaxPollRecords)
	assert.Equal(t, 5, cfg.Kafka.Producer.LingerMs)
	assert.Equal(t, 1000, cfg.Kafka.Producer.BatchSize)
	assert.Equal(t, "lz4", cfg.Kafka.Producer.Compression)

	// Snapshot defaults
	assert.Equal(t, 30*time.Second, cfg.Snapshot.Interval)
	assert.Equal(t, 10, cfg.Snapshot.MaxCount)

	// HA defaults
	assert.Equal(t, 100*time.Millisecond, cfg.HA.HeartbeatInterval)
	assert.Equal(t, 500*time.Millisecond, cfg.HA.FailoverTimeout)

	// Log defaults
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestConfig_SetDefaults_DoesNotOverride(t *testing.T) {
	cfg := &Config{
		Service: ServiceConfig{
			Name:     "custom-name",
			GRPCPort: 9999,
			HTTPPort: 8888,
			NodeID:   "custom-node",
		},
		Kafka: commonConfig.KafkaConfig{
			Consumer: commonConfig.KafkaConsumer{
				MaxPollRecords: 200,
			},
			Producer: commonConfig.KafkaProducer{
				BatchSize:    2000,
				Compression:  "gzip",
				RequiredAcks: 1,
				LingerMs:     10,
			},
		},
		Snapshot: SnapshotConfig{
			Interval: time.Minute,
			MaxCount: 5,
		},
		HA: HAConfig{
			HeartbeatInterval: 200 * time.Millisecond,
			FailoverTimeout:   time.Second,
		},
		Log: commonConfig.LogConfig{
			Level:  "debug",
			Format: "text",
		},
	}
	cfg.setDefaults()

	// 确保已设置的值不被覆盖
	assert.Equal(t, "custom-name", cfg.Service.Name)
	assert.Equal(t, 9999, cfg.Service.GRPCPort)
	assert.Equal(t, 8888, cfg.Service.HTTPPort)
	assert.Equal(t, "custom-node", cfg.Service.NodeID)
	assert.Equal(t, 200, cfg.Kafka.Consumer.MaxPollRecords)
	assert.Equal(t, "gzip", cfg.Kafka.Producer.Compression)
	assert.Equal(t, time.Minute, cfg.Snapshot.Interval)
	assert.Equal(t, "debug", cfg.Log.Level)
}

func TestMarketConfig_ToEngineMarketConfig(t *testing.T) {
	mc := &MarketConfig{
		Symbol:        "BTC-USDC",
		BaseToken:     "BTC",
		QuoteToken:    "USDC",
		PriceDecimals: 2,
		SizeDecimals:  4,
		MinSize:       "0.0001",
		TickSize:      "0.01",
		MakerFeeRate:  "0.001",
		TakerFeeRate:  "0.002",
		MaxSlippage:   "0.05",
	}

	ec := mc.ToEngineMarketConfig()

	assert.Equal(t, "BTC-USDC", ec.Symbol)
	assert.Equal(t, "BTC", ec.BaseToken)
	assert.Equal(t, "USDC", ec.QuoteToken)
	assert.Equal(t, 2, ec.PriceDecimals)
	assert.Equal(t, 4, ec.SizeDecimals)
	assert.True(t, decimal.NewFromFloat(0.0001).Equal(ec.MinSize))
	assert.True(t, decimal.NewFromFloat(0.01).Equal(ec.TickSize))
	assert.True(t, decimal.NewFromFloat(0.001).Equal(ec.MakerFeeRate))
	assert.True(t, decimal.NewFromFloat(0.002).Equal(ec.TakerFeeRate))
	assert.True(t, decimal.NewFromFloat(0.05).Equal(ec.MaxSlippage))
}

func TestMarketConfig_ToEngineMarketConfig_InvalidDecimals(t *testing.T) {
	mc := &MarketConfig{
		Symbol:       "BTC-USDC",
		MinSize:      "invalid",
		TickSize:     "invalid",
		MakerFeeRate: "invalid",
		TakerFeeRate: "invalid",
		MaxSlippage:  "invalid",
	}

	ec := mc.ToEngineMarketConfig()

	// Invalid values should result in zero decimal
	assert.True(t, decimal.Zero.Equal(ec.MinSize))
	assert.True(t, decimal.Zero.Equal(ec.TickSize))
	assert.True(t, decimal.Zero.Equal(ec.MakerFeeRate))
	assert.True(t, decimal.Zero.Equal(ec.TakerFeeRate))
	assert.True(t, decimal.Zero.Equal(ec.MaxSlippage))
}

func TestServiceConfig_Fields(t *testing.T) {
	sc := ServiceConfig{
		Name:     "test-service",
		NodeID:   "node-1",
		GRPCPort: 50052,
		HTTPPort: 8080,
		Env:      "production",
	}

	assert.Equal(t, "test-service", sc.Name)
	assert.Equal(t, "node-1", sc.NodeID)
	assert.Equal(t, 50052, sc.GRPCPort)
	assert.Equal(t, 8080, sc.HTTPPort)
	assert.Equal(t, "production", sc.Env)
}

func TestRedisConfig_Fields(t *testing.T) {
	rc := commonConfig.RedisConfig{
		Addresses: []string{"localhost:6379", "localhost:6380"},
		Password:  "secret",
		DB:        1,
		PoolSize:  100,
	}

	assert.Equal(t, 2, len(rc.Addresses))
	assert.Equal(t, "secret", rc.Password)
	assert.Equal(t, 1, rc.DB)
	assert.Equal(t, 100, rc.PoolSize)
}

func TestKafkaConfig_Fields(t *testing.T) {
	kc := commonConfig.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Consumer: commonConfig.KafkaConsumer{
			Topics:         []string{"orders", "cancels"},
			MaxPollRecords: 100,
		},
		Producer: commonConfig.KafkaProducer{
			Compression:  "lz4",
			BatchSize:    1000,
			LingerMs:     10,
			RequiredAcks: -1,
		},
	}

	assert.Equal(t, 1, len(kc.Brokers))
	assert.Equal(t, 2, len(kc.Consumer.Topics))
	assert.Equal(t, "lz4", kc.Producer.Compression)
}

func TestHAConfig_Fields(t *testing.T) {
	hc := HAConfig{
		Enabled:           true,
		HeartbeatInterval: 100 * time.Millisecond,
		FailoverTimeout:   500 * time.Millisecond,
	}

	assert.True(t, hc.Enabled)
	assert.Equal(t, 100*time.Millisecond, hc.HeartbeatInterval)
	assert.Equal(t, 500*time.Millisecond, hc.FailoverTimeout)
}
