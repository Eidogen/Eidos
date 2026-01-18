// Package blockchain Nonce 管理器测试
// 使用 go test -race 运行竞态测试
package blockchain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBlockchainClient 模拟区块链客户端
type mockBlockchainClient struct {
	mu           sync.RWMutex
	pendingNonce uint64
}

func newMockBlockchainClient(initialNonce uint64) *mockBlockchainClient {
	return &mockBlockchainClient{
		pendingNonce: initialNonce,
	}
}

func (m *mockBlockchainClient) PendingNonceAt(ctx context.Context, wallet common.Address) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingNonce, nil
}

func (m *mockBlockchainClient) SetPendingNonce(nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingNonce = nonce
}

// setupTestNonceManager 创建测试用的 NonceManager
func setupTestNonceManager(t *testing.T, initialNonce uint64) (*NonceManager, *miniredis.Miniredis, *mockBlockchainClient, func()) {
	// 创建 miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 创建模拟区块链客户端
	mockClient := newMockBlockchainClient(initialNonce)

	// 创建一个包装客户端，实现 NonceManager 需要的接口
	wallet := common.HexToAddress("0x1234567890123456789012345678901234567890")

	// 由于 NonceManager 需要一个真正的 Client，我们需要创建一个测试用的包装器
	nm := &NonceManager{
		redis:        rdb,
		wallet:       wallet,
		chainID:      31337,
		lockTimeout:  5 * time.Second,
		syncInterval: 5 * time.Minute,
		maxPending:   100,
		pendingTxs:   make(map[uint64]string),
	}

	cleanup := func() {
		rdb.Close()
		mr.Close()
	}

	return nm, mr, mockClient, cleanup
}

func TestNonceManager_KeyGeneration(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	// 验证 key 格式
	nonceKey := nm.nonceKey()
	lockKey := nm.lockKey()
	pendingKey := nm.pendingKey()

	assert.Contains(t, nonceKey, "eidos:chain:nonce:")
	assert.Contains(t, lockKey, "eidos:chain:nonce:lock:")
	assert.Contains(t, pendingKey, "eidos:chain:nonce:pending:")
	assert.Contains(t, nonceKey, nm.wallet.Hex())
	assert.Contains(t, nonceKey, "31337")
}

func TestNonceManager_AcquireLock(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 第一次获取锁应该成功
	acquired, err := nm.acquireLock(ctx)
	assert.NoError(t, err)
	assert.True(t, acquired)

	// 第二次获取锁应该失败（因为已经被持有）
	acquired2, err := nm.acquireLock(ctx)
	assert.NoError(t, err)
	assert.False(t, acquired2)

	// 释放锁后应该可以重新获取
	err = nm.releaseLock(ctx)
	assert.NoError(t, err)

	acquired3, err := nm.acquireLock(ctx)
	assert.NoError(t, err)
	assert.True(t, acquired3)

	nm.releaseLock(ctx)
}

func TestNonceManager_SetGetCurrentNonce(t *testing.T) {
	nm, mr, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 设置初始 nonce
	mr.Set(nm.nonceKey(), "10")

	// 获取 nonce
	nonce, err := nm.getCurrentNonce(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), nonce)

	// 设置新 nonce
	err = nm.setCurrentNonce(ctx, 20)
	assert.NoError(t, err)

	// 验证
	nonce, err = nm.getCurrentNonce(ctx)
	assert.NoError(t, err)
	assert.Equal(t, uint64(20), nonce)
}

func TestNonceManager_ConfirmNonce(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 模拟获取了一个 nonce
	nonce := uint64(5)
	nm.pendingMu.Lock()
	nm.pendingTxs[nonce] = ""
	nm.pendingMu.Unlock()

	// 确认 nonce
	txHash := "0xabc123"
	err := nm.ConfirmNonce(ctx, nonce, txHash)
	assert.NoError(t, err)

	// 验证 txHash 已关联
	nm.pendingMu.RLock()
	hash, exists := nm.pendingTxs[nonce]
	nm.pendingMu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, txHash, hash)
}

