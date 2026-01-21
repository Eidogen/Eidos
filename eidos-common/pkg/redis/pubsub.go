package redis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

var (
	// ErrPubSubClosed PubSub 已关闭
	ErrPubSubClosed = errors.New("pubsub is closed")
	// ErrSubscriptionNotFound 订阅未找到
	ErrSubscriptionNotFound = errors.New("subscription not found")
	// ErrHandlerNil 处理器为空
	ErrHandlerNil = errors.New("handler is nil")
)

// MessageHandler 消息处理器
type MessageHandler func(channel string, payload string)

// PatternMessageHandler 模式消息处理器
type PatternMessageHandler func(pattern, channel string, payload string)

// Subscription 订阅信息
type Subscription struct {
	Channel string
	Pattern bool
	Handler interface{} // MessageHandler 或 PatternMessageHandler
	PubSub  *redis.PubSub
	cancel  context.CancelFunc
}

// PubSubManager PubSub 管理器
type PubSubManager struct {
	client        redis.UniversalClient
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
	closed        int32
}

// NewPubSubManager 创建 PubSub 管理器
func NewPubSubManager(client redis.UniversalClient) *PubSubManager {
	return &PubSubManager{
		client:        client,
		subscriptions: make(map[string]*Subscription),
	}
}

// Subscribe 订阅频道
func (m *PubSubManager) Subscribe(ctx context.Context, channel string, handler MessageHandler) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrPubSubClosed
	}

	if handler == nil {
		return ErrHandlerNil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已订阅
	if _, exists := m.subscriptions[channel]; exists {
		return nil // 已订阅，直接返回
	}

	// 创建订阅
	pubsub := m.client.Subscribe(ctx, channel)

	// 等待订阅确认
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return err
	}

	// 创建取消上下文
	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		Channel: channel,
		Pattern: false,
		Handler: handler,
		PubSub:  pubsub,
		cancel:  cancel,
	}

	m.subscriptions[channel] = sub

	// 启动消息处理协程
	go m.handleMessages(subCtx, sub)

	logger.Info("subscribed to channel", "channel", channel)
	return nil
}

// PSubscribe 模式订阅
func (m *PubSubManager) PSubscribe(ctx context.Context, pattern string, handler PatternMessageHandler) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrPubSubClosed
	}

	if handler == nil {
		return ErrHandlerNil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := "pattern:" + pattern

	// 检查是否已订阅
	if _, exists := m.subscriptions[key]; exists {
		return nil
	}

	// 创建模式订阅
	pubsub := m.client.PSubscribe(ctx, pattern)

	// 等待订阅确认
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return err
	}

	// 创建取消上下文
	subCtx, cancel := context.WithCancel(context.Background())

	sub := &Subscription{
		Channel: pattern,
		Pattern: true,
		Handler: handler,
		PubSub:  pubsub,
		cancel:  cancel,
	}

	m.subscriptions[key] = sub

	// 启动消息处理协程
	go m.handlePatternMessages(subCtx, sub)

	logger.Info("subscribed to pattern", "pattern", pattern)
	return nil
}

// handleMessages 处理普通消息
func (m *PubSubManager) handleMessages(ctx context.Context, sub *Subscription) {
	handler := sub.Handler.(MessageHandler)
	ch := sub.PubSub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			m.safeHandleMessage(func() {
				handler(msg.Channel, msg.Payload)
			})
		}
	}
}

// handlePatternMessages 处理模式消息
func (m *PubSubManager) handlePatternMessages(ctx context.Context, sub *Subscription) {
	handler := sub.Handler.(PatternMessageHandler)
	ch := sub.PubSub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			m.safeHandleMessage(func() {
				handler(msg.Pattern, msg.Channel, msg.Payload)
			})
		}
	}
}

// safeHandleMessage 安全处理消息 (捕获 panic)
func (m *PubSubManager) safeHandleMessage(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic in message handler", "recover", r)
		}
	}()
	fn()
}

// Unsubscribe 取消订阅
func (m *PubSubManager) Unsubscribe(ctx context.Context, channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, exists := m.subscriptions[channel]
	if !exists {
		return ErrSubscriptionNotFound
	}

	// 取消处理协程
	sub.cancel()

	// 关闭 PubSub
	if err := sub.PubSub.Unsubscribe(ctx, channel); err != nil {
		logger.Warn("unsubscribe failed", "channel", channel, "error", err)
	}
	sub.PubSub.Close()

	delete(m.subscriptions, channel)

	logger.Info("unsubscribed from channel", "channel", channel)
	return nil
}

