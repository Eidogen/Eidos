package event

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// mockOrderBookProcessor 模拟订单簿处理服务
type mockOrderBookProcessor struct {
	processUpdateCalled bool
	processUpdateError  error
	lastUpdate          *model.DepthUpdate
}

func (m *mockOrderBookProcessor) ProcessOrderBookUpdate(ctx context.Context, update *model.DepthUpdate) error {
	m.processUpdateCalled = true
	m.lastUpdate = update
	return m.processUpdateError
}

// toDepthUpdateForTest 转换消息为深度更新（测试辅助函数）
func toDepthUpdateForTest(msg *OrderBookUpdateMessage) (*model.DepthUpdate, error) {
	update := &model.DepthUpdate{
		Market:   msg.Market,
		Sequence: msg.Sequence,
		Bids:     make([]*model.PriceLevel, 0, len(msg.Bids)),
		Asks:     make([]*model.PriceLevel, 0, len(msg.Asks)),
	}

	for _, bid := range msg.Bids {
		price, err := decimal.NewFromString(bid.Price)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(bid.Amount)
		if err != nil {
			return nil, err
		}
		update.Bids = append(update.Bids, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	for _, ask := range msg.Asks {
		price, err := decimal.NewFromString(ask.Price)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(ask.Amount)
		if err != nil {
			return nil, err
		}
		update.Asks = append(update.Asks, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	return update, nil
}

func TestOrderBookHandler_Handle(t *testing.T) {
	logger := zap.NewNop()

	t.Run("success", func(t *testing.T) {
		mockSvc := &mockOrderBookProcessor{}
		handler := NewOrderBookHandlerWithProcessor(mockSvc, logger)

		msg := OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "49900.00", Amount: "10.5"},
				{Price: "49800.00", Amount: "20.0"},
			},
			Asks: []PriceLevelMessage{
				{Price: "50100.00", Amount: "5.0"},
				{Price: "50200.00", Amount: "15.0"},
			},
			Sequence: 100,
		}
		value, _ := json.Marshal(msg)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), value)
		require.NoError(t, err)
		assert.True(t, mockSvc.processUpdateCalled)
		assert.Equal(t, "BTC-USDC", mockSvc.lastUpdate.Market)
		assert.Equal(t, uint64(100), mockSvc.lastUpdate.Sequence)
		assert.Len(t, mockSvc.lastUpdate.Bids, 2)
		assert.Len(t, mockSvc.lastUpdate.Asks, 2)
	})

	t.Run("invalid json", func(t *testing.T) {
		mockSvc := &mockOrderBookProcessor{}
		handler := NewOrderBookHandlerWithProcessor(mockSvc, logger)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), []byte("invalid json"))
		require.Error(t, err)
		assert.False(t, mockSvc.processUpdateCalled)
	})

	t.Run("invalid price format", func(t *testing.T) {
		mockSvc := &mockOrderBookProcessor{}
		handler := NewOrderBookHandlerWithProcessor(mockSvc, logger)

		msg := OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "invalid", Amount: "10.5"},
			},
			Sequence: 101,
		}
		value, _ := json.Marshal(msg)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), value)
		require.Error(t, err)
		assert.False(t, mockSvc.processUpdateCalled)
	})

	t.Run("invalid amount format", func(t *testing.T) {
		mockSvc := &mockOrderBookProcessor{}
		handler := NewOrderBookHandlerWithProcessor(mockSvc, logger)

		msg := OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Asks: []PriceLevelMessage{
				{Price: "50000", Amount: "invalid"},
			},
			Sequence: 102,
		}
		value, _ := json.Marshal(msg)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), value)
		require.Error(t, err)
		assert.False(t, mockSvc.processUpdateCalled)
	})

	t.Run("service error", func(t *testing.T) {
		mockSvc := &mockOrderBookProcessor{processUpdateError: assert.AnError}
		handler := NewOrderBookHandlerWithProcessor(mockSvc, logger)

		msg := OrderBookUpdateMessage{
			Market:   "ETH-USDC",
			Sequence: 103,
		}
		value, _ := json.Marshal(msg)

		err := handler.Handle(context.Background(), []byte("ETH-USDC"), value)
		require.Error(t, err)
		assert.True(t, mockSvc.processUpdateCalled)
	})
}

func TestOrderBookHandler_Topic(t *testing.T) {
	handler := &OrderBookHandler{
		logger: zap.NewNop(),
	}
	assert.Equal(t, "orderbook-updates", handler.Topic())
}

