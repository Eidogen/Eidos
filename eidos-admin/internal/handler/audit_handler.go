package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// AuditHandler 审计日志处理器
type AuditHandler struct {
	auditService *service.AuditService
}

// NewAuditHandler 创建审计日志处理器
func NewAuditHandler(auditService *service.AuditService) *AuditHandler {
	return &AuditHandler{
		auditService: auditService,
	}
}

// List 获取审计日志列表
// @Summary 获取审计日志列表
// @Tags 审计日志
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param admin_id query int false "操作员ID"
// @Param action query string false "操作类型"
// @Param resource_type query string false "资源类型"
// @Param start_time query int false "开始时间戳(毫秒)"
// @Param end_time query int false "结束时间戳(毫秒)"
// @Success 200 {object} PagedResponse{data=[]model.AuditLog}
// @Router /admin/v1/audits [get]
func (h *AuditHandler) List(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	var filter AuditLogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		BadRequest(c, "筛选参数错误")
		return
	}

	query := &service.AuditLogQuery{
		Action:       model.AuditAction(filter.Action),
		ResourceType: model.ResourceType(filter.ResourceType),
	}
	if filter.AdminID > 0 {
		query.AdminID = &filter.AdminID
	}
	if filter.StartTime > 0 {
		query.StartTime = &filter.StartTime
	}
	if filter.EndTime > 0 {
		query.EndTime = &filter.EndTime
	}

	logs, err := h.auditService.List(c.Request.Context(), &page, query)
	if err != nil {
		InternalError(c, "获取审计日志失败")
		return
	}

	SuccessPaged(c, logs, page.Page, page.PageSize, page.Total)
}

// AuditLogFilter 审计日志筛选
type AuditLogFilter struct {
	AdminID      int64  `form:"admin_id"`
	Action       string `form:"action"`
	ResourceType string `form:"resource_type"`
	StartTime    int64  `form:"start_time"`
	EndTime      int64  `form:"end_time"`
}

// Get 获取审计日志详情
// @Summary 获取审计日志详情
// @Tags 审计日志
// @Security Bearer
// @Param id path int true "日志ID"
// @Success 200 {object} Response{data=model.AuditLog}
// @Router /admin/v1/audits/{id} [get]
func (h *AuditHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	log, err := h.auditService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取审计日志失败")
		return
	}
	if log == nil {
		NotFound(c, "审计日志不存在")
		return
	}

	Success(c, log)
}

// GetByAdmin 获取指定管理员的操作日志
// @Summary 获取指定管理员的操作日志
// @Tags 审计日志
// @Security Bearer
// @Param admin_id path int true "管理员ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PagedResponse{data=[]model.AuditLog}
// @Router /admin/v1/audits/admin/{admin_id} [get]
func (h *AuditHandler) GetByAdmin(c *gin.Context) {
	adminID, err := strconv.ParseInt(c.Param("admin_id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的管理员ID")
		return
	}

	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	logs, err := h.auditService.GetByAdminID(c.Request.Context(), adminID, &page)
	if err != nil {
		InternalError(c, "获取审计日志失败")
		return
	}

	SuccessPaged(c, logs, page.Page, page.PageSize, page.Total)
}

// GetByResource 获取指定资源的操作日志
// @Summary 获取指定资源的操作日志
// @Tags 审计日志
// @Security Bearer
// @Param resource_type path string true "资源类型"
// @Param resource_id path string true "资源ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PagedResponse{data=[]model.AuditLog}
// @Router /admin/v1/audits/resource/{resource_type}/{resource_id} [get]
func (h *AuditHandler) GetByResource(c *gin.Context) {
	resourceType := c.Param("resource_type")
	resourceID := c.Param("resource_id")

	if resourceType == "" || resourceID == "" {
		BadRequest(c, "资源类型和资源ID不能为空")
		return
	}

	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	logs, err := h.auditService.GetByResourcePaged(c.Request.Context(), resourceType, resourceID, &page)
	if err != nil {
		InternalError(c, "获取审计日志失败")
		return
	}

	SuccessPaged(c, logs, page.Page, page.PageSize, page.Total)
}

// GetActions 获取所有操作类型
// @Summary 获取所有操作类型
// @Tags 审计日志
// @Security Bearer
// @Success 200 {object} Response{data=[]string}
// @Router /admin/v1/audits/actions [get]
func (h *AuditHandler) GetActions(c *gin.Context) {
	actions := []string{
		"login",
		"logout",
		"create",
		"update",
		"delete",
		"enable",
		"disable",
		"reset_password",
		"change_password",
		"config_update",
		"market_update",
		"status_change",
	}
	Success(c, actions)
}

// GetResourceTypes 获取所有资源类型
// @Summary 获取所有资源类型
// @Tags 审计日志
// @Security Bearer
// @Success 200 {object} Response{data=[]string}
// @Router /admin/v1/audits/resource-types [get]
func (h *AuditHandler) GetResourceTypes(c *gin.Context) {
	resourceTypes := []string{
		"admin",
		"market",
		"config",
		"user",
		"order",
		"trade",
		"withdrawal",
		"settlement",
	}
	Success(c, resourceTypes)
}

// Export 导出审计日志
// @Summary 导出审计日志
// @Tags 审计日志
// @Security Bearer
// @Param admin_id query int false "操作员ID"
// @Param action query string false "操作类型"
// @Param resource_type query string false "资源类型"
// @Param start_time query int true "开始时间戳(毫秒)"
// @Param end_time query int true "结束时间戳(毫秒)"
// @Param format query string false "导出格式" default(csv)
// @Success 200 {file} binary "导出文件"
// @Router /admin/v1/audits/export [get]
func (h *AuditHandler) Export(c *gin.Context) {
	var filter AuditLogFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		BadRequest(c, "筛选参数错误")
		return
	}

	if filter.StartTime == 0 || filter.EndTime == 0 {
		BadRequest(c, "开始时间和结束时间不能为空")
		return
	}

	format := c.DefaultQuery("format", "csv")
	if format != "csv" && format != "json" {
		BadRequest(c, "不支持的导出格式")
		return
	}

	query := &service.AuditLogQuery{
		Action:       model.AuditAction(filter.Action),
		ResourceType: model.ResourceType(filter.ResourceType),
	}
	if filter.AdminID > 0 {
		query.AdminID = &filter.AdminID
	}
	if filter.StartTime > 0 {
		query.StartTime = &filter.StartTime
	}
	if filter.EndTime > 0 {
		query.EndTime = &filter.EndTime
	}

	data, filename, err := h.auditService.Export(c.Request.Context(), query, format)
	if err != nil {
		InternalError(c, "导出审计日志失败")
		return
	}

	contentType := "text/csv"
	if format == "json" {
		contentType = "application/json"
	}

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", contentType)
	c.Data(200, contentType, data)
}
