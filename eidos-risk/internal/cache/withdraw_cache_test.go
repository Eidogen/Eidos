package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithdrawCache_RecordWithdraw(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	toAddress := "0xabcdef1234567890123456789012345678901234"
	timestamp := time.Now().UnixMilli()

	// Record withdraw
	err := cache.RecordWithdraw(ctx, wallet, toAddress, timestamp)
	require.NoError(t, err)

	// Check count
	count, err := cache.GetRecentWithdrawCount(ctx, wallet, 3600) // Last hour
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestWithdrawCache_GetRecentWithdrawCount(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	now := time.Now().UnixMilli()

	// Record multiple withdraws with unique timestamps and addresses
	for i := 0; i < 5; i++ {
		addr := "0xabcdef123456789012345678901234567890123" + string(rune('0'+i))
		ts := now - int64(i*60000) // Different timestamp for each
		err := cache.RecordWithdraw(ctx, wallet, addr, ts)
		require.NoError(t, err)
	}

	// All 5 should be within last hour
	count, err := cache.GetRecentWithdrawCount(ctx, wallet, 3600)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestWithdrawCache_AddressWhitelist(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	address1 := "0xabcdef1234567890123456789012345678901234"
	address2 := "0xfedcba1234567890123456789012345678901234"

	// Initially not whitelisted
	isWhitelisted, err := cache.IsWhitelistedAddress(ctx, wallet, address1)
	require.NoError(t, err)
	assert.False(t, isWhitelisted)

	// Add to whitelist
	err = cache.AddToWhitelist(ctx, wallet, address1)
	require.NoError(t, err)

	// Now should be whitelisted
	isWhitelisted, err = cache.IsWhitelistedAddress(ctx, wallet, address1)
	require.NoError(t, err)
	assert.True(t, isWhitelisted)

	// address2 still not whitelisted
	isWhitelisted, err = cache.IsWhitelistedAddress(ctx, wallet, address2)
	require.NoError(t, err)
	assert.False(t, isWhitelisted)
}

func TestWithdrawCache_RemoveFromWhitelist(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	address := "0xabcdef1234567890123456789012345678901234"

	// Add and then remove
	err := cache.AddToWhitelist(ctx, wallet, address)
	require.NoError(t, err)

	err = cache.RemoveFromWhitelist(ctx, wallet, address)
	require.NoError(t, err)

	// Should not be whitelisted
	isWhitelisted, err := cache.IsWhitelistedAddress(ctx, wallet, address)
	require.NoError(t, err)
	assert.False(t, isWhitelisted)
}

func TestWithdrawCache_GetWhitelistAddresses(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	addresses := []string{
		"0xabcdef1234567890123456789012345678901234",
		"0xfedcba1234567890123456789012345678901234",
		"0x1111111111111111111111111111111111111111",
	}

	// Add all addresses
	for _, addr := range addresses {
		err := cache.AddToWhitelist(ctx, wallet, addr)
		require.NoError(t, err)
	}

	// Get all
	got, err := cache.GetWhitelistAddresses(ctx, wallet)
	require.NoError(t, err)
	assert.Len(t, got, 3)

	// All should be present
	for _, addr := range addresses {
		assert.Contains(t, got, addr)
	}
}

func TestWithdrawCache_ContractAddress(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	contractAddr := "0xcontract1234567890123456789012345678901234"
	eoa := "0xeoa0000000000000000000000000000000000000"

	// Initially not marked
	isContract, err := cache.IsContractAddress(ctx, contractAddr)
	require.NoError(t, err)
	assert.False(t, isContract)

	// Mark as contract
	err = cache.MarkContractAddress(ctx, contractAddr)
	require.NoError(t, err)

	// Now should be marked
	isContract, err = cache.IsContractAddress(ctx, contractAddr)
	require.NoError(t, err)
	assert.True(t, isContract)

	// EOA should not be marked
	isContract, err = cache.IsContractAddress(ctx, eoa)
	require.NoError(t, err)
	assert.False(t, isContract)
}

func TestWithdrawCache_BatchMarkContractAddresses(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	contracts := []string{
		"0xcontract1111111111111111111111111111111111",
		"0xcontract2222222222222222222222222222222222",
		"0xcontract3333333333333333333333333333333333",
	}

	// Batch mark
	err := cache.BatchMarkContractAddresses(ctx, contracts)
	require.NoError(t, err)

	// Verify all marked
	for _, addr := range contracts {
		isContract, err := cache.IsContractAddress(ctx, addr)
		require.NoError(t, err)
		assert.True(t, isContract)
	}
}

func TestWithdrawCache_BatchMarkContractAddresses_Empty(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	// Empty batch should not error
	err := cache.BatchMarkContractAddresses(ctx, []string{})
	require.NoError(t, err)
}

func TestWithdrawCache_ClearWithdrawHistory(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	now := time.Now().UnixMilli()

	// Record some withdraws
	err := cache.RecordWithdraw(ctx, wallet, "0xaddr1234567890123456789012345678901234", now)
	require.NoError(t, err)

	count, err := cache.GetRecentWithdrawCount(ctx, wallet, 3600)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Clear history
	err = cache.ClearWithdrawHistory(ctx, wallet)
	require.NoError(t, err)

	// Count should be 0
	count, err = cache.GetRecentWithdrawCount(ctx, wallet, 3600)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestWithdrawCache_GetWithdrawStats(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	now := time.Now().UnixMilli()

	// Record some withdraws
	for i := 0; i < 3; i++ {
		addr := "0x" + string(rune('a'+i)) + "bcdef1234567890123456789012345678901234"
		err := cache.RecordWithdraw(ctx, wallet, addr, now-int64(i*60000))
		require.NoError(t, err)
	}

	// Add some whitelist addresses
	err := cache.AddToWhitelist(ctx, wallet, "0xwhite1234567890123456789012345678901234")
	require.NoError(t, err)
	err = cache.AddToWhitelist(ctx, wallet, "0xwhite5678901234567890123456789012345678")
	require.NoError(t, err)

	// Get stats
	stats, err := cache.GetWithdrawStats(ctx, wallet)
	require.NoError(t, err)

	assert.Equal(t, int64(3), stats["total_count"])
	assert.Equal(t, int64(3), stats["recent_1h_count"])
	assert.Equal(t, int64(2), stats["whitelist_count"])
}

func TestWithdrawCache_IsNewAddress(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	address := "0xabcdef1234567890123456789012345678901234"
	newAddress := "0xnewaddr234567890123456789012345678901234"
	now := time.Now().UnixMilli()

	// Record a withdraw to address
	err := cache.RecordWithdraw(ctx, wallet, address, now)
	require.NoError(t, err)

	// address should not be new
	isNew, err := cache.IsNewAddress(ctx, wallet, address)
	require.NoError(t, err)
	assert.False(t, isNew)

	// newAddress should be new
	isNew, err = cache.IsNewAddress(ctx, wallet, newAddress)
	require.NoError(t, err)
	assert.True(t, isNew)
}

func TestWithdrawCache_MultipleWallets(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewWithdrawCache(client)
	ctx := context.Background()

	wallet1 := "0x1111111111111111111111111111111111111111"
	wallet2 := "0x2222222222222222222222222222222222222222"
	now := time.Now().UnixMilli()

	// Record withdraws for wallet1 with unique timestamps (in the past, within window)
	for i := 0; i < 3; i++ {
		addr := "0xaddr1234567890123456789012345678901234" + string(rune('0'+i))
		// Use timestamps in the past but within the 1 hour window
		err := cache.RecordWithdraw(ctx, wallet1, addr, now-int64(i*60000))
		require.NoError(t, err)
	}

	// Record withdraws for wallet2 with unique timestamps
	for i := 0; i < 5; i++ {
		addr := "0xaddr5678901234567890123456789012345678" + string(rune('0'+i))
		err := cache.RecordWithdraw(ctx, wallet2, addr, now-int64(i*60000)-100)
		require.NoError(t, err)
	}

	// Check counts are independent
	count1, err := cache.GetRecentWithdrawCount(ctx, wallet1, 3600)
	require.NoError(t, err)
	assert.Equal(t, 3, count1)

	count2, err := cache.GetRecentWithdrawCount(ctx, wallet2, 3600)
	require.NoError(t, err)
	assert.Equal(t, 5, count2)

	// Whitelists are independent
	err = cache.AddToWhitelist(ctx, wallet1, "0xwhite1234567890123456789012345678901234")
	require.NoError(t, err)

	whitelist1, err := cache.GetWhitelistAddresses(ctx, wallet1)
	require.NoError(t, err)
	assert.Len(t, whitelist1, 1)

	whitelist2, err := cache.GetWhitelistAddresses(ctx, wallet2)
	require.NoError(t, err)
	assert.Len(t, whitelist2, 0)
}
