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

func TestRateLimitCache_CheckAndIncrement(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 5
	windowSec := 60

	// First 5 requests should pass
	for i := 0; i < 5; i++ {
		allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
	}

	// 6th request should be denied
	allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRateLimitCache_DifferentActions(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 2
	windowSec := 60

	// Use up order limit
	for i := 0; i < 2; i++ {
		allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Order should be denied
	allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Cancel should still be allowed (different action)
	allowed, err = cache.CheckAndIncrement(ctx, wallet, "cancel", windowSec, maxCount)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimitCache_GetCount(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 10
	windowSec := 60

	// Make 3 requests
	for i := 0; i < 3; i++ {
		_, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
	}

	// Check current count
	count, err := cache.GetCount(ctx, wallet, "order", windowSec)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestRateLimitCache_Reset(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 2
	windowSec := 60

	// Use up limit
	for i := 0; i < 2; i++ {
		_, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
	}

	// Should be denied
	allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Reset
	err = cache.Reset(ctx, wallet, "order", windowSec)
	require.NoError(t, err)

	// Should be allowed again
	allowed, err = cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimitCache_WindowExpiry(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 2
	windowSec := 1 // 1 second window

	// Use up limit
	for i := 0; i < 2; i++ {
		allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Should be denied
	allowed, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Fast forward time
	s.FastForward(2 * time.Second)

	// Should be allowed again after window expires
	allowed, err = cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimitCache_GetTTL(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	windowSec := 60

	// Make a request to create the key
	_, err := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, 10)
	require.NoError(t, err)

	// Check TTL
	ttl, err := cache.GetTTL(ctx, wallet, "order", windowSec)
	require.NoError(t, err)
	assert.True(t, ttl > 0)
	assert.True(t, ttl <= time.Duration(windowSec)*time.Second)
}

func TestRateLimitCache_IncrementBy(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	windowSec := 60

	// Increment by 5
	count, err := cache.IncrementBy(ctx, wallet, "order", windowSec, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, count)

	// Increment by 3 more
	count, err = cache.IncrementBy(ctx, wallet, "order", windowSec, 3)
	require.NoError(t, err)
	assert.Equal(t, 8, count)
}

func TestRateLimitCache_GetMultiple(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	windowSec := 60

	// Create some counts
	cache.IncrementBy(ctx, wallet, "order", windowSec, 5)
	cache.IncrementBy(ctx, wallet, "cancel", windowSec, 3)

	// Get multiple
	counts, err := cache.GetMultiple(ctx, wallet, []string{"order", "cancel", "withdraw"}, windowSec)
	require.NoError(t, err)
	assert.Equal(t, 5, counts["order"])
	assert.Equal(t, 3, counts["cancel"])
	assert.Equal(t, 0, counts["withdraw"]) // Not set
}

func TestRateLimitCache_SlidingWindowCheck(t *testing.T) {
	// Skip this test because miniredis doesn't properly support
	// the Lua script operations for sliding window with ZADD/ZCARD
	t.Skip("Skipping sliding window test - requires real Redis for Lua script support")

	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	windowSec := 10
	maxCount := 3

	// First 3 requests should pass
	for i := 0; i < 3; i++ {
		allowed, err := cache.SlidingWindowCheck(ctx, wallet, "order", windowSec, maxCount)
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
	}

	// 4th request should be denied
	allowed, err := cache.SlidingWindowCheck(ctx, wallet, "order", windowSec, maxCount)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRateLimitCache_ConcurrentRequests(t *testing.T) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	cache := NewRateLimitCache(client)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	maxCount := 10
	windowSec := 60

	// Note: miniredis doesn't truly test concurrent access like real Redis,
	// but this tests the basic logic
	results := make(chan bool, 15)

	for i := 0; i < 15; i++ {
		go func() {
			allowed, _ := cache.CheckAndIncrement(ctx, wallet, "order", windowSec, maxCount)
			results <- allowed
		}()
	}

	allowedCount := 0
	deniedCount := 0
	for i := 0; i < 15; i++ {
		if <-results {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	// Exactly 10 should be allowed
	assert.Equal(t, 10, allowedCount)
	assert.Equal(t, 5, deniedCount)
}
