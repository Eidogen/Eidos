//go:build integration

package integration

import (
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// TestCreateOrder_Success 测试创建订单成功
func (s *IntegrationSuite) TestCreateOrder_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.OrderResponse{
		OrderID:   "order-123",
		Market:    "BTC-USDC",
		Side:      "buy",
		Type:      "limit",
		Price:     "42000.00",
		Amount:    "0.1",
		Status:    "pending",
		CreatedAt: 1704067200000,
	}

	s.orderService.On("CreateOrder", mock.Anything, mock.MatchedBy(func(req *dto.CreateOrderRequest) bool {
		return req.Market == "BTC-USDC" && req.Side == "buy"
	})).Return(expectedResp, nil)

	body := map[string]interface{}{
		"order_id":  "order-123",
		"market":    "BTC-USDC",
		"side":      "buy",
		"type":      "limit",
		"price":     "42000.00",
		"amount":    "0.1",
		"nonce":     123456,
		"signature": "0xmocksig",
	}

	w := s.Request(http.MethodPost, "/api/v1/orders", body, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestCreateOrder_Unauthorized 测试未认证创建订单
func (s *IntegrationSuite) TestCreateOrder_Unauthorized() {
	body := map[string]interface{}{
		"order_id":  "order-123",
		"market":    "BTC-USDC",
		"side":      "buy",
		"type":      "limit",
		"price":     "42000.00",
		"amount":    "0.1",
		"nonce":     123456,
		"signature": "0xmocksig",
	}

	w := s.Request(http.MethodPost, "/api/v1/orders", body, nil)
	s.Equal(http.StatusUnauthorized, w.Code)
}

// TestGetOrder_Success 测试获取订单成功
func (s *IntegrationSuite) TestGetOrder_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.OrderResponse{
		OrderID:   "order-123",
		Market:    "BTC-USDC",
		Side:      "buy",
		Type:      "limit",
		Price:     "42000.00",
		Amount:    "0.1",
		Status:    "open",
		CreatedAt: 1704067200000,
	}

	s.orderService.On("GetOrder", mock.Anything, "order-123").Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/orders/order-123", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestGetOrder_NotFound 测试获取不存在的订单
func (s *IntegrationSuite) TestGetOrder_NotFound() {
	wallet := "0x1234567890123456789012345678901234567890"

	s.orderService.On("GetOrder", mock.Anything, "nonexistent").Return(nil, dto.ErrOrderNotFound)

	w := s.Request(http.MethodGet, "/api/v1/orders/nonexistent", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusNotFound, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(dto.ErrOrderNotFound.Code, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestListOrders_Success 测试列出订单成功
func (s *IntegrationSuite) TestListOrders_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.OrderResponse{
			{OrderID: "order-1", Market: "BTC-USDC", Status: "open"},
			{OrderID: "order-2", Market: "ETH-USDC", Status: "filled"},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	s.orderService.On("ListOrders", mock.Anything, mock.MatchedBy(func(req *dto.ListOrdersRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/orders", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestCancelOrder_Success 测试取消订单成功
func (s *IntegrationSuite) TestCancelOrder_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.OrderResponse{
		OrderID:   "order-123",
		Market:    "BTC-USDC",
		Status:    "cancelled",
		CreatedAt: 1704067200000,
	}

	s.orderService.On("CancelOrder", mock.Anything, "order-123").Return(expectedResp, nil)

	w := s.Request(http.MethodDelete, "/api/v1/orders/order-123", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestListOpenOrders_Success 测试列出当前挂单
func (s *IntegrationSuite) TestListOpenOrders_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := []*dto.OrderResponse{
		{OrderID: "order-1", Market: "BTC-USDC", Status: "open"},
		{OrderID: "order-2", Market: "BTC-USDC", Status: "partial"},
	}

	s.orderService.On("ListOpenOrders", mock.Anything, "BTC-USDC").Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/orders/open?market=BTC-USDC", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestPrepareOrder_Success 测试获取订单签名摘要
func (s *IntegrationSuite) TestPrepareOrder_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PrepareOrderResponse{
		TypedData: "mock typed data",
		OrderID:   "order-123",
	}

	s.orderService.On("PrepareOrder", mock.Anything, mock.Anything).Return(expectedResp, nil)

	body := map[string]interface{}{
		"market": "BTC-USDC",
		"side":   "buy",
		"type":   "limit",
		"price":  "42000.00",
		"amount": "0.1",
	}

	w := s.Request(http.MethodPost, "/api/v1/orders/prepare", body, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}

// TestBatchCancelOrders_Success 测试批量取消订单
func (s *IntegrationSuite) TestBatchCancelOrders_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.BatchCancelResponse{
		Cancelled: []string{"order-1", "order-2"},
		Failed:    []string{},
	}

	s.orderService.On("BatchCancelOrders", mock.Anything, mock.Anything).Return(expectedResp, nil)

	body := map[string]interface{}{
		"order_ids": []string{"order-1", "order-2"},
	}

	w := s.Request(http.MethodDelete, "/api/v1/orders", body, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.orderService.AssertExpectations(s.T())
}
