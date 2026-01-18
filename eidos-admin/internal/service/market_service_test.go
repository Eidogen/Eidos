package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

func setupMarketTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&model.MarketConfig{}, &model.SystemConfig{}, &model.AuditLog{}, &model.ConfigVersion{})
	require.NoError(t, err)

	return db
}

func TestMarketService_Create_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()
	req := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		PriceDecimals:  2,
		SizeDecimals:   8,
		MinSize:        "0.0001",
		MaxSize:        "1000",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		DisplayOrder:   1,
		Description:    "Bitcoin/USDT trading pair",
		OperatorID:     1,
	}

	market, err := svc.Create(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, market)
	assert.Equal(t, "BTC-USDT", market.Symbol)
	assert.Equal(t, "BTC", market.BaseToken)
	assert.Equal(t, "USDT", market.QuoteToken)
	assert.Equal(t, model.MarketStatusSuspended, market.Status) // 默认暂停
	assert.False(t, market.TradingEnabled)
}

func TestMarketService_Create_DuplicateSymbol(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()
	req := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.0001",
		MaxSize:        "1000",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	_, err := svc.Create(ctx, req)
	require.NoError(t, err)

	// 尝试创建重复交易对
	_, err = svc.Create(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "交易对已存在")
}

func TestMarketService_Update_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.0001",
		MaxSize:        "1000",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 更新交易对
	newMinSize := "0.001"
	newMakerFee := "0.0005"
	newDescription := "Updated description"

	updateReq := &UpdateMarketRequest{
		ID:          created.ID,
		MinSize:     &newMinSize,
		MakerFee:    &newMakerFee,
		Description: &newDescription,
		OperatorID:  1,
	}

	updated, err := svc.Update(ctx, updateReq)
	require.NoError(t, err)
	assert.NotNil(t, updated)
	assert.Equal(t, "0.001", updated.MinSize)
	assert.Equal(t, "0.0005", updated.MakerFee)
	assert.Equal(t, "Updated description", updated.Description)
}

func TestMarketService_Update_NotFound(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	newMinSize := "0.001"
	updateReq := &UpdateMarketRequest{
		ID:         9999,
		MinSize:    &newMinSize,
		OperatorID: 1,
	}

	updated, err := svc.Update(ctx, updateReq)
	assert.Error(t, err)
	assert.Nil(t, updated)
	assert.Contains(t, err.Error(), "交易对不存在")
}

func TestMarketService_UpdateStatus_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.0001",
		MaxSize:        "1000",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	_, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 更新状态
	statusReq := &UpdateStatusRequest{
		Symbol:         "BTC-USDT",
		Status:         model.MarketStatusActive,
		TradingEnabled: true,
		OperatorID:     1,
	}

	err = svc.UpdateStatus(ctx, statusReq)
	require.NoError(t, err)

	// 验证状态已更新
	market, err := svc.GetBySymbol(ctx, "BTC-USDT")
	require.NoError(t, err)
	assert.Equal(t, model.MarketStatusActive, market.Status)
	assert.True(t, market.TradingEnabled)
}

func TestMarketService_UpdateStatus_NotFound(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	statusReq := &UpdateStatusRequest{
		Symbol:         "NONEXISTENT",
		Status:         model.MarketStatusActive,
		TradingEnabled: true,
		OperatorID:     1,
	}

	err := svc.UpdateStatus(ctx, statusReq)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "交易对不存在")
}

func TestMarketService_GetBySymbol_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "ETH-USDT",
		BaseToken:      "ETH",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.01",
		MaxSize:        "100",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	_, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 获取交易对
	market, err := svc.GetBySymbol(ctx, "ETH-USDT")
	require.NoError(t, err)
	assert.NotNil(t, market)
	assert.Equal(t, "ETH-USDT", market.Symbol)
	assert.Equal(t, "ETH", market.BaseToken)
}

func TestMarketService_GetBySymbol_NotFound(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	market, err := svc.GetBySymbol(ctx, "NONEXISTENT")
	assert.NoError(t, err)
	assert.Nil(t, market)
}

func TestMarketService_List_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建多个交易对
	symbols := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"}
	for _, symbol := range symbols {
		createReq := &CreateMarketRequest{
			Symbol:         symbol,
			BaseToken:      symbol[:3],
			QuoteToken:     "USDT",
			BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
			QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
			MinSize:        "0.01",
			MaxSize:        "100",
			MinNotional:    "10",
			TickSize:       "0.01",
			MakerFee:       "0.001",
			TakerFee:       "0.002",
			OperatorID:     1,
		}
		_, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
	}

	// 列表查询
	page := &model.Pagination{
		Page:     1,
		PageSize: 10,
	}

	markets, err := svc.List(ctx, page, nil)
	require.NoError(t, err)
	assert.Len(t, markets, 3)
}

