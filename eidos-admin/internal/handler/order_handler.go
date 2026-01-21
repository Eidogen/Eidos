package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// OrderHandler handles order management requests
type OrderHandler struct {
	orderService *service.OrderService
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(orderService *service.OrderService) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
	}
}

// GetOrder retrieves order details
// @Summary 获取订单详情
// @Description 根据订单ID获取订单详情，包括关联的成交记录
// @Tags 订单管理
// @Security Bearer
// @Param order_id path string true "订单ID"
// @Success 200 {object} Response{data=service.OrderDetail}
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/orders/{order_id} [get]
func (h *OrderHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("order_id")
	if orderID == "" {
		BadRequest(c, "订单ID不能为空")
		return
	}

	order, err := h.orderService.GetOrder(c.Request.Context(), orderID)
	if err != nil {
		InternalError(c, "获取订单失败: "+err.Error())
		return
	}

	Success(c, order)
}

// ListOrders retrieves orders with filters
// @Summary 获取订单列表
// @Description 根据条件查询订单列表
// @Tags 订单管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param market query string false "交易对"
// @Param side query int false "方向 (1=买 2=卖)"
// @Param type query int false "类型 (1=限价 2=市价)"
// @Param start_time query int false "开始时间(毫秒)"
// @Param end_time query int false "结束时间(毫秒)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.OrderListResponse}
// @Router /admin/v1/orders [get]
func (h *OrderHandler) ListOrders(c *gin.Context) {
	var req service.OrderListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}

	orders, err := h.orderService.ListOrders(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, "获取订单列表失败: "+err.Error())
		return
	}

	Success(c, orders)
}

// ListOpenOrders retrieves active orders
// @Summary 获取活跃订单
// @Description 获取指定钱包的所有活跃订单
// @Tags 订单管理
// @Security Bearer
// @Param wallet query string true "钱包地址"
// @Param market query string false "交易对"
// @Success 200 {object} Response
// @Router /admin/v1/orders/open [get]
func (h *OrderHandler) ListOpenOrders(c *gin.Context) {
	wallet := c.Query("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	market := c.Query("market")

	orders, err := h.orderService.ListOpenOrders(c.Request.Context(), wallet, market)
	if err != nil {
		InternalError(c, "获取活跃订单失败: "+err.Error())
		return
	}

	Success(c, orders)
}

// CancelOrder cancels an order
// @Summary 取消订单
// @Description 管理员强制取消指定订单
// @Tags 订单管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.CancelOrderRequest true "取消订单请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/orders/cancel [post]
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	var req service.CancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.orderService.CancelOrder(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "订单取消成功", nil)
}

// BatchCancelOrders cancels multiple orders
// @Summary 批量取消订单
// @Description 批量取消指定钱包的订单
// @Tags 订单管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.BatchCancelOrdersRequest true "批量取消请求"
// @Success 200 {object} Response{data=service.BatchCancelOrdersResponse}
// @Failure 400 {object} Response
// @Router /admin/v1/orders/batch-cancel [post]
func (h *OrderHandler) BatchCancelOrders(c *gin.Context) {
	var req service.BatchCancelOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	resp, err := h.orderService.BatchCancelOrders(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, resp)
}

// GetTrade retrieves trade details
// @Summary 获取成交详情
// @Description 根据成交ID获取成交详情
// @Tags 订单管理
// @Security Bearer
// @Param trade_id path string true "成交ID"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/trades/{trade_id} [get]
func (h *OrderHandler) GetTrade(c *gin.Context) {
	tradeID := c.Param("trade_id")
	if tradeID == "" {
		BadRequest(c, "成交ID不能为空")
		return
	}

	trade, err := h.orderService.GetTrade(c.Request.Context(), tradeID)
	if err != nil {
		InternalError(c, "获取成交详情失败: "+err.Error())
		return
	}

	Success(c, trade)
}

// ListTrades retrieves trades with filters
// @Summary 获取成交列表
// @Description 根据条件查询成交列表
// @Tags 订单管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param market query string false "交易对"
// @Param start_time query int false "开始时间(毫秒)"
// @Param end_time query int false "结束时间(毫秒)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.TradeListResponse}
// @Router /admin/v1/trades [get]
func (h *OrderHandler) ListTrades(c *gin.Context) {
	var req service.TradeListRequest

	req.Wallet = c.Query("wallet")
	req.Market = c.Query("market")

	if st, err := strconv.ParseInt(c.Query("start_time"), 10, 64); err == nil {
		req.StartTime = st
	}
	if et, err := strconv.ParseInt(c.Query("end_time"), 10, 64); err == nil {
		req.EndTime = et
	}
	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		req.Page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		req.PageSize = int32(ps)
	}

	trades, err := h.orderService.ListTrades(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, "获取成交列表失败: "+err.Error())
		return
	}

	Success(c, trades)
}

// GetTradesByOrder retrieves trades for an order
// @Summary 获取订单成交记录
// @Description 获取指定订单的所有成交记录
// @Tags 订单管理
// @Security Bearer
// @Param order_id path string true "订单ID"
// @Success 200 {object} Response
// @Router /admin/v1/orders/{order_id}/trades [get]
func (h *OrderHandler) GetTradesByOrder(c *gin.Context) {
	orderID := c.Param("order_id")
	if orderID == "" {
		BadRequest(c, "订单ID不能为空")
		return
	}

	trades, err := h.orderService.GetTradesByOrder(c.Request.Context(), orderID)
	if err != nil {
		InternalError(c, "获取订单成交记录失败: "+err.Error())
		return
	}

	Success(c, trades)
}
