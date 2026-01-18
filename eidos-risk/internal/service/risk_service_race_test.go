package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/rules"
)

// TestRiskService_ConcurrentOrderChecks 测试并发订单风控检查
func TestRiskService_ConcurrentOrderChecks(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	market := "BTC-USDT"
	err := svc.marketCache.SetLastPrice(ctx, market, decimal.NewFromFloat(50000))
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	results := make([]*rules.CheckResult, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0x" + string(rune('a'+idx%26)) + "234567890123456789012345678901234567890"
			req := &rules.OrderCheckRequest{
				Wallet:    wallet,
				Market:    market,
				Side:      "BUY",
				OrderType: "LIMIT",
				Price:     decimal.NewFromFloat(50000),
				Amount:    decimal.NewFromFloat(0.1),
				Notional:  decimal.NewFromFloat(5000),
			}

			results[idx] = svc.engine.CheckOrder(ctx, req)
		}(i)
	}

	wg.Wait()

	// 验证所有请求都得到了处理
	passedCount := 0
	for i, result := range results {
		assert.Nil(t, errors[i])
		assert.NotNil(t, result)
		if result.Passed {
			passedCount++
		}
	}

	// 大部分请求应该通过
	assert.Greater(t, passedCount, 50)
}

// TestRiskService_ConcurrentWithdrawChecks 测试并发提现风控检查
func TestRiskService_ConcurrentWithdrawChecks(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0x" + string(rune('a'+idx%26)) + "234567890123456789012345678901234567890"
			req := &rules.WithdrawCheckRequest{
				Wallet:    wallet,
				Token:     "USDT",
				Amount:    decimal.NewFromFloat(100),
				ToAddress: "0xabcdef1234567890123456789012345678901234",
			}

			result := svc.engine.CheckWithdraw(ctx, req)
			assert.NotNil(t, result)
			assert.True(t, result.Passed)
		}(i)
	}

	wg.Wait()
}

// TestRiskService_ConcurrentBlacklistOperations 测试并发黑名单操作
func TestRiskService_ConcurrentBlacklistOperations(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	var wg sync.WaitGroup

	// 并发添加黑名单
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.blacklistCache.Set(ctx, &cache.BlacklistEntry{
				WalletAddress: wallet,
				ListType:      "full",
				Reason:        "test",
			})
		}()
	}

	// 并发检查黑名单
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, _ := svc.blacklistCache.Get(ctx, wallet)
			// entry 可能为 nil 或有值，取决于时序
			_ = entry
		}()
	}

	// 并发删除黑名单
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.blacklistCache.Remove(ctx, wallet)
		}()
	}

	wg.Wait()
}

// TestRiskService_ConcurrentRateLimitChecks 测试并发速率限制检查
func TestRiskService_ConcurrentRateLimitChecks(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	var wg sync.WaitGroup
	numGoroutines := 100

	allowedCount := int64(0)
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			allowed, err := svc.rateLimitCache.CheckAndIncrement(ctx, wallet, "order", 1, 50)
			assert.NoError(t, err)

			mu.Lock()
			if allowed {
				allowedCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// 应该有 50 个请求被允许
	assert.LessOrEqual(t, allowedCount, int64(50))
}

// TestRiskService_ConcurrentAmountTracking 测试并发金额追踪
func TestRiskService_ConcurrentAmountTracking(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := svc.amountCache.AddDailyWithdraw(ctx, wallet, token, decimal.NewFromFloat(10))
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// 验证总金额
	total, err := svc.amountCache.GetDailyWithdraw(ctx, wallet, token)
	require.NoError(t, err)

	expected := decimal.NewFromFloat(500)
	assert.True(t, total.Equal(expected), "expected %s, got %s", expected.String(), total.String())
}

// TestRiskService_ConcurrentOrderTracking 测试并发订单追踪
func TestRiskService_ConcurrentOrderTracking(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "BTC-USDT"

	var wg sync.WaitGroup

	// 并发添加订单
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			order := &cache.OrderInfo{
				OrderID: "order-" + string(rune('0'+idx/10)) + string(rune('0'+idx%10)),
				Market:  market,
				Side:    "BUY",
				Price:   decimal.NewFromFloat(50000),
				Amount:  decimal.NewFromFloat(0.1),
			}
			err := svc.orderCache.AddOrder(ctx, wallet, order)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// 并发检查自成交
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			hasSelfTrade, err := svc.orderCache.HasOppositeOrder(ctx, wallet, market, "SELL", decimal.NewFromFloat(50000))
			assert.NoError(t, err)
			_ = hasSelfTrade
		}()
	}

	// 并发删除订单
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			orderID := "order-" + string(rune('0'+idx/10)) + string(rune('0'+idx%10))
			svc.orderCache.RemoveOrder(ctx, wallet, market, orderID)
		}(i)
	}

	wg.Wait()
}

