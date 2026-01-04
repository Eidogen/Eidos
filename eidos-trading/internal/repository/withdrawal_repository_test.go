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

// withdrawalColumns 返回 withdrawals 表的所有列名
func withdrawalColumns() []string {
	return []string{
		"id", "withdraw_id", "wallet", "token", "amount", "fee", "net_amount",
		"status", "nonce", "signature", "tx_hash", "reject_reason",
		"submitted_at", "confirmed_at", "refunded_at",
		"created_at", "updated_at",
	}
}

func TestWithdrawalRepository_Create_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()

	withdrawal := &model.Withdrawal{
		WithdrawID: "W123456789",
		Wallet:     "0x1234567890123456789012345678901234567890",
		Token:      "USDT",
		Nonce:      1,
		Status:     model.WithdrawStatusPending,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "withdrawals"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.Create(ctx, withdrawal)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_GetByWithdrawID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(withdrawalColumns()).AddRow(
		1, withdrawID, "0x1234567890123456789012345678901234567890", "USDT",
		"1000.000000000000000000", "1.000000000000000000", "999.000000000000000000",
		model.WithdrawStatusPending, 1, nil, "", "",
		nil, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE withdraw_id = \$1 ORDER BY "withdrawals"\."id" LIMIT \$2`).
		WithArgs(withdrawID, 1).
		WillReturnRows(rows)

	withdrawal, err := repo.GetByWithdrawID(ctx, withdrawID)

	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, withdrawID, withdrawal.WithdrawID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_GetByWithdrawID_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W999999999"

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE withdraw_id = \$1 ORDER BY "withdrawals"\."id" LIMIT \$2`).
		WithArgs(withdrawID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	withdrawal, err := repo.GetByWithdrawID(ctx, withdrawID)

	assert.Nil(t, withdrawal)
	assert.ErrorIs(t, err, ErrWithdrawalNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_GetByWalletNonce_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	nonce := uint64(1)
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(withdrawalColumns()).AddRow(
		1, "W123", wallet, "USDT",
		"1000", "1", "999",
		model.WithdrawStatusPending, nonce, nil, "", "",
		nil, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE wallet = \$1 AND nonce = \$2 ORDER BY "withdrawals"\."id" LIMIT \$3`).
		WithArgs(wallet, nonce, 1).
		WillReturnRows(rows)

	withdrawal, err := repo.GetByWalletNonce(ctx, wallet, nonce)

	assert.NoError(t, err)
	assert.NotNil(t, withdrawal)
	assert.Equal(t, nonce, withdrawal.Nonce)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_ListPending_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(withdrawalColumns()).AddRow(
		1, "W1", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", "1", "999",
		model.WithdrawStatusPending, 1, nil, "", "",
		nil, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE status = \$1 ORDER BY created_at ASC LIMIT \$2`).
		WithArgs(model.WithdrawStatusPending, 100).
		WillReturnRows(rows)

	withdrawals, err := repo.ListPending(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, withdrawals, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_ListProcessing_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(withdrawalColumns()).AddRow(
		1, "W1", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", "1", "999",
		model.WithdrawStatusProcessing, 1, nil, "", "",
		nil, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE status = \$1 ORDER BY created_at ASC LIMIT \$2`).
		WithArgs(model.WithdrawStatusProcessing, 100).
		WillReturnRows(rows)

	withdrawals, err := repo.ListProcessing(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, withdrawals, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_ListSubmitted_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(withdrawalColumns()).AddRow(
		1, "W1", "0x1234567890123456789012345678901234567890", "USDT",
		"1000", "1", "999",
		model.WithdrawStatusSubmitted, 1, nil, "0xabc", "",
		now, nil, nil, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "withdrawals" WHERE status = \$1 ORDER BY submitted_at ASC LIMIT \$2`).
		WithArgs(model.WithdrawStatusSubmitted, 100).
		WillReturnRows(rows)

	withdrawals, err := repo.ListSubmitted(ctx, 100)

	assert.NoError(t, err)
	assert.Len(t, withdrawals, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkProcessing_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkProcessing(ctx, withdrawID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkSubmitted_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"
	txHash := "0xabcdef1234567890"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkSubmitted(ctx, withdrawID, txHash)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkConfirmed_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkConfirmed(ctx, withdrawID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkFailed_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"
	reason := "Insufficient gas"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkFailed(ctx, withdrawID, reason)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkRejected_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"
	reason := "Amount too large"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkRejected(ctx, withdrawID, reason)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_MarkCancelled_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.MarkCancelled(ctx, withdrawID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_CountByWallet_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	mock.ExpectQuery(`SELECT count\(\*\) FROM "withdrawals" WHERE wallet = \$1`).
		WithArgs(wallet).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	count, err := repo.CountByWallet(ctx, wallet, nil)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_CountPendingByWallet_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	mock.ExpectQuery(`SELECT count\(\*\) FROM "withdrawals" WHERE wallet = \$1 AND status IN \(\$2,\$3,\$4\)`).
		WithArgs(wallet,
			model.WithdrawStatusPending,
			model.WithdrawStatusProcessing,
			model.WithdrawStatusSubmitted).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountPendingByWallet(ctx, wallet)

	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithdrawalRepository_UpdateStatus_OptimisticLock(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewWithdrawalRepository(db)
	ctx := context.Background()
	withdrawID := "W999999999"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "withdrawals" SET`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.UpdateStatus(ctx, withdrawID, model.WithdrawStatusPending, model.WithdrawStatusProcessing)

	assert.ErrorIs(t, err, ErrOptimisticLock)
	assert.NoError(t, mock.ExpectationsWereMet())
}
