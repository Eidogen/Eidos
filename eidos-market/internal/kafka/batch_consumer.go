// Package kafka provides Kafka consumer implementations.
package kafka

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
)

// BatchMessageHandler batch message processing interface
type BatchMessageHandler interface {
	HandleBatch(ctx context.Context, messages []*sarama.ConsumerMessage) error
	Topic() string
}

// BatchConsumerConfig batch consumer configuration
type BatchConsumerConfig struct {
	Brokers []string
	GroupID string
	// Batch processing configuration
	BatchSize          int           // Maximum batch size
	BatchTimeout       time.Duration // Maximum time to wait for batch to fill
	MaxConcurrency     int           // Maximum concurrent batch processors
	ProcessingTimeout  time.Duration // Timeout for processing a single batch
	EnableParallel     bool          // Enable parallel message processing within a batch
	WorkerPoolSize     int           // Number of workers for parallel processing
	CommitInterval     time.Duration // Interval for offset commits
	MaxRetries         int           // Maximum retries for failed batches
	RetryBackoff       time.Duration // Backoff between retries
}

// DefaultBatchConsumerConfig returns default batch consumer configuration
func DefaultBatchConsumerConfig() BatchConsumerConfig {
	return BatchConsumerConfig{
		BatchSize:          1000,
		BatchTimeout:       100 * time.Millisecond,
		MaxConcurrency:     4,
		ProcessingTimeout:  10 * time.Second,
		EnableParallel:     true,
		WorkerPoolSize:     8,
		CommitInterval:     time.Second,
		MaxRetries:         3,
		RetryBackoff:       100 * time.Millisecond,
	}
}

// BatchConsumer Kafka batch consumer with high throughput design
type BatchConsumer struct {
	config   BatchConsumerConfig
	client   sarama.ConsumerGroup
	handlers map[string]BatchMessageHandler
	logger   *slog.Logger

	// Batch accumulator
	batchMu       sync.Mutex
	batches       map[string][]*sarama.ConsumerMessage // topic -> messages
	batchSessions map[string]sarama.ConsumerGroupSession

	// Worker pool for parallel processing
	workerPool chan struct{}

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Statistics
	messagesProcessed atomic.Int64
	batchesProcessed  atomic.Int64
	errorsCount       atomic.Int64
}

// NewBatchConsumer creates a new batch consumer
func NewBatchConsumer(config BatchConsumerConfig, logger *slog.Logger) (*BatchConsumer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V3_0_0_0
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest

	// Optimizations for high throughput
	saramaConfig.Consumer.Fetch.Min = 1024 * 10           // 10KB minimum fetch
	saramaConfig.Consumer.Fetch.Max = 1024 * 1024 * 10    // 10MB maximum fetch
	saramaConfig.Consumer.MaxWaitTime = 100 * time.Millisecond
	saramaConfig.Consumer.MaxProcessingTime = 10 * time.Second
	saramaConfig.ChannelBufferSize = 10000

	client, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, saramaConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	workerPool := make(chan struct{}, config.WorkerPoolSize)
	for i := 0; i < config.WorkerPoolSize; i++ {
		workerPool <- struct{}{}
	}

	return &BatchConsumer{
		config:        config,
		client:        client,
		handlers:      make(map[string]BatchMessageHandler),
		logger:        logger.With("component", "batch_consumer"),
		batches:       make(map[string][]*sarama.ConsumerMessage),
		batchSessions: make(map[string]sarama.ConsumerGroupSession),
		workerPool:    workerPool,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// RegisterHandler registers a batch message handler
func (c *BatchConsumer) RegisterHandler(handler BatchMessageHandler) {
	c.handlers[handler.Topic()] = handler
	c.batches[handler.Topic()] = make([]*sarama.ConsumerMessage, 0, c.config.BatchSize)
	c.logger.Info("batch handler registered", "topic", handler.Topic())
}

// Start starts consuming messages
func (c *BatchConsumer) Start() error {
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}

	if len(topics) == 0 {
		c.logger.Warn("no handlers registered, skipping consumer start")
		return nil
	}

	// Start batch flusher
	c.wg.Add(1)
	go c.batchFlushLoop()

	// Start consumer
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				if err := c.client.Consume(c.ctx, topics, c); err != nil {
					c.logger.Error("consumer error", "error", err)
				}
			}
		}
	}()

	c.logger.Info("batch consumer started",
		"topics", topics,
		"batch_size", c.config.BatchSize,
		"batch_timeout", c.config.BatchTimeout)

	return nil
}

// Stop stops the consumer
func (c *BatchConsumer) Stop() error {
	var err error
	c.closeOnce.Do(func() {
		c.cancel()

		// Flush remaining batches
		c.flushAllBatches()

		c.wg.Wait()
		err = c.client.Close()
		c.logger.Info("batch consumer stopped",
			"messages", c.messagesProcessed.Load(),
			"batches", c.batchesProcessed.Load(),
			"errors", c.errorsCount.Load())
	})
	return err
}

