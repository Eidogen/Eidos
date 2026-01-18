package ws

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// createTestLogger 创建测试用 logger
func createTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

// createMockClient 创建测试用客户端（不依赖真实 websocket 连接）
func createMockClient(hub *Hub, id string) *Client {
	return &Client{
		id:            id,
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
}

// ========== Hub 单元测试 ==========

// TestNewHub 测试创建 Hub
func TestNewHub(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	assert.NotNil(t, hub)
	assert.NotNil(t, hub.clients)
	assert.NotNil(t, hub.subscriptions)
	assert.NotNil(t, hub.register)
	assert.NotNil(t, hub.unregister)
	assert.NotNil(t, hub.broadcast)
	assert.NotNil(t, hub.done)
}

// TestHub_RegisterUnregister 测试注册和注销客户端
func TestHub_RegisterUnregister(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	// 启动 hub
	go hub.Run()
	defer hub.Stop()

	// 给 hub 一些启动时间
	time.Sleep(10 * time.Millisecond)

	// 创建客户端
	client := createMockClient(hub, "test-client-1")

	// 注册
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 1, hub.ClientCount())

	// 注销
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 0, hub.ClientCount())
}

// TestHub_MultipleClients 测试多个客户端
func TestHub_MultipleClients(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 注册多个客户端
	clients := make([]*Client, 10)
	for i := 0; i < 10; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune('0'+i)))
		hub.Register(clients[i])
	}
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 10, hub.ClientCount())

	// 注销部分客户端
	for i := 0; i < 5; i++ {
		hub.Unregister(clients[i])
	}
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 5, hub.ClientCount())
}

// TestHub_Subscribe 测试订阅
func TestHub_Subscribe(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	client := createMockClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// 订阅
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, 1, client.SubscriptionCount())

	// 再次订阅同一个频道（应该覆盖）
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))

	// 订阅不同频道
	hub.Subscribe(client, ChannelDepth, "BTC-USDC")
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelDepth, "BTC-USDC"))
	assert.Equal(t, 2, client.SubscriptionCount())
}

// TestHub_Unsubscribe 测试取消订阅
func TestHub_Unsubscribe(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	client := createMockClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// 订阅
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")
	hub.Subscribe(client, ChannelDepth, "BTC-USDC")
	assert.Equal(t, 2, client.SubscriptionCount())

	// 取消订阅
	hub.Unsubscribe(client, ChannelTicker, "BTC-USDC")
	assert.Equal(t, 0, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, 1, client.SubscriptionCount())

	// 取消不存在的订阅（不应该报错）
	hub.Unsubscribe(client, ChannelKline, "ETH-USDC")
	assert.Equal(t, 1, client.SubscriptionCount())
}

// TestHub_MultipleSubscribers 测试多个订阅者
func TestHub_MultipleSubscribers(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 创建多个客户端订阅同一频道
	clients := make([]*Client, 5)
	for i := 0; i < 5; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune('0'+i)))
		hub.Register(clients[i])
	}
	time.Sleep(20 * time.Millisecond)

	// 所有客户端订阅同一频道
	for _, c := range clients {
		hub.Subscribe(c, ChannelTicker, "BTC-USDC")
	}
	assert.Equal(t, 5, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))

	// 部分客户端取消订阅
	hub.Unsubscribe(clients[0], ChannelTicker, "BTC-USDC")
	hub.Unsubscribe(clients[1], ChannelTicker, "BTC-USDC")
	assert.Equal(t, 3, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
}

// TestHub_Broadcast 测试广播
func TestHub_Broadcast(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 创建客户端并订阅
	client1 := createMockClient(hub, "client-1")
	client2 := createMockClient(hub, "client-2")
	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(10 * time.Millisecond)

	hub.Subscribe(client1, ChannelTicker, "BTC-USDC")
	hub.Subscribe(client2, ChannelTicker, "BTC-USDC")

	// 广播消息
	msg := NewUpdateMessage(ChannelTicker, "BTC-USDC", &TickerData{
		Market:    "BTC-USDC",
		LastPrice: "42000.00",
	}, time.Now().UnixMilli())
	hub.Broadcast(ChannelTicker, "BTC-USDC", msg)

	// 验证两个客户端都收到消息
	select {
	case data := <-client1.send:
		assert.NotNil(t, data)
	case <-time.After(100 * time.Millisecond):
		t.Error("client1 did not receive message")
	}

	select {
	case data := <-client2.send:
		assert.NotNil(t, data)
	case <-time.After(100 * time.Millisecond):
		t.Error("client2 did not receive message")
	}
}

