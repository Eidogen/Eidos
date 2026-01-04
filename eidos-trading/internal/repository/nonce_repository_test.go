package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redismock/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// usedNonceColumns 返回 used_nonces 表的所有列名
func usedNonceColumns() []string {
	return []string{
		"id", "wallet", "usage", "nonce", "order_id", "created_at",
	}
}

func TestNonceRepository_IsUsed_ExistsInRedis(t *testing.T) {
	db, _, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, rdbMock := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)

	key := model.GetNonceRedisKey(wallet, usage, nonce)
	rdbMock.ExpectExists(key).SetVal(1)

	isUsed, err := repo.IsUsed(ctx, wallet, usage, nonce)

	assert.NoError(t, err)
	assert.True(t, isUsed)
	assert.NoError(t, rdbMock.ExpectationsWereMet())
}

func TestNonceRepository_IsUsed_NotInRedis_ExistsInDB(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, rdbMock := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)

	key := model.GetNonceRedisKey(wallet, usage, nonce)
	rdbMock.ExpectExists(key).SetVal(0)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "used_nonces" WHERE wallet = \$1 AND usage = \$2 AND nonce = \$3`).
		WithArgs(wallet, usage, nonce).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	isUsed, err := repo.IsUsed(ctx, wallet, usage, nonce)

	assert.NoError(t, err)
	assert.True(t, isUsed)
	assert.NoError(t, rdbMock.ExpectationsWereMet())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_IsUsed_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, rdbMock := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)

	key := model.GetNonceRedisKey(wallet, usage, nonce)
	rdbMock.ExpectExists(key).SetVal(0)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "used_nonces" WHERE wallet = \$1 AND usage = \$2 AND nonce = \$3`).
		WithArgs(wallet, usage, nonce).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	isUsed, err := repo.IsUsed(ctx, wallet, usage, nonce)

	assert.NoError(t, err)
	assert.False(t, isUsed)
	assert.NoError(t, rdbMock.ExpectationsWereMet())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_IsUsed_RedisError_FallbackToDB(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, rdbMock := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)

	key := model.GetNonceRedisKey(wallet, usage, nonce)
	rdbMock.ExpectExists(key).SetErr(errors.New("redis connection error"))

	mock.ExpectQuery(`SELECT count\(\*\) FROM "used_nonces" WHERE wallet = \$1 AND usage = \$2 AND nonce = \$3`).
		WithArgs(wallet, usage, nonce).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	isUsed, err := repo.IsUsed(ctx, wallet, usage, nonce)

	assert.NoError(t, err)
	assert.True(t, isUsed)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_MarkUsed_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, rdbMock := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "used_nonces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	key := model.GetNonceRedisKey(wallet, usage, nonce)
	rdbMock.ExpectSet(key, "1", 7*24*time.Hour).SetVal("OK")

	err := repo.MarkUsed(ctx, wallet, usage, nonce, orderID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	assert.NoError(t, rdbMock.ExpectationsWereMet())
}

func TestNonceRepository_MarkUsed_AlreadyUsed(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "used_nonces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // No row returned = 0 rows affected
	mock.ExpectCommit()

	err := repo.MarkUsed(ctx, wallet, usage, nonce, orderID)

	assert.ErrorIs(t, err, ErrNonceAlreadyUsed)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_MarkUsed_DBError(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "used_nonces"`).
		WillReturnError(errors.New("database error"))
	mock.ExpectRollback()

	err := repo.MarkUsed(ctx, wallet, usage, nonce, orderID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mark nonce used in db failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_MarkUsedWithTx_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "used_nonces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.MarkUsedWithTx(ctx, wallet, usage, nonce, orderID)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_MarkUsedWithTx_AlreadyUsed(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	nonce := uint64(123)
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "used_nonces"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // No row = 0 rows affected
	mock.ExpectCommit()

	err := repo.MarkUsedWithTx(ctx, wallet, usage, nonce, orderID)

	assert.ErrorIs(t, err, ErrNonceAlreadyUsed)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_GetLatestNonce_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(usedNonceColumns()).AddRow(
		1, wallet, usage, 100, "O123", now,
	)

	mock.ExpectQuery(`SELECT \* FROM "used_nonces" WHERE wallet = \$1 AND usage = \$2 ORDER BY nonce DESC,"used_nonces"\."id" LIMIT \$3`).
		WithArgs(wallet, usage, 1).
		WillReturnRows(rows)

	nonce, err := repo.GetLatestNonce(ctx, wallet, usage)

	assert.NoError(t, err)
	assert.Equal(t, uint64(100), nonce)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_GetLatestNonce_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	usage := model.NonceUsageOrder

	mock.ExpectQuery(`SELECT \* FROM "used_nonces" WHERE wallet = \$1 AND usage = \$2 ORDER BY nonce DESC,"used_nonces"\."id" LIMIT \$3`).
		WithArgs(wallet, usage, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	nonce, err := repo.GetLatestNonce(ctx, wallet, usage)

	assert.NoError(t, err)
	assert.Equal(t, uint64(0), nonce)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_CleanExpired_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	beforeTime := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	batchSize := 100

	// First batch: delete 50 records
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "used_nonces" WHERE created_at < \$1`).
		WithArgs(beforeTime).
		WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectCommit()

	deleted, err := repo.CleanExpired(ctx, beforeTime, batchSize)

	assert.NoError(t, err)
	assert.Equal(t, int64(50), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_CleanExpired_MultipleBatches(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	beforeTime := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	batchSize := 100

	// First batch: delete 100 records
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "used_nonces" WHERE created_at < \$1`).
		WithArgs(beforeTime).
		WillReturnResult(sqlmock.NewResult(0, 100))
	mock.ExpectCommit()

	// Second batch: delete 30 records (less than batch size, so stops)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "used_nonces" WHERE created_at < \$1`).
		WithArgs(beforeTime).
		WillReturnResult(sqlmock.NewResult(0, 30))
	mock.ExpectCommit()

	deleted, err := repo.CleanExpired(ctx, beforeTime, batchSize)

	assert.NoError(t, err)
	assert.Equal(t, int64(130), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_CleanExpired_NothingToDelete(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	beforeTime := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	batchSize := 100

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "used_nonces" WHERE created_at < \$1`).
		WithArgs(beforeTime).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	deleted, err := repo.CleanExpired(ctx, beforeTime, batchSize)

	assert.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceRepository_CleanExpired_Error(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rdb, _ := redismock.NewClientMock()

	repo := NewNonceRepository(db, rdb)
	ctx := context.Background()
	beforeTime := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	batchSize := 100

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "used_nonces" WHERE created_at < \$1`).
		WithArgs(beforeTime).
		WillReturnError(errors.New("database error"))
	mock.ExpectRollback()

	deleted, err := repo.CleanExpired(ctx, beforeTime, batchSize)

	assert.Error(t, err)
	assert.Equal(t, int64(0), deleted)
	assert.Contains(t, err.Error(), "clean expired nonces failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}
