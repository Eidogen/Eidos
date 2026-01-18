package ws

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

// Subscriber Redis Pub/Sub 订阅器
// 订阅 eidos-market 服务发布的行情数据，转发到 WebSocket 客户端
type Subscriber struct {
	hub    *Hub
	redis  *redis.Client
	logger *zap.Logger

	done chan struct{}
}

// NewSubscriber 创建订阅器
func NewSubscriber(hub *Hub, redis *redis.Client, logger *zap.Logger) *Subscriber {
	return &Subscriber{
		hub:    hub,
		redis:  redis,
		logger: logger,
		done:   make(chan struct{}),
	}
}

// Start 启动订阅
func (s *Subscriber) Start(ctx context.Context) error {
	// 订阅所有市场频道
	// TODO: 动态订阅，根据 Hub 中实际订阅情况
	patterns := []string{
		"eidos:ticker:*",
		"eidos:depth:*",
		"eidos:kline:*",
		"eidos:trades:*",
	}

	pubsub := s.redis.PSubscribe(ctx, patterns...)

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

	s.logger.Info("subscriber started", zap.Strings("patterns", patterns))
	return nil
}

// Stop 停止订阅
func (s *Subscriber) Stop() {
	close(s.done)
}

// handleMessage 处理 Redis 消息
func (s *Subscriber) handleMessage(msg *redis.Message) {
	// 解析频道: eidos:{channel}:{market}[:interval]
	parts := strings.Split(msg.Channel, ":")
	if len(parts) < 3 {
		metrics.RecordRedisMessage(msg.Channel, false, "invalid_format")
		s.logger.Warn("invalid channel format", zap.String("channel", msg.Channel))
		return
	}

	channelType := parts[1]
	market := parts[2]

	var channel Channel
	switch channelType {
	case "ticker":
		channel = ChannelTicker
	case "depth":
		channel = ChannelDepth
	case "kline":
		channel = ChannelKline
	case "trades":
		channel = ChannelTrades
	default:
		metrics.RecordRedisMessage(channelType, false, "unknown_channel")
		s.logger.Warn("unknown channel type", zap.String("type", channelType))
		return
	}

	// 解析数据
	var data interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
		metrics.RecordRedisMessage(channelType, false, "unmarshal_error")
		s.logger.Error("failed to unmarshal message",
			zap.String("channel", msg.Channel),
			zap.Error(err),
		)
		return
	}

	// 记录成功接收
	metrics.RecordRedisMessage(channelType, true, "")

	// 广播到 Hub
	serverMsg := NewUpdateMessage(channel, market, data, time.Now().UnixMilli())
	s.hub.Broadcast(channel, market, serverMsg)
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
