package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
)

// Note: MockTradeRepository is defined in trade_service_test.go
// Note: MockBalanceRedisRepository, MockOrderRepository, MockBalanceRepository, MockMarketConfigProvider
// are defined in order_service_test.go

// setupMockDB 创建 mock 数据库
func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	return gormDB, sqlMock, func() {
		db.Close()
	}
}

// ========== Test Cases for ProcessTradeResult ==========

func TestClearingService_ProcessTradeResult_Success(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	tradeRepo := new(MockTradeRepository)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, tradeRepo, orderRepo, balanceRepo, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:      "T1234567890123456789",
		Market:       "ETH-USDT",
		MakerOrderID: "O1234567890123456789",
		TakerOrderID: "O1234567890123456790",
		Maker:        "0x1234567890123456789012345678901234567890",
		Taker:        "0x0987654321098765432109876543210987654321",
		Price:        "3000.50",
		Size:         "1.5",
		QuoteAmount:  "4500.75",
		MakerFee:     "2.25",
		TakerFee:     "4.50",
		Timestamp:    1705401600000,
		MakerIsBuyer: true,
	}

	// Mock market config
	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Mock Redis ClearTrade success
	balanceCache.On("ClearTrade", ctx, mock.AnythingOfType("*cache.ClearTradeRequest")).Return(nil)

	// Mock DB transaction for trade persistence
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("INSERT INTO trades").
		WithArgs(
			"T1234567890123456789", "ETH-USDT",
			"O1234567890123456789", "O1234567890123456790",
			"0x1234567890123456789012345678901234567890",
			"0x0987654321098765432109876543210987654321",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
			model.OrderSideBuy, model.SettlementStatusMatchedOffchain,
			int64(1705401600000), int64(1705401600000), int64(1705401600000),
		).WillReturnResult(sqlmock.NewResult(1, 1))

	// Mock order updates (sorted by order ID)
	sqlMock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectCommit()

	err := svc.ProcessTradeResult(ctx, msg)

	assert.NoError(t, err)
	balanceCache.AssertCalled(t, "ClearTrade", ctx, mock.MatchedBy(func(req *cache.ClearTradeRequest) bool {
		return req.TradeID == "T1234567890123456789" &&
			req.BaseToken == "ETH" &&
			req.QuoteToken == "USDT" &&
			req.MakerIsBuy == true
	}))
}

func TestClearingService_ProcessTradeResult_Idempotent(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	tradeRepo := new(MockTradeRepository)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, tradeRepo, orderRepo, balanceRepo, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:      "T1234567890123456789",
		Market:       "ETH-USDT",
		MakerOrderID: "O1234567890123456789",
		TakerOrderID: "O1234567890123456790",
		Maker:        "0x1234567890123456789012345678901234567890",
		Taker:        "0x0987654321098765432109876543210987654321",
		Price:        "3000.50",
		Size:         "1.5",
		QuoteAmount:  "4500.75",
		MakerFee:     "2.25",
		TakerFee:     "4.50",
		Timestamp:    1705401600000,
		MakerIsBuyer: true,
	}

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Redis returns already processed error (idempotent)
	balanceCache.On("ClearTrade", ctx, mock.AnythingOfType("*cache.ClearTradeRequest")).
		Return(cache.ErrRedisTradeProcessed)

	err := svc.ProcessTradeResult(ctx, msg)

	// Should return nil (idempotent - no error for duplicate)
	assert.NoError(t, err)
}

func TestClearingService_ProcessTradeResult_InvalidPrice(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID: "T123",
		Market:  "ETH-USDT",
		Price:   "invalid-price", // Invalid
		Size:    "1.5",
	}

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid price")
}

func TestClearingService_ProcessTradeResult_InvalidSize(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID: "T123",
		Market:  "ETH-USDT",
		Price:   "3000.50",
		Size:    "invalid", // Invalid
	}

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid size")
}

func TestClearingService_ProcessTradeResult_MarketNotFound(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	marketProvider := new(MockMarketConfigProvider)
	svc := NewClearingService(gormDB, nil, nil, nil, nil, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:     "T123",
		Market:      "UNKNOWN-MARKET",
		Price:       "3000.50",
		Size:        "1.5",
		QuoteAmount: "4500.75",
		MakerFee:    "2.25",
		TakerFee:    "4.50",
	}

	marketProvider.On("GetMarket", "UNKNOWN-MARKET").Return(nil, errors.New("market not found"))

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get market config")
}

