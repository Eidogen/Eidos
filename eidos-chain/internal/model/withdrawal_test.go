package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithdrawalTxStatus_String(t *testing.T) {
	tests := []struct {
		status   WithdrawalTxStatus
		expected string
	}{
		{WithdrawalTxStatusPending, "PENDING"},
		{WithdrawalTxStatusSubmitted, "SUBMITTED"},
		{WithdrawalTxStatusConfirmed, "CONFIRMED"},
		{WithdrawalTxStatusFailed, "FAILED"},
		{WithdrawalTxStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestWithdrawalTxStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     WithdrawalTxStatus
		isTerminal bool
	}{
		{WithdrawalTxStatusPending, false},
		{WithdrawalTxStatusSubmitted, false},
		{WithdrawalTxStatusConfirmed, true},
		{WithdrawalTxStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			assert.Equal(t, tt.isTerminal, tt.status.IsTerminal())
		})
	}
}

func TestWithdrawalTx_TableName(t *testing.T) {
	tx := WithdrawalTx{}
	assert.Equal(t, "chain_withdrawal_txs", tx.TableName())
}