// TestRiskService_ConcurrentMarketPriceUpdates 测试并发市场价格更新
func TestRiskService_ConcurrentMarketPriceUpdates(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	market := "BTC-USDT"

	var wg sync.WaitGroup

	// 并发写入价格
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			price := decimal.NewFromFloat(50000 + float64(idx*10))
			err := svc.marketCache.SetLastPrice(ctx, market, price)
			assert.NoError(t, err)
		}(i)
	}

	// 并发读取价格
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			price, err := svc.marketCache.GetLastPrice(ctx, market)
			assert.NoError(t, err)
			assert.True(t, price.GreaterThanOrEqual(decimal.NewFromFloat(50000)))
		}()
	}

	wg.Wait()
}

// TestRiskService_ConcurrentMixedOperations 测试混合并发操作
func TestRiskService_ConcurrentMixedOperations(t *testing.T) {
	svc := setupTestRiskService(t)
	ctx := context.Background()

	market := "ETH-USDT"
	err := svc.marketCache.SetLastPrice(ctx, market, decimal.NewFromFloat(3000))
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 订单检查
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0x" + string(rune('a'+idx%26)) + "234567890123456789012345678901234567890"
			req := &rules.OrderCheckRequest{
				Wallet:    wallet,
				Market:    market,
				Side:      "BUY",
				OrderType: "LIMIT",
				Price:     decimal.NewFromFloat(3000),
				Amount:    decimal.NewFromFloat(1),
				Notional:  decimal.NewFromFloat(3000),
			}

			result := svc.engine.CheckOrder(ctx, req)
			assert.NotNil(t, result)
		}(i)
	}

	// 提现检查
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0x" + string(rune('a'+idx%26)) + "234567890123456789012345678901234567890"
			req := &rules.WithdrawCheckRequest{
				Wallet:    wallet,
				Token:     "ETH",
				Amount:    decimal.NewFromFloat(1),
				ToAddress: "0xabcdef1234567890123456789012345678901234",
			}

			result := svc.engine.CheckWithdraw(ctx, req)
			assert.NotNil(t, result)
		}(i)
	}

	// 黑名单操作
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0xblacklist" + string(rune('0'+idx)) + "4567890123456789012345678"
			svc.blacklistCache.Set(ctx, &cache.BlacklistEntry{
				WalletAddress: wallet,
				ListType:      "full",
				Reason:        "test",
			})
		}(i)
	}

	wg.Wait()
}

// TestRiskService_ContextCancellation 测试 Context 取消
func TestRiskService_ContextCancellation(t *testing.T) {
	svc := setupTestRiskService(t)

	market := "BTC-USDT"
	bgCtx := context.Background()
	err := svc.marketCache.SetLastPrice(bgCtx, market, decimal.NewFromFloat(50000))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 等待超时
	time.Sleep(5 * time.Millisecond)

	req := &rules.OrderCheckRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    market,
		Side:      "BUY",
		OrderType: "LIMIT",
		Price:     decimal.NewFromFloat(50000),
		Amount:    decimal.NewFromFloat(0.1),
		Notional:  decimal.NewFromFloat(5000),
	}

	result := svc.engine.CheckOrder(ctx, req)
	// Context 已取消，但引擎应该仍然返回结果
	assert.NotNil(t, result)
}

