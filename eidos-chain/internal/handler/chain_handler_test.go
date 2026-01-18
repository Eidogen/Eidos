package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// ========================================
// Response Structure Tests
// ========================================

// TestResponseStructs 测试响应结构体序列化
func TestResponseStructs(t *testing.T) {
	t.Run("SettlementBatchResponse", func(t *testing.T) {
		resp := &SettlementBatchResponse{
			BatchID:     "batch-123",
			TradeCount:  5,
			TradeIDs:    []string{"t1", "t2"},
			ChainID:     31337,
			TxHash:      "0xabc",
			BlockNumber: 12345,
			GasUsed:     100000,
			GasPrice:    "20000000000",
			Status:      1,
		}

		assert.Equal(t, "batch-123", resp.BatchID)
		assert.Equal(t, 5, resp.TradeCount)
		assert.Len(t, resp.TradeIDs, 2)
		assert.Equal(t, int64(31337), resp.ChainID)
		assert.Equal(t, "0xabc", resp.TxHash)

		// 测试 JSON 序列化
		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "batch-123")
	})

	t.Run("DepositRecordResponse", func(t *testing.T) {
		resp := &DepositRecordResponse{
			DepositID:     "dep-123",
			WalletAddress: "0x1234",
			Token:         "USDC",
			Amount:        "1000.00",
			Confirmations: 12,
			Status:        1,
		}

		assert.Equal(t, "dep-123", resp.DepositID)
		assert.Equal(t, "USDC", resp.Token)
		assert.Equal(t, 12, resp.Confirmations)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "dep-123")
	})

	t.Run("WithdrawalTxResponse", func(t *testing.T) {
		resp := &WithdrawalTxResponse{
			WithdrawID:    "w-123",
			WalletAddress: "0x1234",
			ToAddress:     "0x5678",
			Token:         "USDC",
			Amount:        "500.00",
			Status:        2,
		}

		assert.Equal(t, "w-123", resp.WithdrawID)
		assert.Equal(t, "0x5678", resp.ToAddress)
		assert.Equal(t, int32(2), resp.Status)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "w-123")
	})

	t.Run("IndexerStatusResponse", func(t *testing.T) {
		resp := &IndexerStatusResponse{
			ChainID:         31337,
			Running:         true,
			CurrentBlock:    12345,
			LatestBlock:     12350,
			LagBlocks:       5,
			CheckpointBlock: 12340,
		}

		assert.Equal(t, int64(31337), resp.ChainID)
		assert.True(t, resp.Running)
		assert.Equal(t, int64(5), resp.LagBlocks)
		assert.Equal(t, uint64(12345), resp.CurrentBlock)
		assert.Equal(t, uint64(12350), resp.LatestBlock)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "31337")
	})

	t.Run("ReconciliationRecordResponse", func(t *testing.T) {
		resp := &ReconciliationRecordResponse{
			ID:             1,
			WalletAddress:  "0x1234",
			Token:          "USDC",
			OnChainBalance: "1000.00",
			Difference:     "0.00",
			Status:         "OK",
		}

		assert.Equal(t, int64(1), resp.ID)
		assert.Equal(t, "OK", resp.Status)
		assert.Equal(t, "USDC", resp.Token)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "USDC")
	})

	t.Run("WalletBalanceResponse", func(t *testing.T) {
		resp := &WalletBalanceResponse{
			WalletAddress: "0x1234",
			Balance:       "1000000000000000000",
			Token:         "ETH",
		}

		assert.Equal(t, "ETH", resp.Token)
		assert.Equal(t, "0x1234", resp.WalletAddress)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "ETH")
	})

	t.Run("WalletNonceResponse", func(t *testing.T) {
		resp := &WalletNonceResponse{
			WalletAddress: "0x1234",
			CurrentNonce:  42,
			PendingCount:  3,
		}

		assert.Equal(t, uint64(42), resp.CurrentNonce)
		assert.Equal(t, 3, resp.PendingCount)

		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "42")
	})
}

// TestConvertBatchToJSON 测试批次转换为 JSON
func TestConvertBatchToJSON(t *testing.T) {
	batch := &model.SettlementBatch{
		BatchID:    "batch-123",
		TradeCount: 3,
		TradeIDs:   `["trade-1","trade-2","trade-3"]`,
		ChainID:    31337,
		TxHash:     "0xabc123",
		Status:     model.SettlementBatchStatusConfirmed,
	}

	data, err := convertBatchToJSON(batch)
	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), "batch-123")
	assert.Contains(t, string(data), "31337")
}

// TestModelStatusConstants 测试模型状态常量
func TestModelStatusConstants(t *testing.T) {
	// SettlementBatchStatus
	assert.Equal(t, model.SettlementBatchStatus(0), model.SettlementBatchStatusPending)
	assert.Equal(t, model.SettlementBatchStatus(1), model.SettlementBatchStatusSubmitted)
	assert.Equal(t, model.SettlementBatchStatus(2), model.SettlementBatchStatusConfirmed)
	assert.Equal(t, model.SettlementBatchStatus(3), model.SettlementBatchStatusFailed)

	// WithdrawalTxStatus
	assert.Equal(t, model.WithdrawalTxStatus(0), model.WithdrawalTxStatusPending)
	assert.Equal(t, model.WithdrawalTxStatus(1), model.WithdrawalTxStatusSubmitted)
	assert.Equal(t, model.WithdrawalTxStatus(2), model.WithdrawalTxStatusConfirmed)
	assert.Equal(t, model.WithdrawalTxStatus(3), model.WithdrawalTxStatusFailed)

	// DepositRecordStatus
	assert.Equal(t, model.DepositRecordStatus(0), model.DepositRecordStatusPending)
	assert.Equal(t, model.DepositRecordStatus(1), model.DepositRecordStatusConfirmed)
	assert.Equal(t, model.DepositRecordStatus(2), model.DepositRecordStatusCredited)
}