func TestClearingService_ProcessTradeResult_RedisClearFailed(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:      "T123",
		Market:       "ETH-USDT",
		MakerOrderID: "O1",
		TakerOrderID: "O2",
		Maker:        "0x1234567890123456789012345678901234567890",
		Taker:        "0x0987654321098765432109876543210987654321",
		Price:        "3000.50",
		Size:         "1.5",
		QuoteAmount:  "4500.75",
		MakerFee:     "2.25",
		TakerFee:     "4.50",
		Timestamp:    1705401600000,
		MakerIsBuyer: true,
	}

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Redis ClearTrade fails with generic error
	balanceCache.On("ClearTrade", ctx, mock.AnythingOfType("*cache.ClearTradeRequest")).
		Return(errors.New("redis connection error"))

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis clear trade")
}

func TestClearingService_ProcessTradeResult_DBPersistFailedWithRollback(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	tradeRepo := new(MockTradeRepository)
	orderRepo := new(MockOrderRepository)
	balanceRepo := new(MockBalanceRepository)
	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, tradeRepo, orderRepo, balanceRepo, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:      "T123",
		Market:       "ETH-USDT",
		MakerOrderID: "O1",
		TakerOrderID: "O2",
		Maker:        "0x1234567890123456789012345678901234567890",
		Taker:        "0x0987654321098765432109876543210987654321",
		Price:        "3000.50",
		Size:         "1.5",
		QuoteAmount:  "4500.75",
		MakerFee:     "2.25",
		TakerFee:     "4.50",
		Timestamp:    1705401600000,
		MakerIsBuyer: true,
	}

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Redis ClearTrade succeeds
	balanceCache.On("ClearTrade", ctx, mock.AnythingOfType("*cache.ClearTradeRequest")).Return(nil)

	// DB transaction fails
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("INSERT INTO trades").
		WillReturnError(errors.New("db connection lost"))
	sqlMock.ExpectRollback()

	// Expect rollback: query trade from DB (for RollbackTrade)
	// Note: GORM First() adds LIMIT 1, so we need 2 arguments
	sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
		WithArgs("T123", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"trade_id", "market", "maker_order_id", "taker_order_id",
			"maker_wallet", "taker_wallet", "price", "amount", "quote_amount",
			"maker_fee", "taker_fee", "maker_side",
		}).AddRow(
			"T123", "ETH-USDT", "O1", "O2",
			"0x1234567890123456789012345678901234567890",
			"0x0987654321098765432109876543210987654321",
			decimal.NewFromFloat(3000.50), decimal.NewFromFloat(1.5), decimal.NewFromFloat(4500.75),
			decimal.NewFromFloat(2.25), decimal.NewFromFloat(4.50), model.OrderSideBuy,
		))

	// Rollback Redis
	balanceCache.On("RollbackTrade", ctx, mock.AnythingOfType("*cache.RollbackTradeRequest")).Return(nil)

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "persist trade to db failed")
	balanceCache.AssertCalled(t, "RollbackTrade", ctx, mock.Anything)
}

// ========== Test Cases for HandleSettlementConfirm ==========

func TestClearingService_HandleSettlementConfirm_Success(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.SettlementConfirmedMessage{
		SettlementID: "S123",
		TradeIDs:     []string{"T1", "T2"},
		TxHash:       "0xabc123",
		BlockNumber:  12345678,
		Status:       "confirmed",
		Timestamp:    1705401700000,
	}

	// Mock DB update for each trade
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("UPDATE trades").
		WithArgs(model.SettlementStatusSettledOnchain, "0xabc123", int64(1705401700000), int64(1705401700000), "T1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectExec("UPDATE trades").
		WithArgs(model.SettlementStatusSettledOnchain, "0xabc123", int64(1705401700000), int64(1705401700000), "T2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectCommit()

	// Mock DB query for settle balances (GORM First() adds LIMIT 1)
	for _, tradeID := range msg.TradeIDs {
		sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
			WithArgs(tradeID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"trade_id", "market", "maker_wallet", "taker_wallet",
				"amount", "quote_amount", "maker_fee", "taker_fee", "maker_side",
			}).AddRow(
				tradeID, "ETH-USDT",
				"0x1234567890123456789012345678901234567890",
				"0x0987654321098765432109876543210987654321",
				decimal.NewFromFloat(1.0), decimal.NewFromFloat(3000.0),
				decimal.NewFromFloat(1.5), decimal.NewFromFloat(3.0), model.OrderSideBuy,
			))
	}

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Mock Redis Settle calls
	balanceCache.On("Settle", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("decimal.Decimal")).Return(nil)

	err := svc.HandleSettlementConfirm(ctx, msg)

	assert.NoError(t, err)
}

