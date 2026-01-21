package repository

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// TestWithdrawalRepository_Errors 测试错误类型
func TestWithdrawalRepository_Errors(t *testing.T) {
	assert.Equal(t, "withdrawal tx not found", ErrWithdrawalTxNotFound.Error())
}

// TestWithdrawalTxStatus_Values 测试提现状态枚举值
func TestWithdrawalTxStatus_Values(t *testing.T) {
	assert.Equal(t, model.WithdrawalTxStatus(0), model.WithdrawalTxStatusPending)
	assert.Equal(t, model.WithdrawalTxStatus(1), model.WithdrawalTxStatusSubmitted)
	assert.Equal(t, model.WithdrawalTxStatus(2), model.WithdrawalTxStatusConfirmed)
	assert.Equal(t, model.WithdrawalTxStatus(3), model.WithdrawalTxStatusFailed)
}

// TestWithdrawalTxStatus_String 测试状态字符串表示
func TestWithdrawalTxStatus_String(t *testing.T) {
	assert.Equal(t, "PENDING", model.WithdrawalTxStatusPending.String())
	assert.Equal(t, "SUBMITTED", model.WithdrawalTxStatusSubmitted.String())
	assert.Equal(t, "CONFIRMED", model.WithdrawalTxStatusConfirmed.String())
	assert.Equal(t, "FAILED", model.WithdrawalTxStatusFailed.String())
	assert.Equal(t, "UNKNOWN", model.WithdrawalTxStatus(99).String())
}

// TestWithdrawalTx_TableName 测试表名
func TestWithdrawalTx_TableName(t *testing.T) {
	tx := &model.WithdrawalTx{}
	assert.Equal(t, "chain_withdrawal_txs", tx.TableName())
}

// TestWithdrawalTx_Fields 测试 WithdrawalTx 字段
func TestWithdrawalTx_Fields(t *testing.T) {
	tx := &model.WithdrawalTx{
		ID:            1,
		WithdrawID:    "withdraw-123",
		WalletAddress: "0x1234567890123456789012345678901234567890",
		ToAddress:     "0xrecipient",
		Token:         "USDC",
		TokenAddress:  "0xtoken",
		Amount:        decimal.NewFromFloat(1000),
		ChainID:       31337,
		TxHash:        "0xabc123",
		BlockNumber:   12345,
		GasUsed:       200000,
		Status:        model.WithdrawalTxStatusConfirmed,
		ErrorMessage:  "",
		RetryCount:    0,
		SubmittedAt:   1234567890000,
		ConfirmedAt:   1234567900000,
		CreatedAt:     1234567890000,
		UpdatedAt:     1234567900000,
	}

	assert.Equal(t, int64(1), tx.ID)
	assert.Equal(t, "withdraw-123", tx.WithdrawID)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", tx.WalletAddress)
	assert.Equal(t, "0xrecipient", tx.ToAddress)
	assert.Equal(t, "USDC", tx.Token)
	assert.Equal(t, "0xtoken", tx.TokenAddress)
	assert.True(t, tx.Amount.Equal(decimal.NewFromFloat(1000)))
	assert.Equal(t, int64(31337), tx.ChainID)
	assert.Equal(t, "0xabc123", tx.TxHash)
	assert.Equal(t, int64(12345), tx.BlockNumber)
	assert.Equal(t, int64(200000), tx.GasUsed)
	assert.Equal(t, model.WithdrawalTxStatusConfirmed, tx.Status)
}

// TestWithdrawalRequest_Fields 测试 WithdrawalRequest 字段
func TestWithdrawalRequest_Fields(t *testing.T) {
	request := &model.WithdrawalRequest{
		WithdrawID:   "withdraw-123",
		Wallet:       "0xwallet",
		ToAddress:    "0xrecipient",
		Token:        "USDC",
		TokenAddress: "0xtoken",
		Amount:       decimal.NewFromFloat(1000),
		Nonce:        12345,
		Signature:    "0xsig",
		CreatedAt:    1234567890000,
	}

	assert.Equal(t, "withdraw-123", request.WithdrawID)
	assert.Equal(t, "0xwallet", request.Wallet)
	assert.Equal(t, "0xrecipient", request.ToAddress)
	assert.Equal(t, "USDC", request.Token)
	assert.True(t, request.Amount.Equal(decimal.NewFromFloat(1000)))
	assert.Equal(t, int64(12345), request.Nonce)
	assert.Equal(t, "0xsig", request.Signature)
	assert.Equal(t, int64(1234567890000), request.CreatedAt)
}

// TestWithdrawalConfirmation_Fields 测试 WithdrawalConfirmation 字段
func TestWithdrawalConfirmation_Fields(t *testing.T) {
	confirmation := &model.WithdrawalConfirmation{
		WithdrawID:  "withdraw-123",
		TxHash:      "0xabc123",
		BlockNumber: 12345,
		GasUsed:     200000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890000,
	}

	assert.Equal(t, "withdraw-123", confirmation.WithdrawID)
	assert.Equal(t, "0xabc123", confirmation.TxHash)
	assert.Equal(t, int64(12345), confirmation.BlockNumber)
	assert.Equal(t, int64(200000), confirmation.GasUsed)
	assert.Equal(t, "CONFIRMED", confirmation.Status)
}

// TestWithdrawalTx_PendingState 测试待处理状态
func TestWithdrawalTx_PendingState(t *testing.T) {
	tx := &model.WithdrawalTx{
		WithdrawID: "withdraw-pending",
		Status:     model.WithdrawalTxStatusPending,
		TxHash:     "",
	}

	assert.Equal(t, model.WithdrawalTxStatusPending, tx.Status)
	assert.Empty(t, tx.TxHash)
}

// TestWithdrawalTx_FailedState 测试失败状态
func TestWithdrawalTx_FailedState(t *testing.T) {
	tx := &model.WithdrawalTx{
		WithdrawID:   "withdraw-failed",
		Status:       model.WithdrawalTxStatusFailed,
		ErrorMessage: "insufficient gas",
		RetryCount:   3,
	}

	assert.Equal(t, model.WithdrawalTxStatusFailed, tx.Status)
	assert.Equal(t, "insufficient gas", tx.ErrorMessage)
	assert.Equal(t, 3, tx.RetryCount)
}
