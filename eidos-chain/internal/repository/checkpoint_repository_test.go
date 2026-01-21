package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestCheckpointRepository_Errors 测试错误类型
func TestCheckpointRepository_Errors(t *testing.T) {
	assert.Equal(t, "checkpoint not found", ErrCheckpointNotFound.Error())
	assert.Equal(t, "event not found", ErrEventNotFound.Error())
}

// TestChainEventType_Values 测试事件类型值
func TestChainEventType_Values(t *testing.T) {
	assert.Equal(t, model.ChainEventType("Deposit"), model.ChainEventTypeDeposit)
	assert.Equal(t, model.ChainEventType("Settlement"), model.ChainEventTypeSettlement)
	assert.Equal(t, model.ChainEventType("Withdraw"), model.ChainEventTypeWithdraw)
}

// TestBlockCheckpoint_TableName 测试表名
func TestBlockCheckpoint_TableName(t *testing.T) {
	checkpoint := &model.BlockCheckpoint{}
	assert.Equal(t, "chain_block_checkpoints", checkpoint.TableName())
}

// TestChainEvent_TableName 测试表名
func TestChainEvent_TableName(t *testing.T) {
	event := &model.ChainEvent{}
	assert.Equal(t, "chain_events", event.TableName())
}

// TestBlockCheckpoint_Fields 测试 BlockCheckpoint 字段
func TestBlockCheckpoint_Fields(t *testing.T) {
	checkpoint := &model.BlockCheckpoint{
		ID:          1,
		ChainID:     31337,
		BlockNumber: 12345,
		BlockHash:   "0xabc123",
		ProcessedAt: 1234567890000,
		CreatedAt:   1234567890000,
		UpdatedAt:   1234567890000,
	}

	assert.Equal(t, int64(1), checkpoint.ID)
	assert.Equal(t, int64(31337), checkpoint.ChainID)
	assert.Equal(t, int64(12345), checkpoint.BlockNumber)
	assert.Equal(t, "0xabc123", checkpoint.BlockHash)
}

// TestChainEvent_Fields 测试 ChainEvent 字段
func TestChainEvent_Fields(t *testing.T) {
	event := &model.ChainEvent{
		ID:          1,
		ChainID:     31337,
		BlockNumber: 12345,
		TxHash:      "0xabc123",
		LogIndex:    0,
		EventType:   model.ChainEventTypeDeposit,
		EventData:   `{"amount": "1000"}`,
		Processed:   false,
		CreatedAt:   1234567890000,
		UpdatedAt:   1234567890000,
	}

	assert.Equal(t, int64(1), event.ID)
	assert.Equal(t, int64(31337), event.ChainID)
	assert.Equal(t, int64(12345), event.BlockNumber)
	assert.Equal(t, "0xabc123", event.TxHash)
	assert.Equal(t, 0, event.LogIndex)
	assert.Equal(t, model.ChainEventTypeDeposit, event.EventType)
	assert.Equal(t, `{"amount": "1000"}`, event.EventData)
	assert.False(t, event.Processed)
}
