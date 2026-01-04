package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return rdb, func() {
		rdb.Close()
		mr.Close()
	}
}

func TestBalanceRedisRepository_GetOrCreateBalance(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()

	// Test create new balance
	balance, err := repo.GetOrCreateBalance(ctx, "0x1234567890123456789012345678901234567890", "USDT")
	require.NoError(t, err)
	assert.Equal(t, "0x1234567890123456789012345678901234567890", balance.Wallet)
	assert.Equal(t, "USDT", balance.Token)
	assert.True(t, balance.SettledAvailable.IsZero())
	assert.True(t, balance.SettledFrozen.IsZero())
	assert.True(t, balance.PendingAvailable.IsZero())
	assert.True(t, balance.PendingFrozen.IsZero())

	// Test get existing balance
	balance2, err := repo.GetOrCreateBalance(ctx, "0x1234567890123456789012345678901234567890", "USDT")
	require.NoError(t, err)
	assert.Equal(t, balance.Version, balance2.Version)
}

func TestBalanceRedisRepository_Credit(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Credit to settled
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(100.5), true)
	require.NoError(t, err)

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(100.5)))

	// Credit to pending
	err = repo.Credit(ctx, wallet, token, decimal.NewFromFloat(50.25), false)
	require.NoError(t, err)

	balance, err = repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.PendingAvailable.Equal(decimal.NewFromFloat(50.25)))
}

func TestBalanceRedisRepository_Freeze(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Setup initial balance
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(100), true)
	require.NoError(t, err)

	// Freeze from settled
	err = repo.Freeze(ctx, wallet, token, decimal.NewFromFloat(30), true, "order-1")
	require.NoError(t, err)

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(70)))
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(30)))
}

func TestBalanceRedisRepository_Freeze_InsufficientBalance(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Setup initial balance
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(50), true)
	require.NoError(t, err)

	// Try to freeze more than available
	err = repo.Freeze(ctx, wallet, token, decimal.NewFromFloat(100), true, "order-1")
	assert.Error(t, err)
}

func TestBalanceRedisRepository_Unfreeze(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Setup: credit and freeze
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(100), true)
	require.NoError(t, err)
	err = repo.Freeze(ctx, wallet, token, decimal.NewFromFloat(50), true, "order-1")
	require.NoError(t, err)

	// Unfreeze
	err = repo.Unfreeze(ctx, wallet, token, decimal.NewFromFloat(30), true)
	require.NoError(t, err)

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(80)))
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(20)))
}

func TestBalanceRedisRepository_Settle(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "ETH"

	// Setup: credit to pending
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(10), false)
	require.NoError(t, err)

	// Settle: move from pending to settled
	err = repo.Settle(ctx, wallet, token, decimal.NewFromFloat(5))
	require.NoError(t, err)

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.PendingAvailable.Equal(decimal.NewFromFloat(5)))
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(5)))
}

func TestBalanceRedisRepository_FreezeForOrder(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	orderID := "O123456789"

	// Setup: credit to settled
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(1000), true)
	require.NoError(t, err)

	// Freeze for order (with nonce and outbox)
	req := &FreezeForOrderRequest{
		Wallet:      wallet,
		Token:       token,
		Amount:      decimal.NewFromFloat(100),
		OrderID:     orderID,
		FromSettled: true,
		OrderJSON:   `{"order_id":"O123456789"}`,
		ShardID:     0,
		NonceKey:    "nonce:0x1234:order:12345",
		NonceTTL:    7 * 24 * time.Hour,
	}

	err = repo.FreezeForOrder(ctx, req)
	require.NoError(t, err)

	// Verify balance
	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(900)))
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(100)))

	// Verify order freeze record
	freeze, err := repo.GetOrderFreeze(ctx, orderID)
	require.NoError(t, err)
	require.NotNil(t, freeze)
	assert.True(t, freeze.SettledAmount.Equal(decimal.NewFromFloat(100)))
}

