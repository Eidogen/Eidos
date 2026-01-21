package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// RiskHandler handles risk management requests
type RiskHandler struct {
	riskService *service.RiskService
}

// NewRiskHandler creates a new risk handler
func NewRiskHandler(riskService *service.RiskService) *RiskHandler {
	return &RiskHandler{
		riskService: riskService,
	}
}

// === Risk Rules ===

// ListRiskRules retrieves all risk rules
// @Summary 获取风控规则列表
// @Description 获取所有风控规则配置
// @Tags 风控管理
// @Security Bearer
// @Param category query string false "规则分类 (order, withdrawal, account, rate_limit)"
// @Param enabled_only query bool false "仅显示启用的规则" default(false)
// @Success 200 {object} Response{data=[]service.RiskRule}
// @Router /admin/v1/risk/rules [get]
func (h *RiskHandler) ListRiskRules(c *gin.Context) {
	category := c.Query("category")
	enabledOnly := c.DefaultQuery("enabled_only", "false") == "true"

	rules, err := h.riskService.ListRiskRules(c.Request.Context(), category, enabledOnly)
	if err != nil {
		InternalError(c, "获取风控规则列表失败: "+err.Error())
		return
	}

	Success(c, rules)
}

// GetRiskRule retrieves a specific risk rule
// @Summary 获取风控规则详情
// @Description 根据规则ID获取风控规则详情
// @Tags 风控管理
// @Security Bearer
// @Param rule_id path string true "规则ID"
// @Success 200 {object} Response{data=service.RiskRule}
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/risk/rules/{rule_id} [get]
func (h *RiskHandler) GetRiskRule(c *gin.Context) {
	ruleID := c.Param("rule_id")
	if ruleID == "" {
		BadRequest(c, "规则ID不能为空")
		return
	}

	rule, err := h.riskService.GetRiskRule(c.Request.Context(), ruleID)
	if err != nil {
		InternalError(c, "获取风控规则失败: "+err.Error())
		return
	}

	Success(c, rule)
}

// UpdateRiskRule updates a risk rule
// @Summary 更新风控规则
// @Description 更新风控规则配置
// @Tags 风控管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.UpdateRiskRuleRequest true "更新规则请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/risk/rules [put]
func (h *RiskHandler) UpdateRiskRule(c *gin.Context) {
	var req service.UpdateRiskRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.riskService.UpdateRiskRule(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "风控规则更新成功", nil)
}

// === Blacklist ===

// ListBlacklist retrieves blacklist entries
// @Summary 获取黑名单列表
// @Description 获取黑名单条目列表
// @Tags 风控管理
// @Security Bearer
// @Param blacklist_type query string false "黑名单类型 (full, withdraw_only, trade_only)"
// @Param source query string false "来源 (manual, ofac, chainalysis)"
// @Param search query string false "搜索钱包地址前缀"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response{data=service.BlacklistListResponse}
// @Router /admin/v1/risk/blacklist [get]
func (h *RiskHandler) ListBlacklist(c *gin.Context) {
	blacklistType := c.Query("blacklist_type")
	source := c.Query("source")
	search := c.Query("search")
	page := int32(1)
	pageSize := int32(20)

	if p, err := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32); err == nil {
		page = int32(p)
	}
	if ps, err := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32); err == nil {
		pageSize = int32(ps)
	}

	blacklist, err := h.riskService.ListBlacklist(c.Request.Context(), blacklistType, source, search, page, pageSize)
	if err != nil {
		InternalError(c, "获取黑名单列表失败: "+err.Error())
		return
	}

	Success(c, blacklist)
}

// CheckBlacklist checks if a wallet is blacklisted
// @Summary 检查黑名单状态
// @Description 检查指定钱包是否在黑名单中
// @Tags 风控管理
// @Security Bearer
// @Param wallet path string true "钱包地址"
// @Success 200 {object} Response
// @Router /admin/v1/risk/blacklist/check/{wallet} [get]
func (h *RiskHandler) CheckBlacklist(c *gin.Context) {
	wallet := c.Param("wallet")
	if wallet == "" {
		BadRequest(c, "钱包地址不能为空")
		return
	}

	result, err := h.riskService.CheckBlacklist(c.Request.Context(), wallet)
	if err != nil {
		InternalError(c, "检查黑名单状态失败: "+err.Error())
		return
	}

	Success(c, result)
}

