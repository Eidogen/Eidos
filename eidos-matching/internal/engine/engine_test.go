// Package engine 撮合引擎单元测试
package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func newTestEngine() *Engine {
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
			MaxSlippage:   decimal.NewFromFloat(0.05),
		},
		TradeIDGen: func() string {
			id := atomic.AddInt64(&counter, 1)
			return "trade-" + decimal.NewFromInt(id).String()
		},
		ChannelSize: 100,
	}

	return NewEngine(cfg)
}

func createEngineTestOrder(id string, side model.OrderSide, price, amount float64, seq int64) *model.Order {
	return &model.Order{
		OrderID:     id,
		Wallet:      "0x" + id,
		Market:      "BTC-USDC",
		Side:        side,
		Type:        model.OrderTypeLimit,
		TimeInForce: model.TimeInForceGTC,
		Price:       decimal.NewFromFloat(price),
		Amount:      decimal.NewFromFloat(amount),
		Remaining:   decimal.NewFromFloat(amount),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    seq,
	}
}

func TestNewEngine(t *testing.T) {
	engine := newTestEngine()

	assert.NotNil(t, engine)
	assert.Equal(t, "BTC-USDC", engine.market)
	assert.NotNil(t, engine.orderBook)
	assert.NotNil(t, engine.matcher)
	assert.NotNil(t, engine.tradesChan)
	assert.NotNil(t, engine.updatesChan)
	assert.NotNil(t, engine.cancelsChan)
}

func TestEngine_StartStop(t *testing.T) {
	engine := newTestEngine()

	assert.False(t, engine.IsRunning())

	engine.Start()
	assert.True(t, engine.IsRunning())

	engine.Stop()
	assert.False(t, engine.IsRunning())
}

func TestEngine_ProcessOrder_NotRunning(t *testing.T) {
	engine := newTestEngine()
	ctx := context.Background()

	// 引擎未启动时处理订单
	order := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 1)
	result, err := engine.ProcessOrder(ctx, order)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not running")
	assert.Nil(t, result)
}

func TestEngine_ProcessOrder_Idempotency_Sequence(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 第一次处理
	order1 := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 1)
	result1, err := engine.ProcessOrder(ctx, order1)
	assert.NoError(t, err)
	assert.NotNil(t, result1)

	// 重复序列号的订单应该被跳过
	order2 := createEngineTestOrder("order-2", model.OrderSideBuy, 101.0, 5.0, 1)
	result2, err := engine.ProcessOrder(ctx, order2)
	assert.NoError(t, err)
	assert.Nil(t, result2) // 幂等跳过
}

func TestEngine_ProcessOrder_Idempotency_OrderID(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 第一次处理
	order1 := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 1)
	engine.ProcessOrder(ctx, order1)

	// 同一订单ID不同序列号
	order2 := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 2)
	result, err := engine.ProcessOrder(ctx, order2)
	assert.NoError(t, err)
	assert.Nil(t, result) // 幂等跳过
}

func TestEngine_ProcessOrder_Match(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 添加卖单
	sellOrder := createEngineTestOrder("sell-1", model.OrderSideSell, 100.0, 10.0, 1)
	engine.ProcessOrder(ctx, sellOrder)

	// 买单成交
	buyOrder := createEngineTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0, 2)
	result, err := engine.ProcessOrder(ctx, buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)

	// 验证输出通道收到成交
	select {
	case trade := <-engine.TradesChan():
		assert.Equal(t, "sell-1", trade.MakerOrderID)
		assert.Equal(t, "buy-1", trade.TakerOrderID)
	case <-time.After(time.Second):
		t.Error("Expected trade in channel")
	}
}

func TestEngine_ProcessCancel_Success(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 先添加订单
	order := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 1)
	engine.ProcessOrder(ctx, order)

	// 取消订单
	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0xorder-1",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
	}

	result, err := engine.ProcessCancel(ctx, cancelMsg)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.True(t, decimal.NewFromFloat(10.0).Equal(result.RemainingSize))

	// 验证订单已从订单簿移除
	assert.Nil(t, engine.GetOrder("order-1"))

	// 验证取消结果发送到通道
	select {
	case cancelResult := <-engine.CancelsChan():
		assert.True(t, cancelResult.Success)
	case <-time.After(time.Second):
		t.Error("Expected cancel result in channel")
	}
}

