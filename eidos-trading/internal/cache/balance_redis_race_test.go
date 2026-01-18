package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== Race Condition Tests for BalanceRedisRepository ==========
// These tests should be run with -race flag: go test -race ./internal/cache/...

func setupRaceTestRedis(t *testing.T) (*redis.Client, func()) {
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

// TestRace_ConcurrentFreeze 测试并发冻结余额的原子性
// 场景: 多个 goroutine 同时尝试从同一账户冻结余额
func TestRace_ConcurrentFreeze(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 初始化: 充值 1000 USDT (已结算)
	initialAmount := decimal.NewFromFloat(1000)
	err := repo.Credit(ctx, wallet, token, initialAmount, true)
	require.NoError(t, err)

	// 并发冻结: 10 个 goroutine 各冻结 50 USDT
	numGoroutines := 10
	freezeAmount := decimal.NewFromFloat(50)
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(orderID int) {
			defer wg.Done()
			err := repo.Freeze(ctx, wallet, token, freezeAmount, true, fmt.Sprintf("order-%d", orderID))
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证: 应该有 10 次成功冻结 (总共 500 USDT 可冻结)
	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 成功次数应 <= 20 (因为 1000/50 = 20)
	assert.LessOrEqual(t, successCount, int32(20))
	// 可用 + 冻结 = 原始金额
	total := balance.SettledAvailable.Add(balance.SettledFrozen)
	assert.True(t, total.Equal(initialAmount), "Total should equal initial amount")
	// 冻结金额应等于成功次数 * 单次冻结金额
	expectedFrozen := freezeAmount.Mul(decimal.NewFromInt(int64(successCount)))
	assert.True(t, balance.SettledFrozen.Equal(expectedFrozen),
		"Frozen should equal successful freeze count * amount: expected %s, got %s",
		expectedFrozen, balance.SettledFrozen)
}

// TestRace_ConcurrentFreezeUnfreeze 测试并发冻结和解冻的原子性
// 场景: 同时进行冻结和解冻操作
func TestRace_ConcurrentFreezeUnfreeze(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 初始化
	initialAmount := decimal.NewFromFloat(1000)
	err := repo.Credit(ctx, wallet, token, initialAmount, true)
	require.NoError(t, err)

	// 先冻结 500 USDT
	err = repo.Freeze(ctx, wallet, token, decimal.NewFromFloat(500), true, "initial-freeze")
	require.NoError(t, err)

	// 并发: 5 个 goroutine 冻结, 5 个 goroutine 解冻
	numPairs := 5
	amount := decimal.NewFromFloat(10)
	var wg sync.WaitGroup

	for i := 0; i < numPairs; i++ {
		// Freeze goroutine
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = repo.Freeze(ctx, wallet, token, amount, true, fmt.Sprintf("freeze-%d", id))
		}(i)

		// Unfreeze goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.Unfreeze(ctx, wallet, token, amount, true)
		}()
	}

	wg.Wait()

	// 验证: 总额应该不变
	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	total := balance.SettledAvailable.Add(balance.SettledFrozen)
	assert.True(t, total.Equal(initialAmount),
		"Total should equal initial amount: expected %s, got %s", initialAmount, total)

	// 确保没有负数余额
	assert.True(t, balance.SettledAvailable.GreaterThanOrEqual(decimal.Zero),
		"Available should be non-negative")
	assert.True(t, balance.SettledFrozen.GreaterThanOrEqual(decimal.Zero),
		"Frozen should be non-negative")
}

// TestRace_ConcurrentCredit 测试并发充值的原子性
// 场景: 多个 goroutine 同时充值
func TestRace_ConcurrentCredit(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "ETH"

	numGoroutines := 20
	creditAmount := decimal.NewFromFloat(1.5)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.Credit(ctx, wallet, token, creditAmount, true)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// 验证: 总额应等于充值次数 * 单次金额
	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	expectedTotal := creditAmount.Mul(decimal.NewFromInt(int64(numGoroutines)))
	assert.True(t, balance.SettledAvailable.Equal(expectedTotal),
		"Total should equal %s, got %s", expectedTotal, balance.SettledAvailable)
}

