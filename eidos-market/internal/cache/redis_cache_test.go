package cache

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

func TestRedisCache_SetTicker(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	ticker := &model.Ticker{
		Market:    "BTC-USDC",
		LastPrice: decimal.NewFromFloat(50000),
		Timestamp: 1700000000000,
	}

	t.Run("success with custom TTL", func(t *testing.T) {
		data, _ := json.Marshal(ticker)
		mock.ExpectSet("eidos:ticker:BTC-USDC", data, 30*time.Second).SetVal("OK")

		err := cache.SetTicker(ctx, "BTC-USDC", ticker, 30*time.Second)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success with default TTL", func(t *testing.T) {
		data, _ := json.Marshal(ticker)
		mock.ExpectSet("eidos:ticker:BTC-USDC", data, DefaultTickerTTL).SetVal("OK")

		err := cache.SetTicker(ctx, "BTC-USDC", ticker, 0)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_GetTicker(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		ticker := &model.Ticker{
			Market:    "BTC-USDC",
			LastPrice: decimal.NewFromFloat(50000),
			Timestamp: 1700000000000,
		}
		data, _ := json.Marshal(ticker)
		mock.ExpectGet("eidos:ticker:BTC-USDC").SetVal(string(data))

		result, err := cache.GetTicker(ctx, "BTC-USDC")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "BTC-USDC", result.Market)
		assert.True(t, result.LastPrice.Equal(decimal.NewFromFloat(50000)))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectGet("eidos:ticker:UNKNOWN").RedisNil()

		result, err := cache.GetTicker(ctx, "UNKNOWN")
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("redis error", func(t *testing.T) {
		mock.ExpectGet("eidos:ticker:ERROR").SetErr(redis.ErrClosed)

		result, err := cache.GetTicker(ctx, "ERROR")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_SetDepth(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	depth := &model.Depth{
		Market: "BTC-USDC",
		Bids: []*model.PriceLevel{
			{Price: decimal.NewFromFloat(49900), Amount: decimal.NewFromFloat(10)},
		},
		Asks: []*model.PriceLevel{
			{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
		},
		Timestamp: 1700000000000,
	}

	t.Run("success with custom TTL", func(t *testing.T) {
		data, _ := json.Marshal(depth)
		mock.ExpectSet("eidos:orderbook:BTC-USDC", data, 10*time.Second).SetVal("OK")

		err := cache.SetDepth(ctx, "BTC-USDC", depth, 10*time.Second)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success with default TTL", func(t *testing.T) {
		data, _ := json.Marshal(depth)
		mock.ExpectSet("eidos:orderbook:BTC-USDC", data, DefaultDepthTTL).SetVal("OK")

		err := cache.SetDepth(ctx, "BTC-USDC", depth, 0)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_GetDepth(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		depth := &model.Depth{
			Market: "BTC-USDC",
			Bids: []*model.PriceLevel{
				{Price: decimal.NewFromFloat(49900), Amount: decimal.NewFromFloat(10)},
			},
			Asks: []*model.PriceLevel{
				{Price: decimal.NewFromFloat(50100), Amount: decimal.NewFromFloat(5)},
			},
			Timestamp: 1700000000000,
		}
		data, _ := json.Marshal(depth)
		mock.ExpectGet("eidos:orderbook:BTC-USDC").SetVal(string(data))

		result, err := cache.GetDepth(ctx, "BTC-USDC")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "BTC-USDC", result.Market)
		assert.Len(t, result.Bids, 1)
		assert.Len(t, result.Asks, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectGet("eidos:orderbook:UNKNOWN").RedisNil()

		result, err := cache.GetDepth(ctx, "UNKNOWN")
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_PushTrade(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	trade := &model.Trade{
		TradeID:   "trade-1",
		Market:    "BTC-USDC",
		Price:     decimal.NewFromFloat(50000),
		Amount:    decimal.NewFromFloat(1.5),
		Timestamp: 1700000000000,
	}

	t.Run("success", func(t *testing.T) {
		data, _ := json.Marshal(trade)
		mock.ExpectLPush("eidos:trades:BTC-USDC", data).SetVal(1)
		mock.ExpectLTrim("eidos:trades:BTC-USDC", int64(0), int64(MaxTradesLen-1)).SetVal("OK")

		err := cache.PushTrade(ctx, "BTC-USDC", trade)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_GetRecentTrades(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		trades := []*model.Trade{
			{TradeID: "trade-1", Market: "BTC-USDC", Price: decimal.NewFromFloat(50000), Timestamp: 1700000000000},
			{TradeID: "trade-2", Market: "BTC-USDC", Price: decimal.NewFromFloat(50100), Timestamp: 1700000001000},
		}
		var data []string
		for _, t := range trades {
			d, _ := json.Marshal(t)
			data = append(data, string(d))
		}
		mock.ExpectLRange("eidos:trades:BTC-USDC", int64(0), int64(49)).SetVal(data)

		result, err := cache.GetRecentTrades(ctx, "BTC-USDC", 50)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "trade-1", result[0].TradeID)
		assert.Equal(t, "trade-2", result[1].TradeID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty list", func(t *testing.T) {
		mock.ExpectLRange("eidos:trades:EMPTY", int64(0), int64(49)).SetVal([]string{})

		result, err := cache.GetRecentTrades(ctx, "EMPTY", 50)
		require.NoError(t, err)
		assert.Len(t, result, 0)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_SetKline(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	kline := &model.Kline{
		Market:   "BTC-USDC",
		Interval: model.Interval1m,
		OpenTime: 1700000000000,
		Open:     decimal.NewFromFloat(50000),
		Close:    decimal.NewFromFloat(50050),
	}

	t.Run("success with custom TTL", func(t *testing.T) {
		data, _ := json.Marshal(kline)
		mock.ExpectSet("eidos:kline:BTC-USDC:1m", data, 120*time.Second).SetVal("OK")

		err := cache.SetKline(ctx, "BTC-USDC", model.Interval1m, kline, 120*time.Second)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success with default TTL", func(t *testing.T) {
		data, _ := json.Marshal(kline)
		mock.ExpectSet("eidos:kline:BTC-USDC:1m", data, DefaultKlineTTL).SetVal("OK")

		err := cache.SetKline(ctx, "BTC-USDC", model.Interval1m, kline, 0)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_GetKline(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		kline := &model.Kline{
			Market:   "BTC-USDC",
			Interval: model.Interval1m,
			OpenTime: 1700000000000,
			Open:     decimal.NewFromFloat(50000),
			Close:    decimal.NewFromFloat(50050),
		}
		data, _ := json.Marshal(kline)
		mock.ExpectGet("eidos:kline:BTC-USDC:1m").SetVal(string(data))

		result, err := cache.GetKline(ctx, "BTC-USDC", model.Interval1m)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "BTC-USDC", result.Market)
		assert.True(t, result.Open.Equal(decimal.NewFromFloat(50000)))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectGet("eidos:kline:UNKNOWN:1m").RedisNil()

		result, err := cache.GetKline(ctx, "UNKNOWN", model.Interval1m)
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_SetMarkets(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	markets := []*model.Market{
		{Symbol: "BTC-USDC", BaseToken: "BTC", QuoteToken: "USDC"},
		{Symbol: "ETH-USDC", BaseToken: "ETH", QuoteToken: "USDC"},
	}

	t.Run("success", func(t *testing.T) {
		data, _ := json.Marshal(markets)
		mock.ExpectSet("eidos:markets", data, time.Hour).SetVal("OK")

		err := cache.SetMarkets(ctx, markets, time.Hour)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_GetMarkets(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		markets := []*model.Market{
			{Symbol: "BTC-USDC", BaseToken: "BTC", QuoteToken: "USDC"},
			{Symbol: "ETH-USDC", BaseToken: "ETH", QuoteToken: "USDC"},
		}
		data, _ := json.Marshal(markets)
		mock.ExpectGet("eidos:markets").SetVal(string(data))

		result, err := cache.GetMarkets(ctx)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "BTC-USDC", result[0].Symbol)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectGet("eidos:markets").RedisNil()

		result, err := cache.GetMarkets(ctx)
		require.NoError(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRedisCache_Ping(t *testing.T) {
	db, mock := redismock.NewClientMock()
	cache := NewRedisCache(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mock.ExpectPing().SetVal("PONG")

		err := cache.Ping(ctx)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectPing().SetErr(redis.ErrClosed)

		err := cache.Ping(ctx)
		require.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCacheKeyFormats(t *testing.T) {
	assert.Equal(t, "eidos:ticker:%s", KeyTicker)
	assert.Equal(t, "eidos:orderbook:%s", KeyDepth)
	assert.Equal(t, "eidos:trades:%s", KeyTrades)
	assert.Equal(t, "eidos:kline:%s:%s", KeyKline)
	assert.Equal(t, "eidos:ticker_bucket:%s:%d", KeyTickerBucket)
	assert.Equal(t, "eidos:markets", KeyMarkets)
}

func TestDefaultTTLs(t *testing.T) {
	assert.Equal(t, 10*time.Second, DefaultTickerTTL)
	assert.Equal(t, 5*time.Second, DefaultDepthTTL)
	assert.Equal(t, 60*time.Second, DefaultKlineTTL)
	assert.Equal(t, 100, MaxTradesLen)
}
