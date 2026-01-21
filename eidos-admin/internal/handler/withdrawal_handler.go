package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// WithdrawalHandler handles withdrawal management requests
type WithdrawalHandler struct {
	withdrawalService *service.WithdrawalService
}

// NewWithdrawalHandler creates a new withdrawal handler
func NewWithdrawalHandler(withdrawalService *service.WithdrawalService) *WithdrawalHandler {
	return &WithdrawalHandler{
		withdrawalService: withdrawalService,
	}
}

// GetWithdrawal retrieves withdrawal details
// @Summary 获取提现详情
// @Description 根据提现ID获取提现详情
// @Tags 提现管理
// @Security Bearer
// @Param withdraw_id path string true "提现ID"
// @Success 200 {object} Response{data=service.WithdrawalDetail}
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/withdrawals/{withdraw_id} [get]
func (h *WithdrawalHandler) GetWithdrawal(c *gin.Context) {
	withdrawID := c.Param("withdraw_id")
	if withdrawID == "" {
		BadRequest(c, "提现ID不能为空")
		return
	}

	withdrawal, err := h.withdrawalService.GetWithdrawal(c.Request.Context(), withdrawID)
	if err != nil {
		InternalError(c, "获取提现详情失败: "+err.Error())
		return
	}

	Success(c, withdrawal)
}

// ListWithdrawals retrieves withdrawals with filters
// @Summary 获取提现列表
// @Description 根据条件查询提现列表
// @Tags 提现管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param token query string false "代币"
// @Param status query int false "状态"
// @Param start_time query int false "开始时间(毫秒)"
// @Param end_time query int false "结束时间(毫秒)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.WithdrawalListResponse}
// @Router /admin/v1/withdrawals [get]
func (h *WithdrawalHandler) ListWithdrawals(c *gin.Context) {
	var req service.WithdrawalListRequest
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

	withdrawals, err := h.withdrawalService.ListWithdrawals(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, "获取提现列表失败: "+err.Error())
		return
	}

	Success(c, withdrawals)
}

// ListPendingWithdrawals retrieves pending withdrawals
// @Summary 获取待处理提现
// @Description 获取待处理的提现列表（从链服务）
// @Tags 提现管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param token query string false "代币"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response
// @Router /admin/v1/withdrawals/pending [get]
func (h *WithdrawalHandler) ListPendingWithdrawals(c *gin.Context) {
	wallet := c.Query("wallet")
	token := c.Query("token")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	withdrawals, err := h.withdrawalService.ListPendingWithdrawals(c.Request.Context(), wallet, token, page, pageSize)
	if err != nil {
		InternalError(c, "获取待处理提现失败: "+err.Error())
		return
	}

	Success(c, withdrawals)
}

// RejectWithdrawal rejects a pending withdrawal
// @Summary 拒绝提现
// @Description 拒绝（取消）待处理的提现申请
// @Tags 提现管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.RejectWithdrawalRequest true "拒绝提现请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/withdrawals/reject [post]
func (h *WithdrawalHandler) RejectWithdrawal(c *gin.Context) {
	var req service.RejectWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.withdrawalService.RejectWithdrawal(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "提现已拒绝", nil)
}

// RetryWithdrawal retries a failed withdrawal
// @Summary 重试提现
// @Description 重试失败的提现交易
// @Tags 提现管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.RetryWithdrawalRequest true "重试提现请求"
// @Success 200 {object} Response{data=service.RetryWithdrawalResponse}
// @Failure 400 {object} Response
// @Router /admin/v1/withdrawals/retry [post]
func (h *WithdrawalHandler) RetryWithdrawal(c *gin.Context) {
	var req service.RetryWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	resp, err := h.withdrawalService.RetryWithdrawal(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, resp)
}

// GetDeposit retrieves deposit details
// @Summary 获取充值详情
// @Description 根据充值ID获取充值详情
// @Tags 充提管理
// @Security Bearer
// @Param deposit_id path string true "充值ID"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/deposits/{deposit_id} [get]
func (h *WithdrawalHandler) GetDeposit(c *gin.Context) {
	depositID := c.Param("deposit_id")
	if depositID == "" {
		BadRequest(c, "充值ID不能为空")
		return
	}

	deposit, err := h.withdrawalService.GetDeposit(c.Request.Context(), depositID)
	if err != nil {
		InternalError(c, "获取充值详情失败: "+err.Error())
		return
	}

	Success(c, deposit)
}

// ListDeposits retrieves deposits with filters
// @Summary 获取充值列表
// @Description 根据条件查询充值列表
// @Tags 充提管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param token query string false "代币"
// @Param status query int false "状态"
// @Param start_time query int false "开始时间(毫秒)"
// @Param end_time query int false "结束时间(毫秒)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.DepositListResponse}
// @Router /admin/v1/deposits [get]
func (h *WithdrawalHandler) ListDeposits(c *gin.Context) {
	var req service.DepositListRequest
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

	deposits, err := h.withdrawalService.ListDeposits(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, "获取充值列表失败: "+err.Error())
		return
	}

	Success(c, deposits)
}
