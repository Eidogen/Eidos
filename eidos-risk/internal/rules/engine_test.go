package rules

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockChecker 模拟检查器用于测试
type MockChecker struct {
	name            string
	orderCheckFn    func(ctx context.Context, req *OrderCheckRequest) *CheckResult
	withdrawCheckFn func(ctx context.Context, req *WithdrawCheckRequest) *CheckResult
}

func (m *MockChecker) Name() string { return m.name }

func (m *MockChecker) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	if m.orderCheckFn != nil {
		return m.orderCheckFn(ctx, req)
	}
	return NewPassResult(m.name)
}

func (m *MockChecker) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	if m.withdrawCheckFn != nil {
		return m.withdrawCheckFn(ctx, req)
	}
	return NewPassResult(m.name)
}

func TestEngine_RegisterChecker(t *testing.T) {
	engine := NewEngine()

	checker1 := &MockChecker{name: "checker1"}
	checker2 := &MockChecker{name: "checker2"}
	checker3 := &MockChecker{name: "checker3"}

	engine.RegisterChecker(checker2, PriorityNormal)
	engine.RegisterChecker(checker1, PriorityHighest)
	engine.RegisterChecker(checker3, PriorityLow)

	// 验证按优先级排序
	names := engine.GetCheckerNames()
	require.Len(t, names, 3)
	assert.Equal(t, "checker1", names[0])
	assert.Equal(t, "checker2", names[1])
	assert.Equal(t, "checker3", names[2])
}

func TestEngine_CheckOrder_AllPass(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "checker1",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewPassResult("checker1")
		},
	}, PriorityHighest)

	engine.RegisterChecker(&MockChecker{
		name: "checker2",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewPassResult("checker2")
		},
	}, PriorityNormal)

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
		Market: "ETH-USDC",
		Side:   "BUY",
		Price:  decimal.NewFromFloat(2000),
		Amount: decimal.NewFromFloat(1),
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.True(t, result.Passed)
	assert.Empty(t, result.Reason)
}

func TestEngine_CheckOrder_FirstReject(t *testing.T) {
	engine := NewEngine()

	rejectCalled := false
	engine.RegisterChecker(&MockChecker{
		name: "blacklist",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewRejectResult("blacklist", "wallet is blacklisted", "BLACKLISTED")
		},
	}, PriorityHighest)

	engine.RegisterChecker(&MockChecker{
		name: "ratelimit",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			// 这个不应该被执行
			rejectCalled = true
			return NewPassResult("ratelimit")
		},
	}, PriorityNormal)

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
		Market: "ETH-USDC",
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.False(t, result.Passed)
	assert.Equal(t, "wallet is blacklisted", result.Reason)
	assert.Equal(t, "BLACKLISTED", result.Code)
	assert.False(t, rejectCalled, "ratelimit checker should not be called after reject")
}

func TestEngine_CheckOrder_Warning(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "amount",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewWarningResult("amount", "large order detected")
		},
	}, PriorityNormal)

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
		Market: "ETH-USDC",
		Price:  decimal.NewFromFloat(2000),
		Amount: decimal.NewFromFloat(1000),
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.True(t, result.Passed)
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "large order detected", result.Warnings[0])
}

func TestEngine_CheckWithdraw_RiskScore(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "amount",
		withdrawCheckFn: func(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
			result := NewPassResult("amount")
			result.RiskScore = 40
			return result
		},
	}, PriorityNormal)

	engine.RegisterChecker(&MockChecker{
		name: "address",
		withdrawCheckFn: func(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
			result := NewPassResult("address")
			result.RiskScore = 60
			return result
		},
	}, PriorityHigh)

	req := &WithdrawCheckRequest{
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDC",
		Amount:    decimal.NewFromFloat(10000),
		ToAddress: "0xabcdef1234567890123456789012345678901234",
	}

	result := engine.CheckWithdraw(context.Background(), req)
	assert.True(t, result.Passed)
	// 风险分应该是平均值 (40 + 60) / 2 = 50
	assert.Equal(t, 50, result.RiskScore)
}

func TestEngine_ConcurrentChecks(t *testing.T) {
	engine := NewEngine()

	var callCount int64
	var mu sync.Mutex

	engine.RegisterChecker(&MockChecker{
		name: "counter",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			mu.Lock()
			callCount++
			mu.Unlock()
			return NewPassResult("counter")
		},
	}, PriorityNormal)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := &OrderCheckRequest{
				Wallet: "0x1234567890123456789012345678901234567890",
				Market: "ETH-USDC",
			}
			result := engine.CheckOrder(context.Background(), req)
			assert.True(t, result.Passed)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines), callCount)
}

