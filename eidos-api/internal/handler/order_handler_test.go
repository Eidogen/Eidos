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

func init() {
	gin.SetMode(gin.TestMode)
}

// MockOrderService Mock 订单服务
type MockOrderService struct {
	mock.Mock
}

func (m *MockOrderService) PrepareOrder(ctx *gin.Context, req *dto.PrepareOrderRequest) (*dto.PrepareOrderResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PrepareOrderResponse), args.Error(1)
}

func (m *MockOrderService) CreateOrder(ctx *gin.Context, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *MockOrderService) GetOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *MockOrderService) ListOrders(ctx *gin.Context, req *dto.ListOrdersRequest) (*dto.PaginatedResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PaginatedResponse), args.Error(1)
}

func (m *MockOrderService) ListOpenOrders(ctx *gin.Context, market string) ([]*dto.OrderResponse, error) {
	args := m.Called(ctx, market)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.OrderResponse), args.Error(1)
}

func (m *MockOrderService) CancelOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.OrderResponse), args.Error(1)
}

func (m *MockOrderService) BatchCancelOrders(ctx *gin.Context, req *dto.BatchCancelRequest) (*dto.BatchCancelResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.BatchCancelResponse), args.Error(1)
}

// setupOrderHandler 设置测试用的路由和 Handler
func setupOrderHandler(svc *MockOrderService) (*gin.Engine, *OrderHandler) {
	r := gin.New()
	h := NewOrderHandler(svc)
	return r, h
}