// TestHub_BroadcastNoSubscribers 测试无订阅者时广播
func TestHub_BroadcastNoSubscribers(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 广播到无订阅者的频道（不应该报错）
	msg := NewUpdateMessage(ChannelTicker, "ETH-USDC", nil, time.Now().UnixMilli())
	hub.Broadcast(ChannelTicker, "ETH-USDC", msg)

	// 不应该有错误
	time.Sleep(10 * time.Millisecond)
}

// TestHub_UnregisterCleansSubscriptions 测试注销时清理订阅
func TestHub_UnregisterCleansSubscriptions(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	client := createMockClient(hub, "test-client")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// 订阅多个频道
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")
	hub.Subscribe(client, ChannelDepth, "BTC-USDC")
	hub.Subscribe(client, ChannelTrades, "ETH-USDC")
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelDepth, "BTC-USDC"))
	assert.Equal(t, 1, hub.SubscriptionCount(ChannelTrades, "ETH-USDC"))

	// 注销客户端
	hub.Unregister(client)
	time.Sleep(20 * time.Millisecond)

	// 所有订阅应该被清理
	assert.Equal(t, 0, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, 0, hub.SubscriptionCount(ChannelDepth, "BTC-USDC"))
	assert.Equal(t, 0, hub.SubscriptionCount(ChannelTrades, "ETH-USDC"))
}

// TestHub_SubKey 测试订阅 key 生成
func TestHub_SubKey(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	assert.Equal(t, "ticker:BTC-USDC", hub.subKey(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, "depth:ETH-USDC", hub.subKey(ChannelDepth, "ETH-USDC"))
	assert.Equal(t, "kline:BTC-USDC", hub.subKey(ChannelKline, "BTC-USDC"))
}

// ========== 竞态测试 (Race Condition Tests) ==========

// TestHub_ConcurrentRegisterUnregister 测试并发注册和注销
func TestHub_ConcurrentRegisterUnregister(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	clients := make([]*Client, 100)

	// 并发注册
	for i := 0; i < 100; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune(i)))
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			hub.Register(c)
		}(clients[i])
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// 验证所有客户端都已注册
	assert.Equal(t, 100, hub.ClientCount())

	// 并发注销
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Unregister(client)
		}(c)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, hub.ClientCount())
}

// TestHub_ConcurrentSubscribeUnsubscribe 测试并发订阅和取消订阅
func TestHub_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 创建多个客户端
	clients := make([]*Client, 50)
	for i := 0; i < 50; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune(i)))
		hub.Register(clients[i])
	}
	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup

	// 并发订阅
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Subscribe(client, ChannelTicker, "BTC-USDC")
			hub.Subscribe(client, ChannelDepth, "BTC-USDC")
		}(c)
	}
	wg.Wait()

	assert.Equal(t, 50, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
	assert.Equal(t, 50, hub.SubscriptionCount(ChannelDepth, "BTC-USDC"))

	// 并发取消订阅
	for _, c := range clients[:25] {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Unsubscribe(client, ChannelTicker, "BTC-USDC")
		}(c)
	}
	wg.Wait()

	assert.Equal(t, 25, hub.SubscriptionCount(ChannelTicker, "BTC-USDC"))
}

// TestHub_ConcurrentBroadcast 测试并发广播
func TestHub_ConcurrentBroadcast(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// 创建并订阅客户端
	clients := make([]*Client, 10)
	for i := 0; i < 10; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune(i)))
		hub.Register(clients[i])
		hub.Subscribe(clients[i], ChannelTicker, "BTC-USDC")
	}
	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup

	// 并发广播
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := NewUpdateMessage(ChannelTicker, "BTC-USDC", &TickerData{
				Market:    "BTC-USDC",
				LastPrice: "42000.00",
			}, time.Now().UnixMilli())
			hub.Broadcast(ChannelTicker, "BTC-USDC", msg)
		}(i)
	}
	wg.Wait()

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)
}

