package repository

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

var testDBCounter int64

// setupTestDB creates an in-memory SQLite database for testing
// Each call creates a unique database to ensure test isolation
func setupTestDB(t *testing.T) *gorm.DB {
	// Use unique database name for each test to ensure complete isolation
	counter := atomic.AddInt64(&testDBCounter, 1)
	dsn := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared", counter)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&model.Kline{}, &model.Market{}, &model.Trade{})
	require.NoError(t, err)

	return db
}

func TestKlineRepository_Upsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewKlineRepository(db)
	ctx := context.Background()

	t.Run("insert new kline", func(t *testing.T) {
		kline := &model.Kline{
			Market:      "BTC-USDC",
			Interval:    model.Interval1m,
			OpenTime:    1700000000000,
			Open:        decimal.NewFromFloat(50000),
			High:        decimal.NewFromFloat(50100),
			Low:         decimal.NewFromFloat(49900),
			Close:       decimal.NewFromFloat(50050),
			Volume:      decimal.NewFromFloat(100),
			QuoteVolume: decimal.NewFromFloat(5000000),
			TradeCount:  50,
		}

		err := repo.Upsert(ctx, kline)
		require.NoError(t, err)

		// Verify insert
		result, err := repo.GetLatest(ctx, "BTC-USDC", model.Interval1m)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(1700000000000), result.OpenTime)
		assert.True(t, result.Open.Equal(decimal.NewFromFloat(50000)))
	})

	t.Run("update existing kline", func(t *testing.T) {
		// Insert first
		kline := &model.Kline{
			Market:      "ETH-USDC",
			Interval:    model.Interval1m,
			OpenTime:    1700000000000,
			Open:        decimal.NewFromFloat(3000),
			High:        decimal.NewFromFloat(3010),
			Low:         decimal.NewFromFloat(2990),
			Close:       decimal.NewFromFloat(3005),
			Volume:      decimal.NewFromFloat(50),
			QuoteVolume: decimal.NewFromFloat(150000),
			TradeCount:  20,
		}
		err := repo.Upsert(ctx, kline)
		require.NoError(t, err)

		// Update with same key
		kline.Close = decimal.NewFromFloat(3020)
		kline.TradeCount = 25
		err = repo.Upsert(ctx, kline)
		require.NoError(t, err)

		// Verify update
		result, err := repo.GetLatest(ctx, "ETH-USDC", model.Interval1m)
		require.NoError(t, err)
		assert.True(t, result.Close.Equal(decimal.NewFromFloat(3020)))
		assert.Equal(t, int32(25), result.TradeCount)
	})
}

func TestKlineRepository_BatchUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewKlineRepository(db)
	ctx := context.Background()

	t.Run("batch insert multiple klines", func(t *testing.T) {
		klines := make([]*model.Kline, 5)
		for i := 0; i < 5; i++ {
			klines[i] = &model.Kline{
				Market:      "BTC-USDC",
				Interval:    model.Interval5m,
				OpenTime:    int64(1700000000000 + i*300000),
				Open:        decimal.NewFromFloat(50000 + float64(i*100)),
				High:        decimal.NewFromFloat(50050 + float64(i*100)),
				Low:         decimal.NewFromFloat(49950 + float64(i*100)),
				Close:       decimal.NewFromFloat(50000 + float64(i*100)),
				Volume:      decimal.NewFromFloat(10),
				QuoteVolume: decimal.NewFromFloat(500000),
				TradeCount:  10,
			}
		}

		err := repo.BatchUpsert(ctx, klines)
		require.NoError(t, err)

		// Verify all were inserted
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval5m, 0, 0, 100)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("batch upsert empty slice", func(t *testing.T) {
		err := repo.BatchUpsert(ctx, []*model.Kline{})
		require.NoError(t, err)
	})
}

