package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

// Subscriber Redis Pub/Sub 订阅器
// 订阅 eidos-market 服务发布的行情数据，转发到 WebSocket 客户端
type Subscriber struct {
	hub    *Hub
	redis  *redis.Client
	logger *slog.Logger

	done chan struct{}
}

// NewSubscriber 创建订阅器
func NewSubscriber(hub *Hub, redis *redis.Client, logger *slog.Logger) *Subscriber {
	return &Subscriber{
		hub:    hub,
		redis:  redis,
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Start 启动订阅
func (s *Subscriber) Start(ctx context.Context) error {
	// 订阅公开行情频道
	publicPatterns := []string{
		"eidos:ticker:*",
		"eidos:depth:*",
		"eidos:kline:*",
		"eidos:trades:*",
	}

	// 订阅私有频道
	privatePatterns := []string{
		"eidos:orders:*",
		"eidos:balances:*",
		"eidos:positions:*",
	}

	allPatterns := append(publicPatterns, privatePatterns...)
	pubsub := s.redis.PSubscribe(ctx, allPatterns...)

	go func() {
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case msg := <-ch:
				s.handleMessage(msg)
			case <-s.done:
				s.logger.Info("subscriber stopping")
				return
			case <-ctx.Done():
				s.logger.Info("subscriber context cancelled")
				return
			}
		}
	}()

	s.logger.Info("subscriber started", "patterns", allPatterns)
	return nil
}

// Stop 停止订阅
func (s *Subscriber) Stop() {
	close(s.done)
}

// handleMessage 处理 Redis 消息
func (s *Subscriber) handleMessage(msg *redis.Message) {
	// 解析频道: eidos:{channel}:{market}[:interval]
	// 或私有频道: eidos:{channel}:{wallet}
	parts := strings.Split(msg.Channel, ":")
	if len(parts) < 3 {
		metrics.RecordRedisMessage(msg.Channel, false, "invalid_format")
		s.logger.Warn("invalid channel format", "channel", msg.Channel)
		return
	}

	channelType := parts[1]
	target := parts[2] // market for public, wallet for private

	var channel Channel
	var isPrivate bool
	switch channelType {
	case "ticker":
		channel = ChannelTicker
	case "depth":
		channel = ChannelDepth
	case "kline":
		channel = ChannelKline
	case "trades":
		channel = ChannelTrades
	case "orders":
		channel = ChannelOrders
		isPrivate = true
	case "balances":
		channel = ChannelBalances
		isPrivate = true
	case "positions":
		channel = ChannelPositions
		isPrivate = true
	default:
		metrics.RecordRedisMessage(channelType, false, "unknown_channel")
		s.logger.Warn("unknown channel type", "type", channelType)
		return
	}

	// 解析数据
	var data interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
		metrics.RecordRedisMessage(channelType, false, "unmarshal_error")
		s.logger.Error("failed to unmarshal message",
			"channel", msg.Channel,
			"error", err,
		)
		return
	}

	// 记录成功接收
	metrics.RecordRedisMessage(channelType, true, "")

	// 广播到 Hub
	timestamp := time.Now().UnixMilli()
	serverMsg := NewUpdateMessage(channel, target, data, timestamp)

	if isPrivate {
		// 私有频道：使用钱包地址作为 target
		s.hub.BroadcastPrivate(channel, target, serverMsg)
	} else {
		// 公开频道：使用市场作为 target
		s.hub.Broadcast(channel, target, serverMsg)
	}
}

// PublishTicker 发布 Ticker 更新（用于测试）
func (s *Subscriber) PublishTicker(ctx context.Context, market string, ticker *TickerData) error {
	data, err := json.Marshal(ticker)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:ticker:"+market, data).Err()
}

// PublishDepth 发布深度更新（用于测试）
func (s *Subscriber) PublishDepth(ctx context.Context, market string, depth *DepthData) error {
	data, err := json.Marshal(depth)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:depth:"+market, data).Err()
}

// PublishKline 发布 K线更新（用于测试）
func (s *Subscriber) PublishKline(ctx context.Context, market, interval string, kline *KlineData) error {
	data, err := json.Marshal(kline)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:kline:"+market+":"+interval, data).Err()
}

// PublishTrade 发布成交更新（用于测试）
func (s *Subscriber) PublishTrade(ctx context.Context, market string, trade *TradeData) error {
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:trades:"+market, data).Err()
}

// PublishOrderUpdate 发布订单更新（私有频道）
func (s *Subscriber) PublishOrderUpdate(ctx context.Context, wallet string, order *OrderData) error {
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:orders:"+strings.ToLower(wallet), data).Err()
}

// PublishBalanceUpdate 发布余额更新（私有频道）
func (s *Subscriber) PublishBalanceUpdate(ctx context.Context, wallet string, balance *BalanceData) error {
	data, err := json.Marshal(balance)
	if err != nil {
		return err
	}
	return s.redis.Publish(ctx, "eidos:balances:"+strings.ToLower(wallet), data).Err()
}

// BalanceData 余额数据（私有频道）
type BalanceData struct {
	Wallet    string `json:"wallet"`
	Token     string `json:"token"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
	Total     string `json:"total"`
	UpdatedAt int64  `json:"updated_at"`
}

// PositionData 仓位数据（私有频道，预留）
type PositionData struct {
	Wallet    string `json:"wallet"`
	Market    string `json:"market"`
	Size      string `json:"size"`
	Side      string `json:"side"`
	EntryPrice string `json:"entry_price"`
	UpdatedAt int64  `json:"updated_at"`
}
