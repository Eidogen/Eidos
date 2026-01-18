package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExpandEnvVars 测试环境变量展开
func TestExpandEnvVars(t *testing.T) {
	t.Run("simple variable", func(t *testing.T) {
		os.Setenv("TEST_VAR", "hello")
		defer os.Unsetenv("TEST_VAR")

		result := expandEnvVars("value is ${TEST_VAR}")
		assert.Equal(t, "value is hello", result)
	})

	t.Run("variable with default", func(t *testing.T) {
		// 不设置环境变量，使用默认值
		result := expandEnvVars("value is ${NOT_EXISTS:default_value}")
		assert.Equal(t, "value is default_value", result)
	})

	t.Run("variable with default overridden", func(t *testing.T) {
		os.Setenv("MY_VAR", "actual_value")
		defer os.Unsetenv("MY_VAR")

		result := expandEnvVars("value is ${MY_VAR:default_value}")
		assert.Equal(t, "value is actual_value", result)
	})

	t.Run("multiple variables", func(t *testing.T) {
		os.Setenv("VAR1", "first")
		os.Setenv("VAR2", "second")
		defer os.Unsetenv("VAR1")
		defer os.Unsetenv("VAR2")

		result := expandEnvVars("${VAR1} and ${VAR2}")
		assert.Equal(t, "first and second", result)
	})

	t.Run("no variables", func(t *testing.T) {
		result := expandEnvVars("no variables here")
		assert.Equal(t, "no variables here", result)
	})

	t.Run("empty default", func(t *testing.T) {
		result := expandEnvVars("value is ${NOT_EXISTS:}")
		assert.Equal(t, "value is ", result)
	})

	t.Run("default with colon", func(t *testing.T) {
		result := expandEnvVars("value is ${NOT_EXISTS:default:with:colons}")
		assert.Equal(t, "value is default:with:colons", result)
	})
}

// TestSetDefaults 测试默认值设置
func TestSetDefaults(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		cfg := &Config{}
		setDefaults(cfg)

		assert.Equal(t, "eidos-chain", cfg.Service.Name)
		assert.Equal(t, 50054, cfg.Service.GRPCPort)
		assert.Equal(t, "dev", cfg.Service.Env)

		assert.Equal(t, 5432, cfg.Postgres.Port)
		assert.Equal(t, 50, cfg.Postgres.MaxConnections)
		assert.Equal(t, 10, cfg.Postgres.MaxIdleConns)
		assert.Equal(t, 3600, cfg.Postgres.ConnMaxLifetime)

		assert.Equal(t, 50, cfg.Redis.PoolSize)

		assert.Equal(t, int64(31337), cfg.Blockchain.ChainID)

		assert.Equal(t, 100, cfg.Settlement.BatchSize)
		assert.Equal(t, 10, cfg.Settlement.BatchInterval)
		assert.Equal(t, 3, cfg.Settlement.MaxRetries)
		assert.Equal(t, 30, cfg.Settlement.RetryBackoff)

		assert.Equal(t, "info", cfg.Log.Level)
		assert.Equal(t, "json", cfg.Log.Format)
	})

	t.Run("partial config", func(t *testing.T) {
		cfg := &Config{
			Service: ServiceConfig{
				Name:     "custom-name",
				GRPCPort: 9999,
			},
			Blockchain: BlockchainConfig{
				ChainID: 42161, // Arbitrum mainnet
			},
		}
		setDefaults(cfg)

		// 已设置的值不应该被覆盖
		assert.Equal(t, "custom-name", cfg.Service.Name)
		assert.Equal(t, 9999, cfg.Service.GRPCPort)
		assert.Equal(t, int64(42161), cfg.Blockchain.ChainID)

		// 未设置的值应该使用默认值
		assert.Equal(t, "dev", cfg.Service.Env)
		assert.Equal(t, 5432, cfg.Postgres.Port)
	})
}

// TestGetEnvInt 测试获取环境变量整数值
func TestGetEnvInt(t *testing.T) {
	t.Run("env variable exists", func(t *testing.T) {
		os.Setenv("TEST_INT", "42")
		defer os.Unsetenv("TEST_INT")

		result := GetEnvInt("TEST_INT", 0)
		assert.Equal(t, 42, result)
	})

	t.Run("env variable not exists", func(t *testing.T) {
		result := GetEnvInt("NOT_EXISTS_INT", 100)
		assert.Equal(t, 100, result)
	})

	t.Run("env variable invalid", func(t *testing.T) {
		os.Setenv("TEST_INVALID_INT", "not-a-number")
		defer os.Unsetenv("TEST_INVALID_INT")

		result := GetEnvInt("TEST_INVALID_INT", 50)
		assert.Equal(t, 50, result)
	})

	t.Run("env variable empty", func(t *testing.T) {
		os.Setenv("TEST_EMPTY_INT", "")
		defer os.Unsetenv("TEST_EMPTY_INT")

		result := GetEnvInt("TEST_EMPTY_INT", 25)
		assert.Equal(t, 25, result)
	})
}

// TestGetEnvString 测试获取环境变量字符串值
func TestGetEnvString(t *testing.T) {
	t.Run("env variable exists", func(t *testing.T) {
		os.Setenv("TEST_STRING", "hello")
		defer os.Unsetenv("TEST_STRING")

		result := GetEnvString("TEST_STRING", "default")
		assert.Equal(t, "hello", result)
	})

	t.Run("env variable not exists", func(t *testing.T) {
		result := GetEnvString("NOT_EXISTS_STRING", "default")
		assert.Equal(t, "default", result)
	})

	t.Run("env variable empty", func(t *testing.T) {
		os.Setenv("TEST_EMPTY_STRING", "")
		defer os.Unsetenv("TEST_EMPTY_STRING")

		result := GetEnvString("TEST_EMPTY_STRING", "default")
		assert.Equal(t, "default", result)
	})
}

