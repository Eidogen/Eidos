package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// WithdrawalService 提现服务接口
type WithdrawalService interface {
	CreateWithdrawal(ctx *gin.Context, req *dto.CreateWithdrawalRequest) (*dto.WithdrawalResponse, error)
	GetWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error)
	ListWithdrawals(ctx *gin.Context, req *dto.ListWithdrawalsRequest) (*dto.PaginatedResponse, error)
	CancelWithdrawal(ctx *gin.Context, withdrawID string) (*dto.WithdrawalResponse, error)
}

// WithdrawalHandler 提现处理器
type WithdrawalHandler struct {
	svc WithdrawalService
}

// NewWithdrawalHandler 创建提现处理器
func NewWithdrawalHandler(svc WithdrawalService) *WithdrawalHandler {
	return &WithdrawalHandler{svc: svc}
}

// CreateWithdrawal 创建提现
// POST /api/v1/withdrawals
func (h *WithdrawalHandler) CreateWithdrawal(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	var req dto.CreateWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, dto.ErrInvalidParams.WithMessage(err.Error()))
		return
	}
	req.Wallet = wallet.(string)

	// 验证必填字段
	if err := validateCreateWithdrawalRequest(&req); err != nil {
		Error(c, err)
		return
	}

	resp, err := h.svc.CreateWithdrawal(c, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// GetWithdrawal 获取提现详情
// GET /api/v1/withdrawals/:id
func (h *WithdrawalHandler) GetWithdrawal(c *gin.Context) {
	withdrawID := c.Param("id")
	if withdrawID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("withdraw id is required"))
		return
	}

	withdrawal, err := h.svc.GetWithdrawal(c, withdrawID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, withdrawal)
}

// ListWithdrawals 查询提现记录
// GET /api/v1/withdrawals
func (h *WithdrawalHandler) ListWithdrawals(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	req := &dto.ListWithdrawalsRequest{
		Wallet: wallet.(string),
		Token:  c.Query("token"),
		Status: c.Query("status"),
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

	resp, err := h.svc.ListWithdrawals(c, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	SuccessPaginated(c, resp)
}

// CancelWithdrawal 取消提现
// DELETE /api/v1/withdrawals/:id
func (h *WithdrawalHandler) CancelWithdrawal(c *gin.Context) {
	withdrawID := c.Param("id")
	if withdrawID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("withdraw id is required"))
		return
	}

	resp, err := h.svc.CancelWithdrawal(c, withdrawID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, resp)
}

// validateCreateWithdrawalRequest 验证提现请求
func validateCreateWithdrawalRequest(req *dto.CreateWithdrawalRequest) *dto.BizError {
	if req.Token == "" {
		return dto.ErrInvalidParams.WithMessage("token is required")
	}
	if req.Amount == "" {
		return dto.ErrInvalidParams.WithMessage("amount is required")
	}
	if req.ToAddress == "" {
		return dto.ErrInvalidWithdrawAddress
	}
	// 验证地址格式 (简单校验)
	if len(req.ToAddress) != 42 || req.ToAddress[:2] != "0x" {
		return dto.ErrInvalidWithdrawAddress
	}
	if req.Signature == "" {
		return dto.ErrInvalidSignature
	}
	return nil
}
