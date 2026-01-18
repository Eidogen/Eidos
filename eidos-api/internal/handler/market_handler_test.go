package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// MockMarketService Mock 行情服务
type MockMarketService struct {
	mock.Mock
}

func (m *MockMarketService) ListMarkets(ctx *gin.Context) ([]*dto.MarketResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.MarketResponse), args.Error(1)
}

func (m *MockMarketService) GetTicker(ctx *gin.Context, market string) (*dto.TickerResponse, error) {
	args := m.Called(ctx, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TickerResponse), args.Error(1)
}

func (m *MockMarketService) ListTickers(ctx *gin.Context) ([]*dto.TickerResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.TickerResponse), args.Error(1)
}

func (m *MockMarketService) GetDepth(ctx *gin.Context, market string, limit int) (*dto.DepthResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepthResponse), args.Error(1)
}

func (m *MockMarketService) GetKlines(ctx *gin.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error) {
	args := m.Called(ctx, market, interval, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.KlineResponse), args.Error(1)
}

func (m *MockMarketService) GetRecentTrades(ctx *gin.Context, market string, limit int) ([]*dto.RecentTradeResponse, error) {
	args := m.Called(ctx, market, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.RecentTradeResponse), args.Error(1)
}

// setupMarketHandler 设置测试用的路由和 Handler
func setupMarketHandler(svc MarketService) (*gin.Engine, *MarketHandler) {
	r := gin.New()
	h := NewMarketHandler(svc)
	return r, h
}

// ========== ListMarkets 测试 ==========

// TestListMarkets_WithService 测试有服务时获取交易对列表
func TestListMarkets_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedMarkets := []*dto.MarketResponse{
		{
			Symbol:         "BTC-USDC",
			BaseToken:      "BTC",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 6,
			MinAmount:      "0.0001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
		{
			Symbol:         "ETH-USDC",
			BaseToken:      "ETH",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 5,
			MinAmount:      "0.001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
	}

	mockSvc.On("ListMarkets", mock.Anything).Return(expectedMarkets, nil)

	r.GET("/markets", h.ListMarkets)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/markets", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestListMarkets_NilService 测试无服务时返回 Mock 数据
func TestListMarkets_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/markets", h.ListMarkets)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/markets", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
	// 应该返回 Mock 数据
	assert.NotNil(t, resp.Data)
}

// TestListMarkets_ServiceError 测试服务层错误
func TestListMarkets_ServiceError(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	mockSvc.On("ListMarkets", mock.Anything).Return(nil, dto.ErrInternalError)

	r.GET("/markets", h.ListMarkets)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/markets", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockSvc.AssertExpectations(t)
}

// ========== GetTicker 测试 ==========

// TestGetTicker_WithService 测试有服务时获取 Ticker
func TestGetTicker_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedTicker := &dto.TickerResponse{
		Market:             "BTC-USDC",
		LastPrice:          "42000.00",
		PriceChange:        "1000.00",
		PriceChangePercent: "2.44",
		High:               "43000.00",
		Low:                "41000.00",
		Volume:             "100.5",
		QuoteVolume:        "4221000.00",
		Open:               "41000.00",
		BestBid:            "41999.00",
		BestAsk:            "42001.00",
		Timestamp:          time.Now().UnixMilli(),
	}

	mockSvc.On("GetTicker", mock.Anything, "BTC-USDC").Return(expectedTicker, nil)

	r.GET("/ticker/:market", h.GetTicker)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ticker/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetTicker_NilService 测试无服务时返回 Mock 数据
func TestGetTicker_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/ticker/:market", h.GetTicker)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ticker/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
}

