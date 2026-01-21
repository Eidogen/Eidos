package ws

import (
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

func TestNewClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{
		MaxMessageSize:   4096,
		MaxSubscriptions: 100,
	}

	// 由于 NewClient 需要 websocket.Conn，我们测试基本属性
	// 实际连接测试在 handler 集成测试中覆盖
	client := &Client{
		id:            "test-client-id",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	assert.NotNil(t, client)
	assert.NotEmpty(t, client.ID())
	assert.Empty(t, client.Wallet())
	assert.Equal(t, 0, client.SubscriptionCount())
}

func TestClient_WalletOperations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	// 使用 mock 连接测试
	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	assert.Empty(t, client.Wallet())

	client.SetWallet("0x1234567890123456789012345678901234567890")
	assert.Equal(t, "0x1234567890123456789012345678901234567890", client.Wallet())
}

func TestClient_SubscriptionOperations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 初始状态
	assert.Equal(t, 0, client.SubscriptionCount())
	assert.Empty(t, client.Subscriptions())

	// 添加订阅
	client.AddSubscription("ticker:BTC-USDC")
	assert.Equal(t, 1, client.SubscriptionCount())
	assert.Contains(t, client.Subscriptions(), "ticker:BTC-USDC")

	// 添加更多订阅
	client.AddSubscription("depth:BTC-USDC")
	client.AddSubscription("kline:BTC-USDC")
	assert.Equal(t, 3, client.SubscriptionCount())

	// 移除订阅
	client.RemoveSubscription("ticker:BTC-USDC")
	assert.Equal(t, 2, client.SubscriptionCount())
	assert.NotContains(t, client.Subscriptions(), "ticker:BTC-USDC")

	// 移除不存在的订阅（应该不报错）
	client.RemoveSubscription("nonexistent")
	assert.Equal(t, 2, client.SubscriptionCount())
}

func TestClient_SubscriptionConcurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// 并发添加和移除订阅
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := "sub-" + string(rune('A'+id))
				client.AddSubscription(key)
				_ = client.SubscriptionCount()
				_ = client.Subscriptions()
				client.RemoveSubscription(key)
			}
		}(i)
	}

	wg.Wait()
	// 测试通过即表明没有 race condition
}

func TestClient_Send(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送消息到未关闭的客户端
	ok := client.Send([]byte("test message"))
	assert.True(t, ok)

	// 验证消息在通道中
	select {
	case msg := <-client.send:
		assert.Equal(t, "test message", string(msg))
	default:
		t.Fatal("message not received")
	}
}

func TestClient_Send_BufferFull(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	// 创建只有 1 个缓冲的通道
	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 1),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 第一条消息应该成功
	ok := client.Send([]byte("message 1"))
	assert.True(t, ok)

	// 第二条消息应该因为缓冲已满而失败
	ok = client.Send([]byte("message 2"))
	assert.False(t, ok)
}

func TestClient_Send_Closed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 关闭客户端
	client.Close()

	// 发送到已关闭的客户端应该返回 false
	ok := client.Send([]byte("test message"))
	assert.False(t, ok)
}

func TestClient_Close(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	assert.False(t, client.IsClosed())

	// 关闭客户端
	client.Close()
	assert.True(t, client.IsClosed())

	// 重复关闭应该是安全的
	client.Close()
	assert.True(t, client.IsClosed())
}

func TestClient_MarkClosed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	assert.False(t, client.IsClosed())

	// 标记关闭（不关闭通道）
	client.MarkClosed()
	assert.True(t, client.IsClosed())

	// 通道仍然可以发送（虽然 Send 会返回 false）
	ok := client.Send([]byte("test"))
	assert.False(t, ok) // 因为已标记关闭
}

func TestClient_CloseConcurrency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// 并发关闭和发送
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.Close()
			_ = client.IsClosed()
			_ = client.Send([]byte("test"))
		}()
	}

	wg.Wait()
	assert.True(t, client.IsClosed())
}

func TestClient_IsValidChannel(t *testing.T) {
	tests := []struct {
		channel Channel
		valid   bool
	}{
		{ChannelTicker, true},
		{ChannelDepth, true},
		{ChannelKline, true},
		{ChannelTrades, true},
		{ChannelOrders, true},
		{Channel("invalid"), false},
		{Channel(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.channel), func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidChannel(tt.channel))
		})
	}
}

func TestClient_HandleMessage_Ping(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送 ping 消息
	pingMsg := `{"type":"ping"}`
	client.handleMessage([]byte(pingMsg))

	// 检查是否收到 pong 响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypePong, resp.Type)
	case <-time.After(time.Second):
		t.Fatal("no pong response received")
	}
}

