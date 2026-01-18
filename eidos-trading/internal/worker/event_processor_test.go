package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEventHandler 事件处理器 Mock
type MockEventHandler struct {
	mock.Mock
}

func (m *MockEventHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	args := m.Called(ctx, eventType, payload)
	return args.Error(0)
}

// ========== EventProcessor Tests ==========

func TestNewEventProcessor(t *testing.T) {
	processor := NewEventProcessor()
	assert.NotNil(t, processor)
	assert.NotNil(t, processor.handlers)
	assert.Empty(t, processor.handlers)
}

func TestEventProcessor_RegisterHandler(t *testing.T) {
	processor := NewEventProcessor()
	handler := new(MockEventHandler)

	processor.RegisterHandler("trade-results", handler)

	handlers := processor.Handlers()
	assert.Len(t, handlers, 1)
	assert.Equal(t, handler, handlers["trade-results"])
}

func TestEventProcessor_RegisterMultipleHandlers(t *testing.T) {
	processor := NewEventProcessor()
	handler1 := new(MockEventHandler)
	handler2 := new(MockEventHandler)
	handler3 := new(MockEventHandler)

	processor.RegisterHandler("trade-results", handler1)
	processor.RegisterHandler("order-cancelled", handler2)
	processor.RegisterHandler("deposits", handler3)

	handlers := processor.Handlers()
	assert.Len(t, handlers, 3)
	assert.Equal(t, handler1, handlers["trade-results"])
	assert.Equal(t, handler2, handlers["order-cancelled"])
	assert.Equal(t, handler3, handlers["deposits"])
}

func TestEventProcessor_Handle_Success(t *testing.T) {
	processor := NewEventProcessor()
	handler := new(MockEventHandler)
	ctx := context.Background()

	payload := []byte(`{"trade_id": "T123"}`)
	handler.On("HandleEvent", ctx, "trade-results", payload).Return(nil)

	processor.RegisterHandler("trade-results", handler)

	err := processor.Handle(ctx, &kafka.Message{
		Topic:     "trade-results",
		Partition: 0,
		Offset:    100,
		Value:     payload,
		Timestamp: 1705401600000,
	})

	assert.NoError(t, err)
	handler.AssertExpectations(t)
}

func TestEventProcessor_Handle_NoHandlerRegistered(t *testing.T) {
	processor := NewEventProcessor()
	ctx := context.Background()

	// Should not error, just log warning
	err := processor.Handle(ctx, &kafka.Message{
		Topic:     "unknown-topic",
		Partition: 0,
		Offset:    100,
		Value:     []byte(`{}`),
		Timestamp: 1705401600000,
	})

	assert.NoError(t, err)
}

