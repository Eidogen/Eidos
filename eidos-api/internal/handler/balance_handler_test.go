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

// MockBalanceService Mock 余额服务
type MockBalanceService struct {
	mock.Mock
}

func (m *MockBalanceService) GetBalance(ctx *gin.Context, wallet, token string) (*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.BalanceResponse), args.Error(1)
}

func (m *MockBalanceService) ListBalances(ctx *gin.Context, wallet string) ([]*dto.BalanceResponse, error) {
	args := m.Called(ctx, wallet)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.BalanceResponse), args.Error(1)
}

func (m *MockBalanceService) ListTransactions(ctx *gin.Context, req *dto.ListTransactionsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// setupBalanceHandler 设置测试用的路由和 Handler
func setupBalanceHandler(svc *MockBalanceService) (*gin.Engine, *BalanceHandler) {
	r := gin.New()
	h := NewBalanceHandler(svc)
	return r, h
}

// TestGetBalance_Success 测试获取余额成功
func TestGetBalance_Success(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDC"

	expectedBalance := &dto.BalanceResponse{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: "10000.00",
		SettledFrozen:    "500.00",
		TotalAvailable:   "10000.00",
		Total:            "10500.00",
		Withdrawable:     "10000.00",
	}

	mockSvc.On("GetBalance", mock.Anything, wallet, token).Return(expectedBalance, nil)

	r.GET("/balances/:token", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.GetBalance(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances/USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetBalance_NoWallet 测试获取余额无钱包
func TestGetBalance_NoWallet(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	r.GET("/balances/:token", h.GetBalance)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances/USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrUnauthorized.Code, resp.Code)
}

// TestGetBalance_EmptyToken 测试获取余额空代币
func TestGetBalance_EmptyToken(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	r.GET("/balances/:token", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.GetBalance(c)
	})

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/balances/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestListBalances_Success 测试获取所有余额成功
func TestListBalances_Success(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedBalances := []*dto.BalanceResponse{
		{
			Wallet:           wallet,
			Token:            "BTC",
			SettledAvailable: "1.5",
			Total:            "1.5",
		},
		{
			Wallet:           wallet,
			Token:            "USDC",
			SettledAvailable: "10000.00",
			Total:            "10000.00",
		},
	}

	mockSvc.On("ListBalances", mock.Anything, wallet).Return(expectedBalances, nil)

	r.GET("/balances", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListBalances(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestListBalances_NoWallet 测试获取所有余额无钱包
func TestListBalances_NoWallet(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	r.GET("/balances", h.ListBalances)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListBalances_Empty 测试获取空余额列表
func TestListBalances_Empty(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListBalances", mock.Anything, wallet).Return([]*dto.BalanceResponse{}, nil)

	r.GET("/balances", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListBalances(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTransactions_Success 测试查询资金流水成功
func TestListTransactions_Success(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.TransactionResponse{
			{
				ID:     "tx-1",
				Wallet: wallet,
				Token:  "USDC",
				Type:   "deposit",
				Amount: "1000.00",
			},
			{
				ID:     "tx-2",
				Wallet: wallet,
				Token:  "USDC",
				Type:   "trade",
				Amount: "-500.00",
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/transactions", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTransactions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTransactions_WithFilters 测试带过滤条件的资金流水查询
func TestListTransactions_WithFilters(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TransactionResponse{},
		Total:      0,
		Page:       1,
		PageSize:   10,
		TotalPages: 0,
	}

	mockSvc.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		return req.Wallet == wallet &&
			req.Token == "BTC" &&
			req.Type == "trade" &&
			req.Page == 2 &&
			req.PageSize == 10
	})).Return(expectedResp, nil)

	r.GET("/transactions", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTransactions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions?token=BTC&type=trade&page=2&page_size=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTransactions_NoWallet 测试查询资金流水无钱包
func TestListTransactions_NoWallet(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	r.GET("/transactions", h.ListTransactions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListTransactions_DefaultPagination 测试默认分页参数
func TestListTransactions_DefaultPagination(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TransactionResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		return req.Page == 1 && req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/transactions", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTransactions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListTransactions_MaxPageSize 测试最大分页大小限制
func TestListTransactions_MaxPageSize(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TransactionResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20, // 超过 100 的会被忽略，使用默认值
		TotalPages: 0,
	}

	mockSvc.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		// 超过 100 的 page_size 会被忽略
		return req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/transactions", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTransactions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions?page_size=1000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetBalance_ServiceError 测试服务层错误
func TestGetBalance_ServiceError(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("GetBalance", mock.Anything, wallet, "INVALID").Return(nil, dto.ErrTokenNotSupported)

	r.GET("/balances/:token", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.GetBalance(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances/INVALID", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrTokenNotSupported.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestListBalances_ServiceError 测试获取余额列表服务错误
func TestListBalances_ServiceError(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListBalances", mock.Anything, wallet).Return(nil, dto.ErrInternalError)

	r.GET("/balances", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListBalances(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/balances", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestListTransactions_ServiceError 测试查询资金流水服务错误
func TestListTransactions_ServiceError(t *testing.T) {
	mockSvc := new(MockBalanceService)
	r, h := setupBalanceHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListTransactions", mock.Anything, mock.Anything).Return(nil, dto.ErrInternalError)

	r.GET("/transactions", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListTransactions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/transactions", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}
