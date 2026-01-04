package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// ========== Mock Trade Repository ==========

type MockTradeRepository struct {
	mock.Mock
}

func (m *MockTradeRepository) Create(ctx context.Context, trade *model.Trade) error {
	args := m.Called(ctx, trade)
	return args.Error(0)
}

func (m *MockTradeRepository) BatchCreate(ctx context.Context, trades []*model.Trade) error {
	args := m.Called(ctx, trades)
	return args.Error(0)
}

func (m *MockTradeRepository) GetByID(ctx context.Context, id int64) (*model.Trade, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) GetByTradeID(ctx context.Context, tradeID string) (*model.Trade, error) {
	args := m.Called(ctx, tradeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) ListByOrderID(ctx context.Context, orderID string) ([]*model.Trade, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) ListByWallet(ctx context.Context, wallet string, filter *repository.TradeFilter, page *repository.Pagination) ([]*model.Trade, error) {
	args := m.Called(ctx, wallet, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) ListByMarket(ctx context.Context, market string, filter *repository.TradeFilter, page *repository.Pagination) ([]*model.Trade, error) {
	args := m.Called(ctx, market, filter, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) ListBySettlementStatus(ctx context.Context, status model.SettlementStatus, limit int) ([]*model.Trade, error) {
	args := m.Called(ctx, status, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) ListByBatchID(ctx context.Context, batchID string) ([]*model.Trade, error) {
	args := m.Called(ctx, batchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) UpdateSettlementStatus(ctx context.Context, tradeID string, oldStatus, newStatus model.SettlementStatus, batchID string) error {
	args := m.Called(ctx, tradeID, oldStatus, newStatus, batchID)
	return args.Error(0)
}

func (m *MockTradeRepository) BatchUpdateSettlementStatus(ctx context.Context, tradeIDs []string, newStatus model.SettlementStatus, txHash string) error {
	args := m.Called(ctx, tradeIDs, newStatus, txHash)
	return args.Error(0)
}

func (m *MockTradeRepository) MarkSettled(ctx context.Context, tradeID string, txHash string) error {
	args := m.Called(ctx, tradeID, txHash)
	return args.Error(0)
}

func (m *MockTradeRepository) CreateBatch(ctx context.Context, batch *model.SettlementBatch) error {
	args := m.Called(ctx, batch)
	return args.Error(0)
}

func (m *MockTradeRepository) GetBatch(ctx context.Context, batchID string) (*model.SettlementBatch, error) {
	args := m.Called(ctx, batchID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.SettlementBatch), args.Error(1)
}

func (m *MockTradeRepository) UpdateBatch(ctx context.Context, batch *model.SettlementBatch) error {
	args := m.Called(ctx, batch)
	return args.Error(0)
}

func (m *MockTradeRepository) ListPendingBatches(ctx context.Context, limit int) ([]*model.SettlementBatch, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.SettlementBatch), args.Error(1)
}

func (m *MockTradeRepository) GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Trade), args.Error(1)
}

func (m *MockTradeRepository) CountByMarket(ctx context.Context, market string, timeRange *repository.TimeRange) (int64, error) {
	args := m.Called(ctx, market, timeRange)
	return args.Get(0).(int64), args.Error(1)
}

// ========== Test Cases ==========
// Note: ProcessTradeResult tests have been moved to clearing_service_test.go
// as ProcessTradeResult is now implemented in ClearingService using Redis Lua atomic operations.

func TestTradeService_GetTrade_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	svc := NewTradeService(tradeRepo, nil, nil, nil, nil)

	ctx := context.Background()
	tradeID := "T123456789"

	expectedTrade := &model.Trade{
		TradeID: tradeID,
		Market:  "ETH-USDT",
	}

	tradeRepo.On("GetByTradeID", ctx, tradeID).Return(expectedTrade, nil)

	trade, err := svc.GetTrade(ctx, tradeID)

	assert.NoError(t, err)
	assert.NotNil(t, trade)
	assert.Equal(t, tradeID, trade.TradeID)
}

func TestTradeService_ListTradesByOrder_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	svc := NewTradeService(tradeRepo, nil, nil, nil, nil)

	ctx := context.Background()
	orderID := "O123456789"

	expectedTrades := []*model.Trade{
		{TradeID: "T1", MakerOrderID: orderID},
		{TradeID: "T2", TakerOrderID: orderID},
	}

	tradeRepo.On("ListByOrderID", ctx, orderID).Return(expectedTrades, nil)

	trades, err := svc.ListTradesByOrder(ctx, orderID)

	assert.NoError(t, err)
	assert.Len(t, trades, 2)
}