// TestLoad 测试配置加载
func TestLoad(t *testing.T) {
	t.Run("file not exists", func(t *testing.T) {
		_, err := Load("/path/to/nonexistent/config.yaml")
		assert.Error(t, err)
	})

	t.Run("valid config file", func(t *testing.T) {
		// 创建临时配置文件
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		configContent := `
service:
  name: eidos-chain-test
  grpc_port: 50055
  env: test

postgres:
  host: localhost
  port: 5432
  database: eidos_chain_test
  user: postgres
  password: ${DB_PASSWORD:test_password}

redis:
  addresses:
    - localhost:6379
  password: ""

kafka:
  brokers:
    - localhost:9092
  group_id: eidos-chain-test

blockchain:
  rpc_url: http://localhost:8545
  chain_id: 31337
  contract_address: "0x0000000000000000000000000000000000000000"

settlement:
  batch_size: 50
  batch_interval: 5

log:
  level: debug
  format: console
`
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		assert.NoError(t, err)

		cfg, err := Load(configPath)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)

		// 验证配置值
		assert.Equal(t, "eidos-chain-test", cfg.Service.Name)
		assert.Equal(t, 50055, cfg.Service.GRPCPort)
		assert.Equal(t, "test", cfg.Service.Env)
		assert.Equal(t, "localhost", cfg.Postgres.Host)
		assert.Equal(t, "test_password", cfg.Postgres.Password) // 使用默认值
		assert.Equal(t, 50, cfg.Settlement.BatchSize)
		assert.Equal(t, "debug", cfg.Log.Level)
	})

	t.Run("config with env override", func(t *testing.T) {
		// 创建临时配置文件
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		configContent := `
service:
  name: eidos-chain
  grpc_port: 50054

postgres:
  password: ${DB_PASSWORD:default_pw}
`
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		assert.NoError(t, err)

		// 设置环境变量
		os.Setenv("DB_PASSWORD", "secret_password")
		defer os.Unsetenv("DB_PASSWORD")

		cfg, err := Load(configPath)
		assert.NoError(t, err)
		assert.Equal(t, "secret_password", cfg.Postgres.Password)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yaml")

		// 无效的 YAML
		invalidContent := `
service:
  name: [this is not valid
  grpc_port 50054
`
		err := os.WriteFile(configPath, []byte(invalidContent), 0644)
		assert.NoError(t, err)

		_, err = Load(configPath)
		assert.Error(t, err)
	})
}

// TestConfigStructs 测试配置结构体
func TestConfigStructs(t *testing.T) {
	t.Run("ServiceConfig", func(t *testing.T) {
		cfg := ServiceConfig{
			Name:     "test-service",
			GRPCPort: 50054,
			HTTPPort: 8080,
			Env:      "production",
		}

		assert.Equal(t, "test-service", cfg.Name)
		assert.Equal(t, 50054, cfg.GRPCPort)
		assert.Equal(t, 8080, cfg.HTTPPort)
		assert.Equal(t, "production", cfg.Env)
	})

	t.Run("PostgresConfig", func(t *testing.T) {
		cfg := PostgresConfig{
			Host:            "db.example.com",
			Port:            5432,
			Database:        "eidos_chain",
			User:            "admin",
			Password:        "secret",
			MaxConnections:  100,
			MaxIdleConns:    20,
			ConnMaxLifetime: 7200,
		}

		assert.Equal(t, "db.example.com", cfg.Host)
		assert.Equal(t, 5432, cfg.Port)
		assert.Equal(t, 100, cfg.MaxConnections)
	})

	t.Run("RedisConfig", func(t *testing.T) {
		cfg := RedisConfig{
			Addresses: []string{"redis1:6379", "redis2:6379"},
			Password:  "redis_password",
			DB:        0,
			PoolSize:  100,
		}

		assert.Len(t, cfg.Addresses, 2)
		assert.Equal(t, 100, cfg.PoolSize)
	})

	t.Run("KafkaConfig", func(t *testing.T) {
		cfg := KafkaConfig{
			Brokers:  []string{"kafka1:9092", "kafka2:9092"},
			GroupID:  "eidos-chain",
			ClientID: "eidos-chain-1",
		}

		assert.Len(t, cfg.Brokers, 2)
		assert.Equal(t, "eidos-chain", cfg.GroupID)
	})

	t.Run("BlockchainConfig", func(t *testing.T) {
		cfg := BlockchainConfig{
			RPCURL:          "https://arb1.arbitrum.io/rpc",
			BackupRPCURLs:   []string{"https://arb1-backup.arbitrum.io/rpc"},
			ChainID:         42161,
			ContractAddress: "0x1234567890123456789012345678901234567890",
			PrivateKey:      "0x...",
			Confirmations: ConfirmationsConfig{
				Deposit:    0,
				Settlement: 0,
			},
		}

		assert.Equal(t, int64(42161), cfg.ChainID)
		assert.Equal(t, 0, cfg.Confirmations.Deposit)
	})

	t.Run("SettlementConfig", func(t *testing.T) {
		cfg := SettlementConfig{
			BatchSize:     100,
			BatchInterval: 10,
			MaxRetries:    5,
			RetryBackoff:  60,
		}

		assert.Equal(t, 100, cfg.BatchSize)
		assert.Equal(t, 5, cfg.MaxRetries)
	})
}
