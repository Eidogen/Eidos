package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSettlementProvider 测试用的模拟提供者
type MockSettlementProvider struct {
	mock.Mock
}

func (m *MockSettlementProvider) GetPendingTrades(ctx context.Context, limit int) ([]*PendingTrade, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*PendingTrade), args.Error(1)
}

func (m *MockSettlementProvider) CreateSettlementBatch(ctx context.Context, tradeIDs []string) (*SettlementBatch, error) {
	args := m.Called(ctx, tradeIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SettlementBatch), args.Error(1)
}

func (m *MockSettlementProvider) SubmitSettlementBatch(ctx context.Context, batchID string) error {
	args := m.Called(ctx, batchID)
	return args.Error(0)
}

func (m *MockSettlementProvider) GetTimedOutBatches(ctx context.Context, timeout time.Duration) ([]*SettlementBatch, error) {
	args := m.Called(ctx, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*SettlementBatch), args.Error(1)
}

func (m *MockSettlementProvider) RetrySettlementBatch(ctx context.Context, batchID string) error {
	args := m.Called(ctx, batchID)
	return args.Error(0)
}

func (m *MockSettlementProvider) MarkBatchFailed(ctx context.Context, batchID string, reason string) error {
	args := m.Called(ctx, batchID, reason)
	return args.Error(0)
}

func (m *MockSettlementProvider) GetLastBatchTime(ctx context.Context) (time.Time, error) {
	args := m.Called(ctx)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockSettlementProvider) GetPendingTradeCount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func TestSettlementTriggerJob_Execute_NoPendingTrades(t *testing.T) {
	provider := new(MockSettlementProvider)
	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return([]*SettlementBatch{}, nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(0, nil)

	job := NewSettlementTriggerJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ProcessedCount)
	assert.Equal(t, 0, result.AffectedCount)

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_Execute_WithPendingTrades(t *testing.T) {
	provider := new(MockSettlementProvider)

	trades := []*PendingTrade{
		{TradeID: "trade1", Market: "BTC-USDC"},
		{TradeID: "trade2", Market: "BTC-USDC"},
	}

	batch := &SettlementBatch{
		BatchID: "batch1",
		Market:  "BTC-USDC",
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return([]*SettlementBatch{}, nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(20, nil)
	provider.On("GetLastBatchTime", mock.Anything).Return(time.Now().Add(-10*time.Minute), nil)
	provider.On("GetPendingTrades", mock.Anything, 100).Return(trades, nil)
	provider.On("CreateSettlementBatch", mock.Anything, []string{"trade1", "trade2"}).Return(batch, nil)
	provider.On("SubmitSettlementBatch", mock.Anything, "batch1").Return(nil)

	job := NewSettlementTriggerJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ProcessedCount) // 1 batch created
	assert.Equal(t, 1, result.AffectedCount)  // 1 batch submitted

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_Execute_WithTimedOutBatches(t *testing.T) {
	provider := new(MockSettlementProvider)

	timedOutBatches := []*SettlementBatch{
		{BatchID: "batch1", RetryCount: 0},
		{BatchID: "batch2", RetryCount: 1},
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return(timedOutBatches, nil)
	provider.On("RetrySettlementBatch", mock.Anything, "batch1").Return(nil)
	provider.On("RetrySettlementBatch", mock.Anything, "batch2").Return(nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(0, nil)

	job := NewSettlementTriggerJob(provider, nil)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.Details["retry_count"])

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_Execute_BatchExceededMaxRetries(t *testing.T) {
	provider := new(MockSettlementProvider)

	config := &SettlementConfig{
		MaxRetries: 3,
	}

	timedOutBatches := []*SettlementBatch{
		{BatchID: "batch1", RetryCount: 3}, // Exceeded max retries
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return(timedOutBatches, nil)
	provider.On("MarkBatchFailed", mock.Anything, "batch1", "exceeded max retries").Return(nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(0, nil)

	job := NewSettlementTriggerJob(provider, config)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_Execute_WaitForMinBatchSize(t *testing.T) {
	provider := new(MockSettlementProvider)

	config := &SettlementConfig{
		MinBatchSize: 10,
		MaxWaitTime:  5 * time.Minute,
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return([]*SettlementBatch{}, nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(5, nil)             // Less than MinBatchSize
	provider.On("GetLastBatchTime", mock.Anything).Return(time.Now(), nil)        // Recent batch

	job := NewSettlementTriggerJob(provider, config)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ProcessedCount) // No batch created

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_Execute_ForceCreateAfterMaxWait(t *testing.T) {
	provider := new(MockSettlementProvider)

	config := &SettlementConfig{
		BatchSize:    100,
		MinBatchSize: 10,
		MaxWaitTime:  5 * time.Minute,
	}

	trades := []*PendingTrade{
		{TradeID: "trade1", Market: "BTC-USDC"},
	}

	batch := &SettlementBatch{
		BatchID: "batch1",
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return([]*SettlementBatch{}, nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(5, nil)                                  // Less than MinBatchSize
	provider.On("GetLastBatchTime", mock.Anything).Return(time.Now().Add(-10*time.Minute), nil)       // Old batch (>MaxWaitTime)
	provider.On("GetPendingTrades", mock.Anything, 100).Return(trades, nil)
	provider.On("CreateSettlementBatch", mock.Anything, []string{"trade1"}).Return(batch, nil)
	provider.On("SubmitSettlementBatch", mock.Anything, "batch1").Return(nil)

	job := NewSettlementTriggerJob(provider, config)
	ctx := context.Background()

	result, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ProcessedCount) // 1 batch created

	provider.AssertExpectations(t)
}

func TestSettlementTriggerJob_WithAlertFunc(t *testing.T) {
	provider := new(MockSettlementProvider)

	config := &SettlementConfig{
		BatchSize: 100,
	}

	var alertReceived *SettlementAlert

	// Simulate high pending count to trigger alert
	trades := make([]*PendingTrade, 0)
	for i := 0; i < 50; i++ {
		trades = append(trades, &PendingTrade{TradeID: "trade" + string(rune(i))})
	}

	provider.On("GetTimedOutBatches", mock.Anything, mock.Anything).Return([]*SettlementBatch{}, nil)
	provider.On("GetPendingTradeCount", mock.Anything).Return(400, nil) // High count
	provider.On("GetLastBatchTime", mock.Anything).Return(time.Now().Add(-10*time.Minute), nil)
	provider.On("GetPendingTrades", mock.Anything, 100).Return(trades[:50], nil)
	provider.On("CreateSettlementBatch", mock.Anything, mock.Anything).Return(&SettlementBatch{BatchID: "b1"}, nil)
	provider.On("SubmitSettlementBatch", mock.Anything, mock.Anything).Return(nil)

	job := NewSettlementTriggerJob(provider, config)
	job.SetAlertFunc(func(ctx context.Context, alert *SettlementAlert) error {
		alertReceived = alert
		return nil
	})

	ctx := context.Background()
	_, err := job.Execute(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, alertReceived)
	assert.Equal(t, "high_pending", alertReceived.AlertType)
}
