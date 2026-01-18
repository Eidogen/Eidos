package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login 登录
// @Summary 管理员登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body service.LoginRequest true "登录请求"
// @Success 200 {object} Response{data=service.LoginResponse}
// @Router /admin/v1/session [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	resp, err := h.authService.Login(c.Request.Context(), &req, ip, userAgent)
	if err != nil {
		Error(c, 401, err.Error())
		return
	}

	Success(c, resp)
}

// Logout 登出
// @Summary 管理员登出
// @Tags 认证
// @Security Bearer
// @Success 200 {object} Response
// @Router /admin/v1/session [delete]
func (h *AuthHandler) Logout(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	username := middleware.GetUsername(c)
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	if err := h.authService.Logout(c.Request.Context(), adminID, username, ip, userAgent); err != nil {
		InternalError(c, "登出失败")
		return
	}

	Success(c, nil)
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ChangePassword 修改密码
// @Summary 修改密码
// @Tags 认证
// @Security Bearer
// @Accept json
// @Produce json
// @Param request body ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} Response
// @Router /admin/v1/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}

	adminID := middleware.GetAdminID(c)

	if err := h.authService.ChangePassword(c.Request.Context(), adminID, req.OldPassword, req.NewPassword); err != nil {
		Error(c, 400, err.Error())
		return
	}

	SuccessWithMessage(c, "密码修改成功", nil)
}

// GetProfile 获取当前用户信息
// @Summary 获取当前用户信息
// @Tags 认证
// @Security Bearer
// @Success 200 {object} Response{data=service.Claims}
// @Router /admin/v1/profile [get]
func (h *AuthHandler) GetProfile(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		Unauthorized(c, "未认证")
		return
	}

	Success(c, gin.H{
		"admin_id":    claims.AdminID,
		"username":    claims.Username,
		"role":        claims.Role,
		"permissions": claims.Permissions,
	})
}
