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

// depositColumns 返回 deposits 表的所有列名
func depositColumns() []string {
	return []string{
		"id", "deposit_id", "wallet", "token", "amount", "status",
		"tx_hash", "log_index", "block_number", "from_address",
		"detected_at", "confirmed_at", "credited_at",
		"created_at", "updated_at",
	}
}

func TestDepositRepository_Create_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()

	deposit := &model.Deposit{
		DepositID: "D123456789",
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		TxHash:    "0xabcdef1234567890",
		LogIndex:  0,
		Status:    model.DepositStatusPending,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "trading_deposits"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.Create(ctx, deposit)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_GetByDepositID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	depositID := "D123456789"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(depositColumns()).AddRow(
		1, depositID, "0x1234567890123456789012345678901234567890", "USDT",
		"1000.000000000000000000", model.DepositStatusPending,
		"0xabcdef1234567890", 0, 1000000, "0x0000000000000000000000000000000000000000",
		now, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trading_deposits" WHERE deposit_id = \$1 ORDER BY "trading_deposits"\."id" LIMIT \$2`).
		WithArgs(depositID, 1).
		WillReturnRows(rows)

	deposit, err := repo.GetByDepositID(ctx, depositID)

	assert.NoError(t, err)
	assert.NotNil(t, deposit)
	assert.Equal(t, depositID, deposit.DepositID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_GetByDepositID_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	depositID := "D999999999"

	mock.ExpectQuery(`SELECT \* FROM "trading_deposits" WHERE deposit_id = \$1 ORDER BY "trading_deposits"\."id" LIMIT \$2`).
		WithArgs(depositID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	deposit, err := repo.GetByDepositID(ctx, depositID)

	assert.Nil(t, deposit)
	assert.ErrorIs(t, err, ErrDepositNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_GetByTxHashLogIndex_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	txHash := "0xabcdef1234567890"
	logIndex := uint32(0)
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(depositColumns()).AddRow(
		1, "D123", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", model.DepositStatusPending,
		txHash, logIndex, 1000000, "0x0000000000000000000000000000000000000000",
		now, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trading_deposits" WHERE tx_hash = \$1 AND log_index = \$2 ORDER BY "trading_deposits"\."id" LIMIT \$3`).
		WithArgs(txHash, logIndex, 1).
		WillReturnRows(rows)

	deposit, err := repo.GetByTxHashLogIndex(ctx, txHash, logIndex)

	assert.NoError(t, err)
	assert.NotNil(t, deposit)
	assert.Equal(t, txHash, deposit.TxHash)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_ListPending_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(depositColumns()).AddRow(
		1, "D1", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", model.DepositStatusPending,
		"0xabc", 0, 1000000, "0x0000000000000000000000000000000000000000",
		now, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trading_deposits" WHERE status = \$1 ORDER BY detected_at ASC LIMIT \$2`).
		WithArgs(model.DepositStatusPending, 100).
		WillReturnRows(rows)

	deposits, err := repo.ListPending(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, deposits, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_ListConfirmed_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(depositColumns()).AddRow(
		1, "D1", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", model.DepositStatusConfirmed,
		"0xabc", 0, 1000000, "0x0000000000000000000000000000000000000000",
		now, now, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "trading_deposits" WHERE status = \$1 ORDER BY confirmed_at ASC LIMIT \$2`).
		WithArgs(model.DepositStatusConfirmed, 100).
		WillReturnRows(rows)

	deposits, err := repo.ListConfirmed(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, deposits, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_MarkConfirmed_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	depositID := "D123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trading_deposits" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkConfirmed(ctx, depositID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_MarkConfirmed_OptimisticLock(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	depositID := "D999999999"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trading_deposits" SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.MarkConfirmed(ctx, depositID)

	assert.ErrorIs(t, err, ErrOptimisticLock)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_MarkCredited_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	depositID := "D123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "trading_deposits" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkCredited(ctx, depositID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDepositRepository_CountByWallet_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewDepositRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	mock.ExpectQuery(`SELECT count\(\*\) FROM "trading_deposits" WHERE wallet = \$1`).
		WithArgs(wallet).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	count, err := repo.CountByWallet(ctx, wallet, nil)

	assert.NoError(t, err)
	assert.Equal(t, int64(10), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}
