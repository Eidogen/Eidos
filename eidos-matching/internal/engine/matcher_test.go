// Package engine 撮合器单元测试
package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// 测试辅助函数

var tradeIDCounter int64

func mockTradeIDGen() string {
	id := atomic.AddInt64(&tradeIDCounter, 1)
	return "trade-" + decimal.NewFromInt(id).String()
}

func newTestMatcher() *Matcher {
	cfg := &MarketConfig{
		Symbol:        "BTC-USDC",
		BaseToken:     "BTC",
		QuoteToken:    "USDC",
		PriceDecimals: 2,
		SizeDecimals:  4,
		MinSize:       decimal.NewFromFloat(0.0001),
		TickSize:      decimal.NewFromFloat(0.01),
		MakerFeeRate:  decimal.NewFromFloat(0.001), // 0.1%
		TakerFeeRate:  decimal.NewFromFloat(0.002), // 0.2%
		MaxSlippage:   decimal.NewFromFloat(0.05),  // 5%
	}

	ob := orderbook.NewOrderBook("BTC-USDC")
	return NewMatcher(cfg, ob, mockTradeIDGen)
}

func createOrder(id string, side model.OrderSide, orderType model.OrderType, tif model.TimeInForce, price, amount float64) *model.Order {
	return &model.Order{
		OrderID:     id,
		Wallet:      "0x" + id,
		Market:      "BTC-USDC",
		Side:        side,
		Type:        orderType,
		TimeInForce: tif,
		Price:       decimal.NewFromFloat(price),
		Amount:      decimal.NewFromFloat(amount),
		Remaining:   decimal.NewFromFloat(amount),
		Timestamp:   time.Now().UnixNano(),
		Sequence:    1,
	}
}

// 基础撮合测试

func TestMatcher_NoMatch_EmptyOrderBook(t *testing.T) {
	m := newTestMatcher()

	// 买单进入空订单簿，应该直接加入
	order := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	result, err := m.Match(order)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Trades)
	assert.Equal(t, 1, len(result.OrderUpdates)) // ADD 更新
	assert.Equal(t, "ADD", result.OrderUpdates[0].UpdateType)
	assert.False(t, result.TakerFilled)
	assert.False(t, result.TakerCancelled)
}

func TestMatcher_NoMatch_PriceNotCross(t *testing.T) {
	m := newTestMatcher()

	// 先添加卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 101.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// 买单价格低于卖单，不能成交
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Empty(t, result.Trades)
	assert.Equal(t, 1, len(result.OrderUpdates))
	assert.False(t, result.TakerFilled)
}

func TestMatcher_FullMatch_SingleOrder(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单 @100, qty=10
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// 买单 @100, qty=10 应该完全成交
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)
	assert.False(t, result.TakerCancelled)

	trade := result.Trades[0]
	assert.Equal(t, "sell-1", trade.MakerOrderID)
	assert.Equal(t, "buy-1", trade.TakerOrderID)
	assert.True(t, decimal.NewFromFloat(100.0).Equal(trade.Price))
	assert.True(t, decimal.NewFromFloat(10.0).Equal(trade.Amount))
	assert.True(t, trade.MakerOrderFilled)
	assert.True(t, trade.TakerOrderFilled)

	// 验证手续费
	expectedQuoteAmount := decimal.NewFromFloat(1000.0) // 100 * 10
	expectedMakerFee := expectedQuoteAmount.Mul(decimal.NewFromFloat(0.001))
	expectedTakerFee := expectedQuoteAmount.Mul(decimal.NewFromFloat(0.002))
	assert.True(t, expectedMakerFee.Equal(trade.MakerFee))
	assert.True(t, expectedTakerFee.Equal(trade.TakerFee))
}

func TestMatcher_PartialMatch_TakerLarger(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单 @100, qty=5
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	m.orderBook.AddOrder(sellOrder)

	// 买单 @100, qty=10，只能成交 5
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.False(t, result.TakerFilled)

	trade := result.Trades[0]
	assert.True(t, decimal.NewFromFloat(5.0).Equal(trade.Amount))
	assert.True(t, trade.MakerOrderFilled)
	assert.False(t, trade.TakerOrderFilled)

	// 剩余部分应该加入订单簿 (GTC)
	addUpdate := result.OrderUpdates[len(result.OrderUpdates)-1]
	assert.Equal(t, "ADD", addUpdate.UpdateType)
	assert.True(t, decimal.NewFromFloat(5.0).Equal(addUpdate.Size))
}

