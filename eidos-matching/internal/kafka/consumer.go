// Package kafka Kafka 消费者实现
//
// 订阅的 Topic (Input):
//   - orders: 新订单消息
//   - 发送方: eidos-trading
//   - 消息格式: OrderMessage (JSON)
//   - 分区策略: 按 market 字段分区，保证同一市场的订单顺序
//   - cancel-requests: 取消请求
//   - 发送方: eidos-trading
//   - 消息格式: CancelMessage (JSON)
//   - 分区策略: 按 market 字段分区
//
// 消费者组: eidos-matching-{instance_id}
//   - 每个实例消费特定分区，实现市场级别的水平扩展
//   - 分区分配策略: 手动分配或按市场分片
//
// 已完成对接:
//   - 死信队列 (DLQ): 解析失败/处理失败的消息发送到 orders-dlq / cancel-requests-dlq
//   - 日志系统: 使用 zap 统一日志
//   - 监控指标: 消费延迟、处理失败率、DLQ 计数
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers      []string
	GroupID      string
	OrdersTopic  string
	CancelsTopic string
	BatchSize    int
	LingerMs     int
	StartOffset  int64  // -1: latest, -2: earliest
	CommitMode   string // "auto" or "manual"
	// 重试配置
	MaxRetries   int           // 最大重试次数，默认 3
	RetryBackoff time.Duration // 重试退避时间，默认 100ms
	// 死信队列配置
	EnableDLQ        bool   // 是否启用死信队列
	OrdersDLQTopic   string // 订单死信队列 topic
	CancelsDLQTopic  string // 取消死信队列 topic
}

// OrderHandler 订单处理函数
type OrderHandler func(ctx context.Context, order *model.Order) error

// CancelHandler 取消处理函数
type CancelHandler func(ctx context.Context, cancel *model.CancelMessage) error

// Consumer Kafka 消费者
type Consumer struct {
	ordersReader  *kafkago.Reader
	cancelsReader *kafkago.Reader
	config        *ConsumerConfig

	orderHandler  OrderHandler
	cancelHandler CancelHandler

	// 死信队列 writer
	ordersDLQWriter  *kafkago.Writer
	cancelsDLQWriter *kafkago.Writer

	// 偏移量管理 (用于快照恢复)
	offsets   map[string]int64 // topic:partition -> offset
	offsetsMu sync.RWMutex

	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConsumer 创建消费者
func NewConsumer(cfg *ConsumerConfig) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())

	// 设置默认值
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.OrdersDLQTopic == "" {
		cfg.OrdersDLQTopic = cfg.OrdersTopic + "-dlq"
	}
	if cfg.CancelsDLQTopic == "" {
		cfg.CancelsDLQTopic = cfg.CancelsTopic + "-dlq"
	}

	c := &Consumer{
		config:  cfg,
		offsets: make(map[string]int64),
		ctx:     ctx,
		cancel:  cancel,
	}

	// 创建订单消费者
	c.ordersReader = kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.OrdersTopic,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        time.Duration(cfg.LingerMs) * time.Millisecond,
		StartOffset:    cfg.StartOffset,
		CommitInterval: time.Second, // 自动提交间隔
	})

	// 创建取消消费者
	c.cancelsReader = kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.CancelsTopic,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        time.Duration(cfg.LingerMs) * time.Millisecond,
		StartOffset:    cfg.StartOffset,
		CommitInterval: time.Second,
	})

	// 创建死信队列 writer
	if cfg.EnableDLQ {
		c.ordersDLQWriter = &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        cfg.OrdersDLQTopic,
			Balancer:     &kafkago.LeastBytes{},
			RequiredAcks: kafkago.RequireAll,
		}
		c.cancelsDLQWriter = &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        cfg.CancelsDLQTopic,
			Balancer:     &kafkago.LeastBytes{},
			RequiredAcks: kafkago.RequireAll,
		}
	}

	return c
}

// SetOrderHandler 设置订单处理函数
func (c *Consumer) SetOrderHandler(h OrderHandler) {
	c.orderHandler = h
}

// SetCancelHandler 设置取消处理函数
func (c *Consumer) SetCancelHandler(h CancelHandler) {
	c.cancelHandler = h
}

// Start 启动消费
func (c *Consumer) Start() error {
	if c.orderHandler == nil || c.cancelHandler == nil {
		return fmt.Errorf("handlers not set")
	}

	// 启动订单消费协程
	c.wg.Add(1)
	go c.consumeOrders()

	// 启动取消消费协程
	c.wg.Add(1)
	go c.consumeCancels()

	return nil
}

