package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/go-redis/redismock/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

func TestPubSub_PublishTicker(t *testing.T) {
	db, mock := redismock.NewClientMock()
	pubsub := NewPubSub(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	ticker := &model.Ticker{
		Market:    "BTC-USDC",
		LastPrice: decimal.NewFromFloat(50000),
		Timestamp: 1700000000000,
	}

	t.Run("success", func(t *testing.T) {
		data, _ := json.Marshal(ticker)
		mock.ExpectPublish("eidos:ticker:BTC-USDC", data).SetVal(1)

		err := pubsub.PublishTicker(ctx, "BTC-USDC", ticker)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPubSub_PublishDepth(t *testing.T) {
	db, mock := redismock.NewClientMock()
	pubsub := NewPubSub(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	t.Run("success", func(t *testing.T) {
		data, _ := json.Marshal(depth)
		mock.ExpectPublish("eidos:depth:BTC-USDC", data).SetVal(1)

		err := pubsub.PublishDepth(ctx, "BTC-USDC", depth)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPubSub_PublishTrade(t *testing.T) {
	db, mock := redismock.NewClientMock()
	pubsub := NewPubSub(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
		mock.ExpectPublish("eidos:trades:BTC-USDC", data).SetVal(1)

		err := pubsub.PublishTrade(ctx, "BTC-USDC", trade)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPubSub_PublishKline(t *testing.T) {
	db, mock := redismock.NewClientMock()
	pubsub := NewPubSub(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	kline := &model.Kline{
		Market:   "BTC-USDC",
		Interval: model.Interval1m,
		OpenTime: 1700000000000,
		Open:     decimal.NewFromFloat(50000),
		Close:    decimal.NewFromFloat(50050),
	}

	t.Run("success", func(t *testing.T) {
		data, _ := json.Marshal(kline)
		mock.ExpectPublish("eidos:kline:BTC-USDC:1m", data).SetVal(1)

		err := pubsub.PublishKline(ctx, "BTC-USDC", model.Interval1m, kline)
		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestPubSubChannelFormats(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []interface{}
		expected string
	}{
		{"ticker channel", ChannelTicker, []interface{}{"BTC-USDC"}, "eidos:ticker:BTC-USDC"},
		{"depth channel", ChannelDepth, []interface{}{"BTC-USDC"}, "eidos:depth:BTC-USDC"},
		{"trades channel", ChannelTrades, []interface{}{"BTC-USDC"}, "eidos:trades:BTC-USDC"},
		{"kline channel", ChannelKline, []interface{}{"BTC-USDC", "1m"}, "eidos:kline:BTC-USDC:1m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fmt.Sprintf(tt.format, tt.args...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPubSubConstants(t *testing.T) {
	assert.Equal(t, "eidos:ticker:%s", ChannelTicker)
	assert.Equal(t, "eidos:depth:%s", ChannelDepth)
	assert.Equal(t, "eidos:trades:%s", ChannelTrades)
	assert.Equal(t, "eidos:kline:%s:%s", ChannelKline)
}
