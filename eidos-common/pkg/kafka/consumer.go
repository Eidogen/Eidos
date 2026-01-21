package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// MessageHandler 消息处理函数
type MessageHandler func(ctx context.Context, msg *Message) error

// BatchMessageHandler 批量消息处理函数
type BatchMessageHandler func(ctx context.Context, msgs []*Message) error

// Consumer Kafka 消费者
type Consumer struct {
	config       *ConsumerConfig
	client       sarama.Client
	consumerGroup sarama.ConsumerGroup

	handler      MessageHandler
	batchHandler BatchMessageHandler

	// 状态
	started  int32
	closed   int32
	closeCh  chan struct{}
	closeWg  sync.WaitGroup

	// 生产者 (用于发送到重试/死信主题)
	retryProducer *Producer

	// 指标
	metrics *ConsumerMetrics
}

// ConsumerMetrics 消费者指标
type ConsumerMetrics struct {
	MessagesReceived   int64
	MessagesProcessed  int64
	MessagesFailed     int64
	MessagesRetried    int64
	MessagesDeadLetter int64
	BytesReceived      int64
	CommitCount        int64
	RebalanceCount     int64
	LastCommitTime     time.Time
	LastMessageTime    time.Time
	ProcessingLatency  int64 // 纳秒
}

// NewConsumer 创建消费者
func NewConsumer(cfg *ConsumerConfig) (*Consumer, error) {
	if cfg == nil {
		cfg = DefaultConsumerConfig()
	}

	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka brokers is required")
	}

	if cfg.GroupID == "" {
		return nil, errors.New("consumer group id is required")
	}

	saramaConfig, err := buildSaramaConsumerConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build sarama config failed: %w", err)
	}

	client, err := sarama.NewClient(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("create kafka client failed: %w", err)
	}

	consumerGroup, err := sarama.NewConsumerGroupFromClient(cfg.GroupID, client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create consumer group failed: %w", err)
	}

	c := &Consumer{
		config:        cfg,
		client:        client,
		consumerGroup: consumerGroup,
		closeCh:       make(chan struct{}),
		metrics:       &ConsumerMetrics{},
	}

	logger.Info("kafka consumer created",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("group_id", cfg.GroupID),
		zap.Strings("topics", cfg.Topics),
	)

	return c, nil
}

