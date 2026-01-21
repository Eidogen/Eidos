// Package kafka 提供 Kafka 消费者和生产者功能
//
// ========================================
// Kafka 消息流对接说明
// ========================================
//
// ## 消费者 (Consumer) - 本服务订阅的 Topic
//
// 1. Topic: settlements
//    - 生产者: eidos-trading (清算服务)
//    - 消息内容: SettlementTrade (成交记录，需要上链结算)
//    - 处理逻辑: 收集成交，批量提交到区块链
//    - 对接服务: eidos-trading
//    - 确认 eidos-trading 的 trade-results 处理后发送到此 topic
//
// 2. Topic: withdrawals
//    - 生产者: eidos-trading (提现服务)
//    - 消息内容: WithdrawalRequest (提现请求)
//    - 处理逻辑: 验证签名，调用合约执行提现
//    - 对接服务: eidos-trading
//    - 确认 eidos-trading 提现审核通过后发送到此 topic
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
	"github.com/eidos-exchange/eidos/eidos-chain/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Kafka 消费者订阅的 Topic
//
// Topic 命名遵循 00-协议总表.md 规范 (无前缀，短横线分隔)
const (
	// TopicSettlements 结算消息 Topic
	// 生产者: eidos-trading
	// 消费者: eidos-chain
	// Partition Key: batch_id
	// 消息格式: model.SettlementTrade
	TopicSettlements = "settlements"

	// TopicWithdrawals 提现请求 Topic
	// 生产者: eidos-trading
	// 消费者: eidos-chain
	// Partition Key: wallet
	// 消息格式: model.WithdrawalRequest
	TopicWithdrawals = "withdrawals"
)

// Consumer Kafka 消费者
type Consumer struct {
	client           sarama.ConsumerGroup
	settlementSvc    *service.SettlementService
	withdrawalSvc    *service.WithdrawalService
	topics           []string
	groupID          string

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers           []string
	GroupID           string
	SettlementService *service.SettlementService
	WithdrawalService *service.WithdrawalService
}

// NewConsumer 创建消费者
func NewConsumer(cfg *ConsumerConfig) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_8_0_0
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = time.Second

	client, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, config)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		client:        client,
		settlementSvc: cfg.SettlementService,
		withdrawalSvc: cfg.WithdrawalService,
		topics:        []string{TopicSettlements, TopicWithdrawals},
		groupID:       cfg.GroupID,
	}, nil
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.New("consumer already running")
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	handler := &consumerGroupHandler{
		settlementSvc: c.settlementSvc,
		withdrawalSvc: c.withdrawalSvc,
	}

	go func() {
		for {
			select {
			case <-c.stopCh:
				return
			case <-ctx.Done():
				return
			default:
			}

			if err := c.client.Consume(ctx, c.topics, handler); err != nil {
				logger.Error("kafka consume error", "error", err)
				time.Sleep(time.Second)
			}
		}
	}()

	logger.Info("kafka consumer started",
		"topics", c.topics,
		"group_id", c.groupID)

	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	close(c.stopCh)
	c.running = false

	return c.client.Close()
}

// consumerGroupHandler 消费组处理器
type consumerGroupHandler struct {
	settlementSvc *service.SettlementService
	withdrawalSvc *service.WithdrawalService
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		ctx := context.Background()

		switch msg.Topic {
		case TopicSettlements:
			if err := h.handleSettlement(ctx, msg.Value); err != nil {
				logger.Error("failed to handle settlement message",
					"topic", msg.Topic,
					"offset", msg.Offset,
					"error", err)
				continue // 继续处理下一条消息
			}

		case TopicWithdrawals:
			if err := h.handleWithdrawal(ctx, msg.Value); err != nil {
				logger.Error("failed to handle withdrawal message",
					"topic", msg.Topic,
					"offset", msg.Offset,
					"error", err)
				continue
			}

		default:
			logger.Warn("unknown topic", "topic", msg.Topic)
		}

		session.MarkMessage(msg, "")
	}
	return nil
}

func (h *consumerGroupHandler) handleSettlement(ctx context.Context, data []byte) error {
	var trade model.SettlementTrade
	if err := json.Unmarshal(data, &trade); err != nil {
		return err
	}

	logger.Debug("received settlement trade",
		"trade_id", trade.TradeID,
		"market", trade.Market)

	return h.settlementSvc.AddTrade(ctx, &trade)
}

func (h *consumerGroupHandler) handleWithdrawal(ctx context.Context, data []byte) error {
	var req model.WithdrawalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	logger.Debug("received withdrawal request",
		"withdraw_id", req.WithdrawID,
		"wallet", req.Wallet)

	return h.withdrawalSvc.ProcessWithdrawalRequest(ctx, &req)
}
