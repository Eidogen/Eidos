// Package engine 引擎管理器单元测试
package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func newTestManagerConfig() *ManagerConfig {
	var counter int64
	return &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:        "BTC-USDC",
				BaseToken:     "BTC",
				QuoteToken:    "USDC",
				PriceDecimals: 2,
				SizeDecimals:  4,
				MinSize:       decimal.NewFromFloat(0.0001),
				TickSize:      decimal.NewFromFloat(0.01),
				MakerFeeRate:  decimal.NewFromFloat(0.001),
				TakerFeeRate:  decimal.NewFromFloat(0.002),
				MaxSlippage:   decimal.NewFromFloat(0.05),
			},
			{
				Symbol:        "ETH-USDC",
				BaseToken:     "ETH",
				QuoteToken:    "USDC",
				PriceDecimals: 2,
				SizeDecimals:  4,
				MinSize:       decimal.NewFromFloat(0.001),
				TickSize:      decimal.NewFromFloat(0.01),
				MakerFeeRate:  decimal.NewFromFloat(0.001),
				TakerFeeRate:  decimal.NewFromFloat(0.002),
				MaxSlippage:   decimal.NewFromFloat(0.05),
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}
}

func TestNewEngineManager(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)

	assert.NotNil(t, manager)
	assert.Equal(t, 2, len(manager.configs))
	assert.NotNil(t, manager.configs["BTC-USDC"])
	assert.NotNil(t, manager.configs["ETH-USDC"])
}

func TestEngineManager_StartStop(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)

	ctx := context.Background()
	err := manager.Start(ctx)
	assert.NoError(t, err)

	// 验证引擎已创建并启动
	btcEngine, err := manager.GetEngine("BTC-USDC")
	assert.NoError(t, err)
	assert.True(t, btcEngine.IsRunning())

	ethEngine, err := manager.GetEngine("ETH-USDC")
	assert.NoError(t, err)
	assert.True(t, ethEngine.IsRunning())

	// 停止
	manager.Stop()
	assert.False(t, btcEngine.IsRunning())
	assert.False(t, ethEngine.IsRunning())
}

func TestEngineManager_GetEngine(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	// 获取存在的引擎
	engine, err := manager.GetEngine("BTC-USDC")
	assert.NoError(t, err)
	assert.NotNil(t, engine)

	// 获取不存在的引擎
	_, err = manager.GetEngine("DOGE-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not found")
}

func TestEngineManager_ProcessOrder(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 处理 BTC 订单
	btcOrder := &model.Order{
		OrderID:     "btc-order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}

	result, err := manager.ProcessOrder(ctx, btcOrder)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 处理 ETH 订单
	ethOrder := &model.Order{
		OrderID:     "eth-order-1",
		Wallet:      "0xtest",
		Market:      "ETH-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(3000.0),
		Amount:      decimal.NewFromFloat(10.0),
		Remaining:   decimal.NewFromFloat(10.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    2,
	}

	result, err = manager.ProcessOrder(ctx, ethOrder)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 处理不存在的市场订单
	invalidOrder := &model.Order{
		OrderID: "invalid-1",
		Market:  "DOGE-USDC",
	}

	_, err = manager.ProcessOrder(ctx, invalidOrder)
	assert.Error(t, err)
}

func TestEngineManager_ProcessCancel(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 先添加订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 取消订单
	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
	}

	result, err := manager.ProcessCancel(ctx, cancelMsg)
	assert.NoError(t, err)
	assert.True(t, result.Success)

	// 取消不存在市场的订单
	invalidCancel := &model.CancelMessage{
		OrderID: "order-1",
		Market:  "DOGE-USDC",
	}

	_, err = manager.ProcessCancel(ctx, invalidCancel)
	assert.Error(t, err)
}

func TestEngineManager_GetDepth(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 添加订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 获取深度
	depth, err := manager.GetDepth("BTC-USDC", 10)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(depth.Bids))

	// 不存在的市场
	_, err = manager.GetDepth("DOGE-USDC", 10)
	assert.Error(t, err)
}

func TestEngineManager_GetOrder(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 添加订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 获取订单
	found := manager.GetOrder("BTC-USDC", "order-1")
	assert.NotNil(t, found)

	// 不存在的订单
	notFound := manager.GetOrder("BTC-USDC", "non-exist")
	assert.Nil(t, notFound)

	// 不存在的市场
	nilResult := manager.GetOrder("DOGE-USDC", "order-1")
	assert.Nil(t, nilResult)
}

func TestEngineManager_GetStats(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 处理一些订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	stats := manager.GetStats()
	assert.Equal(t, 2, len(stats)) // BTC-USDC 和 ETH-USDC

	btcStats := stats["BTC-USDC"]
	assert.NotNil(t, btcStats)
	assert.Equal(t, int64(1), btcStats.OrdersProcessed)
}