// Stop 停止消费
func (c *Consumer) Stop() error {
	c.cancel()
	c.wg.Wait()

	var errs []error

	if err := c.ordersReader.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close orders reader: %w", err))
	}
	if err := c.cancelsReader.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close cancels reader: %w", err))
	}

	// 关闭死信队列 writer
	if c.ordersDLQWriter != nil {
		if err := c.ordersDLQWriter.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close orders DLQ writer: %w", err))
		}
	}
	if c.cancelsDLQWriter != nil {
		if err := c.cancelsDLQWriter.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close cancels DLQ writer: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop consumer errors: %v", errs)
	}
	return nil
}

// consumeOrders 消费订单消息
func (c *Consumer) consumeOrders() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		msg, err := c.ordersReader.FetchMessage(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return // 上下文取消
			}
			metrics.RecordKafkaError("orders", "consume")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 记录接收消息和消费延迟
		metrics.RecordKafkaMessage("orders", true)
		lagSeconds := time.Since(msg.Time).Seconds()
		metrics.RecordKafkaLag("orders", lagSeconds)

		// 解析订单消息
		order, err := c.parseOrderMessage(msg.Value)
		if err != nil {
			// 解析失败，发送到死信队列
			metrics.RecordKafkaError("orders", "parse")
			c.sendToDLQ(c.ordersDLQWriter, msg, "parse_error", err.Error())
			c.commitMessage(c.ordersReader, msg)
			continue
		}

		// 处理订单 (带重试)
		if err := c.processWithRetry(func() error {
			return c.orderHandler(c.ctx, order)
		}); err != nil {
			// 重试失败，发送到死信队列
			metrics.RecordKafkaError("orders", "handle")
			c.sendToDLQ(c.ordersDLQWriter, msg, "handle_error", err.Error())
		}

		// 更新偏移量
		c.updateOffset(msg.Topic, msg.Partition, msg.Offset)

		// 提交偏移量
		c.commitMessage(c.ordersReader, msg)
	}
}

// consumeCancels 消费取消消息
func (c *Consumer) consumeCancels() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		msg, err := c.cancelsReader.FetchMessage(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			metrics.RecordKafkaError("cancel-requests", "consume")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 记录接收消息和消费延迟
		metrics.RecordKafkaMessage("cancel-requests", true)
		lagSeconds := time.Since(msg.Time).Seconds()
		metrics.RecordKafkaLag("cancel-requests", lagSeconds)

		// 解析取消消息
		cancel, err := c.parseCancelMessage(msg.Value)
		if err != nil {
			// 解析失败，发送到死信队列
			metrics.RecordKafkaError("cancel-requests", "parse")
			c.sendToDLQ(c.cancelsDLQWriter, msg, "parse_error", err.Error())
			c.commitMessage(c.cancelsReader, msg)
			continue
		}

		// 处理取消 (带重试)
		if err := c.processWithRetry(func() error {
			return c.cancelHandler(c.ctx, cancel)
		}); err != nil {
			// 重试失败，发送到死信队列
			metrics.RecordKafkaError("cancel-requests", "handle")
			c.sendToDLQ(c.cancelsDLQWriter, msg, "handle_error", err.Error())
		}

		// 更新偏移量
		c.updateOffset(msg.Topic, msg.Partition, msg.Offset)

		// 提交偏移量
		c.commitMessage(c.cancelsReader, msg)
	}
}

// parseOrderMessage 解析订单消息
func (c *Consumer) parseOrderMessage(data []byte) (*model.Order, error) {
	var msg model.OrderMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal order message: %w", err)
	}

	price, err := decimal.NewFromString(msg.Price)
	if err != nil {
		return nil, fmt.Errorf("parse price: %w", err)
	}

	amount, err := decimal.NewFromString(msg.Amount)
	if err != nil {
		return nil, fmt.Errorf("parse amount: %w", err)
	}

	return &model.Order{
		OrderID:     msg.OrderID,
		Wallet:      msg.Wallet,
		Market:      msg.Market,
		Side:        model.OrderSide(msg.Side),
		Type:        model.OrderType(msg.OrderType),
		TimeInForce: model.TimeInForce(msg.TimeInForce),
		Price:       price,
		Amount:      amount,
		Remaining:   amount, // 初始剩余数量等于总数量
		Timestamp:   msg.Timestamp,
		Sequence:    msg.Sequence,
	}, nil
}

// parseCancelMessage 解析取消消息
func (c *Consumer) parseCancelMessage(data []byte) (*model.CancelMessage, error) {
	var msg model.CancelMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal cancel message: %w", err)
	}
	return &msg, nil
}

