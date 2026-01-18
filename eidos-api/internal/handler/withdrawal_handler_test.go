package handler

import (
	"bytes"
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

// MockWithdrawalService Mock 提现服务
type MockWithdrawalService struct {
	mock.Mock
}

func (m *MockWithdrawalService) CreateWithdrawal(ctx *gin.Context, req *dto.CreateWithdrawalRequest) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

func (m *MockWithdrawalService) GetWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

func (m *MockWithdrawalService) ListWithdrawals(ctx *gin.Context, req *dto.ListWithdrawalsRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

func (m *MockWithdrawalService) CancelWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error) {
	args := m.Called(ctx, withdrawID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.WithdrawalResponse), args.Error(1)
}

// setupWithdrawalHandler 设置测试用的路由和 Handler
func setupWithdrawalHandler(svc *MockWithdrawalService) (*gin.Engine, *WithdrawalHandler) {
	r := gin.New()
	h := NewWithdrawalHandler(svc)
	return r, h
}

// TestCreateWithdrawal_Success 测试创建提现成功
func TestCreateWithdrawal_Success(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"
	toAddress := "0x9876543210987654321098765432109876543210"

	expectedResp := &dto.WithdrawalResponse{
		WithdrawID:        "wd-123",
		Wallet:    wallet,
		Token:     "USDC",
		Amount:    "500.00",
		ToAddress: toAddress,
		Status:    "pending",
		CreatedAt: 1704067200000,
	}

	mockSvc.On("CreateWithdrawal", mock.Anything, mock.MatchedBy(func(req *dto.CreateWithdrawalRequest) bool {
		return req.Wallet == wallet &&
			req.Token == "USDC" &&
			req.Amount == "500.00" &&
			req.ToAddress == toAddress
	})).Return(expectedResp, nil)

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	reqBody := map[string]interface{}{
		"token":      "USDC",
		"amount":     "500.00",
		"to_address": toAddress,
		"nonce":      123456,
		"signature":  "0x1234567890",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestCreateWithdrawal_NoWallet 测试创建提现无钱包
func TestCreateWithdrawal_NoWallet(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	r.POST("/withdrawals", h.CreateWithdrawal)

	reqBody := map[string]interface{}{
		"token":      "USDC",
		"amount":     "500.00",
		"to_address": "0x9876543210987654321098765432109876543210",
		"signature":  "0x1234567890",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestCreateWithdrawal_MissingToken 测试创建提现缺少 Token
func TestCreateWithdrawal_MissingToken(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	reqBody := map[string]interface{}{
		"amount":     "500.00",
		"to_address": "0x9876543210987654321098765432109876543210",
		"signature":  "0x1234567890",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
}

// TestCreateWithdrawal_MissingAmount 测试创建提现缺少 Amount
func TestCreateWithdrawal_MissingAmount(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	reqBody := map[string]interface{}{
		"token":      "USDC",
		"to_address": "0x9876543210987654321098765432109876543210",
		"signature":  "0x1234567890",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateWithdrawal_InvalidAddress 测试创建提现无效地址
func TestCreateWithdrawal_InvalidAddress(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	tests := []struct {
		name      string
		toAddress string
	}{
		{"empty", ""},
		{"too_short", "0x123"},
		{"missing_prefix", "1234567890123456789012345678901234567890"},
		{"too_long", "0x12345678901234567890123456789012345678901234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]interface{}{
				"token":      "USDC",
				"amount":     "500.00",
				"to_address": tt.toAddress,
				"signature":  "0x1234567890",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestCreateWithdrawal_MissingSignature 测试创建提现缺少签名
func TestCreateWithdrawal_MissingSignature(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	reqBody := map[string]interface{}{
		"token":      "USDC",
		"amount":     "500.00",
		"to_address": "0x9876543210987654321098765432109876543210",
		"nonce":      123456,
		// missing signature
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// signature is required in binding, returns 400 for missing required field
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
}

// TestCreateWithdrawal_InsufficientBalance 测试创建提现余额不足
func TestCreateWithdrawal_InsufficientBalance(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("CreateWithdrawal", mock.Anything, mock.Anything).Return(nil, dto.ErrInsufficientBalance)

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	reqBody := map[string]interface{}{
		"token":      "USDC",
		"amount":     "999999.00",
		"to_address": "0x9876543210987654321098765432109876543210",
		"nonce":      123456,
		"signature":  "0x1234567890",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrInsufficientBalance.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetWithdrawal_Success 测试获取提现详情成功
func TestGetWithdrawal_Success(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	expectedResp := &dto.WithdrawalResponse{
		WithdrawID:        "wd-123",
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDC",
		Amount:    "500.00",
		ToAddress: "0x9876543210987654321098765432109876543210",
		Status:    "completed",
		TxHash:    "0xabcd1234",
		CreatedAt: 1704067200000,
	}

	mockSvc.On("GetWithdrawal", mock.Anything, "wd-123").Return(expectedResp, nil)

	r.GET("/withdrawals/:id", h.GetWithdrawal)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals/wd-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetWithdrawal_NotFound 测试获取提现详情不存在
func TestGetWithdrawal_NotFound(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	mockSvc.On("GetWithdrawal", mock.Anything, "wd-nonexistent").Return(nil, dto.ErrWithdrawNotFound)

	r.GET("/withdrawals/:id", h.GetWithdrawal)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals/wd-nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrWithdrawNotFound.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetWithdrawal_EmptyID 测试获取提现详情空ID
func TestGetWithdrawal_EmptyID(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	r.GET("/withdrawals/:id", h.GetWithdrawal)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestListWithdrawals_Success 测试查询提现记录成功
func TestListWithdrawals_Success(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.WithdrawalResponse{
			{
				WithdrawID:     "wd-1",
				Wallet: wallet,
				Token:  "USDC",
				Amount: "500.00",
				Status: "completed",
			},
			{
				WithdrawID:     "wd-2",
				Wallet: wallet,
				Token:  "BTC",
				Amount: "0.1",
				Status: "pending",
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListWithdrawals", mock.Anything, mock.MatchedBy(func(req *dto.ListWithdrawalsRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	r.GET("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListWithdrawals(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListWithdrawals_NoWallet 测试查询提现记录无钱包
func TestListWithdrawals_NoWallet(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	r.GET("/withdrawals", h.ListWithdrawals)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListWithdrawals_WithFilters 测试带过滤条件的提现记录查询
func TestListWithdrawals_WithFilters(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.WithdrawalResponse{},
		Total:      0,
		Page:       2,
		PageSize:   10,
		TotalPages: 0,
	}

	mockSvc.On("ListWithdrawals", mock.Anything, mock.MatchedBy(func(req *dto.ListWithdrawalsRequest) bool {
		return req.Wallet == wallet &&
			req.Token == "USDC" &&
			req.Status == "pending" &&
			req.Page == 2 &&
			req.PageSize == 10 &&
			req.StartTime == 1704067200000 &&
			req.EndTime == 1704153600000
	})).Return(expectedResp, nil)

	r.GET("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListWithdrawals(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals?token=USDC&status=pending&page=2&page_size=10&start_time=1704067200000&end_time=1704153600000", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelWithdrawal_Success 测试取消提现成功
func TestCancelWithdrawal_Success(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	expectedResp := &dto.WithdrawalResponse{
		WithdrawID:        "wd-123",
		Wallet:    "0x1234567890123456789012345678901234567890",
		Token:     "USDC",
		Amount:    "500.00",
		Status:    "cancelled",
		CreatedAt: 1704067200000,
	}

	mockSvc.On("CancelWithdrawal", mock.Anything, "wd-123").Return(expectedResp, nil)

	r.DELETE("/withdrawals/:id", h.CancelWithdrawal)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/withdrawals/wd-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelWithdrawal_NotCancellable 测试取消提现不可取消
func TestCancelWithdrawal_NotCancellable(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	mockSvc.On("CancelWithdrawal", mock.Anything, "wd-123").Return(nil, dto.ErrWithdrawNotCancellable)

	r.DELETE("/withdrawals/:id", h.CancelWithdrawal)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/withdrawals/wd-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrWithdrawNotCancellable.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelWithdrawal_NotFound 测试取消提现不存在
func TestCancelWithdrawal_NotFound(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	mockSvc.On("CancelWithdrawal", mock.Anything, "wd-nonexistent").Return(nil, dto.ErrWithdrawNotFound)

	r.DELETE("/withdrawals/:id", h.CancelWithdrawal)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/withdrawals/wd-nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelWithdrawal_EmptyID 测试取消提现空ID
func TestCancelWithdrawal_EmptyID(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	r.DELETE("/withdrawals/:id", h.CancelWithdrawal)

	w := httptest.NewRecorder()
	// 路由不匹配
	req, _ := http.NewRequest(http.MethodDelete, "/withdrawals/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestValidateCreateWithdrawalRequest 测试提现请求验证函数
func TestValidateCreateWithdrawalRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *dto.CreateWithdrawalRequest
		wantErr *dto.BizError
	}{
		{
			name: "valid_request",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "100.00",
				ToAddress: "0x1234567890123456789012345678901234567890",
				Signature: "0xabc123",
			},
			wantErr: nil,
		},
		{
			name: "empty_token",
			req: &dto.CreateWithdrawalRequest{
				Token:     "",
				Amount:    "100.00",
				ToAddress: "0x1234567890123456789012345678901234567890",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidParams,
		},
		{
			name: "empty_amount",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "",
				ToAddress: "0x1234567890123456789012345678901234567890",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidParams,
		},
		{
			name: "empty_to_address",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "100.00",
				ToAddress: "",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidWithdrawAddress,
		},
		{
			name: "short_address",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "100.00",
				ToAddress: "0x1234",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidWithdrawAddress,
		},
		{
			name: "address_without_prefix",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "100.00",
				ToAddress: "1x1234567890123456789012345678901234567890",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidWithdrawAddress,
		},
		{
			name: "empty_signature",
			req: &dto.CreateWithdrawalRequest{
				Token:     "USDC",
				Amount:    "100.00",
				ToAddress: "0x1234567890123456789012345678901234567890",
				Signature: "",
			},
			wantErr: dto.ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateWithdrawalRequest(tt.req)
			if tt.wantErr == nil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Equal(t, tt.wantErr.Code, err.Code)
			}
		})
	}
}

// TestCreateWithdrawal_InvalidJSON 测试创建提现无效 JSON
func TestCreateWithdrawal_InvalidJSON(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	r.POST("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.CreateWithdrawal(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/withdrawals", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestListWithdrawals_ServiceError 测试查询提现列表服务错误
func TestListWithdrawals_ServiceError(t *testing.T) {
	mockSvc := new(MockWithdrawalService)
	r, h := setupWithdrawalHandler(mockSvc)

	wallet := "0x1234567890123456789012345678901234567890"

	mockSvc.On("ListWithdrawals", mock.Anything, mock.Anything).Return(nil, dto.ErrInternalError)

	r.GET("/withdrawals", func(c *gin.Context) {
		c.Set("wallet", wallet)
		h.ListWithdrawals(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/withdrawals", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}