// TestRace_ConcurrentCreditDebit 测试并发充值和扣款
// 场景: 模拟并发交易清算
func TestRace_ConcurrentCreditDebit(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 初始化: 先冻结一些资金 (用于扣款)
	initialFrozen := decimal.NewFromFloat(1000)
	err := repo.Credit(ctx, wallet, token, initialFrozen, true)
	require.NoError(t, err)
	err = repo.Freeze(ctx, wallet, token, initialFrozen, true, "initial")
	require.NoError(t, err)

	// 并发: Credit (充值) 和 Debit (从冻结扣款)
	numOps := 10
	amount := decimal.NewFromFloat(50)
	var wg sync.WaitGroup

	for i := 0; i < numOps; i++ {
		// Credit
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.Credit(ctx, wallet, token, amount, false) // 充值到 pending
		}()

		// Debit
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.Debit(ctx, wallet, token, amount, true) // 从 settled frozen 扣款
		}()
	}

	wg.Wait()

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 验证: pending available 应该 = numOps * amount (所有充值成功)
	expectedPending := amount.Mul(decimal.NewFromInt(int64(numOps)))
	assert.True(t, balance.PendingAvailable.Equal(expectedPending),
		"Pending available should equal %s, got %s", expectedPending, balance.PendingAvailable)

	// 冻结应该减少 (扣款成功的次数)
	assert.True(t, balance.SettledFrozen.LessThanOrEqual(initialFrozen),
		"Frozen should be less than or equal to initial frozen")

	// 确保没有负数
	assert.True(t, balance.SettledFrozen.GreaterThanOrEqual(decimal.Zero),
		"Frozen should be non-negative")
}

// TestRace_ConcurrentSettle 测试并发结算 (pending → settled)
func TestRace_ConcurrentSettle(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "ETH"

	// 初始化: 充值到 pending
	initialPending := decimal.NewFromFloat(100)
	err := repo.Credit(ctx, wallet, token, initialPending, false)
	require.NoError(t, err)

	// 并发结算: 尝试多次结算同一笔资金
	numGoroutines := 10
	settleAmount := decimal.NewFromFloat(10)
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.Settle(ctx, wallet, token, settleAmount)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 验证: pending + settled 应该等于初始 pending
	total := balance.PendingAvailable.Add(balance.SettledAvailable)
	assert.True(t, total.Equal(initialPending),
		"Total should equal initial: expected %s, got %s", initialPending, total)

	// 成功次数应 <= initialPending / settleAmount
	maxSuccesses := int32(initialPending.Div(settleAmount).IntPart())
	assert.LessOrEqual(t, successCount, maxSuccesses,
		"Success count should be <= max possible: %d, got %d", maxSuccesses, successCount)
}

// TestRace_ConcurrentFreezeForOrder 测试并发 FreezeForOrder 操作
// 场景: 多个订单同时尝试冻结资金
func TestRace_ConcurrentFreezeForOrder(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 初始化
	initialAmount := decimal.NewFromFloat(10000)
	err := repo.Credit(ctx, wallet, token, initialAmount, true)
	require.NoError(t, err)

	// 并发创建订单并冻结资金
	numOrders := 20
	freezeAmount := decimal.NewFromFloat(100)
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()
			req := &FreezeForOrderRequest{
				Wallet:      wallet,
				Token:       token,
				Amount:      freezeAmount,
				OrderID:     fmt.Sprintf("O%010d", orderNum),
				FromSettled: true,
				OrderJSON:   fmt.Sprintf(`{"order_id":"O%010d"}`, orderNum),
				ShardID:     0,
				NonceKey:    fmt.Sprintf("nonce:%s:order:%d", wallet, orderNum),
				NonceTTL:    7 * 24 * time.Hour,
			}
			err := repo.FreezeForOrder(ctx, req)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 验证: available + frozen = initial
	total := balance.SettledAvailable.Add(balance.SettledFrozen)
	assert.True(t, total.Equal(initialAmount),
		"Total should equal initial: expected %s, got %s", initialAmount, total)

	// 验证: 所有订单冻结应该成功 (因为余额充足)
	assert.Equal(t, int32(numOrders), successCount,
		"All orders should succeed")

	// 验证: 冻结金额 = 成功次数 * 单次金额
	expectedFrozen := freezeAmount.Mul(decimal.NewFromInt(int64(successCount)))
	assert.True(t, balance.SettledFrozen.Equal(expectedFrozen),
		"Frozen should equal %s, got %s", expectedFrozen, balance.SettledFrozen)
}

