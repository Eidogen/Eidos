package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
	adminService *service.AdminService
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler(adminService *service.AdminService) *AdminHandler {
	return &AdminHandler{
		adminService: adminService,
	}
}

// List 获取管理员列表
// @Summary 获取管理员列表
// @Tags 管理员
// @Security Bearer
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} PagedResponse{data=[]model.Admin}
// @Router /admin/v1/admins [get]
func (h *AdminHandler) List(c *gin.Context) {
	var page model.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		BadRequest(c, "参数错误")
		return
	}

	admins, err := h.adminService.List(c.Request.Context(), &page)
	if err != nil {
		InternalError(c, "获取管理员列表失败")
		return
	}

	SuccessPaged(c, admins, page.Page, page.PageSize, page.Total)
}

// Get 获取管理员详情
// @Summary 获取管理员详情
// @Tags 管理员
// @Security Bearer
// @Param id path int true "管理员ID"
// @Success 200 {object} Response{data=model.Admin}
// @Router /admin/v1/admins/{id} [get]
func (h *AdminHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	admin, err := h.adminService.GetByID(c.Request.Context(), id)
	if err != nil {
		InternalError(c, "获取管理员失败")
		return
	}
	if admin == nil {
		NotFound(c, "管理员不存在")
		return
	}

	Success(c, admin)
}

// Create 创建管理员
// @Summary 创建管理员
// @Tags 管理员
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body service.CreateAdminRequest true "创建管理员请求"
// @Success 200 {object} Response{data=model.Admin}
// @Router /admin/v1/admins [post]
func (h *AdminHandler) Create(c *gin.Context) {
	var req service.CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.OperatorID = middleware.GetAdminID(c)

	admin, err := h.adminService.Create(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, admin)
}

// Update 更新管理员
// @Summary 更新管理员
// @Tags 管理员
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "管理员ID"
// @Param request body service.UpdateAdminRequest true "更新管理员请求"
// @Success 200 {object} Response{data=model.Admin}
// @Router /admin/v1/admins/{id} [put]
func (h *AdminHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req service.UpdateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	req.ID = id
	req.OperatorID = middleware.GetAdminID(c)

	admin, err := h.adminService.Update(c.Request.Context(), &req)
	if err != nil {
		Error(c, 400, err.Error())
		return
	}

	Success(c, admin)
}

// Delete 删除管理员
// @Summary 删除管理员
// @Tags 管理员
// @Security Bearer
// @Param id path int true "管理员ID"
// @Success 200 {object} Response
// @Router /admin/v1/admins/{id} [delete]
func (h *AdminHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	operatorID := middleware.GetAdminID(c)

	if err := h.adminService.Delete(c.Request.Context(), id, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "删除成功", nil)
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ResetPassword 重置管理员密码
// @Summary 重置管理员密码
// @Tags 管理员
// @Security Bearer
// @Accept json
// @Produce json
// @Param id path int true "管理员ID"
// @Param request body ResetPasswordRequest true "重置密码请求"
// @Success 200 {object} Response
// @Router /admin/v1/admins/{id}/password [put]
func (h *AdminHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的ID")
		return
	}

	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	operatorID := middleware.GetAdminID(c)

	if err := h.adminService.ResetPassword(c.Request.Context(), id, req.NewPassword, operatorID); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "密码重置成功", nil)
}
