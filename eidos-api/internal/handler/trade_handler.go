package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// TradeService 成交服务接口
type TradeService interface {
	GetTrade(ctx *gin.Context, tradeID string) (*dto.TradeResponse, error)
	ListTrades(ctx *gin.Context, req *dto.ListTradesRequest) (*dto.PaginatedResponse, error)
}

// TradeHandler 成交处理器
type TradeHandler struct {
	svc TradeService
}

// NewTradeHandler 创建成交处理器
func NewTradeHandler(svc TradeService) *TradeHandler {
	return &TradeHandler{svc: svc}
}

// GetTrade 获取成交详情
// GET /api/v1/trades/:id
func (h *TradeHandler) GetTrade(c *gin.Context) {
	tradeID := c.Param("id")
	if tradeID == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("trade id is required"))
		return
	}

	trade, err := h.svc.GetTrade(c, tradeID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, trade)
}

// ListTrades 查询成交记录
// GET /api/v1/trades
func (h *TradeHandler) ListTrades(c *gin.Context) {
	wallet, exists := c.Get("wallet")
	if !exists {
		Error(c, dto.ErrUnauthorized)
		return
	}

	req := &dto.ListTradesRequest{
		Wallet:  wallet.(string),
		Market:  c.Query("market"),
		OrderID: c.Query("order_id"),
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

	resp, err := h.svc.ListTrades(c, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	SuccessPaginated(c, resp)
}
