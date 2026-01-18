// Package orderbook 订单簿单元测试
package orderbook

import (
	"testing"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewOrderBook(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	assert.Equal(t, "BTC-USDC", ob.Market)
	assert.NotNil(t, ob.Bids)
	assert.NotNil(t, ob.Asks)
	assert.NotNil(t, ob.Orders)
	assert.True(t, ob.LastPrice.IsZero())
	assert.Equal(t, 0, ob.OrderCount)
}

func createTestOrder(id string, side model.OrderSide, price, amount float64) *model.Order {
	return &model.Order{
		OrderID:   id,
		Wallet:    "0x123",
		Market:    "BTC-USDC",
		Side:      side,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(price),
		Amount:    decimal.NewFromFloat(amount),
		Remaining: decimal.NewFromFloat(amount),
		Timestamp: 1000,
		Sequence:  1,
	}
}

func TestOrderBook_AddOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 添加买单
	buyOrder := createTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0)
	ok := ob.AddOrder(buyOrder)
	assert.True(t, ok)
	assert.Equal(t, 1, ob.OrderCount)

	// 添加卖单
	sellOrder := createTestOrder("sell-1", model.OrderSideSell, 101.0, 5.0)
	ok = ob.AddOrder(sellOrder)
	assert.True(t, ok)
	assert.Equal(t, 2, ob.OrderCount)

	// 重复添加同一订单应该失败
	ok = ob.AddOrder(buyOrder)
	assert.False(t, ok)
	assert.Equal(t, 2, ob.OrderCount)
}

func TestOrderBook_RemoveOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	order := createTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0)
	ob.AddOrder(order)
	assert.Equal(t, 1, ob.OrderCount)

	// 移除存在的订单
	removed := ob.RemoveOrder("order-1")
	assert.NotNil(t, removed)
	assert.Equal(t, "order-1", removed.OrderID)
	assert.Equal(t, 0, ob.OrderCount)

	// 移除不存在的订单
	removed = ob.RemoveOrder("non-exist")
	assert.Nil(t, removed)
}

func TestOrderBook_GetOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	order := createTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0)
	ob.AddOrder(order)

	// 获取存在的订单
	found := ob.GetOrder("order-1")
	assert.NotNil(t, found)
	assert.Equal(t, "order-1", found.OrderID)

	// 获取不存在的订单
	notFound := ob.GetOrder("non-exist")
	assert.Nil(t, notFound)
}

func TestOrderBook_HasOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	order := createTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0)
	ob.AddOrder(order)

	assert.True(t, ob.HasOrder("order-1"))
	assert.False(t, ob.HasOrder("non-exist"))
}

func TestOrderBook_BestBidAndAsk(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 空订单簿
	assert.Nil(t, ob.BestBid())
	assert.Nil(t, ob.BestAsk())
	assert.True(t, ob.BestBidPrice().IsZero())
	assert.True(t, ob.BestAskPrice().IsZero())

	// 添加多个买单
	ob.AddOrder(createTestOrder("buy-1", model.OrderSideBuy, 99.0, 10.0))
	ob.AddOrder(createTestOrder("buy-2", model.OrderSideBuy, 100.0, 5.0))
	ob.AddOrder(createTestOrder("buy-3", model.OrderSideBuy, 98.0, 8.0))

	// 最优买价应该是最高价
	bestBid := ob.BestBid()
	assert.NotNil(t, bestBid)
	assert.True(t, decimal.NewFromFloat(100.0).Equal(bestBid.Price))
	assert.True(t, decimal.NewFromFloat(100.0).Equal(ob.BestBidPrice()))

	// 添加多个卖单
	ob.AddOrder(createTestOrder("sell-1", model.OrderSideSell, 101.0, 10.0))
	ob.AddOrder(createTestOrder("sell-2", model.OrderSideSell, 103.0, 5.0))
	ob.AddOrder(createTestOrder("sell-3", model.OrderSideSell, 102.0, 8.0))

	// 最优卖价应该是最低价
	bestAsk := ob.BestAsk()
	assert.NotNil(t, bestAsk)
	assert.True(t, decimal.NewFromFloat(101.0).Equal(bestAsk.Price))
	assert.True(t, decimal.NewFromFloat(101.0).Equal(ob.BestAskPrice()))
}

func TestOrderBook_Spread(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 空订单簿价差为 0
	assert.True(t, ob.Spread().IsZero())

	// 只有买单
	ob.AddOrder(createTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0))
	assert.True(t, ob.Spread().IsZero())

	// 有买卖单
	ob.AddOrder(createTestOrder("sell-1", model.OrderSideSell, 101.0, 10.0))
	spread := ob.Spread()
	assert.True(t, decimal.NewFromFloat(1.0).Equal(spread))
}