// buildSaramaConsumerConfig 构建 Sarama 消费者配置
func buildSaramaConsumerConfig(cfg *ConsumerConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()

	// 版本
	if cfg.Version != "" {
		version, err := sarama.ParseKafkaVersion(cfg.Version)
		if err != nil {
			return nil, fmt.Errorf("parse kafka version failed: %w", err)
		}
		saramaConfig.Version = version
	}

	// 客户端 ID
	if cfg.ClientID != "" {
		saramaConfig.ClientID = cfg.ClientID
	}

	// 初始 offset
	switch cfg.InitialOffset {
	case "oldest":
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	default:
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// 自动提交配置
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = cfg.AutoCommit.Enable
	saramaConfig.Consumer.Offsets.AutoCommit.Interval = cfg.AutoCommit.Interval

	// 会话配置
	saramaConfig.Consumer.Group.Session.Timeout = cfg.Session.Timeout
	saramaConfig.Consumer.Group.Heartbeat.Interval = cfg.Session.HeartbeatInterval

	// 拉取配置
	saramaConfig.Consumer.Fetch.Min = int32(cfg.Fetch.MinBytes)
	saramaConfig.Consumer.Fetch.Max = int32(cfg.Fetch.MaxBytes)
	saramaConfig.Consumer.MaxWaitTime = cfg.Fetch.MaxWaitTime

	// 消息处理时间
	saramaConfig.Consumer.MaxProcessingTime = cfg.MaxProcessingTime

	// 重平衡策略
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategySticky(),
	}

	// 返回错误
	saramaConfig.Consumer.Return.Errors = true

	// SASL 配置
	if cfg.SASL != nil && cfg.SASL.Enable {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = cfg.SASL.Username
		saramaConfig.Net.SASL.Password = cfg.SASL.Password

		switch cfg.SASL.Mechanism {
		case "SCRAM-SHA-256":
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &xdgScramClient{HashGeneratorFcn: SHA256}
			}
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		case "SCRAM-SHA-512":
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &xdgScramClient{HashGeneratorFcn: SHA512}
			}
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		default:
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	// TLS 配置
	if cfg.TLS != nil && cfg.TLS.Enable {
		tlsConfig, err := buildTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("build tls config failed: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	return saramaConfig, nil
}

// SetHandler 设置消息处理函数
func (c *Consumer) SetHandler(handler MessageHandler) {
	c.handler = handler
}

// SetBatchHandler 设置批量消息处理函数
func (c *Consumer) SetBatchHandler(handler BatchMessageHandler) {
	c.batchHandler = handler
}

// SetRetryProducer 设置重试生产者
func (c *Consumer) SetRetryProducer(producer *Producer) {
	c.retryProducer = producer
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.started, 0, 1) {
		return errors.New("consumer already started")
	}

	if c.handler == nil && c.batchHandler == nil {
		return errors.New("message handler is required")
	}

	if len(c.config.Topics) == 0 {
		return errors.New("topics is required")
	}

	logger.Info("starting kafka consumer",
		zap.Strings("topics", c.config.Topics),
		zap.String("group_id", c.config.GroupID),
	)

	// 启动多个并发消费者
	for i := 0; i < c.config.Concurrency; i++ {
		c.closeWg.Add(1)
		go c.consumeLoop(ctx, i)
	}

	// 启动错误处理循环
	c.closeWg.Add(1)
	go c.errorLoop()

	return nil
}

// consumeLoop 消费循环
func (c *Consumer) consumeLoop(ctx context.Context, workerID int) {
	defer c.closeWg.Done()

	handler := &consumerGroupHandler{
		consumer: c,
		workerID: workerID,
	}

	for {
		select {
		case <-c.closeCh:
			logger.Info("consumer loop stopped", zap.Int("worker_id", workerID))
			return
		case <-ctx.Done():
			logger.Info("consumer loop cancelled", zap.Int("worker_id", workerID))
			return
		default:
		}

		// 阻塞式消费
		if err := c.consumerGroup.Consume(ctx, c.config.Topics, handler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return
			}
			logger.Error("consume error",
				zap.Int("worker_id", workerID),
				zap.Error(err),
			)
		}
	}
}

// errorLoop 错误处理循环
func (c *Consumer) errorLoop() {
	defer c.closeWg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		case err, ok := <-c.consumerGroup.Errors():
			if !ok {
				return
			}
			logger.Error("consumer group error", zap.Error(err))
		}
	}
}

// consumerGroupHandler 实现 sarama.ConsumerGroupHandler 接口
type consumerGroupHandler struct {
	consumer *Consumer
	workerID int
}

// Setup 在新 session 开始时调用
func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	atomic.AddInt64(&h.consumer.metrics.RebalanceCount, 1)
	logger.Info("consumer group session setup",
		zap.Int("worker_id", h.workerID),
		zap.Int32("generation_id", session.GenerationID()),
		zap.String("member_id", session.MemberID()),
	)
	return nil
}

// Cleanup 在 session 结束时调用
func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	logger.Info("consumer group session cleanup",
		zap.Int("worker_id", h.workerID),
		zap.Int32("generation_id", session.GenerationID()),
	)
	return nil
}

// ConsumeClaim 消费分区消息
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	logger.Info("start consuming partition",
		zap.Int("worker_id", h.workerID),
		zap.String("topic", claim.Topic()),
		zap.Int32("partition", claim.Partition()),
		zap.Int64("initial_offset", claim.InitialOffset()),
	)

	for {
		select {
		case <-session.Context().Done():
			return nil
		case <-h.consumer.closeCh:
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			// 处理消息
			h.processMessage(session, msg)
		}
	}
}

