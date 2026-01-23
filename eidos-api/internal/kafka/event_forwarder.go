// Package kafka 提供 Kafka 消息消费和转发功能
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"log/slog"

	commonKafka "github.com/eidos-exchange/eidos/eidos-common/pkg/kafka"
)

// Kafka topic 常量
const (
	TopicOrderUpdates   = "order-updates"
	TopicBalanceUpdates = "balance-updates"
)

// Redis channel 前缀
const (
	RedisChannelOrders   = "eidos:orders:"
	RedisChannelBalances = "eidos:balances:"
)

// EventForwarder 消费 Kafka 消息并转发到 Redis Pub/Sub
// 桥接 eidos-trading 发布的订单/余额更新消息到 WebSocket
// 支持高并发场景：批量发布、Pipeline、背压控制
type EventForwarder struct {
	consumer *commonKafka.Consumer
	redis    *redis.Client
	logger   *slog.Logger

	// 高并发支持：缓冲和批量处理
	orderBuffer   chan *publishTask
	balanceBuffer chan *publishTask
	batchSize     int           // 批量大小
	batchTimeout  time.Duration // 批量超时

	// 统计
	ordersForwarded   int64
	balancesForwarded int64
	batchCount        int64
	errors            int64
	redisLatencyNs    int64 // Redis 发布延迟 (纳秒)

	// 状态
	started int32
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// publishTask 发布任务
type publishTask struct {
	channel string
	payload []byte
}

// OrderUpdateMessage 订单更新消息 (来自 eidos-trading)
type OrderUpdateMessage struct {
	OrderID       string `json:"order_id"`
	Wallet        string `json:"wallet"`
	Market        string `json:"market"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	Price         string `json:"price"`
	Amount        string `json:"amount"`
	FilledAmount  string `json:"filled_amount"`
	FilledQuote   string `json:"filled_quote"`
	Status        string `json:"status"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	RejectReason  string `json:"reject_reason,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	Timestamp     int64  `json:"timestamp"`
}

// BalanceUpdateMessage 余额更新消息 (来自 eidos-trading)
type BalanceUpdateMessage struct {
	Wallet    string `json:"wallet"`
	Token     string `json:"token"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Pending   string `json:"pending"`
	Total     string `json:"total"`
	EventType string `json:"event_type"`
	EventID   string `json:"event_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Config EventForwarder 配置
type Config struct {
	// BufferSize 缓冲区大小 (每种类型)
	BufferSize int
	// BatchSize 批量发布大小
	BatchSize int
	// BatchTimeout 批量超时
	BatchTimeout time.Duration
	// Workers 发布 worker 数量
	Workers int
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		BufferSize:   10000, // 10K 缓冲
		BatchSize:    100,   // 每批 100 条
		BatchTimeout: 10 * time.Millisecond,
		Workers:      4, // 4 个 worker
	}
}

// NewEventForwarder 创建事件转发器
func NewEventForwarder(kafkaCfg *commonKafka.ConsumerConfig, redisClient *redis.Client, logger *slog.Logger) (*EventForwarder, error) {
	return NewEventForwarderWithConfig(kafkaCfg, redisClient, logger, nil)
}

// NewEventForwarderWithConfig 使用自定义配置创建事件转发器
func NewEventForwarderWithConfig(kafkaCfg *commonKafka.ConsumerConfig, redisClient *redis.Client, logger *slog.Logger, cfg *Config) (*EventForwarder, error) {
	if kafkaCfg == nil {
		return nil, fmt.Errorf("kafka consumer config is nil")
	}

	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 创建 Kafka consumer
	consumer, err := commonKafka.NewConsumer(kafkaCfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	ef := &EventForwarder{
		consumer:      consumer,
		redis:         redisClient,
		logger:        logger,
		orderBuffer:   make(chan *publishTask, cfg.BufferSize),
		balanceBuffer: make(chan *publishTask, cfg.BufferSize),
		batchSize:     cfg.BatchSize,
		batchTimeout:  cfg.BatchTimeout,
		stopCh:        make(chan struct{}),
	}

	// 设置消息处理器
	consumer.SetHandler(ef.handleMessage)

	return ef, nil
}

// Start 启动事件转发器
func (ef *EventForwarder) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&ef.started, 0, 1) {
		return fmt.Errorf("event forwarder already started")
	}

	ef.logger.Info("启动事件转发器",
		"topics", []string{TopicOrderUpdates, TopicBalanceUpdates},
		"batch_size", ef.batchSize,
		"batch_timeout", ef.batchTimeout,
	)

	// 启动 Redis 批量发布 workers
	numWorkers := 4
	for i := 0; i < numWorkers; i++ {
		ef.wg.Add(2)
		go ef.orderPublishWorker(ctx, i)
		go ef.balancePublishWorker(ctx, i)
	}

	// 启动 Kafka 消费
	if err := ef.consumer.Start(ctx); err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}

	// 启动统计上报
	ef.wg.Add(1)
	go ef.reportStats(ctx)

	return nil
}

// Stop 停止事件转发器
func (ef *EventForwarder) Stop() error {
	if ef.consumer == nil {
		return nil
	}

	close(ef.stopCh)

	// 等待 workers 完成
	done := make(chan struct{})
	go func() {
		ef.wg.Wait()
		close(done)
	}()

	// 最多等待 10 秒
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		ef.logger.Warn("event forwarder stop timeout")
	}

	if err := ef.consumer.Close(); err != nil {
		return fmt.Errorf("close consumer: %w", err)
	}

	ef.logger.Info("事件转发器已停止",
		"orders_forwarded", atomic.LoadInt64(&ef.ordersForwarded),
		"balances_forwarded", atomic.LoadInt64(&ef.balancesForwarded),
		"batch_count", atomic.LoadInt64(&ef.batchCount),
		"errors", atomic.LoadInt64(&ef.errors),
	)

	return nil
}

// handleMessage 处理 Kafka 消息
func (ef *EventForwarder) handleMessage(ctx context.Context, msg *commonKafka.Message) error {
	switch msg.Topic {
	case TopicOrderUpdates:
		return ef.handleOrderUpdate(ctx, msg)
	case TopicBalanceUpdates:
		return ef.handleBalanceUpdate(ctx, msg)
	default:
		ef.logger.Warn("unknown topic", "topic", msg.Topic)
		return nil
	}
}

// handleOrderUpdate 处理订单更新消息
func (ef *EventForwarder) handleOrderUpdate(ctx context.Context, msg *commonKafka.Message) error {
	var orderMsg OrderUpdateMessage
	if err := json.Unmarshal(msg.Value, &orderMsg); err != nil {
		atomic.AddInt64(&ef.errors, 1)
		ef.logger.Error("解析订单更新消息失败",
			"error", err,
			"value", string(msg.Value),
		)
		return nil // 不重试解析错误
	}

	// 构建 Redis channel: eidos:orders:{wallet}
	wallet := strings.ToLower(orderMsg.Wallet)
	channel := RedisChannelOrders + wallet

	// 放入缓冲区（非阻塞）
	task := &publishTask{channel: channel, payload: msg.Value}
	select {
	case ef.orderBuffer <- task:
		// 成功
	default:
		// 缓冲区满，直接发布（背压）
		atomic.AddInt64(&ef.errors, 1)
		ef.logger.Warn("订单缓冲区满，直接发布",
			"order_id", orderMsg.OrderID,
			"buffer_len", len(ef.orderBuffer),
		)
		return ef.publishDirect(ctx, channel, msg.Value)
	}

	return nil
}

// handleBalanceUpdate 处理余额更新消息
func (ef *EventForwarder) handleBalanceUpdate(ctx context.Context, msg *commonKafka.Message) error {
	var balanceMsg BalanceUpdateMessage
	if err := json.Unmarshal(msg.Value, &balanceMsg); err != nil {
		atomic.AddInt64(&ef.errors, 1)
		ef.logger.Error("解析余额更新消息失败",
			"error", err,
			"value", string(msg.Value),
		)
		return nil // 不重试解析错误
	}

	// 构建 Redis channel: eidos:balances:{wallet}
	wallet := strings.ToLower(balanceMsg.Wallet)
	channel := RedisChannelBalances + wallet

	// 放入缓冲区（非阻塞）
	task := &publishTask{channel: channel, payload: msg.Value}
	select {
	case ef.balanceBuffer <- task:
		// 成功
	default:
		// 缓冲区满，直接发布（背压）
		atomic.AddInt64(&ef.errors, 1)
		ef.logger.Warn("余额缓冲区满，直接发布",
			"wallet", wallet,
			"token", balanceMsg.Token,
			"buffer_len", len(ef.balanceBuffer),
		)
		return ef.publishDirect(ctx, channel, msg.Value)
	}

	return nil
}

// orderPublishWorker 订单发布 worker (批量 Pipeline)
func (ef *EventForwarder) orderPublishWorker(ctx context.Context, workerID int) {
	defer ef.wg.Done()

	batch := make([]*publishTask, 0, ef.batchSize)
	ticker := time.NewTicker(ef.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ef.flushBatch(ctx, batch, "orders")
			return
		case <-ef.stopCh:
			ef.flushBatch(ctx, batch, "orders")
			return
		case task := <-ef.orderBuffer:
			batch = append(batch, task)
			if len(batch) >= ef.batchSize {
				ef.flushBatch(ctx, batch, "orders")
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				ef.flushBatch(ctx, batch, "orders")
				batch = batch[:0]
			}
		}
	}
}

// balancePublishWorker 余额发布 worker (批量 Pipeline)
func (ef *EventForwarder) balancePublishWorker(ctx context.Context, workerID int) {
	defer ef.wg.Done()

	batch := make([]*publishTask, 0, ef.batchSize)
	ticker := time.NewTicker(ef.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ef.flushBatch(ctx, batch, "balances")
			return
		case <-ef.stopCh:
			ef.flushBatch(ctx, batch, "balances")
			return
		case task := <-ef.balanceBuffer:
			batch = append(batch, task)
			if len(batch) >= ef.batchSize {
				ef.flushBatch(ctx, batch, "balances")
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				ef.flushBatch(ctx, batch, "balances")
				batch = batch[:0]
			}
		}
	}
}

// flushBatch 使用 Pipeline 批量发布
func (ef *EventForwarder) flushBatch(ctx context.Context, batch []*publishTask, batchType string) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	// 使用 Pipeline 批量发布
	pipe := ef.redis.Pipeline()
	for _, task := range batch {
		pipe.Publish(ctx, task.channel, task.payload)
	}

	_, err := pipe.Exec(ctx)
	latency := time.Since(start)
	atomic.StoreInt64(&ef.redisLatencyNs, latency.Nanoseconds())

	if err != nil {
		atomic.AddInt64(&ef.errors, int64(len(batch)))
		ef.logger.Error("批量发布到 Redis 失败",
			"type", batchType,
			"batch_size", len(batch),
			"error", err,
			"latency", latency,
		)
		return
	}

	atomic.AddInt64(&ef.batchCount, 1)

	// 更新统计
	if batchType == "orders" {
		atomic.AddInt64(&ef.ordersForwarded, int64(len(batch)))
	} else {
		atomic.AddInt64(&ef.balancesForwarded, int64(len(batch)))
	}

	ef.logger.Debug("批量发布完成",
		"type", batchType,
		"batch_size", len(batch),
		"latency", latency,
	)
}

// publishDirect 直接发布（背压时使用）
func (ef *EventForwarder) publishDirect(ctx context.Context, channel string, payload []byte) error {
	start := time.Now()
	err := ef.redis.Publish(ctx, channel, payload).Err()
	latency := time.Since(start)
	atomic.StoreInt64(&ef.redisLatencyNs, latency.Nanoseconds())

	if err != nil {
		atomic.AddInt64(&ef.errors, 1)
		return err
	}

	return nil
}

// reportStats 定期上报统计信息
func (ef *EventForwarder) reportStats(ctx context.Context) {
	defer ef.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ef.stopCh:
			return
		case <-ticker.C:
			metrics := ef.consumer.Metrics()
			ef.logger.Info("事件转发器统计",
				"orders_forwarded", atomic.LoadInt64(&ef.ordersForwarded),
				"balances_forwarded", atomic.LoadInt64(&ef.balancesForwarded),
				"batch_count", atomic.LoadInt64(&ef.batchCount),
				"errors", atomic.LoadInt64(&ef.errors),
				"order_buffer_len", len(ef.orderBuffer),
				"balance_buffer_len", len(ef.balanceBuffer),
				"redis_latency_ms", float64(atomic.LoadInt64(&ef.redisLatencyNs))/1e6,
				"kafka_messages_received", metrics.MessagesReceived,
				"kafka_messages_processed", metrics.MessagesProcessed,
				"kafka_messages_failed", metrics.MessagesFailed,
				"kafka_processing_latency_ms", float64(metrics.ProcessingLatency)/1e6,
			)
		}
	}
}

// Stats 返回统计信息
func (ef *EventForwarder) Stats() map[string]interface{} {
	return map[string]interface{}{
		"orders_forwarded":    atomic.LoadInt64(&ef.ordersForwarded),
		"balances_forwarded":  atomic.LoadInt64(&ef.balancesForwarded),
		"batch_count":         atomic.LoadInt64(&ef.batchCount),
		"errors":              atomic.LoadInt64(&ef.errors),
		"order_buffer_len":    len(ef.orderBuffer),
		"balance_buffer_len":  len(ef.balanceBuffer),
		"redis_latency_ns":    atomic.LoadInt64(&ef.redisLatencyNs),
	}
}
