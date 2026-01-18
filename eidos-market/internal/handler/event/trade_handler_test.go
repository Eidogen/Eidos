package event

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// mockTradeProcessor 模拟成交处理服务
type mockTradeProcessor struct {
	processTradeCalled bool
	processTradeError  error
	lastTrade          *model.TradeEvent
}

func (m *mockTradeProcessor) ProcessTrade(ctx context.Context, trade *model.TradeEvent) error {
	m.processTradeCalled = true
	m.lastTrade = trade
	return m.processTradeError
}

func TestTradeHandler_Handle(t *testing.T) {
	logger := zap.NewNop()

	t.Run("success", func(t *testing.T) {
		mockSvc := &mockTradeProcessor{}
		handler := NewTradeHandlerWithProcessor(mockSvc, logger)

		tradeEvent := model.TradeEvent{
			TradeID:      "trade-123",
			Market:       "BTC-USDC",
			Price:        "50000.00",
			Amount:       "1.5",
			QuoteAmount:  "75000.00",
			MakerSide:    0, // 0=买
			MakerOrderID: "order-maker",
			TakerOrderID: "order-taker",
			Timestamp:    1700000000000,
		}
		value, _ := json.Marshal(tradeEvent)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), value)
		require.NoError(t, err)
		assert.True(t, mockSvc.processTradeCalled)
		assert.Equal(t, "trade-123", mockSvc.lastTrade.TradeID)
		assert.Equal(t, "BTC-USDC", mockSvc.lastTrade.Market)
		assert.Equal(t, "50000.00", mockSvc.lastTrade.Price)
		assert.Equal(t, "1.5", mockSvc.lastTrade.Amount)
	})

	t.Run("invalid json", func(t *testing.T) {
		mockSvc := &mockTradeProcessor{}
		handler := NewTradeHandlerWithProcessor(mockSvc, logger)

		err := handler.Handle(context.Background(), []byte("BTC-USDC"), []byte("invalid json"))
		require.Error(t, err)
		assert.False(t, mockSvc.processTradeCalled)
	})

	t.Run("service error", func(t *testing.T) {
		mockSvc := &mockTradeProcessor{processTradeError: assert.AnError}
		handler := NewTradeHandlerWithProcessor(mockSvc, logger)

		tradeEvent := model.TradeEvent{
			TradeID: "trade-456",
			Market:  "ETH-USDC",
		}
		value, _ := json.Marshal(tradeEvent)

		err := handler.Handle(context.Background(), []byte("ETH-USDC"), value)
		require.Error(t, err)
		assert.True(t, mockSvc.processTradeCalled)
	})
}

func TestTradeHandler_Topic(t *testing.T) {
	handler := &TradeHandler{
		logger: zap.NewNop(),
	}
	assert.Equal(t, "trade-results", handler.Topic())
}

func TestTradeEventParsing(t *testing.T) {
	t.Run("parse valid trade event", func(t *testing.T) {
		jsonStr := `{
			"trade_id": "trade-001",
			"market": "BTC-USDC",
			"price": "50000.00",
			"amount": "1.5",
			"quote_amount": "75000.00",
			"maker_side": 0,
			"maker_order_id": "order-maker-001",
			"taker_order_id": "order-taker-001",
			"timestamp": 1700000000000
		}`

		var trade model.TradeEvent
		err := json.Unmarshal([]byte(jsonStr), &trade)
		require.NoError(t, err)

		assert.Equal(t, "trade-001", trade.TradeID)
		assert.Equal(t, "BTC-USDC", trade.Market)
		assert.Equal(t, "50000.00", trade.Price)
		assert.Equal(t, "1.5", trade.Amount)
		assert.Equal(t, "75000.00", trade.QuoteAmount)
		assert.Equal(t, int8(0), trade.MakerSide)
		assert.Equal(t, "order-maker-001", trade.MakerOrderID)
		assert.Equal(t, "order-taker-001", trade.TakerOrderID)
		assert.Equal(t, int64(1700000000000), trade.Timestamp)
	})

	t.Run("parse sell side", func(t *testing.T) {
		jsonStr := `{"trade_id": "trade-002", "market": "ETH-USDC", "price": "3000.00", "amount": "10", "maker_side": 1, "timestamp": 1700000001000}`

		var trade model.TradeEvent
		err := json.Unmarshal([]byte(jsonStr), &trade)
		require.NoError(t, err)
		assert.Equal(t, int8(1), trade.MakerSide)
	})

	t.Run("parse empty values", func(t *testing.T) {
		jsonStr := `{"trade_id": "trade-003", "market": "NEW-USDC", "timestamp": 1700000002000}`

		var trade model.TradeEvent
		err := json.Unmarshal([]byte(jsonStr), &trade)
		require.NoError(t, err)
		assert.Equal(t, "", trade.Price)
		assert.Equal(t, "", trade.Amount)
	})
}

func BenchmarkTradeEventUnmarshal(b *testing.B) {
	jsonStr := `{
		"trade_id": "trade-001",
		"market": "BTC-USDC",
		"price": "50000.00",
		"amount": "1.5",
		"quote_amount": "75000.00",
		"maker_side": 0,
		"maker_order_id": "order-maker-001",
		"taker_order_id": "order-taker-001",
		"timestamp": 1700000000000
	}`
	data := []byte(jsonStr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var trade model.TradeEvent
		_ = json.Unmarshal(data, &trade)
	}
}
