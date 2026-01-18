// Package kafka Kafka 生产者单元测试
package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewProducer(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:               []string{"localhost:9092"},
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
		BatchSize:             100,
		BatchTimeout:          10 * time.Millisecond,
		Compression:           "gzip",
		RequiredAcks:          -1,
	}

	p := NewProducer(cfg)

	assert.NotNil(t, p)
	assert.NotNil(t, p.tradesWriter)
	assert.NotNil(t, p.cancelsWriter)
	assert.NotNil(t, p.updatesWriter)
	assert.Equal(t, cfg, p.config)
}

func TestNewProducer_CompressionOptions(t *testing.T) {
	tests := []struct {
		name        string
		compression string
	}{
		{"no compression", "none"},
		{"gzip", "gzip"},
		{"snappy", "snappy"},
		{"lz4", "lz4"},
		{"unknown defaults to none", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ProducerConfig{
				Brokers:               []string{"localhost:9092"},
				TradeResultsTopic:     "trade-results",
				OrderCancelledTopic:   "order-cancelled",
				OrderbookUpdatesTopic: "orderbook-updates",
				Compression:           tt.compression,
			}

			p := NewProducer(cfg)
			assert.NotNil(t, p)
		})
	}
}

func TestNewProducer_RequiredAcksOptions(t *testing.T) {
	tests := []struct {
		name         string
		requiredAcks int
	}{
		{"require none", 0},
		{"require one", 1},
		{"require all", -1},
		{"default to all", 99}, // unknown value defaults to all
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ProducerConfig{
				Brokers:               []string{"localhost:9092"},
				TradeResultsTopic:     "trade-results",
				OrderCancelledTopic:   "order-cancelled",
				OrderbookUpdatesTopic: "orderbook-updates",
				RequiredAcks:          tt.requiredAcks,
			}

			p := NewProducer(cfg)
			assert.NotNil(t, p)
		})
	}
}

func TestProducer_GetStats(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:               []string{"localhost:9092"},
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
	}

	p := NewProducer(cfg)

	// 初始状态
	stats := p.GetStats()
	assert.Equal(t, int64(0), stats.MessagesSent)
	assert.Equal(t, int64(0), stats.ErrorsCount)
}

func TestProducer_IncrementStats(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:               []string{"localhost:9092"},
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
	}

	p := NewProducer(cfg)

	// 手动调用内部方法测试
	p.incrementSent()
	p.incrementSent()
	p.incrementErrors()

	stats := p.GetStats()
	assert.Equal(t, int64(2), stats.MessagesSent)
	assert.Equal(t, int64(1), stats.ErrorsCount)
}

func TestProducerStats_Fields(t *testing.T) {
	stats := ProducerStats{
		MessagesSent: 100,
		ErrorsCount:  5,
	}

	assert.Equal(t, int64(100), stats.MessagesSent)
	assert.Equal(t, int64(5), stats.ErrorsCount)
}

func TestProducerConfig_Fields(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:               []string{"broker1:9092", "broker2:9092"},
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
		BatchSize:             100,
		BatchTimeout:          50 * time.Millisecond,
		Compression:           "snappy",
		RequiredAcks:          1,
	}

	assert.Equal(t, 2, len(cfg.Brokers))
	assert.Equal(t, "trade-results", cfg.TradeResultsTopic)
	assert.Equal(t, "order-cancelled", cfg.OrderCancelledTopic)
	assert.Equal(t, "orderbook-updates", cfg.OrderbookUpdatesTopic)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 50*time.Millisecond, cfg.BatchTimeout)
	assert.Equal(t, "snappy", cfg.Compression)
	assert.Equal(t, 1, cfg.RequiredAcks)
}

func TestProducer_IncrementStats_Concurrent(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:               []string{"localhost:9092"},
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
	}

	p := NewProducer(cfg)

	// 并发测试
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				p.incrementSent()
			}
			done <- struct{}{}
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := p.GetStats()
	assert.Equal(t, int64(1000), stats.MessagesSent)
}
