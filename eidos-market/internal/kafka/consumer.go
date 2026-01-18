package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
)

// MessageHandler 消息处理接口
type MessageHandler interface {
	Handle(ctx context.Context, key, value []byte) error
	Topic() string
}

// ConsumerConfig 消费者配置
// TODO [Kafka]: 生产环境配置要点:
//   - GroupID 应使用 "eidos-market-consumer" (所有实例共享，实现水平扩展)
//   - Topics: ["trade-results", "orderbook-updates"] (来自 eidos-matching)
//   - 确保 Kafka 分区数 >= eidos-market 实例数，分区键为 market 字段
type ConsumerConfig struct {
	Brokers   []string
	GroupID   string
	ClientID  string
	Topics    []string
	MaxRetry  int  // 消息处理最大重试次数，默认 3
	CommitOnError bool // 处理失败时是否提交 offset，默认 true（跳过失败消息）
}

// Consumer Kafka 消费者
type Consumer struct {
	config   ConsumerConfig
	client   sarama.ConsumerGroup
	handlers map[string]MessageHandler
	logger   *zap.Logger

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// 统计指标
	processedCount atomic.Int64
	errorCount     atomic.Int64
	retryCount     atomic.Int64
}

// NewConsumer 创建 Kafka 消费者
// TODO [eidos-matching]: 确保 eidos-matching 使用以下分区策略:
//   - 分区键: 使用 market 字段 (如 "BTC-USDC") 作为 Kafka 消息的 Key
//   - 这样同一个交易对的所有消息会路由到同一个 eidos-market 实例
//   - 保证单个交易对的消息顺序性
func NewConsumer(config ConsumerConfig, logger *zap.Logger) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V3_0_0_0
	// 使用 RoundRobin 策略，配合按 market 分区，实现市场级别的水平扩展
	saramaConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaConfig.ClientID = config.ClientID

	client, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, saramaConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Consumer{
		config:   config,
		client:   client,
		handlers: make(map[string]MessageHandler),
		logger:   logger.Named("kafka_consumer"),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// RegisterHandler 注册消息处理器
func (c *Consumer) RegisterHandler(handler MessageHandler) {
	c.handlers[handler.Topic()] = handler
	c.logger.Info("handler registered", zap.String("topic", handler.Topic()))
}

// Start 启动消费
func (c *Consumer) Start() error {
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}

	if len(topics) == 0 {
		c.logger.Warn("no handlers registered, skipping consumer start")
		return nil
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				if err := c.client.Consume(c.ctx, topics, c); err != nil {
					c.logger.Error("consumer error", zap.Error(err))
				}
			}
		}
	}()

	c.logger.Info("consumer started", zap.Strings("topics", topics))
	return nil
}

// Stop 停止消费
func (c *Consumer) Stop() error {
	var err error
	c.closeOnce.Do(func() {
		c.cancel()
		c.wg.Wait()
		err = c.client.Close()
		c.logger.Info("consumer stopped")
	})
	return err
}

// Setup 实现 sarama.ConsumerGroupHandler
func (c *Consumer) Setup(session sarama.ConsumerGroupSession) error {
	c.logger.Info("consumer session setup",
		zap.Int32("generation", session.GenerationID()),
		zap.String("member", session.MemberID()))
	return nil
}

// Cleanup 实现 sarama.ConsumerGroupHandler
func (c *Consumer) Cleanup(session sarama.ConsumerGroupSession) error {
	c.logger.Info("consumer session cleanup",
		zap.Int32("generation", session.GenerationID()))
	return nil
}

// ConsumeClaim 实现 sarama.ConsumerGroupHandler
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	handler, ok := c.handlers[claim.Topic()]
	if !ok {
		c.logger.Warn("no handler for topic", zap.String("topic", claim.Topic()))
		return nil
	}

	maxRetry := c.config.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			startTime := time.Now()
			var lastErr error
			for attempt := 1; attempt <= maxRetry; attempt++ {
				if err := handler.Handle(c.ctx, msg.Key, msg.Value); err != nil {
					lastErr = err
					c.retryCount.Add(1)
					metrics.KafkaConsumerRetries.WithLabelValues(msg.Topic).Inc()
					if attempt < maxRetry {
						c.logger.Warn("message handling failed, retrying",
							zap.String("topic", msg.Topic),
							zap.Int32("partition", msg.Partition),
							zap.Int64("offset", msg.Offset),
							zap.Int("attempt", attempt),
							zap.Int("max_retry", maxRetry),
							zap.Error(err))
						// 指数退避重试
						time.Sleep(time.Duration(attempt*100) * time.Millisecond)
						continue
					}
				} else {
					lastErr = nil
					break
				}
			}

			if lastErr != nil {
				c.errorCount.Add(1)
				metrics.KafkaConsumerErrors.WithLabelValues(msg.Topic, "handler_error").Inc()
				c.logger.Error("message handling failed after retries",
					zap.String("topic", msg.Topic),
					zap.Int32("partition", msg.Partition),
					zap.Int64("offset", msg.Offset),
					zap.Int("retries", maxRetry),
					zap.Error(lastErr))
				// 根据配置决定是否提交 offset
				if !c.config.CommitOnError {
					// 不提交 offset，消息将在重启后重新消费
					// 警告：可能导致消息堆积，生产环境慎用
					continue
				}
			}

			c.processedCount.Add(1)
			metrics.KafkaMessagesConsumed.WithLabelValues(msg.Topic).Inc()
			metrics.KafkaMessageProcessingDuration.WithLabelValues(msg.Topic).Observe(time.Since(startTime).Seconds())
			session.MarkMessage(msg, "")

		case <-c.ctx.Done():
			return nil
		}
	}
}

// Stats 返回消费者统计信息
func (c *Consumer) Stats() map[string]int64 {
	return map[string]int64{
		"processed_count": c.processedCount.Load(),
		"error_count":     c.errorCount.Load(),
		"retry_count":     c.retryCount.Load(),
	}
}
