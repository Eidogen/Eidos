package cache

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

// setupTestRedis 设置测试用 Redis
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return mr, rdb
}

// ========== ReplayGuard 单元测试 ==========

// TestNewReplayGuard 测试创建 ReplayGuard
func TestNewReplayGuard(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	assert.NotNil(t, guard)
	assert.Equal(t, DefaultReplayTTL, guard.ttl)
}

// TestNewReplayGuardWithTTL 测试创建带自定义 TTL 的 ReplayGuard
func TestNewReplayGuardWithTTL(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	customTTL := 10 * time.Minute
	guard := NewReplayGuardWithTTL(rdb, customTTL)
	assert.NotNil(t, guard)
	assert.Equal(t, customTTL, guard.ttl)
}

// TestReplayGuard_CheckAndMark_Success 测试首次检查并标记
func TestReplayGuard_CheckAndMark_Success(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 首次应该返回 true
	ok, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestReplayGuard_CheckAndMark_Replay 测试重放检测
func TestReplayGuard_CheckAndMark_Replay(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 首次应该返回 true
	ok, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok)

	// 第二次应该返回 false（重放）
	ok, err = guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestReplayGuard_CheckAndMark_DifferentSignatures 测试不同签名
func TestReplayGuard_CheckAndMark_DifferentSignatures(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"

	// 不同的签名应该都可以使用
	ok1, err := guard.CheckAndMark(ctx, wallet, timestamp, "0xsig1")
	require.NoError(t, err)
	assert.True(t, ok1)

	ok2, err := guard.CheckAndMark(ctx, wallet, timestamp, "0xsig2")
	require.NoError(t, err)
	assert.True(t, ok2)

	ok3, err := guard.CheckAndMark(ctx, wallet, timestamp, "0xsig3")
	require.NoError(t, err)
	assert.True(t, ok3)
}

// TestReplayGuard_CheckAndMark_DifferentWallets 测试不同钱包
func TestReplayGuard_CheckAndMark_DifferentWallets(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 不同的钱包使用相同签名应该都可以
	ok1, err := guard.CheckAndMark(ctx, "0xwallet1", timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok1)

	ok2, err := guard.CheckAndMark(ctx, "0xwallet2", timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok2)
}

// TestReplayGuard_Check_NotExists 测试检查未使用的签名
func TestReplayGuard_Check_NotExists(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 未使用的签名应该返回 true
	ok, err := guard.Check(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestReplayGuard_Check_Exists 测试检查已使用的签名
func TestReplayGuard_Check_Exists(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 先标记
	_, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)

	// 检查应该返回 false（已存在）
	ok, err := guard.Check(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestReplayGuard_Check_DoesNotMark 测试 Check 不会标记
func TestReplayGuard_Check_DoesNotMark(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// Check 不应该标记
	ok1, err := guard.Check(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok1)

	// 再次 Check 仍然应该返回 true
	ok2, err := guard.Check(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok2)

	// CheckAndMark 应该仍然可以成功
	ok3, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok3)
}

// TestReplayGuard_Mark 测试直接标记
func TestReplayGuard_Mark(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 直接标记
	err := guard.Mark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)

	// 检查应该返回 false（已标记）
	ok, err := guard.Check(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.False(t, ok)

	// CheckAndMark 也应该返回 false
	ok, err = guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestReplayGuard_TTL 测试 TTL 过期
func TestReplayGuard_TTL(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer rdb.Close()

	// 使用短 TTL
	guard := NewReplayGuardWithTTL(rdb, 1*time.Second)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 首次标记
	ok, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok)

	// 立即检查应该返回 false
	ok, err = guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.False(t, ok)

	// 使用 miniredis 的 FastForward 模拟时间流逝
	mr.FastForward(2 * time.Second)

	// TTL 过期后应该可以再次使用
	ok, err = guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestReplayGuard_ComputeHash 测试哈希计算一致性
func TestReplayGuard_ComputeHash(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 相同输入应该产生相同哈希
	hash1 := guard.computeHash(wallet, timestamp, signature)
	hash2 := guard.computeHash(wallet, timestamp, signature)
	assert.Equal(t, hash1, hash2)

	// 不同输入应该产生不同哈希
	hash3 := guard.computeHash(wallet, timestamp, "0xdifferent")
	assert.NotEqual(t, hash1, hash3)

	// 哈希应该是 64 字符的十六进制字符串 (SHA-256 = 32 bytes = 64 hex chars)
	assert.Len(t, hash1, 64)
}

// ========== 竞态测试 ==========

// TestReplayGuard_ConcurrentCheckAndMark 测试并发检查和标记
func TestReplayGuard_ConcurrentCheckAndMark(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	var wg sync.WaitGroup
	var successCount int32
	var mu sync.Mutex

	// 100 个并发请求
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
			if err == nil && ok {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// 只应该有一个成功
	assert.Equal(t, int32(1), successCount)
}

// TestReplayGuard_ConcurrentDifferentSignatures 测试并发不同签名
func TestReplayGuard_ConcurrentDifferentSignatures(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"

	var wg sync.WaitGroup
	var successCount int32
	var mu sync.Mutex

	// 50 个不同签名的并发请求
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			signature := "0xsig" + string(rune('0'+idx%10)) + string(rune('0'+idx/10))
			ok, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
			if err == nil && ok {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// 所有 50 个不同签名都应该成功
	assert.Equal(t, int32(50), successCount)
}

// TestReplayGuard_ConcurrentMixedOperations 测试并发混合操作
func TestReplayGuard_ConcurrentMixedOperations(t *testing.T) {
	_, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"

	var wg sync.WaitGroup

	// 并发执行各种操作
	for i := 0; i < 30; i++ {
		wg.Add(3)

		// Check 操作
		go func(idx int) {
			defer wg.Done()
			signature := "0xsig" + string(rune('a'+idx))
			_, _ = guard.Check(ctx, wallet, timestamp, signature)
		}(i)

		// Mark 操作
		go func(idx int) {
			defer wg.Done()
			signature := "0xmark" + string(rune('a'+idx))
			_ = guard.Mark(ctx, wallet, timestamp, signature)
		}(i)

		// CheckAndMark 操作
		go func(idx int) {
			defer wg.Done()
			signature := "0xcam" + string(rune('a'+idx))
			_, _ = guard.CheckAndMark(ctx, wallet, timestamp, signature)
		}(i)
	}
	wg.Wait()

	// 测试通过即可，主要验证不会 panic
}

// ========== 表驱动测试 ==========

// TestReplayGuard_CheckAndMark_TableDriven 表驱动测试
func TestReplayGuard_CheckAndMark_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		wallet    string
		timestamp string
		signature string
		// 执行两次 CheckAndMark 后的预期结果
		firstExpected  bool
		secondExpected bool
	}{
		{
			name:           "standard_signature",
			wallet:         "0x1234567890123456789012345678901234567890",
			timestamp:      "1704067200000",
			signature:      "0xabc123",
			firstExpected:  true,
			secondExpected: false,
		},
		{
			name:           "empty_signature",
			wallet:         "0x1234567890123456789012345678901234567890",
			timestamp:      "1704067200000",
			signature:      "",
			firstExpected:  true,
			secondExpected: false,
		},
		{
			name:           "long_signature",
			wallet:         "0x1234567890123456789012345678901234567890",
			timestamp:      "1704067200000",
			signature:      "0x" + string(make([]byte, 1000)),
			firstExpected:  true,
			secondExpected: false,
		},
		{
			name:           "special_characters",
			wallet:         "0x1234567890123456789012345678901234567890",
			timestamp:      "1704067200000",
			signature:      "0x!@#$%^&*()_+-=[]{}|;':\",./<>?",
			firstExpected:  true,
			secondExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rdb := setupTestRedis(t)
			defer rdb.Close()

			guard := NewReplayGuard(rdb)
			ctx := context.Background()

			// 第一次调用
			ok1, err := guard.CheckAndMark(ctx, tt.wallet, tt.timestamp, tt.signature)
			require.NoError(t, err)
			assert.Equal(t, tt.firstExpected, ok1)

			// 第二次调用
			ok2, err := guard.CheckAndMark(ctx, tt.wallet, tt.timestamp, tt.signature)
			require.NoError(t, err)
			assert.Equal(t, tt.secondExpected, ok2)
		})
	}
}

// TestReplayGuard_KeyPrefix 测试 Redis Key 前缀
func TestReplayGuard_KeyPrefix(t *testing.T) {
	mr, rdb := setupTestRedis(t)
	defer rdb.Close()

	guard := NewReplayGuard(rdb)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	timestamp := "1704067200000"
	signature := "0xabc123def456"

	// 标记签名
	_, err := guard.CheckAndMark(ctx, wallet, timestamp, signature)
	require.NoError(t, err)

	// 检查 Redis 中的 key 是否以正确前缀开头
	keys := mr.Keys()
	require.Len(t, keys, 1)
	assert.True(t, len(keys[0]) > len(ReplayKeyPrefix))
	assert.Equal(t, ReplayKeyPrefix, keys[0][:len(ReplayKeyPrefix)])
}
