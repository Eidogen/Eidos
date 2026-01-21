package service

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// Note: MockTradeRepository is defined in trade_service_test.go

// MockSettlementPublisher implements SettlementPublisher for testing
type MockSettlementPublisher struct {
	mock.Mock
}

func (m *MockSettlementPublisher) PublishSettlementTrade(ctx context.Context, trade *model.Trade) error {
	args := m.Called(ctx, trade)
	return args.Error(0)
}

func (m *MockSettlementPublisher) PublishSettlementTradeFromResult(
	ctx context.Context,
	tradeID, market, makerWallet, takerWallet string,
	makerOrderID, takerOrderID string,
	price, amount, quoteAmount, makerFee, takerFee decimal.Decimal,
	makerIsBuyer bool,
	matchedAt int64,
) error {
	args := m.Called(ctx, tradeID, market, makerWallet, takerWallet,
		makerOrderID, takerOrderID, price, amount, quoteAmount, makerFee, takerFee, makerIsBuyer, matchedAt)
	return args.Error(0)
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto migrate the schema
	err = db.AutoMigrate(&SettlementBatch{})
	assert.NoError(t, err)

	return db
}

func TestSettlementService_CreateBatch_Success(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()
	tradeIDs := []string{"T1", "T2", "T3"}

	// Mock trade retrieval for total amount calculation
	tradeRepo.On("GetByTradeID", ctx, "T1").Return(&model.Trade{
		TradeID:     "T1",
		QuoteAmount: decimal.NewFromFloat(1000),
	}, nil)
	tradeRepo.On("GetByTradeID", ctx, "T2").Return(&model.Trade{
		TradeID:     "T2",
		QuoteAmount: decimal.NewFromFloat(2000),
	}, nil)
	tradeRepo.On("GetByTradeID", ctx, "T3").Return(&model.Trade{
		TradeID:     "T3",
		QuoteAmount: decimal.NewFromFloat(500),
	}, nil)

	idGen.On("Generate").Return(int64(123456789), nil)

	// Mock update batch ID for each trade
	tradeRepo.On("UpdateSettlementBatchID", ctx, "T1", "SB123456789").Return(nil)
	tradeRepo.On("UpdateSettlementBatchID", ctx, "T2", "SB123456789").Return(nil)
	tradeRepo.On("UpdateSettlementBatchID", ctx, "T3", "SB123456789").Return(nil)

	batch, err := svc.CreateBatch(ctx, tradeIDs)

	assert.NoError(t, err)
	assert.NotNil(t, batch)
	assert.Equal(t, "SB123456789", batch.BatchID)
	assert.Equal(t, 3, batch.TradeCount)
	assert.True(t, batch.TotalAmount.Equal(decimal.NewFromFloat(3500)))
	assert.Equal(t, BatchStatusPending, batch.Status)
}

func TestSettlementService_CreateBatch_NoTrades(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	batch, err := svc.CreateBatch(ctx, []string{})

	assert.Nil(t, batch)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoTradesToSettle))
}

func TestSettlementService_CreateBatch_ExceedsMaxSize(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create more trades than max allowed
	tradeIDs := make([]string, MaxBatchSize+1)
	for i := range tradeIDs {
		tradeIDs[i] = "T" + string(rune(i))
	}

	batch, err := svc.CreateBatch(ctx, tradeIDs)

	assert.Nil(t, batch)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrBatchSizeExceeded))
}

func TestSettlementService_GetBatch_Success(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a batch first
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeIDs:    []string{"T1", "T2"},
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusPending,
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Get the batch
	result, err := svc.GetBatch(ctx, "SB123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "SB123", result.BatchID)
	assert.Equal(t, 2, result.TradeCount)
}

func TestSettlementService_GetBatch_NotFound(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	result, err := svc.GetBatch(ctx, "SB-NOT-EXIST")

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrBatchNotFound))
}