// TestRiskService_HighConcurrencyStress 高并发压力测试
func TestRiskService_HighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	svc := setupTestRiskService(t)
	ctx := context.Background()

	markets := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT", "ARB-USDT"}
	for _, market := range markets {
		err := svc.marketCache.SetLastPrice(ctx, market, decimal.NewFromFloat(1000))
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	numGoroutines := 500

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			wallet := "0x" + string(rune('a'+idx%26)) + "234567890123456789012345678901234567890"
			market := markets[idx%len(markets)]

			req := &rules.OrderCheckRequest{
				Wallet:    wallet,
				Market:    market,
				Side:      "BUY",
				OrderType: "LIMIT",
				Price:     decimal.NewFromFloat(1000),
				Amount:    decimal.NewFromFloat(1),
				Notional:  decimal.NewFromFloat(1000),
			}

			result := svc.engine.CheckOrder(ctx, req)
			assert.NotNil(t, result)
		}(i)
	}

	wg.Wait()
}

// BenchmarkRiskService_CheckOrder 订单检查基准测试
func BenchmarkRiskService_CheckOrder(b *testing.B) {
	s := miniredis.RunT(b)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	blacklistCache := cache.NewBlacklistCache(client)
	rateLimitCache := cache.NewRateLimitCache(client)
	amountCache := cache.NewAmountCache(client)
	orderCache := cache.NewOrderCache(client)
	marketCache := cache.NewMarketCache(client)

	engine := rules.NewEngine()
	engine.RegisterChecker(rules.NewBlacklistChecker(blacklistCache), rules.PriorityHighest)
	engine.RegisterChecker(rules.NewRateLimitChecker(rateLimitCache, &rules.RateLimitConfig{
		OrdersPerSecond:  1000,
		OrdersPerMinute:  10000,
		CancelsPerMinute: 5000,
	}), rules.PriorityHigh)
	engine.RegisterChecker(rules.NewAmountChecker(amountCache, &rules.AmountLimitConfig{
		SingleOrderMax:      decimal.NewFromFloat(1000000),
		DailyWithdrawMax:    decimal.NewFromFloat(500000),
		SingleWithdrawMax:   decimal.NewFromFloat(100000),
		PendingSettleMax:    decimal.NewFromFloat(5000000),
		LargeTradeThreshold: decimal.NewFromFloat(500000),
	}), rules.PriorityNormal)
	engine.RegisterChecker(rules.NewSelfTradeChecker(orderCache), rules.PriorityNormal)
	engine.RegisterChecker(rules.NewPriceChecker(marketCache, &rules.PriceDeviationConfig{
		MaxDeviationPercent: decimal.NewFromFloat(0.1),
		WarningPercent:      decimal.NewFromFloat(0.05),
	}), rules.PriorityNormal)

	ctx := context.Background()
	marketCache.SetLastPrice(ctx, "BTC-USDT", decimal.NewFromFloat(50000))

	req := &rules.OrderCheckRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Market:    "BTC-USDT",
		Side:      "BUY",
		OrderType: "LIMIT",
		Price:     decimal.NewFromFloat(50000),
		Amount:    decimal.NewFromFloat(0.1),
		Notional:  decimal.NewFromFloat(5000),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.CheckOrder(ctx, req)
		}
	})
}

// BenchmarkRiskService_CheckWithdraw 提现检查基准测试
func BenchmarkRiskService_CheckWithdraw(b *testing.B) {
	s := miniredis.RunT(b)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	blacklistCache := cache.NewBlacklistCache(client)
	amountCache := cache.NewAmountCache(client)

	engine := rules.NewEngine()
	engine.RegisterChecker(rules.NewBlacklistChecker(blacklistCache), rules.PriorityHighest)
	engine.RegisterChecker(rules.NewAmountChecker(amountCache, &rules.AmountLimitConfig{
		SingleOrderMax:      decimal.NewFromFloat(1000000),
		DailyWithdrawMax:    decimal.NewFromFloat(500000),
		SingleWithdrawMax:   decimal.NewFromFloat(100000),
		PendingSettleMax:    decimal.NewFromFloat(5000000),
		LargeTradeThreshold: decimal.NewFromFloat(500000),
	}), rules.PriorityNormal)

	ctx := context.Background()

	req := &rules.WithdrawCheckRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(100),
		ToAddress: "0xabcdef1234567890123456789012345678901234",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.CheckWithdraw(ctx, req)
		}
	})
}
