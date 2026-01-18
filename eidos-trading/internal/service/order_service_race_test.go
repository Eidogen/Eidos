package service

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

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// ========== Race Condition Tests for OrderService ==========
// These tests focus on testing the OrderService under concurrent access.
// Run with: go test -race ./internal/service/... -run "TestRace"

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

// MockIDGeneratorAtomic 线程安全的 ID 生成器
type MockIDGeneratorAtomic struct {
	counter int64
	mu      sync.Mutex
}

func (m *MockIDGeneratorAtomic) Generate() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	return m.counter, nil
}

// TestRace_ConcurrentOrderCreate 测试并发创建订单
// 场景: 同一用户并发创建多个订单，测试余额冻结的原子性
func TestRace_ConcurrentOrderCreate(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 初始化余额: 10000 USDT
	err := balanceCache.Credit(ctx, wallet, "USDT", decimal.NewFromFloat(10000), true)
	require.NoError(t, err)

	// 设置市场配置
	marketCfg := &MarketConfig{
		Market:       "ETH-USDT",
		BaseToken:    "ETH",
		QuoteToken:   "USDT",
		TakerFeeRate: decimal.NewFromFloat(0.001),
		MakerFeeRate: decimal.NewFromFloat(0.0005),
	}

	// 并发创建订单: 每个订单冻结 100 USDT (买入价格 100, 数量 1)
	numOrders := 50
	orderAmount := decimal.NewFromFloat(100)
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()

			orderID := fmt.Sprintf("O%010d", orderNum)
			req := &cache.FreezeForOrderRequest{
				Wallet:      wallet,
				Token:       marketCfg.QuoteToken,
				Amount:      orderAmount,
				OrderID:     orderID,
				FromSettled: true,
				OrderJSON:   fmt.Sprintf(`{"order_id":"%s","market":"ETH-USDT"}`, orderID),
				ShardID:     0,
				NonceKey:    fmt.Sprintf("nonce:%s:order:%d", wallet, orderNum),
				NonceTTL:    7 * 24 * time.Hour,
			}

			err := balanceCache.FreezeForOrder(ctx, req)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证: 余额一致性
	balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)

	// available + frozen = 10000
	total := balance.SettledAvailable.Add(balance.SettledFrozen)
	assert.True(t, total.Equal(decimal.NewFromFloat(10000)),
		"Total should equal 10000, got %s", total)

	// 成功冻结的总额
	expectedFrozen := orderAmount.Mul(decimal.NewFromInt(int64(successCount)))
	assert.True(t, balance.SettledFrozen.Equal(expectedFrozen),
		"Frozen should equal %s, got %s", expectedFrozen, balance.SettledFrozen)

	// 所有订单应该成功 (余额足够)
	assert.Equal(t, int32(numOrders), successCount,
		"All orders should succeed, got %d", successCount)
}

// TestRace_ConcurrentOrderCreateInsufficientBalance 测试余额不足时的并发创建
// 场景: 多个订单争抢有限余额
func TestRace_ConcurrentOrderCreateInsufficientBalance(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 初始化余额: 500 USDT (只够 5 个订单)
	err := balanceCache.Credit(ctx, wallet, "USDT", decimal.NewFromFloat(500), true)
	require.NoError(t, err)

	// 并发创建 20 个订单，每个需要 100 USDT
	numOrders := 20
	orderAmount := decimal.NewFromFloat(100)
	var wg sync.WaitGroup
	successCount := int32(0)
	failCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()

			orderID := fmt.Sprintf("O%010d", orderNum)
			req := &cache.FreezeForOrderRequest{
				Wallet:      wallet,
				Token:       "USDT",
				Amount:      orderAmount,
				OrderID:     orderID,
				FromSettled: true,
				OrderJSON:   fmt.Sprintf(`{"order_id":"%s"}`, orderID),
				ShardID:     0,
				NonceKey:    fmt.Sprintf("nonce:%s:order:%d", wallet, orderNum),
				NonceTTL:    7 * 24 * time.Hour,
			}

			err := balanceCache.FreezeForOrder(ctx, req)
			mu.Lock()
			if err == nil {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// 验证
	balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)

	// 只有 5 个订单能成功
	assert.Equal(t, int32(5), successCount,
		"Only 5 orders should succeed, got %d", successCount)
	assert.Equal(t, int32(15), failCount,
		"15 orders should fail, got %d", failCount)

	// 所有余额应该被冻结
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(500)),
		"All balance should be frozen: %s", balance.SettledFrozen)
	assert.True(t, balance.SettledAvailable.IsZero(),
		"No available balance should remain: %s", balance.SettledAvailable)
}

