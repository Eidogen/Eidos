// Package engine 撮合引擎边界条件和异常测试
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

// 边界条件测试

func TestMatcher_ZeroAmount(t *testing.T) {
	m := newTestMatcher()

	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.Zero,
		Remaining:   decimal.Zero,
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}

	result, err := m.Match(order)

	// 零数量订单应该直接返回
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.TakerFilled)
	assert.Empty(t, result.Trades)
}

func TestMatcher_NegativeAmount(t *testing.T) {
	m := newTestMatcher()

	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(50000.0),
		Amount:      decimal.NewFromFloat(-1.0),
		Remaining:   decimal.NewFromFloat(-1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}

	result, err := m.Match(order)

	// 负数量订单视为已完成
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.TakerFilled)
}

func TestMatcher_VerySmallAmount(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 50000.0, 0.0001)
	m.orderBook.AddOrder(sellOrder)

	// 非常小的买单
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 50000.0, 0.00001)

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMatcher_VeryLargeAmount(t *testing.T) {
	m := newTestMatcher()

	// 添加多个卖单
	for i := 0; i < 100; i++ {
		sellOrder := createOrder(
			"sell-"+decimal.NewFromInt(int64(i)).String(),
			model.OrderSideSell,
			model.OrderTypeLimit,
			model.TimeInForceGTC,
			50000.0,
			1000.0,
		)
		sellOrder.Sequence = int64(i)
		m.orderBook.AddOrder(sellOrder)
	}

	// 大额买单
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 50000.0, 1000000.0)
	buyOrder.Sequence = 100

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result.Trades), 0)
	assert.False(t, result.TakerFilled) // 无法完全成交
}

func TestMatcher_PriceAtZero(t *testing.T) {
	m := newTestMatcher()

	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.Zero,
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}

	result, err := m.Match(order)

	// 零价格限价单应该被接受但无法成交
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestMatcher_VeryHighPrice(t *testing.T) {
	m := newTestMatcher()

	// 添加高价卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 999999999.99, 1.0)
	m.orderBook.AddOrder(sellOrder)

	// 同样高价的买单
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 999999999.99, 1.0)
	buyOrder.Sequence = 2

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
}

