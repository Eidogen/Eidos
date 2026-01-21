package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// UserHandler handles user management requests
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// GetUser retrieves user information
// @Summary 获取用户信息
// @Description 根据钱包地址获取用户详情，包括账户状态、余额、风控评分等
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Success 200 {object} Response{data=service.UserInfo}
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/users/{wallet} [get]
func (h *UserHandler) GetUser(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	user, err := h.userService.GetUser(c.Request.Context(), wallet)
	if err != nil {
		InternalError(c, "获取用户信息失败: "+err.Error())
		return
	}

	Success(c, user)
}

// GetUserBalances retrieves user balances
// @Summary 获取用户余额
// @Description 获取用户所有代币余额
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Param hide_zero query bool false "隐藏零余额" default(false)
// @Success 200 {object} Response{data=[]service.Balance}
// @Router /admin/v1/users/{wallet}/balances [get]
func (h *UserHandler) GetUserBalances(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	hideZero := c.DefaultQuery("hide_zero", "false") == "true"

	balances, err := h.userService.GetUserBalances(c.Request.Context(), wallet, hideZero)
	if err != nil {
		InternalError(c, "获取用户余额失败: "+err.Error())
		return
	}

	Success(c, balances)
}

// FreezeAccount freezes a user account
// @Summary 冻结用户账户
// @Description 冻结用户账户，可选择完全冻结、仅冻结交易或仅冻结提现
// @Tags 用户管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.FreezeAccountRequest true "冻结请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/users/freeze [post]
func (h *UserHandler) FreezeAccount(c *gin.Context) {
	var req service.FreezeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.userService.FreezeAccount(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "账户冻结成功", nil)
}

// UnfreezeAccount unfreezes a user account
// @Summary 解冻用户账户
// @Description 解冻用户账户
// @Tags 用户管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.UnfreezeAccountRequest true "解冻请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/users/unfreeze [post]
func (h *UserHandler) UnfreezeAccount(c *gin.Context) {
	var req service.UnfreezeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.userService.UnfreezeAccount(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "账户解冻成功", nil)
}

// GetUserLimits retrieves user limits
// @Summary 获取用户限额
// @Description 获取用户的各类限额配置
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Success 200 {object} Response
// @Router /admin/v1/users/{wallet}/limits [get]
func (h *UserHandler) GetUserLimits(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	limits, err := h.userService.GetUserLimits(c.Request.Context(), wallet)
	if err != nil {
		InternalError(c, "获取用户限额失败: "+err.Error())
		return
	}

	Success(c, limits)
}

// SetUserLimits sets user limits
// @Summary 设置用户限额
// @Description 为用户设置自定义限额
// @Tags 用户管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.SetUserLimitsRequest true "设置限额请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/users/limits [post]
func (h *UserHandler) SetUserLimits(c *gin.Context) {
	var req service.SetUserLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.userService.SetUserLimits(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "用户限额设置成功", nil)
}

// GetRateLimitStatus retrieves rate limit status
// @Summary 获取用户限流状态
// @Description 获取用户当前的限流计数器状态
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Success 200 {object} Response
// @Router /admin/v1/users/{wallet}/rate-limit [get]
func (h *UserHandler) GetRateLimitStatus(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	status, err := h.userService.GetRateLimitStatus(c.Request.Context(), wallet)
	if err != nil {
		InternalError(c, "获取限流状态失败: "+err.Error())
		return
	}

	Success(c, status)
}

// ResetRateLimit resets rate limit counters
// @Summary 重置用户限流
// @Description 重置用户的限流计数器
// @Tags 用户管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.ResetRateLimitRequest true "重置限流请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/users/rate-limit/reset [post]
func (h *UserHandler) ResetRateLimit(c *gin.Context) {
	var req service.ResetRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.userService.ResetRateLimit(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "限流重置成功", nil)
}

// GetUserOrders retrieves user orders
// @Summary 获取用户订单
// @Description 获取用户的订单列表
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Param market query string false "交易对"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.UserOrdersResponse}
// @Router /admin/v1/users/{wallet}/orders [get]
func (h *UserHandler) GetUserOrders(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	market := c.Query("market")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	orders, err := h.userService.GetUserOrders(c.Request.Context(), wallet, market, page, pageSize)
	if err != nil {
		InternalError(c, "获取用户订单失败: "+err.Error())
		return
	}

	Success(c, orders)
}

// GetUserTrades retrieves user trades
// @Summary 获取用户成交
// @Description 获取用户的成交历史
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Param market query string false "交易对"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.UserTradesResponse}
// @Router /admin/v1/users/{wallet}/trades [get]
func (h *UserHandler) GetUserTrades(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	market := c.Query("market")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	trades, err := h.userService.GetUserTrades(c.Request.Context(), wallet, market, page, pageSize)
	if err != nil {
		InternalError(c, "获取用户成交失败: "+err.Error())
		return
	}

	Success(c, trades)
}

// GetUserDeposits retrieves user deposits
// @Summary 获取用户充值记录
// @Description 获取用户的充值历史
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Param token query string false "代币"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response
// @Router /admin/v1/users/{wallet}/deposits [get]
func (h *UserHandler) GetUserDeposits(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	token := c.Query("token")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	deposits, err := h.userService.GetUserDeposits(c.Request.Context(), wallet, token, page, pageSize)
	if err != nil {
		InternalError(c, "获取用户充值记录失败: "+err.Error())
		return
	}

	Success(c, deposits)
}

// GetUserWithdrawals retrieves user withdrawals
// @Summary 获取用户提现记录
// @Description 获取用户的提现历史
// @Tags 用户管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Param token query string false "代币"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response
// @Router /admin/v1/users/{wallet}/withdrawals [get]
func (h *UserHandler) GetUserWithdrawals(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	token := c.Query("token")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	withdrawals, err := h.userService.GetUserWithdrawals(c.Request.Context(), wallet, token, page, pageSize)
	if err != nil {
		InternalError(c, "获取用户提现记录失败: "+err.Error())
		return
	}

	Success(c, withdrawals)
}
