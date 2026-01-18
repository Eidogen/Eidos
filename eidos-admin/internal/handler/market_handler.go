package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// MarketHandler 市场配置处理器
type MarketHandler struct {
	marketService *service.MarketService
}

// NewMarketHandler 创建市场配置处理器
func NewMarketHandler(marketService *service.MarketService) *MarketHandler {
	return &MarketHandler{
		marketService: marketService,
	}
}

// List 获取交易对列表
// @Summary 获取交易对列表
// @Tags 市场配置
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态筛选"
// @Success 200 {object} PagedResponse{data=[]model.MarketConfig}
// @Router /admin/v1/markets [get]
func (h *MarketHandler) List(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	var status *model.MarketStatus
	if statusStr := c.Query("status"); statusStr != "" {
		switch statusStr {
		case "active":
			s := model.MarketStatusActive
			status = &s
		case "suspended":
			s := model.MarketStatusSuspended
			status = &s
		case "offline":
			s := model.MarketStatusOffline
			status = &s
		}
	}

	markets, err := h.marketService.List(c.Request.Context(), &page, status)
	if err != nil {
		InternalError(c, "获取交易对列表失败")
		return
	}

	SuccessPaged(c, markets, page.Page, page.PageSize, page.Total)
}

// Get 获取交易对详情
// @Summary 获取交易对详情
// @Tags 市场配置
// @Security Bearer
// @Param id path int true "交易对ID"
// @Success 200 {object} Response{data=model.MarketConfig}
// @Router /admin/v1/markets/{id} [get]
func (h *MarketHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	market, err := h.marketService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取交易对失败")
		return
	}
	if market == nil {
		NotFound(c, "交易对不存在")
		return
	}

	Success(c, market)
}

// GetBySymbol 根据符号获取交易对
// @Summary 根据符号获取交易对
// @Tags 市场配置
// @Security Bearer
// @Param symbol path string true "交易对符号"
// @Success 200 {object} Response{data=model.MarketConfig}
// @Router /admin/v1/markets/symbol/{symbol} [get]
func (h *MarketHandler) GetBySymbol(c *gin.Context) {
	symbol := c.Param("symbol")
	if symbol == "" {
		BadRequest(c, "符号不能为空")
		return
	}

	market, err := h.marketService.GetBySymbol(c.Request.Context(), symbol)
	if err != nil {
		InternalError(c, "获取交易对失败")
		return
	}
	if market == nil {
		NotFound(c, "交易对不存在")
		return
	}

	Success(c, market)
}

// Create 创建交易对
// @Summary 创建交易对
// @Tags 市场配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.CreateMarketRequest true "创建交易对请求"
// @Success 200 {object} Response{data=model.MarketConfig}
// @Router /admin/v1/markets [post]
func (h *MarketHandler) Create(c *gin.Context) {
	var req service.CreateMarketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	market, err := h.marketService.Create(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, market)
}

// Update 更新交易对
// @Summary 更新交易对
// @Tags 市场配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "交易对ID"
// @Param request body service.UpdateMarketRequest true "更新交易对请求"
// @Success 200 {object} Response{data=model.MarketConfig}
// @Router /admin/v1/markets/{id} [put]
func (h *MarketHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req service.UpdateMarketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.ID = id
	req.OperatorID = middleware.GetAdminID(c)

	market, err := h.marketService.Update(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, market)
}

// UpdateStatus 更新交易对状态
// @Summary 更新交易对状态
// @Tags 市场配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "交易对ID"
// @Param request body UpdateMarketStatusRequest true "更新状态请求"
// @Success 200 {object} Response
// @Router /admin/v1/markets/{id}/status [put]
func (h *MarketHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req UpdateMarketStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	// 先获取交易对信息
	market, err := h.marketService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取交易对失败")
		return
	}
	if market == nil {
		NotFound(c, "交易对不存在")
		return
	}

	// 解析状态
	var status model.MarketStatus
	switch req.Status {
	case "active":
		status = model.MarketStatusActive
	case "suspended":
		status = model.MarketStatusSuspended
	case "offline":
		status = model.MarketStatusOffline
	default:
		BadRequest(c, "无效的状态值")
		return
	}

	updateReq := &service.UpdateStatusRequest{
		Symbol:         market.Symbol,
		Status:         status,
		TradingEnabled: req.TradingEnabled,
		OperatorID:     middleware.GetAdminID(c),
	}

	if err := h.marketService.UpdateStatus(c.Request.Context(), updateReq); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "状态更新成功", nil)
}

// UpdateMarketStatusRequest 更新状态请求
type UpdateMarketStatusRequest struct {
	Status         string `json:"status" binding:"required,oneof=active suspended offline"`
	TradingEnabled bool   `json:"trading_enabled"`
}

// Delete 删除交易对
// @Summary 删除交易对
// @Tags 市场配置
// @Security Bearer
// @Param id path int true "交易对ID"
// @Success 200 {object} Response
// @Router /admin/v1/markets/{id} [delete]
func (h *MarketHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	operatorID := middleware.GetAdminID(c)

	if err := h.marketService.Delete(c.Request.Context(), id, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "删除成功", nil)
}

// GetAllActive 获取所有活跃交易对
// @Summary 获取所有活跃交易对
// @Tags 市场配置
// @Security Bearer
// @Success 200 {object} Response{data=[]model.MarketConfig}
// @Router /admin/v1/markets/active [get]
func (h *MarketHandler) GetAllActive(c *gin.Context) {
	markets, err := h.marketService.GetAllActive(c.Request.Context())
	if err != nil {
		InternalError(c, "获取活跃交易对失败")
		return
	}

	Success(c, markets)
}
