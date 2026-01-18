package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSettlementBatchStatus_String(t *testing.T) {
	tests := []struct {
		status   SettlementBatchStatus
		expected string
	}{
		{SettlementBatchStatusPending, "PENDING"},
		{SettlementBatchStatusSubmitted, "SUBMITTED"},
		{SettlementBatchStatusConfirmed, "CONFIRMED"},
		{SettlementBatchStatusFailed, "FAILED"},
		{SettlementBatchStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestSettlementBatchStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     SettlementBatchStatus
		isTerminal bool
	}{
		{SettlementBatchStatusPending, false},
		{SettlementBatchStatusSubmitted, false},
		{SettlementBatchStatusConfirmed, true},
		{SettlementBatchStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			assert.Equal(t, tt.isTerminal, tt.status.IsTerminal())
		})
	}
}

func TestSettlementBatch_TableName(t *testing.T) {
	batch := SettlementBatch{}
	assert.Equal(t, "eidos_chain_settlement_batches", batch.TableName())
}

func TestSettlementBatch_GetSetTradeIDList(t *testing.T) {
	batch := &SettlementBatch{}

	// Test setting trade IDs
	tradeIDs := []string{"trade-1", "trade-2", "trade-3"}
	err := batch.SetTradeIDList(tradeIDs)
	assert.NoError(t, err)
	assert.Equal(t, 3, batch.TradeCount)
	assert.Equal(t, `["trade-1","trade-2","trade-3"]`, batch.TradeIDs)

	// Test getting trade IDs
	got, err := batch.GetTradeIDList()
	assert.NoError(t, err)
	assert.Equal(t, tradeIDs, got)
}

func TestSettlementBatch_GetTradeIDList_Empty(t *testing.T) {
	batch := &SettlementBatch{}

	err := batch.SetTradeIDList([]string{})
	assert.NoError(t, err)
	assert.Equal(t, 0, batch.TradeCount)

	got, err := batch.GetTradeIDList()
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestSettlementBatch_GetTradeIDList_InvalidJSON(t *testing.T) {
	batch := &SettlementBatch{
		TradeIDs: "invalid json",
	}

	_, err := batch.GetTradeIDList()
	assert.Error(t, err)
}

func TestSettlementRollbackLog_TableName(t *testing.T) {
	log := SettlementRollbackLog{}
	assert.Equal(t, "eidos_chain_settlement_rollback_logs", log.TableName())
}