func TestSettlementService_MarkBatchSubmitted(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a pending batch
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusPending,
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Mark as submitted
	err = svc.MarkBatchSubmitted(ctx, "SB123", "0xabc123")

	assert.NoError(t, err)

	// Verify the update
	var updated SettlementBatch
	db.First(&updated, "batch_id = ?", "SB123")
	assert.Equal(t, BatchStatusSubmitted, updated.Status)
	assert.Equal(t, "0xabc123", updated.TxHash)
	assert.Greater(t, updated.SubmittedAt, int64(0))
}

func TestSettlementService_MarkBatchSubmitted_NotPending(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a batch that's already submitted
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusSubmitted,
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Try to mark as submitted again
	err = svc.MarkBatchSubmitted(ctx, "SB123", "0xabc123")

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrBatchNotPending))
}

func TestSettlementService_MarkBatchConfirmed(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a submitted batch
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusSubmitted,
		TxHash:      "0xabc123",
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Mark as confirmed
	err = svc.MarkBatchConfirmed(ctx, "SB123", "0xabc123", 12345)

	assert.NoError(t, err)

	// Verify the update
	var updated SettlementBatch
	db.First(&updated, "batch_id = ?", "SB123")
	assert.Equal(t, BatchStatusConfirmed, updated.Status)
	assert.Equal(t, int64(12345), updated.BlockNumber)
	assert.Greater(t, updated.ConfirmedAt, int64(0))
}

func TestSettlementService_MarkBatchConfirmed_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a batch that's already confirmed
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusConfirmed,
		TxHash:      "0xabc123",
		BlockNumber: 12345,
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Try to confirm again (should be idempotent)
	err = svc.MarkBatchConfirmed(ctx, "SB123", "0xabc123", 12345)

	assert.NoError(t, err)
}

func TestSettlementService_MarkBatchFailed(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a batch
	batch := &SettlementBatch{
		BatchID:     "SB123",
		TradeCount:  2,
		TotalAmount: decimal.NewFromFloat(1000),
		Status:      BatchStatusSubmitted,
	}
	err := db.Create(batch).Error
	assert.NoError(t, err)

	// Mark as failed
	err = svc.MarkBatchFailed(ctx, "SB123", "transaction reverted")

	assert.NoError(t, err)

	// Verify the update
	var updated SettlementBatch
	db.First(&updated, "batch_id = ?", "SB123")
	assert.Equal(t, BatchStatusFailed, updated.Status)
	assert.Equal(t, "transaction reverted", updated.ErrorMsg)
}

func TestSettlementService_TriggerSettlement_Success(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	trades := []*model.Trade{
		{
			TradeID:      "T1",
			Market:       "ETH-USDT",
			MakerWallet:  "0x111",
			TakerWallet:  "0x222",
			MakerOrderID: "O1",
			TakerOrderID: "O2",
			Price:        decimal.NewFromFloat(2000),
			Amount:       decimal.NewFromFloat(1),
			QuoteAmount:  decimal.NewFromFloat(2000),
			MakerFee:     decimal.NewFromFloat(0.5),
			TakerFee:     decimal.NewFromFloat(1),
			MakerSide:    model.OrderSideBuy,
		},
	}

	tradeRepo.On("ListUnsettledTrades", ctx, DefaultBatchSize).Return(trades, nil)
	tradeRepo.On("GetByTradeID", ctx, "T1").Return(trades[0], nil)
	idGen.On("Generate").Return(int64(123456789), nil)
	tradeRepo.On("UpdateSettlementBatchID", ctx, "T1", "SB123456789").Return(nil)
	publisher.On("PublishSettlementTradeFromResult", ctx,
		"T1", "ETH-USDT", "0x111", "0x222", "O1", "O2",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		true, mock.Anything).Return(nil)

	batch, err := svc.TriggerSettlement(ctx, 0)

	assert.NoError(t, err)
	assert.NotNil(t, batch)
	assert.Equal(t, "SB123456789", batch.BatchID)
	assert.Equal(t, 1, batch.TradeCount)
}

