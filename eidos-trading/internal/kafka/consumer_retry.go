// Package kafka provides Kafka consumer with retry capability
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// RetryableError 可重试的错误
type RetryableError struct {
	Err       error
	Retryable bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError 创建可重试错误
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, Retryable: true}
}

// NewNonRetryableError 创建不可重试错误
func NewNonRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, Retryable: false}
}

// IsRetryable 检查错误是否可重试
func IsRetryable(err error) bool {
	var retryErr *RetryableError
	if errors.As(err, &retryErr) {
		return retryErr.Retryable
	}
	// 默认可重试
	return true
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries     int           // 最大重试次数
	InitialBackoff time.Duration // 初始退避时间
	MaxBackoff     time.Duration // 最大退避时间
	BackoffFactor  float64       // 退避因子
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
	}
}

// DeadLetterMessage 死信消息
type DeadLetterMessage struct {
	OriginalTopic string `json:"original_topic"`
	Key           []byte `json:"key"`
	Value         []byte `json:"value"`
	Partition     int32  `json:"partition"`
	Offset        int64  `json:"offset"`
	RetryCount    int    `json:"retry_count"`
	LastError     string `json:"last_error"`
	FirstFailedAt int64  `json:"first_failed_at"`
	LastFailedAt  int64  `json:"last_failed_at"`
}

// RetryableHandler 带重试能力的消息处理器
type RetryableHandler interface {
	Handler
	// HandleWithRetry 处理消息，返回是否需要重试
	HandleWithRetry(ctx context.Context, msg *Message) error
}

// RetryConsumerGroup 带重试的消费者组
type RetryConsumerGroup struct {
	*ConsumerGroup
	brokers         []string
	retryConfig     *RetryConfig
	deadLetterTopic string
	producer        MessageProducer
	mu              sync.RWMutex
	retryHandlers   map[string]RetryableHandler
}

// MessageProducer Kafka 生产者接口 (用于发送死信)
type MessageProducer interface {
	SendWithContext(ctx context.Context, topic string, key, value []byte) error
}

// RetryConsumerConfig 带重试的消费者配置
type RetryConsumerConfig struct {
	ConsumerConfig
	RetryConfig     *RetryConfig
	DeadLetterTopic string          // 死信队列 topic
	Producer        MessageProducer // 用于发送死信
}

// NewRetryConsumerGroup 创建带重试的消费者组
func NewRetryConsumerGroup(cfg *RetryConsumerConfig) (*RetryConsumerGroup, error) {
	cg, err := NewConsumerGroup(&cfg.ConsumerConfig)
	if err != nil {
		return nil, err
	}

	retryConfig := cfg.RetryConfig
	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	deadLetterTopic := cfg.DeadLetterTopic
	if deadLetterTopic == "" {
		deadLetterTopic = TopicDeadLetter
	}

	return &RetryConsumerGroup{
		ConsumerGroup:   cg,
		brokers:         cfg.Brokers,
		retryConfig:     retryConfig,
		deadLetterTopic: deadLetterTopic,
		producer:        cfg.Producer,
		retryHandlers:   make(map[string]RetryableHandler),
	}, nil
}

// RegisterRetryHandler 注册可重试的处理器
func (c *RetryConsumerGroup) RegisterRetryHandler(topic string, handler RetryableHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retryHandlers[topic] = handler
	// 同时注册到基础 ConsumerGroup
	c.ConsumerGroup.RegisterHandler(topic, c.wrapHandler(topic, handler))
}

// wrapHandler 包装处理器添加重试逻辑
func (c *RetryConsumerGroup) wrapHandler(topic string, handler RetryableHandler) Handler {
	return HandlerFunc(func(ctx context.Context, msg *Message) error {
		return c.handleWithRetry(ctx, msg, handler)
	})
}

// handleWithRetry 带重试的消息处理
func (c *RetryConsumerGroup) handleWithRetry(ctx context.Context, msg *Message, handler RetryableHandler) error {
	var lastErr error
	startTime := time.Now()

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		// 处理消息
		err := handler.HandleWithRetry(ctx, msg)
		if err == nil {
			// 成功
			if attempt > 0 {
				logger.Info("message processed after retry",
					"topic", msg.Topic,
					"partition", msg.Partition,
					"offset", msg.Offset,
					"attempts", attempt+1)
				metrics.RecordKafkaRetry(msg.Topic, "success")
			}
			return nil
		}

		lastErr = err

		// 检查是否可重试
		if !IsRetryable(err) {
			logger.Warn("non-retryable error, sending to DLQ",
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
				"error", err)
			c.sendToDeadLetter(ctx, msg, attempt, err)
			metrics.RecordKafkaRetry(msg.Topic, "non_retryable")
			return nil // 返回 nil 以确认消息
		}

		// 最后一次尝试失败
		if attempt == c.retryConfig.MaxRetries {
			logger.Error("max retries exceeded, sending to DLQ",
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
				"max_retries", c.retryConfig.MaxRetries,
				"error", err)
			c.sendToDeadLetter(ctx, msg, attempt+1, err)
			metrics.RecordKafkaRetry(msg.Topic, "max_retries_exceeded")
			return nil // 返回 nil 以确认消息
		}

		// 计算退避时间
		backoff := c.calculateBackoff(attempt)
		logger.Warn("retrying message",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err)

		metrics.RecordKafkaRetry(msg.Topic, "retry")

		// 等待退避时间
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	// 不应该到达这里
	duration := time.Since(startTime)
	logger.Error("message processing failed after retries",
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"duration", duration,
		"error", lastErr)

	return lastErr
}