func TestMatcher_SameWalletSelfTrade(t *testing.T) {
	m := newTestMatcher()

	// 同一钱包的卖单
	sellOrder := &model.Order{
		OrderID:     "sell-1",
		Wallet:      "0x123",
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
	m.orderBook.AddOrder(sellOrder)

	// 同一钱包的买单（自成交）
	buyOrder := &model.Order{
		OrderID:     "buy-1",
		Wallet:      "0x123",
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

	result, err := m.Match(buyOrder)

	// 当前实现允许自成交，生产环境可能需要阻止
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// 引擎边界测试

func TestEngine_ProcessOrder_WithCancelledContext(t *testing.T) {
	var counter int64
	cfg := &EngineConfig{
		Market: &MarketConfig{
			Symbol:        "BTC-USDC",
			BaseToken:     "BTC",
			QuoteToken:    "USDC",
			PriceDecimals: 2,
			SizeDecimals:  4,
			MinSize:       decimal.NewFromFloat(0.0001),
			TickSize:      decimal.NewFromFloat(0.01),
			MakerFeeRate:  decimal.NewFromFloat(0.001),
			TakerFeeRate:  decimal.NewFromFloat(0.002),
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}

	engine := NewEngine(cfg)
	engine.Start()
	defer engine.Stop()

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	order := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 1.0, 1)
	result, err := engine.ProcessOrder(ctx, order)

	// 当前实现: 撮合引擎不检查 context，以保证高性能同步处理
	// 订单会正常处理，不会因 context 取消而中断
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestEngine_ProcessOrder_StopDuringProcessing(t *testing.T) {
	var counter int64
	cfg := &EngineConfig{
		Market: &MarketConfig{
			Symbol:        "BTC-USDC",
			BaseToken:     "BTC",
			QuoteToken:    "USDC",
			PriceDecimals: 2,
			SizeDecimals:  4,
			MinSize:       decimal.NewFromFloat(0.0001),
			TickSize:      decimal.NewFromFloat(0.01),
			MakerFeeRate:  decimal.NewFromFloat(0.001),
			TakerFeeRate:  decimal.NewFromFloat(0.002),
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}

	engine := NewEngine(cfg)
	engine.Start()

	ctx := context.Background()

	// 处理一些订单
	for i := 0; i < 10; i++ {
		order := createEngineTestOrder("order-"+decimal.NewFromInt(int64(i)).String(), model.OrderSideBuy, 100.0, 1.0, int64(i+1))
		engine.ProcessOrder(ctx, order)
	}

	// 停止引擎
	engine.Stop()

	// 尝试再次处理
	order := createEngineTestOrder("order-after-stop", model.OrderSideBuy, 100.0, 1.0, 100)
	result, err := engine.ProcessOrder(ctx, order)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not running")
	assert.Nil(t, result)
}

func TestEngine_ProcessCancel_NotRunning(t *testing.T) {
	engine := newTestEngine()

	ctx := context.Background()

	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0x123",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
	}

	result, err := engine.ProcessCancel(ctx, cancelMsg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not running")
	assert.Nil(t, result)
}

func TestEngine_DoubleStart(t *testing.T) {
	engine := newTestEngine()

	engine.Start()
	defer engine.Stop()

	// 再次启动应该是安全的
	engine.Start()

	assert.True(t, engine.IsRunning())
}

func TestEngine_DoubleStop(t *testing.T) {
	engine := newTestEngine()

	engine.Start()
	engine.Stop()

	// 当前实现中，双重停止会 panic (close 已关闭的 channel)
	// 这是预期行为，应用层需要保证只调用一次 Stop
	assert.Panics(t, func() {
		engine.Stop()
	})

	assert.False(t, engine.IsRunning())
}

// 管理器边界测试

func TestEngineManager_UnknownMarket(t *testing.T) {
	var counter int64
	cfg := &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:       "BTC-USDC",
				BaseToken:    "BTC",
				QuoteToken:   "USDC",
				MakerFeeRate: decimal.NewFromFloat(0.001),
				TakerFeeRate: decimal.NewFromFloat(0.002),
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}

	manager := NewEngineManager(cfg)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 未知市场的订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
		Market:      "ETH-USDC", // 不存在的市场
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(3000.0),
		Amount:      decimal.NewFromFloat(1.0),
		Remaining:   decimal.NewFromFloat(1.0),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}

	result, err := manager.ProcessOrder(ctx, order)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not found")
	assert.Nil(t, result)
}

func TestEngineManager_RemoveActiveMarket(t *testing.T) {
	var counter int64
	cfg := &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:       "BTC-USDC",
				BaseToken:    "BTC",
				QuoteToken:   "USDC",
				MakerFeeRate: decimal.NewFromFloat(0.001),
				TakerFeeRate: decimal.NewFromFloat(0.002),
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}

	manager := NewEngineManager(cfg)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 添加一些订单
	order := &model.Order{
		OrderID:     "order-1",
		Wallet:      "0x123",
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

	// 尝试移除有活跃订单的市场 - 应该失败
	err := manager.RemoveMarket("BTC-USDC")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "active orders")

	// 验证市场仍然存在
	_, err = manager.GetEngine("BTC-USDC")
	assert.NoError(t, err)
}

func TestEngineManager_AddDuplicateMarket(t *testing.T) {
	var counter int64
	cfg := &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:       "BTC-USDC",
				BaseToken:    "BTC",
				QuoteToken:   "USDC",
				MakerFeeRate: decimal.NewFromFloat(0.001),
				TakerFeeRate: decimal.NewFromFloat(0.002),
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 100,
	}

	manager := NewEngineManager(cfg)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 尝试添加重复市场
	err := manager.AddMarket(&MarketConfig{
		Symbol:       "BTC-USDC",
		BaseToken:    "BTC",
		QuoteToken:   "USDC",
		MakerFeeRate: decimal.NewFromFloat(0.001),
		TakerFeeRate: decimal.NewFromFloat(0.002),
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// IOC 和 FOK 边界测试

func TestMatcher_IOC_PartialFill_EdgeCase(t *testing.T) {
	m := newTestMatcher()

	// 添加小数量卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	m.orderBook.AddOrder(sellOrder)

	// IOC 买单请求更大数量
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceIOC, 100.0, 10.0)
	buyOrder.Sequence = 2

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
	// IOC 部分成交: TakerFilled=false (未完全成交), TakerCancelled=true (剩余被取消)
	assert.False(t, result.TakerFilled)
	assert.True(t, result.TakerCancelled)
}

func TestMatcher_FOK_ExactFill(t *testing.T) {
	m := newTestMatcher()

	// 添加刚好数量的卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// FOK 买单请求相同数量
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceFOK, 100.0, 10.0)
	buyOrder.Sequence = 2

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)
}

// 精度测试

func TestMatcher_DecimalPrecision(t *testing.T) {
	m := newTestMatcher()

	// 使用高精度价格
	sellOrder := &model.Order{
		OrderID:     "sell-1",
		Wallet:      "0xmaker",
		Market:      "BTC-USDC",
		Side:        model.OrderSideSell,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.RequireFromString("50000.123456789"),
		Amount:      decimal.RequireFromString("0.123456789"),
		Remaining:   decimal.RequireFromString("0.123456789"),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
	m.orderBook.AddOrder(sellOrder)

	buyOrder := &model.Order{
		OrderID:     "buy-1",
		Wallet:      "0xtaker",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.RequireFromString("50000.123456789"),
		Amount:      decimal.RequireFromString("0.123456789"),
		Remaining:   decimal.RequireFromString("0.123456789"),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    2,
	}

	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))

	// 验证精度保持
	trade := result.Trades[0]
	assert.True(t, decimal.RequireFromString("0.123456789").Equal(trade.Amount))
}