// Setup implements sarama.ConsumerGroupHandler
func (c *BatchConsumer) Setup(session sarama.ConsumerGroupSession) error {
	c.logger.Info("consumer session setup",
		"generation", session.GenerationID(),
		"member", session.MemberID())
	return nil
}

// Cleanup implements sarama.ConsumerGroupHandler
func (c *BatchConsumer) Cleanup(session sarama.ConsumerGroupSession) error {
	// Flush remaining batches before cleanup
	c.flushAllBatches()
	c.logger.Info("consumer session cleanup",
		"generation", session.GenerationID())
	return nil
}

// ConsumeClaim implements sarama.ConsumerGroupHandler
func (c *BatchConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	topic := claim.Topic()

	_, ok := c.handlers[topic]
	if !ok {
		c.logger.Warn("no handler for topic", "topic", topic)
		return nil
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			c.addToBatch(topic, msg, session)

		case <-c.ctx.Done():
			return nil
		}
	}
}

// addToBatch adds a message to the batch
func (c *BatchConsumer) addToBatch(topic string, msg *sarama.ConsumerMessage, session sarama.ConsumerGroupSession) {
	c.batchMu.Lock()
	defer c.batchMu.Unlock()

	c.batches[topic] = append(c.batches[topic], msg)
	c.batchSessions[topic] = session

	// Check if batch is full
	if len(c.batches[topic]) >= c.config.BatchSize {
		c.processBatchLocked(topic)
	}
}

// batchFlushLoop periodically flushes batches
func (c *BatchConsumer) batchFlushLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.flushAllBatches()
		case <-c.ctx.Done():
			return
		}
	}
}

// flushAllBatches flushes all pending batches
func (c *BatchConsumer) flushAllBatches() {
	c.batchMu.Lock()
	defer c.batchMu.Unlock()

	for topic := range c.batches {
		if len(c.batches[topic]) > 0 {
			c.processBatchLocked(topic)
		}
	}
}

// processBatchLocked processes a batch (must be called with lock held)
func (c *BatchConsumer) processBatchLocked(topic string) {
	messages := c.batches[topic]
	session := c.batchSessions[topic]

	if len(messages) == 0 {
		return
	}

	// Reset batch
	c.batches[topic] = make([]*sarama.ConsumerMessage, 0, c.config.BatchSize)

	// Process batch asynchronously
	c.wg.Add(1)
	go c.processBatch(topic, messages, session)
}

// processBatch processes a batch of messages
func (c *BatchConsumer) processBatch(topic string, messages []*sarama.ConsumerMessage, session sarama.ConsumerGroupSession) {
	defer c.wg.Done()

	handler := c.handlers[topic]
	if handler == nil {
		return
	}

	startTime := time.Now()

	// Get worker from pool
	select {
	case <-c.workerPool:
		defer func() { c.workerPool <- struct{}{} }()
	case <-c.ctx.Done():
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, c.config.ProcessingTimeout)
	defer cancel()

	// Process with retry
	var lastErr error
	for attempt := 1; attempt <= c.config.MaxRetries; attempt++ {
		if err := handler.HandleBatch(ctx, messages); err != nil {
			lastErr = err
			c.errorsCount.Add(1)
			metrics.KafkaConsumerErrors.WithLabelValues(topic, "batch_error").Inc()
			c.logger.Warn("batch processing failed, retrying",
				"topic", topic,
				"batch_size", len(messages),
				"attempt", attempt,
				"error", err)

			if attempt < c.config.MaxRetries {
				time.Sleep(c.config.RetryBackoff * time.Duration(attempt))
				continue
			}
		} else {
			lastErr = nil
			break
		}
	}

	if lastErr != nil {
		c.logger.Error("batch processing failed after retries",
			"topic", topic,
			"batch_size", len(messages),
			"error", lastErr)
	}

	// Mark messages as consumed
	if session != nil {
		for _, msg := range messages {
			session.MarkMessage(msg, "")
		}
	}

	c.messagesProcessed.Add(int64(len(messages)))
	c.batchesProcessed.Add(1)
	metrics.KafkaMessagesConsumed.WithLabelValues(topic).Add(float64(len(messages)))
	metrics.KafkaMessageProcessingDuration.WithLabelValues(topic).Observe(time.Since(startTime).Seconds())
}

// Stats returns consumer statistics
func (c *BatchConsumer) Stats() map[string]int64 {
	return map[string]int64{
		"messages_processed": c.messagesProcessed.Load(),
		"batches_processed":  c.batchesProcessed.Load(),
		"errors_count":       c.errorsCount.Load(),
	}
}
