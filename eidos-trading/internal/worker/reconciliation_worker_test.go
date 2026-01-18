package worker

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// ========== Config Tests ==========

func TestDefaultReconciliationWorkerConfig(t *testing.T) {
	cfg := DefaultReconciliationWorkerConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 5*time.Minute, cfg.CheckInterval)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.True(t, cfg.Threshold.Equal(decimal.NewFromFloat(0.000001)))
}

func TestReconciliationWorkerConfig_Custom(t *testing.T) {
	cfg := &ReconciliationWorkerConfig{
		CheckInterval: 1 * time.Minute,
		BatchSize:     50,
		Threshold:     decimal.NewFromFloat(0.01),
	}

	assert.Equal(t, 1*time.Minute, cfg.CheckInterval)
	assert.Equal(t, 50, cfg.BatchSize)
	assert.True(t, cfg.Threshold.Equal(decimal.NewFromFloat(0.01)))
}

// ========== Lock Tests ==========

func TestReconciliationWorker_LockKey(t *testing.T) {
	assert.Equal(t, "eidos:trading:reconciliation:lock", reconciliationLockKey)
	assert.Equal(t, 30*time.Second, reconciliationLockTTL)
}

func TestReconciliationWorker_TryAcquireLock_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg := DefaultReconciliationWorkerConfig()

	worker := &ReconciliationWorker{
		cfg: cfg,
		rdb: rdb,
	}

	ctx := context.Background()

	// First acquire should succeed
	got := worker.tryAcquireLock(ctx)
	assert.True(t, got)

	// Verify lock exists in Redis
	exists, err := rdb.Exists(ctx, reconciliationLockKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}

func TestReconciliationWorker_TryAcquireLock_AlreadyHeld(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg := DefaultReconciliationWorkerConfig()

	worker := &ReconciliationWorker{
		cfg: cfg,
		rdb: rdb,
	}

	ctx := context.Background()

	// Manually set the lock to simulate another instance holding it
	err := rdb.SetNX(ctx, reconciliationLockKey, "locked", reconciliationLockTTL).Err()
	assert.NoError(t, err)

	// Second acquire should fail (lock held)
	got := worker.tryAcquireLock(ctx)
	assert.False(t, got)
}

func TestReconciliationWorker_ReleaseLock(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg := DefaultReconciliationWorkerConfig()

	worker := &ReconciliationWorker{
		cfg: cfg,
		rdb: rdb,
	}

	ctx := context.Background()

	// Acquire lock first
	worker.tryAcquireLock(ctx)

	// Release lock
	worker.releaseLock(ctx)

	// Verify lock is gone
	exists, err := rdb.Exists(ctx, reconciliationLockKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestReconciliationWorker_LockAcquireAfterRelease(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg := DefaultReconciliationWorkerConfig()

	worker := &ReconciliationWorker{
		cfg: cfg,
		rdb: rdb,
	}

	ctx := context.Background()

	// Acquire lock
	got := worker.tryAcquireLock(ctx)
	assert.True(t, got)

	// Release lock
	worker.releaseLock(ctx)

	// Acquire again should succeed
	got = worker.tryAcquireLock(ctx)
	assert.True(t, got)
}

// ========== isEqual Tests ==========

func TestReconciliationWorker_isEqual(t *testing.T) {
	cfg := &ReconciliationWorkerConfig{
		Threshold: decimal.NewFromFloat(0.0001),
	}

	worker := &ReconciliationWorker{
		cfg: cfg,
	}

	tests := []struct {
		name     string
		a        decimal.Decimal
		b        decimal.Decimal
		expected bool
	}{
		{
			name:     "exactly equal",
			a:        decimal.NewFromFloat(100.0),
			b:        decimal.NewFromFloat(100.0),
			expected: true,
		},
		{
			name:     "within threshold",
			a:        decimal.NewFromFloat(100.0),
			b:        decimal.NewFromFloat(100.00005),
			expected: true,
		},
		{
			name:     "at threshold boundary",
			a:        decimal.NewFromFloat(100.0),
			b:        decimal.NewFromFloat(100.0001),
			expected: true,
		},
		{
			name:     "outside threshold",
			a:        decimal.NewFromFloat(100.0),
			b:        decimal.NewFromFloat(100.001),
			expected: false,
		},
		{
			name:     "negative difference within threshold",
			a:        decimal.NewFromFloat(100.00005),
			b:        decimal.NewFromFloat(100.0),
			expected: true,
		},
		{
			name:     "large numbers equal",
			a:        decimal.NewFromFloat(1000000.123456),
			b:        decimal.NewFromFloat(1000000.123456),
			expected: true,
		},
		{
			name:     "zero values",
			a:        decimal.Zero,
			b:        decimal.Zero,
			expected: true,
		},
		{
			name:     "zero vs small",
			a:        decimal.Zero,
			b:        decimal.NewFromFloat(0.00005),
			expected: true,
		},
		{
			name:     "zero vs outside threshold",
			a:        decimal.Zero,
			b:        decimal.NewFromFloat(0.001),
			expected: false,
		},
		{
			name:     "negative values equal",
			a:        decimal.NewFromFloat(-100.0),
			b:        decimal.NewFromFloat(-100.0),
			expected: true,
		},
		{
			name:     "negative values within threshold",
			a:        decimal.NewFromFloat(-100.0),
			b:        decimal.NewFromFloat(-100.00005),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := worker.isEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReconciliationWorker_isEqual_DifferentThresholds(t *testing.T) {
	tests := []struct {
		name      string
		threshold decimal.Decimal
		a         decimal.Decimal
		b         decimal.Decimal
		expected  bool
	}{
		{
			name:      "tight threshold - within",
			threshold: decimal.NewFromFloat(0.000001),
			a:         decimal.NewFromFloat(100.0),
			b:         decimal.NewFromFloat(100.0000005),
			expected:  true,
		},
		{
			name:      "tight threshold - outside",
			threshold: decimal.NewFromFloat(0.000001),
			a:         decimal.NewFromFloat(100.0),
			b:         decimal.NewFromFloat(100.00001),
			expected:  false,
		},
		{
			name:      "loose threshold - within",
			threshold: decimal.NewFromFloat(1.0),
			a:         decimal.NewFromFloat(100.0),
			b:         decimal.NewFromFloat(100.5),
			expected:  true,
		},
		{
			name:      "loose threshold - outside",
			threshold: decimal.NewFromFloat(1.0),
			a:         decimal.NewFromFloat(100.0),
			b:         decimal.NewFromFloat(102.0),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := &ReconciliationWorker{
				cfg: &ReconciliationWorkerConfig{
					Threshold: tt.threshold,
				},
			}
			result := worker.isEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}