func TestMatcher_PartialMatch_MakerLarger(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单 @100, qty=10
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// 买单 @100, qty=3
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 3.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)

	trade := result.Trades[0]
	assert.True(t, decimal.NewFromFloat(3.0).Equal(trade.Amount))
	assert.False(t, trade.MakerOrderFilled)
	assert.True(t, trade.TakerOrderFilled)

	// Maker 订单应该有更新
	updateFound := false
	for _, u := range result.OrderUpdates {
		if u.UpdateType == "UPDATE" {
			updateFound = true
			assert.True(t, decimal.NewFromFloat(7.0).Equal(u.Size)) // 剩余 7
		}
	}
	assert.True(t, updateFound)
}

func TestMatcher_MultipleMatches_PriceTimePriority(t *testing.T) {
	m := newTestMatcher()

	// 添加多个卖单 (时间优先)
	sell1 := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	sell1.Timestamp = 1000
	m.orderBook.AddOrder(sell1)

	sell2 := createOrder("sell-2", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	sell2.Timestamp = 2000
	m.orderBook.AddOrder(sell2)

	sell3 := createOrder("sell-3", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 99.0, 3.0) // 更优价格
	sell3.Timestamp = 3000
	m.orderBook.AddOrder(sell3)

	// 买单 @100, qty=10
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 3, len(result.Trades))

	// 验证成交顺序: 先成交 99 (价格优先)，再成交 100 的两个 (时间优先)
	assert.Equal(t, "sell-3", result.Trades[0].MakerOrderID)
	assert.True(t, decimal.NewFromFloat(99.0).Equal(result.Trades[0].Price))

	assert.Equal(t, "sell-1", result.Trades[1].MakerOrderID)
	assert.True(t, decimal.NewFromFloat(100.0).Equal(result.Trades[1].Price))

	assert.Equal(t, "sell-2", result.Trades[2].MakerOrderID)
}

// IOC 订单测试

func TestMatcher_IOC_PartialFill(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单 @100, qty=5
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	m.orderBook.AddOrder(sellOrder)

	// IOC 买单 @100, qty=10，只能成交 5，剩余取消
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceIOC, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.False(t, result.TakerFilled)
	assert.True(t, result.TakerCancelled) // 剩余部分被取消

	// IOC 不应该加入订单簿
	for _, u := range result.OrderUpdates {
		if u.UpdateType == "ADD" && u.Side == model.OrderSideBuy {
			t.Error("IOC order should not be added to orderbook")
		}
	}
}

func TestMatcher_IOC_NoMatch(t *testing.T) {
	m := newTestMatcher()

	// IOC 买单进入空订单簿
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceIOC, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Empty(t, result.Trades)
	assert.True(t, result.TakerCancelled)
}

// FOK 订单测试

func TestMatcher_FOK_FullFill(t *testing.T) {
	m := newTestMatcher()

	// 添加足够的流动性
	sell1 := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	sell2 := createOrder("sell-2", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	m.orderBook.AddOrder(sell1)
	m.orderBook.AddOrder(sell2)

	// FOK 买单可以完全成交
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceFOK, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Trades))
	assert.True(t, result.TakerFilled)
	assert.False(t, result.TakerCancelled)
}

func TestMatcher_FOK_NotEnoughLiquidity(t *testing.T) {
	m := newTestMatcher()

	// 添加不足的流动性
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	m.orderBook.AddOrder(sellOrder)

	// FOK 买单需要 10，只有 5，应该被取消
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceFOK, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Empty(t, result.Trades) // 没有成交
	assert.True(t, result.TakerCancelled)

	// 原有卖单应该不受影响
	assert.NotNil(t, m.orderBook.GetOrder("sell-1"))
}

func TestMatcher_FOK_PriceTooFar(t *testing.T) {
	m := newTestMatcher()

	// 添加高价卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 102.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// FOK 买单价格不够高
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceFOK, 100.0, 10.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Empty(t, result.Trades)
	assert.True(t, result.TakerCancelled)
}

// 市价单测试

func TestMatcher_MarketOrder_Success(t *testing.T) {
	m := newTestMatcher()

	// 先设置一个成交价作为参考
	m.updateLastPrice(decimal.NewFromFloat(100.0), time.Now().UnixNano())

	// 添加卖单
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	// 市价买单
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeMarket, model.TimeInForceGTC, 0, 5.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)
}

func TestMatcher_MarketOrder_NoLiquidity(t *testing.T) {
	m := newTestMatcher()

	// 设置参考价
	m.updateLastPrice(decimal.NewFromFloat(100.0), time.Now().UnixNano())

	// 市价买单进入空订单簿
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeMarket, model.TimeInForceGTC, 0, 5.0)
	result, err := m.Match(buyOrder)

	assert.Error(t, err) // 应该返回错误
	assert.True(t, result.TakerCancelled)
}

