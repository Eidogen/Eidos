// Package publisher 提供 Kafka 消息发布功能
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// BalancePublisher 余额变更发布者
// 发布消息到 balance-updates topic，供 eidos-api WebSocket 推送
type BalancePublisher struct {
	producer KafkaProducer
}

// NewBalancePublisher 创建余额发布者
func NewBalancePublisher(producer KafkaProducer) *BalancePublisher {
	return &BalancePublisher{
		producer: producer,
	}
}

// BalanceUpdateMessage 余额变更消息
type BalanceUpdateMessage struct {
	Wallet    string `json:"wallet"`
	Token     string `json:"token"`
	Available string `json:"available"` // 可用余额
	Frozen    string `json:"frozen"`    // 冻结金额
	Pending   string `json:"pending"`   // 待结算
	Total     string `json:"total"`     // 总余额 = available + frozen + pending
	EventType string `json:"event_type"` // deposit, withdraw, trade, freeze, unfreeze, settle
	EventID   string `json:"event_id,omitempty"` // 关联事件 ID (订单ID/交易ID/充提ID)
	Timestamp int64  `json:"timestamp"`
}

// BalanceSnapshot 余额快照
type BalanceSnapshot struct {
	Wallet    string
	Token     string
	Available decimal.Decimal
	Frozen    decimal.Decimal
	Pending   decimal.Decimal
}

// PublishBalanceUpdate 发布余额变更
func (p *BalancePublisher) PublishBalanceUpdate(ctx context.Context, snapshot *BalanceSnapshot, eventType, eventID string) error {
	if p.producer == nil {
		return nil // Kafka 未启用
	}

	total := snapshot.Available.Add(snapshot.Frozen).Add(snapshot.Pending)

	msg := &BalanceUpdateMessage{
		Wallet:    snapshot.Wallet,
		Token:     snapshot.Token,
		Available: snapshot.Available.String(),
		Frozen:    snapshot.Frozen.String(),
		Pending:   snapshot.Pending.String(),
		Total:     total.String(),
		EventType: eventType,
		EventID:   eventID,
		Timestamp: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal balance update message: %w", err)
	}

	// 使用 wallet:token 作为 key，保证同一用户同一代币的消息有序
	key := []byte(fmt.Sprintf("%s:%s", snapshot.Wallet, snapshot.Token))

	if err := p.producer.SendWithContext(ctx, kafka.TopicBalanceUpdates, key, data); err != nil {
		logger.Error("publish balance update failed",
			zap.String("wallet", snapshot.Wallet),
			zap.String("token", snapshot.Token),
			zap.Error(err),
		)
		return fmt.Errorf("send balance update: %w", err)
	}

	logger.Debug("balance update published",
		zap.String("wallet", snapshot.Wallet),
		zap.String("token", snapshot.Token),
		zap.String("event_type", eventType),
	)

	return nil
}

// PublishDeposit 发布充值余额变更
func (p *BalancePublisher) PublishDeposit(ctx context.Context, snapshot *BalanceSnapshot, depositID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "deposit", depositID)
}

// PublishWithdraw 发布提现余额变更
func (p *BalancePublisher) PublishWithdraw(ctx context.Context, snapshot *BalanceSnapshot, withdrawalID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "withdraw", withdrawalID)
}

// PublishFreeze 发布冻结余额变更 (下单冻结)
func (p *BalancePublisher) PublishFreeze(ctx context.Context, snapshot *BalanceSnapshot, orderID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "freeze", orderID)
}

// PublishUnfreeze 发布解冻余额变更 (取消订单/订单成交)
func (p *BalancePublisher) PublishUnfreeze(ctx context.Context, snapshot *BalanceSnapshot, orderID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "unfreeze", orderID)
}

// PublishTrade 发布成交余额变更
func (p *BalancePublisher) PublishTrade(ctx context.Context, snapshot *BalanceSnapshot, tradeID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "trade", tradeID)
}

// PublishSettle 发布结算余额变更
func (p *BalancePublisher) PublishSettle(ctx context.Context, snapshot *BalanceSnapshot, settlementID string) error {
	return p.PublishBalanceUpdate(ctx, snapshot, "settle", settlementID)
}