func TestEngine_ProcessCancel_NotFound(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	cancelMsg := &model.CancelMessage{
		OrderID:   "non-exist",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
	}

	result, err := engine.ProcessCancel(ctx, cancelMsg)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "ORDER_NOT_FOUND", result.Reason)
}

func TestEngine_ProcessCancel_Idempotency(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 设置初始序列号
	engine.SetInputSequence(5)

	// 旧序列号应该被跳过
	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  3, // 小于当前序列号
	}

	result, err := engine.ProcessCancel(ctx, cancelMsg)
	assert.NoError(t, err)
	assert.Nil(t, result) // 幂等跳过
}

func TestEngine_GetOrder(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 添加订单
	order := createEngineTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0, 1)
	engine.ProcessOrder(ctx, order)

	// 获取存在的订单
	found := engine.GetOrder("order-1")
	assert.NotNil(t, found)
	assert.Equal(t, "order-1", found.OrderID)

	// 获取不存在的订单
	notFound := engine.GetOrder("non-exist")
	assert.Nil(t, notFound)
}

func TestEngine_GetDepth(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 添加订单
	engine.ProcessOrder(ctx, createEngineTestOrder("buy-1", model.OrderSideBuy, 99.0, 10.0, 1))
	engine.ProcessOrder(ctx, createEngineTestOrder("buy-2", model.OrderSideBuy, 100.0, 5.0, 2))
	engine.ProcessOrder(ctx, createEngineTestOrder("sell-1", model.OrderSideSell, 101.0, 8.0, 3))

	depth, err := engine.GetDepth(10)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(depth.Bids))
	assert.Equal(t, 1, len(depth.Asks))
}

func TestEngine_GetStats(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 处理一些订单
	engine.ProcessOrder(ctx, createEngineTestOrder("sell-1", model.OrderSideSell, 100.0, 10.0, 1))
	engine.ProcessOrder(ctx, createEngineTestOrder("buy-1", model.OrderSideBuy, 100.0, 5.0, 2))

	stats := engine.GetStats()
	assert.Equal(t, "BTC-USDC", stats.Market)
	assert.Equal(t, int64(2), stats.OrdersProcessed)
	assert.Equal(t, int64(1), stats.TradesGenerated)
	assert.Greater(t, stats.AvgLatencyUs, int64(0))
}

func TestEngine_SetInputSequence(t *testing.T) {
	engine := newTestEngine()

	engine.SetInputSequence(100)
	assert.Equal(t, int64(100), engine.GetInputSequence())
}

func TestEngine_GetOrderBook(t *testing.T) {
	engine := newTestEngine()

	ob := engine.GetOrderBook()
	assert.NotNil(t, ob)
	assert.Equal(t, "BTC-USDC", ob.Market)
}

func TestEngine_ChannelOverflow(t *testing.T) {
	// 创建小缓冲区的引擎
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
		ChannelSize: 1, // 很小的缓冲区
	}

	engine := NewEngine(cfg)
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 添加卖单
	for i := 0; i < 5; i++ {
		sell := createEngineTestOrder("sell-"+decimal.NewFromInt(int64(i)).String(), model.OrderSideSell, 100.0, 1.0, int64(i+1))
		engine.ProcessOrder(ctx, sell)
	}

	// 买单会产生多次成交，但通道满了应该不会阻塞
	buy := createEngineTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0, 100)
	result, err := engine.ProcessOrder(ctx, buy)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// 即使通道满了，处理也不应该阻塞
}

