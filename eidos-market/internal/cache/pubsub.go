package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// Redis Pub/Sub 频道格式
const (
	ChannelTicker = "eidos:ticker:%s"   // market
	ChannelDepth  = "eidos:depth:%s"    // market
	ChannelTrades = "eidos:trades:%s"   // market
	ChannelKline  = "eidos:kline:%s:%s" // market:interval
)

// PubSub Redis Pub/Sub 实现
// [eidos-api] 订阅以下 Redis 频道，推送到 WebSocket:
//
//	频道格式:
//	  - eidos:ticker:{market}   - Ticker 更新 (如 eidos:ticker:BTC-USDC)
//	  - eidos:depth:{market}    - 深度更新 (如 eidos:depth:BTC-USDC)
//	  - eidos:trades:{market}   - 成交流 (如 eidos:trades:BTC-USDC)
//	  - eidos:kline:{market}:{interval} - K线更新 (如 eidos:kline:BTC-USDC:1m)
//
//	消息格式: JSON 序列化的 model.Ticker / model.Depth / model.Trade / model.Kline
//
//	WebSocket 推送频率建议:
//	  - Ticker: 直接转发 (约 100ms 间隔)
//	  - Depth: 直接转发 (约 100ms 间隔)
//	  - Trades: 直接转发 (实时)
//	  - Kline: 直接转发 (约 1s 间隔)
type PubSub struct {
	client redis.UniversalClient
	logger *slog.Logger
}

// NewPubSub 创建 Pub/Sub
func NewPubSub(client redis.UniversalClient, logger *slog.Logger) *PubSub {
	return &PubSub{
		client: client,
		logger: logger.With("component", "pubsub"),
	}
}

// PublishTicker 发布 Ticker 更新
func (p *PubSub) PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error {
	channel := fmt.Sprintf(ChannelTicker, market)
	data, err := json.Marshal(ticker)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, channel, data).Err()
}

// PublishDepth 发布深度更新
func (p *PubSub) PublishDepth(ctx context.Context, market string, depth *model.Depth) error {
	channel := fmt.Sprintf(ChannelDepth, market)
	data, err := json.Marshal(depth)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, channel, data).Err()
}

// PublishTrade 发布成交
func (p *PubSub) PublishTrade(ctx context.Context, market string, trade *model.Trade) error {
	channel := fmt.Sprintf(ChannelTrades, market)
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, channel, data).Err()
}

// PublishKline 发布 K 线更新
func (p *PubSub) PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	channel := fmt.Sprintf(ChannelKline, market, interval)
	data, err := json.Marshal(kline)
	if err != nil {
		return err
	}
	return p.client.Publish(ctx, channel, data).Err()
}

// Subscribe 订阅频道
func (p *PubSub) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return p.client.Subscribe(ctx, channels...)
}

// SubscribeTicker 订阅 Ticker 频道
func (p *PubSub) SubscribeTicker(ctx context.Context, market string) *redis.PubSub {
	channel := fmt.Sprintf(ChannelTicker, market)
	return p.client.Subscribe(ctx, channel)
}

// SubscribeDepth 订阅深度频道
func (p *PubSub) SubscribeDepth(ctx context.Context, market string) *redis.PubSub {
	channel := fmt.Sprintf(ChannelDepth, market)
	return p.client.Subscribe(ctx, channel)
}

// SubscribeTrades 订阅成交频道
func (p *PubSub) SubscribeTrades(ctx context.Context, market string) *redis.PubSub {
	channel := fmt.Sprintf(ChannelTrades, market)
	return p.client.Subscribe(ctx, channel)
}

// SubscribeKline 订阅 K 线频道
func (p *PubSub) SubscribeKline(ctx context.Context, market string, interval model.KlineInterval) *redis.PubSub {
	channel := fmt.Sprintf(ChannelKline, market, interval)
	return p.client.Subscribe(ctx, channel)
}