func TestTradeService_ListTrades_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	svc := NewTradeService(tradeRepo, nil, nil, nil, nil)

	ctx := context.Background()
	wallet := "0x1234567890123456789012345678901234567890"

	expectedTrades := []*model.Trade{
		{TradeID: "T1", MakerWallet: wallet},
		{TradeID: "T2", TakerWallet: wallet},
	}

	tradeRepo.On("ListByWallet", ctx, wallet, (*repository.TradeFilter)(nil), (*repository.Pagination)(nil)).Return(expectedTrades, nil)

	trades, err := svc.ListTrades(ctx, wallet, nil, nil)

	assert.NoError(t, err)
	assert.Len(t, trades, 2)
}

func TestTradeService_ListRecentTrades_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	svc := NewTradeService(tradeRepo, nil, nil, nil, nil)

	ctx := context.Background()
	market := "ETH-USDT"
	limit := 100

	expectedTrades := []*model.Trade{
		{TradeID: "T1", Market: market},
		{TradeID: "T2", Market: market},
	}

	tradeRepo.On("GetRecentTrades", ctx, market, limit).Return(expectedTrades, nil)

	trades, err := svc.ListRecentTrades(ctx, market, limit)

	assert.NoError(t, err)
	assert.Len(t, trades, 2)
}

func TestTradeService_CreateSettlementBatch_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewTradeService(tradeRepo, balanceRepo, nil, idGen, marketCfg)

	ctx := context.Background()
	tradeIDs := []string{"T1", "T2", "T3"}

	trades := []*model.Trade{
		{TradeID: "T1", QuoteAmount: decimal.NewFromFloat(1000)},
		{TradeID: "T2", QuoteAmount: decimal.NewFromFloat(2000)},
		{TradeID: "T3", QuoteAmount: decimal.NewFromFloat(3000)},
	}

	idGen.On("Generate").Return(int64(987654321), nil)
	tradeRepo.On("GetByTradeID", ctx, "T1").Return(trades[0], nil)
	tradeRepo.On("GetByTradeID", ctx, "T2").Return(trades[1], nil)
	tradeRepo.On("GetByTradeID", ctx, "T3").Return(trades[2], nil)
	tradeRepo.On("CreateBatch", mock.Anything, mock.AnythingOfType("*model.SettlementBatch")).Return(nil)
	tradeRepo.On("UpdateSettlementStatus", mock.Anything, "T1", model.SettlementStatusMatchedOffchain, model.SettlementStatusPending, "B987654321").Return(nil)
	tradeRepo.On("UpdateSettlementStatus", mock.Anything, "T2", model.SettlementStatusMatchedOffchain, model.SettlementStatusPending, "B987654321").Return(nil)
	tradeRepo.On("UpdateSettlementStatus", mock.Anything, "T3", model.SettlementStatusMatchedOffchain, model.SettlementStatusPending, "B987654321").Return(nil)
	balanceRepo.On("Transaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fn := args.Get(1).(func(context.Context) error)
		_ = fn(ctx)
	}).Return(nil)

	batch, err := svc.CreateSettlementBatch(ctx, tradeIDs)

	assert.NoError(t, err)
	assert.NotNil(t, batch)
	assert.Equal(t, "B987654321", batch.BatchID)
	assert.Equal(t, 3, batch.TradeCount)
	// Total amount = 1000 + 2000 + 3000 = 6000
	assert.True(t, batch.TotalAmount.Equal(decimal.NewFromFloat(6000)))
}

func TestTradeService_CreateSettlementBatch_EmptyTradeIDs(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	svc := NewTradeService(tradeRepo, nil, nil, nil, nil)

	ctx := context.Background()
	tradeIDs := []string{}

	batch, err := svc.CreateSettlementBatch(ctx, tradeIDs)

	assert.Nil(t, batch)
	assert.Error(t, err)
}