// TestGetTicker_EmptyMarket 测试空市场参数
func TestGetTicker_EmptyMarket(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	r.GET("/ticker/:market", h.GetTicker)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/ticker/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetTicker_MarketNotFound 测试市场不存在
func TestGetTicker_MarketNotFound(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	mockSvc.On("GetTicker", mock.Anything, "INVALID-MARKET").Return(nil, dto.ErrMarketNotFound)

	r.GET("/ticker/:market", h.GetTicker)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ticker/INVALID-MARKET", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockSvc.AssertExpectations(t)
}

// ========== ListTickers 测试 ==========

// TestListTickers_WithService 测试有服务时获取所有 Ticker
func TestListTickers_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedTickers := []*dto.TickerResponse{
		{
			Market:    "BTC-USDC",
			LastPrice: "42000.00",
		},
		{
			Market:    "ETH-USDC",
			LastPrice: "2500.00",
		},
	}

	mockSvc.On("ListTickers", mock.Anything).Return(expectedTickers, nil)

	r.GET("/tickers", h.ListTickers)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/tickers", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTickers_NilService 测试无服务时返回 Mock 数据
func TestListTickers_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/tickers", h.ListTickers)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/tickers", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
}

// ========== GetDepth 测试 ==========

// TestGetDepth_WithService 测试有服务时获取深度
func TestGetDepth_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedDepth := &dto.DepthResponse{
		Market: "BTC-USDC",
		Bids: [][]string{
			{"41999.00", "0.5"},
			{"41998.00", "1.2"},
		},
		Asks: [][]string{
			{"42001.00", "0.3"},
			{"42002.00", "0.8"},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	mockSvc.On("GetDepth", mock.Anything, "BTC-USDC", 20).Return(expectedDepth, nil)

	r.GET("/depth/:market", h.GetDepth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/depth/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDepth_WithLimit 测试带 limit 参数获取深度
func TestGetDepth_WithLimit(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedDepth := &dto.DepthResponse{
		Market:    "BTC-USDC",
		Bids:      [][]string{},
		Asks:      [][]string{},
		Timestamp: time.Now().UnixMilli(),
	}

	mockSvc.On("GetDepth", mock.Anything, "BTC-USDC", 50).Return(expectedDepth, nil)

	r.GET("/depth/:market", h.GetDepth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/depth/BTC-USDC?limit=50", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDepth_MaxLimit 测试超过最大 limit 限制
func TestGetDepth_MaxLimit(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedDepth := &dto.DepthResponse{
		Market:    "BTC-USDC",
		Bids:      [][]string{},
		Asks:      [][]string{},
		Timestamp: time.Now().UnixMilli(),
	}

	// 超过 100 的 limit 会被忽略，使用默认值 20
	mockSvc.On("GetDepth", mock.Anything, "BTC-USDC", 20).Return(expectedDepth, nil)

	r.GET("/depth/:market", h.GetDepth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/depth/BTC-USDC?limit=1000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDepth_NilService 测试无服务时返回空深度
func TestGetDepth_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/depth/:market", h.GetDepth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/depth/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
}

// TestGetDepth_EmptyMarket 测试空市场参数
func TestGetDepth_EmptyMarket(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	r.GET("/depth/:market", h.GetDepth)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/depth/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ========== GetKlines 测试 ==========

// TestGetKlines_WithService 测试有服务时获取 K 线
func TestGetKlines_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedKlines := []*dto.KlineResponse{
		{
			OpenTime: 1704067200000,
			Open:     "42000.00",
			High:     "42500.00",
			Low:      "41500.00",
			Close:    "42300.00",
			Volume:   "10.5",
		},
		{
			OpenTime: 1704067260000,
			Open:     "42300.00",
			High:     "42600.00",
			Low:      "42100.00",
			Close:    "42500.00",
			Volume:   "8.2",
		},
	}

	mockSvc.On("GetKlines", mock.Anything, "BTC-USDC", "1m", int64(0), int64(0), 100).Return(expectedKlines, nil)

	r.GET("/klines/:market", h.GetKlines)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/klines/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetKlines_WithParams 测试带参数获取 K 线
func TestGetKlines_WithParams(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedKlines := []*dto.KlineResponse{}

	mockSvc.On("GetKlines", mock.Anything, "BTC-USDC", "1h", int64(1704067200000), int64(1704153600000), 500).Return(expectedKlines, nil)

	r.GET("/klines/:market", h.GetKlines)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/klines/BTC-USDC?interval=1h&start_time=1704067200000&end_time=1704153600000&limit=500", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetKlines_MaxLimit 测试超过最大 limit 限制
func TestGetKlines_MaxLimit(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedKlines := []*dto.KlineResponse{}

	// 超过 1000 的 limit 会被忽略，使用默认值 100
	mockSvc.On("GetKlines", mock.Anything, "BTC-USDC", "1m", int64(0), int64(0), 100).Return(expectedKlines, nil)

	r.GET("/klines/:market", h.GetKlines)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/klines/BTC-USDC?limit=5000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetKlines_NilService 测试无服务时返回空数据
func TestGetKlines_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/klines/:market", h.GetKlines)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/klines/BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
}

// TestGetKlines_EmptyMarket 测试空市场参数
func TestGetKlines_EmptyMarket(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	r.GET("/klines/:market", h.GetKlines)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/klines/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ========== GetRecentTrades 测试 ==========

// TestGetRecentTrades_WithService 测试有服务时获取最近成交
func TestGetRecentTrades_WithService(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedTrades := []*dto.RecentTradeResponse{
		{
			TradeID:   "trade-1",
			Market:    "BTC-USDC",
			Price:     "42000.00",
			Amount:    "0.1",
			Side:      "buy",
			Timestamp: 1704067200000,
		},
		{
			TradeID:   "trade-2",
			Market:    "BTC-USDC",
			Price:     "41999.00",
			Amount:    "0.05",
			Side:      "sell",
			Timestamp: 1704067199000,
		},
	}

	mockSvc.On("GetRecentTrades", mock.Anything, "BTC-USDC", 50).Return(expectedTrades, nil)

	r.GET("/trades/:market/recent", h.GetRecentTrades)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/BTC-USDC/recent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetRecentTrades_WithLimit 测试带 limit 参数获取最近成交
func TestGetRecentTrades_WithLimit(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedTrades := []*dto.RecentTradeResponse{}

	mockSvc.On("GetRecentTrades", mock.Anything, "BTC-USDC", 20).Return(expectedTrades, nil)

	r.GET("/trades/:market/recent", h.GetRecentTrades)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/BTC-USDC/recent?limit=20", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetRecentTrades_MaxLimit 测试超过最大 limit 限制
func TestGetRecentTrades_MaxLimit(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	expectedTrades := []*dto.RecentTradeResponse{}

	// 超过 100 的 limit 会被忽略，使用默认值 50
	mockSvc.On("GetRecentTrades", mock.Anything, "BTC-USDC", 50).Return(expectedTrades, nil)

	r.GET("/trades/:market/recent", h.GetRecentTrades)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/BTC-USDC/recent?limit=500", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetRecentTrades_NilService 测试无服务时返回空数据
func TestGetRecentTrades_NilService(t *testing.T) {
	r, h := setupMarketHandler(nil)

	r.GET("/trades/:market/recent", h.GetRecentTrades)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/BTC-USDC/recent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
}

// TestGetRecentTrades_EmptyMarket 测试空市场参数
// 注意：Gin 会将 "/trades//recent" 匹配到路由，但 market 参数为空字符串
// Handler 会检查并返回 400 ErrInvalidParams
func TestGetRecentTrades_EmptyMarket(t *testing.T) {
	mockSvc := new(MockMarketService)
	r, h := setupMarketHandler(mockSvc)

	r.GET("/trades/:market/recent", h.GetRecentTrades)

	w := httptest.NewRecorder()
	// Gin 会匹配此路由，market 参数为空字符串
	req, _ := http.NewRequest(http.MethodGet, "/trades//recent", nil)
	r.ServeHTTP(w, req)

	// Handler 检查 market 为空并返回 400
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
}
