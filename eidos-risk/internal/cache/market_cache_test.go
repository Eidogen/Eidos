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

func TestMarketCache_SetAndGetLastPrice(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"
	price := decimal.NewFromFloat(50000.5)

	// Set price
	err := cache.SetLastPrice(ctx, market, price)
	require.NoError(t, err)

	// Get price
	got, err := cache.GetLastPrice(ctx, market)
	require.NoError(t, err)
	assert.True(t, price.Equal(got))
}

func TestMarketCache_GetLastPrice_NotFound(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	price, err := cache.GetLastPrice(ctx, "NONEXISTENT-USDT")
	require.NoError(t, err)
	assert.True(t, price.IsZero())
}

func TestMarketCache_SetAndGetTicker(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"
	ticker := map[string]string{
		"last_price":           "50000.5",
		"high_24h":             "52000",
		"low_24h":              "48000",
		"volume_24h":           "10000",
		"price_change_percent": "5.25",
	}

	// Set ticker
	err := cache.SetTicker(ctx, market, ticker)
	require.NoError(t, err)

	// Get ticker
	got, err := cache.GetTicker(ctx, market)
	require.NoError(t, err)
	assert.Equal(t, "50000.5", got["last_price"])
	assert.Equal(t, "52000", got["high_24h"])
	assert.Equal(t, "48000", got["low_24h"])
	assert.Equal(t, "10000", got["volume_24h"])
	assert.Equal(t, "5.25", got["price_change_percent"])
}

func TestMarketCache_GetPriceChange24h(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"
	ticker := map[string]string{
		"price_change_percent": "5.25",
	}

	// Set ticker
	err := cache.SetTicker(ctx, market, ticker)
	require.NoError(t, err)

	// Get price change
	change, err := cache.GetPriceChange24h(ctx, market)
	require.NoError(t, err)
	assert.True(t, change.Equal(decimal.NewFromFloat(5.25)))
}

func TestMarketCache_GetPriceChange24h_NotSet(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"
	ticker := map[string]string{
		"volume_24h": "10000",
	}

	// Set ticker without price_change_percent
	err := cache.SetTicker(ctx, market, ticker)
	require.NoError(t, err)

	// Should return zero
	change, err := cache.GetPriceChange24h(ctx, market)
	require.NoError(t, err)
	assert.True(t, change.IsZero())
}

func TestMarketCache_GetVolume24h(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"
	ticker := map[string]string{
		"volume_24h": "12345.67",
	}

	err := cache.SetTicker(ctx, market, ticker)
	require.NoError(t, err)

	volume, err := cache.GetVolume24h(ctx, market)
	require.NoError(t, err)
	assert.True(t, volume.Equal(decimal.NewFromFloat(12345.67)))
}

func TestMarketCache_BatchSetLastPrice(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	prices := map[string]decimal.Decimal{
		"BTC-USDT": decimal.NewFromFloat(50000),
		"ETH-USDT": decimal.NewFromFloat(3000),
		"SOL-USDT": decimal.NewFromFloat(100),
	}

	// Batch set
	err := cache.BatchSetLastPrice(ctx, prices)
	require.NoError(t, err)

	// Verify each price
	btcPrice, err := cache.GetLastPrice(ctx, "BTC-USDT")
	require.NoError(t, err)
	assert.True(t, btcPrice.Equal(decimal.NewFromFloat(50000)))

	ethPrice, err := cache.GetLastPrice(ctx, "ETH-USDT")
	require.NoError(t, err)
	assert.True(t, ethPrice.Equal(decimal.NewFromFloat(3000)))

	solPrice, err := cache.GetLastPrice(ctx, "SOL-USDT")
	require.NoError(t, err)
	assert.True(t, solPrice.Equal(decimal.NewFromFloat(100)))
}

func TestMarketCache_BatchSetLastPrice_Empty(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	// Empty batch should not error
	err := cache.BatchSetLastPrice(ctx, map[string]decimal.Decimal{})
	require.NoError(t, err)
}

func TestMarketCache_BatchGetLastPrice(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	// Set some prices
	err := cache.SetLastPrice(ctx, "BTC-USDT", decimal.NewFromFloat(50000))
	require.NoError(t, err)
	err = cache.SetLastPrice(ctx, "ETH-USDT", decimal.NewFromFloat(3000))
	require.NoError(t, err)

	// Batch get
	markets := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"} // SOL not set
	prices, err := cache.BatchGetLastPrice(ctx, markets)
	require.NoError(t, err)

	assert.True(t, prices["BTC-USDT"].Equal(decimal.NewFromFloat(50000)))
	assert.True(t, prices["ETH-USDT"].Equal(decimal.NewFromFloat(3000)))
	// SOL should not be in result (not set)
	_, hasSol := prices["SOL-USDT"]
	assert.False(t, hasSol)
}

func TestMarketCache_BatchGetLastPrice_Empty(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	prices, err := cache.BatchGetLastPrice(ctx, []string{})
	require.NoError(t, err)
	assert.Empty(t, prices)
}

func TestMarketCache_IsMarketActive(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"

	// Initially not active
	active, err := cache.IsMarketActive(ctx, market)
	require.NoError(t, err)
	assert.False(t, active)

	// Set price
	err = cache.SetLastPrice(ctx, market, decimal.NewFromFloat(50000))
	require.NoError(t, err)

	// Now should be active
	active, err = cache.IsMarketActive(ctx, market)
	require.NoError(t, err)
	assert.True(t, active)
}

func TestMarketCache_GetMarketStats(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	market := "BTC-USDT"

	// Set price and ticker
	err := cache.SetLastPrice(ctx, market, decimal.NewFromFloat(50000.5))
	require.NoError(t, err)

	ticker := map[string]string{
		"high_24h":   "52000",
		"low_24h":    "48000",
		"volume_24h": "10000",
	}
	err = cache.SetTicker(ctx, market, ticker)
	require.NoError(t, err)

	// Get stats
	stats, err := cache.GetMarketStats(ctx, market)
	require.NoError(t, err)

	assert.Equal(t, market, stats["market"])
	assert.Equal(t, "50000.5", stats["last_price"])
	assert.Equal(t, "52000", stats["high_24h"])
	assert.Equal(t, "48000", stats["low_24h"])
	assert.Equal(t, "10000", stats["volume_24h"])
}

func TestMarketCache_MultipleMarkets(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewMarketCache(client)
	ctx := context.Background()

	// Set different prices for different markets
	err := cache.SetLastPrice(ctx, "BTC-USDT", decimal.NewFromFloat(50000))
	require.NoError(t, err)
	err = cache.SetLastPrice(ctx, "ETH-USDT", decimal.NewFromFloat(3000))
	require.NoError(t, err)

	// Prices should be independent
	btcPrice, err := cache.GetLastPrice(ctx, "BTC-USDT")
	require.NoError(t, err)
	assert.True(t, btcPrice.Equal(decimal.NewFromFloat(50000)))

	ethPrice, err := cache.GetLastPrice(ctx, "ETH-USDT")
	require.NoError(t, err)
	assert.True(t, ethPrice.Equal(decimal.NewFromFloat(3000)))
}
