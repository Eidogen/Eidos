package ws

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/crypto"
)

// Client WebSocket 客户端
type Client struct {
	id     string
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *slog.Logger
	cfg    config.WebSocketConfig

	// 订阅管理
	subscriptions map[string]bool
	subMu         sync.RWMutex

	// 认证信息
	wallet        string
	authenticated bool // 是否已认证（用于私有频道）

	// 关闭控制
	closeOnce sync.Once
	closed    bool
	closedMu  sync.RWMutex
}

// NewClient 创建客户端
func NewClient(hub *Hub, conn *websocket.Conn, logger *slog.Logger, cfg config.WebSocketConfig) *Client {
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
	c.wallet = strings.ToLower(wallet)
}

// Wallet 返回钱包地址
func (c *Client) Wallet() string {
	return c.wallet
}

// SetAuthenticated 设置认证状态
func (c *Client) SetAuthenticated(auth bool) {
	c.authenticated = auth
}

// IsAuthenticated 返回是否已认证
func (c *Client) IsAuthenticated() bool {
	return c.authenticated
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
					"client", c.id,
					"error", err,
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
	case MsgTypeAuth:
		c.handleAuth(msg)
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

	// 验证市场（非私有频道需要市场参数）
	if msg.Market == "" && !isPrivateChannel(msg.Channel) {
		c.sendError(msg.ID, 400, "market is required")
		return
	}

	// 检查订阅数量限制
	if c.SubscriptionCount() >= c.cfg.MaxSubscriptions {
		c.sendError(msg.ID, 429, "max subscriptions exceeded")
		return
	}

	// 私有频道需要认证
	if isPrivateChannel(msg.Channel) {
		if !c.authenticated {
			c.sendError(msg.ID, 401, "authentication required for private channel")
			return
		}
		// 私有频道使用钱包地址作为 market key
		if msg.Market == "" {
			msg.Market = c.wallet
		}
	}

	// 订阅
	c.hub.Subscribe(c, msg.Channel, msg.Market)

	// 发送确认
	resp := NewAckMessage(msg.ID, msg.Channel, msg.Market)
	data, _ := resp.ToJSON()
	c.send <- data

	c.logger.Debug("client subscribed",
		"client", c.id,
		"channel", string(msg.Channel),
		"market", msg.Market,
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

// AuthMessage 认证消息
type AuthMessage struct {
	Wallet    string `json:"wallet"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
}

// handleAuth 处理认证请求（用于私有频道订阅）
func (c *Client) handleAuth(msg *ClientMessage) {
	// 如果已经认证，直接返回成功
	if c.authenticated {
		c.sendAuthSuccess(msg.ID)
		return
	}

	// 解析认证参数
	authData, ok := msg.Params.(map[string]interface{})
	if !ok {
		c.sendError(msg.ID, 400, "invalid auth params")
		return
	}

	wallet, _ := authData["wallet"].(string)
	timestampFloat, _ := authData["timestamp"].(float64)
	timestamp := int64(timestampFloat)
	signature, _ := authData["signature"].(string)

	if wallet == "" || timestamp == 0 || signature == "" {
		c.sendError(msg.ID, 400, "missing auth params: wallet, timestamp, signature required")
		return
	}

	// 验证时间戳（5分钟有效期）
	now := time.Now().UnixMilli()
	if now-timestamp > 300000 || timestamp-now > 300000 {
		c.sendError(msg.ID, 401, "signature expired")
		return
	}

	// 验证 EIP-712 签名
	if !c.cfg.EnablePrivateAuth {
		// 如果禁用私有认证，跳过签名验证（开发模式）
		c.wallet = strings.ToLower(wallet)
		c.authenticated = true
		c.sendAuthSuccess(msg.ID)
		return
	}

	// 验证签名
	if err := c.verifyAuthSignature(wallet, timestamp, signature); err != nil {
		c.logger.Warn("auth signature verification failed",
			"wallet", wallet,
			"error", err,
		)
		c.sendError(msg.ID, 401, "invalid signature")
		return
	}

	// 认证成功
	c.wallet = strings.ToLower(wallet)
	c.authenticated = true
	c.sendAuthSuccess(msg.ID)

	c.logger.Info("client authenticated",
		"client", c.id,
		"wallet", c.wallet,
	)
}

// verifyAuthSignature 验证认证签名
func (c *Client) verifyAuthSignature(wallet string, timestamp int64, signature string) error {
	// 构建签名消息
	message := buildAuthMessage(wallet, timestamp)

	// 解码签名
	sig, err := decodeHexSignature(signature)
	if err != nil {
		return err
	}

	// 使用简单的个人签名验证
	// 实际生产环境应该使用完整的 EIP-712 验证
	messageHash := crypto.Keccak256([]byte(message))

	valid, err := crypto.VerifyPersonalSignature(wallet, messageHash, sig)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidSignature
	}

	return nil
}

// buildAuthMessage 构建认证消息
func buildAuthMessage(wallet string, timestamp int64) string {
	return "Eidos Exchange WebSocket Authentication\n" +
		"Wallet: " + wallet + "\n" +
		"Timestamp: " + strconv.FormatInt(timestamp, 10)
}

// decodeHexSignature 解码十六进制签名
func decodeHexSignature(sig string) ([]byte, error) {
	sig = strings.TrimPrefix(sig, "0x")
	return hex.DecodeString(sig)
}

// sendAuthSuccess 发送认证成功消息
func (c *Client) sendAuthSuccess(id string) {
	resp := &ServerMessage{
		Type:    MsgTypeAuth,
		ID:      id,
		Message: "authenticated",
		Data: map[string]interface{}{
			"wallet":        c.wallet,
			"authenticated": true,
		},
	}
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
			"client", c.id,
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
				"client", c.id,
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
	case ChannelTicker, ChannelDepth, ChannelKline, ChannelTrades,
		ChannelOrders, ChannelBalances, ChannelPositions:
		return true
	default:
		return false
	}
}

// isPrivateChannel 判断是否为私有频道
func isPrivateChannel(ch Channel) bool {
	switch ch {
	case ChannelOrders, ChannelBalances, ChannelPositions:
		return true
	default:
		return false
	}
}

// SendJSON 发送 JSON 消息
func (c *Client) SendJSON(v interface{}) bool {
	data, err := json.Marshal(v)
	if err != nil {
		c.logger.Error("failed to marshal message", "error", err)
		return false
	}
	return c.Send(data)
}