func TestEventProcessor_Handle_HandlerError(t *testing.T) {
	processor := NewEventProcessor()
	handler := new(MockEventHandler)
	ctx := context.Background()

	payload := []byte(`{"invalid": "data"}`)
	expectedErr := errors.New("handler error")
	handler.On("HandleEvent", ctx, "trade-results", payload).Return(expectedErr)

	processor.RegisterHandler("trade-results", handler)

	err := processor.Handle(ctx, &kafka.Message{
		Topic:     "trade-results",
		Partition: 0,
		Offset:    100,
		Value:     payload,
		Timestamp: 1705401600000,
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	handler.AssertExpectations(t)
}

// ========== Message Parsing Tests ==========

func TestParseTradeResult_Success(t *testing.T) {
	data := []byte(`{
		"trade_id": "T1234567890123456789",
		"market": "ETH-USDC",
		"maker_order_id": "O1234567890123456789",
		"taker_order_id": "O1234567890123456790",
		"maker": "0x1234abcd",
		"taker": "0x5678efgh",
		"side": "buy",
		"price": "3000.50",
		"size": "1.5",
		"quote_amount": "4500.75",
		"maker_fee": "2.25",
		"taker_fee": "4.50",
		"timestamp": 1705401600000,
		"sequence": 12345,
		"maker_is_buyer": true
	}`)

	msg, err := ParseTradeResult(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "T1234567890123456789", msg.TradeID)
	assert.Equal(t, "ETH-USDC", msg.Market)
	assert.Equal(t, "O1234567890123456789", msg.MakerOrderID)
	assert.Equal(t, "O1234567890123456790", msg.TakerOrderID)
	assert.Equal(t, "0x1234abcd", msg.Maker)
	assert.Equal(t, "0x5678efgh", msg.Taker)
	assert.Equal(t, "buy", msg.Side)
	assert.Equal(t, "3000.50", msg.Price)
	assert.Equal(t, "1.5", msg.Size)
	assert.Equal(t, "4500.75", msg.QuoteAmount)
	assert.Equal(t, "2.25", msg.MakerFee)
	assert.Equal(t, "4.50", msg.TakerFee)
	assert.Equal(t, int64(1705401600000), msg.Timestamp)
	assert.Equal(t, int64(12345), msg.Sequence)
	assert.True(t, msg.MakerIsBuyer)
}

func TestParseTradeResult_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseTradeResult(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse trade result failed")
}

func TestParseTradeResult_Empty(t *testing.T) {
	data := []byte(`{}`)

	msg, err := ParseTradeResult(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Empty(t, msg.TradeID)
}

func TestParseOrderCancelled_Success(t *testing.T) {
	data := []byte(`{
		"order_id": "O123456789",
		"market": "ETH-USDC",
		"result": "success",
		"remaining_size": "0.5",
		"filled_size": "1.5",
		"timestamp": 1705401600000,
		"sequence": 12345
	}`)

	msg, err := ParseOrderCancelled(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "O123456789", msg.OrderID)
	assert.Equal(t, "ETH-USDC", msg.Market)
	assert.Equal(t, "success", msg.Result)
	assert.Equal(t, "0.5", msg.RemainingSize)
	assert.Equal(t, "1.5", msg.FilledSize)
	assert.Equal(t, int64(1705401600000), msg.Timestamp)
	assert.Equal(t, int64(12345), msg.Sequence)
}

func TestParseOrderCancelled_DifferentResults(t *testing.T) {
	tests := []struct {
		name           string
		result         string
		expectedResult string
	}{
		{"success", "success", "success"},
		{"not_found", "not_found", "not_found"},
		{"already_cancelled", "already_cancelled", "already_cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(`{"order_id": "O123", "result": "` + tt.result + `"}`)
			msg, err := ParseOrderCancelled(data)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, msg.Result)
		})
	}
}

func TestParseOrderCancelled_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseOrderCancelled(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse order cancelled failed")
}

func TestParseOrderAccepted_Success(t *testing.T) {
	data := []byte(`{
		"order_id": "O123456789",
		"market": "ETH-USDC",
		"timestamp": 1705401600000,
		"sequence": 12345
	}`)

	msg, err := ParseOrderAccepted(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "O123456789", msg.OrderID)
	assert.Equal(t, "ETH-USDC", msg.Market)
	assert.Equal(t, int64(1705401600000), msg.Timestamp)
	assert.Equal(t, int64(12345), msg.Sequence)
}

func TestParseOrderAccepted_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseOrderAccepted(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse order accepted failed")
}

func TestParseSettlementConfirmed_Success(t *testing.T) {
	data := []byte(`{
		"settlement_id": "S1234567890",
		"trade_ids": ["T123", "T124", "T125"],
		"tx_hash": "0xdef456...",
		"block_number": 12345679,
		"status": "confirmed",
		"timestamp": 1705401700000
	}`)

	msg, err := ParseSettlementConfirmed(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "S1234567890", msg.SettlementID)
	assert.Equal(t, []string{"T123", "T124", "T125"}, msg.TradeIDs)
	assert.Equal(t, "0xdef456...", msg.TxHash)
	assert.Equal(t, int64(12345679), msg.BlockNumber)
	assert.Equal(t, "confirmed", msg.Status)
	assert.Equal(t, int64(1705401700000), msg.Timestamp)
}

func TestParseSettlementConfirmed_Failed(t *testing.T) {
	data := []byte(`{
		"settlement_id": "S1234567890",
		"trade_ids": ["T123", "T124"],
		"status": "failed",
		"timestamp": 1705401700000
	}`)

	msg, err := ParseSettlementConfirmed(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "failed", msg.Status)
}

func TestParseSettlementConfirmed_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseSettlementConfirmed(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse settlement confirmed failed")
}

func TestParseDeposit_Success(t *testing.T) {
	data := []byte(`{
		"tx_hash": "0xabc123...",
		"wallet": "0x1234abcd",
		"token": "USDC",
		"amount": "1000.00",
		"block_number": 12345678,
		"timestamp": 1705401600000
	}`)

	msg, err := ParseDeposit(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "0xabc123...", msg.TxHash)
	assert.Equal(t, "0x1234abcd", msg.Wallet)
	assert.Equal(t, "USDC", msg.Token)
	assert.Equal(t, "1000.00", msg.Amount)
	assert.Equal(t, int64(12345678), msg.BlockNumber)
	assert.Equal(t, int64(1705401600000), msg.Timestamp)
}

func TestParseDeposit_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseDeposit(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse deposit failed")
}

func TestParseWithdrawalConfirmed_Success(t *testing.T) {
	data := []byte(`{
		"withdrawal_id": "W123456789",
		"tx_hash": "0xdef456...",
		"block_number": 12345679,
		"status": "confirmed",
		"timestamp": 1705401700000
	}`)

	msg, err := ParseWithdrawalConfirmed(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "W123456789", msg.WithdrawalID)
	assert.Equal(t, "0xdef456...", msg.TxHash)
	assert.Equal(t, int64(12345679), msg.BlockNumber)
	assert.Equal(t, "confirmed", msg.Status)
	assert.Equal(t, int64(1705401700000), msg.Timestamp)
}

func TestParseWithdrawalConfirmed_Failed(t *testing.T) {
	data := []byte(`{
		"withdrawal_id": "W123456789",
		"status": "failed",
		"timestamp": 1705401700000
	}`)

	msg, err := ParseWithdrawalConfirmed(data)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "failed", msg.Status)
}

