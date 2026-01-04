package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// setupMockDB 创建 mock 数据库连接
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

// orderColumns 返回 orders 表的所有列名
func orderColumns() []string {
	return []string{
		"id", "order_id", "wallet", "market", "side", "order_type", "price", "amount",
		"filled_amount", "filled_quote", "remaining_amount", "avg_price", "status",
		"time_in_force", "nonce", "client_order_id", "expire_at", "signature",
		"reject_reason", "freeze_token", "freeze_amount", "created_at", "updated_at",
		"created_by", "updated_by",
	}
}

func TestOrderRepository_Create_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()

	order := &model.Order{
		OrderID:      "O123456789",
		Wallet:       "0x1234567890123456789012345678901234567890",
		Market:       "ETH-USDT",
		Side:         model.OrderSideBuy,
		Type:         model.OrderTypeLimit,
		Price:        decimal.NewFromFloat(2000),
		Amount:       decimal.NewFromFloat(1),
		Status:       model.OrderStatusPending,
		FreezeToken:  "USDT",
		FreezeAmount: decimal.NewFromFloat(2000),
		Nonce:        1,
		ExpireAt:     time.Now().Add(24 * time.Hour).UnixMilli(),
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "orders"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	err := repo.Create(ctx, order)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByOrderID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O123456789"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(orderColumns()).AddRow(
		1, orderID, "0x1234567890123456789012345678901234567890", "ETH-USDT",
		1, 1, "2000.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", "0.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", 1, 1, 1, "", now,
		nil, "", "USDT", "2000.000000000000000000",
		now, now, "", "",
	)

	// GORM First() 会添加 ORDER BY id LIMIT 1
	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE order_id = \$1 ORDER BY "orders"\."id" LIMIT \$2`).
		WithArgs(orderID, 1).
		WillReturnRows(rows)

	order, err := repo.GetByOrderID(ctx, orderID)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, orderID, order.OrderID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByOrderID_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O999999999"

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE order_id = \$1 ORDER BY "orders"\."id" LIMIT \$2`).
		WithArgs(orderID, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	order, err := repo.GetByOrderID(ctx, orderID)

	assert.Nil(t, order)
	assert.ErrorIs(t, err, ErrOrderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByClientOrderID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	clientOrderID := "client-123"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(orderColumns()).AddRow(
		1, "O123", wallet, "ETH-USDT",
		1, 1, "2000.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", "0.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", 1, 1, 1, clientOrderID, now,
		nil, "", "USDT", "2000.000000000000000000",
		now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE wallet = \$1 AND client_order_id = \$2 ORDER BY "orders"\."id" LIMIT \$3`).
		WithArgs(wallet, clientOrderID, 1).
		WillReturnRows(rows)

	order, err := repo.GetByClientOrderID(ctx, wallet, clientOrderID)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, clientOrderID, order.ClientOrderID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByWalletNonce_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	nonce := uint64(12345)
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(orderColumns()).AddRow(
		1, "O123", wallet, "ETH-USDT",
		1, 1, "2000.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", "0.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", 1, 1, nonce, "", now,
		nil, "", "USDT", "2000.000000000000000000",
		now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE wallet = \$1 AND nonce = \$2 ORDER BY "orders"\."id" LIMIT \$3`).
		WithArgs(wallet, nonce, 1).
		WillReturnRows(rows)

	order, err := repo.GetByWalletNonce(ctx, wallet, nonce)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, nonce, order.Nonce)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_UpdateStatus_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "orders" SET "status"=\$1,"updated_at"=\$2 WHERE order_id = \$3 AND status = \$4`).
		WithArgs(model.OrderStatusCancelled, sqlmock.AnyArg(), orderID, model.OrderStatusOpen).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpdateStatus(ctx, orderID, model.OrderStatusOpen, model.OrderStatusCancelled)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_UpdateStatus_OptimisticLock(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O123456789"

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "orders" SET "status"=\$1,"updated_at"=\$2 WHERE order_id = \$3 AND status = \$4`).
		WithArgs(model.OrderStatusCancelled, sqlmock.AnyArg(), orderID, model.OrderStatusOpen).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := repo.UpdateStatus(ctx, orderID, model.OrderStatusOpen, model.OrderStatusCancelled)

	assert.ErrorIs(t, err, ErrOptimisticLock)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_ListOpenOrders_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(orderColumns()).AddRow(
		1, "O1", wallet, market, 1, 1, "2000", "1", "0", "0", "1", "0",
		model.OrderStatusOpen, 1, 1, "", now, nil, "", "USDT", "2000",
		now, now, "", "",
	).AddRow(
		2, "O2", wallet, market, 2, 1, "2100", "0.5", "0.2", "420", "0.3", "2100",
		model.OrderStatusPartial, 1, 2, "", now, nil, "", "ETH", "0.5",
		now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE status IN \(\$1,\$2\) AND wallet = \$3 AND market = \$4 ORDER BY created_at ASC`).
		WithArgs(model.OrderStatusOpen, model.OrderStatusPartial, wallet, market).
		WillReturnRows(rows)

	orders, err := repo.ListOpenOrders(ctx, wallet, market)

	assert.NoError(t, err)
	assert.Len(t, orders, 2)
	assert.Equal(t, "O1", orders[0].OrderID)
	assert.Equal(t, "O2", orders[1].OrderID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_ListOpenOrders_EmptyFilters(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows(orderColumns())

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE status IN \(\$1,\$2\) ORDER BY created_at ASC`).
		WithArgs(model.OrderStatusOpen, model.OrderStatusPartial).
		WillReturnRows(rows)

	orders, err := repo.ListOpenOrders(ctx, "", "")

	assert.NoError(t, err)
	assert.Len(t, orders, 0)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_CountByWallet_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	mock.ExpectQuery(`SELECT count\(\*\) FROM "orders" WHERE wallet = \$1`).
		WithArgs(wallet).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	count, err := repo.CountByWallet(ctx, wallet, nil)

	assert.NoError(t, err)
	assert.Equal(t, int64(10), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_CountByWallet_WithFilter(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	filter := &OrderFilter{
		Market:   "ETH-USDT",
		Statuses: []model.OrderStatus{model.OrderStatusOpen, model.OrderStatusPartial},
	}

	mock.ExpectQuery(`SELECT count\(\*\) FROM "orders" WHERE wallet = \$1 AND market = \$2 AND status IN \(\$3,\$4\)`).
		WithArgs(wallet, "ETH-USDT", model.OrderStatusOpen, model.OrderStatusPartial).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	count, err := repo.CountByWallet(ctx, wallet, filter)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_UpdateFilled_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O123456789"

	mock.ExpectExec(`UPDATE orders`).
		WithArgs("0.5", "1000", model.OrderStatusPartial, sqlmock.AnyArg(), orderID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateFilled(ctx, orderID, "0.5", "1000", model.OrderStatusPartial)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_UpdateFilled_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	orderID := "O999999999"

	mock.ExpectExec(`UPDATE orders`).
		WithArgs("0.5", "1000", model.OrderStatusPartial, sqlmock.AnyArg(), orderID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.UpdateFilled(ctx, orderID, "0.5", "1000", model.OrderStatusPartial)

	assert.ErrorIs(t, err, ErrOrderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByID_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	id := int64(1)
	now := time.Now().UnixMilli()

	rows := sqlmock.NewRows(orderColumns()).AddRow(
		id, "O123", "0x1234567890123456789012345678901234567890", "ETH-USDT",
		1, 1, "2000.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", "0.000000000000000000", "1.000000000000000000",
		"0.000000000000000000", 1, 1, 1, "", now,
		nil, "", "USDT", "2000.000000000000000000",
		now, now, "", "",
	)

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE id = \$1 ORDER BY "orders"\."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnRows(rows)

	order, err := repo.GetByID(ctx, id)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, id, order.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_GetByID_NotFound(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	id := int64(999)

	mock.ExpectQuery(`SELECT \* FROM "orders" WHERE id = \$1 ORDER BY "orders"\."id" LIMIT \$2`).
		WithArgs(id, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	order, err := repo.GetByID(ctx, id)

	assert.Nil(t, order)
	assert.ErrorIs(t, err, ErrOrderNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOrderRepository_Update_Success(t *testing.T) {
	db, mock, cleanup := setupMockDB(t)
	defer cleanup()

	repo := NewOrderRepository(db)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	order := &model.Order{
		ID:      1,
		OrderID: "O123456789",
		Wallet:  "0x1234567890123456789012345678901234567890",
		Market:  "ETH-USDT",
		Status:  model.OrderStatusOpen,
	}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "orders" SET`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// 设置时间戳
	order.UpdatedAt = now
	err := repo.Update(ctx, order)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 辅助函数：消除未使用 import 警告
var _ sql.DB
