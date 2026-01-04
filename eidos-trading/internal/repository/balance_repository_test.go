package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// balanceColumns 返回 balances 表的所有列名
func balanceColumns() []string {
	return []string{
		"id", "wallet", "token", "settled_available", "settled_frozen",
		"pending_available", "pending_frozen", "pending_total", "version",
		"created_at", "updated_at", "created_by", "updated_by",
	}
}

func TestBalanceRepository_GetByWalletToken_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(balanceColumns()).AddRow(
		1, wallet, token, "1000.000000000000000000", "100.000000000000000000",
		"500.000000000000000000", "50.000000000000000000", "550.000000000000000000",
		1, now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1 AND token = \$2 ORDER BY "balances"\."id" LIMIT \$3`).
		WithArgs(wallet, token, 1).
		WillReturnRows(rows)

	balance, err := repo.GetByWalletToken(ctx, wallet, token)

	assert.NoError(t, err)
	assert.NotNil(t, balance)
	assert.Equal(t, wallet, balance.Wallet)
	assert.Equal(t, token, balance.Token)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromInt(1000)))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_GetByWalletToken_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1 AND token = \$2 ORDER BY "balances"\."id" LIMIT \$3`).
		WithArgs(wallet, token, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	balance, err := repo.GetByWalletToken(ctx, wallet, token)

	assert.Nil(t, balance)
	assert.ErrorIs(t, err, ErrBalanceNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_ListByWallet_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(balanceColumns()).AddRow(
		1, wallet, "USDT", "1000", "100", "500", "50", "550", 1, now, now, "", "",
	).AddRow(
		2, wallet, "ETH", "10", "1", "5", "0.5", "5.5", 1, now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1`).
		WithArgs(wallet).
		WillReturnRows(rows)

	balances, err := repo.ListByWallet(ctx, wallet)

	assert.NoError(t, err)
	assert.Len(t, balances, 2)
	assert.Equal(t, "USDT", balances[0].Token)
	assert.Equal(t, "ETH", balances[1].Token)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Freeze_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Freeze(ctx, wallet, token, amount, true)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Freeze_InsufficientBalance(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Freeze(ctx, wallet, token, amount, true)

	assert.ErrorIs(t, err, ErrInsufficientBalance)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Freeze_FromPending(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Freeze(ctx, wallet, token, amount, false) // fromSettled = false

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Unfreeze_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(50)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Unfreeze(ctx, wallet, token, amount, true)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Unfreeze_ToPending(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(50)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Unfreeze(ctx, wallet, token, amount, false) // toSettled = false

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Credit_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	now := time.Now().UnixMilli()

	// GetOrCreate 先查询
	rows := sqlmock.NewRows(balanceColumns()).AddRow(
		1, wallet, token, "1000", "100", "500", "50", "550", 1, now, now, "", "",
	)
	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1 AND token = \$2 ORDER BY "balances"\."id" LIMIT \$3`).
		WithArgs(wallet, token, 1).
		WillReturnRows(rows)

	// Credit 更新
	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Credit(ctx, wallet, token, amount, true)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Credit_ToPending(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)
	now := time.Now().UnixMilli()

	// GetOrCreate 先查询
	rows := sqlmock.NewRows(balanceColumns()).AddRow(
		1, wallet, token, "1000", "100", "500", "50", "550", 1, now, now, "", "",
	)
	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1 AND token = \$2 ORDER BY "balances"\."id" LIMIT \$3`).
		WithArgs(wallet, token, 1).
		WillReturnRows(rows)

	// Credit 更新到 pending
	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Credit(ctx, wallet, token, amount, false)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Debit_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(50)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Debit(ctx, wallet, token, amount, true)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Debit_InsufficientBalance(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(50)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Debit(ctx, wallet, token, amount, true)

	assert.ErrorIs(t, err, ErrInsufficientBalance)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Settle_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Settle(ctx, wallet, token, amount)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Settle_InsufficientBalance(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(100)

	mock.ExpectExec(`UPDATE balances`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Settle(ctx, wallet, token, amount)

	assert.ErrorIs(t, err, ErrInsufficientBalance)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_Freeze_InvalidAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 负数金额
	err := repo.Freeze(ctx, wallet, token, decimal.NewFromFloat(-100), true)
	assert.Error(t, err)

	// 零金额
	err = repo.Freeze(ctx, wallet, token, decimal.Zero, true)
	assert.Error(t, err)
}

func TestBalanceRepository_Unfreeze_InvalidAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 负数金额
	err := repo.Unfreeze(ctx, wallet, token, decimal.NewFromFloat(-100), true)
	assert.Error(t, err)

	// 零金额
	err = repo.Unfreeze(ctx, wallet, token, decimal.Zero, true)
	assert.Error(t, err)
}

func TestBalanceRepository_Credit_InvalidAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 负数金额
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(-100), true)
	assert.Error(t, err)

	// 零金额
	err = repo.Credit(ctx, wallet, token, decimal.Zero, true)
	assert.Error(t, err)
}

func TestBalanceRepository_Debit_InvalidAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 负数金额
	err := repo.Debit(ctx, wallet, token, decimal.NewFromFloat(-100), true)
	assert.Error(t, err)

	// 零金额
	err = repo.Debit(ctx, wallet, token, decimal.Zero, true)
	assert.Error(t, err)
}

func TestBalanceRepository_Settle_InvalidAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 负数金额
	err := repo.Settle(ctx, wallet, token, decimal.NewFromFloat(-100))
	assert.Error(t, err)

	// 零金额
	err = repo.Settle(ctx, wallet, token, decimal.Zero)
	assert.Error(t, err)
}

func TestBalanceRepository_GetByWalletTokenForUpdate_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(balanceColumns()).AddRow(
		1, wallet, token, "1000", "100", "500", "50", "550", 1, now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "balances" WHERE wallet = \$1 AND token = \$2 ORDER BY "balances"\."id" LIMIT \$3 FOR UPDATE`).
		WithArgs(wallet, token, 1).
		WillReturnRows(rows)

	balance, err := repo.GetByWalletTokenForUpdate(ctx, wallet, token)

	assert.NoError(t, err)
	assert.NotNil(t, balance)
	assert.Equal(t, wallet, balance.Wallet)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_CreateBalanceLog_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	log := &model.BalanceLog{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Type:      model.BalanceLogTypeDeposit,
		Amount:    decimal.NewFromFloat(100),
		CreatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "balance_logs"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.CreateBalanceLog(ctx, log)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_GetFeeAccount_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	bucketID := 1
	token := "USDT"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows([]string{
		"id", "bucket_id", "token", "balance", "version", "created_at", "updated_at",
	}).AddRow(
		1, bucketID, token, "1000.000000000000000000", 1, now, now,
	)

	mock.ExpectQuery(`SELECT \* FROM "fee_accounts" WHERE bucket_id = \$1 AND token = \$2 ORDER BY "fee_accounts"\."id" LIMIT \$3`).
		WithArgs(bucketID, token, 1).
		WillReturnRows(rows)

	account, err := repo.GetFeeAccount(ctx, bucketID, token)

	assert.NoError(t, err)
	assert.NotNil(t, account)
	assert.Equal(t, bucketID, account.BucketID)
	assert.Equal(t, token, account.Token)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_CreditFeeAccount_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	bucketID := 1
	token := "USDT"
	amount := decimal.NewFromFloat(10)

	mock.ExpectExec(`UPDATE fee_accounts`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.CreditFeeAccount(ctx, bucketID, token, amount)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBalanceRepository_CreditFeeAccount_ZeroAmount(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewBalanceRepository(db)
	ctx := context.Background()
	bucketID := 1
	token := "USDT"

	// 零金额应该直接返回 nil
	err := repo.CreditFeeAccount(ctx, bucketID, token, decimal.Zero)

	assert.NoError(t, err)
}