// TestHub_ConcurrentMixedOperations 测试并发混合操作
// 注意：此测试验证 Hub 在并发操作下不会 panic
// 由于广播消息可能在客户端注销后到达，我们不对消息接收做断言
func TestHub_ConcurrentMixedOperations(t *testing.T) {
	logger := createTestLogger()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup

	// 阶段 1：并发注册所有客户端
	clients := make([]*Client, 50)
	for i := 0; i < 50; i++ {
		clients[i] = createMockClient(hub, "client-"+string(rune(i)))
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			hub.Register(c)
		}(clients[i])
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// 阶段 2：并发订阅
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Subscribe(client, ChannelTicker, "BTC-USDC")
		}(c)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	// 阶段 3：并发广播（在有订阅者时）
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := NewUpdateMessage(ChannelTicker, "BTC-USDC", nil, time.Now().UnixMilli())
			hub.Broadcast(ChannelTicker, "BTC-USDC", msg)
		}()
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	// 阶段 4：并发取消订阅
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Unsubscribe(client, ChannelTicker, "BTC-USDC")
		}(c)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	// 阶段 5：并发注销
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			hub.Unregister(client)
		}(c)
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 最终应该没有客户端
	assert.Equal(t, 0, hub.ClientCount())
}

// ========== Client 单元测试 ==========

// TestClient_Subscriptions 测试客户端订阅管理
func TestClient_Subscriptions(t *testing.T) {
	client := &Client{
		id:            "test",
		subscriptions: make(map[string]bool),
	}

	// 添加订阅
	client.AddSubscription("ticker:BTC-USDC")
	client.AddSubscription("depth:BTC-USDC")
	assert.Equal(t, 2, client.SubscriptionCount())

	// 获取订阅列表
	subs := client.Subscriptions()
	assert.Len(t, subs, 2)

	// 移除订阅
	client.RemoveSubscription("ticker:BTC-USDC")
	assert.Equal(t, 1, client.SubscriptionCount())

	// 移除不存在的订阅
	client.RemoveSubscription("nonexistent")
	assert.Equal(t, 1, client.SubscriptionCount())
}

// TestClient_WalletManagement 测试钱包管理
func TestClient_WalletManagement(t *testing.T) {
	client := &Client{id: "test"}

	assert.Equal(t, "", client.Wallet())

	client.SetWallet("0x1234567890123456789012345678901234567890")
	assert.Equal(t, "0x1234567890123456789012345678901234567890", client.Wallet())
}

// TestClient_ID 测试客户端 ID
func TestClient_ID(t *testing.T) {
	client := &Client{id: "test-client-123"}
	assert.Equal(t, "test-client-123", client.ID())
}

// TestClient_ConcurrentSubscriptions 测试并发订阅操作
func TestClient_ConcurrentSubscriptions(t *testing.T) {
	client := &Client{
		id:            "test",
		subscriptions: make(map[string]bool),
	}

	var wg sync.WaitGroup

	// 并发添加订阅
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client.AddSubscription("channel-" + string(rune(idx)))
		}(i)
	}
	wg.Wait()

	// 并发读取订阅
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Subscriptions()
			_ = client.SubscriptionCount()
		}()
	}
	wg.Wait()

	// 并发移除订阅
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			client.RemoveSubscription("channel-" + string(rune(idx)))
		}(i)
	}
	wg.Wait()
}

