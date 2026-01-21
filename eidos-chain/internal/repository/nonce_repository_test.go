package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestNonceRepository_Errors 测试错误类型
func TestNonceRepository_Errors(t *testing.T) {
	assert.Equal(t, "wallet nonce not found", ErrWalletNonceNotFound.Error())
	assert.Equal(t, "pending tx not found", ErrPendingTxNotFound.Error())
}

// TestWalletNonce_TableName 测试表名
func TestWalletNonce_TableName(t *testing.T) {
	nonce := &model.WalletNonce{}
	assert.Equal(t, "chain_wallet_nonces", nonce.TableName())
}

// TestPendingTx_TableName 测试表名
func TestPendingTx_TableName(t *testing.T) {
	tx := &model.PendingTx{}
	assert.Equal(t, "chain_pending_txs", tx.TableName())
}

// TestWalletNonce_Fields 测试 WalletNonce 字段
func TestWalletNonce_Fields(t *testing.T) {
	nonce := &model.WalletNonce{
		ID:            1,
		WalletAddress: "0x1234567890123456789012345678901234567890",
		ChainID:       31337,
		CurrentNonce:  10,
		SyncedAt:      1234567890000,
		CreatedAt:     1234567890000,
		UpdatedAt:     1234567890000,
	}

	assert.Equal(t, int64(1), nonce.ID)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", nonce.WalletAddress)
	assert.Equal(t, int64(31337), nonce.ChainID)
	assert.Equal(t, int64(10), nonce.CurrentNonce)
}

// TestPendingTx_Fields 测试 PendingTx 字段
func TestPendingTx_Fields(t *testing.T) {
	tx := &model.PendingTx{
		ID:            1,
		TxHash:        "0xabc123",
		TxType:        model.PendingTxTypeSettlement,
		RefID:         "batch-123",
		WalletAddress: "0x123",
		ChainID:       31337,
		Nonce:         10,
		GasPrice:      "1000000000",
		GasLimit:      300000,
		SubmittedAt:   1234567890000,
		TimeoutAt:     1234567890000 + 600000,
		Status:        model.PendingTxStatusPending,
		CreatedAt:     1234567890000,
		UpdatedAt:     1234567890000,
	}

	assert.Equal(t, int64(1), tx.ID)
	assert.Equal(t, "0xabc123", tx.TxHash)
	assert.Equal(t, model.PendingTxTypeSettlement, tx.TxType)
	assert.Equal(t, "batch-123", tx.RefID)
	assert.Equal(t, "0x123", tx.WalletAddress)
	assert.Equal(t, int64(31337), tx.ChainID)
	assert.Equal(t, int64(10), tx.Nonce)
	assert.Equal(t, "1000000000", tx.GasPrice)
	assert.Equal(t, int64(300000), tx.GasLimit)
	assert.Equal(t, model.PendingTxStatusPending, tx.Status)
}

// TestPendingTxType_Values 测试交易类型枚举值
func TestPendingTxType_Values(t *testing.T) {
	assert.Equal(t, model.PendingTxType("settlement"), model.PendingTxTypeSettlement)
	assert.Equal(t, model.PendingTxType("withdrawal"), model.PendingTxTypeWithdrawal)
}

// TestPendingTxStatus_Values 测试交易状态枚举值
func TestPendingTxStatus_Values(t *testing.T) {
	assert.Equal(t, model.PendingTxStatus(0), model.PendingTxStatusPending)
	assert.Equal(t, model.PendingTxStatus(1), model.PendingTxStatusConfirmed)
	assert.Equal(t, model.PendingTxStatus(2), model.PendingTxStatusFailed)
	assert.Equal(t, model.PendingTxStatus(3), model.PendingTxStatusReplaced)
}

// TestPendingTxStatus_String 测试状态字符串表示
func TestPendingTxStatus_String(t *testing.T) {
	assert.Equal(t, "PENDING", model.PendingTxStatusPending.String())
	assert.Equal(t, "CONFIRMED", model.PendingTxStatusConfirmed.String())
	assert.Equal(t, "FAILED", model.PendingTxStatusFailed.String())
	assert.Equal(t, "REPLACED", model.PendingTxStatusReplaced.String())
	assert.Equal(t, "UNKNOWN", model.PendingTxStatus(99).String())
}