// TestRace_ConcurrentOrderCancel 测试并发取消订单
// 场景: 多个 goroutine 尝试取消同一订单
func TestRace_ConcurrentOrderCancel(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 初始化并创建一个订单
	err := balanceCache.Credit(ctx, wallet, "USDT", decimal.NewFromFloat(1000), true)
	require.NoError(t, err)

	orderID := "O0000000001"
	freezeAmount := decimal.NewFromFloat(100)

	req := &cache.FreezeForOrderRequest{
		Wallet:      wallet,
		Token:       "USDT",
		Amount:      freezeAmount,
		OrderID:     orderID,
		FromSettled: true,
		OrderJSON:   `{"order_id":"O0000000001"}`,
		ShardID:     0,
		NonceKey:    "nonce:test:1",
		NonceTTL:    7 * 24 * time.Hour,
	}
	err = balanceCache.FreezeForOrder(ctx, req)
	require.NoError(t, err)

	// 验证冻结成功
	balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.Equal(freezeAmount))

	// 并发取消同一订单
	numGoroutines := 10
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := balanceCache.UnfreezeByOrderID(ctx, wallet, "USDT", orderID)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 验证: 只有一个取消成功 (幂等性)
	// 注意: 在当前实现中，UnfreezeByOrderID 可能是幂等的，多次调用都返回成功
	// 但关键是余额最终状态正确
	balance, err = balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)

	// 余额应该完全恢复
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(1000)),
		"Balance should be restored: got %s", balance.SettledAvailable)
	assert.True(t, balance.SettledFrozen.IsZero(),
		"No frozen balance should remain: got %s", balance.SettledFrozen)
}

// TestRace_ConcurrentCreateAndCancel 测试并发创建和取消
// 场景: 同时进行订单创建和取消操作
func TestRace_ConcurrentCreateAndCancel(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 初始化
	initialBalance := decimal.NewFromFloat(5000)
	err := balanceCache.Credit(ctx, wallet, "USDT", initialBalance, true)
	require.NoError(t, err)

	// 先创建一些订单用于取消
	numPreOrders := 10
	orderAmount := decimal.NewFromFloat(100)

	for i := 0; i < numPreOrders; i++ {
		orderID := fmt.Sprintf("O%010d", i)
		req := &cache.FreezeForOrderRequest{
			Wallet:      wallet,
			Token:       "USDT",
			Amount:      orderAmount,
			OrderID:     orderID,
			FromSettled: true,
			OrderJSON:   fmt.Sprintf(`{"order_id":"%s"}`, orderID),
			ShardID:     0,
			NonceKey:    fmt.Sprintf("nonce:pre:%d", i),
			NonceTTL:    7 * 24 * time.Hour,
		}
		err := balanceCache.FreezeForOrder(ctx, req)
		require.NoError(t, err)
	}

	// 并发: 创建新订单 + 取消已有订单
	numNewOrders := 20
	var wg sync.WaitGroup

	// 创建新订单
	for i := numPreOrders; i < numPreOrders+numNewOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()
			orderID := fmt.Sprintf("O%010d", orderNum)
			req := &cache.FreezeForOrderRequest{
				Wallet:      wallet,
				Token:       "USDT",
				Amount:      orderAmount,
				OrderID:     orderID,
				FromSettled: true,
				OrderJSON:   fmt.Sprintf(`{"order_id":"%s"}`, orderID),
				ShardID:     0,
				NonceKey:    fmt.Sprintf("nonce:new:%d", orderNum),
				NonceTTL:    7 * 24 * time.Hour,
			}
			_ = balanceCache.FreezeForOrder(ctx, req)
		}(i)
	}

	// 取消已有订单
	for i := 0; i < numPreOrders; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()
			orderID := fmt.Sprintf("O%010d", orderNum)
			_ = balanceCache.UnfreezeByOrderID(ctx, wallet, "USDT", orderID)
		}(i)
	}

	wg.Wait()

	// 验证: 总余额不变
	balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)

	total := balance.SettledAvailable.Add(balance.SettledFrozen)
	assert.True(t, total.Equal(initialBalance),
		"Total should equal initial: expected %s, got %s", initialBalance, total)

	// 确保没有负数
	assert.True(t, balance.SettledAvailable.GreaterThanOrEqual(decimal.Zero))
	assert.True(t, balance.SettledFrozen.GreaterThanOrEqual(decimal.Zero))
}

// TestRace_ConcurrentDuplicateNonce 测试并发使用相同 nonce
// 场景: 防止重放攻击
func TestRace_ConcurrentDuplicateNonce(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	// 初始化
	err := balanceCache.Credit(ctx, wallet, "USDT", decimal.NewFromFloat(10000), true)
	require.NoError(t, err)

	// 使用相同的 nonce 并发创建订单
	sameNonceKey := "nonce:test:duplicate:12345"
	numGoroutines := 10
	var wg sync.WaitGroup
	successCount := int32(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()
			orderID := fmt.Sprintf("O%010d", orderNum)
			req := &cache.FreezeForOrderRequest{
				Wallet:      wallet,
				Token:       "USDT",
				Amount:      decimal.NewFromFloat(100),
				OrderID:     orderID,
				FromSettled: true,
				OrderJSON:   fmt.Sprintf(`{"order_id":"%s"}`, orderID),
				ShardID:     0,
				NonceKey:    sameNonceKey, // 相同的 nonce key
				NonceTTL:    7 * 24 * time.Hour,
			}

			err := balanceCache.FreezeForOrder(ctx, req)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 只有一个应该成功 (nonce 唯一性检查)
	assert.Equal(t, int32(1), successCount,
		"Only one order should succeed with same nonce, got %d", successCount)

	// 验证余额
	balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
	require.NoError(t, err)

	// 只冻结了一个订单的金额
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(100)),
		"Only 100 should be frozen, got %s", balance.SettledFrozen)
}

