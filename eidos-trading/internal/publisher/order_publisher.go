// Package publisher 提供 Kafka 消息发布功能
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"go.uber.org/zap"
)

// OrderPublisher 订单状态更新发布者
// 发布消息到 order-updates topic，供 eidos-api WebSocket 推送
type OrderPublisher struct {
	producer KafkaProducer
}

// KafkaProducer Kafka 生产者接口
type KafkaProducer interface {
	SendWithContext(ctx context.Context, topic string, key, value []byte) error
}

// NewOrderPublisher 创建订单发布者
func NewOrderPublisher(producer KafkaProducer) *OrderPublisher {
	return &OrderPublisher{
		producer: producer,
	}
}

// OrderUpdateMessage 订单状态更新消息
type OrderUpdateMessage struct {
	OrderID       string `json:"order_id"`
	Wallet        string `json:"wallet"`
	Market        string `json:"market"`
	Side          string `json:"side"`          // buy, sell
	Type          string `json:"type"`          // limit, market
	Price         string `json:"price"`         // 订单价格
	Amount        string `json:"amount"`        // 订单数量
	FilledAmount  string `json:"filled_amount"` // 已成交数量
	FilledQuote   string `json:"filled_quote"`  // 已成交金额
	Status        string `json:"status"`        // pending, open, partial, filled, cancelled, expired, rejected
	ClientOrderID string `json:"client_order_id,omitempty"`
	RejectReason  string `json:"reject_reason,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	Timestamp     int64  `json:"timestamp"` // 消息时间戳
}

// PublishOrderUpdate 发布订单状态更新
func (p *OrderPublisher) PublishOrderUpdate(ctx context.Context, order *model.Order) error {
	if p.producer == nil {
		return nil // Kafka 未启用
	}

	msg := &OrderUpdateMessage{
		OrderID:       order.OrderID,
		Wallet:        order.Wallet,
		Market:        order.Market,
		Side:          order.Side.String(),
		Type:          order.Type.String(),
		Price:         order.Price.String(),
		Amount:        order.Amount.String(),
		FilledAmount:  order.FilledAmount.String(),
		FilledQuote:   order.FilledQuote.String(),
		Status:        order.Status.String(),
		ClientOrderID: order.ClientOrderID,
		RejectReason:  order.RejectReason,
		CreatedAt:     order.CreatedAt,
		UpdatedAt:     order.UpdatedAt,
		Timestamp:     time.Now().UnixMilli(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal order update message: %w", err)
	}

	// 使用 wallet 作为 key，保证同一用户的订单消息有序
	key := []byte(order.Wallet)

	if err := p.producer.SendWithContext(ctx, kafka.TopicOrderUpdates, key, data); err != nil {
		logger.Error("publish order update failed",
			zap.String("order_id", order.OrderID),
			zap.Error(err),
		)
		return fmt.Errorf("send order update: %w", err)
	}

	logger.Debug("order update published",
		zap.String("order_id", order.OrderID),
		zap.String("status", order.Status.String()),
	)

	return nil
}

// PublishOrderCreated 发布订单创建消息
func (p *OrderPublisher) PublishOrderCreated(ctx context.Context, order *model.Order) error {
	return p.PublishOrderUpdate(ctx, order)
}

// PublishOrderCancelled 发布订单取消消息
func (p *OrderPublisher) PublishOrderCancelled(ctx context.Context, order *model.Order) error {
	return p.PublishOrderUpdate(ctx, order)
}

// PublishOrderFilled 发布订单成交消息
func (p *OrderPublisher) PublishOrderFilled(ctx context.Context, order *model.Order) error {
	return p.PublishOrderUpdate(ctx, order)
}

// PublishOrderExpired 发布订单过期消息
func (p *OrderPublisher) PublishOrderExpired(ctx context.Context, order *model.Order) error {
	return p.PublishOrderUpdate(ctx, order)
}

// PublishOrderRejected 发布订单拒绝消息
func (p *OrderPublisher) PublishOrderRejected(ctx context.Context, order *model.Order) error {
	return p.PublishOrderUpdate(ctx, order)
}
