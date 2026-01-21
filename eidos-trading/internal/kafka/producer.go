// Package kafka 提供 Kafka 生产者和消费者
package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Producer Kafka 异步生产者
type Producer struct {
	producer sarama.AsyncProducer
	wg       sync.WaitGroup
	closed   bool
	mu       sync.RWMutex
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Brokers       []string
	RequiredAcks  sarama.RequiredAcks // 默认 WaitForAll
	MaxRetry      int                 // 默认 3
	RetryBackoff  time.Duration       // 默认 100ms
	FlushMessages int                 // 批量发送消息数，默认 100
	FlushBytes    int                 // 批量发送字节数，默认 1MB
	FlushFreq     time.Duration       // 批量发送间隔，默认 10ms
}

// DefaultProducerConfig 返回默认生产者配置
func DefaultProducerConfig(brokers []string) *ProducerConfig {
	return &ProducerConfig{
		Brokers:       brokers,
		RequiredAcks:  sarama.WaitForAll,
		MaxRetry:      3,
		RetryBackoff:  100 * time.Millisecond,
		FlushMessages: 100,
		FlushBytes:    1024 * 1024, // 1MB
		FlushFreq:     10 * time.Millisecond,
	}
}

// NewProducer 创建异步生产者
func NewProducer(cfg *ProducerConfig) (*Producer, error) {
	config := sarama.NewConfig()

	// 生产者配置
	config.Producer.RequiredAcks = cfg.RequiredAcks
	config.Producer.Retry.Max = cfg.MaxRetry
	config.Producer.Retry.Backoff = cfg.RetryBackoff
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	// 批量配置
	config.Producer.Flush.Messages = cfg.FlushMessages
	config.Producer.Flush.Bytes = cfg.FlushBytes
	config.Producer.Flush.Frequency = cfg.FlushFreq

	// 压缩配置
	config.Producer.Compression = sarama.CompressionSnappy

	// 幂等性配置 (需要 Kafka 0.11+)
	config.Producer.Idempotent = true
	config.Net.MaxOpenRequests = 1 // 幂等性要求

	producer, err := sarama.NewAsyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer failed: %w", err)
	}

	p := &Producer{
		producer: producer,
	}

	// 启动成功/错误处理协程
	p.wg.Add(2)
	go p.handleSuccesses()
	go p.handleErrors()

	logger.Info("kafka producer started",
		"brokers", cfg.Brokers,
	)

	return p, nil
}

// Send 异步发送消息
func (p *Producer) Send(topic string, key, value []byte) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	if key != nil {
		msg.Key = sarama.ByteEncoder(key)
	}

	p.producer.Input() <- msg
	return nil
}

// SendWithContext 异步发送消息（支持 context）
func (p *Producer) SendWithContext(ctx context.Context, topic string, key, value []byte) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	if key != nil {
		msg.Key = sarama.ByteEncoder(key)
	}

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleSuccesses 处理成功的消息
func (p *Producer) handleSuccesses() {
	defer p.wg.Done()

	for msg := range p.producer.Successes() {
		logger.Debug("kafka message sent",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
		)
	}
}

// handleErrors 处理失败的消息
func (p *Producer) handleErrors() {
	defer p.wg.Done()

	for err := range p.producer.Errors() {
		logger.Error("kafka message send failed",
			"topic", err.Msg.Topic,
			"error", err.Err,
		)
		// 失败消息处理见 consumer_retry.go 的 DLQ 机制
	}
}

// Close 关闭生产者
func (p *Producer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// 关闭生产者会关闭 Successes 和 Errors channel
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close kafka producer failed: %w", err)
	}

	// 等待处理协程退出
	p.wg.Wait()

	logger.Info("kafka producer closed")
	return nil
}
