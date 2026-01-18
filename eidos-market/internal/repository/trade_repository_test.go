package repository

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

func TestTradeRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTradeRepository(db)
	ctx := context.Background()

	t.Run("create trade", func(t *testing.T) {
		trade := &model.Trade{
			TradeID:     "trade-001",
			Market:      "BTC-USDC",
			Price:       decimal.NewFromFloat(50000),
			Amount:      decimal.NewFromFloat(1.5),
			QuoteAmount: decimal.NewFromFloat(75000),
			Side:        model.TradeSideBuy,
			Timestamp:   1700000000000,
		}

		err := repo.Create(ctx, trade)
		require.NoError(t, err)

		// Verify
		result, err := repo.GetByID(ctx, "trade-001")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "trade-001", result.TradeID)
		assert.Equal(t, "BTC-USDC", result.Market)
		assert.True(t, result.Price.Equal(decimal.NewFromFloat(50000)))
	})
}

func TestTradeRepository_BatchCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTradeRepository(db)
	ctx := context.Background()

	t.Run("batch create trades", func(t *testing.T) {
		trades := []*model.Trade{
			{
				TradeID:   "batch-001",
				Market:    "ETH-USDC",
				Price:     decimal.NewFromFloat(3000),
				Amount:    decimal.NewFromFloat(10),
				Timestamp: 1700000001000,
			},
			{
				TradeID:   "batch-002",
				Market:    "ETH-USDC",
				Price:     decimal.NewFromFloat(3001),
				Amount:    decimal.NewFromFloat(5),
				Timestamp: 1700000002000,
			},
			{
				TradeID:   "batch-003",
				Market:    "ETH-USDC",
				Price:     decimal.NewFromFloat(3002),
				Amount:    decimal.NewFromFloat(8),
				Timestamp: 1700000003000,
			},
		}

		err := repo.BatchCreate(ctx, trades)
		require.NoError(t, err)

		// Verify all were created
		result, err := repo.ListRecent(ctx, "ETH-USDC", 100)
		require.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("batch create empty slice", func(t *testing.T) {
		err := repo.BatchCreate(ctx, []*model.Trade{})
		require.NoError(t, err)
	})
}

func TestTradeRepository_ListRecent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTradeRepository(db)
	ctx := context.Background()

	// Insert test data
	for i := 0; i < 20; i++ {
		trade := &model.Trade{
			TradeID:   "recent-" + string(rune('A'+i)),
			Market:    "BTC-USDC",
			Price:     decimal.NewFromFloat(50000 + float64(i)),
			Amount:    decimal.NewFromFloat(1),
			Timestamp: int64(1700000000000 + i*1000),
		}
		_ = repo.Create(ctx, trade)
	}

	t.Run("list with default limit", func(t *testing.T) {
		result, err := repo.ListRecent(ctx, "BTC-USDC", 0)
		require.NoError(t, err)
		assert.Len(t, result, 20) // Default is 50, but we only have 20
		// Should be in ascending order (oldest first)
		assert.True(t, result[0].Timestamp < result[len(result)-1].Timestamp)
	})

	t.Run("list with specific limit", func(t *testing.T) {
		result, err := repo.ListRecent(ctx, "BTC-USDC", 5)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("list with limit exceeding max", func(t *testing.T) {
		result, err := repo.ListRecent(ctx, "BTC-USDC", 200)
		require.NoError(t, err)
		assert.Len(t, result, 20) // Max is 100, but we only have 20
	})

	t.Run("list unknown market", func(t *testing.T) {
		result, err := repo.ListRecent(ctx, "UNKNOWN-USDC", 10)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestTradeRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTradeRepository(db)
	ctx := context.Background()

	t.Run("get existing trade", func(t *testing.T) {
		trade := &model.Trade{
			TradeID:   "get-001",
			Market:    "BTC-USDC",
			Price:     decimal.NewFromFloat(50000),
			Amount:    decimal.NewFromFloat(1),
			Timestamp: 1700000000000,
		}
		_ = repo.Create(ctx, trade)

		result, err := repo.GetByID(ctx, "get-001")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "get-001", result.TradeID)
	})

	t.Run("get non-existing trade", func(t *testing.T) {
		result, err := repo.GetByID(ctx, "unknown-trade")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestTradeRepository_ListByTimeRange(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTradeRepository(db).(*tradeRepository)
	ctx := context.Background()

	// Insert test data
	baseTime := int64(1700000000000)
	for i := 0; i < 10; i++ {
		trade := &model.Trade{
			TradeID:   "range-" + string(rune('A'+i)),
			Market:    "BTC-USDC",
			Price:     decimal.NewFromFloat(50000),
			Amount:    decimal.NewFromFloat(1),
			Timestamp: baseTime + int64(i*1000),
		}
		_ = repo.Create(ctx, trade)
	}

	t.Run("query with start time", func(t *testing.T) {
		result, err := repo.ListByTimeRange(ctx, "BTC-USDC", baseTime+5000, 0, 100)
		require.NoError(t, err)
		assert.Len(t, result, 5)
	})

	t.Run("query with end time", func(t *testing.T) {
		result, err := repo.ListByTimeRange(ctx, "BTC-USDC", 0, baseTime+5000, 100)
		require.NoError(t, err)
		assert.Len(t, result, 6)
	})

	t.Run("query with time range", func(t *testing.T) {
		result, err := repo.ListByTimeRange(ctx, "BTC-USDC", baseTime+2000, baseTime+7000, 100)
		require.NoError(t, err)
		assert.Len(t, result, 6)
	})

	t.Run("query with limit", func(t *testing.T) {
		result, err := repo.ListByTimeRange(ctx, "BTC-USDC", 0, 0, 3)
		require.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("query with default limit", func(t *testing.T) {
		result, err := repo.ListByTimeRange(ctx, "BTC-USDC", 0, 0, 0)
		require.NoError(t, err)
		assert.Len(t, result, 10) // Default is 100, but we only have 10
	})
}