// commitMessage 提交偏移量
func (c *Consumer) commitMessage(reader *kafkago.Reader, msg kafkago.Message) {
	if c.config.CommitMode == "manual" {
		if err := reader.CommitMessages(c.ctx, msg); err != nil {
			zap.L().Warn("commit message failed",
				zap.String("topic", msg.Topic),
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
		}
	}
}

// updateOffset 更新偏移量
func (c *Consumer) updateOffset(topic string, partition int, offset int64) {
	key := fmt.Sprintf("%s:%d", topic, partition)
	c.offsetsMu.Lock()
	c.offsets[key] = offset
	c.offsetsMu.Unlock()
}

// GetOffsets 获取当前偏移量 (用于快照)
func (c *Consumer) GetOffsets() map[string]int64 {
	c.offsetsMu.RLock()
	defer c.offsetsMu.RUnlock()

	result := make(map[string]int64, len(c.offsets))
	for k, v := range c.offsets {
		result[k] = v
	}
	return result
}

// SeekToOffset 设置偏移量 (用于恢复)
// TODO: 需要在 Start 之前调用
func (c *Consumer) SeekToOffset(topic string, partition int, offset int64) error {
	// kafka-go 需要在创建 Reader 时指定 StartOffset
	// 或者使用 SetOffset 方法
	return fmt.Errorf("seek not implemented: use StartOffset in config")
}

// processWithRetry 带重试的处理函数
func (c *Consumer) processWithRetry(fn func() error) error {
	var lastErr error
	backoff := c.config.RetryBackoff

	for i := 0; i <= c.config.MaxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			// 检查是否是可重试错误
			if !c.isRetryableError(err) {
				return err // 不可重试，直接返回
			}
			if i < c.config.MaxRetries {
				// 指数退避
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second // 最大退避 5 秒
				}
			}
			continue
		}
		return nil // 成功
	}
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isRetryableError 判断错误是否可重试
func (c *Consumer) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// 暂时简化处理：所有错误都可重试
	// 实际生产环境中应该区分：
	// - 可重试：网络超时、临时故障
	// - 不可重试：数据格式错误、业务逻辑错误
	return true
}

// sendToDLQ 发送消息到死信队列
func (c *Consumer) sendToDLQ(writer *kafkago.Writer, originalMsg kafkago.Message, errorType, errorMsg string) {
	if writer == nil || !c.config.EnableDLQ {
		return // DLQ 未启用
	}

	// 构建 DLQ 消息，保留原始消息并添加错误信息
	dlqMsg := DLQMessage{
		OriginalTopic:     originalMsg.Topic,
		OriginalPartition: originalMsg.Partition,
		OriginalOffset:    originalMsg.Offset,
		OriginalKey:       string(originalMsg.Key),
		OriginalValue:     string(originalMsg.Value),
		OriginalTime:      originalMsg.Time.UnixNano(),
		ErrorType:         errorType,
		ErrorMessage:      errorMsg,
		FailedAt:          time.Now().UnixNano(),
		RetryCount:        c.config.MaxRetries,
	}

	data, err := json.Marshal(dlqMsg)
	if err != nil {
		metrics.RecordKafkaError(writer.Topic, "dlq_marshal")
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	if err := writer.WriteMessages(ctx, kafkago.Message{
		Key:   originalMsg.Key,
		Value: data,
	}); err != nil {
		metrics.RecordKafkaError(writer.Topic, "dlq_send")
		zap.L().Error("send to DLQ failed",
			zap.String("dlq_topic", writer.Topic),
			zap.String("original_topic", originalMsg.Topic),
			zap.Int64("offset", originalMsg.Offset),
			zap.Error(err))
		return
	}

	zap.L().Warn("message sent to DLQ",
		zap.String("dlq_topic", writer.Topic),
		zap.String("original_topic", originalMsg.Topic),
		zap.Int64("offset", originalMsg.Offset),
		zap.String("error_type", errorType),
		zap.String("error_msg", errorMsg))
	metrics.RecordKafkaMessage(writer.Topic, false)
}

// DLQMessage 死信队列消息格式
type DLQMessage struct {
	OriginalTopic     string `json:"original_topic"`
	OriginalPartition int    `json:"original_partition"`
	OriginalOffset    int64  `json:"original_offset"`
	OriginalKey       string `json:"original_key"`
	OriginalValue     string `json:"original_value"`
	OriginalTime      int64  `json:"original_time"`
	ErrorType         string `json:"error_type"`
	ErrorMessage      string `json:"error_message"`
	FailedAt          int64  `json:"failed_at"`
	RetryCount        int    `json:"retry_count"`
}
