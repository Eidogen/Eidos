// Package integration provides integration tests for the Eidos trading system.
// These tests verify the complete data flow across all services.
package integration

import (
	"os"
	"time"
)

// TestConfig holds configuration for integration tests.
type TestConfig struct {
	// Service endpoints
	TradingServiceAddr  string
	MatchingServiceAddr string
	MarketServiceAddr   string
	ChainServiceAddr    string
	RiskServiceAddr     string
	APIServiceAddr      string

	// Infrastructure
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string

	TimescaleDBHost     string
	TimescaleDBPort     string
	TimescaleDBUser     string
	TimescaleDBPassword string
	TimescaleDBDB       string

	RedisHost     string
	RedisPort     string
	RedisPassword string

	KafkaBrokers []string

	// Test settings
	Timeout         time.Duration
	RetryInterval   time.Duration
	MaxRetries      int
	TestWalletAddr  string
	TestMarket      string
	TestBaseToken   string
	TestQuoteToken  string
}

// DefaultTestConfig returns the default test configuration.
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		// Service endpoints
		TradingServiceAddr:  getEnv("TRADING_SERVICE_ADDR", "localhost:50051"),
		MatchingServiceAddr: getEnv("MATCHING_SERVICE_ADDR", "localhost:50052"),
		MarketServiceAddr:   getEnv("MARKET_SERVICE_ADDR", "localhost:50053"),
		ChainServiceAddr:    getEnv("CHAIN_SERVICE_ADDR", "localhost:50054"),
		RiskServiceAddr:     getEnv("RISK_SERVICE_ADDR", "localhost:50055"),
		APIServiceAddr:      getEnv("API_SERVICE_ADDR", "localhost:8080"),

		// PostgreSQL
		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "eidos"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "eidos123"),
		PostgresDB:       getEnv("POSTGRES_DB", "eidos"),

		// TimescaleDB
		TimescaleDBHost:     getEnv("TIMESCALEDB_HOST", "localhost"),
		TimescaleDBPort:     getEnv("TIMESCALEDB_PORT", "5433"),
		TimescaleDBUser:     getEnv("TIMESCALEDB_USER", "eidos"),
		TimescaleDBPassword: getEnv("TIMESCALEDB_PASSWORD", "eidos123"),
		TimescaleDBDB:       getEnv("TIMESCALEDB_DB", "eidos_market"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// Kafka
		KafkaBrokers: []string{getEnv("KAFKA_BROKERS", "localhost:29092")},

		// Test settings
		Timeout:         30 * time.Second,
		RetryInterval:   500 * time.Millisecond,
		MaxRetries:      60,
		TestWalletAddr:  "0x742d35Cc6634C0532925a3b844Bc9e7595f1aB34",
		TestMarket:      "ETH-USDT",
		TestBaseToken:   "ETH",
		TestQuoteToken:  "USDT",
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
