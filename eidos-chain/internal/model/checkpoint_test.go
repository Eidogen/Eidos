package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockCheckpoint_TableName(t *testing.T) {
	checkpoint := BlockCheckpoint{}
	assert.Equal(t, "chain_block_checkpoints", checkpoint.TableName())
}

func TestChainEventType_Constants(t *testing.T) {
	// 注意：常量值是首字母大写的字符串
	assert.Equal(t, ChainEventType("Deposit"), ChainEventTypeDeposit)
	assert.Equal(t, ChainEventType("Settlement"), ChainEventTypeSettlement)
	assert.Equal(t, ChainEventType("Withdraw"), ChainEventTypeWithdraw)
}

func TestChainEvent_TableName(t *testing.T) {
	event := ChainEvent{}
	assert.Equal(t, "chain_events", event.TableName())
}
