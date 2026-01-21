package kafka

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/shopspring/decimal"
)

const (
	TopicOrderUpdates   = "order-updates"
	TopicTradeResults   = "trade-results"
	TopicBalanceUpdates = "balance-updates"
	TopicWithdrawals    = "withdrawals"
)

// Consumer Kafka 消费者
type Consumer struct {
	client        sarama.ConsumerGroup
	orderCache    *cache.OrderCache
	amountCache   *cache.AmountCache
	marketCache   *cache.MarketCache
	withdrawCache *cache.WithdrawCache

	ready chan bool
	ctx   context.Context
	wg    sync.WaitGroup
}

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	Brokers []string
	GroupID string
}

// NewConsumer 创建 Kafka 消费者
func NewConsumer(
	cfg *ConsumerConfig,
	orderCache *cache.OrderCache,
	amountCache *cache.AmountCache,
	marketCache *cache.MarketCache,
	withdrawCache *cache.WithdrawCache,
) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	client, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, config)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		client:        client,
		orderCache:    orderCache,
		amountCache:   amountCache,
		marketCache:   marketCache,
		withdrawCache: withdrawCache,
		ready:         make(chan bool),
	}, nil
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context) error {
	c.ctx = ctx
	topics := []string{
		TopicOrderUpdates,
		TopicTradeResults,
		TopicBalanceUpdates,
		TopicWithdrawals,
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			if err := c.client.Consume(ctx, topics, c); err != nil {
				logger.Error("consumer error", "error", err)
			}
			if ctx.Err() != nil {
				return
			}
			c.ready = make(chan bool)
		}
	}()

	<-c.ready
	logger.Info("kafka consumer started", "topics", topics)
	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	c.wg.Wait()
	return c.client.Close()
}

// Setup 初始化
func (c *Consumer) Setup(sarama.ConsumerGroupSession) error {
	close(c.ready)
	return nil
}

// Cleanup 清理
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费消息
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			if err := c.handleMessage(c.ctx, message); err != nil {
				logger.Error("failed to handle message",
					"topic", message.Topic,
					"error", err)
			}

			session.MarkMessage(message, "")

		case <-c.ctx.Done():
			return nil
		}
	}
}

// handleMessage 处理消息
func (c *Consumer) handleMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	switch msg.Topic {
	case TopicOrderUpdates:
		return c.handleOrderUpdate(ctx, msg.Value)
	case TopicTradeResults:
		return c.handleTradeResult(ctx, msg.Value)
	case TopicBalanceUpdates:
		return c.handleBalanceUpdate(ctx, msg.Value)
	case TopicWithdrawals:
		return c.handleWithdrawal(ctx, msg.Value)
	default:
		logger.Warn("unknown topic", "topic", msg.Topic)
	}
	return nil
}

// OrderUpdateMessage 订单更新消息
type OrderUpdateMessage struct {
	OrderID string `json:"order_id"`
	Wallet  string `json:"wallet"`
	Market  string `json:"market"`
	Side    string `json:"side"`
	Price   string `json:"price"`
	Amount  string `json:"amount"`
	Status  string `json:"status"` // OPEN, PARTIAL, FILLED, CANCELLED
}

// handleOrderUpdate 处理订单更新
func (c *Consumer) handleOrderUpdate(ctx context.Context, data []byte) error {
	var msg OrderUpdateMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	price, _ := decimal.NewFromString(msg.Price)
	amount, _ := decimal.NewFromString(msg.Amount)

	switch msg.Status {
	case "OPEN", "PARTIAL":
		// 添加或更新订单缓存
		order := &cache.OrderInfo{
			OrderID: msg.OrderID,
			Market:  msg.Market,
			Side:    msg.Side,
			Price:   price,
			Amount:  amount,
		}
		if err := c.orderCache.AddOrder(ctx, msg.Wallet, order); err != nil {
			logger.Error("failed to add order to cache", "error", err)
		}
		// 增加订单计数
		c.orderCache.IncrementOrderCount(ctx, msg.Wallet)

	case "FILLED", "CANCELLED", "EXPIRED", "REJECTED":
		// 移除订单缓存
		if err := c.orderCache.RemoveOrder(ctx, msg.Wallet, msg.Market, msg.OrderID); err != nil {
			logger.Error("failed to remove order from cache", "error", err)
		}
		// 减少订单计数
		c.orderCache.DecrementOrderCount(ctx, msg.Wallet)
	}

	logger.Debug("order update processed",
		"order_id", msg.OrderID,
		"status", msg.Status)

	return nil
}

