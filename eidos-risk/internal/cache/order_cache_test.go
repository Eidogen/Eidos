package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderCache_AddAndGetOrder(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	order := &OrderInfo{
		OrderID: "order-001",
		Market:  "BTC-USDT",
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}

	// Add order
	err := cache.AddOrder(ctx, wallet, order)
	require.NoError(t, err)

	// Get order
	got, err := cache.GetOrder(ctx, wallet, order.Market, order.OrderID)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, order.OrderID, got.OrderID)
	assert.Equal(t, order.Market, got.Market)
	assert.Equal(t, order.Side, got.Side)
	assert.True(t, order.Price.Equal(got.Price))
	assert.True(t, order.Amount.Equal(got.Amount))
}

func TestOrderCache_GetOrderNotFound(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	got, err := cache.GetOrder(ctx, "0xwallet", "BTC-USDT", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestOrderCache_RemoveOrder(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	order := &OrderInfo{
		OrderID: "order-001",
		Market:  "BTC-USDT",
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}

	// Add and then remove
	err := cache.AddOrder(ctx, wallet, order)
	require.NoError(t, err)

	err = cache.RemoveOrder(ctx, wallet, order.Market, order.OrderID)
	require.NoError(t, err)

	// Should not be found
	got, err := cache.GetOrder(ctx, wallet, order.Market, order.OrderID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestOrderCache_GetUserOrders(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "BTC-USDT"

	orders := []*OrderInfo{
		{OrderID: "order-001", Market: market, Side: "BUY", Price: decimal.NewFromFloat(50000), Amount: decimal.NewFromFloat(0.1)},
		{OrderID: "order-002", Market: market, Side: "SELL", Price: decimal.NewFromFloat(51000), Amount: decimal.NewFromFloat(0.2)},
		{OrderID: "order-003", Market: market, Side: "BUY", Price: decimal.NewFromFloat(49000), Amount: decimal.NewFromFloat(0.3)},
	}

	// Add all orders
	for _, order := range orders {
		err := cache.AddOrder(ctx, wallet, order)
		require.NoError(t, err)
	}

	// Get all orders
	got, err := cache.GetUserOrders(ctx, wallet, market)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestOrderCache_HasOppositeOrder_SelfTradeBuy(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "BTC-USDT"

	// Add a SELL order at price 50000
	sellOrder := &OrderInfo{
		OrderID: "order-001",
		Market:  market,
		Side:    "SELL",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}
	err := cache.AddOrder(ctx, wallet, sellOrder)
	require.NoError(t, err)

	// Check if BUY at 50000 would cause self-trade (should be true)
	hasOpposite, err := cache.HasOppositeOrder(ctx, wallet, market, "BUY", decimal.NewFromFloat(50000))
	require.NoError(t, err)
	assert.True(t, hasOpposite)

	// Check if BUY at 49000 would cause self-trade (should be false)
	hasOpposite, err = cache.HasOppositeOrder(ctx, wallet, market, "BUY", decimal.NewFromFloat(49000))
	require.NoError(t, err)
	assert.False(t, hasOpposite)
}

func TestOrderCache_HasOppositeOrder_SelfTradeSell(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "BTC-USDT"

	// Add a BUY order at price 50000
	buyOrder := &OrderInfo{
		OrderID: "order-001",
		Market:  market,
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}
	err := cache.AddOrder(ctx, wallet, buyOrder)
	require.NoError(t, err)

	// Check if SELL at 50000 would cause self-trade (should be true)
	hasOpposite, err := cache.HasOppositeOrder(ctx, wallet, market, "SELL", decimal.NewFromFloat(50000))
	require.NoError(t, err)
	assert.True(t, hasOpposite)

	// Check if SELL at 51000 would cause self-trade (should be false)
	hasOpposite, err = cache.HasOppositeOrder(ctx, wallet, market, "SELL", decimal.NewFromFloat(51000))
	require.NoError(t, err)
	assert.False(t, hasOpposite)
}

func TestOrderCache_OrderCount(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Initial count should be 0
	count, err := cache.GetOpenOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Increment
	count, err = cache.IncrementOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = cache.IncrementOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Check
	count, err = cache.GetOpenOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Decrement
	err = cache.DecrementOrderCount(ctx, wallet)
	require.NoError(t, err)

	count, err = cache.GetOpenOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestOrderCache_DecrementOrderCount_FloorZero(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Decrement when count is 0 - should stay at 0
	err := cache.DecrementOrderCount(ctx, wallet)
	require.NoError(t, err)

	count, err := cache.GetOpenOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOrderCache_SetOrderCount(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Set count directly
	err := cache.SetOrderCount(ctx, wallet, 10)
	require.NoError(t, err)

	count, err := cache.GetOpenOrderCount(ctx, wallet)
	require.NoError(t, err)
	assert.Equal(t, 10, count)
}

func TestOrderCache_ClearUserOrders(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "BTC-USDT"

	// Add orders
	order := &OrderInfo{
		OrderID: "order-001",
		Market:  market,
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}
	err := cache.AddOrder(ctx, wallet, order)
	require.NoError(t, err)

	// Clear
	err = cache.ClearUserOrders(ctx, wallet, market)
	require.NoError(t, err)

	// Should be empty
	orders, err := cache.GetUserOrders(ctx, wallet, market)
	require.NoError(t, err)
	assert.Len(t, orders, 0)
}

func TestOrderCache_MultipleMarkets(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	btcOrder := &OrderInfo{
		OrderID: "btc-001",
		Market:  "BTC-USDT",
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}
	ethOrder := &OrderInfo{
		OrderID: "eth-001",
		Market:  "ETH-USDT",
		Side:    "SELL",
		Price:   decimal.NewFromFloat(3000),
		Amount:  decimal.NewFromFloat(1),
	}

	// Add orders to different markets
	err := cache.AddOrder(ctx, wallet, btcOrder)
	require.NoError(t, err)
	err = cache.AddOrder(ctx, wallet, ethOrder)
	require.NoError(t, err)

	// Get BTC orders
	btcOrders, err := cache.GetUserOrders(ctx, wallet, "BTC-USDT")
	require.NoError(t, err)
	assert.Len(t, btcOrders, 1)
	assert.Equal(t, "btc-001", btcOrders[0].OrderID)

	// Get ETH orders
	ethOrders, err := cache.GetUserOrders(ctx, wallet, "ETH-USDT")
	require.NoError(t, err)
	assert.Len(t, ethOrders, 1)
	assert.Equal(t, "eth-001", ethOrders[0].OrderID)
}

func TestOrderCache_GetAllMarketOrders(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewOrderCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Add orders to multiple markets
	btcOrder := &OrderInfo{
		OrderID: "btc-001",
		Market:  "BTC-USDT",
		Side:    "BUY",
		Price:   decimal.NewFromFloat(50000),
		Amount:  decimal.NewFromFloat(0.1),
	}
	ethOrder := &OrderInfo{
		OrderID: "eth-001",
		Market:  "ETH-USDT",
		Side:    "SELL",
		Price:   decimal.NewFromFloat(3000),
		Amount:  decimal.NewFromFloat(1),
	}

	err := cache.AddOrder(ctx, wallet, btcOrder)
	require.NoError(t, err)
	err = cache.AddOrder(ctx, wallet, ethOrder)
	require.NoError(t, err)

	// Get all market orders
	allOrders, err := cache.GetAllMarketOrders(ctx, wallet)
	require.NoError(t, err)
	assert.Len(t, allOrders, 2)
	assert.Contains(t, allOrders, "BTC-USDT")
	assert.Contains(t, allOrders, "ETH-USDT")
}
