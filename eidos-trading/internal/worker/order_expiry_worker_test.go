package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockOrderExpirer 订单过期处理器 Mock
type MockOrderExpirer struct {
	mock.Mock
	mu      sync.Mutex
	expired []string
}

func (m *MockOrderExpirer) ExpireOrder(ctx context.Context, orderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expired = append(m.expired, orderID)
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockOrderExpirer) GetExpired() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.expired...)
}

// ========== Config Tests ==========

func TestDefaultOrderExpiryWorkerConfig(t *testing.T) {
	cfg := DefaultOrderExpiryWorkerConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 30*time.Second, cfg.CheckInterval)
	assert.Equal(t, 100, cfg.BatchSize)
}

func TestOrderExpiryWorkerConfig_Custom(t *testing.T) {
	cfg := &OrderExpiryWorkerConfig{
		CheckInterval: 10 * time.Second,
		BatchSize:     50,
	}

	assert.Equal(t, 10*time.Second, cfg.CheckInterval)
	assert.Equal(t, 50, cfg.BatchSize)
}
