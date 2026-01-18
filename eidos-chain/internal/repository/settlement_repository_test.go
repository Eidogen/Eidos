package repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
)

// setupMockDB 创建模拟数据库
func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return gormDB, mock, cleanup
}

// TestSettlementRepository_Errors 测试错误类型
func TestSettlementRepository_Errors(t *testing.T) {
	assert.Equal(t, "settlement batch not found", ErrSettlementBatchNotFound.Error())
	assert.Equal(t, "rollback log not found", ErrRollbackLogNotFound.Error())
}

// TestSettlementBatchStatus_Values 测试结算批次状态枚举值
func TestSettlementBatchStatus_Values(t *testing.T) {
	assert.Equal(t, model.SettlementBatchStatus(0), model.SettlementBatchStatusPending)
	assert.Equal(t, model.SettlementBatchStatus(1), model.SettlementBatchStatusSubmitted)
	assert.Equal(t, model.SettlementBatchStatus(2), model.SettlementBatchStatusConfirmed)
	assert.Equal(t, model.SettlementBatchStatus(3), model.SettlementBatchStatusFailed)
}

// TestSettlementBatchStatus_String 测试状态字符串表示
func TestSettlementBatchStatus_String(t *testing.T) {
	assert.Equal(t, "PENDING", model.SettlementBatchStatusPending.String())
	assert.Equal(t, "SUBMITTED", model.SettlementBatchStatusSubmitted.String())
	assert.Equal(t, "CONFIRMED", model.SettlementBatchStatusConfirmed.String())
	assert.Equal(t, "FAILED", model.SettlementBatchStatusFailed.String())
	assert.Equal(t, "UNKNOWN", model.SettlementBatchStatus(99).String())
}

// TestSettlementBatch_TableName 测试表名
func TestSettlementBatch_TableName(t *testing.T) {
	batch := &model.SettlementBatch{}
	assert.Equal(t, "eidos_chain_settlement_batches", batch.TableName())
}

// TestSettlementRollbackLog_TableName 测试表名
func TestSettlementRollbackLog_TableName(t *testing.T) {
	log := &model.SettlementRollbackLog{}
	assert.Equal(t, "eidos_chain_settlement_rollback_logs", log.TableName())
}

// TestSettlementBatch_Fields 测试 SettlementBatch 字段
func TestSettlementBatch_Fields(t *testing.T) {
	batch := &model.SettlementBatch{
		ID:           1,
		BatchID:      "batch-123",
		TradeCount:   5,
		TradeIDs:     `["trade-1","trade-2","trade-3","trade-4","trade-5"]`,
		ChainID:      31337,
		TxHash:       "0xabc123",
		BlockNumber:  12345,
		GasUsed:      300000,
		GasPrice:     "1000000000",
		Status:       model.SettlementBatchStatusConfirmed,
		ErrorMessage: "",
		RetryCount:   0,
		SubmittedAt:  1234567890000,
		ConfirmedAt:  1234567900000,
		CreatedAt:    1234567890000,
		UpdatedAt:    1234567900000,
	}

	assert.Equal(t, int64(1), batch.ID)
	assert.Equal(t, "batch-123", batch.BatchID)
	assert.Equal(t, 5, batch.TradeCount)
	assert.Equal(t, `["trade-1","trade-2","trade-3","trade-4","trade-5"]`, batch.TradeIDs)
	assert.Equal(t, int64(31337), batch.ChainID)
	assert.Equal(t, "0xabc123", batch.TxHash)
	assert.Equal(t, int64(12345), batch.BlockNumber)
	assert.Equal(t, int64(300000), batch.GasUsed)
	assert.Equal(t, "1000000000", batch.GasPrice)
	assert.Equal(t, model.SettlementBatchStatusConfirmed, batch.Status)
}

// TestSettlementRollbackLog_Fields 测试 SettlementRollbackLog 字段
func TestSettlementRollbackLog_Fields(t *testing.T) {
	log := &model.SettlementRollbackLog{
		ID:             1,
		BatchID:        "batch-123",
		TradeCount:     5,
		AffectedUsers:  2,
		FailureReason:  "gas limit exceeded",
		RollbackReason: "manual rollback",
		Operator:       "0xabc",
		ApprovedBy:     "0xdef",
		RollbackAt:     1234567890000,
		CreatedAt:      1234567890000,
	}

	assert.Equal(t, int64(1), log.ID)
	assert.Equal(t, "batch-123", log.BatchID)
	assert.Equal(t, 5, log.TradeCount)
	assert.Equal(t, 2, log.AffectedUsers)
	assert.Equal(t, "gas limit exceeded", log.FailureReason)
	assert.Equal(t, "manual rollback", log.RollbackReason)
	assert.Equal(t, "0xabc", log.Operator)
	assert.Equal(t, "0xdef", log.ApprovedBy)
}

// TestSettlementTrade_Fields 测试 SettlementTrade 字段
func TestSettlementTrade_Fields(t *testing.T) {
	// SettlementTrade is used in Kafka messages, not stored in DB
	// Test that the struct can be created with expected fields
	trade := &model.SettlementTrade{
		TradeID:      "trade-123",
		Market:       "BTC-USDC",
		MakerWallet:  "0xmaker",
		TakerWallet:  "0xtaker",
		MakerOrderID: "order-maker-1",
		TakerOrderID: "order-taker-1",
		MakerSide:    1, // 1=buy
		MatchedAt:    1234567890000,
	}

	assert.Equal(t, "trade-123", trade.TradeID)
	assert.Equal(t, "BTC-USDC", trade.Market)
	assert.Equal(t, "0xmaker", trade.MakerWallet)
	assert.Equal(t, "0xtaker", trade.TakerWallet)
	assert.Equal(t, "order-maker-1", trade.MakerOrderID)
	assert.Equal(t, "order-taker-1", trade.TakerOrderID)
	assert.Equal(t, int8(1), trade.MakerSide)
}

// TestSettlementConfirmation_Fields 测试 SettlementConfirmation 字段
func TestSettlementConfirmation_Fields(t *testing.T) {
	confirmation := &model.SettlementConfirmation{
		BatchID:     "batch-123",
		TradeIDs:    []string{"trade-1", "trade-2"},
		TxHash:      "0xabc123",
		BlockNumber: 12345,
		GasUsed:     300000,
		Status:      "CONFIRMED",
		ConfirmedAt: 1234567890000,
	}

	assert.Equal(t, "batch-123", confirmation.BatchID)
	assert.Len(t, confirmation.TradeIDs, 2)
	assert.Equal(t, "0xabc123", confirmation.TxHash)
	assert.Equal(t, "CONFIRMED", confirmation.Status)
}