func TestMarketService_List_FilterByStatus(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.01",
		MaxSize:        "100",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	_, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 更新一个为活跃状态
	statusReq := &UpdateStatusRequest{
		Symbol:         "BTC-USDT",
		Status:         model.MarketStatusActive,
		TradingEnabled: true,
		OperatorID:     1,
	}
	err = svc.UpdateStatus(ctx, statusReq)
	require.NoError(t, err)

	// 创建另一个保持暂停状态
	createReq2 := &CreateMarketRequest{
		Symbol:         "ETH-USDT",
		BaseToken:      "ETH",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567891",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.01",
		MaxSize:        "100",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}
	_, err = svc.Create(ctx, createReq2)
	require.NoError(t, err)

	// 过滤活跃状态
	page := &model.Pagination{Page: 1, PageSize: 10}
	activeStatus := model.MarketStatusActive
	markets, err := svc.List(ctx, page, &activeStatus)
	require.NoError(t, err)
	assert.Len(t, markets, 1)
	assert.Equal(t, "BTC-USDT", markets[0].Symbol)
}

func TestMarketService_Delete_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.01",
		MaxSize:        "100",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 删除
	err = svc.Delete(ctx, created.ID, 1)
	require.NoError(t, err)

	// 验证已删除
	market, err := svc.GetByID(ctx, created.ID)
	assert.NoError(t, err)
	assert.Nil(t, market)
}

func TestMarketService_Delete_NotFound(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	err := svc.Delete(ctx, 9999, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "交易对不存在")
}

func TestMarketService_GetAllActive_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建多个交易对
	symbols := []string{"BTC-USDT", "ETH-USDT", "SOL-USDT"}
	for i, symbol := range symbols {
		createReq := &CreateMarketRequest{
			Symbol:         symbol,
			BaseToken:      symbol[:3],
			QuoteToken:     "USDT",
			BaseTokenAddr:  "0x123456789012345678901234567890123456789" + string(rune('0'+i)),
			QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
			MinSize:        "0.01",
			MaxSize:        "100",
			MinNotional:    "10",
			TickSize:       "0.01",
			MakerFee:       "0.001",
			TakerFee:       "0.002",
			OperatorID:     1,
		}
		_, err := svc.Create(ctx, createReq)
		require.NoError(t, err)
	}

	// 激活部分交易对
	for _, symbol := range []string{"BTC-USDT", "ETH-USDT"} {
		statusReq := &UpdateStatusRequest{
			Symbol:         symbol,
			Status:         model.MarketStatusActive,
			TradingEnabled: true,
			OperatorID:     1,
		}
		err := svc.UpdateStatus(ctx, statusReq)
		require.NoError(t, err)
	}

	// 获取所有活跃交易对
	markets, err := svc.GetAllActive(ctx)
	require.NoError(t, err)
	assert.Len(t, markets, 2)
}

func TestMarketService_GetByID_Success(t *testing.T) {
	db := setupMarketTestDB(t)
	marketRepo := repository.NewMarketConfigRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	svc := NewMarketService(marketRepo, configRepo, auditRepo)

	ctx := context.Background()

	// 创建交易对
	createReq := &CreateMarketRequest{
		Symbol:         "BTC-USDT",
		BaseToken:      "BTC",
		QuoteToken:     "USDT",
		BaseTokenAddr:  "0x1234567890123456789012345678901234567890",
		QuoteTokenAddr: "0x0987654321098765432109876543210987654321",
		MinSize:        "0.01",
		MaxSize:        "100",
		MinNotional:    "10",
		TickSize:       "0.01",
		MakerFee:       "0.001",
		TakerFee:       "0.002",
		OperatorID:     1,
	}

	created, err := svc.Create(ctx, createReq)
	require.NoError(t, err)

	// 通过 ID 获取
	market, err := svc.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, market)
	assert.Equal(t, "BTC-USDT", market.Symbol)
}

func TestMarketStatus_Values(t *testing.T) {
	assert.Equal(t, model.MarketStatus(1), model.MarketStatusActive)
	assert.Equal(t, model.MarketStatus(2), model.MarketStatusSuspended)
	assert.Equal(t, model.MarketStatus(3), model.MarketStatusOffline)
}
