package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Handler 消息处理器接口
type Handler interface {
	// Handle 处理消息，返回 error 表示处理失败（消息不会被确认）
	Handle(ctx context.Context, msg *Message) error
}

// HandlerFunc 函数类型的处理器
type HandlerFunc func(ctx context.Context, msg *Message) error

// Handle 实现 Handler 接口
func (f HandlerFunc) Handle(ctx context.Context, msg *Message) error {
	return f(ctx, msg)
}

// ConsumerGroup Kafka 消费者组
type ConsumerGroup struct {
	client   sarama.ConsumerGroup
	handlers map[string]Handler
	topics   []string
	ready    chan struct{}
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers []string
	GroupID string
	Topics  []string
	// OffsetNewest 从最新消息开始消费
	// OffsetOldest 从最早消息开始消费
	InitialOffset int64
}

// NewConsumerGroup 创建消费者组
func NewConsumerGroup(cfg *ConsumerConfig) (*ConsumerGroup, error) {
	config := sarama.NewConfig()

	// 消费者配置
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = cfg.InitialOffset
	if cfg.InitialOffset == 0 {
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// 手动提交 offset
	config.Consumer.Offsets.AutoCommit.Enable = true

	client, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, config)
	if err != nil {
		return nil, fmt.Errorf("create consumer group failed: %w", err)
	}

	c := &ConsumerGroup{
		client:   client,
		handlers: make(map[string]Handler),
		topics:   cfg.Topics,
		ready:    make(chan struct{}),
	}

	logger.Info("kafka consumer group created",
		"brokers", cfg.Brokers,
		"group_id", cfg.GroupID,
		"topics", cfg.Topics,
	)

	return c, nil
}

// RegisterHandler 注册 topic 处理器
func (c *ConsumerGroup) RegisterHandler(topic string, handler Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = handler
}

// Start 启动消费
func (c *ConsumerGroup) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		for {
			// Consume 会在重平衡时返回，需要循环调用
			if err := c.client.Consume(ctx, c.topics, c); err != nil {
				if err == sarama.ErrClosedConsumerGroup {
					return
				}
				logger.Error("consumer error", "error", err)
			}

			// 检查是否需要退出
			if ctx.Err() != nil {
				return
			}

			// 重置 ready channel
			c.ready = make(chan struct{})
		}
	}()

	// 等待消费者准备就绪
	<-c.ready
	logger.Info("kafka consumer started")

	return nil
}

// Stop 停止消费
func (c *ConsumerGroup) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}

	c.wg.Wait()

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("close consumer group failed: %w", err)
	}

	logger.Info("kafka consumer stopped")
	return nil
}

// Setup 实现 sarama.ConsumerGroupHandler
func (c *ConsumerGroup) Setup(session sarama.ConsumerGroupSession) error {
	logger.Info("consumer session setup",
		"generation_id", session.GenerationID(),
		"member_id", session.MemberID(),
	)
	close(c.ready)
	return nil
}

// Cleanup 实现 sarama.ConsumerGroupHandler
func (c *ConsumerGroup) Cleanup(session sarama.ConsumerGroupSession) error {
	logger.Info("consumer session cleanup",
		"generation_id", session.GenerationID(),
	)
	return nil
}

// ConsumeClaim 实现 sarama.ConsumerGroupHandler
func (c *ConsumerGroup) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	topic := claim.Topic()

	c.mu.RLock()
	handler, ok := c.handlers[topic]
	c.mu.RUnlock()

	if !ok {
		logger.Warn("no handler for topic", "topic", topic)
		return nil
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			// 转换消息格式
			m := &Message{
				Topic:     msg.Topic,
				Key:       msg.Key,
				Value:     msg.Value,
				Partition: msg.Partition,
				Offset:    msg.Offset,
				Timestamp: msg.Timestamp.UnixMilli(),
			}

			// 调用处理器
			if err := handler.Handle(session.Context(), m); err != nil {
				logger.Error("handle message failed",
					"topic", topic,
					"partition", msg.Partition,
					"offset", msg.Offset,
					"error", err,
				)
				// 不标记消息，下次重新消费
				continue
			}

			// 标记消息已处理
			session.MarkMessage(msg, "")

		case <-session.Context().Done():
			return nil
		}
	}
}