// processMessage 处理单条消息
func (h *consumerGroupHandler) processMessage(session sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) {
	startTime := time.Now()

	atomic.AddInt64(&h.consumer.metrics.MessagesReceived, 1)
	atomic.AddInt64(&h.consumer.metrics.BytesReceived, int64(len(msg.Value)))
	h.consumer.metrics.LastMessageTime = time.Now()

	// 转换消息
	kafkaMsg := h.convertMessage(msg)

	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(session.Context(), h.consumer.config.MaxProcessingTime)
	defer cancel()

	// 处理消息
	var err error
	if h.consumer.handler != nil {
		err = h.consumer.handler(ctx, kafkaMsg)
	}

	// 记录处理延迟
	latency := time.Since(startTime).Nanoseconds()
	atomic.StoreInt64(&h.consumer.metrics.ProcessingLatency, latency)

	if err != nil {
		atomic.AddInt64(&h.consumer.metrics.MessagesFailed, 1)
		logger.Error("message processing failed",
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset),
			zap.Error(err),
		)

		// 重试处理
		h.handleRetry(ctx, kafkaMsg, err)
	} else {
		atomic.AddInt64(&h.consumer.metrics.MessagesProcessed, 1)
		logger.Debug("message processed",
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset),
			zap.Duration("latency", time.Duration(latency)),
		)
	}

	// 手动提交 offset (标记消息已处理)
	session.MarkMessage(msg, "")
	atomic.AddInt64(&h.consumer.metrics.CommitCount, 1)
	h.consumer.metrics.LastCommitTime = time.Now()
}

// handleRetry 处理消息重试
func (h *consumerGroupHandler) handleRetry(ctx context.Context, msg *Message, processErr error) {
	if h.consumer.retryProducer == nil {
		return
	}

	msg.RetryCount++

	if msg.OriginalTopic == "" {
		msg.OriginalTopic = msg.Topic
	}

	// 达到最大重试次数，发送到死信主题
	if msg.RetryCount > h.consumer.config.MaxRetries {
		if h.consumer.config.DeadLetterTopic != "" {
			deadLetterMsg := msg.Clone()
			deadLetterMsg.Topic = h.consumer.config.DeadLetterTopic
			deadLetterMsg.Headers = append(deadLetterMsg.Headers, Header{
				Key:   "error",
				Value: []byte(processErr.Error()),
			})

			if _, err := h.consumer.retryProducer.Send(ctx, deadLetterMsg); err != nil {
				logger.Error("send to dead letter topic failed",
					zap.String("topic", h.consumer.config.DeadLetterTopic),
					zap.Error(err),
				)
			} else {
				atomic.AddInt64(&h.consumer.metrics.MessagesDeadLetter, 1)
				logger.Warn("message sent to dead letter topic",
					zap.String("original_topic", msg.OriginalTopic),
					zap.Int("retry_count", msg.RetryCount),
				)
			}
		}
		return
	}

	// 发送到重试主题
	if h.consumer.config.RetryTopic != "" {
		retryMsg := msg.Clone()
		retryMsg.Topic = h.consumer.config.RetryTopic
		retryMsg.Headers = append(retryMsg.Headers,
			Header{Key: "retry_count", Value: []byte(fmt.Sprintf("%d", msg.RetryCount))},
			Header{Key: "original_topic", Value: []byte(msg.OriginalTopic)},
			Header{Key: "error", Value: []byte(processErr.Error())},
		)

		if _, err := h.consumer.retryProducer.Send(ctx, retryMsg); err != nil {
			logger.Error("send to retry topic failed",
				zap.String("topic", h.consumer.config.RetryTopic),
				zap.Error(err),
			)
		} else {
			atomic.AddInt64(&h.consumer.metrics.MessagesRetried, 1)
			logger.Info("message sent to retry topic",
				zap.String("original_topic", msg.OriginalTopic),
				zap.Int("retry_count", msg.RetryCount),
			)
		}
	}
}

