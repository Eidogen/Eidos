// Package kafka 提供 Kafka 生产者功能
//
// ========================================
// Kafka 生产者对接说明
// ========================================
//
// ## 生产者 (Producer) - 本服务发送的 Topic
//
// 1. Topic: deposits
//    - 消费者: eidos-trading (充值入账)
//    - 消息内容: DepositEvent (链上充值事件)
//    - 处理逻辑: Indexer 监听链上 Deposit 事件后发送
//    - 对接服务: eidos-trading
//    - TODO: eidos-trading 需要订阅此 topic 并调用 DepositService.CreditDeposit
//
// 2. Topic: settlement-confirmed
//    - 消费者: eidos-trading (结算确认)
//    - 消息内容: SettlementConfirmation (链上结算确认)
//    - 处理逻辑: 链上交易确认后发送，包含 txHash, blockNumber
//    - 对接服务: eidos-trading
//    - TODO: eidos-trading 需要订阅此 topic 并更新成交状态为 SETTLED_ONCHAIN
//
// 3. Topic: withdrawal-confirmed
//    - 消费者: eidos-trading (提现确认)
//    - 消息内容: WithdrawalConfirmation (链上提现确认)
//    - 处理逻辑: 链上提现交易确认后发送
//    - 对接服务: eidos-trading
//    - TODO: eidos-trading 需要订阅此 topic 并更新提现状态为 COMPLETED
//
// ========================================
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
)

// Kafka 生产者发送的 Topic
//
// Topic 命名遵循 00-协议总表.md 规范 (无前缀，短横线分隔)
const (
	// TopicDeposits 充值事件 Topic
	// 生产者: eidos-chain (Indexer)
	// 消费者: eidos-trading
	// Partition Key: tx_hash
	// 消息格式: model.DepositEvent
	TopicDeposits = "deposits"

	// TopicSettlementConfirmed 结算确认 Topic
	// 生产者: eidos-chain (Settlement)
	// 消费者: eidos-trading
	// Partition Key: batch_id
	// 消息格式: model.SettlementConfirmation
	TopicSettlementConfirmed = "settlement-confirmed"

	// TopicWithdrawalConfirmed 提现确认 Topic
	// 生产者: eidos-chain (Withdrawal)
	// 消费者: eidos-trading
	// Partition Key: withdraw_id
	// 消息格式: model.WithdrawalConfirmation
	TopicWithdrawalConfirmed = "withdrawal-confirmed"
)

// Producer Kafka 生产者
type Producer struct {
	producer sarama.SyncProducer
	mu       sync.RWMutex
	closed   bool
}

// ProducerConfig 生产者配置
type ProducerConfig struct {
	Brokers      []string
	ClientID     string
	RequiredAcks sarama.RequiredAcks
	MaxRetries   int
	RetryBackoff time.Duration
}

// NewProducer 创建生产者
func NewProducer(cfg *ProducerConfig) (*Producer, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_8_0_0
	config.ClientID = cfg.ClientID
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	requiredAcks := cfg.RequiredAcks
	if requiredAcks == 0 {
		requiredAcks = sarama.WaitForAll
	}
	config.Producer.RequiredAcks = requiredAcks

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	config.Producer.Retry.Max = maxRetries

	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = 100 * time.Millisecond
	}
	config.Producer.Retry.Backoff = retryBackoff

	producer, err := sarama.NewSyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, err
	}

	return &Producer{
		producer: producer,
	}, nil
}

// Close 关闭生产者
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	return p.producer.Close()
}

// send 发送消息
func (p *Producer) send(topic string, key string, value []byte) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return errors.New("producer is closed")
	}
	p.mu.RUnlock()

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		logger.Error("failed to send kafka message",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err))
		return err
	}

	logger.Debug("kafka message sent",
		zap.String("topic", topic),
		zap.String("key", key),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// SendDepositEvent 发送充值事件
func (p *Producer) SendDepositEvent(ctx context.Context, deposit *model.DepositEvent) error {
	data, err := json.Marshal(deposit)
	if err != nil {
		return err
	}

	return p.send(TopicDeposits, deposit.Wallet, data)
}

// SendSettlementConfirmation 发送结算确认事件
func (p *Producer) SendSettlementConfirmation(ctx context.Context, confirmation *model.SettlementConfirmation) error {
	data, err := json.Marshal(confirmation)
	if err != nil {
		return err
	}

	return p.send(TopicSettlementConfirmed, confirmation.BatchID, data)
}

// SendWithdrawalConfirmation 发送提现确认事件
func (p *Producer) SendWithdrawalConfirmation(ctx context.Context, confirmation *model.WithdrawalConfirmation) error {
	data, err := json.Marshal(confirmation)
	if err != nil {
		return err
	}

	return p.send(TopicWithdrawalConfirmed, confirmation.WithdrawID, data)
}

// EventPublisher 事件发布器接口
type EventPublisher interface {
	PublishDeposit(ctx context.Context, deposit *model.DepositEvent) error
	PublishSettlementConfirmation(ctx context.Context, confirmation *model.SettlementConfirmation) error
	PublishWithdrawalConfirmation(ctx context.Context, confirmation *model.WithdrawalConfirmation) error
}

// KafkaEventPublisher Kafka 事件发布器
type KafkaEventPublisher struct {
	producer *Producer
}

// NewKafkaEventPublisher 创建 Kafka 事件发布器
func NewKafkaEventPublisher(producer *Producer) *KafkaEventPublisher {
	return &KafkaEventPublisher{
		producer: producer,
	}
}

func (p *KafkaEventPublisher) PublishDeposit(ctx context.Context, deposit *model.DepositEvent) error {
	return p.producer.SendDepositEvent(ctx, deposit)
}

func (p *KafkaEventPublisher) PublishSettlementConfirmation(ctx context.Context, confirmation *model.SettlementConfirmation) error {
	return p.producer.SendSettlementConfirmation(ctx, confirmation)
}

func (p *KafkaEventPublisher) PublishWithdrawalConfirmation(ctx context.Context, confirmation *model.WithdrawalConfirmation) error {
	return p.producer.SendWithdrawalConfirmation(ctx, confirmation)
}