// TestIsValidChannel 测试频道验证
func TestIsValidChannel(t *testing.T) {
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

// ========== Message 单元测试 ==========

// TestParseClientMessage 测试解析客户端消息
func TestParseClientMessage(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
		check   func(t *testing.T, msg *ClientMessage)
	}{
		{
			name:    "valid_subscribe",
			data:    `{"type":"subscribe","id":"123","channel":"ticker","market":"BTC-USDC"}`,
			wantErr: false,
			check: func(t *testing.T, msg *ClientMessage) {
				assert.Equal(t, MsgTypeSubscribe, msg.Type)
				assert.Equal(t, "123", msg.ID)
				assert.Equal(t, ChannelTicker, msg.Channel)
				assert.Equal(t, "BTC-USDC", msg.Market)
			},
		},
		{
			name:    "valid_unsubscribe",
			data:    `{"type":"unsubscribe","channel":"depth","market":"ETH-USDC"}`,
			wantErr: false,
			check: func(t *testing.T, msg *ClientMessage) {
				assert.Equal(t, MsgTypeUnsubscribe, msg.Type)
				assert.Equal(t, ChannelDepth, msg.Channel)
			},
		},
		{
			name:    "valid_ping",
			data:    `{"type":"ping"}`,
			wantErr: false,
			check: func(t *testing.T, msg *ClientMessage) {
				assert.Equal(t, MsgTypePing, msg.Type)
			},
		},
		{
			name:    "invalid_json",
			data:    `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "empty_string",
			data:    ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseClientMessage([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, msg)
				}
			}
		})
	}
}

// TestServerMessage_ToJSON 测试服务端消息序列化
func TestServerMessage_ToJSON(t *testing.T) {
	msg := &ServerMessage{
		Type:      MsgTypeUpdate,
		Channel:   ChannelTicker,
		Market:    "BTC-USDC",
		Timestamp: 1704067200000,
		Data: &TickerData{
			Market:    "BTC-USDC",
			LastPrice: "42000.00",
		},
	}

	data, err := msg.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"update"`)
	assert.Contains(t, string(data), `"channel":"ticker"`)
	assert.Contains(t, string(data), `"market":"BTC-USDC"`)
}

// TestNewPongMessage 测试创建 Pong 消息
func TestNewPongMessage(t *testing.T) {
	msg := NewPongMessage()
	assert.Equal(t, MsgTypePong, msg.Type)
}

// TestNewErrorMessage 测试创建错误消息
func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage("req-123", 400, "invalid channel")
	assert.Equal(t, MsgTypeError, msg.Type)
	assert.Equal(t, "req-123", msg.ID)
	assert.Equal(t, 400, msg.Code)
	assert.Equal(t, "invalid channel", msg.Message)
}

// TestNewAckMessage 测试创建确认消息
func TestNewAckMessage(t *testing.T) {
	msg := NewAckMessage("req-456", ChannelTicker, "BTC-USDC")
	assert.Equal(t, MsgTypeAck, msg.Type)
	assert.Equal(t, "req-456", msg.ID)
	assert.Equal(t, ChannelTicker, msg.Channel)
	assert.Equal(t, "BTC-USDC", msg.Market)
}

// TestNewSnapshotMessage 测试创建快照消息
func TestNewSnapshotMessage(t *testing.T) {
	data := &DepthData{
		Market: "BTC-USDC",
		Bids:   [][]string{{"42000", "1.0"}},
		Asks:   [][]string{{"42001", "0.5"}},
	}
	msg := NewSnapshotMessage(ChannelDepth, "BTC-USDC", data, 1704067200000)
	assert.Equal(t, MsgTypeSnapshot, msg.Type)
	assert.Equal(t, ChannelDepth, msg.Channel)
	assert.Equal(t, "BTC-USDC", msg.Market)
	assert.Equal(t, int64(1704067200000), msg.Timestamp)
	assert.Equal(t, data, msg.Data)
}

// TestNewUpdateMessage 测试创建更新消息
func TestNewUpdateMessage(t *testing.T) {
	data := &TradeData{
		Market:  "BTC-USDC",
		TradeID: "trade-123",
		Price:   "42000.00",
		Amount:  "0.1",
		Side:    "buy",
	}
	msg := NewUpdateMessage(ChannelTrades, "BTC-USDC", data, 1704067200000)
	assert.Equal(t, MsgTypeUpdate, msg.Type)
	assert.Equal(t, ChannelTrades, msg.Channel)
	assert.Equal(t, "BTC-USDC", msg.Market)
	assert.Equal(t, data, msg.Data)
}

// TestClient_SendWhenClosed 测试客户端关闭后发送消息
func TestClient_SendWhenClosed(t *testing.T) {
	client := &Client{
		id:            "test",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}

	// 关闭客户端
	client.Close()

	// 发送消息应该返回 false
	assert.False(t, client.Send([]byte("test")))
}

// TestClient_SendBufferFull 测试发送缓冲区满
func TestClient_SendBufferFull(t *testing.T) {
	logger := zap.NewNop()
	client := &Client{
		id:            "test",
		send:          make(chan []byte, 1), // 小缓冲区
		subscriptions: make(map[string]bool),
		logger:        logger,
	}

	// 填满缓冲区
	client.send <- []byte("first")

	// 再次发送应该返回 false（缓冲区满）
	assert.False(t, client.Send([]byte("second")))
}

// TestClient_DoubleClose 测试重复关闭客户端
func TestClient_DoubleClose(t *testing.T) {
	client := &Client{
		id:            "test",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}

	// 第一次关闭
	client.Close()
	assert.True(t, client.IsClosed())

	// 第二次关闭不应该 panic
	client.Close()
	assert.True(t, client.IsClosed())
}
