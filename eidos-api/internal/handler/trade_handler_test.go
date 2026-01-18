package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// MockTradeService Mock 成交服务
type MockTradeService struct {
	mock.Mock
}

func (m *MockTradeService) GetTrade(ctx *gin.Context, tradeID string) (*dto.TradeResponse, error) {
	args := m.Called(ctx, tradeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.TradeResponse), args.Error(1)
}

func (m *MockTradeService) ListTrades(ctx *gin.Context, req *dto.ListTradesRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// setupTradeHandler 设置测试用的路由和 Handler
func setupTradeHandler(svc *MockTradeService) (*gin.Engine, *TradeHandler) {
	r := gin.New()
	h := NewTradeHandler(svc)
	return r, h
}

// TestGetTrade_Success 测试获取成交详情成功
func TestGetTrade_Success(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	expectedTrade := &dto.TradeResponse{
		TradeID:      "trade-123",
		TakerOrderID: "order-456",
		Market:       "BTC-USDC",
		Side:         "buy",
		Price:        "42000.00",
		Amount:       "0.1",
		TakerFee:     "0.84",
		FeeToken:     "USDC",
		MatchedAt:    1704067200000,
	}

	mockSvc.On("GetTrade", mock.Anything, "trade-123").Return(expectedTrade, nil)

	r.GET("/trades/:id", h.GetTrade)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/trade-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetTrade_NotFound 测试获取成交详情不存在
func TestGetTrade_NotFound(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	mockSvc.On("GetTrade", mock.Anything, "trade-nonexistent").Return(nil, dto.ErrTradeNotFound)

	r.GET("/trades/:id", h.GetTrade)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/trade-nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrTradeNotFound.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetTrade_EmptyID 测试获取成交详情空ID
func TestGetTrade_EmptyID(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	r.GET("/trades/:id", h.GetTrade)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/trades/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestListTrades_Success 测试查询成交记录成功
func TestListTrades_Success(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.TradeResponse{
			{
				TradeID:      "trade-1",
				TakerOrderID: "order-1",
				Market:       "BTC-USDC",
				Side:         "buy",
				Price:        "42000.00",
				Amount:       "0.1",
				TakerFee:     "0.84",
				FeeToken:     "USDC",
			},
			{
				TradeID:      "trade-2",
				TakerOrderID: "order-2",
				Market:       "ETH-USDC",
				Side:         "sell",
				Price:        "2500.00",
				Amount:       "1.0",
				TakerFee:     "5.00",
				FeeToken:     "USDC",
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_NoWallet 测试查询成交记录无钱包
func TestListTrades_NoWallet(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	r.GET("/trades", h.ListTrades)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListTrades_WithFilters 测试带过滤条件的成交记录查询
func TestListTrades_WithFilters(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TradeResponse{},
		Total:      0,
		Page:       2,
		PageSize:   10,
		TotalPages: 0,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.Wallet == wallet &&
			req.Market == "BTC-USDC" &&
			req.OrderID == "order-123" &&
			req.Page == 2 &&
			req.PageSize == 10 &&
			req.StartTime == 1704067200000 &&
			req.EndTime == 1704153600000
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades?market=BTC-USDC&order_id=order-123&page=2&page_size=10&start_time=1704067200000&end_time=1704153600000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_DefaultPagination 测试默认分页参数
func TestListTrades_DefaultPagination(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TradeResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.Page == 1 && req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_MaxPageSize 测试最大分页大小限制
func TestListTrades_MaxPageSize(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TradeResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		// 超过 100 的 page_size 会被忽略，使用默认值
		return req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades?page_size=1000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_ServiceError 测试服务层错误
func TestListTrades_ServiceError(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListTrades", mock.Anything, mock.Anything).Return(nil, dto.ErrInternalError)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_ByMarket 测试按市场查询成交记录
func TestListTrades_ByMarket(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.TradeResponse{
			{
				TradeID: "trade-1",
				Market:  "BTC-USDC",
			},
		},
		Total:      1,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.Market == "BTC-USDC"
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades?market=BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_ByOrderID 测试按订单ID查询成交记录
func TestListTrades_ByOrderID(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.TradeResponse{
			{
				TradeID:      "trade-1",
				TakerOrderID: "order-123",
			},
			{
				TradeID:      "trade-2",
				TakerOrderID: "order-123",
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.OrderID == "order-123"
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades?order_id=order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetTrade_ServiceError 测试获取成交详情服务错误
func TestGetTrade_ServiceError(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	mockSvc.On("GetTrade", mock.Anything, "trade-error").Return(nil, dto.ErrInternalError)

	r.GET("/trades/:id", h.GetTrade)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades/trade-error", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTrades_Empty 测试空成交列表
func TestListTrades_Empty(t *testing.T) {
	mockSvc := new(MockTradeService)
	r, h := setupTradeHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TradeResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListTrades", mock.Anything, mock.MatchedBy(func(req *dto.ListTradesRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/trades", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTrades(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/trades", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}