func TestTradeService_ConfirmSettlement_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewTradeService(tradeRepo, balanceRepo, nil, idGen, marketCfg)

	ctx := context.Background()
	batchID := "B123456789"
	txHash := "0x1234567890123456789012345678901234567890123456789012345678901234"

	batch := &model.SettlementBatch{
		BatchID:    batchID,
		TradeCount: 2,
		Status:     model.SettlementStatusPending,
	}

	trades := []*model.Trade{
		{
			TradeID:     "T1",
			Market:      "ETH-USDT",
			MakerWallet: "0x1111111111111111111111111111111111111111",
			TakerWallet: "0x2222222222222222222222222222222222222222",
			Amount:      decimal.NewFromFloat(1),
			QuoteAmount: decimal.NewFromFloat(2000),
			MakerSide:   model.OrderSideBuy,
		},
	}

	marketConfig := &MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}

	tradeRepo.On("GetBatch", ctx, batchID).Return(batch, nil)
	tradeRepo.On("ListByBatchID", ctx, batchID).Return(trades, nil)
	tradeRepo.On("UpdateBatch", mock.Anything, mock.AnythingOfType("*model.SettlementBatch")).Return(nil)
	tradeRepo.On("BatchUpdateSettlementStatus", mock.Anything, []string{"T1"}, model.SettlementStatusSettledOnchain, txHash).Return(nil)
	marketCfg.On("GetMarket", "ETH-USDT").Return(marketConfig, nil)
	balanceRepo.On("Settle", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	balanceRepo.On("Transaction", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		fn := args.Get(1).(func(context.Context) error)
		_ = fn(ctx)
	}).Return(nil)

	err := svc.ConfirmSettlement(ctx, batchID, txHash)

	assert.NoError(t, err)
}

func TestTradeService_FailSettlement_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewTradeService(tradeRepo, balanceRepo, nil, idGen, marketCfg)

	ctx := context.Background()
	batchID := "B123456789"
	reason := "Transaction reverted"

	batch := &model.SettlementBatch{
		BatchID:    batchID,
		Status:     model.SettlementStatusSubmitted,
		RetryCount: 0,
	}

	trades := []*model.Trade{
		{TradeID: "T1"},
		{TradeID: "T2"},
	}

	tradeRepo.On("GetBatch", ctx, batchID).Return(batch, nil)
	tradeRepo.On("UpdateBatch", ctx, mock.AnythingOfType("*model.SettlementBatch")).Return(nil)
	tradeRepo.On("ListByBatchID", ctx, batchID).Return(trades, nil)
	tradeRepo.On("BatchUpdateSettlementStatus", ctx, []string{"T1", "T2"}, model.SettlementStatusFailed, "").Return(nil)

	err := svc.FailSettlement(ctx, batchID, reason)

	assert.NoError(t, err)
}

func TestTradeService_RollbackTrades_Success(t *testing.T) {
	tradeRepo := new(MockTradeRepository)
	balanceRepo := new(ExtendedMockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	idGen := new(MockIDGenerator)
	marketCfg := new(MockMarketConfigProvider)

	svc := NewTradeService(tradeRepo, balanceRepo, balanceCache, idGen, marketCfg)

	ctx := context.Background()
	tradeIDs := []string{"T1", "T2"}

	trades := []*model.Trade{
		{
			TradeID:          "T1",
			Market:           "ETH-USDT",
			MakerWallet:      "0x1111111111111111111111111111111111111111",
			TakerWallet:      "0x2222222222222222222222222222222222222222",
			Amount:           decimal.NewFromFloat(1),
			QuoteAmount:      decimal.NewFromFloat(2000),
			MakerFee:         decimal.Zero,
			TakerFee:         decimal.Zero,
			MakerSide:        model.OrderSideBuy,
			SettlementStatus: model.SettlementStatusFailed,
		},
		{
			TradeID:          "T2",
			Market:           "ETH-USDT",
			MakerWallet:      "0x3333333333333333333333333333333333333333",
			TakerWallet:      "0x4444444444444444444444444444444444444444",
			Amount:           decimal.NewFromFloat(2),
			QuoteAmount:      decimal.NewFromFloat(4000),
			MakerFee:         decimal.Zero,
			TakerFee:         decimal.Zero,
			MakerSide:        model.OrderSideSell,
			SettlementStatus: model.SettlementStatusFailed,
		},
	}

	marketConfig := &MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}

	tradeRepo.On("GetByTradeID", ctx, "T1").Return(trades[0], nil)
	tradeRepo.On("GetByTradeID", ctx, "T2").Return(trades[1], nil)
	marketCfg.On("GetMarket", "ETH-USDT").Return(marketConfig, nil)
	balanceCache.On("RollbackTrade", ctx, mock.AnythingOfType("*cache.RollbackTradeRequest")).Return(nil)
	tradeRepo.On("UpdateSettlementStatus", ctx, "T1", model.SettlementStatusFailed, model.SettlementStatusRolledBack, "").Return(nil)
	tradeRepo.On("UpdateSettlementStatus", ctx, "T2", model.SettlementStatusFailed, model.SettlementStatusRolledBack, "").Return(nil)
	balanceRepo.On("CreateBalanceLog", ctx, mock.AnythingOfType("*model.BalanceLog")).Return(nil)

	err := svc.RollbackTrades(ctx, tradeIDs)

	assert.NoError(t, err)
	balanceCache.AssertNumberOfCalls(t, "RollbackTrade", 2)
}
