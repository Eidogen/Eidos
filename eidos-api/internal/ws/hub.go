package ws

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

// Hub WebSocket 连接管理中心
type Hub struct {
	logger *zap.Logger

	// 客户端管理
	clients    map[*Client]bool
	clientsMu  sync.RWMutex
	register   chan *Client
	unregister chan *Client

	// 订阅管理: channel:market -> clients
	subscriptions map[string]map[*Client]bool
	subMu         sync.RWMutex

	// 广播
	broadcast chan *BroadcastMessage

	// 控制
	done chan struct{}
}

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	Channel Channel
	Market  string
	Message *ServerMessage
}

// NewHub 创建 Hub
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		logger:        logger,
		clients:       make(map[*Client]bool),
		register:      make(chan *Client, 256),
		unregister:    make(chan *Client, 256),
		subscriptions: make(map[string]map[*Client]bool),
		broadcast:     make(chan *BroadcastMessage, 1024),
		done:          make(chan struct{}),
	}
}

// Run 运行 Hub
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.clientsMu.Lock()
			h.clients[client] = true
			h.clientsMu.Unlock()
			metrics.RecordWSConnection(true)
			h.logger.Debug("client registered",
				zap.String("id", client.id),
				zap.Int("total", len(h.clients)),
			)

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				// 原子操作：标记关闭并关闭 channel，防止竞态
				client.Close()
				metrics.RecordWSConnection(false)
			}
			h.clientsMu.Unlock()

			// 清理订阅
			h.removeClientFromAllSubscriptions(client)
			h.logger.Debug("client unregistered",
				zap.String("id", client.id),
				zap.Int("total", len(h.clients)),
			)

		case msg := <-h.broadcast:
			h.broadcastToSubscribers(msg)

		case <-ticker.C:
			// 定期统计
			h.logStats()

		case <-h.done:
			h.logger.Info("hub stopping")
			h.closeAllClients()
			return
		}
	}
}

// Stop 停止 Hub
func (h *Hub) Stop() {
	close(h.done)
}

// Register 注册客户端
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 注销客户端
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Subscribe 订阅
func (h *Hub) Subscribe(client *Client, channel Channel, market string) {
	key := h.subKey(channel, market)

	h.subMu.Lock()
	defer h.subMu.Unlock()

	if h.subscriptions[key] == nil {
		h.subscriptions[key] = make(map[*Client]bool)
	}
	h.subscriptions[key][client] = true
	client.AddSubscription(key)
	metrics.RecordWSSubscription(string(channel), true)

	h.logger.Debug("client subscribed",
		zap.String("client", client.id),
		zap.String("channel", string(channel)),
		zap.String("market", market),
	)
}

// Unsubscribe 取消订阅
func (h *Hub) Unsubscribe(client *Client, channel Channel, market string) {
	key := h.subKey(channel, market)

	h.subMu.Lock()
	defer h.subMu.Unlock()

	if clients, ok := h.subscriptions[key]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.subscriptions, key)
		}
		metrics.RecordWSSubscription(string(channel), false)
	}
	client.RemoveSubscription(key)

	h.logger.Debug("client unsubscribed",
		zap.String("client", client.id),
		zap.String("channel", string(channel)),
		zap.String("market", market),
	)
}

// Broadcast 广播消息到订阅频道
func (h *Hub) Broadcast(channel Channel, market string, msg *ServerMessage) {
	select {
	case h.broadcast <- &BroadcastMessage{
		Channel: channel,
		Market:  market,
		Message: msg,
	}:
	default:
		metrics.WSMessagesDropped.Inc()
		h.logger.Warn("broadcast channel full, dropping message",
			zap.String("channel", string(channel)),
			zap.String("market", market),
		)
	}
}

// BroadcastPrivate 广播私有消息（用于订单、余额等私有频道）
// 私有频道使用钱包地址作为 market 参数
func (h *Hub) BroadcastPrivate(channel Channel, wallet string, msg *ServerMessage) {
	// 私有频道使用小写钱包地址作为 key
	normalizedWallet := strings.ToLower(wallet)
	h.Broadcast(channel, normalizedWallet, msg)
}

// BroadcastToWallet 广播消息到特定钱包的所有私有频道
func (h *Hub) BroadcastToWallet(wallet string, msg *ServerMessage) {
	normalizedWallet := strings.ToLower(wallet)

	// 广播到所有私有频道
	privateChannels := []Channel{ChannelOrders, ChannelBalances, ChannelPositions}
	for _, ch := range privateChannels {
		h.Broadcast(ch, normalizedWallet, msg)
	}
}

// ClientCount 返回当前客户端数量
func (h *Hub) ClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}

// SubscriptionCount 返回订阅数量
func (h *Hub) SubscriptionCount(channel Channel, market string) int {
	key := h.subKey(channel, market)
	h.subMu.RLock()
	defer h.subMu.RUnlock()
	if clients, ok := h.subscriptions[key]; ok {
		return len(clients)
	}
	return 0
}

// subKey 生成订阅 key
func (h *Hub) subKey(channel Channel, market string) string {
	return string(channel) + ":" + market
}

// broadcastToSubscribers 向订阅者广播
func (h *Hub) broadcastToSubscribers(msg *BroadcastMessage) {
	key := h.subKey(msg.Channel, msg.Market)

	// 复制客户端列表，避免在迭代时被其他 goroutine 修改
	h.subMu.RLock()
	originalClients := h.subscriptions[key]
	if len(originalClients) == 0 {
		h.subMu.RUnlock()
		return
	}
	// 复制到新的 slice，而不是直接引用 map
	clients := make([]*Client, 0, len(originalClients))
	for client := range originalClients {
		clients = append(clients, client)
	}
	h.subMu.RUnlock()

	data, err := msg.Message.ToJSON()
	if err != nil {
		h.logger.Error("failed to marshal message", zap.Error(err))
		return
	}

	for _, client := range clients {
		// 使用安全的 Send 方法，避免向已关闭的 channel 发送
		if client.Send(data) {
			metrics.RecordWSMessage(string(msg.Channel), true)
		} else {
			h.logger.Debug("failed to send to client (closed or full)",
				zap.String("client", client.id),
			)
		}
	}
}

// removeClientFromAllSubscriptions 从所有订阅中移除客户端
func (h *Hub) removeClientFromAllSubscriptions(client *Client) {
	h.subMu.Lock()
	defer h.subMu.Unlock()

	for _, key := range client.Subscriptions() {
		if clients, ok := h.subscriptions[key]; ok {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.subscriptions, key)
			}
		}
	}
}

// closeAllClients 关闭所有客户端
func (h *Hub) closeAllClients() {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	for client := range h.clients {
		client.Close()
		delete(h.clients, client)
	}
}

// logStats 记录统计信息
func (h *Hub) logStats() {
	h.clientsMu.RLock()
	clientCount := len(h.clients)
	h.clientsMu.RUnlock()

	h.subMu.RLock()
	subCount := len(h.subscriptions)
	h.subMu.RUnlock()

	h.logger.Info("hub stats",
		zap.Int("clients", clientCount),
		zap.Int("subscriptions", subCount),
	)
}