func TestMatcher_MarketOrder_NoReferencePrice(t *testing.T) {
	m := newTestMatcher()

	// 不设置任何参考价，空订单簿

	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeMarket, model.TimeInForceGTC, 0, 5.0)
	_, err := m.Match(buyOrder)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no reference price")
}

func TestMatcher_MarketOrder_SlippageProtection(t *testing.T) {
	m := newTestMatcher()

	// 设置参考价
	m.updateLastPrice(decimal.NewFromFloat(100.0), time.Now().UnixNano())

	// 添加高价卖单 (超过滑点保护)
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 110.0, 10.0) // 10% 滑点
	m.orderBook.AddOrder(sellOrder)

	// 市价买单 (MaxSlippage = 5%)
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeMarket, model.TimeInForceGTC, 0, 5.0)
	result, err := m.Match(buyOrder)

	assert.NoError(t, err)
	// 由于滑点保护，不应该成交
	assert.Empty(t, result.Trades)
	assert.True(t, result.TakerCancelled)
}

// 卖单测试

func TestMatcher_SellOrder_Match(t *testing.T) {
	m := newTestMatcher()

	// 添加买单
	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(buyOrder)

	// 卖单成交
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 5.0)
	result, err := m.Match(sellOrder)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Trades))
	assert.True(t, result.TakerFilled)

	trade := result.Trades[0]
	assert.Equal(t, model.OrderSideBuy, trade.MakerSide)
	assert.Equal(t, "buy-1", trade.MakerOrderID)
	assert.Equal(t, "sell-1", trade.TakerOrderID)
}

// 边界条件测试

func TestMatcher_UnsupportedOrderType(t *testing.T) {
	m := newTestMatcher()

	order := &model.Order{
		OrderID:     "test-1",
		Market:      "BTC-USDC",
		Side:        model.OrderSideBuy,
		Type:        model.OrderType(99), // 无效类型
		Price:       decimal.NewFromFloat(100.0),
		Amount:      decimal.NewFromFloat(10.0),
		Remaining:   decimal.NewFromFloat(10.0),
		TimeInForce: model.TimeInForceGTC,
	}

	_, err := m.Match(order)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported order type")
}

func TestMatcher_UpdateIndexPrice(t *testing.T) {
	m := newTestMatcher()

	m.UpdateIndexPrice(decimal.NewFromFloat(100.0))

	// 验证指数价格被设置
	refPrice := m.getReferencePrice()
	assert.True(t, decimal.NewFromFloat(100.0).Equal(refPrice))
}

func TestMatcher_GetLastPrice(t *testing.T) {
	m := newTestMatcher()

	assert.True(t, m.GetLastPrice().IsZero())

	m.updateLastPrice(decimal.NewFromFloat(100.0), time.Now().UnixNano())
	assert.True(t, decimal.NewFromFloat(100.0).Equal(m.GetLastPrice()))
}

func TestMatcher_GetStats(t *testing.T) {
	m := newTestMatcher()

	// 添加卖单并成交
	sellOrder := createOrder("sell-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.orderBook.AddOrder(sellOrder)

	buyOrder := createOrder("buy-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 100.0, 10.0)
	m.Match(buyOrder)

	stats := m.GetStats()
	assert.Equal(t, int64(1), stats.TotalTrades)
	assert.True(t, decimal.NewFromFloat(10.0).Equal(stats.TotalVolume))
	assert.True(t, decimal.NewFromFloat(100.0).Equal(stats.LastPrice))
}

// 参考价格优先级测试

func TestMatcher_ReferencePricePriority(t *testing.T) {
	m := newTestMatcher()

	// 1. 无任何价格
	refPrice := m.getReferencePrice()
	assert.True(t, refPrice.IsZero())

	// 2. 只有指数价格
	m.UpdateIndexPrice(decimal.NewFromFloat(100.0))
	refPrice = m.getReferencePrice()
	assert.True(t, decimal.NewFromFloat(100.0).Equal(refPrice))

	// 3. 有买卖盘，使用中间价
	bid := createOrder("bid-1", model.OrderSideBuy, model.OrderTypeLimit, model.TimeInForceGTC, 99.0, 10.0)
	ask := createOrder("ask-1", model.OrderSideSell, model.OrderTypeLimit, model.TimeInForceGTC, 101.0, 10.0)
	m.orderBook.AddOrder(bid)
	m.orderBook.AddOrder(ask)

	refPrice = m.getReferencePrice()
	assert.True(t, decimal.NewFromFloat(100.0).Equal(refPrice)) // (99+101)/2

	// 4. 有最近成交价，优先使用
	m.updateLastPrice(decimal.NewFromFloat(100.5), time.Now().UnixNano())
	refPrice = m.getReferencePrice()
	assert.True(t, decimal.NewFromFloat(100.5).Equal(refPrice))
}
