// Package kafka Kafka 生产者实现
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	kafkago "github.com/segmentio/kafka-go"
)

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Brokers               []string
	TradeResultsTopic     string
	OrderCancelledTopic   string
	OrderbookUpdatesTopic string
	OrderUpdatesTopic     string // 订单状态更新 topic
	BatchSize             int
	BatchTimeout          time.Duration
	Compression           string // "none", "gzip", "snappy", "lz4"
	RequiredAcks          int    // -1: all, 0: none, 1: leader
}

// Producer Kafka 生产者
type Producer struct {
	tradesWriter       *kafkago.Writer
	cancelsWriter      *kafkago.Writer
	updatesWriter      *kafkago.Writer
	orderUpdatesWriter *kafkago.Writer // 订单状态更新写入器
	config             *ProducerConfig

	// 统计
	messagesSent int64
	errorsCount  int64
	mu           sync.Mutex
}

// NewProducer 创建生产者
func NewProducer(cfg *ProducerConfig) *Producer {
	// 选择压缩算法
	var compression kafkago.Compression
	switch cfg.Compression {
	case "gzip":
		compression = kafkago.Gzip
	case "snappy":
		compression = kafkago.Snappy
	case "lz4":
		compression = kafkago.Lz4
	default:
		compression = 0 // no compression
	}

	// 确定 ack 设置
	requiredAcks := kafkago.RequireAll
	switch cfg.RequiredAcks {
	case 0:
		requiredAcks = kafkago.RequireNone
	case 1:
		requiredAcks = kafkago.RequireOne
	}

	p := &Producer{
		config: cfg,
	}

	// 成交结果写入器
	p.tradesWriter = &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.TradeResultsTopic,
		Balancer:     &kafkago.Hash{}, // 按 key 分区
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: requiredAcks,
		Compression:  compression,
		Async:        false, // 同步写入，确保可靠性
	}

	// 取消结果写入器
	p.cancelsWriter = &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.OrderCancelledTopic,
		Balancer:     &kafkago.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: requiredAcks,
		Compression:  compression,
		Async:        false,
	}

	// 订单簿更新写入器 (可以使用异步模式)
	p.updatesWriter = &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.OrderbookUpdatesTopic,
		Balancer:     &kafkago.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		RequiredAcks: kafkago.RequireOne, // 更新消息可以接受较低的可靠性
		Compression:  compression,
		Async:        true, // 异步写入，提高吞吐
	}

	// 订单状态更新写入器 (如果配置了 topic)
	if cfg.OrderUpdatesTopic != "" {
		p.orderUpdatesWriter = &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        cfg.OrderUpdatesTopic,
			Balancer:     &kafkago.Hash{},
			BatchSize:    cfg.BatchSize,
			BatchTimeout: cfg.BatchTimeout,
			RequiredAcks: requiredAcks,
			Compression:  compression,
			Async:        false, // 同步写入，确保可靠性
		}
	}

	return p
}

// SendTradeResult 发送成交结果
// 使用 TradeResultMessage 格式，与 eidos-market 对齐
func (p *Producer) SendTradeResult(ctx context.Context, trade *model.TradeResult) error {
	// 转换为 Kafka 消息格式
	msg := trade.ToMessage()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal trade result: %w", err)
	}

	kafkaMsg := kafkago.Message{
		Key:   []byte(trade.Market), // 按市场分区
		Value: data,
		Time:  time.Now(),
	}

	if err := p.tradesWriter.WriteMessages(ctx, kafkaMsg); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write trade result: %w", err)
	}

	p.incrementSent()
	return nil
}

// SendTradeResults 批量发送成交结果
// 使用 TradeResultMessage 格式，与 eidos-market 对齐
func (p *Producer) SendTradeResults(ctx context.Context, trades []*model.TradeResult) error {
	if len(trades) == 0 {
		return nil
	}

	messages := make([]kafkago.Message, len(trades))
	for i, trade := range trades {
		// 转换为 Kafka 消息格式
		msg := trade.ToMessage()
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal trade result: %w", err)
		}
		messages[i] = kafkago.Message{
			Key:   []byte(trade.Market),
			Value: data,
			Time:  time.Now(),
		}
	}

	if err := p.tradesWriter.WriteMessages(ctx, messages...); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write trade results: %w", err)
	}

	p.mu.Lock()
	p.messagesSent += int64(len(messages))
	p.mu.Unlock()

	return nil
}

