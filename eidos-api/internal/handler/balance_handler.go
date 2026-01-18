package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// BalanceService 余额服务接口
type BalanceService interface {
	GetBalance(ctx *gin.Context, wallet, token string) (*dto.BalanceResponse, error)
	ListBalances(ctx *gin.Context, wallet string) ([]*dto.BalanceResponse, error)
	ListTransactions(ctx *gin.Context, req *dto.ListTransactionsRequest) (*dto.PaginatedResponse, error)
}

// BalanceHandler 余额处理器
type BalanceHandler struct {
	svc BalanceService
}

// NewBalanceHandler 创建余额处理器
func NewBalanceHandler(svc BalanceService) *BalanceHandler {
	return &BalanceHandler{svc: svc}
}

// GetBalance 获取单个代币余额
// GET /api/v1/balances/:token
func (h *BalanceHandler) GetBalance(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	token := c.Param("token")
	if token == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("token is required"))
		return
	}

	balance, err := h.svc.GetBalance(c, wallet.(string), token)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, balance)
}

// ListBalances 获取所有余额
// GET /api/v1/balances
func (h *BalanceHandler) ListBalances(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	balances, err := h.svc.ListBalances(c, wallet.(string))
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, balances)
}

// ListTransactions 查询资金流水
// GET /api/v1/transactions
func (h *BalanceHandler) ListTransactions(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	req := &dto.ListTransactionsRequest{
		Wallet: wallet.(string),
		Token:  c.Query("token"),
		Type:   c.Query("type"),
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

	resp, err := h.svc.ListTransactions(c, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	SuccessPaginated(c, resp)
}
