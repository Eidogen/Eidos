package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// DepositService 充值服务接口
type DepositService interface {
	GetDeposit(ctx *gin.Context, depositID string) (*dto.DepositResponse, error)
	ListDeposits(ctx *gin.Context, req *dto.ListDepositsRequest) (*dto.PaginatedResponse, error)
}

// DepositHandler 充值处理器
type DepositHandler struct {
	svc DepositService
}

// NewDepositHandler 创建充值处理器
func NewDepositHandler(svc DepositService) *DepositHandler {
	return &DepositHandler{svc: svc}
}

// GetDeposit 获取充值详情
// GET /api/v1/deposits/:id
func (h *DepositHandler) GetDeposit(c *gin.Context) {
	depositID := c.Param("id")
	if depositID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("deposit id is required"))
		return
	}

	deposit, err := h.svc.GetDeposit(c, depositID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, deposit)
}

// ListDeposits 查询充值记录
// GET /api/v1/deposits
func (h *DepositHandler) ListDeposits(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	req := &dto.ListDepositsRequest{
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

	resp, err := h.svc.ListDeposits(c, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	SuccessPaginated(c, resp)
}
