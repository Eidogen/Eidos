package ws

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewSubscriber(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	assert.NotNil(t, sub)
	assert.NotNil(t, sub.hub)
	assert.NotNil(t, sub.redis)
	assert.NotNil(t, sub.logger)
	assert.NotNil(t, sub.done)
}

func TestSubscriber_StartStop(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sub.Start(ctx)
	require.NoError(t, err)

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic
	sub.Stop()

	// Give time for the goroutine to exit
	time.Sleep(50 * time.Millisecond)
}

func TestSubscriber_StartWithContextCancel(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	ctx, cancel := context.WithCancel(context.Background())

	err := sub.Start(ctx)
	require.NoError(t, err)

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context should stop the subscriber
	cancel()

	// Give time for the goroutine to exit
	time.Sleep(50 * time.Millisecond)
}

func TestSubscriber_HandleMessage_Ticker(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	// Start hub in background
	go hub.Run()
	defer hub.Stop()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	// Create a mock client to receive messages
	client := &Client{
		id:            "test-client",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond) // Wait for registration

	// Subscribe client to ticker
	hub.Subscribe(client, ChannelTicker, "BTC-USDC")

	// Test handleMessage with ticker channel
	tickerData := &TickerData{
		Market:    "BTC-USDC",
		LastPrice: "50000.00",
		High24h:   "51000.00",
		Low24h:    "49000.00",
		Volume24h: "1000.5",
	}
	payload, _ := json.Marshal(tickerData)

	msg := &redis.Message{
		Channel: "eidos:ticker:BTC-USDC",
		Payload: string(payload),
	}

	sub.handleMessage(msg)

	// Check if client received the message
	select {
	case received := <-client.send:
		var serverMsg ServerMessage
		err := json.Unmarshal(received, &serverMsg)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeUpdate, serverMsg.Type)
		assert.Equal(t, ChannelTicker, serverMsg.Channel)
		assert.Equal(t, "BTC-USDC", serverMsg.Market)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_HandleMessage_Depth(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	client := &Client{
		id:            "test-client",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.Subscribe(client, ChannelDepth, "ETH-USDC")

	depthData := &DepthData{
		Market: "ETH-USDC",
		Bids:   [][]string{{"3000.00", "10.5"}, {"2999.00", "20.0"}},
		Asks:   [][]string{{"3001.00", "5.0"}, {"3002.00", "15.0"}},
	}
	payload, _ := json.Marshal(depthData)

	msg := &redis.Message{
		Channel: "eidos:depth:ETH-USDC",
		Payload: string(payload),
	}

	sub.handleMessage(msg)

	select {
	case received := <-client.send:
		var serverMsg ServerMessage
		err := json.Unmarshal(received, &serverMsg)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeUpdate, serverMsg.Type)
		assert.Equal(t, ChannelDepth, serverMsg.Channel)
		assert.Equal(t, "ETH-USDC", serverMsg.Market)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_HandleMessage_Kline(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	client := &Client{
		id:            "test-client",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.Subscribe(client, ChannelKline, "BTC-USDC")

	klineData := &KlineData{
		Market:    "BTC-USDC",
		Interval:  "1m",
		OpenTime:  1704067200000,
		Open:      "50000.00",
		High:      "50100.00",
		Low:       "49900.00",
		Close:     "50050.00",
		Volume:    "100.5",
		CloseTime: 1704067260000,
	}
	payload, _ := json.Marshal(klineData)

	msg := &redis.Message{
		Channel: "eidos:kline:BTC-USDC:1m",
		Payload: string(payload),
	}

	sub.handleMessage(msg)

	select {
	case received := <-client.send:
		var serverMsg ServerMessage
		err := json.Unmarshal(received, &serverMsg)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeUpdate, serverMsg.Type)
		assert.Equal(t, ChannelKline, serverMsg.Channel)
		assert.Equal(t, "BTC-USDC", serverMsg.Market)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_HandleMessage_Trades(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	go hub.Run()
	defer hub.Stop()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	client := &Client{
		id:            "test-client",
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.Subscribe(client, ChannelTrades, "SOL-USDC")

	tradeData := &TradeData{
		Market:    "SOL-USDC",
		TradeID:   "trade-123",
		Price:     "100.50",
		Amount:    "10.0",
		Side:      "buy",
		Timestamp: 1704067200000,
	}
	payload, _ := json.Marshal(tradeData)

	msg := &redis.Message{
		Channel: "eidos:trades:SOL-USDC",
		Payload: string(payload),
	}

	sub.handleMessage(msg)

	select {
	case received := <-client.send:
		var serverMsg ServerMessage
		err := json.Unmarshal(received, &serverMsg)
		require.NoError(t, err)
		assert.Equal(t, MsgTypeUpdate, serverMsg.Type)
		assert.Equal(t, ChannelTrades, serverMsg.Channel)
		assert.Equal(t, "SOL-USDC", serverMsg.Market)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriber_HandleMessage_InvalidChannel(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	// Invalid channel format (too few parts)
	msg := &redis.Message{
		Channel: "invalid",
		Payload: "{}",
	}

	// Should not panic
	sub.handleMessage(msg)
}

func TestSubscriber_HandleMessage_UnknownChannelType(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	msg := &redis.Message{
		Channel: "eidos:unknown:BTC-USDC",
		Payload: "{}",
	}

	// Should not panic, just log warning
	sub.handleMessage(msg)
}

func TestSubscriber_HandleMessage_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	msg := &redis.Message{
		Channel: "eidos:ticker:BTC-USDC",
		Payload: "invalid json{{{",
	}

	// Should not panic, just log error
	sub.handleMessage(msg)
}

func TestSubscriber_PublishTicker(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	ticker := &TickerData{
		Market:    "BTC-USDC",
		LastPrice: "50000.00",
	}

	ctx := context.Background()
	err := sub.PublishTicker(ctx, "BTC-USDC", ticker)
	require.NoError(t, err)
}

func TestSubscriber_PublishDepth(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	depth := &DepthData{
		Market: "ETH-USDC",
		Bids:   [][]string{{"3000.00", "10.0"}},
		Asks:   [][]string{{"3001.00", "5.0"}},
	}

	ctx := context.Background()
	err := sub.PublishDepth(ctx, "ETH-USDC", depth)
	require.NoError(t, err)
}

func TestSubscriber_PublishKline(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	kline := &KlineData{
		Market:   "BTC-USDC",
		Interval: "1m",
		Open:     "50000.00",
		Close:    "50100.00",
	}

	ctx := context.Background()
	err := sub.PublishKline(ctx, "BTC-USDC", "1m", kline)
	require.NoError(t, err)
}

func TestSubscriber_PublishTrade(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	trade := &TradeData{
		Market:  "SOL-USDC",
		TradeID: "trade-456",
		Price:   "100.00",
		Amount:  "5.0",
		Side:    "sell",
	}

	ctx := context.Background()
	err := sub.PublishTrade(ctx, "SOL-USDC", trade)
	require.NoError(t, err)
}

func TestSubscriber_ConcurrentPublish(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	sub := NewSubscriber(hub, rdb, logger)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Publish concurrently - should not panic or error
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ticker := &TickerData{
				Market:    "BTC-USDC",
				LastPrice: "50000.00",
			}
			_ = sub.PublishTicker(ctx, "BTC-USDC", ticker)
		}(i)
	}

	wg.Wait()
}