func TestEngineManager_GetMarkets(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	markets := manager.GetMarkets()
	assert.Equal(t, 2, len(markets))
	assert.Contains(t, markets, "BTC-USDC")
	assert.Contains(t, markets, "ETH-USDC")
}

func TestEngineManager_AddMarket(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	// 添加新市场
	newMarket := &MarketConfig{
		Symbol:        "SOL-USDC",
		BaseToken:     "SOL",
		QuoteToken:    "USDC",
		PriceDecimals: 2,
		SizeDecimals:  2,
		MinSize:       decimal.NewFromFloat(0.1),
		TickSize:      decimal.NewFromFloat(0.01),
		MakerFeeRate:  decimal.NewFromFloat(0.001),
		TakerFeeRate:  decimal.NewFromFloat(0.002),
	}

	err := manager.AddMarket(newMarket)
	assert.NoError(t, err)

	// 验证新市场可用
	engine, err := manager.GetEngine("SOL-USDC")
	assert.NoError(t, err)
	assert.NotNil(t, engine)
	assert.True(t, engine.IsRunning())

	// 添加已存在的市场应该失败
	err = manager.AddMarket(newMarket)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestEngineManager_RemoveMarket(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	// 移除空市场
	err := manager.RemoveMarket("ETH-USDC")
	assert.NoError(t, err)

	// 验证市场已移除
	_, err = manager.GetEngine("ETH-USDC")
	assert.Error(t, err)

	assert.Equal(t, 1, len(manager.GetMarkets()))
}

func TestEngineManager_RemoveMarket_WithActiveOrders(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	ctx := context.Background()

	// 添加订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 移除有活跃订单的市场应该失败
	err := manager.RemoveMarket("BTC-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "active orders")
}

func TestEngineManager_RemoveMarket_NotFound(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())
	defer manager.Stop()

	err := manager.RemoveMarket("DOGE-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEngineManager_CollectTrades(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	// 添加卖单
	sellOrder := &model.Order{
		OrderID:     "sell-1",
		Wallet:      "0xseller",
		Market:      "BTC-USDC",
		Side:        model.OrderSideSell,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, sellOrder)

	// 成交
	buyOrder := &model.Order{
		OrderID:     "buy-1",
		Wallet:      "0xbuyer",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    2,
	}
	manager.ProcessOrder(ctx, buyOrder)

	// 收集成交
	var collectedTrades []*model.TradeResult
	done := make(chan struct{})

	go func() {
		manager.CollectTrades(ctx, func(trade *model.TradeResult) error {
			collectedTrades = append(collectedTrades, trade)
			if len(collectedTrades) >= 1 {
				cancel()
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, 1, len(collectedTrades))
	case <-time.After(5 * time.Second):
		cancel()
		t.Error("Timeout waiting for trades")
	}

	manager.Stop()
}

func TestEngineManager_DefaultChannelSize(t *testing.T) {
	var counter int64
	cfg := &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:     "BTC-USDC",
				BaseToken:  "BTC",
				QuoteToken: "USDC",
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 0, // 未设置
	}

	manager := NewEngineManager(cfg)
	assert.Equal(t, 10000, manager.channelSize) // 默认值
}

func TestEngineManager_CollectUpdates(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	// 添加订单会产生订单簿更新
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 收集更新
	var collectedUpdates []*model.OrderBookUpdate
	done := make(chan struct{})

	go func() {
		manager.CollectUpdates(ctx, func(update *model.OrderBookUpdate) error {
			collectedUpdates = append(collectedUpdates, update)
			if len(collectedUpdates) >= 1 {
				cancel()
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
		assert.GreaterOrEqual(t, len(collectedUpdates), 1)
	case <-time.After(5 * time.Second):
		cancel()
		t.Error("Timeout waiting for updates")
	}

	manager.Stop()
}

func TestEngineManager_CollectCancels(t *testing.T) {
	cfg := newTestManagerConfig()
	manager := NewEngineManager(cfg)
	manager.Start(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	// 先添加订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0xtest",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	manager.ProcessOrder(ctx, order)

	// 取消订单
	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
	}
	manager.ProcessCancel(ctx, cancelMsg)

	// 收集取消结果
	var collectedCancels []*model.CancelResult
	done := make(chan struct{})

	go func() {
		manager.CollectCancels(ctx, func(cancelResult *model.CancelResult) error {
			collectedCancels = append(collectedCancels, cancelResult)
			if len(collectedCancels) >= 1 {
				cancel()
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, 1, len(collectedCancels))
		assert.True(t, collectedCancels[0].Success)
	case <-time.After(5 * time.Second):
		cancel()
		t.Error("Timeout waiting for cancels")
	}

	manager.Stop()
}