// SendCancelResult 发送取消结果
func (p *Producer) SendCancelResult(ctx context.Context, cancel *model.CancelResult) error {
	data, err := json.Marshal(cancel)
	if err != nil {
		return fmt.Errorf("marshal cancel result: %w", err)
	}

	msg := kafkago.Message{
		Key:   []byte(cancel.OrderID), // 按订单ID分区
		Value: data,
		Time:  time.Now(),
	}

	if err := p.cancelsWriter.WriteMessages(ctx, msg); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write cancel result: %w", err)
	}

	p.incrementSent()
	return nil
}

// SendOrderBookUpdate 发送单个订单簿更新 (内部格式)
// Deprecated: 请使用 SendOrderBookUpdateMessage 发送与 eidos-market 兼容的格式
func (p *Producer) SendOrderBookUpdate(ctx context.Context, update *model.OrderBookUpdate) error {
	data, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("marshal orderbook update: %w", err)
	}

	msg := kafkago.Message{
		Key:   []byte(update.Market), // 按市场分区
		Value: data,
		Time:  time.Now(),
	}

	if err := p.updatesWriter.WriteMessages(ctx, msg); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write orderbook update: %w", err)
	}

	p.incrementSent()
	return nil
}

// SendOrderBookUpdates 批量发送订单簿更新 (内部格式)
// Deprecated: 请使用 SendOrderBookUpdateMessage 发送与 eidos-market 兼容的格式
func (p *Producer) SendOrderBookUpdates(ctx context.Context, updates []*model.OrderBookUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	messages := make([]kafkago.Message, len(updates))
	for i, update := range updates {
		data, err := json.Marshal(update)
		if err != nil {
			return fmt.Errorf("marshal orderbook update: %w", err)
		}
		messages[i] = kafkago.Message{
			Key:   []byte(update.Market),
			Value: data,
			Time:  time.Now(),
		}
	}

	if err := p.updatesWriter.WriteMessages(ctx, messages...); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write orderbook updates: %w", err)
	}

	p.mu.Lock()
	p.messagesSent += int64(len(messages))
	p.mu.Unlock()

	return nil
}

// SendOrderBookUpdateMessage 发送订单簿更新消息
// 使用 OrderBookUpdateMessage 格式，与 eidos-market 对齐
// 支持批量更新多个价格档位
func (p *Producer) SendOrderBookUpdateMessage(ctx context.Context, msg *model.OrderBookUpdateMessage) error {
	if msg.IsEmpty() {
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal orderbook update message: %w", err)
	}

	kafkaMsg := kafkago.Message{
		Key:   []byte(msg.Market), // 按市场分区
		Value: data,
		Time:  time.Now(),
	}

	if err := p.updatesWriter.WriteMessages(ctx, kafkaMsg); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write orderbook update message: %w", err)
	}

	p.incrementSent()
	return nil
}

// SendOrderRejected 发送订单被拒绝消息
func (p *Producer) SendOrderRejected(ctx context.Context, msg *model.OrderRejectedMessage) error {
	if p.orderUpdatesWriter == nil {
		return nil // order-updates topic 未配置
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal order rejected: %w", err)
	}

	kafkaMsg := kafkago.Message{
		Key:   []byte(msg.Wallet), // 按钱包分区，保证同一用户消息有序
		Value: data,
		Time:  time.Now(),
	}

	if err := p.orderUpdatesWriter.WriteMessages(ctx, kafkaMsg); err != nil {
		p.incrementErrors()
		return fmt.Errorf("write order rejected: %w", err)
	}

	p.incrementSent()
	return nil
}

// Close 关闭生产者
func (p *Producer) Close() error {
	var errs []error

	if err := p.tradesWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.cancelsWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := p.updatesWriter.Close(); err != nil {
		errs = append(errs, err)
	}
	if p.orderUpdatesWriter != nil {
		if err := p.orderUpdatesWriter.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close producer errors: %v", errs)
	}
	return nil
}

// GetStats 获取统计
func (p *Producer) GetStats() ProducerStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	return ProducerStats{
		MessagesSent: p.messagesSent,
		ErrorsCount:  p.errorsCount,
	}
}

func (p *Producer) incrementSent() {
	p.mu.Lock()
	p.messagesSent++
	p.mu.Unlock()
}

func (p *Producer) incrementErrors() {
	p.mu.Lock()
	p.errorsCount++
	p.mu.Unlock()
}

// ProducerStats 生产者统计
type ProducerStats struct {
	MessagesSent int64 `json:"messages_sent"`
	ErrorsCount  int64 `json:"errors_count"`
}