// calculateBackoff 计算退避时间
func (c *RetryConsumerGroup) calculateBackoff(attempt int) time.Duration {
	backoff := float64(c.retryConfig.InitialBackoff)
	for i := 0; i < attempt; i++ {
		backoff *= c.retryConfig.BackoffFactor
	}

	if backoff > float64(c.retryConfig.MaxBackoff) {
		backoff = float64(c.retryConfig.MaxBackoff)
	}

	return time.Duration(backoff)
}

// sendToDeadLetter 发送消息到死信队列
func (c *RetryConsumerGroup) sendToDeadLetter(ctx context.Context, msg *Message, retryCount int, lastErr error) {
	if c.producer == nil {
		logger.Warn("no producer available for DLQ, message dropped",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset)
		return
	}

	now := time.Now().UnixMilli()
	dlqMsg := &DeadLetterMessage{
		OriginalTopic: msg.Topic,
		Key:           msg.Key,
		Value:         msg.Value,
		Partition:     msg.Partition,
		Offset:        msg.Offset,
		RetryCount:    retryCount,
		LastError:     lastErr.Error(),
		FirstFailedAt: now,
		LastFailedAt:  now,
	}

	data, err := json.Marshal(dlqMsg)
	if err != nil {
		logger.Error("marshal DLQ message failed",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
			"error", err)
		return
	}

	if err := c.producer.SendWithContext(ctx, c.deadLetterTopic, msg.Key, data); err != nil {
		logger.Error("send to DLQ failed",
			"topic", msg.Topic,
			"dlq_topic", c.deadLetterTopic,
			"partition", msg.Partition,
			"offset", msg.Offset,
			"error", err)
		return
	}

	logger.Info("message sent to DLQ",
		"original_topic", msg.Topic,
		"dlq_topic", c.deadLetterTopic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"retry_count", retryCount)

	metrics.RecordKafkaDeadLetter(msg.Topic)
}

// RetryConsumerGroupWithDLQ 创建带死信队列的消费者组
// 简化版本，自动配置重试和死信队列
func NewRetryConsumerGroupWithDLQ(
	brokers []string,
	groupID string,
	topics []string,
	producer MessageProducer,
) (*RetryConsumerGroup, error) {
	cfg := &RetryConsumerConfig{
		ConsumerConfig: ConsumerConfig{
			Brokers:       brokers,
			GroupID:       groupID,
			Topics:        append(topics, TopicDeadLetter), // 同时订阅 DLQ
			InitialOffset: sarama.OffsetNewest,
		},
		RetryConfig:     DefaultRetryConfig(),
		DeadLetterTopic: TopicDeadLetter,
		Producer:        producer,
	}

	return NewRetryConsumerGroup(cfg)
}

// ProcessDeadLetterQueue 处理死信队列消息
// 返回处理成功的消息数量
func (c *RetryConsumerGroup) ProcessDeadLetterQueue(
	ctx context.Context,
	handler func(ctx context.Context, dlqMsg *DeadLetterMessage) error,
	limit int,
) (int, error) {
	logger.Info("processing dead letter queue", "limit", limit)

	// 创建单独的消费者来读取 DLQ
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumer(c.brokers, config)
	if err != nil {
		return 0, fmt.Errorf("create DLQ consumer: %w", err)
	}
	defer consumer.Close()

	// 获取 DLQ topic 的分区
	partitions, err := consumer.Partitions(c.deadLetterTopic)
	if err != nil {
		return 0, fmt.Errorf("get DLQ partitions: %w", err)
	}

	processed := 0
	for _, partition := range partitions {
		if processed >= limit {
			break
		}

		pc, err := consumer.ConsumePartition(c.deadLetterTopic, partition, sarama.OffsetOldest)
		if err != nil {
			logger.Warn("consume DLQ partition failed",
				"partition", partition,
				"error", err)
			continue
		}

		// 使用 timeout 来避免无限等待
		timeout := time.After(5 * time.Second)
	consumeLoop:
		for processed < limit {
			select {
			case <-ctx.Done():
				pc.Close()
				return processed, ctx.Err()
			case <-timeout:
				break consumeLoop
			case msg := <-pc.Messages():
				if msg == nil {
					break consumeLoop
				}

				var dlqMsg DeadLetterMessage
				if err := json.Unmarshal(msg.Value, &dlqMsg); err != nil {
					logger.Warn("unmarshal DLQ message failed",
						"offset", msg.Offset,
						"error", err)
					continue
				}

				if err := handler(ctx, &dlqMsg); err != nil {
					logger.Warn("process DLQ message failed",
						"original_topic", dlqMsg.OriginalTopic,
						"error", err)
					continue
				}

				processed++
				logger.Info("DLQ message processed successfully",
					"original_topic", dlqMsg.OriginalTopic,
					"retry_count", dlqMsg.RetryCount)
			}
		}
		pc.Close()
	}

	logger.Info("DLQ processing completed", "processed", processed)
	return processed, nil
}

// RetryableHandlerAdapter 将普通 Handler 适配为 RetryableHandler
type RetryableHandlerAdapter struct {
	handler Handler
}

// NewRetryableHandlerAdapter 创建适配器
func NewRetryableHandlerAdapter(handler Handler) RetryableHandler {
	return &RetryableHandlerAdapter{handler: handler}
}

// Handle 实现 Handler 接口
func (a *RetryableHandlerAdapter) Handle(ctx context.Context, msg *Message) error {
	return a.handler.Handle(ctx, msg)
}

// HandleWithRetry 实现 RetryableHandler 接口
func (a *RetryableHandlerAdapter) HandleWithRetry(ctx context.Context, msg *Message) error {
	return a.handler.Handle(ctx, msg)
}