// convertMessage 转换 Sarama 消息
func (h *consumerGroupHandler) convertMessage(msg *sarama.ConsumerMessage) *Message {
	kafkaMsg := &Message{
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Key:       msg.Key,
		Value:     msg.Value,
		Timestamp: msg.Timestamp,
		Headers:   make([]Header, len(msg.Headers)),
	}

	for i, h := range msg.Headers {
		kafkaMsg.Headers[i] = Header{
			Key:   string(h.Key),
			Value: h.Value,
		}
	}

	// 提取 trace_id
	if traceID, ok := kafkaMsg.GetHeaderString("trace_id"); ok {
		kafkaMsg.TraceID = traceID
	}

	// 提取重试信息
	if retryCount, ok := kafkaMsg.GetHeaderString("retry_count"); ok {
		fmt.Sscanf(retryCount, "%d", &kafkaMsg.RetryCount)
	}
	if originalTopic, ok := kafkaMsg.GetHeaderString("original_topic"); ok {
		kafkaMsg.OriginalTopic = originalTopic
	}

	return kafkaMsg
}

// Metrics 获取指标
func (c *Consumer) Metrics() *ConsumerMetrics {
	return &ConsumerMetrics{
		MessagesReceived:   atomic.LoadInt64(&c.metrics.MessagesReceived),
		MessagesProcessed:  atomic.LoadInt64(&c.metrics.MessagesProcessed),
		MessagesFailed:     atomic.LoadInt64(&c.metrics.MessagesFailed),
		MessagesRetried:    atomic.LoadInt64(&c.metrics.MessagesRetried),
		MessagesDeadLetter: atomic.LoadInt64(&c.metrics.MessagesDeadLetter),
		BytesReceived:      atomic.LoadInt64(&c.metrics.BytesReceived),
		CommitCount:        atomic.LoadInt64(&c.metrics.CommitCount),
		RebalanceCount:     atomic.LoadInt64(&c.metrics.RebalanceCount),
		LastCommitTime:     c.metrics.LastCommitTime,
		LastMessageTime:    c.metrics.LastMessageTime,
		ProcessingLatency:  atomic.LoadInt64(&c.metrics.ProcessingLatency),
	}
}

// Pause 暂停消费指定主题分区
func (c *Consumer) Pause(topicPartitions map[string][]int32) {
	c.consumerGroup.Pause(topicPartitions)
	logger.Info("consumer paused", zap.Any("topic_partitions", topicPartitions))
}

// Resume 恢复消费指定主题分区
func (c *Consumer) Resume(topicPartitions map[string][]int32) {
	c.consumerGroup.Resume(topicPartitions)
	logger.Info("consumer resumed", zap.Any("topic_partitions", topicPartitions))
}

// PauseAll 暂停所有消费
func (c *Consumer) PauseAll() {
	topicPartitions := make(map[string][]int32)
	for _, topic := range c.config.Topics {
		partitions, err := c.client.Partitions(topic)
		if err != nil {
			logger.Error("get partitions failed", zap.String("topic", topic), zap.Error(err))
			continue
		}
		topicPartitions[topic] = partitions
	}
	c.Pause(topicPartitions)
}

// ResumeAll 恢复所有消费
func (c *Consumer) ResumeAll() {
	topicPartitions := make(map[string][]int32)
	for _, topic := range c.config.Topics {
		partitions, err := c.client.Partitions(topic)
		if err != nil {
			logger.Error("get partitions failed", zap.String("topic", topic), zap.Error(err))
			continue
		}
		topicPartitions[topic] = partitions
	}
	c.Resume(topicPartitions)
}

// Close 优雅关闭消费者
func (c *Consumer) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	logger.Info("closing kafka consumer")

	// 发送关闭信号
	close(c.closeCh)

	// 等待消费循环退出
	done := make(chan struct{})
	go func() {
		c.closeWg.Wait()
		close(done)
	}()

	// 等待最多 30 秒
	select {
	case <-done:
		logger.Info("consumer loops stopped gracefully")
	case <-time.After(30 * time.Second):
		logger.Warn("consumer loops stop timeout")
	}

	// 关闭消费者组
	var errs []error
	if err := c.consumerGroup.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close consumer group: %w", err))
	}

	if err := c.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close client: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("close consumer errors: %v", errs)
	}

	logger.Info("kafka consumer closed",
		zap.Int64("messages_received", c.metrics.MessagesReceived),
		zap.Int64("messages_processed", c.metrics.MessagesProcessed),
		zap.Int64("messages_failed", c.metrics.MessagesFailed),
	)

	return nil
}

// IsClosed 检查是否已关闭
func (c *Consumer) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}
