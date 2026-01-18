package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// OrderService 订单服务接口
type OrderService interface {
	PrepareOrder(ctx *gin.Context, req *dto.PrepareOrderRequest) (*dto.PrepareOrderResponse, error)
	CreateOrder(ctx *gin.Context, req *dto.CreateOrderRequest) (*dto.OrderResponse, error)
	GetOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error)
	ListOrders(ctx *gin.Context, req *dto.ListOrdersRequest) (*dto.PaginatedResponse, error)
	ListOpenOrders(ctx *gin.Context, market string) ([]*dto.OrderResponse, error)
	CancelOrder(ctx *gin.Context, orderID string) (*dto.OrderResponse, error)
	BatchCancelOrders(ctx *gin.Context, req *dto.BatchCancelRequest) (*dto.BatchCancelResponse, error)
}

// OrderHandler 订单处理器
type OrderHandler struct {
	svc OrderService
}

// NewOrderHandler 创建订单处理器
func NewOrderHandler(svc OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// PrepareOrder 获取订单签名摘要
// POST /api/v1/orders/prepare
func (h *OrderHandler) PrepareOrder(c *gin.Context) {
	var req dto.PrepareOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, dto.ErrInvalidParams.WithMessage(err.Error()))
		return
	}

	// 验证必填字段
	if err := validatePrepareOrderRequest(&req); err != nil {
		Error(c, err)
		return
	}

	resp, err := h.svc.PrepareOrder(c, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// CreateOrder 创建订单
// POST /api/v1/orders
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req dto.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, dto.ErrInvalidParams.WithMessage(err.Error()))
		return
	}

	// 从上下文获取钱包地址
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}
	req.Wallet = wallet.(string)

	// 验证必填字段
	if err := validateCreateOrderRequest(&req); err != nil {
		Error(c, err)
		return
	}

	resp, err := h.svc.CreateOrder(c, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// GetOrder 获取订单详情
// GET /api/v1/orders/:id
func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("order id is required"))
		return
	}

	resp, err := h.svc.GetOrder(c, orderID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// ListOrders 查询订单列表
// GET /api/v1/orders
func (h *OrderHandler) ListOrders(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	req := &dto.ListOrdersRequest{
		Wallet:  wallet.(string),
		Market:  c.Query("market"),
		Side:    c.Query("side"),
		Status:  c.Query("status"),
		OrderBy: c.DefaultQuery("order_by", "created_at"),
		Order:   c.DefaultQuery("order", "desc"),
	}

	// 分页参数
	if page := c.Query("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			req.Page = p
		}
	}
	if pageSize := c.Query("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 && ps <= 100 {
			req.PageSize = ps
		}
	}

	// 时间范围
	if startTime := c.Query("start_time"); startTime != "" {
		if st, err := strconv.ParseInt(startTime, 10, 64); err == nil {
			req.StartTime = st
		}
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if et, err := strconv.ParseInt(endTime, 10, 64); err == nil {
			req.EndTime = et
		}
	}

	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	resp, err := h.svc.ListOrders(c, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	SuccessPaginated(c, resp)
}

// ListOpenOrders 查询当前挂单
// GET /api/v1/orders/open
func (h *OrderHandler) ListOpenOrders(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	// 设置 wallet 到上下文供 service 使用
	c.Set("wallet", wallet)

	market := c.Query("market")

	orders, err := h.svc.ListOpenOrders(c, market)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, orders)
}

// CancelOrder 取消订单
// DELETE /api/v1/orders/:id
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("order id is required"))
		return
	}

	resp, err := h.svc.CancelOrder(c, orderID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// BatchCancelOrders 批量取消订单
// DELETE /api/v1/orders
func (h *OrderHandler) BatchCancelOrders(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	var req dto.BatchCancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 如果没有 body，使用查询参数
		req.Market = c.Query("market")
		req.Side = c.Query("side")
	}
	req.Wallet = wallet.(string)

	resp, err := h.svc.BatchCancelOrders(c, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// validatePrepareOrderRequest 验证准备订单请求
func validatePrepareOrderRequest(req *dto.PrepareOrderRequest) *dto.BizError {
	if req.Market == "" {
		return dto.ErrInvalidParams.WithMessage("market is required")
	}
	if req.Side == "" {
		return dto.ErrInvalidOrderSide
	}
	if req.Side != "buy" && req.Side != "sell" {
		return dto.ErrInvalidOrderSide
	}
	if req.Type == "" {
		return dto.ErrInvalidOrderType
	}
	if req.Type != "limit" && req.Type != "market" {
		return dto.ErrInvalidOrderType
	}
	if req.Type == "limit" && req.Price == "" {
		return dto.ErrInvalidPrice
	}
	if req.Amount == "" {
		return dto.ErrInvalidAmount
	}
	return nil
}

// validateCreateOrderRequest 验证创建订单请求
func validateCreateOrderRequest(req *dto.CreateOrderRequest) *dto.BizError {
	if req.Market == "" {
		return dto.ErrInvalidParams.WithMessage("market is required")
	}
	if req.Side == "" {
		return dto.ErrInvalidOrderSide
	}
	if req.Side != "buy" && req.Side != "sell" {
		return dto.ErrInvalidOrderSide
	}
	if req.Type == "" {
		return dto.ErrInvalidOrderType
	}
	if req.Type != "limit" && req.Type != "market" {
		return dto.ErrInvalidOrderType
	}
	if req.Type == "limit" && req.Price == "" {
		return dto.ErrInvalidPrice
	}
	if req.Amount == "" {
		return dto.ErrInvalidAmount
	}
	if req.Signature == "" {
		return dto.ErrInvalidSignature
	}
	return nil
}

// handleServiceError 处理服务层错误
func handleServiceError(c *gin.Context, err error) {
	if bizErr, ok := err.(*dto.BizError); ok {
		Error(c, bizErr)
		return
	}
	c.JSON(http.StatusInternalServerError, dto.Response{
		Code:    dto.ErrInternalError.Code,
		Message: err.Error(),
	})
}