func TestOrderBook_UpdateOrderRemaining(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	order := createTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0)
	ob.AddOrder(order)

	// 部分成交 3.5
	ob.UpdateOrderRemaining(order, decimal.NewFromFloat(3.5))

	assert.True(t, decimal.NewFromFloat(6.5).Equal(order.Remaining))

	// 验证价格档位的 TotalSize 也更新了
	pl := ob.BestBid()
	assert.True(t, decimal.NewFromFloat(6.5).Equal(pl.TotalSize))
}

func TestOrderBook_RemoveOrderIfFilled(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	order := createTestOrder("order-1", model.OrderSideBuy, 100.0, 10.0)
	ob.AddOrder(order)

	// 未完全成交
	order.Remaining = decimal.NewFromFloat(5.0)
	removed := ob.RemoveOrderIfFilled(order)
	assert.False(t, removed)
	assert.Equal(t, 1, ob.OrderCount)

	// 完全成交
	order.Remaining = decimal.Zero
	removed = ob.RemoveOrderIfFilled(order)
	assert.True(t, removed)
	assert.Equal(t, 0, ob.OrderCount)
}

func TestOrderBook_Depth(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 添加多个买卖单
	ob.AddOrder(createTestOrder("buy-1", model.OrderSideBuy, 99.0, 10.0))
	ob.AddOrder(createTestOrder("buy-2", model.OrderSideBuy, 100.0, 5.0))
	ob.AddOrder(createTestOrder("buy-3", model.OrderSideBuy, 100.0, 3.0)) // 同价格
	ob.AddOrder(createTestOrder("sell-1", model.OrderSideSell, 101.0, 10.0))
	ob.AddOrder(createTestOrder("sell-2", model.OrderSideSell, 102.0, 5.0))

	// 获取全部深度
	depth, err := ob.Depth(0)
	assert.NoError(t, err)
	assert.Equal(t, "BTC-USDC", depth.Market)
	assert.Equal(t, 2, len(depth.Bids))
	assert.Equal(t, 2, len(depth.Asks))

	// 验证买单排序 (价格降序)
	assert.True(t, decimal.NewFromFloat(100.0).Equal(depth.Bids[0].Price))
	assert.True(t, decimal.NewFromFloat(8.0).Equal(depth.Bids[0].Size)) // 5+3
	assert.Equal(t, 2, depth.Bids[0].Count)

	// 验证卖单排序 (价格升序)
	assert.True(t, decimal.NewFromFloat(101.0).Equal(depth.Asks[0].Price))

	// 获取限制深度
	depth, err = ob.Depth(1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(depth.Bids))
	assert.Equal(t, 1, len(depth.Asks))
}

func TestOrderBook_NextSequence(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	assert.Equal(t, int64(1), ob.NextSequence())
	assert.Equal(t, int64(2), ob.NextSequence())
	assert.Equal(t, int64(3), ob.NextSequence())
}

func TestOrderBook_SetLastPrice(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	price := decimal.NewFromFloat(100.5)
	timestamp := int64(1234567890)
	ob.SetLastPrice(price, timestamp)

	assert.True(t, price.Equal(ob.LastPrice))
	assert.Equal(t, timestamp, ob.LastTradeAt)
}

func TestOrderBook_Stats(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	ob.AddOrder(createTestOrder("buy-1", model.OrderSideBuy, 99.0, 10.0))
	ob.AddOrder(createTestOrder("buy-2", model.OrderSideBuy, 100.0, 5.0))
	ob.AddOrder(createTestOrder("sell-1", model.OrderSideSell, 101.0, 10.0))

	ob.SetLastPrice(decimal.NewFromFloat(100.5), 123456)
	ob.TradeCount = 5
	ob.NextSequence()

	stats := ob.Stats()
	assert.Equal(t, "BTC-USDC", stats.Market)
	assert.Equal(t, 2, stats.BidLevels)
	assert.Equal(t, 1, stats.AskLevels)
	assert.Equal(t, 3, stats.OrderCount)
	assert.Equal(t, int64(5), stats.TradeCount)
	assert.True(t, decimal.NewFromFloat(100.0).Equal(stats.BestBid))
	assert.True(t, decimal.NewFromFloat(101.0).Equal(stats.BestAsk))
	assert.True(t, decimal.NewFromFloat(1.0).Equal(stats.Spread))
	assert.True(t, decimal.NewFromFloat(100.5).Equal(stats.LastPrice))
}

func TestOrderBook_PriceLevelCleanup(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 添加同一价格的多个订单
	order1 := createTestOrder("buy-1", model.OrderSideBuy, 100.0, 10.0)
	order2 := createTestOrder("buy-2", model.OrderSideBuy, 100.0, 5.0)

	ob.AddOrder(order1)
	ob.AddOrder(order2)
	assert.Equal(t, 1, ob.Bids.Len()) // 只有一个价格档位

	// 移除第一个订单
	ob.RemoveOrder("buy-1")
	assert.Equal(t, 1, ob.Bids.Len()) // 价格档位还在

	// 移除第二个订单后，价格档位应该被清理
	ob.RemoveOrder("buy-2")
	assert.Equal(t, 0, ob.Bids.Len()) // 价格档位被删除
}
