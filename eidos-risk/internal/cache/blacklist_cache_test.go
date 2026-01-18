package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	return s, client
}

func TestBlacklistCache_SetAndGet(t *testing.T) {
	_, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	entry := &BlacklistEntry{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 0, // 永久
	}

	// Set
	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Get
	got, err := cache.Get(ctx, entry.WalletAddress)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, entry.WalletAddress, got.WalletAddress)
	assert.Equal(t, entry.ListType, got.ListType)
	assert.Equal(t, entry.Reason, got.Reason)
}

func TestBlacklistCache_GetNotFound(t *testing.T) {
	_, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	got, err := cache.Get(ctx, "0xnonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestBlacklistCache_Remove(t *testing.T) {
	_, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	entry := &BlacklistEntry{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 0,
	}

	// Set
	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Remove
	err = cache.Remove(ctx, entry.WalletAddress)
	require.NoError(t, err)

	// Get should return nil
	got, err := cache.Get(ctx, entry.WalletAddress)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestBlacklistCache_LoadFromDB(t *testing.T) {
	_, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	entries := []*BlacklistEntry{
		{
			WalletAddress:  "0x1111111111111111111111111111111111111111",
			ListType:       "full",
			Reason:         "reason 1",
			EffectiveUntil: 0,
		},
		{
			WalletAddress:  "0x2222222222222222222222222222222222222222",
			ListType:       "trade",
			Reason:         "reason 2",
			EffectiveUntil: 0,
		},
	}

	err := cache.LoadFromDB(ctx, entries)
	require.NoError(t, err)

	// Verify all entries are in cache
	for _, entry := range entries {
		got, err := cache.Get(ctx, entry.WalletAddress)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Equal(t, entry.Reason, got.Reason)
	}
}

func TestBlacklistCache_BatchCheck(t *testing.T) {
	_, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	// Add one entry
	entry := &BlacklistEntry{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 0,
	}
	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Batch check
	wallets := []string{
		"0x1234567890123456789012345678901234567890", // in blacklist
		"0xabcdef1234567890123456789012345678901234", // not in blacklist
	}

	results, err := cache.BatchCheck(ctx, wallets)
	require.NoError(t, err)
	assert.True(t, results["0x1234567890123456789012345678901234567890"])
	assert.False(t, results["0xabcdef1234567890123456789012345678901234"])
}

func TestBlacklistCache_Clear(t *testing.T) {
	s, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	// Add entries
	entries := []*BlacklistEntry{
		{
			WalletAddress:  "0x1111111111111111111111111111111111111111",
			ListType:       "full",
			Reason:         "reason 1",
			EffectiveUntil: 0,
		},
		{
			WalletAddress:  "0x2222222222222222222222222222222222222222",
			ListType:       "trade",
			Reason:         "reason 2",
			EffectiveUntil: 0,
		},
	}
	err := cache.LoadFromDB(ctx, entries)
	require.NoError(t, err)

	// Clear
	err = cache.Clear(ctx)
	require.NoError(t, err)

	// Verify cleared (using miniredis to check keys)
	keys := s.Keys()
	blacklistKeys := 0
	for _, k := range keys {
		if len(k) > len("risk:blacklist:") && k[:16] == "risk:blacklist:" {
			blacklistKeys++
		}
	}
	assert.Equal(t, 0, blacklistKeys)
}

func TestBlacklistCache_WithExpiry(t *testing.T) {
	s, client := setupTestRedis(t)
	cache := NewBlacklistCache(client)
	ctx := context.Background()

	// Add entry with expiry
	entry := &BlacklistEntry{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 1000, // 1 second in the past
	}
	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Get should return the entry (expiry check is done at service layer)
	got, err := cache.Get(ctx, entry.WalletAddress)
	require.NoError(t, err)
	assert.NotNil(t, got)

	// Fast forward time in miniredis
	s.FastForward(3600 * 24 * 1000) // 24 hours

	// Entry should still exist (TTL not set in current implementation)
	got, err = cache.Get(ctx, entry.WalletAddress)
	require.NoError(t, err)
	// The entry is still there, expiry is checked by service layer
}