func TestClearingService_HandleSettlementConfirm_Failed_Rollback(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.SettlementConfirmedMessage{
		SettlementID: "S456",
		TradeIDs:     []string{"T3", "T4"},
		TxHash:       "",
		BlockNumber:  0,
		Status:       "failed", // Settlement failed
		Timestamp:    1705401800000,
	}

	// For each trade: query from DB for rollback (GORM First() adds LIMIT 1)
	for _, tradeID := range msg.TradeIDs {
		sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
			WithArgs(tradeID, 1).
			WillReturnRows(sqlmock.NewRows([]string{
				"trade_id", "market", "maker_wallet", "taker_wallet",
				"amount", "quote_amount", "maker_fee", "taker_fee", "maker_side",
			}).AddRow(
				tradeID, "ETH-USDT",
				"0x1234567890123456789012345678901234567890",
				"0x0987654321098765432109876543210987654321",
				decimal.NewFromFloat(1.0), decimal.NewFromFloat(3000.0),
				decimal.NewFromFloat(1.5), decimal.NewFromFloat(3.0), model.OrderSideBuy,
			))
	}

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Mock Redis RollbackTrade
	balanceCache.On("RollbackTrade", ctx, mock.AnythingOfType("*cache.RollbackTradeRequest")).Return(nil)

	// Mock DB update status to rolled_back
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("UPDATE trades").
		WithArgs(model.SettlementStatusRolledBack, sqlmock.AnyArg(), "T3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectExec("UPDATE trades").
		WithArgs(model.SettlementStatusRolledBack, sqlmock.AnyArg(), "T4").
		WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectCommit()

	err := svc.HandleSettlementConfirm(ctx, msg)

	assert.NoError(t, err)
	balanceCache.AssertNumberOfCalls(t, "RollbackTrade", 2)
}

func TestClearingService_HandleSettlementConfirm_Rollback_PartialFailure(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.SettlementConfirmedMessage{
		SettlementID: "S789",
		TradeIDs:     []string{"T5", "T6"},
		Status:       "failed",
		Timestamp:    1705401900000,
	}

	// T5: query succeeds (GORM First() adds LIMIT 1)
	sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
		WithArgs("T5", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"trade_id", "market", "maker_wallet", "taker_wallet",
			"amount", "quote_amount", "maker_fee", "taker_fee", "maker_side",
		}).AddRow(
			"T5", "ETH-USDT",
			"0x1234567890123456789012345678901234567890",
			"0x0987654321098765432109876543210987654321",
			decimal.NewFromFloat(1.0), decimal.NewFromFloat(3000.0),
			decimal.NewFromFloat(1.5), decimal.NewFromFloat(3.0), model.OrderSideBuy,
		))

	// T6: query fails (GORM First() adds LIMIT 1)
	sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
		WithArgs("T6", 1).
		WillReturnError(errors.New("trade not found"))

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// T5 rollback succeeds
	balanceCache.On("RollbackTrade", ctx, mock.MatchedBy(func(req *cache.RollbackTradeRequest) bool {
		return req.TradeID == "T5"
	})).Return(nil)

	// DB update status
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("UPDATE trades").WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectExec("UPDATE trades").WillReturnResult(sqlmock.NewResult(0, 1))
	sqlMock.ExpectCommit()

	err := svc.HandleSettlementConfirm(ctx, msg)

	// Should return error due to partial failure
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errors")
}

// ========== Test settling balances for different trade directions ==========

func TestClearingService_SettleBalances_MakerBuy(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.SettlementConfirmedMessage{
		SettlementID: "S100",
		TradeIDs:     []string{"T100"},
		TxHash:       "0xdef456",
		BlockNumber:  12345680,
		Status:       "confirmed",
		Timestamp:    1705402000000,
	}

	// DB update and query trade within same transaction
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("UPDATE trades").WillReturnResult(sqlmock.NewResult(0, 1))

	// Query trade within transaction: Maker is BUYER (GORM First() adds LIMIT 1)
	// Note: settleBalancesForTrades is called inside the transaction
	sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
		WithArgs("T100", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"trade_id", "market", "maker_wallet", "taker_wallet",
			"amount", "quote_amount", "maker_fee", "taker_fee", "maker_side",
		}).AddRow(
			"T100", "ETH-USDT",
			"0xMaker", "0xTaker",
			decimal.NewFromFloat(2.0),    // amount (ETH)
			decimal.NewFromFloat(6000.0), // quoteAmount (USDT)
			decimal.NewFromFloat(0.01),   // makerFee (ETH)
			decimal.NewFromFloat(6.0),    // takerFee (USDT)
			model.OrderSideBuy,           // Maker is buyer
		))

	sqlMock.ExpectCommit()

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Maker (buyer) receives: base - makerFee = 2.0 - 0.01 = 1.99 ETH
	balanceCache.On("Settle", ctx, "0xMaker", "ETH", mock.MatchedBy(func(d decimal.Decimal) bool {
		return d.Equal(decimal.NewFromFloat(1.99))
	})).Return(nil)

	// Taker (seller) receives: quote - takerFee = 6000 - 6 = 5994 USDT
	balanceCache.On("Settle", ctx, "0xTaker", "USDT", mock.MatchedBy(func(d decimal.Decimal) bool {
		return d.Equal(decimal.NewFromFloat(5994.0))
	})).Return(nil)

	err := svc.HandleSettlementConfirm(ctx, msg)

	assert.NoError(t, err)
	balanceCache.AssertExpectations(t)
}