// TestGetOrder_Success 测试获取订单详情成功
func TestGetOrder_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedOrder := &dto.OrderResponse{
		OrderID: "order-123",
		Wallet:  "0x1234567890123456789012345678901234567890",
		Market:  "BTC-USDC",
		Side:    "buy",
		Type:    "limit",
		Price:   "42000.00",
		Amount:  "0.1",
		Status:  "open",
	}

	mockSvc.On("GetOrder", mock.Anything, "order-123").Return(expectedOrder, nil)

	r.GET("/orders/:id", h.GetOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetOrder_NotFound 测试获取不存在的订单
func TestGetOrder_NotFound(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("GetOrder", mock.Anything, "order-not-exist").Return(nil, dto.ErrOrderNotFound)

	r.GET("/orders/:id", h.GetOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/order-not-exist", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrOrderNotFound.Code, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestGetOrder_EmptyID 测试空订单 ID
func TestGetOrder_EmptyID(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.GET("/orders/:id", h.GetOrder)

	w := httptest.NewRecorder()
	// 使用空 ID 会导致路由不匹配，所以测试路径参数为空格
	req, _ := http.NewRequest(http.MethodGet, "/orders/", nil)
	r.ServeHTTP(w, req)

	// 空路径不会匹配到路由
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestCreateOrder_Success 测试创建订单成功
func TestCreateOrder_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedResp := &dto.OrderResponse{
		OrderID: "order-123",
		Status:  "pending",
	}

	mockSvc.On("CreateOrder", mock.Anything, mock.MatchedBy(func(req *dto.CreateOrderRequest) bool {
		return req.OrderID == "order-123" &&
			req.Market == "BTC-USDC" &&
			req.Side == "buy" &&
			req.Type == "limit" &&
			req.Price == "42000.00" &&
			req.Amount == "0.1" &&
			req.Nonce == uint64(123456)
	})).Return(expectedResp, nil)

	r.POST("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CreateOrder(c)
	})

	body := `{
		"order_id": "order-123",
		"market": "BTC-USDC",
		"side": "buy",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1",
		"nonce": 123456,
		"signature": "0xabc123"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Code)

	mockSvc.AssertExpectations(t)
}

// TestCreateOrder_NoWallet 测试创建订单无钱包
// 注意：binding 验证会先执行，所以缺少必填字段会返回 400 而非 401
func TestCreateOrder_NoWallet(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders", h.CreateOrder)

	// 提供完整请求体，但无 wallet
	body := `{
		"order_id": "order-123",
		"market": "BTC-USDC",
		"side": "buy",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1",
		"nonce": 123456,
		"signature": "0xabc123"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, dto.ErrUnauthorized.Code, resp.Code)
}

// TestCreateOrder_InvalidJSON 测试创建订单无效 JSON
func TestCreateOrder_InvalidJSON(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CreateOrder(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreateOrder_MissingFields 测试创建订单缺少必填字段
func TestCreateOrder_MissingFields(t *testing.T) {
	// 完整请求体的基础，每个测试用例缺少一个字段
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing_order_id",
			body: `{"market": "BTC-USDC", "side": "buy", "type": "limit", "price": "42000", "amount": "0.1", "nonce": 123456, "signature": "0x123"}`,
		},
		{
			name: "missing_market",
			body: `{"order_id": "order-123", "side": "buy", "type": "limit", "price": "42000", "amount": "0.1", "nonce": 123456, "signature": "0x123"}`,
		},
		{
			name: "missing_side",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "type": "limit", "price": "42000", "amount": "0.1", "nonce": 123456, "signature": "0x123"}`,
		},
		{
			name: "missing_type",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "side": "buy", "price": "42000", "amount": "0.1", "nonce": 123456, "signature": "0x123"}`,
		},
		{
			name: "missing_amount",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "side": "buy", "type": "limit", "price": "42000", "nonce": 123456, "signature": "0x123"}`,
		},
		{
			name: "missing_nonce",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "side": "buy", "type": "limit", "price": "42000", "amount": "0.1", "signature": "0x123"}`,
		},
		{
			name: "missing_signature",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "side": "buy", "type": "limit", "price": "42000", "amount": "0.1", "nonce": 123456}`,
		},
		{
			name: "limit_missing_price",
			body: `{"order_id": "order-123", "market": "BTC-USDC", "side": "buy", "type": "limit", "amount": "0.1", "nonce": 123456, "signature": "0x123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockOrderService)
			r, h := setupOrderHandler(mockSvc)

			r.POST("/orders", func(c *gin.Context) {
				c.Set("wallet", "0x1234567890123456789012345678901234567890")
				h.CreateOrder(c)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestCreateOrder_InvalidSide 测试无效的订单方向
// 注意：binding 验证 oneof=buy sell 会先执行，返回 ErrInvalidParams
func TestCreateOrder_InvalidSide(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CreateOrder(c)
	})

	body := `{
		"order_id": "order-123",
		"market": "BTC-USDC",
		"side": "invalid",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1",
		"nonce": 123456,
		"signature": "0xabc123"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	// binding 验证 oneof 失败返回 ErrInvalidParams
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
}

// TestCreateOrder_InvalidType 测试无效的订单类型
// 注意：binding 验证 oneof=limit market 会先执行，返回 ErrInvalidParams
func TestCreateOrder_InvalidType(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CreateOrder(c)
	})

	body := `{
		"order_id": "order-123",
		"market": "BTC-USDC",
		"side": "buy",
		"type": "invalid",
		"price": "42000.00",
		"amount": "0.1",
		"nonce": 123456,
		"signature": "0xabc123"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	// binding 验证 oneof 失败返回 ErrInvalidParams
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
}

// TestListOrders_Success 测试查询订单列表成功
func TestListOrders_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.OrderResponse{
			{OrderID: "order-1", Status: "open"},
			{OrderID: "order-2", Status: "filled"},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	mockSvc.On("ListOrders", mock.Anything, mock.MatchedBy(func(req *dto.ListOrdersRequest) bool {
		return req.Wallet == "0x1234567890123456789012345678901234567890"
	})).Return(expectedResp, nil)

	r.GET("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.ListOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListOrders_WithFilters 测试带过滤条件的订单列表查询
func TestListOrders_WithFilters(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.OrderResponse{},
		Total:      0,
		Page:       1,
		PageSize:   10,
		TotalPages: 0,
	}

	mockSvc.On("ListOrders", mock.Anything, mock.MatchedBy(func(req *dto.ListOrdersRequest) bool {
		return req.Market == "BTC-USDC" &&
			req.Side == "buy" &&
			req.Page == 2 &&
			req.PageSize == 10
	})).Return(expectedResp, nil)

	r.GET("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.ListOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders?market=BTC-USDC&side=buy&page=2&page_size=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_Success 测试取消订单成功
func TestCancelOrder_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedOrder := &dto.OrderResponse{
		OrderID: "order-123",
		Status:  "cancelled",
	}

	mockSvc.On("CancelOrder", mock.Anything, "order-123").Return(expectedOrder, nil)

	r.DELETE("/orders/:id", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CancelOrder(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders/order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_NotFound 测试取消不存在的订单
func TestCancelOrder_NotFound(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("CancelOrder", mock.Anything, "order-not-exist").Return(nil, dto.ErrOrderNotFound)

	r.DELETE("/orders/:id", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CancelOrder(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders/order-not-exist", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_AlreadyCancelled 测试取消已取消的订单
func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("CancelOrder", mock.Anything, "order-123").Return(nil, dto.ErrOrderAlreadyCancelled)

	r.DELETE("/orders/:id", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CancelOrder(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders/order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestListOpenOrders_Success 测试查询挂单列表成功
func TestListOpenOrders_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedOrders := []*dto.OrderResponse{
		{OrderID: "order-1", Status: "open"},
		{OrderID: "order-2", Status: "partial"},
	}

	mockSvc.On("ListOpenOrders", mock.Anything, "BTC-USDC").Return(expectedOrders, nil)

	r.GET("/orders/open", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.ListOpenOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/open?market=BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestBatchCancelOrders_Success 测试批量取消订单成功
func TestBatchCancelOrders_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	expectedResp := &dto.BatchCancelResponse{
		Cancelled: []string{"order-1", "order-2"},
		Failed:    []string{},
	}

	mockSvc.On("BatchCancelOrders", mock.Anything, mock.MatchedBy(func(req *dto.BatchCancelRequest) bool {
		return req.Market == "BTC-USDC"
	})).Return(expectedResp, nil)

	r.DELETE("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.BatchCancelOrders(c)
	})

	body := `{"market": "BTC-USDC"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockSvc.AssertExpectations(t)
}

// TestBatchCancelOrders_InvalidJSON_FallbackToQuery 测试批量取消无效 JSON 回退到查询参数
func TestBatchCancelOrders_InvalidJSON_FallbackToQuery(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	// 当 JSON 无效时，handler 会回退到查询参数，然后继续调用服务
	mockSvc.On("BatchCancelOrders", mock.Anything, mock.Anything).
		Return(&dto.BatchCancelResponse{Cancelled: []string{}, Failed: []string{}}, nil)

	r.DELETE("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.BatchCancelOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders?market=BTC-USDC", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 应该成功，因为 handler 回退到使用查询参数
	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestBatchCancelOrders_NoWallet 测试批量取消无钱包
func TestBatchCancelOrders_NoWallet(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.DELETE("/orders", h.BatchCancelOrders)

	body := `{"market": "BTC-USDC"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestBatchCancelOrders_ServiceError 测试批量取消服务错误
func TestBatchCancelOrders_ServiceError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("BatchCancelOrders", mock.Anything, mock.Anything).
		Return(nil, dto.ErrInternalError)

	r.DELETE("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.BatchCancelOrders(c)
	})

	body := `{"market": "BTC-USDC"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestListOrders_NoWallet 测试查询订单列表无钱包
func TestListOrders_NoWallet(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.GET("/orders", h.ListOrders)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListOrders_ServiceError 测试查询订单列表服务错误
func TestListOrders_ServiceError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("ListOrders", mock.Anything, mock.Anything).
		Return(nil, dto.ErrInternalError)

	r.GET("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.ListOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestListOpenOrders_NoWallet 测试查询挂单无钱包
func TestListOpenOrders_NoWallet(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.GET("/orders/open", h.ListOpenOrders)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/open?market=BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestListOpenOrders_ServiceError 测试查询挂单服务错误
func TestListOpenOrders_ServiceError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("ListOpenOrders", mock.Anything, "BTC-USDC").
		Return(nil, dto.ErrInternalError)

	r.GET("/orders/open", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.ListOpenOrders(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/open?market=BTC-USDC", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_EmptyOrderID 测试取消订单空 ID
func TestCancelOrder_EmptyOrderID(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	// 不使用 :id 参数，直接调用处理程序
	r.DELETE("/orders", func(c *gin.Context) {
		h.CancelOrder(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders", nil)
	r.ServeHTTP(w, req)

	// order id 为空时应该返回 400
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestValidatePrepareOrderRequest 测试准备订单请求验证
func TestValidatePrepareOrderRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *dto.PrepareOrderRequest
		wantErr *dto.BizError
	}{
		{
			name: "valid_limit_order",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "buy",
				Type:   "limit",
				Price:  "42000.00",
				Amount: "0.1",
			},
			wantErr: nil,
		},
		{
			name: "valid_market_order",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "sell",
				Type:   "market",
				Amount: "0.1",
			},
			wantErr: nil,
		},
		{
			name: "missing_market",
			req: &dto.PrepareOrderRequest{
				Side:   "buy",
				Type:   "limit",
				Price:  "42000.00",
				Amount: "0.1",
			},
			wantErr: dto.ErrInvalidParams,
		},
		{
			name: "invalid_side",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "invalid",
				Type:   "limit",
				Price:  "42000.00",
				Amount: "0.1",
			},
			wantErr: dto.ErrInvalidOrderSide,
		},
		{
			name: "invalid_type",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "buy",
				Type:   "invalid",
				Price:  "42000.00",
				Amount: "0.1",
			},
			wantErr: dto.ErrInvalidOrderType,
		},
		{
			name: "limit_without_price",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "buy",
				Type:   "limit",
				Amount: "0.1",
			},
			wantErr: dto.ErrInvalidPrice,
		},
		{
			name: "missing_amount",
			req: &dto.PrepareOrderRequest{
				Market: "BTC-USDC",
				Side:   "buy",
				Type:   "limit",
				Price:  "42000.00",
			},
			wantErr: dto.ErrInvalidAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePrepareOrderRequest(tt.req)
			if tt.wantErr == nil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Equal(t, tt.wantErr.Code, err.Code)
			}
		})
	}
}

// TestValidateCreateOrderRequest 测试创建订单请求验证
func TestValidateCreateOrderRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *dto.CreateOrderRequest
		wantErr *dto.BizError
	}{
		{
			name: "valid_limit_order",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "buy",
				Type:      "limit",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: nil,
		},
		{
			name: "valid_market_order",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "sell",
				Type:      "market",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: nil,
		},
		{
			name: "missing_market",
			req: &dto.CreateOrderRequest{
				Side:      "buy",
				Type:      "limit",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidParams,
		},
		{
			name: "empty_side",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "",
				Type:      "limit",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidOrderSide,
		},
		{
			name: "invalid_side",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "invalid",
				Type:      "limit",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidOrderSide,
		},
		{
			name: "empty_type",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "buy",
				Type:      "",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidOrderType,
		},
		{
			name: "invalid_type",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "buy",
				Type:      "invalid",
				Price:     "42000.00",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidOrderType,
		},
		{
			name: "limit_without_price",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "buy",
				Type:      "limit",
				Amount:    "0.1",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidPrice,
		},
		{
			name: "missing_amount",
			req: &dto.CreateOrderRequest{
				Market:    "BTC-USDC",
				Side:      "buy",
				Type:      "limit",
				Price:     "42000.00",
				Signature: "0xabc123",
			},
			wantErr: dto.ErrInvalidAmount,
		},
		{
			name: "missing_signature",
			req: &dto.CreateOrderRequest{
				Market: "BTC-USDC",
				Side:   "buy",
				Type:   "limit",
				Price:  "42000.00",
				Amount: "0.1",
			},
			wantErr: dto.ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreateOrderRequest(tt.req)
			if tt.wantErr == nil {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Equal(t, tt.wantErr.Code, err.Code)
			}
		})
	}
}

// TestHandleServiceError 测试服务错误处理
func TestHandleServiceError(t *testing.T) {
	t.Run("biz_error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		handleServiceError(c, dto.ErrOrderNotFound)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp dto.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, dto.ErrOrderNotFound.Code, resp.Code)
	})

	t.Run("generic_error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		handleServiceError(c, assert.AnError)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp dto.Response
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, dto.ErrInternalError.Code, resp.Code)
	})
}

// TestCreateOrder_ServiceError 测试创建订单服务错误
func TestCreateOrder_ServiceError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("CreateOrder", mock.Anything, mock.Anything).
		Return(nil, dto.ErrInsufficientBalance)

	r.POST("/orders", func(c *gin.Context) {
		c.Set("wallet", "0x1234567890123456789012345678901234567890")
		h.CreateOrder(c)
	})

	body := `{
		"order_id": "order-123",
		"market": "BTC-USDC",
		"side": "buy",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1",
		"nonce": 123456,
		"signature": "0xabc123"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestPrepareOrder_Success 测试准备订单成功
func TestPrepareOrder_Success(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("PrepareOrder", mock.Anything, mock.Anything).
		Return(&dto.PrepareOrderResponse{
			TypedData: "typed-data-hash",
		}, nil)

	r.POST("/orders/prepare", h.PrepareOrder)

	body := `{
		"market": "BTC-USDC",
		"side": "buy",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders/prepare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestPrepareOrder_InvalidJSON 测试准备订单无效 JSON
func TestPrepareOrder_InvalidJSON(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders/prepare", h.PrepareOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders/prepare", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestPrepareOrder_ValidationError 测试准备订单验证错误
func TestPrepareOrder_ValidationError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	r.POST("/orders/prepare", h.PrepareOrder)

	// 缺少必填字段
	body := `{"side": "buy"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders/prepare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestPrepareOrder_ServiceError 测试准备订单服务错误
func TestPrepareOrder_ServiceError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("PrepareOrder", mock.Anything, mock.Anything).
		Return(nil, dto.ErrNotImplemented)

	r.POST("/orders/prepare", h.PrepareOrder)

	body := `{
		"market": "BTC-USDC",
		"side": "buy",
		"type": "limit",
		"price": "42000.00",
		"amount": "0.1"
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/orders/prepare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestGetOrder_ServiceNotFound 测试获取不存在的订单 (服务层错误)
func TestGetOrder_ServiceNotFound(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("GetOrder", mock.Anything, "order-notfound").
		Return(nil, dto.ErrOrderNotFound)

	r.GET("/orders/:id", h.GetOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/orders/order-notfound", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_SuccessResponse 测试取消订单成功响应
func TestCancelOrder_SuccessResponse(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("CancelOrder", mock.Anything, "order-123").
		Return(&dto.OrderResponse{
			OrderID: "order-123",
			Status:  "cancelled",
		}, nil)

	r.DELETE("/orders/:id", h.CancelOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders/order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestCancelOrder_NotFoundError 测试取消订单未找到错误
func TestCancelOrder_NotFoundError(t *testing.T) {
	mockSvc := new(MockOrderService)
	r, h := setupOrderHandler(mockSvc)

	mockSvc.On("CancelOrder", mock.Anything, "order-123").
		Return(nil, dto.ErrOrderNotFound)

	r.DELETE("/orders/:id", h.CancelOrder)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/orders/order-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}