func TestOrderBookUpdateMessage(t *testing.T) {
	t.Run("parse valid message", func(t *testing.T) {
		jsonStr := `{
			"market": "BTC-USDC",
			"bids": [
				{"price": "49900.00", "amount": "10.5"},
				{"price": "49800.00", "amount": "20.0"}
			],
			"asks": [
				{"price": "50100.00", "amount": "5.0"},
				{"price": "50200.00", "amount": "15.0"}
			],
			"sequence": 12345
		}`

		var msg OrderBookUpdateMessage
		err := json.Unmarshal([]byte(jsonStr), &msg)
		require.NoError(t, err)

		assert.Equal(t, "BTC-USDC", msg.Market)
		assert.Equal(t, uint64(12345), msg.Sequence)
		assert.Len(t, msg.Bids, 2)
		assert.Len(t, msg.Asks, 2)
		assert.Equal(t, "49900.00", msg.Bids[0].Price)
		assert.Equal(t, "10.5", msg.Bids[0].Amount)
		assert.Equal(t, "50100.00", msg.Asks[0].Price)
		assert.Equal(t, "5.0", msg.Asks[0].Amount)
	})

	t.Run("parse empty bids and asks", func(t *testing.T) {
		jsonStr := `{"market": "ETH-USDC", "bids": [], "asks": [], "sequence": 0}`

		var msg OrderBookUpdateMessage
		err := json.Unmarshal([]byte(jsonStr), &msg)
		require.NoError(t, err)

		assert.Equal(t, "ETH-USDC", msg.Market)
		assert.Len(t, msg.Bids, 0)
		assert.Len(t, msg.Asks, 0)
	})

	t.Run("parse delete update (amount=0)", func(t *testing.T) {
		jsonStr := `{
			"market": "BTC-USDC",
			"bids": [{"price": "49900.00", "amount": "0"}],
			"asks": [],
			"sequence": 12346
		}`

		var msg OrderBookUpdateMessage
		err := json.Unmarshal([]byte(jsonStr), &msg)
		require.NoError(t, err)

		assert.Equal(t, "0", msg.Bids[0].Amount)
	})
}

func TestToDepthUpdate(t *testing.T) {
	t.Run("convert bids and asks", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "49900.50", Amount: "10.123"},
			},
			Asks: []PriceLevelMessage{
				{Price: "50100.25", Amount: "5.456"},
			},
			Sequence: 200,
		}

		update, err := toDepthUpdateForTest(msg)
		require.NoError(t, err)

		assert.Equal(t, "BTC-USDC", update.Market)
		assert.Equal(t, uint64(200), update.Sequence)
		assert.Len(t, update.Bids, 1)
		assert.Len(t, update.Asks, 1)

		assert.True(t, update.Bids[0].Price.Equal(decimal.NewFromFloat(49900.50)))
		assert.True(t, update.Bids[0].Amount.Equal(decimal.RequireFromString("10.123")))
		assert.True(t, update.Asks[0].Price.Equal(decimal.NewFromFloat(50100.25)))
		assert.True(t, update.Asks[0].Amount.Equal(decimal.RequireFromString("5.456")))
	})

	t.Run("handle zero amount", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "49900.00", Amount: "0"},
			},
			Sequence: 201,
		}

		update, err := toDepthUpdateForTest(msg)
		require.NoError(t, err)
		assert.True(t, update.Bids[0].Amount.IsZero())
	})

	t.Run("invalid bid price", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "not-a-number", Amount: "10.0"},
			},
		}

		_, err := toDepthUpdateForTest(msg)
		require.Error(t, err)
	})

	t.Run("invalid bid amount", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Bids: []PriceLevelMessage{
				{Price: "49900.00", Amount: "not-a-number"},
			},
		}

		_, err := toDepthUpdateForTest(msg)
		require.Error(t, err)
	})

	t.Run("invalid ask price", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Asks: []PriceLevelMessage{
				{Price: "invalid", Amount: "10.0"},
			},
		}

		_, err := toDepthUpdateForTest(msg)
		require.Error(t, err)
	})

	t.Run("invalid ask amount", func(t *testing.T) {
		msg := &OrderBookUpdateMessage{
			Market: "BTC-USDC",
			Asks: []PriceLevelMessage{
				{Price: "50000.00", Amount: "invalid"},
			},
		}

		_, err := toDepthUpdateForTest(msg)
		require.Error(t, err)
	})
}

func BenchmarkOrderBookUpdateUnmarshal(b *testing.B) {
	jsonStr := `{
		"market": "BTC-USDC",
		"bids": [
			{"price": "49900.00", "amount": "10.5"},
			{"price": "49800.00", "amount": "20.0"},
			{"price": "49700.00", "amount": "30.0"},
			{"price": "49600.00", "amount": "40.0"},
			{"price": "49500.00", "amount": "50.0"}
		],
		"asks": [
			{"price": "50100.00", "amount": "5.0"},
			{"price": "50200.00", "amount": "15.0"},
			{"price": "50300.00", "amount": "25.0"},
			{"price": "50400.00", "amount": "35.0"},
			{"price": "50500.00", "amount": "45.0"}
		],
		"sequence": 12345
	}`
	data := []byte(jsonStr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msg OrderBookUpdateMessage
		_ = json.Unmarshal(data, &msg)
	}
}

func BenchmarkToDepthUpdate(b *testing.B) {
	msg := &OrderBookUpdateMessage{
		Market: "BTC-USDC",
		Bids: []PriceLevelMessage{
			{Price: "49900.00", Amount: "10.5"},
			{Price: "49800.00", Amount: "20.0"},
			{Price: "49700.00", Amount: "30.0"},
			{Price: "49600.00", Amount: "40.0"},
			{Price: "49500.00", Amount: "50.0"},
		},
		Asks: []PriceLevelMessage{
			{Price: "50100.00", Amount: "5.0"},
			{Price: "50200.00", Amount: "15.0"},
			{Price: "50300.00", Amount: "25.0"},
			{Price: "50400.00", Amount: "35.0"},
			{Price: "50500.00", Amount: "45.0"},
		},
		Sequence: 12345,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = toDepthUpdateForTest(msg)
	}
}