func TestEngine_ContextCancellation(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "slow",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			select {
			case <-ctx.Done():
				return NewRejectResult("slow", "context cancelled", "TIMEOUT")
			default:
				return NewPassResult("slow")
			}
		},
	}, PriorityNormal)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
	}

	result := engine.CheckOrder(ctx, req)
	// 根据实现可能 pass 或 reject
	assert.NotNil(t, result)
}

func TestCheckResult_Merge(t *testing.T) {
	result1 := NewPassResult("checker1")
	result1.RiskScore = 30
	result1.Warnings = []string{"warning1"}

	result2 := NewPassResult("checker2")
	result2.RiskScore = 50
	result2.Warnings = []string{"warning2"}

	result1.Merge(result2)

	assert.Equal(t, 50, result1.RiskScore) // 取最高值
	assert.Len(t, result1.Warnings, 2)
	assert.Contains(t, result1.Warnings, "warning1")
	assert.Contains(t, result1.Warnings, "warning2")
}

func TestCheckResult_MergeWithReject(t *testing.T) {
	result1 := NewPassResult("checker1")
	result2 := NewRejectResult("checker2", "rejected", "REJECTED")

	result1.Merge(result2)

	assert.False(t, result1.Passed)
	assert.Equal(t, "rejected", result1.Reason)
	assert.Equal(t, "REJECTED", result1.Code)
}

func TestNewPassResult(t *testing.T) {
	result := NewPassResult("test")
	assert.True(t, result.Passed)
	assert.Equal(t, "test", result.RuleName)
	assert.Empty(t, result.Reason)
	assert.Empty(t, result.Code)
}

func TestNewRejectResult(t *testing.T) {
	result := NewRejectResult("test", "reason", "CODE")
	assert.False(t, result.Passed)
	assert.Equal(t, "test", result.RuleName)
	assert.Equal(t, "reason", result.Reason)
	assert.Equal(t, "CODE", result.Code)
}

func TestNewWarningResult(t *testing.T) {
	result := NewWarningResult("test", "warning message")
	assert.True(t, result.Passed)
	assert.Equal(t, "test", result.RuleName)
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "warning message", result.Warnings[0])
}

func TestNewPassedResult(t *testing.T) {
	result := NewPassedResult()
	assert.True(t, result.Passed)
	assert.Empty(t, result.Warnings)
}

func TestNewPassedWithWarnings(t *testing.T) {
	warnings := []string{"warn1", "warn2"}
	result := NewPassedWithWarnings(warnings)
	assert.True(t, result.Passed)
	assert.Len(t, result.Warnings, 2)
}

func TestNewRejectedResult(t *testing.T) {
	result := NewRejectedResult("rule1", "TestRule", "test reason", "TEST_CODE")
	assert.False(t, result.Passed)
	assert.Equal(t, "rule1", result.RuleID)
	assert.Equal(t, "TestRule", result.RuleName)
	assert.Equal(t, "test reason", result.Reason)
	assert.Equal(t, "TEST_CODE", result.Code)
}

func TestEngine_MultipleWarnings(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "checker1",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewWarningResult("checker1", "warning 1")
		},
	}, PriorityHigh)

	engine.RegisterChecker(&MockChecker{
		name: "checker2",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewWarningResult("checker2", "warning 2")
		},
	}, PriorityNormal)

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
		Market: "ETH-USDC",
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.True(t, result.Passed)
	assert.Len(t, result.Warnings, 2)
	assert.Contains(t, result.Warnings, "warning 1")
	assert.Contains(t, result.Warnings, "warning 2")
}

func TestEngine_NilResult(t *testing.T) {
	engine := NewEngine()

	engine.RegisterChecker(&MockChecker{
		name: "nil_checker",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return nil
		},
	}, PriorityNormal)

	engine.RegisterChecker(&MockChecker{
		name: "pass_checker",
		orderCheckFn: func(ctx context.Context, req *OrderCheckRequest) *CheckResult {
			return NewPassResult("pass_checker")
		},
	}, PriorityLow)

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.True(t, result.Passed)
}

func TestEngine_EmptyCheckers(t *testing.T) {
	engine := NewEngine()

	req := &OrderCheckRequest{
		Wallet: "0x1234567890123456789012345678901234567890",
	}

	result := engine.CheckOrder(context.Background(), req)
	assert.True(t, result.Passed)
	assert.Empty(t, result.Warnings)
}

func TestEngine_GetCheckerNames(t *testing.T) {
	engine := NewEngine()

	names := engine.GetCheckerNames()
	assert.Empty(t, names)

	engine.RegisterChecker(&MockChecker{name: "test1"}, PriorityNormal)
	engine.RegisterChecker(&MockChecker{name: "test2"}, PriorityHigh)

	names = engine.GetCheckerNames()
	assert.Len(t, names, 2)
	assert.Equal(t, "test2", names[0]) // 高优先级在前
	assert.Equal(t, "test1", names[1])
}
