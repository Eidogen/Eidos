package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPendingTxStatus_String(t *testing.T) {
	tests := []struct {
		status   PendingTxStatus
		expected string
	}{
		{PendingTxStatusPending, "PENDING"},
		{PendingTxStatusConfirmed, "CONFIRMED"},
		{PendingTxStatusFailed, "FAILED"},
		{PendingTxStatusReplaced, "REPLACED"},
		{PendingTxStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestWalletNonce_TableName(t *testing.T) {
	nonce := WalletNonce{}
	assert.Equal(t, "eidos_chain_wallet_nonces", nonce.TableName())
}

func TestPendingTx_TableName(t *testing.T) {
	tx := PendingTx{}
	assert.Equal(t, "eidos_chain_pending_txs", tx.TableName())
}

func TestPendingTxType_Constants(t *testing.T) {
	assert.Equal(t, PendingTxType("settlement"), PendingTxTypeSettlement)
	assert.Equal(t, PendingTxType("withdrawal"), PendingTxTypeWithdrawal)
}