func TestClient_HandleMessage_Subscribe_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送订阅消息
	subMsg := `{"id":"1","type":"subscribe","channel":"ticker","market":"BTC-USDC"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到 ack 响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeAck, resp.Type)
		assert.Equal(t, "1", resp.ID)
		assert.Equal(t, ChannelTicker, resp.Channel)
		assert.Equal(t, "BTC-USDC", resp.Market)
	case <-time.After(time.Second):
		t.Fatal("no ack response received")
	}
}

func TestClient_HandleMessage_Subscribe_InvalidChannel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送无效频道的订阅消息
	subMsg := `{"id":"1","type":"subscribe","channel":"invalid","market":"BTC-USDC"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Equal(t, 400, resp.Code)
		assert.Contains(t, resp.Message, "invalid channel")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_HandleMessage_Subscribe_MissingMarket(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送没有 market 的订阅消息
	subMsg := `{"id":"1","type":"subscribe","channel":"ticker"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Contains(t, resp.Message, "market is required")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_HandleMessage_Subscribe_MaxSubscriptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 2, // 限制只能订阅 2 个
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 添加 2 个订阅达到限制
	client.AddSubscription("sub-1")
	client.AddSubscription("sub-2")

	// 尝试添加第 3 个订阅
	subMsg := `{"id":"1","type":"subscribe","channel":"ticker","market":"BTC-USDC"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Equal(t, 429, resp.Code)
		assert.Contains(t, resp.Message, "max subscriptions exceeded")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_HandleMessage_Subscribe_PrivateChannelNoAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
		wallet:        "", // 未认证
	}

	// 尝试订阅私有频道（orders）
	subMsg := `{"id":"1","type":"subscribe","channel":"orders"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Equal(t, 401, resp.Code)
		assert.Contains(t, resp.Message, "authentication required")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_HandleMessage_Subscribe_PrivateChannelWithAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
		wallet:        "0x1234567890123456789012345678901234567890",
		authenticated: true, // 已认证
	}

	// 订阅私有频道（orders），不需要 market
	subMsg := `{"id":"1","type":"subscribe","channel":"orders"}`
	client.handleMessage([]byte(subMsg))

	// 检查是否收到 ack 响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeAck, resp.Type)
	case <-time.After(time.Second):
		t.Fatal("no ack response received")
	}
}

func TestClient_HandleMessage_Unsubscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 先订阅 (hub.Subscribe 不发送消息到 client.send)
	hub.Register(client)
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")

	// 发送取消订阅消息
	unsubMsg := `{"id":"1","type":"unsubscribe","channel":"ticker","market":"BTC-USDC"}`
	client.handleMessage([]byte(unsubMsg))

	// 检查是否收到 ack 响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeAck, resp.Type)
	case <-time.After(time.Second):
		t.Fatal("no ack response received")
	}
}

func TestClient_HandleMessage_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送无效 JSON
	client.handleMessage([]byte("invalid json"))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Equal(t, 400, resp.Code)
		assert.Contains(t, resp.Message, "invalid message format")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_HandleMessage_UnknownType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxSubscriptions: 100,
	}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送未知类型消息
	unknownMsg := `{"id":"1","type":"unknown"}`
	client.handleMessage([]byte(unknownMsg))

	// 检查是否收到错误响应
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Contains(t, resp.Message, "unknown message type")
	case <-time.After(time.Second):
		t.Fatal("no error response received")
	}
}

func TestClient_SendError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)

	cfg := config.WebSocketConfig{}

	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 256),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 发送错误消息
	client.sendError("req-1", 500, "test error")

	// 检查错误消息
	select {
	case msg := <-client.send:
		var resp ServerMessage
		err := json.Unmarshal(msg, &resp)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeError, resp.Type)
		assert.Equal(t, "req-1", resp.ID)
		assert.Equal(t, 500, resp.Code)
		assert.Equal(t, "test error", resp.Message)
	case <-time.After(time.Second):
		t.Fatal("no error message received")
	}
}

func TestClient_SendError_BufferFull(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)

	cfg := config.WebSocketConfig{}

	// 创建缓冲已满的通道
	client := &Client{
		id:            "test-client",
		hub:           hub,
		send:          make(chan []byte, 1),
		logger:        logger,
		cfg:           cfg,
		subscriptions: make(map[string]bool),
	}

	// 填满缓冲
	client.send <- []byte("blocking")

	// 发送错误消息应该被丢弃，不会阻塞
	client.sendError("req-1", 500, "test error")
	// 测试通过即表明没有阻塞
}
