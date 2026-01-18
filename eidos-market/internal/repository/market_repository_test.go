package repository

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

func TestMarketRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketRepository(db)
	ctx := context.Background()

	t.Run("create market", func(t *testing.T) {
		market := &model.Market{
			Symbol:         "BTC-USDC",
			BaseToken:      "BTC",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			SizeDecimals:   6,
			MinSize:        decimal.NewFromFloat(0.0001),
			MaxSize:        decimal.NewFromFloat(1000),
			MinNotional:    decimal.NewFromFloat(10),
			TickSize:       decimal.NewFromFloat(0.01),
			MakerFee:       decimal.NewFromFloat(0.001),
			TakerFee:       decimal.NewFromFloat(0.001),
			Status:         model.MarketStatusActive,
			TradingEnabled: true,
			CreatedAt:      1700000000000,
			UpdatedAt:      1700000000000,
		}

		err := repo.Create(ctx, market)
		require.NoError(t, err)

		// Verify
		result, err := repo.GetBySymbol(ctx, "BTC-USDC")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "BTC-USDC", result.Symbol)
		assert.Equal(t, "BTC", result.BaseToken)
		assert.Equal(t, "USDC", result.QuoteToken)
		assert.Equal(t, model.MarketStatusActive, result.Status)
	})
}

func TestMarketRepository_GetBySymbol(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketRepository(db)
	ctx := context.Background()

	// Create test market
	market := &model.Market{
		Symbol:     "ETH-USDC",
		BaseToken:  "ETH",
		QuoteToken: "USDC",
		Status:     model.MarketStatusActive,
		CreatedAt:  1700000000000,
		UpdatedAt:  1700000000000,
	}
	_ = repo.Create(ctx, market)

	t.Run("get existing market", func(t *testing.T) {
		result, err := repo.GetBySymbol(ctx, "ETH-USDC")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ETH-USDC", result.Symbol)
	})

	t.Run("get non-existing market", func(t *testing.T) {
		result, err := repo.GetBySymbol(ctx, "UNKNOWN-USDC")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestMarketRepository_ListActiveMarkets(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketRepository(db)
	ctx := context.Background()

	// Create test markets
	markets := []*model.Market{
		{
			Symbol:     "BTC-USDC",
			BaseToken:  "BTC",
			QuoteToken: "USDC",
			Status:     model.MarketStatusActive,
			CreatedAt:  1700000000000,
			UpdatedAt:  1700000000000,
		},
		{
			Symbol:     "ETH-USDC",
			BaseToken:  "ETH",
			QuoteToken: "USDC",
			Status:     model.MarketStatusActive,
			CreatedAt:  1700000000000,
			UpdatedAt:  1700000000000,
		},
		{
			Symbol:     "SUSPEND-USDC",
			BaseToken:  "SUSPEND",
			QuoteToken: "USDC",
			Status:     model.MarketStatusSuspend, // Use Suspend (2) instead of Inactive (0)
			CreatedAt:  1700000000000,
			UpdatedAt:  1700000000000,
		},
	}

	for _, m := range markets {
		_ = repo.Create(ctx, m)
	}

	t.Run("list active markets only", func(t *testing.T) {
		result, err := repo.ListActiveMarkets(ctx)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		// Verify all are active
		for _, m := range result {
			assert.Equal(t, model.MarketStatusActive, m.Status)
		}
	})

	t.Run("list returns sorted by symbol", func(t *testing.T) {
		result, err := repo.ListActiveMarkets(ctx)
		require.NoError(t, err)
		assert.True(t, result[0].Symbol < result[1].Symbol)
	})
}

func TestMarketRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketRepository(db)
	ctx := context.Background()

	// Create test market
	market := &model.Market{
		Symbol:         "UPDATE-USDC",
		BaseToken:      "UPDATE",
		QuoteToken:     "USDC",
		Status:         model.MarketStatusActive,
		TradingEnabled: true,
		CreatedAt:      1700000000000,
		UpdatedAt:      1700000000000,
	}
	_ = repo.Create(ctx, market)

	t.Run("update market", func(t *testing.T) {
		// Fetch the market first
		existing, _ := repo.GetBySymbol(ctx, "UPDATE-USDC")
		require.NotNil(t, existing)

		// Update fields
		existing.TradingEnabled = false
		existing.Status = model.MarketStatusInactive
		existing.UpdatedAt = 1700000001000

		err := repo.Update(ctx, existing)
		require.NoError(t, err)

		// Verify update
		result, err := repo.GetBySymbol(ctx, "UPDATE-USDC")
		require.NoError(t, err)
		assert.False(t, result.TradingEnabled)
		assert.Equal(t, model.MarketStatusInactive, result.Status)
	})
}

func TestMarketRepository_ListAllMarkets(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketRepository(db).(*marketRepository)
	ctx := context.Background()

	// Create test markets with different statuses
	markets := []*model.Market{
		{
			Symbol:    "ACTIVE1-USDC",
			Status:    model.MarketStatusActive,
			CreatedAt: 1700000000000,
			UpdatedAt: 1700000000000,
		},
		{
			Symbol:    "ACTIVE2-USDC",
			Status:    model.MarketStatusActive,
			CreatedAt: 1700000000000,
			UpdatedAt: 1700000000000,
		},
		{
			Symbol:    "SUSPEND1-USDC",
			Status:    model.MarketStatusSuspend, // Use Suspend (2) instead of Inactive (0) due to GORM default behavior
			CreatedAt: 1700000000000,
			UpdatedAt: 1700000000000,
		},
	}

	for _, m := range markets {
		_ = repo.Create(ctx, m)
	}

	t.Run("list all markets including non-active", func(t *testing.T) {
		result, err := repo.ListAllMarkets(ctx)
		require.NoError(t, err)
		assert.Len(t, result, 3)

		// Verify we have both active and suspended markets
		var activeCount, suspendCount int
		for _, m := range result {
			if m.Status == model.MarketStatusActive {
				activeCount++
			} else if m.Status == model.MarketStatusSuspend {
				suspendCount++
			}
		}
		assert.Equal(t, 2, activeCount)
		assert.Equal(t, 1, suspendCount)
	})
}
