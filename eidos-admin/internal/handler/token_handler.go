package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// TokenHandler 代币配置处理器
type TokenHandler struct {
	tokenService *service.TokenService
}

// NewTokenHandler 创建代币配置处理器
func NewTokenHandler(tokenService *service.TokenService) *TokenHandler {
	return &TokenHandler{
		tokenService: tokenService,
	}
}

// List 获取代币列表
// @Summary 获取代币列表
// @Tags 代币配置
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param status query string false "状态筛选" Enums(active, disabled)
// @Success 200 {object} PagedResponse{data=[]model.TokenConfig}
// @Router /admin/v1/tokens [get]
func (h *TokenHandler) List(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	var status *model.TokenStatus
	if statusStr := c.Query("status"); statusStr != "" {
		switch statusStr {
		case "active":
			s := model.TokenStatusActive
			status = &s
		case "disabled":
			s := model.TokenStatusDisabled
			status = &s
		}
	}

	tokens, err := h.tokenService.List(c.Request.Context(), &page, status)
	if err != nil {
		InternalError(c, "获取代币列表失败")
		return
	}

	SuccessPaged(c, tokens, page.Page, page.PageSize, page.Total)
}

// Get 获取代币详情
// @Summary 获取代币详情
// @Tags 代币配置
// @Security Bearer
// @Param id path int true "代币ID"
// @Success 200 {object} Response{data=model.TokenConfig}
// @Router /admin/v1/tokens/{id} [get]
func (h *TokenHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	token, err := h.tokenService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取代币失败")
		return
	}
	if token == nil {
		NotFound(c, "代币不存在")
		return
	}

	Success(c, token)
}

// GetBySymbol 根据符号获取代币
// @Summary 根据符号获取代币
// @Tags 代币配置
// @Security Bearer
// @Param symbol path string true "代币符号"
// @Success 200 {object} Response{data=model.TokenConfig}
// @Router /admin/v1/tokens/symbol/{symbol} [get]
func (h *TokenHandler) GetBySymbol(c *gin.Context) {
	symbol := c.Param("symbol")
	if symbol == "" {
		BadRequest(c, "符号不能为空")
		return
	}

	token, err := h.tokenService.GetBySymbol(c.Request.Context(), symbol)
	if err != nil {
		InternalError(c, "获取代币失败")
		return
	}
	if token == nil {
		NotFound(c, "代币不存在")
		return
	}

	Success(c, token)
}

// Create 创建代币
// @Summary 创建代币
// @Tags 代币配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.CreateTokenRequest true "创建代币请求"
// @Success 200 {object} Response{data=model.TokenConfig}
// @Router /admin/v1/tokens [post]
func (h *TokenHandler) Create(c *gin.Context) {
	var req service.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	token, err := h.tokenService.Create(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, token)
}

// Update 更新代币
// @Summary 更新代币
// @Tags 代币配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "代币ID"
// @Param request body service.UpdateTokenRequest true "更新代币请求"
// @Success 200 {object} Response{data=model.TokenConfig}
// @Router /admin/v1/tokens/{id} [put]
func (h *TokenHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req service.UpdateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.ID = id
	req.OperatorID = middleware.GetAdminID(c)

	token, err := h.tokenService.Update(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, token)
}

// UpdateStatus 更新代币状态
// @Summary 更新代币状态
// @Tags 代币配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "代币ID"
// @Param request body UpdateTokenStatusRequest true "更新状态请求"
// @Success 200 {object} Response
// @Router /admin/v1/tokens/{id}/status [put]
func (h *TokenHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req UpdateTokenStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	// 先获取代币信息
	token, err := h.tokenService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取代币失败")
		return
	}
	if token == nil {
		NotFound(c, "代币不存在")
		return
	}

	// 解析状态
	var status model.TokenStatus
	switch req.Status {
	case "active":
		status = model.TokenStatusActive
	case "disabled":
		status = model.TokenStatusDisabled
	default:
		BadRequest(c, "无效的状态值")
		return
	}

	updateReq := &service.TokenUpdateStatusRequest{
		Symbol:     token.Symbol,
		Status:     status,
		OperatorID: middleware.GetAdminID(c),
	}

	if err := h.tokenService.UpdateStatus(c.Request.Context(), updateReq); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "状态更新成功", nil)
}

// UpdateTokenStatusRequest 更新状态请求
type UpdateTokenStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// Delete 删除代币
// @Summary 删除代币
// @Tags 代币配置
// @Security Bearer
// @Param id path int true "代币ID"
// @Success 200 {object} Response
// @Router /admin/v1/tokens/{id} [delete]
func (h *TokenHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	operatorID := middleware.GetAdminID(c)

	if err := h.tokenService.Delete(c.Request.Context(), id, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "删除成功", nil)
}

// GetAllActive 获取所有活跃代币
// @Summary 获取所有活跃代币
// @Tags 代币配置
// @Security Bearer
// @Success 200 {object} Response{data=[]model.TokenConfig}
// @Router /admin/v1/tokens/active [get]
func (h *TokenHandler) GetAllActive(c *gin.Context) {
	tokens, err := h.tokenService.ListActive(c.Request.Context())
	if err != nil {
		InternalError(c, "获取活跃代币失败")
		return
	}

	Success(c, tokens)
}

// GetDepositEnabled 获取可充值代币
// @Summary 获取可充值代币列表
// @Tags 代币配置
// @Security Bearer
// @Success 200 {object} Response{data=[]model.TokenConfig}
// @Router /admin/v1/tokens/deposit-enabled [get]
func (h *TokenHandler) GetDepositEnabled(c *gin.Context) {
	tokens, err := h.tokenService.ListDepositEnabled(c.Request.Context())
	if err != nil {
		InternalError(c, "获取可充值代币失败")
		return
	}

	Success(c, tokens)
}

// GetWithdrawEnabled 获取可提现代币
// @Summary 获取可提现代币列表
// @Tags 代币配置
// @Security Bearer
// @Success 200 {object} Response{data=[]model.TokenConfig}
// @Router /admin/v1/tokens/withdraw-enabled [get]
func (h *TokenHandler) GetWithdrawEnabled(c *gin.Context) {
	tokens, err := h.tokenService.ListWithdrawEnabled(c.Request.Context())
	if err != nil {
		InternalError(c, "获取可提现代币失败")
		return
	}

	Success(c, tokens)
}