func TestParseWithdrawalConfirmed_Invalid(t *testing.T) {
	data := []byte(`{invalid json}`)

	msg, err := ParseWithdrawalConfirmed(data)

	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "parse withdrawal confirmed failed")
}

// ========== JSON Serialization Tests ==========

func TestTradeResultMessage_JSONRoundTrip(t *testing.T) {
	original := &TradeResultMessage{
		TradeID:      "T123",
		Market:       "ETH-USDC",
		MakerOrderID: "O1",
		TakerOrderID: "O2",
		Maker:        "0x1111",
		Taker:        "0x2222",
		Side:         "buy",
		Price:        "3000.0",
		Size:         "1.0",
		QuoteAmount:  "3000.0",
		MakerFee:     "1.5",
		TakerFee:     "3.0",
		Timestamp:    1705401600000,
		Sequence:     12345,
		MakerIsBuyer: true,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	parsed, err := ParseTradeResult(data)
	assert.NoError(t, err)
	assert.Equal(t, original.TradeID, parsed.TradeID)
	assert.Equal(t, original.Market, parsed.Market)
	assert.Equal(t, original.MakerIsBuyer, parsed.MakerIsBuyer)
}

func TestOrderCancelledMessage_JSONRoundTrip(t *testing.T) {
	original := &OrderCancelledMessage{
		OrderID:       "O123",
		Market:        "ETH-USDC",
		Result:        "success",
		RemainingSize: "0.5",
		FilledSize:    "1.5",
		Timestamp:     1705401600000,
		Sequence:      12345,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	parsed, err := ParseOrderCancelled(data)
	assert.NoError(t, err)
	assert.Equal(t, original.OrderID, parsed.OrderID)
	assert.Equal(t, original.Result, parsed.Result)
}

func TestSettlementConfirmedMessage_JSONRoundTrip(t *testing.T) {
	original := &SettlementConfirmedMessage{
		SettlementID: "S123",
		TradeIDs:     []string{"T1", "T2", "T3"},
		TxHash:       "0xabc",
		BlockNumber:  12345,
		Status:       "confirmed",
		Timestamp:    1705401600000,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	parsed, err := ParseSettlementConfirmed(data)
	assert.NoError(t, err)
	assert.Equal(t, original.SettlementID, parsed.SettlementID)
	assert.Equal(t, original.TradeIDs, parsed.TradeIDs)
	assert.Equal(t, original.Status, parsed.Status)
}

func TestDepositMessage_JSONRoundTrip(t *testing.T) {
	original := &DepositMessage{
		TxHash:      "0xabc",
		Wallet:      "0x1234",
		Token:       "USDC",
		Amount:      "1000.00",
		BlockNumber: 12345,
		Timestamp:   1705401600000,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	parsed, err := ParseDeposit(data)
	assert.NoError(t, err)
	assert.Equal(t, original.TxHash, parsed.TxHash)
	assert.Equal(t, original.Wallet, parsed.Wallet)
	assert.Equal(t, original.Token, parsed.Token)
}

func TestWithdrawalConfirmedMessage_JSONRoundTrip(t *testing.T) {
	original := &WithdrawalConfirmedMessage{
		WithdrawalID: "W123",
		TxHash:       "0xabc",
		BlockNumber:  12345,
		Status:       "confirmed",
		Timestamp:    1705401600000,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	parsed, err := ParseWithdrawalConfirmed(data)
	assert.NoError(t, err)
	assert.Equal(t, original.WithdrawalID, parsed.WithdrawalID)
	assert.Equal(t, original.Status, parsed.Status)
}