func TestSettlementService_TriggerSettlement_NoTrades(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	tradeRepo.On("ListUnsettledTrades", ctx, DefaultBatchSize).Return([]*model.Trade{}, nil)

	batch, err := svc.TriggerSettlement(ctx, 0)

	assert.Nil(t, batch)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoTradesToSettle))
}

func TestSettlementService_ListPendingBatches(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create batches with different statuses
	batches := []*SettlementBatch{
		{BatchID: "SB1", Status: BatchStatusPending, TradeCount: 1, TotalAmount: decimal.NewFromFloat(100)},
		{BatchID: "SB2", Status: BatchStatusSubmitted, TradeCount: 1, TotalAmount: decimal.NewFromFloat(200)},
		{BatchID: "SB3", Status: BatchStatusPending, TradeCount: 1, TotalAmount: decimal.NewFromFloat(300)},
	}
	for _, b := range batches {
		db.Create(b)
	}

	// List pending batches
	result, err := svc.ListPendingBatches(ctx, 10)

	assert.NoError(t, err)
	assert.Len(t, result, 2) // Only pending batches
}

func TestSettlementService_GetUnconfirmedBatches(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create batches
	batches := []*SettlementBatch{
		{BatchID: "SB1", Status: BatchStatusSubmitted, SubmittedAt: 1000, TradeCount: 1, TotalAmount: decimal.NewFromFloat(100)},
		{BatchID: "SB2", Status: BatchStatusSubmitted, SubmittedAt: 5000, TradeCount: 1, TotalAmount: decimal.NewFromFloat(200)},
		{BatchID: "SB3", Status: BatchStatusConfirmed, SubmittedAt: 1000, TradeCount: 1, TotalAmount: decimal.NewFromFloat(300)},
	}
	for _, b := range batches {
		db.Create(b)
	}

	// Get unconfirmed batches submitted before time 3000
	result, err := svc.GetUnconfirmedBatches(ctx, 3000, 10)

	assert.NoError(t, err)
	assert.Len(t, result, 1) // Only SB1
	assert.Equal(t, "SB1", result[0].BatchID)
}

func TestSettlementService_RetryFailedBatches(t *testing.T) {
	db := setupTestDB(t)
	tradeRepo := new(MockTradeRepository)
	idGen := new(MockIDGenerator)
	publisher := new(MockSettlementPublisher)

	svc := NewSettlementService(db, tradeRepo, idGen, publisher)

	ctx := context.Background()

	// Create a failed batch
	batch := &SettlementBatch{
		BatchID:      "SB1",
		TradeIDs:     []string{"T1"},
		TradeIDsJSON: `["T1"]`,
		TradeCount:   1,
		TotalAmount:  decimal.NewFromFloat(100),
		Status:       BatchStatusFailed,
		ErrorMsg:     "previous error",
	}
	db.Create(batch)

	// Mock trade repo
	tradeRepo.On("GetByTradeID", ctx, "T1").Return(&model.Trade{
		TradeID:      "T1",
		Market:       "ETH-USDT",
		MakerWallet:  "0x111",
		TakerWallet:  "0x222",
		MakerOrderID: "O1",
		TakerOrderID: "O2",
		Price:        decimal.NewFromFloat(2000),
		Amount:       decimal.NewFromFloat(1),
		QuoteAmount:  decimal.NewFromFloat(2000),
		MakerFee:     decimal.Zero,
		TakerFee:     decimal.Zero,
		MakerSide:    model.OrderSideBuy,
	}, nil)

	// Mock publisher
	publisher.On("PublishSettlementTradeFromResult", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Retry failed batches
	count, err := svc.RetryFailedBatches(ctx, 10)

	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify batch is now pending
	var updated SettlementBatch
	db.First(&updated, "batch_id = ?", "SB1")
	assert.Equal(t, BatchStatusPending, updated.Status)
	assert.Empty(t, updated.ErrorMsg)
}
