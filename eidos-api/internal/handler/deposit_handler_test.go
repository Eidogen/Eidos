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

// MockDepositService Mock 充值服务
type MockDepositService struct {
	mock.Mock
}

func (m *MockDepositService) GetDeposit(ctx *gin.Context, depositID string) (*dto.DepositResponse, error) {
	args := m.Called(ctx, depositID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.DepositResponse), args.Error(1)
}

func (m *MockDepositService) ListDeposits(ctx *gin.Context, req *dto.ListDepositsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

// setupDepositHandler 设置测试用的路由和 Handler
func setupDepositHandler(svc *MockDepositService) (*gin.Engine, *DepositHandler) {
	r := gin.New()
	h := NewDepositHandler(svc)
	return r, h
}

// TestGetDeposit_Success 测试获取充值详情成功
func TestGetDeposit_Success(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	expectedDeposit := &dto.DepositResponse{
		DepositID: "dep-123",
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDC",
		Amount:    "1000.00",
		TxHash:    "0xabcd1234",
		Status:    "confirmed",
		CreatedAt: 1704067200000,
	}

	mockSvc.On("GetDeposit", mock.Anything, "dep-123").Return(expectedDeposit, nil)

	r.GET("/deposits/:id", h.GetDeposit)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits/dep-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDeposit_NotFound 测试获取充值详情不存在
func TestGetDeposit_NotFound(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	mockSvc.On("GetDeposit", mock.Anything, "dep-nonexistent").Return(nil, dto.ErrDepositNotFound)

	r.GET("/deposits/:id", h.GetDeposit)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits/dep-nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrDepositNotFound.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDeposit_EmptyID 测试获取充值详情空ID
func TestGetDeposit_EmptyID(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	r.GET("/deposits/:id", h.GetDeposit)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/deposits/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestListDeposits_Success 测试查询充值记录成功
func TestListDeposits_Success(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.DepositResponse{
			{
				DepositID: "dep-1",
				Wallet:    wallet,
				Token:     "USDC",
				Amount:    "1000.00",
				Status:    "confirmed",
			},
			{
				DepositID: "dep-2",
				Wallet:    wallet,
				Token:     "BTC",
				Amount:    "0.5",
				Status:    "pending",
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListDeposits", mock.Anything, mock.MatchedBy(func(req *dto.ListDepositsRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListDeposits_NoWallet 测试查询充值记录无钱包
func TestListDeposits_NoWallet(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	r.GET("/deposits", h.ListDeposits)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListDeposits_WithFilters 测试带过滤条件的充值记录查询
func TestListDeposits_WithFilters(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.DepositResponse{},
		Total:      0,
		Page:       2,
		PageSize:   10,
		TotalPages: 0,
	}

	mockSvc.On("ListDeposits", mock.Anything, mock.MatchedBy(func(req *dto.ListDepositsRequest) bool {
		return req.Wallet == wallet &&
			req.Token == "USDC" &&
			req.Status == "confirmed" &&
			req.Page == 2 &&
			req.PageSize == 10 &&
			req.StartTime == 1704067200000 &&
			req.EndTime == 1704153600000
	})).Return(expectedResp, nil)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits?token=USDC&status=confirmed&page=2&page_size=10&start_time=1704067200000&end_time=1704153600000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListDeposits_DefaultPagination 测试默认分页参数
func TestListDeposits_DefaultPagination(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.DepositResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListDeposits", mock.Anything, mock.MatchedBy(func(req *dto.ListDepositsRequest) bool {
		return req.Page == 1 && req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListDeposits_MaxPageSize 测试最大分页大小限制
func TestListDeposits_MaxPageSize(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.DepositResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListDeposits", mock.Anything, mock.MatchedBy(func(req *dto.ListDepositsRequest) bool {
		// 超过 100 的 page_size 会被忽略，使用默认值
		return req.PageSize == 20
	})).Return(expectedResp, nil)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits?page_size=1000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListDeposits_ServiceError 测试服务层错误
func TestListDeposits_ServiceError(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListDeposits", mock.Anything, mock.Anything).Return(nil, dto.ErrInternalError)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetDeposit_ServiceError 测试获取充值详情服务错误
func TestGetDeposit_ServiceError(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	mockSvc.On("GetDeposit", mock.Anything, "dep-error").Return(nil, dto.ErrInternalError)

	r.GET("/deposits/:id", h.GetDeposit)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits/dep-error", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListDeposits_Empty 测试空充值记录
func TestListDeposits_Empty(t *testing.T) {
	mockSvc := new(MockDepositService)
	r, h := setupDepositHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.DepositResponse{},
		Total:      0,
		Page:       1,
		PageSize:   20,
		TotalPages: 0,
	}

	mockSvc.On("ListDeposits", mock.Anything, mock.MatchedBy(func(req *dto.ListDepositsRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/deposits", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListDeposits(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/deposits", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}