func TestNonceManager_ReleaseNonce(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 模拟获取了一个 nonce
	nonce := uint64(5)
	nm.pendingMu.Lock()
	nm.pendingTxs[nonce] = ""
	nm.pendingMu.Unlock()

	// 释放 nonce
	err := nm.ReleaseNonce(ctx, nonce)
	assert.NoError(t, err)

	// 验证已从 pending 移除
	nm.pendingMu.RLock()
	_, exists := nm.pendingTxs[nonce]
	nm.pendingMu.RUnlock()
	assert.False(t, exists)
}

func TestNonceManager_ReleaseNonce_NotAcquired(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 尝试释放未获取的 nonce
	err := nm.ReleaseNonce(ctx, 99)
	assert.ErrorIs(t, err, ErrNonceNotAcquired)
}

func TestNonceManager_OnTxConfirmed(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 模拟有一个待确认交易
	nonce := uint64(5)
	txHash := "0xabc123"
	nm.pendingMu.Lock()
	nm.pendingTxs[nonce] = txHash
	nm.pendingMu.Unlock()

	// 确认交易
	err := nm.OnTxConfirmed(ctx, nonce, txHash)
	assert.NoError(t, err)

	// 验证已从 pending 移除
	nm.pendingMu.RLock()
	_, exists := nm.pendingTxs[nonce]
	nm.pendingMu.RUnlock()
	assert.False(t, exists)
}

func TestNonceManager_OnTxFailed(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 模拟有一个待确认交易
	nonce := uint64(5)
	txHash := "0xabc123"
	nm.pendingMu.Lock()
	nm.pendingTxs[nonce] = txHash
	nm.pendingMu.Unlock()

	// 交易失败
	err := nm.OnTxFailed(ctx, nonce, txHash)
	assert.NoError(t, err)

	// 验证已从 pending 移除
	nm.pendingMu.RLock()
	_, exists := nm.pendingTxs[nonce]
	nm.pendingMu.RUnlock()
	assert.False(t, exists)
}

func TestNonceManager_GetPendingCount(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	// 初始应该为 0
	assert.Equal(t, 0, nm.GetPendingCount())

	// 添加一些待确认交易
	nm.pendingMu.Lock()
	nm.pendingTxs[1] = "0xabc"
	nm.pendingTxs[2] = "0xdef"
	nm.pendingTxs[3] = "0xghi"
	nm.pendingMu.Unlock()

	assert.Equal(t, 3, nm.GetPendingCount())
}

func TestNonceManager_NeedsSync(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	// 初始应该需要同步（因为 lastSyncTime 是零值）
	assert.True(t, nm.needsSync())

	// 设置最近同步时间
	nm.mu.Lock()
	nm.lastSyncTime = time.Now()
	nm.mu.Unlock()

	// 现在不应该需要同步
	assert.False(t, nm.needsSync())

	// 模拟时间过去
	nm.mu.Lock()
	nm.lastSyncTime = time.Now().Add(-10 * time.Minute)
	nm.mu.Unlock()

	// 应该需要同步
	assert.True(t, nm.needsSync())
}

// ======== 竞态条件测试 ========

// TestNonceManager_ConcurrentLockAcquisition 测试并发锁获取
func TestNonceManager_ConcurrentLockAcquisition(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()
	concurrency := 10
	iterations := 100

	var successCount int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				acquired, err := nm.acquireLock(ctx)
				if err != nil {
					continue
				}
				if acquired {
					mu.Lock()
					successCount++
					mu.Unlock()
					nm.releaseLock(ctx)
				}
			}
		}()
	}

	wg.Wait()

	// 验证锁机制正常工作
	assert.Greater(t, successCount, int64(0))
	t.Logf("Lock acquired %d times out of %d attempts", successCount, concurrency*iterations)
}

