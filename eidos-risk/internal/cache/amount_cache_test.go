package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAmountCache_DailyWithdraw(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Initial should be zero
	amount, err := cache.GetDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())

	// Add first withdraw
	err = cache.AddDailyWithdraw(ctx, wallet, token, decimal.NewFromFloat(100.5))
	require.NoError(t, err)

	// Check amount
	amount, err = cache.GetDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(100.5)))

	// Add another withdraw
	err = cache.AddDailyWithdraw(ctx, wallet, token, decimal.NewFromFloat(50.25))
	require.NoError(t, err)

	// Check accumulated amount
	amount, err = cache.GetDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(150.75)))
}

func TestAmountCache_DailyWithdraw_NoToken(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Test with empty token
	err := cache.AddDailyWithdraw(ctx, wallet, "", decimal.NewFromFloat(200))
	require.NoError(t, err)

	amount, err := cache.GetDailyWithdraw(ctx, wallet, "")
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(200)))
}

func TestAmountCache_PendingSettle(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Initial should be zero
	amount, err := cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())

	// Add pending settlement
	err = cache.AddPendingSettle(ctx, wallet, decimal.NewFromFloat(1000))
	require.NoError(t, err)

	amount, err = cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(1000)))

	// Add more
	err = cache.AddPendingSettle(ctx, wallet, decimal.NewFromFloat(500))
	require.NoError(t, err)

	amount, err = cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(1500)))

	// Reduce pending settlement
	err = cache.ReducePendingSettle(ctx, wallet, decimal.NewFromFloat(300))
	require.NoError(t, err)

	amount, err = cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(1200)))
}

func TestAmountCache_ReducePendingSettle_ToZero(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Add pending settlement
	err := cache.AddPendingSettle(ctx, wallet, decimal.NewFromFloat(100))
	require.NoError(t, err)

	// Reduce to zero - should delete key
	err = cache.ReducePendingSettle(ctx, wallet, decimal.NewFromFloat(100))
	require.NoError(t, err)

	amount, err := cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())
}

func TestAmountCache_ReducePendingSettle_BelowZero(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Add pending settlement
	err := cache.AddPendingSettle(ctx, wallet, decimal.NewFromFloat(100))
	require.NoError(t, err)

	// Try to reduce more than exists - should floor at 0
	err = cache.ReducePendingSettle(ctx, wallet, decimal.NewFromFloat(200))
	require.NoError(t, err)

	amount, err := cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())
}

func TestAmountCache_SystemPendingSettle(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	// Initial should be zero
	amount, err := cache.GetSystemPendingSettle(ctx)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())

	// Add system pending
	err = cache.AddSystemPendingSettle(ctx, decimal.NewFromFloat(10000))
	require.NoError(t, err)

	amount, err = cache.GetSystemPendingSettle(ctx)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(10000)))

	// Reduce system pending
	err = cache.ReduceSystemPendingSettle(ctx, decimal.NewFromFloat(3000))
	require.NoError(t, err)

	amount, err = cache.GetSystemPendingSettle(ctx)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(7000)))
}

func TestAmountCache_SetPendingSettle(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// Set directly
	err := cache.SetPendingSettle(ctx, wallet, decimal.NewFromFloat(500))
	require.NoError(t, err)

	amount, err := cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.NewFromFloat(500)))

	// Set to zero should delete
	err = cache.SetPendingSettle(ctx, wallet, decimal.Zero)
	require.NoError(t, err)

	amount, err = cache.GetPendingSettle(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())
}

func TestAmountCache_ClearDailyWithdraw(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// Add withdraw
	err := cache.AddDailyWithdraw(ctx, wallet, token, decimal.NewFromFloat(100))
	require.NoError(t, err)

	// Clear
	err = cache.ClearDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)

	// Should be zero
	amount, err := cache.GetDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)
	assert.True(t, amount.IsZero())
}

func TestAmountCache_GetDailyResetTime(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)

	resetTime := cache.GetDailyResetTime()

	// Should be in the future
	assert.Greater(t, resetTime, int64(0))
}

func TestAmountCache_MultipleWallets(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewAmountCache(client)
	ctx := context.Background()

	wallet1 := "0x1111111111111111111111111111111111111111"
	wallet2 := "0x2222222222222222222222222222222222222222"
	token := "USDT"

	// Add to wallet1
	err := cache.AddDailyWithdraw(ctx, wallet1, token, decimal.NewFromFloat(100))
	require.NoError(t, err)

	// Add to wallet2
	err = cache.AddDailyWithdraw(ctx, wallet2, token, decimal.NewFromFloat(200))
	require.NoError(t, err)

	// Check they are independent
	amount1, err := cache.GetDailyWithdraw(ctx, wallet1, token)
	require.NoError(t, err)
	assert.True(t, amount1.Equal(decimal.NewFromFloat(100)))

	amount2, err := cache.GetDailyWithdraw(ctx, wallet2, token)
	require.NoError(t, err)
	assert.True(t, amount2.Equal(decimal.NewFromFloat(200)))
}