// TestRace_ConcurrentUnfreezeByOrderID 测试并发按订单ID解冻
func TestRace_ConcurrentUnfreezeByOrderID(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 初始化: 创建多个订单冻结
	numOrders := 10
	freezeAmount := decimal.NewFromFloat(100)
	initialAmount := decimal.NewFromFloat(10000)

	err := repo.Credit(ctx, wallet, token, initialAmount, true)
	require.NoError(t, err)

	// 创建订单冻结
	for i := 0; i < numOrders; i++ {
		req := &FreezeForOrderRequest{
			Wallet:      wallet,
			Token:       token,
			Amount:      freezeAmount,
			OrderID:     fmt.Sprintf("O%010d", i),
			FromSettled: true,
			OrderJSON:   fmt.Sprintf(`{"order_id":"O%010d"}`, i),
			ShardID:     0,
			NonceKey:    fmt.Sprintf("nonce:%s:order:%d", wallet, i),
			NonceTTL:    7 * 24 * time.Hour,
		}
		err := repo.FreezeForOrder(ctx, req)
		require.NoError(t, err)
	}

	// 并发解冻: 每个订单尝试解冻两次 (测试幂等性)
	var wg sync.WaitGroup

	for i := 0; i < numOrders; i++ {
		orderID := fmt.Sprintf("O%010d", i)

		// 第一次解冻
		wg.Add(1)
		go func(oid string) {
			defer wg.Done()
			_ = repo.UnfreezeByOrderID(ctx, wallet, token, oid)
		}(orderID)

		// 第二次解冻 (应该是幂等的)
		wg.Add(1)
		go func(oid string) {
			defer wg.Done()
			_ = repo.UnfreezeByOrderID(ctx, wallet, token, oid)
		}(orderID)
	}

	wg.Wait()

	balance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 验证: 所有资金应该恢复可用
	assert.True(t, balance.SettledAvailable.Equal(initialAmount),
		"All funds should be available: expected %s, got %s", initialAmount, balance.SettledAvailable)
	assert.True(t, balance.SettledFrozen.IsZero(),
		"No funds should be frozen: got %s", balance.SettledFrozen)
}

// TestRace_ConcurrentTradeProcessed 测试并发检查和标记成交处理状态
func TestRace_ConcurrentTradeProcessed(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	tradeID := "T1234567890"

	// 并发: 检查和标记
	numGoroutines := 10
	var wg sync.WaitGroup
	firstMarkerCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 检查是否已处理
			processed, err := repo.CheckTradeProcessed(ctx, tradeID)
			require.NoError(t, err)

			if !processed {
				// 尝试标记为已处理
				err := repo.MarkTradeProcessed(ctx, tradeID, 1*time.Hour)
				if err == nil {
					mu.Lock()
					firstMarkerCount++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// 验证: 最终应该是已处理状态
	processed, err := repo.CheckTradeProcessed(ctx, tradeID)
	require.NoError(t, err)
	assert.True(t, processed, "Trade should be marked as processed")

	// 可能有多个 goroutine 同时看到未处理并尝试标记
	// 但最终状态是一致的
	assert.GreaterOrEqual(t, firstMarkerCount, int32(1),
		"At least one goroutine should succeed in marking")
}

// TestRace_ConcurrentMultipleWallets 测试多钱包并发操作
// 场景: 多个钱包同时进行操作，验证不会相互影响
func TestRace_ConcurrentMultipleWallets(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()

	numWallets := 5
	opsPerWallet := 10
	amount := decimal.NewFromFloat(100)
	var wg sync.WaitGroup

	// 并发: 每个钱包进行多次操作
	for w := 0; w < numWallets; w++ {
		wallet := fmt.Sprintf("0x%040d", w)

		// 初始化钱包
		err := repo.Credit(ctx, wallet, "USDT", decimal.NewFromFloat(10000), true)
		require.NoError(t, err)

		for i := 0; i < opsPerWallet; i++ {
			wg.Add(1)
			go func(w, i int, wallet string) {
				defer wg.Done()

				// 交替执行冻结和解冻
				if i%2 == 0 {
					_ = repo.Freeze(ctx, wallet, "USDT", amount, true, fmt.Sprintf("order-%d-%d", w, i))
				} else {
					_ = repo.Unfreeze(ctx, wallet, "USDT", amount, true)
				}
			}(w, i, wallet)
		}
	}

	wg.Wait()

	// 验证: 每个钱包的总额应该不变
	for w := 0; w < numWallets; w++ {
		wallet := fmt.Sprintf("0x%040d", w)
		balance, err := repo.GetBalance(ctx, wallet, "USDT")
		require.NoError(t, err)

		total := balance.SettledAvailable.Add(balance.SettledFrozen)
		assert.True(t, total.Equal(decimal.NewFromFloat(10000)),
			"Wallet %s total should be 10000, got %s", wallet, total)

		// 确保没有负数
		assert.True(t, balance.SettledAvailable.GreaterThanOrEqual(decimal.Zero))
		assert.True(t, balance.SettledFrozen.GreaterThanOrEqual(decimal.Zero))
	}
}

// TestRace_VersionIncrement 测试版本号原子递增
func TestRace_VersionIncrement(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	repo := NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"
	token := "ETH"

	// 初始化
	err := repo.Credit(ctx, wallet, token, decimal.NewFromFloat(1000), true)
	require.NoError(t, err)

	initialBalance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)
	initialVersion := initialBalance.Version

	// 并发操作
	numOps := 20
	var wg sync.WaitGroup

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.Credit(ctx, wallet, token, decimal.NewFromFloat(1), true)
		}()
	}

	wg.Wait()

	// 验证版本号递增
	finalBalance, err := repo.GetBalance(ctx, wallet, token)
	require.NoError(t, err)

	// 版本号应该增加了 numOps 次
	assert.Greater(t, finalBalance.Version, initialVersion,
		"Version should have incremented")
}