// TestNonceManager_ConcurrentPendingTxOperations 测试并发待确认交易操作
func TestNonceManager_ConcurrentPendingTxOperations(t *testing.T) {
	nm, _, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()
	concurrency := 10
	iterations := 100

	var wg sync.WaitGroup

	// 并发添加
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				nonce := uint64(base*iterations + j)
				nm.pendingMu.Lock()
				nm.pendingTxs[nonce] = ""
				nm.pendingMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证所有交易都被添加
	assert.Equal(t, concurrency*iterations, nm.GetPendingCount())

	// 并发删除 - 使用 OnTxConfirmed 来删除条目
	// 注意: ConfirmNonce 只是更新 txHash，不会删除条目
	// OnTxConfirmed 和 OnTxFailed 才会从 pendingTxs 中删除
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				nonce := uint64(base*iterations + j)
				txHash := "0x" + string(rune('a'+base))

				// 使用 OnTxConfirmed 或 OnTxFailed 来删除条目
				if j%2 == 0 {
					nm.OnTxConfirmed(ctx, nonce, txHash)
				} else {
					nm.OnTxFailed(ctx, nonce, txHash)
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证所有交易都被处理
	assert.Equal(t, 0, nm.GetPendingCount())
}

// TestNonceManager_ConcurrentReads 测试并发读取
func TestNonceManager_ConcurrentReads(t *testing.T) {
	nm, mr, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 设置初始 nonce
	mr.Set(nm.nonceKey(), "100")

	// 添加一些待确认交易
	for i := 0; i < 50; i++ {
		nm.pendingMu.Lock()
		nm.pendingTxs[uint64(i)] = "0xhash"
		nm.pendingMu.Unlock()
	}

	var wg sync.WaitGroup
	concurrency := 20
	iterations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = nm.GetPendingCount()
				_, _ = nm.GetCurrentNonce(ctx)
				_ = nm.needsSync()
			}
		}()
	}

	wg.Wait()
}

// TestNonceManager_ConcurrentReadWrite 测试并发读写
func TestNonceManager_ConcurrentReadWrite(t *testing.T) {
	nm, mr, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 设置初始 nonce
	mr.Set(nm.nonceKey(), "0")

	var wg sync.WaitGroup
	concurrency := 10

	// 写入 goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				nonce := uint64(id*50 + j)

				// 模拟获取锁
				acquired, err := nm.acquireLock(ctx)
				if err != nil || !acquired {
					continue
				}

				// 设置 nonce
				nm.setCurrentNonce(ctx, nonce)

				// 添加到 pending
				nm.pendingMu.Lock()
				nm.pendingTxs[nonce] = "0xhash"
				nm.pendingMu.Unlock()

				nm.releaseLock(ctx)
			}
		}(i)
	}

	// 读取 goroutines
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = nm.GetCurrentNonce(ctx)
				_ = nm.GetPendingCount()
			}
		}()
	}

	wg.Wait()
}

// TestNonceManager_LockTimeout 测试锁超时
func TestNonceManager_LockTimeout(t *testing.T) {
	nm, mr, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 设置较短的锁超时
	nm.lockTimeout = 100 * time.Millisecond

	// 获取锁
	acquired, err := nm.acquireLock(ctx)
	assert.NoError(t, err)
	assert.True(t, acquired)

	// 等待锁过期
	mr.FastForward(200 * time.Millisecond)

	// 现在应该可以获取锁（因为前一个锁已过期）
	acquired2, err := nm.acquireLock(ctx)
	assert.NoError(t, err)
	assert.True(t, acquired2)

	nm.releaseLock(ctx)
}

// TestNonceManager_HighConcurrencyStress 高并发压力测试
func TestNonceManager_HighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	nm, mr, _, cleanup := setupTestNonceManager(t, 0)
	defer cleanup()

	ctx := context.Background()

	// 设置初始 nonce
	mr.Set(nm.nonceKey(), "0")

	var wg sync.WaitGroup
	concurrency := 50
	iterations := 200

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch j % 5 {
				case 0:
					// 尝试获取锁并设置 nonce
					acquired, _ := nm.acquireLock(ctx)
					if acquired {
						nm.setCurrentNonce(ctx, uint64(id*iterations+j))
						nm.releaseLock(ctx)
					}
				case 1:
					// 添加 pending tx
					nonce := uint64(id*iterations + j)
					nm.pendingMu.Lock()
					nm.pendingTxs[nonce] = ""
					nm.pendingMu.Unlock()
				case 2:
					// 确认 nonce
					nonce := uint64(id*iterations + j - 1)
					nm.ConfirmNonce(ctx, nonce, "0xhash")
				case 3:
					// 读取 pending count
					_ = nm.GetPendingCount()
				case 4:
					// 读取当前 nonce
					_, _ = nm.GetCurrentNonce(ctx)
				}
			}
		}(i)
	}

	wg.Wait()
}
