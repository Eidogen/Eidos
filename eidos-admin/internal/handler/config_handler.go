package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// ConfigHandler 配置处理器
type ConfigHandler struct {
	configService *service.ConfigService
}

// NewConfigHandler 创建配置处理器
func NewConfigHandler(configService *service.ConfigService) *ConfigHandler {
	return &ConfigHandler{
		configService: configService,
	}
}

// List 获取系统配置列表
// @Summary 获取系统配置列表
// @Tags 系统配置
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param category query string false "配置分类"
// @Success 200 {object} PagedResponse{data=[]model.SystemConfig}
// @Router /admin/v1/configs [get]
func (h *ConfigHandler) List(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	category := c.Query("category")

	configs, err := h.configService.List(c.Request.Context(), &page, category)
	if err != nil {
		InternalError(c, "获取配置列表失败")
		return
	}

	SuccessPaged(c, configs, page.Page, page.PageSize, page.Total)
}

// Get 获取配置详情
// @Summary 获取配置详情
// @Tags 系统配置
// @Security Bearer
// @Param id path int true "配置ID"
// @Success 200 {object} Response{data=model.SystemConfig}
// @Router /admin/v1/configs/{id} [get]
func (h *ConfigHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	config, err := h.configService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取配置失败")
		return
	}
	if config == nil {
		NotFound(c, "配置不存在")
		return
	}

	Success(c, config)
}

// GetByKey 根据键获取配置
// @Summary 根据键获取配置
// @Tags 系统配置
// @Security Bearer
// @Param key path string true "配置键"
// @Success 200 {object} Response{data=model.SystemConfig}
// @Router /admin/v1/configs/key/{key} [get]
func (h *ConfigHandler) GetByKey(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		BadRequest(c, "配置键不能为空")
		return
	}

	config, err := h.configService.GetByKey(c.Request.Context(), key)
	if err != nil {
		InternalError(c, "获取配置失败")
		return
	}
	if config == nil {
		NotFound(c, "配置不存在")
		return
	}

	Success(c, config)
}

// Create 创建配置
// @Summary 创建配置
// @Tags 系统配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.CreateConfigRequest true "创建配置请求"
// @Success 200 {object} Response{data=model.SystemConfig}
// @Router /admin/v1/configs [post]
func (h *ConfigHandler) Create(c *gin.Context) {
	var req service.CreateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	config, err := h.configService.Create(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, config)
}

// Update 更新配置
// @Summary 更新配置
// @Tags 系统配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param request body service.UpdateConfigRequest true "更新配置请求"
// @Success 200 {object} Response{data=model.SystemConfig}
// @Router /admin/v1/configs/{id} [put]
func (h *ConfigHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req service.UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.ID = id
	req.OperatorID = middleware.GetAdminID(c)

	config, err := h.configService.Update(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, config)
}

// Delete 删除配置
// @Summary 删除配置
// @Tags 系统配置
// @Security Bearer
// @Param id path int true "配置ID"
// @Success 200 {object} Response
// @Router /admin/v1/configs/{id} [delete]
func (h *ConfigHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	operatorID := middleware.GetAdminID(c)

	if err := h.configService.DeleteByID(c.Request.Context(), id, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "删除成功", nil)
}

// GetByCategory 获取分类下的所有配置
// @Summary 获取分类下的所有配置
// @Tags 系统配置
// @Security Bearer
// @Param category path string true "配置分类"
// @Success 200 {object} Response{data=[]model.SystemConfig}
// @Router /admin/v1/configs/category/{category} [get]
func (h *ConfigHandler) GetByCategory(c *gin.Context) {
	category := c.Param("category")
	if category == "" {
		BadRequest(c, "配置分类不能为空")
		return
	}

	configs, err := h.configService.GetByCategory(c.Request.Context(), category)
	if err != nil {
		InternalError(c, "获取配置失败")
		return
	}

	Success(c, configs)
}

// GetCurrentVersion 获取当前配置版本
// @Summary 获取当前配置版本
// @Tags 系统配置
// @Security Bearer
// @Success 200 {object} Response{data=ConfigVersionResponse}
// @Router /admin/v1/configs/version [get]
func (h *ConfigHandler) GetCurrentVersion(c *gin.Context) {
	version, err := h.configService.GetCurrentVersion(c.Request.Context())
	if err != nil {
		InternalError(c, "获取配置版本失败")
		return
	}

	Success(c, ConfigVersionResponse{Version: version})
}

// ConfigVersionResponse 配置版本响应
type ConfigVersionResponse struct {
	Version int64 `json:"version"`
}

// GetVersionHistory 获取配置版本历史
// @Summary 获取配置版本历史
// @Tags 系统配置
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PagedResponse{data=[]model.ConfigVersion}
// @Router /admin/v1/configs/versions [get]
func (h *ConfigHandler) GetVersionHistory(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	versions, err := h.configService.GetVersionHistory(c.Request.Context(), &page)
	if err != nil {
		InternalError(c, "获取版本历史失败")
		return
	}

	SuccessPaged(c, versions, page.Page, page.PageSize, page.Total)
}

// BatchUpdate 批量更新配置
// @Summary 批量更新配置
// @Tags 系统配置
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body BatchUpdateConfigRequest true "批量更新请求"
// @Success 200 {object} Response
// @Router /admin/v1/configs/batch [put]
func (h *ConfigHandler) BatchUpdate(c *gin.Context) {
	var req BatchUpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	operatorID := middleware.GetAdminID(c)

	// 转换为 service 层需要的类型
	configs := make([]struct {
		Key   string
		Value string
	}, len(req.Configs))
	for i, item := range req.Configs {
		configs[i].Key = item.Key
		configs[i].Value = item.Value
	}

	if err := h.configService.BatchUpdate(c.Request.Context(), configs, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "批量更新成功", nil)
}

// BatchUpdateConfigRequest 批量更新配置请求
type BatchUpdateConfigRequest struct {
	Configs []ConfigItem `json:"configs" binding:"required,min=1"`
}

// ConfigItem 配置项
type ConfigItem struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value" binding:"required"`
}
