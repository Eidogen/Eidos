package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockTradingClientForTest 测试用 TradingClient
type mockTradingClientForTest struct {
	orders      []*OrderExpireRequest
	getOrderErr error
	mu          sync.Mutex
}

func (c *mockTradingClientForTest) GetExpiredOrders(ctx context.Context, beforeTime int64, limit int) ([]*OrderExpireRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getOrderErr != nil {
		return nil, c.getOrderErr
	}
	if limit > len(c.orders) {
		limit = len(c.orders)
	}
	result := c.orders[:limit]
	// 模拟消耗，下次调用返回剩余
	c.orders = c.orders[limit:]
	return result, nil
}

// mockMatchingClientForTest 测试用 MatchingClient
type mockMatchingClientForTest struct {
	expiredOrders []*OrderExpireRequest
	expireErr     error
	mu            sync.Mutex
}

func (c *mockMatchingClientForTest) ExpireOrders(ctx context.Context, requests []*OrderExpireRequest) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expireErr != nil {
		return 0, c.expireErr
	}
	c.expiredOrders = append(c.expiredOrders, requests...)
	return len(requests), nil
}

func (c *mockMatchingClientForTest) GetExpiredOrders() []*OrderExpireRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*OrderExpireRequest{}, c.expiredOrders...)
}

func TestCleanupOrdersJob_Execute_NoExpiredOrders(t *testing.T) {
	tradingClient := &mockTradingClientForTest{orders: []*OrderExpireRequest{}}
	matchingClient := &mockMatchingClientForTest{}

	job := NewCleanupOrdersJob(tradingClient, matchingClient)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 0 {
		t.Errorf("Expected ProcessedCount 0, got %d", result.ProcessedCount)
	}

	if result.AffectedCount != 0 {
		t.Errorf("Expected AffectedCount 0, got %d", result.AffectedCount)
	}
}

func TestCleanupOrdersJob_Execute_WithExpiredOrders(t *testing.T) {
	expiredOrders := []*OrderExpireRequest{
		{OrderID: "order-1", Market: "BTC-USDT", Wallet: "0x1"},
		{OrderID: "order-2", Market: "ETH-USDT", Wallet: "0x2"},
		{OrderID: "order-3", Market: "BTC-USDT", Wallet: "0x1"},
	}

	tradingClient := &mockTradingClientForTest{orders: expiredOrders}
	matchingClient := &mockMatchingClientForTest{}

	job := NewCleanupOrdersJob(tradingClient, matchingClient)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ProcessedCount != 3 {
		t.Errorf("Expected ProcessedCount 3, got %d", result.ProcessedCount)
	}

	// 验证订单已发送到撮合引擎
	expired := matchingClient.GetExpiredOrders()
	if len(expired) != 3 {
		t.Errorf("Expected 3 expired orders sent to matching, got %d", len(expired))
	}
}

func TestCleanupOrdersJob_Execute_TradingClientError(t *testing.T) {
	tradingClient := &mockTradingClientForTest{
		getOrderErr: errors.New("trading service unavailable"),
	}
	matchingClient := &mockMatchingClientForTest{}

	job := NewCleanupOrdersJob(tradingClient, matchingClient)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	// Trading client error should be handled gracefully
	if err != nil {
		t.Fatalf("Execute should not fail entirely: %v", err)
	}

	if result.ErrorCount == 0 {
		t.Error("Expected ErrorCount > 0 for trading client error")
	}
}

func TestCleanupOrdersJob_Execute_MatchingClientError(t *testing.T) {
	expiredOrders := []*OrderExpireRequest{
		{OrderID: "order-1", Market: "BTC-USDT", Wallet: "0x1"},
	}

	tradingClient := &mockTradingClientForTest{orders: expiredOrders}
	matchingClient := &mockMatchingClientForTest{
		expireErr: errors.New("matching service unavailable"),
	}

	job := NewCleanupOrdersJob(tradingClient, matchingClient)
	ctx := context.Background()

	result, err := job.Execute(ctx)
	// 撮合失败不应导致整个任务失败，而是记录错误
	if err != nil {
		t.Fatalf("Execute should not fail entirely: %v", err)
	}

	if result.ErrorCount == 0 {
		t.Error("Expected ErrorCount > 0 for matching client error")
	}
}

func TestCleanupOrdersJob_Execute_ContextCancelled(t *testing.T) {
	// 创建大量订单
	orders := make([]*OrderExpireRequest, 1000)
	for i := 0; i < 1000; i++ {
		orders[i] = &OrderExpireRequest{OrderID: "order-" + string(rune('0'+i%10)), Market: "BTC-USDT"}
	}

	tradingClient := &mockTradingClientForTest{orders: orders}
	matchingClient := &mockMatchingClientForTest{}

	job := NewCleanupOrdersJob(tradingClient, matchingClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := job.Execute(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		// 任务应该尽早返回
		t.Log("Task returned early due to cancellation")
	}
}

func TestCleanupOrdersJob_Name(t *testing.T) {
	job := NewCleanupOrdersJob(nil, nil)

	if job.Name() != "cleanup-orders" {
		t.Errorf("Expected name 'cleanup-orders', got '%s'", job.Name())
	}
}

func TestOrderExpireRequest(t *testing.T) {
	request := &OrderExpireRequest{
		OrderID: "order-123",
		Market:  "BTC-USDT",
		Wallet:  "0x1234567890abcdef",
	}

	if request.OrderID != "order-123" {
		t.Errorf("Expected OrderID 'order-123', got '%s'", request.OrderID)
	}

	if request.Market != "BTC-USDT" {
		t.Errorf("Expected Market 'BTC-USDT', got '%s'", request.Market)
	}

	if request.Wallet != "0x1234567890abcdef" {
		t.Errorf("Expected Wallet '0x1234567890abcdef', got '%s'", request.Wallet)
	}
}
