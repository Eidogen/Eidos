package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// EventHandler 事件处理器接口
type EventHandler interface {
	// HandleEvent 处理事件
	HandleEvent(ctx context.Context, eventType string, payload []byte) error
}

// EventProcessor 事件处理器
// 负责消费 Kafka 消息并分发到对应的处理器
type EventProcessor struct {
	handlers map[string]EventHandler
}

// NewEventProcessor 创建事件处理器
func NewEventProcessor() *EventProcessor {
	return &EventProcessor{
		handlers: make(map[string]EventHandler),
	}
}

// RegisterHandler 注册事件处理器
func (p *EventProcessor) RegisterHandler(topic string, handler EventHandler) {
	p.handlers[topic] = handler
}

// Handlers 返回所有注册的处理器
func (p *EventProcessor) Handlers() map[string]EventHandler {
	return p.handlers
}

// Handle 实现 kafka.Handler 接口
func (p *EventProcessor) Handle(ctx context.Context, msg *kafka.Message) error {
	handler, ok := p.handlers[msg.Topic]
	if !ok {
		logger.Warn("no handler for topic", "topic", msg.Topic)
		return nil
	}

	logger.Debug("processing event",
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
	)

	// 记录消息延迟 (Consumer Lag)
	lag := time.Since(time.UnixMilli(msg.Timestamp)).Seconds()
	metrics.RecordConsumerLag(msg.Topic, lag)

	if err := handler.HandleEvent(ctx, msg.Topic, msg.Value); err != nil {
		logger.Error("handle event failed",
			"topic", msg.Topic,
			"error", err,
		)
		return err
	}

	return nil
}

// TradeResultMessage 成交结果消息
type TradeResultMessage struct {
	TradeID      string `json:"trade_id"`
	Market       string `json:"market"`
	MakerOrderID string `json:"maker_order_id"`
	TakerOrderID string `json:"taker_order_id"`
	Maker        string `json:"maker"`
	Taker        string `json:"taker"`
	Side         string `json:"side"`           // taker 方向
	Price        string `json:"price"`          // 成交价格
	Size         string `json:"size"`           // 成交数量
	QuoteAmount  string `json:"quote_amount"`   // 成交金额
	MakerFee     string `json:"maker_fee"`      // maker 手续费
	TakerFee     string `json:"taker_fee"`      // taker 手续费
	Timestamp    int64  `json:"timestamp"`      // 成交时间
	Sequence     int64  `json:"sequence"`       // 序列号
	MakerIsBuyer bool   `json:"maker_is_buyer"` // maker 是否是买方
}

// OrderCancelledMessage 订单取消消息
type OrderCancelledMessage struct {
	OrderID       string `json:"order_id"`
	Market        string `json:"market"`
	Result        string `json:"result"`         // success, not_found, already_cancelled
	RemainingSize string `json:"remaining_size"` // 剩余数量
	FilledSize    string `json:"filled_size"`    // 已成交数量
	Timestamp     int64  `json:"timestamp"`
	Sequence      int64  `json:"sequence"`
}

// OrderAcceptedMessage 订单接受消息
type OrderAcceptedMessage struct {
	OrderID   string `json:"order_id"`
	Market    string `json:"market"`
	Timestamp int64  `json:"timestamp"`
	Sequence  int64  `json:"sequence"`
}

// SettlementConfirmedMessage 结算确认消息
type SettlementConfirmedMessage struct {
	SettlementID string   `json:"settlement_id"`
	TradeIDs     []string `json:"trade_ids"`
	TxHash       string   `json:"tx_hash"`
	BlockNumber  int64    `json:"block_number"`
	Status       string   `json:"status"` // confirmed, failed
	Timestamp    int64    `json:"timestamp"`
}

// DepositMessage 充值消息
type DepositMessage struct {
	TxHash      string `json:"tx_hash"`
	Wallet      string `json:"wallet"`
	Token       string `json:"token"`
	Amount      string `json:"amount"`
	BlockNumber int64  `json:"block_number"`
	Timestamp   int64  `json:"timestamp"`
}

// WithdrawalConfirmedMessage 提现确认消息
type WithdrawalConfirmedMessage struct {
	WithdrawalID string `json:"withdrawal_id"`
	TxHash       string `json:"tx_hash"`
	BlockNumber  int64  `json:"block_number"`
	Status       string `json:"status"` // confirmed, failed
	Timestamp    int64  `json:"timestamp"`
}

// ParseTradeResult 解析成交结果消息
func ParseTradeResult(data []byte) (*TradeResultMessage, error) {
	var msg TradeResultMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse trade result failed: %w", err)
	}
	return &msg, nil
}

// ParseOrderCancelled 解析订单取消消息
func ParseOrderCancelled(data []byte) (*OrderCancelledMessage, error) {
	var msg OrderCancelledMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse order cancelled failed: %w", err)
	}
	return &msg, nil
}

// ParseOrderAccepted 解析订单接受消息
func ParseOrderAccepted(data []byte) (*OrderAcceptedMessage, error) {
	var msg OrderAcceptedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse order accepted failed: %w", err)
	}
	return &msg, nil
}

// ParseSettlementConfirmed 解析结算确认消息
func ParseSettlementConfirmed(data []byte) (*SettlementConfirmedMessage, error) {
	var msg SettlementConfirmedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse settlement confirmed failed: %w", err)
	}
	return &msg, nil
}

// ParseDeposit 解析充值消息
func ParseDeposit(data []byte) (*DepositMessage, error) {
	var msg DepositMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse deposit failed: %w", err)
	}
	return &msg, nil
}

// ParseWithdrawalConfirmed 解析提现确认消息
func ParseWithdrawalConfirmed(data []byte) (*WithdrawalConfirmedMessage, error) {
	var msg WithdrawalConfirmedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse withdrawal confirmed failed: %w", err)
	}
	return &msg, nil
}
