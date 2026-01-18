package kafka

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestConsumerConfig 测试消费者配置
func TestConsumerConfig_Defaults(t *testing.T) {
	cfg := &ConsumerConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "test-group",
	}

	assert.Len(t, cfg.Brokers, 1)
	assert.Equal(t, "test-group", cfg.GroupID)
}

// TestSettlementTradeDeserialization 测试结算交易反序列化
func TestSettlementTradeDeserialization(t *testing.T) {
	jsonData := `{
		"trade_id": "trade-123",
		"market": "BTC-USDC",
		"maker_wallet": "0xmaker",
		"taker_wallet": "0xtaker",
		"maker_order_id": "order-1",
		"taker_order_id": "order-2",
		"price": "50000.00",
		"amount": "1.5",
		"quote_amount": "75000.00",
		"maker_fee": "10.00",
		"taker_fee": "15.00",
		"maker_side": 1,
		"matched_at": 1234567890
	}`

	var trade model.SettlementTrade
	err := json.Unmarshal([]byte(jsonData), &trade)

	assert.NoError(t, err)
	assert.Equal(t, "trade-123", trade.TradeID)
	assert.Equal(t, "BTC-USDC", trade.Market)
	assert.Equal(t, "0xmaker", trade.MakerWallet)
	assert.Equal(t, "0xtaker", trade.TakerWallet)
	assert.True(t, trade.Price.Equal(decimal.NewFromFloat(50000.00)))
	assert.True(t, trade.Amount.Equal(decimal.NewFromFloat(1.5)))
	assert.True(t, trade.QuoteAmount.Equal(decimal.NewFromFloat(75000.00)))
	assert.Equal(t, int8(1), trade.MakerSide)
	assert.Equal(t, int64(1234567890), trade.MatchedAt)
}

// TestWithdrawalRequestDeserialization 测试提现请求反序列化
func TestWithdrawalRequestDeserialization(t *testing.T) {
	jsonData := `{
		"withdraw_id": "withdraw-123",
		"wallet": "0xwallet",
		"to_address": "0xrecipient",
		"token": "USDC",
		"token_address": "0xtoken",
		"amount": "1000.00",
		"signature": "0xsig123",
		"created_at": 1234567890
	}`

	var request model.WithdrawalRequest
	err := json.Unmarshal([]byte(jsonData), &request)

	assert.NoError(t, err)
	assert.Equal(t, "withdraw-123", request.WithdrawID)
	assert.Equal(t, "0xwallet", request.Wallet)
	assert.Equal(t, "0xrecipient", request.ToAddress)
	assert.Equal(t, "USDC", request.Token)
	assert.True(t, request.Amount.Equal(decimal.NewFromFloat(1000.00)))
	assert.Equal(t, "0xsig123", request.Signature)
}

// TestSettlementConfirmationSerialization 测试结算确认序列化
func TestSettlementConfirmationSerialization(t *testing.T) {
	confirmation := &model.SettlementConfirmation{
		BatchID:     "batch-123",
		TradeIDs:    []string{"trade-1", "trade-2", "trade-3"},
		TxHash:      "0xabc123def456",
		BlockNumber: 12345678,
		GasUsed:     350000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890000,
	}

	data, err := json.Marshal(confirmation)
	assert.NoError(t, err)

	// 验证可以反序列化回来
	var decoded model.SettlementConfirmation
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, confirmation.BatchID, decoded.BatchID)
	assert.Equal(t, confirmation.TradeIDs, decoded.TradeIDs)
	assert.Equal(t, confirmation.TxHash, decoded.TxHash)
	assert.Equal(t, confirmation.Status, decoded.Status)
}

// TestWithdrawalConfirmationSerialization 测试提现确认序列化
func TestWithdrawalConfirmationSerialization(t *testing.T) {
	confirmation := &model.WithdrawalConfirmation{
		WithdrawID:  "withdraw-123",
		TxHash:      "0xabc123def456",
		BlockNumber: 12345678,
		GasUsed:     200000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890000,
	}

	data, err := json.Marshal(confirmation)
	assert.NoError(t, err)

	// 验证可以反序列化回来
	var decoded model.WithdrawalConfirmation
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, confirmation.WithdrawID, decoded.WithdrawID)
	assert.Equal(t, confirmation.TxHash, decoded.TxHash)
	assert.Equal(t, confirmation.Status, decoded.Status)
}

// TestDepositEventSerialization 测试充值事件序列化
func TestDepositEventSerialization(t *testing.T) {
	deposit := &model.DepositEvent{
		DepositID:    "dep-123",
		Wallet:       "0x1234567890123456789012345678901234567890",
		Token:        "USDC",
		TokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		Amount:       decimal.NewFromFloat(1000.50),
		TxHash:       "0xabc123def456789",
		BlockNumber:  12345678,
		LogIndex:     0,
		DetectedAt:   1234567890000,
	}

	data, err := json.Marshal(deposit)
	assert.NoError(t, err)

	// 验证可以反序列化回来
	var decoded model.DepositEvent
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, deposit.DepositID, decoded.DepositID)
	assert.Equal(t, deposit.Wallet, decoded.Wallet)
	assert.Equal(t, deposit.Token, decoded.Token)
	assert.True(t, deposit.Amount.Equal(decoded.Amount))
}

// TestTopicConstants 测试 Topic 常量定义
func TestTopicConstants(t *testing.T) {
	// Consumer topics
	assert.Equal(t, "settlements", TopicSettlements)
	assert.Equal(t, "withdrawals", TopicWithdrawals)

	// Producer topics
	assert.Equal(t, "deposits", TopicDeposits)
	assert.Equal(t, "settlement-confirmed", TopicSettlementConfirmed)
	assert.Equal(t, "withdrawal-confirmed", TopicWithdrawalConfirmed)
}
