package ws

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

// Client WebSocket 客户端
type Client struct {
	id     string
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *zap.Logger
	cfg    config.WebSocketConfig

	// 订阅管理
	subscriptions map[string]bool
	subMu         sync.RWMutex

	// 认证信息（可选）
	wallet string

	// 关闭控制
	closeOnce sync.Once
	closed    bool
	closedMu  sync.RWMutex
}

// NewClient 创建客户端
func NewClient(hub *Hub, conn *websocket.Conn, logger *zap.Logger, cfg config.WebSocketConfig) *Client {
	return &Client{
		id:            uuid.New().String(),
		hub:           hub,
		conn:          conn,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}
}

// ID 返回客户端 ID
func (c *Client) ID() string {
	return c.id
}

// SetWallet 设置钱包地址
func (c *Client) SetWallet(wallet string) {
	c.wallet = wallet
}

// Wallet 返回钱包地址
func (c *Client) Wallet() string {
	return c.wallet
}

// AddSubscription 添加订阅
func (c *Client) AddSubscription(key string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	c.subscriptions[key] = true
}

// RemoveSubscription 移除订阅
func (c *Client) RemoveSubscription(key string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	delete(c.subscriptions, key)
}

// Subscriptions 返回所有订阅
func (c *Client) Subscriptions() []string {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	keys := make([]string, 0, len(c.subscriptions))
	for k := range c.subscriptions {
		keys = append(keys, k)
	}
	return keys
}

// SubscriptionCount 返回订阅数量
func (c *Client) SubscriptionCount() int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscriptions)
}

// ReadPump 读取消息
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(int64(c.cfg.MaxMessageSize))
	c.conn.SetReadDeadline(time.Now().Add(c.cfg.PongTimeout()))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.cfg.PongTimeout()))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Debug("websocket read error",
					zap.String("client", c.id),
					zap.Error(err),
				)
			}
			break
		}

		c.handleMessage(message)
	}
}

// WritePump 写入消息
func (c *Client) WritePump() {
	ticker := time.NewTicker(c.cfg.PingInterval())
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteWait()))
			if !ok {
				// Hub 关闭了 channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量发送队列中的消息
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteWait()))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 处理消息
func (c *Client) handleMessage(data []byte) {
	msg, err := ParseClientMessage(data)
	if err != nil {
		c.sendError("", 400, "invalid message format")
		return
	}

	switch msg.Type {
	case MsgTypePing:
		c.handlePing()
	case MsgTypeSubscribe:
		c.handleSubscribe(msg)
	case MsgTypeUnsubscribe:
		c.handleUnsubscribe(msg)
	default:
		c.sendError(msg.ID, 400, "unknown message type")
	}
}

// handlePing 处理 Ping
func (c *Client) handlePing() {
	resp := NewPongMessage()
	data, _ := resp.ToJSON()
	c.send <- data
}

// handleSubscribe 处理订阅
func (c *Client) handleSubscribe(msg *ClientMessage) {
	// 验证频道
	if !isValidChannel(msg.Channel) {
		c.sendError(msg.ID, 400, "invalid channel")
		return
	}

	// 验证市场
	if msg.Market == "" && msg.Channel != ChannelOrders {
		c.sendError(msg.ID, 400, "market is required")
		return
	}

	// 检查订阅数量限制
	if c.SubscriptionCount() >= c.cfg.MaxSubscriptions {
		c.sendError(msg.ID, 429, "max subscriptions exceeded")
		return
	}

	// 私有频道需要认证
	if msg.Channel == ChannelOrders && c.wallet == "" {
		c.sendError(msg.ID, 401, "authentication required for private channel")
		return
	}

	// 订阅
	c.hub.Subscribe(c, msg.Channel, msg.Market)

	// 发送确认
	resp := NewAckMessage(msg.ID, msg.Channel, msg.Market)
	data, _ := resp.ToJSON()
	c.send <- data

	c.logger.Debug("client subscribed",
		zap.String("client", c.id),
		zap.String("channel", string(msg.Channel)),
		zap.String("market", msg.Market),
	)
}

// handleUnsubscribe 处理取消订阅
func (c *Client) handleUnsubscribe(msg *ClientMessage) {
	c.hub.Unsubscribe(c, msg.Channel, msg.Market)

	// 发送确认
	resp := NewAckMessage(msg.ID, msg.Channel, msg.Market)
	data, _ := resp.ToJSON()
	c.send <- data
}

// sendError 发送错误消息
func (c *Client) sendError(id string, code int, message string) {
	resp := NewErrorMessage(id, code, message)
	data, _ := resp.ToJSON()
	select {
	case c.send <- data:
	default:
		c.logger.Warn("client send buffer full, dropping error message",
			zap.String("client", c.id),
		)
	}
}

// Send 发送消息（线程安全）
func (c *Client) Send(data []byte) bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()

	if c.closed {
		return false
	}

	select {
	case c.send <- data:
		return true
	default:
		if c.logger != nil {
			c.logger.Warn("client send buffer full",
				zap.String("client", c.id),
			)
		}
		return false
	}
}

// Close 关闭客户端（原子操作：标记关闭并关闭发送通道）
func (c *Client) Close() {
	c.closedMu.Lock()
	defer c.closedMu.Unlock()

	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// MarkClosed 标记客户端已关闭（不关闭通道，用于外部已关闭通道的情况）
func (c *Client) MarkClosed() {
	c.closedMu.Lock()
	c.closed = true
	c.closedMu.Unlock()
}

// IsClosed 检查客户端是否已关闭
func (c *Client) IsClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed
}

// isValidChannel 验证频道
func isValidChannel(ch Channel) bool {
	switch ch {
	case ChannelTicker, ChannelDepth, ChannelKline, ChannelTrades, ChannelOrders:
		return true
	default:
		return false
	}
}