func TestKlineRepository_Query(t *testing.T) {
	db := setupTestDB(t)
	repo := NewKlineRepository(db)
	ctx := context.Background()

	// Insert test data
	baseTime := int64(1700000000000)
	for i := 0; i < 10; i++ {
		kline := &model.Kline{
			Market:      "BTC-USDC",
			Interval:    model.Interval1m,
			OpenTime:    baseTime + int64(i*60000),
			Open:        decimal.NewFromFloat(50000),
			High:        decimal.NewFromFloat(50100),
			Low:         decimal.NewFromFloat(49900),
			Close:       decimal.NewFromFloat(50050),
			Volume:      decimal.NewFromFloat(10),
			QuoteVolume: decimal.NewFromFloat(500000),
			TradeCount:  5,
		}
		_ = repo.Upsert(ctx, kline)
	}

	t.Run("query all", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, 0, 0, 0)
		require.NoError(t, err)
		assert.Len(t, result, 10)
		// Should be in ascending order
		assert.True(t, result[0].OpenTime < result[len(result)-1].OpenTime)
	})

	t.Run("query with start time", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, baseTime+300000, 0, 0)
		require.NoError(t, err)
		assert.Len(t, result, 5) // Should skip first 5
	})

	t.Run("query with end time", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, 0, baseTime+300000, 0)
		require.NoError(t, err)
		assert.Len(t, result, 6) // Should include first 6
	})

	t.Run("query with time range", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, baseTime+120000, baseTime+420000, 0)
		require.NoError(t, err)
		assert.Len(t, result, 6)
	})

	t.Run("query with limit", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, 0, 0, 5)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("query with limit exceeding max", func(t *testing.T) {
		result, err := repo.Query(ctx, "BTC-USDC", model.Interval1m, 0, 0, 2000)
		require.NoError(t, err)
		assert.Len(t, result, 10) // Should be capped at 1500, but we only have 10
	})

	t.Run("query unknown market", func(t *testing.T) {
		result, err := repo.Query(ctx, "UNKNOWN-USDC", model.Interval1m, 0, 0, 0)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestKlineRepository_GetLatest(t *testing.T) {
	db := setupTestDB(t)
	repo := NewKlineRepository(db)
	ctx := context.Background()

	t.Run("get latest when exists", func(t *testing.T) {
		// Insert multiple klines
		for i := 0; i < 3; i++ {
			kline := &model.Kline{
				Market:      "BTC-USDC",
				Interval:    model.Interval1h,
				OpenTime:    int64(1700000000000 + i*3600000),
				Open:        decimal.NewFromFloat(50000),
				High:        decimal.NewFromFloat(50100),
				Low:         decimal.NewFromFloat(49900),
				Close:       decimal.NewFromFloat(50050),
				Volume:      decimal.NewFromFloat(100),
				QuoteVolume: decimal.NewFromFloat(5000000),
				TradeCount:  50,
			}
			_ = repo.Upsert(ctx, kline)
		}

		result, err := repo.GetLatest(ctx, "BTC-USDC", model.Interval1h)
		require.NoError(t, err)
		require.NotNil(t, result)
		// Should be the latest one
		assert.Equal(t, int64(1700000000000+2*3600000), result.OpenTime)
	})

	t.Run("get latest when not exists", func(t *testing.T) {
		result, err := repo.GetLatest(ctx, "UNKNOWN-USDC", model.Interval1h)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestKlineRepository_CleanOldKlines(t *testing.T) {
	db := setupTestDB(t)
	repo := NewKlineRepository(db).(*klineRepository)
	ctx := context.Background()

	// Insert test data
	baseTime := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 5; i++ {
		kline := &model.Kline{
			Market:      "BTC-USDC",
			Interval:    model.Interval1m,
			OpenTime:    baseTime.Add(time.Duration(i) * time.Hour).UnixMilli(),
			Open:        decimal.NewFromFloat(50000),
			High:        decimal.NewFromFloat(50100),
			Low:         decimal.NewFromFloat(49900),
			Close:       decimal.NewFromFloat(50050),
			Volume:      decimal.NewFromFloat(10),
			QuoteVolume: decimal.NewFromFloat(500000),
			TradeCount:  5,
		}
		_ = repo.Upsert(ctx, kline)
	}

	t.Run("clean old klines", func(t *testing.T) {
		// Clean klines older than 24 hours ago
		cutoffTime := time.Now().Add(-24 * time.Hour)
		deleted, err := repo.CleanOldKlines(ctx, model.Interval1m, cutoffTime)
		require.NoError(t, err)
		assert.Greater(t, deleted, int64(0))
	})
}
