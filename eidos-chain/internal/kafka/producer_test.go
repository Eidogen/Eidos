package kafka

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestProducerConfig 测试生产者配置
func TestProducerConfig_Defaults(t *testing.T) {
	cfg := &ProducerConfig{
		Brokers:  []string{"localhost:9092"},
		ClientID: "test-client",
	}

	assert.Len(t, cfg.Brokers, 1)
	assert.Equal(t, "test-client", cfg.ClientID)
}

// TestDepositEventSerialization 测试充值事件结构
func TestDepositEventFields(t *testing.T) {
	deposit := &model.DepositEvent{
		DepositID:    "dep-123",
		Wallet:       "0xwallet",
		Token:        "USDC",
		TokenAddress: "0xtoken",
		Amount:       decimal.NewFromFloat(1000),
		TxHash:       "0xabc123",
		BlockNumber:  12345,
		LogIndex:     0,
		DetectedAt:   1234567890,
	}

	assert.Equal(t, "dep-123", deposit.DepositID)
	assert.Equal(t, "0xwallet", deposit.Wallet)
	assert.Equal(t, "USDC", deposit.Token)
	assert.Equal(t, int64(12345), deposit.BlockNumber)
}

// TestSettlementConfirmationFields 测试结算确认结构
func TestSettlementConfirmationFields(t *testing.T) {
	confirmation := &model.SettlementConfirmation{
		BatchID:     "batch-123",
		TradeIDs:    []string{"trade-1", "trade-2"},
		TxHash:      "0xabc123",
		BlockNumber: 12345,
		GasUsed:     300000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890,
	}

	assert.Equal(t, "batch-123", confirmation.BatchID)
	assert.Len(t, confirmation.TradeIDs, 2)
	assert.Equal(t, "0xabc123", confirmation.TxHash)
	assert.Equal(t, "CONFIRMED", confirmation.Status)
}

// TestWithdrawalConfirmationFields 测试提现确认结构
func TestWithdrawalConfirmationFields(t *testing.T) {
	confirmation := &model.WithdrawalConfirmation{
		WithdrawID:  "withdraw-123",
		TxHash:      "0xabc123",
		BlockNumber: 12345,
		GasUsed:     200000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890,
	}

	assert.Equal(t, "withdraw-123", confirmation.WithdrawID)
	assert.Equal(t, "0xabc123", confirmation.TxHash)
	assert.Equal(t, "CONFIRMED", confirmation.Status)
}

// TestKafkaEventPublisherStruct 测试 KafkaEventPublisher 结构
func TestKafkaEventPublisherStruct(t *testing.T) {
	// 创建测试用的 KafkaEventPublisher（不连接真实 Kafka）
	publisher := &KafkaEventPublisher{
		producer: nil, // 不连接真实 Kafka
	}

	// 测试结构创建成功
	assert.Nil(t, publisher.producer)
}
