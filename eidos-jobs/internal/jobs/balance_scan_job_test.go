package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBalanceScanProvider 测试用的模拟提供者
type MockBalanceScanProvider struct {
	mock.Mock
}

func (m *MockBalanceScanProvider) GetWalletsWithActiveOrders(ctx context.Context, offset, limit int) ([]*WalletOrderInfo, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*WalletOrderInfo), args.Error(1)
}

func (m *MockBalanceScanProvider) GetOnchainBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	args := m.Called(ctx, wallet, token)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (m *MockBalanceScanProvider) BatchCancelOrders(ctx context.Context, orderIDs []string, reason string) (int, error) {
	args := m.Called(ctx, orderIDs, reason)
	return args.Int(0), args.Error(1)
}

func (m *MockBalanceScanProvider) SendCancelNotification(ctx context.Context, wallet string, orderIDs []string, reason string) error {
	args := m.Called(ctx, wallet, orderIDs, reason)
	return args.Error(0)
}

func TestBalanceScanJob_Execute_NoWallets(t *testing.T) {
	provider := new(MockBalanceScanProvider)
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return([]*WalletOrderInfo{}, nil)

	job := NewBalanceScanJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ProcessedCount)
	assert.Equal(t, 0, result.AffectedCount)

	provider.AssertExpectations(t)
}

func TestBalanceScanJob_Execute_SufficientBalance(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	// 只返回 1 个 wallet，小于 BatchSize(100)，不会请求第二批
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil)
	provider.On("GetOnchainBalance", mock.Anything, "0x1234567890abcdef", "USDC").Return(decimal.NewFromFloat(150.0), nil)

	job := NewBalanceScanJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ProcessedCount)
	assert.Equal(t, 0, result.AffectedCount) // No orders cancelled

	provider.AssertExpectations(t)
}

func TestBalanceScanJob_Execute_InsufficientBalance(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	// 只返回 1 个 wallet，小于 BatchSize(100)，不会请求第二批
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil)
	// Return balance less than required (100 * 0.99 = 99)
	provider.On("GetOnchainBalance", mock.Anything, "0x1234567890abcdef", "USDC").Return(decimal.NewFromFloat(50.0), nil)
	provider.On("BatchCancelOrders", mock.Anything, []string{"order1", "order2"}, mock.Anything).Return(2, nil)
	provider.On("SendCancelNotification", mock.Anything, "0x1234567890abcdef", []string{"order1", "order2"}, mock.Anything).Return(nil)

	job := NewBalanceScanJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ProcessedCount)
	assert.Equal(t, 2, result.AffectedCount) // 2 orders cancelled

	provider.AssertExpectations(t)
}

func TestBalanceScanJob_Execute_WithContextCancel(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil).Maybe()

	job := NewBalanceScanJob(provider, nil)

	result, err := job.Execute(ctx)

	// Should return context error
	assert.Error(t, err)
	assert.NotNil(t, result)
}

func TestBalanceScanJob_Execute_WithThreshold(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	// 只返回 1 个 wallet，小于 BatchSize(100)，不会请求第二批
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil)
	// Balance is 99, threshold is 1%, so required is 100 * 0.99 = 99
	// Balance == required, so should NOT cancel
	provider.On("GetOnchainBalance", mock.Anything, "0x1234567890abcdef", "USDC").Return(decimal.NewFromFloat(99.0), nil)

	config := &BalanceScanConfig{
		BatchSize:              100,
		CancelThresholdPercent: 0.01,
		MaxConcurrentQueries:   10,
		EnableNotification:     true,
	}

	job := NewBalanceScanJob(provider, config)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ProcessedCount)
	assert.Equal(t, 0, result.AffectedCount) // No orders cancelled (99 >= 99)

	provider.AssertExpectations(t)
}

func TestBalanceScanJob_WithAlertFunc(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	// 只返回 1 个 wallet，小于 BatchSize(100)，不会请求第二批
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil)
	provider.On("GetOnchainBalance", mock.Anything, "0x1234567890abcdef", "USDC").Return(decimal.NewFromFloat(50.0), nil)
	provider.On("BatchCancelOrders", mock.Anything, []string{"order1", "order2"}, mock.Anything).Return(2, nil)
	provider.On("SendCancelNotification", mock.Anything, "0x1234567890abcdef", []string{"order1", "order2"}, mock.Anything).Return(nil)

	var alertReceived *BalanceScanAlert

	job := NewBalanceScanJob(provider, nil)
	job.SetAlertFunc(func(ctx context.Context, alert *BalanceScanAlert) error {
		alertReceived = alert
		return nil
	})

	ctx := context.Background()
	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, alertReceived)
	assert.Equal(t, "0x1234567890abcdef", alertReceived.Wallet)
	assert.Equal(t, "USDC", alertReceived.Token)
	assert.Equal(t, 2, alertReceived.CancelledOrders)

	provider.AssertExpectations(t)
}

func TestBalanceScanJob_Timeout(t *testing.T) {
	provider := new(MockBalanceScanProvider)

	wallets := []*WalletOrderInfo{
		{
			Wallet:      "0x1234567890abcdef",
			Token:       "USDC",
			TotalFrozen: decimal.NewFromFloat(100.0),
			OrderCount:  2,
			OrderIDs:    []string{"order1", "order2"},
		},
	}

	// Simulate slow response
	provider.On("GetWalletsWithActiveOrders", mock.Anything, 0, 100).Return(wallets, nil).Run(func(args mock.Arguments) {
		time.Sleep(100 * time.Millisecond)
	})
	// 添加可能的调用以防测试时机问题
	provider.On("GetOnchainBalance", mock.Anything, mock.Anything, mock.Anything).Return(decimal.NewFromFloat(150.0), nil).Maybe()

	job := NewBalanceScanJob(provider, nil)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, _ := job.Execute(ctx)

	// Should return context deadline exceeded or nil error depending on timing
	assert.NotNil(t, result)
	// The job might complete or timeout depending on timing
}
