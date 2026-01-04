package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// tradeColumns 返回 trades 表的所有列名
func tradeColumns() []string {
	return []string{
		"id", "trade_id", "market", "maker_order_id", "taker_order_id",
		"maker_wallet", "taker_wallet", "price", "amount", "quote_amount",
		"maker_side", "maker_fee", "taker_fee", "fee_token", "settlement_status",
		"batch_id", "tx_hash", "matched_at", "settled_at", "created_at", "updated_at",
	}
}

// batchColumns 返回 settlement_batches 表的所有列名
func batchColumns() []string {
	return []string{
		"id", "batch_id", "market", "trade_count", "total_amount", "total_quote",
		"status", "tx_hash", "created_at", "updated_at",
	}
}

func TestTradeRepository_Create_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()

	trade := &model.Trade{
		TradeID:      "T123456789",
		Market:       "ETH-USDT",
		MakerOrderID: "O1",
		TakerOrderID: "O2",
		MakerWallet:  "0x1111111111111111111111111111111111111111",
		TakerWallet:  "0x2222222222222222222222222222222222222222",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "trades"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.Create(ctx, trade)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetByTradeID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T123456789"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		1, tradeID, "ETH-USDT", "O1", "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000.000000000000000000", "1.000000000000000000", "2000.000000000000000000",
		1, "2.000000000000000000", "4.000000000000000000", "USDT",
		model.SettlementStatusMatchedOffchain, "", "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE trade_id = \$1 ORDER BY "trades"\."id" LIMIT \$2`).
		WithArgs(tradeID, 1).
		WillReturnRows(rows)

	trade, err := repo.GetByTradeID(ctx, tradeID)

	assert.NoError(t, err)
	assert.NotNil(t, trade)
	assert.Equal(t, tradeID, trade.TradeID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetByTradeID_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T999999999"

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE trade_id = \$1 ORDER BY "trades"\."id" LIMIT \$2`).
		WithArgs(tradeID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	trade, err := repo.GetByTradeID(ctx, tradeID)

	assert.Nil(t, trade)
	assert.ErrorIs(t, err, ErrTradeNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_ListByOrderID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	orderID := "O1"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		1, "T1", "ETH-USDT", orderID, "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000", "1", "2000", 1, "2", "4", "USDT",
		model.SettlementStatusMatchedOffchain, "", "", now, nil, now, now,
	).AddRow(
		2, "T2", "ETH-USDT", "O3", orderID,
		"0x3333333333333333333333333333333333333333",
		"0x1111111111111111111111111111111111111111",
		"2100", "0.5", "1050", 2, "1", "2", "USDT",
		model.SettlementStatusMatchedOffchain, "", "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE maker_order_id = \$1 OR taker_order_id = \$2 ORDER BY matched_at ASC`).
		WithArgs(orderID, orderID).
		WillReturnRows(rows)

	trades, err := repo.ListByOrderID(ctx, orderID)

	assert.NoError(t, err)
	assert.Len(t, trades, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_ListBySettlementStatus_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	status := model.SettlementStatusMatchedOffchain
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		1, "T1", "ETH-USDT", "O1", "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000", "1", "2000", 1, "2", "4", "USDT",
		status, "", "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE settlement_status = \$1 ORDER BY matched_at ASC LIMIT \$2`).
		WithArgs(status, 100).
		WillReturnRows(rows)

	trades, err := repo.ListBySettlementStatus(ctx, status, 100)

	assert.NoError(t, err)
	assert.Len(t, trades, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_UpdateSettlementStatus_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trades" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpdateSettlementStatus(ctx, tradeID,
		model.SettlementStatusMatchedOffchain,
		model.SettlementStatusPending,
		"BATCH001")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_UpdateSettlementStatus_OptimisticLock(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trades" SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.UpdateSettlementStatus(ctx, tradeID,
		model.SettlementStatusMatchedOffchain,
		model.SettlementStatusPending,
		"")

	assert.ErrorIs(t, err, ErrOptimisticLock)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_MarkSettled_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T123456789"
	txHash := "0xabcdef1234567890"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trades" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkSettled(ctx, tradeID, txHash)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_MarkSettled_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	tradeID := "T999999999"
	txHash := "0xabcdef1234567890"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trades" SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.MarkSettled(ctx, tradeID, txHash)

	assert.ErrorIs(t, err, ErrTradeNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetRecentTrades_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	market := "ETH-USDT"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		1, "T1", market, "O1", "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000", "1", "2000", 1, "2", "4", "USDT",
		model.SettlementStatusMatchedOffchain, "", "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE market = \$1 ORDER BY matched_at DESC LIMIT \$2`).
		WithArgs(market, 10).
		WillReturnRows(rows)

	trades, err := repo.GetRecentTrades(ctx, market, 10)

	assert.NoError(t, err)
	assert.Len(t, trades, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_CountByMarket_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	market := "ETH-USDT"

	mock.ExpectQuery(`SELECT count\(\*\) FROM "trades" WHERE market = \$1`).
		WithArgs(market).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(100))

	count, err := repo.CountByMarket(ctx, market, nil)

	assert.NoError(t, err)
	assert.Equal(t, int64(100), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetBatch_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	batchID := "BATCH001"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(batchColumns()).AddRow(
		1, batchID, "ETH-USDT", 10, "100.000000000000000000", "200000.000000000000000000",
		model.SettlementStatusPending, "", now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "settlement_batches" WHERE batch_id = \$1 ORDER BY "settlement_batches"\."id" LIMIT \$2`).
		WithArgs(batchID, 1).
		WillReturnRows(rows)

	batch, err := repo.GetBatch(ctx, batchID)

	assert.NoError(t, err)
	assert.NotNil(t, batch)
	assert.Equal(t, batchID, batch.BatchID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetBatch_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	batchID := "BATCH999"

	mock.ExpectQuery(`SELECT \* FROM "settlement_batches" WHERE batch_id = \$1 ORDER BY "settlement_batches"\."id" LIMIT \$2`).
		WithArgs(batchID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	batch, err := repo.GetBatch(ctx, batchID)

	assert.Nil(t, batch)
	assert.ErrorIs(t, err, ErrBatchNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_CreateBatch_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()

	batch := &model.SettlementBatch{
		BatchID:    "BATCH001",
		TradeCount: 10,
		Status:     model.SettlementStatusPending,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "settlement_batches"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.CreateBatch(ctx, batch)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_ListPendingBatches_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(batchColumns()).AddRow(
		1, "BATCH001", "ETH-USDT", 10, "100", "200000",
		model.SettlementStatusPending, "", now, now,
	).AddRow(
		2, "BATCH002", "BTC-USDT", 5, "0.5", "50000",
		model.SettlementStatusSubmitted, "0xabc", now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "settlement_batches" WHERE status IN \(\$1,\$2\) ORDER BY created_at ASC LIMIT \$3`).
		WithArgs(model.SettlementStatusPending, model.SettlementStatusSubmitted, 100).
		WillReturnRows(rows)

	batches, err := repo.ListPendingBatches(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, batches, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_GetByID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	id := int64(1)
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		id, "T123", "ETH-USDT", "O1", "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000", "1", "2000", 1, "2", "4", "USDT",
		model.SettlementStatusMatchedOffchain, "", "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE id = \$1 ORDER BY "trades"\."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnRows(rows)

	trade, err := repo.GetByID(ctx, id)

	assert.NoError(t, err)
	assert.NotNil(t, trade)
	assert.Equal(t, id, trade.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTradeRepository_ListByBatchID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewTradeRepository(db)
	ctx := context.Background()
	batchID := "BATCH001"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(tradeColumns()).AddRow(
		1, "T1", "ETH-USDT", "O1", "O2",
		"0x1111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222",
		"2000", "1", "2000", 1, "2", "4", "USDT",
		model.SettlementStatusPending, batchID, "", now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trades" WHERE batch_id = \$1 ORDER BY matched_at ASC`).
		WithArgs(batchID).
		WillReturnRows(rows)

	trades, err := repo.ListByBatchID(ctx, batchID)

	assert.NoError(t, err)
	assert.Len(t, trades, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}