// AddToBlacklist adds a wallet to the blacklist
// @Summary 添加黑名单
// @Description 将钱包地址添加到黑名单
// @Tags 风控管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.AddToBlacklistRequest true "添加黑名单请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/risk/blacklist [post]
func (h *RiskHandler) AddToBlacklist(c *gin.Context) {
	var req service.AddToBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.riskService.AddToBlacklist(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "黑名单添加成功", nil)
}

// RemoveFromBlacklist removes a wallet from the blacklist
// @Summary 移除黑名单
// @Description 将钱包地址从黑名单中移除
// @Tags 风控管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.RemoveFromBlacklistRequest true "移除黑名单请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/risk/blacklist/remove [post]
func (h *RiskHandler) RemoveFromBlacklist(c *gin.Context) {
	var req service.RemoveFromBlacklistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.riskService.RemoveFromBlacklist(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "黑名单移除成功", nil)
}

// === Risk Events ===

// ListRiskEvents retrieves risk events
// @Summary 获取风险事件列表
// @Description 获取风险事件列表
// @Tags 风控管理
// @Security Bearer
// @Param wallet query string false "钱包地址"
// @Param event_type query int false "事件类型"
// @Param level query int false "风险等级"
// @Param unacknowledged_only query bool false "仅显示未确认的" default(false)
// @Param start_time query int false "开始时间(毫秒)"
// @Param end_time query int false "结束时间(毫秒)"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} Response
// @Router /admin/v1/risk/events [get]
func (h *RiskHandler) ListRiskEvents(c *gin.Context) {
	var req service.RiskEventListRequest
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

	events, err := h.riskService.ListRiskEvents(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, "获取风险事件列表失败: "+err.Error())
		return
	}

	Success(c, events)
}

// GetRiskEvent retrieves a specific risk event
// @Summary 获取风险事件详情
// @Description 根据事件ID获取风险事件详情
// @Tags 风控管理
// @Security Bearer
// @Param event_id path string true "事件ID"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Failure 404 {object} Response
// @Router /admin/v1/risk/events/{event_id} [get]
func (h *RiskHandler) GetRiskEvent(c *gin.Context) {
	eventID := c.Param("event_id")
	if eventID == "" {
		BadRequest(c, "事件ID不能为空")
		return
	}

	event, err := h.riskService.GetRiskEvent(c.Request.Context(), eventID)
	if err != nil {
		InternalError(c, "获取风险事件失败: "+err.Error())
		return
	}

	Success(c, event)
}

// AcknowledgeRiskEvent acknowledges a risk event
// @Summary 确认风险事件
// @Description 确认（处理）风险事件
// @Tags 风控管理
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.AcknowledgeRiskEventRequest true "确认事件请求"
// @Success 200 {object} Response
// @Failure 400 {object} Response
// @Router /admin/v1/risk/events/acknowledge [post]
func (h *RiskHandler) AcknowledgeRiskEvent(c *gin.Context) {
	var req service.AcknowledgeRiskEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	if err := h.riskService.AcknowledgeRiskEvent(c.Request.Context(), &req); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "风险事件已确认", nil)
}

// === Risk Statistics ===

// GetRiskStats retrieves risk statistics
// @Summary 获取风控统计
// @Description 获取风控相关统计数据
// @Tags 风控管理
// @Security Bearer
// @Param period_hours query int false "统计周期(小时)" default(24)
// @Success 200 {object} Response
// @Router /admin/v1/risk/stats [get]
func (h *RiskHandler) GetRiskStats(c *gin.Context) {
	periodHours := int32(24)
	if ph, err := strconv.ParseInt(c.DefaultQuery("period_hours", "24"), 10, 32); err == nil {
		periodHours = int32(ph)
	}

	stats, err := h.riskService.GetRiskStats(c.Request.Context(), periodHours)
	if err != nil {
		InternalError(c, "获取风控统计失败: "+err.Error())
		return
	}

	Success(c, stats)
}