// TradeResultMessage 成交结果消息
type TradeResultMessage struct {
	TradeID     string `json:"trade_id"`
	Market      string `json:"market"`
	MakerWallet string `json:"maker_wallet"`
	TakerWallet string `json:"taker_wallet"`
	Price       string `json:"price"`
	Amount      string `json:"amount"`
	LastPrice   string `json:"last_price"`
}

// handleTradeResult 处理成交结果
func (c *Consumer) handleTradeResult(ctx context.Context, data []byte) error {
	var msg TradeResultMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	price, _ := decimal.NewFromString(msg.Price)
	amount, _ := decimal.NewFromString(msg.Amount)
	notional := price.Mul(amount)

	// 更新市场最新价格
	if msg.LastPrice != "" {
		lastPrice, _ := decimal.NewFromString(msg.LastPrice)
		c.marketCache.SetLastPrice(ctx, msg.Market, lastPrice)
	}

	// 增加 maker 和 taker 的待结算金额
	c.amountCache.AddPendingSettle(ctx, msg.MakerWallet, notional)
	c.amountCache.AddPendingSettle(ctx, msg.TakerWallet, notional)

	// 增加系统总待结算金额
	c.amountCache.AddSystemPendingSettle(ctx, notional.Mul(decimal.NewFromInt(2)))

	logger.Debug("trade result processed",
		"trade_id", msg.TradeID,
		"market", msg.Market)

	return nil
}

// BalanceUpdateMessage 余额更新消息
type BalanceUpdateMessage struct {
	Wallet     string `json:"wallet"`
	Token      string `json:"token"`
	UpdateType string `json:"update_type"` // SETTLE, DEPOSIT, WITHDRAW
	Amount     string `json:"amount"`
}

// handleBalanceUpdate 处理余额更新
func (c *Consumer) handleBalanceUpdate(ctx context.Context, data []byte) error {
	var msg BalanceUpdateMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	amount, _ := decimal.NewFromString(msg.Amount)

	switch msg.UpdateType {
	case "SETTLE":
		// 结算完成，减少待结算金额
		c.amountCache.ReducePendingSettle(ctx, msg.Wallet, amount)
		c.amountCache.ReduceSystemPendingSettle(ctx, amount)
	}

	logger.Debug("balance update processed",
		"wallet", msg.Wallet,
		"update_type", msg.UpdateType)

	return nil
}

// WithdrawalMessage 提现消息
type WithdrawalMessage struct {
	WithdrawalID string `json:"withdrawal_id"`
	Wallet       string `json:"wallet"`
	Token        string `json:"token"`
	Amount       string `json:"amount"`
	ToAddress    string `json:"to_address"`
	Status       string `json:"status"`
}

// handleWithdrawal 处理提现消息
func (c *Consumer) handleWithdrawal(ctx context.Context, data []byte) error {
	var msg WithdrawalMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}

	amount, _ := decimal.NewFromString(msg.Amount)

	switch msg.Status {
	case "CONFIRMED":
		// 提现成功，记录到历史并增加每日提现额度
		c.withdrawCache.RecordWithdraw(ctx, msg.Wallet, msg.ToAddress, 0)
		c.amountCache.AddDailyWithdraw(ctx, msg.Wallet, msg.Token, amount)

		logger.Debug("withdrawal confirmed",
			"withdrawal_id", msg.WithdrawalID,
			"wallet", msg.Wallet)
	}

	return nil
}