func TestClearingService_SettleBalances_MakerSell(t *testing.T) {
	gormDB, sqlMock, cleanup := setupMockDB(t)
	defer cleanup()

	balanceCache := new(MockBalanceRedisRepository)
	marketProvider := new(MockMarketConfigProvider)

	svc := NewClearingService(gormDB, nil, nil, nil, balanceCache, marketProvider, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.SettlementConfirmedMessage{
		SettlementID: "S101",
		TradeIDs:     []string{"T101"},
		TxHash:       "0xghi789",
		BlockNumber:  12345681,
		Status:       "confirmed",
		Timestamp:    1705402100000,
	}

	// DB update and query trade within same transaction
	sqlMock.ExpectBegin()
	sqlMock.ExpectExec("UPDATE trades").WillReturnResult(sqlmock.NewResult(0, 1))

	// Query trade within transaction: Maker is SELLER (GORM First() adds LIMIT 1)
	// Note: settleBalancesForTrades is called inside the transaction
	sqlMock.ExpectQuery("SELECT(.+)FROM(.+)trades").
		WithArgs("T101", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"trade_id", "market", "maker_wallet", "taker_wallet",
			"amount", "quote_amount", "maker_fee", "taker_fee", "maker_side",
		}).AddRow(
			"T101", "ETH-USDT",
			"0xMaker", "0xTaker",
			decimal.NewFromFloat(1.0),    // amount (ETH)
			decimal.NewFromFloat(3000.0), // quoteAmount (USDT)
			decimal.NewFromFloat(3.0),    // makerFee (USDT)
			decimal.NewFromFloat(0.005),  // takerFee (ETH)
			model.OrderSideSell,          // Maker is seller
		))

	sqlMock.ExpectCommit()

	marketProvider.On("GetMarket", "ETH-USDT").Return(&MarketConfig{
		Market:     "ETH-USDT",
		BaseToken:  "ETH",
		QuoteToken: "USDT",
	}, nil)

	// Maker (seller) receives: quote - makerFee = 3000 - 3 = 2997 USDT
	balanceCache.On("Settle", ctx, "0xMaker", "USDT", mock.MatchedBy(func(d decimal.Decimal) bool {
		return d.Equal(decimal.NewFromFloat(2997.0))
	})).Return(nil)

	// Taker (buyer) receives: base - takerFee = 1.0 - 0.005 = 0.995 ETH
	balanceCache.On("Settle", ctx, "0xTaker", "ETH", mock.MatchedBy(func(d decimal.Decimal) bool {
		return d.Equal(decimal.NewFromFloat(0.995))
	})).Return(nil)

	err := svc.HandleSettlementConfirm(ctx, msg)

	assert.NoError(t, err)
	balanceCache.AssertExpectations(t)
}

// ========== Edge Cases ==========

func TestClearingService_ProcessTradeResult_InvalidQuoteAmount(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:     "T123",
		Market:      "ETH-USDT",
		Price:       "3000.50",
		Size:        "1.5",
		QuoteAmount: "not-a-number",
	}

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid quote amount")
}

func TestClearingService_ProcessTradeResult_InvalidMakerFee(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:     "T123",
		Market:      "ETH-USDT",
		Price:       "3000.50",
		Size:        "1.5",
		QuoteAmount: "4500.75",
		MakerFee:    "invalid",
	}

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid maker fee")
}

func TestClearingService_ProcessTradeResult_InvalidTakerFee(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	msg := &worker.TradeResultMessage{
		TradeID:     "T123",
		Market:      "ETH-USDT",
		Price:       "3000.50",
		Size:        "1.5",
		QuoteAmount: "4500.75",
		MakerFee:    "2.25",
		TakerFee:    "bad-value",
	}

	err := svc.ProcessTradeResult(ctx, msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid taker fee")
}

func TestClearingService_Shutdown(t *testing.T) {
	gormDB, _, cleanup := setupMockDB(t)
	defer cleanup()

	svc := NewClearingService(gormDB, nil, nil, nil, nil, nil, nil, nil, nil)

	ctx := context.Background()
	err := svc.Shutdown(ctx)

	assert.NoError(t, err)
}