func TestEngine_SequentialAccess(t *testing.T) {
	// 注意: Engine 设计为单线程处理 (每个市场一个 Engine)
	// Kafka 分区按 market 路由，保证同一市场订单串行消费
	// 此测试验证顺序处理能力
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 按顺序处理订单 (模拟 Kafka 消费)
	// 交替添加买卖单，使用递增的序列号
	for i := 0; i < 100; i++ {
		seq := int64(i + 1)
		var order *model.Order
		if i%2 == 0 {
			order = createEngineTestOrder("sell-"+decimal.NewFromInt(seq).String(), model.OrderSideSell, 100.0, 1.0, seq)
		} else {
			order = createEngineTestOrder("buy-"+decimal.NewFromInt(seq).String(), model.OrderSideBuy, 100.0, 1.0, seq)
		}
		engine.ProcessOrder(ctx, order)
	}

	// 验证统计
	stats := engine.GetStats()
	assert.Equal(t, int64(100), stats.OrdersProcessed)
	// 应该有成交 (买单与之前的卖单匹配)
	assert.Greater(t, stats.TradesGenerated, int64(0))
}

func TestEngine_UpdatesChan(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 添加订单会产生订单簿更新
	order := createEngineTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0, 1)
	engine.ProcessOrder(ctx, order)

	// 验证 UpdatesChan 返回有效通道
	updatesChan := engine.UpdatesChan()
	assert.NotNil(t, updatesChan)

	// 从通道读取更新
	select {
	case update := <-updatesChan:
		assert.NotNil(t, update)
		assert.Equal(t, "BTC-USDC", update.Market)
	case <-time.After(time.Second):
		t.Error("Expected update in channel")
	}
}

func TestEngine_SetOrderBook(t *testing.T) {
	engine := newTestEngine()

	// 创建新的订单簿
	newOB := orderbook.NewOrderBook("BTC-USDC")

	// 添加一些订单到新订单簿
	order := &model.Order{
		OrderID:   "restored-1",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(50000.0),
		Amount:    decimal.NewFromFloat(1.0),
		Remaining: decimal.NewFromFloat(1.0),
		Timestamp: time.Now().UnixNano(),
	}
	newOB.AddOrder(order)

	// 设置订单簿
	engine.SetOrderBook(newOB)

	// 验证订单簿已更新
	assert.Equal(t, newOB, engine.GetOrderBook())

	// 验证可以获取恢复的订单
	found := engine.GetOrder("restored-1")
	assert.NotNil(t, found)
	assert.Equal(t, "restored-1", found.OrderID)
}

func TestEngine_SetOrderBook_UpdatesMatcher(t *testing.T) {
	engine := newTestEngine()
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 创建新订单簿并添加卖单
	newOB := orderbook.NewOrderBook("BTC-USDC")
	sellOrder := &model.Order{
		OrderID:   "sell-1",
		Wallet:    "0xseller",
		Market:    "BTC-USDC",
		Side:      model.OrderSideSell,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(100.0),
		Amount:    decimal.NewFromFloat(10.0),
		Remaining: decimal.NewFromFloat(10.0),
		Timestamp: time.Now().UnixNano(),
	}
	newOB.AddOrder(sellOrder)

	// 设置订单簿
	engine.SetOrderBook(newOB)
	engine.SetInputSequence(0)

	// 买单应该能与恢复的卖单成交
	buyOrder := createEngineTestOrder("buy-1", model.OrderSideBuy, 100.0, 5.0, 1)
	result, err := engine.ProcessOrder(ctx, buyOrder)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Trades))
}

func TestEngine_ProcessCancel_EngineNotStarted(t *testing.T) {
	engine := newTestEngine()
	// 不启动引擎

	ctx := context.Background()
	cancelMsg := &model.CancelMessage{
		OrderID:   "order-1",
		Wallet:    "0xtest",
		Market:    "BTC-USDC",
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
	}

	result, err := engine.ProcessCancel(ctx, cancelMsg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "engine not running")
	assert.Nil(t, result)
}

func TestEngine_DefaultChannelSize(t *testing.T) {
	var counter int64
	cfg := &EngineConfig{
		Market: &MarketConfig{
			Symbol:     "BTC-USDC",
			BaseToken:  "BTC",
			QuoteToken: "USDC",
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 0, // 未设置，应使用默认值
	}

	engine := NewEngine(cfg)
	assert.NotNil(t, engine)
	// 通道应该有默认容量 10000
	assert.Equal(t, 10000, cap(engine.tradesChan))
}