// PUnsubscribe 取消模式订阅
func (m *PubSubManager) PUnsubscribe(ctx context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := "pattern:" + pattern
	sub, exists := m.subscriptions[key]
	if !exists {
		return ErrSubscriptionNotFound
	}

	// 取消处理协程
	sub.cancel()

	// 关闭 PubSub
	if err := sub.PubSub.PUnsubscribe(ctx, pattern); err != nil {
		logger.Warn("punsubscribe failed", "pattern", pattern, "error", err)
	}
	sub.PubSub.Close()

	delete(m.subscriptions, key)

	logger.Info("unsubscribed from pattern", "pattern", pattern)
	return nil
}

// Publish 发布消息
func (m *PubSubManager) Publish(ctx context.Context, channel string, message interface{}) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrPubSubClosed
	}

	return m.client.Publish(ctx, channel, message).Err()
}

// GetSubscriptions 获取所有订阅
func (m *PubSubManager) GetSubscriptions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channels := make([]string, 0, len(m.subscriptions))
	for ch := range m.subscriptions {
		channels = append(channels, ch)
	}
	return channels
}

// Close 关闭管理器
func (m *PubSubManager) Close() {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, sub := range m.subscriptions {
		sub.cancel()
		sub.PubSub.Close()
		delete(m.subscriptions, key)
	}

	logger.Info("pubsub manager closed")
}

// PubSubStats PubSub 统计信息
type PubSubStats struct {
	Subscriptions     int
	PatternSubscriptions int
}

// Stats 获取统计信息
func (m *PubSubManager) Stats() *PubSubStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &PubSubStats{}
	for key, sub := range m.subscriptions {
		if sub.Pattern {
			stats.PatternSubscriptions++
		} else {
			_ = key // 避免未使用变量警告
			stats.Subscriptions++
		}
	}
	return stats
}

// ChannelSubscriber 频道订阅器 (便捷封装)
type ChannelSubscriber struct {
	manager *PubSubManager
	channel string
	handler MessageHandler
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewChannelSubscriber 创建频道订阅器
func NewChannelSubscriber(manager *PubSubManager, channel string, handler MessageHandler) *ChannelSubscriber {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelSubscriber{
		manager: manager,
		channel: channel,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动订阅
func (s *ChannelSubscriber) Start() error {
	return s.manager.Subscribe(s.ctx, s.channel, s.handler)
}

// Stop 停止订阅
func (s *ChannelSubscriber) Stop() error {
	s.cancel()
	return s.manager.Unsubscribe(context.Background(), s.channel)
}

// Publisher 发布器
type Publisher struct {
	client  redis.UniversalClient
	channel string
}

// NewPublisher 创建发布器
func NewPublisher(client redis.UniversalClient, channel string) *Publisher {
	return &Publisher{
		client:  client,
		channel: channel,
	}
}

// Publish 发布消息
func (p *Publisher) Publish(ctx context.Context, message interface{}) error {
	return p.client.Publish(ctx, p.channel, message).Err()
}

// PublishWithRetry 发布消息 (带重试)
func (p *Publisher) PublishWithRetry(ctx context.Context, message interface{}, maxRetries int, retryInterval time.Duration) error {
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		if err := p.Publish(ctx, message); err != nil {
			lastErr = err
			logger.Warn("publish failed, retrying",
				"channel", p.channel,
				"attempt", i+1,
				"error", err,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}
		return nil
	}
	return lastErr
}

// StreamPublisher 流式发布器 (支持批量)
type StreamPublisher struct {
	client    redis.UniversalClient
	channel   string
	batchSize int
	buffer    []interface{}
	mu        sync.Mutex
}

// NewStreamPublisher 创建流式发布器
func NewStreamPublisher(client redis.UniversalClient, channel string, batchSize int) *StreamPublisher {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &StreamPublisher{
		client:    client,
		channel:   channel,
		batchSize: batchSize,
		buffer:    make([]interface{}, 0, batchSize),
	}
}

// Add 添加消息
func (p *StreamPublisher) Add(message interface{}) {
	p.mu.Lock()
	p.buffer = append(p.buffer, message)
	p.mu.Unlock()
}

// Flush 发送所有缓冲消息
func (p *StreamPublisher) Flush(ctx context.Context) error {
	p.mu.Lock()
	messages := p.buffer
	p.buffer = make([]interface{}, 0, p.batchSize)
	p.mu.Unlock()

	if len(messages) == 0 {
		return nil
	}

	// 使用管道批量发送
	_, err := p.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, msg := range messages {
			pipe.Publish(ctx, p.channel, msg)
		}
		return nil
	})

	if err != nil {
		logger.Error("stream publish failed",
			"channel", p.channel,
			"count", len(messages),
			"error", err,
		)
	}

	return err
}
