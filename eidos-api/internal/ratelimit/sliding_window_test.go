package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) (*miniredis.Miniredis, *redis.Client, *SlidingWindow) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sw := NewSlidingWindow(rdb)
	return mr, rdb, sw
}

func TestNewSlidingWindow(t *testing.T) {
	_, rdb, sw := setupTest(t)
	defer rdb.Close()

	assert.NotNil(t, sw)
	assert.NotNil(t, sw.rdb)
	assert.NotNil(t, sw.script)
}

func TestSlidingWindow_Allow(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:allow"
	window := time.Minute
	limit := 5

	// 前 5 个请求应该被允许
	for i := 0; i < limit; i++ {
		allowed, err := sw.Allow(ctx, key, window, limit, uniqueID(i))
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i)
	}

	// 第 6 个请求应该被拒绝
	allowed, err := sw.Allow(ctx, key, window, limit, uniqueID(5))
	require.NoError(t, err)
	assert.False(t, allowed, "request 6 should be denied")
}

func TestSlidingWindow_Allow_ExpiredRequests(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:expired"
	window := 100 * time.Millisecond
	limit := 3

	// 发送 3 个请求
	for i := 0; i < limit; i++ {
		allowed, err := sw.Allow(ctx, key, window, limit, uniqueID(i))
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// 第 4 个应该被拒绝
	allowed, err := sw.Allow(ctx, key, window, limit, uniqueID(3))
	require.NoError(t, err)
	assert.False(t, allowed)

	// 快进时间，让窗口过期
	mr.FastForward(200 * time.Millisecond)

	// 现在应该可以再次发送请求
	allowed, err = sw.Allow(ctx, key, window, limit, uniqueID(4))
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestSlidingWindow_AllowN(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:allowN"
	window := time.Minute
	limit := 10

	// 一次请求 5 个配额
	allowed, err := sw.AllowN(ctx, key, window, limit, 5, "batch-1")
	require.NoError(t, err)
	assert.True(t, allowed)

	// 再请求 5 个应该成功
	allowed, err = sw.AllowN(ctx, key, window, limit, 5, "batch-2")
	require.NoError(t, err)
	assert.True(t, allowed)

	// 再请求 1 个应该失败
	allowed, err = sw.AllowN(ctx, key, window, limit, 1, "batch-3")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestSlidingWindow_AllowN_ExceedLimit(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:allowN:exceed"
	window := time.Minute
	limit := 5

	// 一次请求 6 个配额，超过限制
	allowed, err := sw.AllowN(ctx, key, window, limit, 6, "batch-1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestSlidingWindow_Remaining(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:remaining"
	window := time.Minute
	limit := 10

	// 初始应该有 10 个配额
	remaining, err := sw.Remaining(ctx, key, window, limit)
	require.NoError(t, err)
	assert.Equal(t, 10, remaining)

	// 消耗 3 个
	for i := 0; i < 3; i++ {
		sw.Allow(ctx, key, window, limit, uniqueID(i))
	}

	// 应该剩余 7 个
	remaining, err = sw.Remaining(ctx, key, window, limit)
	require.NoError(t, err)
	assert.Equal(t, 7, remaining)

	// 消耗完所有
	for i := 3; i < 10; i++ {
		sw.Allow(ctx, key, window, limit, uniqueID(i))
	}

	// 应该剩余 0 个
	remaining, err = sw.Remaining(ctx, key, window, limit)
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)
}

func TestSlidingWindow_Reset(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:reset"
	window := time.Minute
	limit := 5

	// 消耗所有配额
	for i := 0; i < limit; i++ {
		sw.Allow(ctx, key, window, limit, uniqueID(i))
	}

	// 验证配额已用完
	allowed, _ := sw.Allow(ctx, key, window, limit, uniqueID(5))
	assert.False(t, allowed)

	// 重置
	err := sw.Reset(ctx, key)
	require.NoError(t, err)

	// 重置后应该可以继续请求
	allowed, err = sw.Allow(ctx, key, window, limit, uniqueID(6))
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestSlidingWindow_Concurrent(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	ctx := context.Background()
	key := "test:concurrent"
	window := time.Minute
	limit := 100

	var wg sync.WaitGroup
	allowedCount := 0
	var mu sync.Mutex

	// 并发发送 150 个请求
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			allowed, err := sw.Allow(ctx, key, window, limit, uniqueID(id))
			require.NoError(t, err)
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 应该正好有 100 个请求被允许
	assert.Equal(t, limit, allowedCount)
}

func TestLimiter(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	limiter := NewLimiter(sw, Config{
		Window: time.Minute,
		Limit:  5,
		Prefix: "ratelimit:test:",
	})

	ctx := context.Background()
	identifier := "user123"

	// 测试 Allow
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, identifier, uniqueID(i))
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// 超过限制
	allowed, err := limiter.Allow(ctx, identifier, uniqueID(5))
	require.NoError(t, err)
	assert.False(t, allowed)

	// 测试 Remaining
	remaining, err := limiter.Remaining(ctx, identifier)
	require.NoError(t, err)
	assert.Equal(t, 0, remaining)
}

func TestLimiter_DifferentIdentifiers(t *testing.T) {
	mr, rdb, sw := setupTest(t)
	defer rdb.Close()
	defer mr.Close()

	limiter := NewLimiter(sw, Config{
		Window: time.Minute,
		Limit:  3,
		Prefix: "ratelimit:multi:",
	})

	ctx := context.Background()

	// 用户 A 消耗完配额
	for i := 0; i < 3; i++ {
		allowed, _ := limiter.Allow(ctx, "userA", uniqueID(i))
		assert.True(t, allowed)
	}
	allowed, _ := limiter.Allow(ctx, "userA", uniqueID(3))
	assert.False(t, allowed)

	// 用户 B 应该有自己的配额
	for i := 0; i < 3; i++ {
		allowed, _ := limiter.Allow(ctx, "userB", uniqueID(i))
		assert.True(t, allowed)
	}
}

// uniqueID 生成唯一 ID
func uniqueID(i int) string {
	return time.Now().Format("20060102150405") + "-" + string(rune('0'+i))
}