func TestBalanceRedisRepository_UnfreezeByOrderID(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	orderID := "O987654321"

	// Setup: credit and freeze for order
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(1000), true)
	require.NoError(t, err)

	req := &FreezeForOrderRequest{
		Wallet:      wallet,
		Token:       token,
		Amount:      decimal.NewFromFloat(200),
		OrderID:     orderID,
		FromSettled: true,
		OrderJSON:   `{"order_id":"O987654321"}`,
		ShardID:     0,
		NonceKey:    "nonce:0x1234:order:99999",
		NonceTTL:    7 * 24 * time.Hour,
	}
	err = repo.FreezeForOrder(ctx, req)
	require.NoError(t, err)

	// Unfreeze by order ID
	err = repo.UnfreezeByOrderID(ctx, wallet, token, orderID)
	require.NoError(t, err)

	// Verify balance restored
	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(1000)))
	assert.True(t, balance.SettledFrozen.IsZero())

	// Verify order freeze record deleted
	freeze, err := repo.GetOrderFreeze(ctx, orderID)
	require.NoError(t, err)
	assert.Nil(t, freeze)
}

func TestBalanceRedisRepository_CheckTradeProcessed(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	tradeID := "T123456789"

	// Initially not processed
	processed, err := repo.CheckTradeProcessed(ctx, tradeID)
	require.NoError(t, err)
	assert.False(t, processed)

	// Mark as processed
	err = repo.MarkTradeProcessed(ctx, tradeID, 7*24*time.Hour)
	require.NoError(t, err)

	// Now should be processed
	processed, err = repo.CheckTradeProcessed(ctx, tradeID)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestBalanceRedisRepository_SyncBalanceFromDB(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()

	// Sync balance from DB model
	dbBalance := &model.Balance{
		Wallet:           "0xABCDEF1234567890123456789012345678901234",
		Token:            "ETH",
		SettledAvailable: decimal.NewFromFloat(10.5),
		SettledFrozen:    decimal.NewFromFloat(2.5),
		PendingAvailable: decimal.NewFromFloat(1.0),
		PendingFrozen:    decimal.NewFromFloat(0.5),
		Version:          5,
		UpdatedAt:        time.Now().UnixMilli(),
	}

	err := repo.SyncBalanceFromDB(ctx, dbBalance)
	require.NoError(t, err)

	// Verify synced balance
	balance, err := repo.GetBalance(ctx, dbBalance.Wallet, dbBalance.Token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(dbBalance.SettledAvailable))
	assert.True(t, balance.SettledFrozen.Equal(dbBalance.SettledFrozen))
	assert.True(t, balance.PendingAvailable.Equal(dbBalance.PendingAvailable))
	assert.True(t, balance.PendingFrozen.Equal(dbBalance.PendingFrozen))
	assert.Equal(t, dbBalance.Version, balance.Version)
}

func TestBalanceRedisRepository_TotalAvailable(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Credit to both settled and pending
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(100), true)
	require.NoError(t, err)
	err = repo.Credit(ctx, wallet, token, decimal.NewFromFloat(50), false)
	require.NoError(t, err)

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// TotalAvailable should be settled + pending
	assert.True(t, balance.TotalAvailable().Equal(decimal.NewFromFloat(150)))
}

func TestBalanceRedisRepository_CreditFeeBucket(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()

	// Credit fee to bucket
	err := repo.CreditFeeBucket(ctx, "ETH-USDT", "USDT", 0, decimal.NewFromFloat(10.5))
	require.NoError(t, err)

	err = repo.CreditFeeBucket(ctx, "ETH-USDT", "USDT", 0, decimal.NewFromFloat(5.25))
	require.NoError(t, err)

	// Verify total (via raw Redis read)
	key := "eidos:trading:fee_bucket:ETH-USDT:USDT:0"
	val, err := rdb.Get(ctx, key).Float64()
	require.NoError(t, err)
	assert.InDelta(t, 15.75, val, 0.001)
}

func TestBalanceRedisRepository_GetUserPendingTotal(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// Initially zero
	total, err := repo.GetUserPendingTotal(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, total.IsZero())

	// Set a pending total manually
	key := "eidos:trading:pending:0x1234567890123456789012345678901234567890"
	rdb.Set(ctx, key, "1000.5", 0)

	total, err = repo.GetUserPendingTotal(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, total.Equal(decimal.NewFromFloat(1000.5)))
}
