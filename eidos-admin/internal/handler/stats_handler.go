package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// StatsHandler 统计处理器
type StatsHandler struct {
	statsService *service.StatsService
}

// NewStatsHandler 创建统计处理器
func NewStatsHandler(statsService *service.StatsService) *StatsHandler {
	return &StatsHandler{
		statsService: statsService,
	}
}

// GetOverview 获取系统概览
// @Summary 获取系统概览
// @Tags 统计
// @Security Bearer
// @Success 200 {object} Response{data=model.SystemOverview}
// @Router /admin/v1/stats/overview [get]
func (h *StatsHandler) GetOverview(c *gin.Context) {
	overview, err := h.statsService.GetOverview(c.Request.Context())
	if err != nil {
		InternalError(c, "获取系统概览失败")
		return
	}

	Success(c, overview)
}

// GetDailyStats 获取每日统计
// @Summary 获取每日统计
// @Tags 统计
// @Security Bearer
// @Param date query string true "日期 (YYYY-MM-DD)"
// @Success 200 {object} Response{data=model.DailyStats}
// @Router /admin/v1/stats/daily [get]
func (h *StatsHandler) GetDailyStats(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		BadRequest(c, "日期不能为空")
		return
	}

	stats, err := h.statsService.GetDailyStats(c.Request.Context(), date)
	if err != nil {
		InternalError(c, "获取每日统计失败")
		return
	}
	if stats == nil {
		NotFound(c, "指定日期无统计数据")
		return
	}

	Success(c, stats)
}

// GetDailyStatsList 获取每日统计列表
// @Summary 获取每日统计列表
// @Tags 统计
// @Security Bearer
// @Param start_date query string true "开始日期 (YYYY-MM-DD)"
// @Param end_date query string true "结束日期 (YYYY-MM-DD)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PagedResponse{data=[]model.DailyStats}
// @Router /admin/v1/stats/daily/list [get]
func (h *StatsHandler) GetDailyStatsList(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		BadRequest(c, "开始日期和结束日期不能为空")
		return
	}

	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	stats, err := h.statsService.GetDailyStatsList(c.Request.Context(), startDate, endDate, &page)
	if err != nil {
		InternalError(c, "获取每日统计列表失败")
		return
	}

	SuccessPaged(c, stats, page.Page, page.PageSize, page.Total)
}

// GetTradingStats 获取交易统计
// @Summary 获取交易统计
// @Tags 统计
// @Security Bearer
// @Param symbol query string false "交易对符号"
// @Param start_date query string true "开始日期 (YYYY-MM-DD)"
// @Param end_date query string true "结束日期 (YYYY-MM-DD)"
// @Success 200 {object} Response{data=TradingStatsResponse}
// @Router /admin/v1/stats/trading [get]
func (h *StatsHandler) GetTradingStats(c *gin.Context) {
	symbol := c.Query("symbol")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		BadRequest(c, "开始日期和结束日期不能为空")
		return
	}

	stats, err := h.statsService.GetTradingStats(c.Request.Context(), symbol, startDate, endDate)
	if err != nil {
		InternalError(c, "获取交易统计失败")
		return
	}

	Success(c, stats)
}

// TradingStatsResponse 交易统计响应
type TradingStatsResponse struct {
	Symbol      string `json:"symbol,omitempty"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	TotalOrders int64  `json:"total_orders"`
	TotalTrades int64  `json:"total_trades"`
	TotalVolume string `json:"total_volume"`
	TotalFees   string `json:"total_fees"`
}

// GetUserStats 获取用户统计
// @Summary 获取用户统计
// @Tags 统计
// @Security Bearer
// @Param start_date query string true "开始日期 (YYYY-MM-DD)"
// @Param end_date query string true "结束日期 (YYYY-MM-DD)"
// @Success 200 {object} Response{data=UserStatsResponse}
// @Router /admin/v1/stats/users [get]
func (h *StatsHandler) GetUserStats(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		BadRequest(c, "开始日期和结束日期不能为空")
		return
	}

	stats, err := h.statsService.GetUserStats(c.Request.Context(), startDate, endDate)
	if err != nil {
		InternalError(c, "获取用户统计失败")
		return
	}

	Success(c, stats)
}

// UserStatsResponse 用户统计响应
type UserStatsResponse struct {
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	TotalUsers    int64  `json:"total_users"`
	NewUsers      int64  `json:"new_users"`
	ActiveUsers   int64  `json:"active_users"`
	TradingUsers  int64  `json:"trading_users"`
}

// GetSettlementStats 获取结算统计
// @Summary 获取结算统计
// @Tags 统计
// @Security Bearer
// @Param start_date query string true "开始日期 (YYYY-MM-DD)"
// @Param end_date query string true "结束日期 (YYYY-MM-DD)"
// @Success 200 {object} Response{data=SettlementStatsResponse}
// @Router /admin/v1/stats/settlements [get]
func (h *StatsHandler) GetSettlementStats(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		BadRequest(c, "开始日期和结束日期不能为空")
		return
	}

	stats, err := h.statsService.GetSettlementStats(c.Request.Context(), startDate, endDate)
	if err != nil {
		InternalError(c, "获取结算统计失败")
		return
	}

	Success(c, stats)
}

// SettlementStatsResponse 结算统计响应
type SettlementStatsResponse struct {
	StartDate          string `json:"start_date"`
	EndDate            string `json:"end_date"`
	TotalSettlements   int64  `json:"total_settlements"`
	SuccessSettlements int64  `json:"success_settlements"`
	FailedSettlements  int64  `json:"failed_settlements"`
	PendingSettlements int64  `json:"pending_settlements"`
	TotalVolume        string `json:"total_volume"`
	TotalGasFees       string `json:"total_gas_fees"`
}