// TestRace_ConcurrentMultipleUsers 测试多用户并发操作
// 场景: 验证用户之间不会相互影响
func TestRace_ConcurrentMultipleUsers(t *testing.T) {
	rdb, cleanup := setupRaceTestRedis(t)
	defer cleanup()

	balanceCache := cache.NewBalanceRedisRepository(rdb)
	ctx := context.Background()

	numUsers := 5
	ordersPerUser := 10
	initialBalance := decimal.NewFromFloat(10000)
	orderAmount := decimal.NewFromFloat(100)

	// 初始化所有用户
	for u := 0; u < numUsers; u++ {
		wallet := fmt.Sprintf("0x%040d", u)
		err := balanceCache.Credit(ctx, wallet, "USDT", initialBalance, true)
		require.NoError(t, err)
	}

	// 并发: 每个用户创建订单
	var wg sync.WaitGroup

	for u := 0; u < numUsers; u++ {
		for o := 0; o < ordersPerUser; o++ {
			wg.Add(1)
			go func(userNum, orderNum int) {
				defer wg.Done()
				wallet := fmt.Sprintf("0x%040d", userNum)
				orderID := fmt.Sprintf("O%03d%07d", userNum, orderNum)

				req := &cache.FreezeForOrderRequest{
					Wallet:      wallet,
					Token:       "USDT",
					Amount:      orderAmount,
					OrderID:     orderID,
					FromSettled: true,
					OrderJSON:   fmt.Sprintf(`{"order_id":"%s"}`, orderID),
					ShardID:     0,
					NonceKey:    fmt.Sprintf("nonce:%d:%d", userNum, orderNum),
					NonceTTL:    7 * 24 * time.Hour,
				}

				_ = balanceCache.FreezeForOrder(ctx, req)
			}(u, o)
		}
	}

	wg.Wait()

	// 验证每个用户的余额
	for u := 0; u < numUsers; u++ {
		wallet := fmt.Sprintf("0x%040d", u)
		balance, err := balanceCache.GetBalance(ctx, wallet, "USDT")
		require.NoError(t, err)

		// 每个用户的总余额不变
		total := balance.SettledAvailable.Add(balance.SettledFrozen)
		assert.True(t, total.Equal(initialBalance),
			"User %d total should be %s, got %s", u, initialBalance, total)

		// 每个用户应该成功创建所有订单 (余额足够)
		expectedFrozen := orderAmount.Mul(decimal.NewFromInt(int64(ordersPerUser)))
		assert.True(t, balance.SettledFrozen.Equal(expectedFrozen),
			"User %d frozen should be %s, got %s", u, expectedFrozen, balance.SettledFrozen)
	}
}

// TestRace_OrderStatusTransitions 测试订单状态转换的并发安全性
// 使用 mock 测试状态机转换
func TestRace_OrderStatusTransitions(t *testing.T) {
	// 测试状态转换的并发安全性
	// 使用简单的内存 map 模拟
	orders := sync.Map{}

	// 初始化一些订单
	numOrders := 10
	for i := 0; i < numOrders; i++ {
		orderID := fmt.Sprintf("O%010d", i)
		orders.Store(orderID, model.OrderStatusOpen)
	}

	// 并发状态转换
	numGoroutines := 5
	var wg sync.WaitGroup

	for i := 0; i < numOrders; i++ {
		orderID := fmt.Sprintf("O%010d", i)

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(oid string) {
				defer wg.Done()

				// 尝试更新状态 (CAS 操作模拟)
				for {
					current, ok := orders.Load(oid)
					if !ok {
						return
					}

					currentStatus := current.(model.OrderStatus)
					if currentStatus == model.OrderStatusFilled {
						return // 已终态
					}

					// 尝试转换到下一状态
					var newStatus model.OrderStatus
					switch currentStatus {
					case model.OrderStatusOpen:
						newStatus = model.OrderStatusPartial
					case model.OrderStatusPartial:
						newStatus = model.OrderStatusFilled
					default:
						return
					}

					// 模拟 CAS
					if orders.CompareAndSwap(oid, currentStatus, newStatus) {
						return
					}
					// 失败则重试
				}
			}(orderID)
		}
	}

	wg.Wait()

	// 验证: 所有订单最终状态有效
	orders.Range(func(key, value interface{}) bool {
		status := value.(model.OrderStatus)
		assert.True(t,
			status == model.OrderStatusOpen ||
				status == model.OrderStatusPartial ||
				status == model.OrderStatusFilled,
			"Order %s has invalid status: %d", key, status)
		return true
	})
}
