package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

const (
	// AuthHeader 认证头
	AuthHeader = "Authorization"
	// BearerPrefix Bearer 前缀
	BearerPrefix = "Bearer "
	// ContextKeyClaims 上下文中的 Claims 键
	ContextKeyClaims = "claims"
	// ContextKeyAdminID 上下文中的 AdminID 键
	ContextKeyAdminID = "admin_id"
	// ContextKeyUsername 上下文中的 Username 键
	ContextKeyUsername = "username"
	// ContextKeyRole 上下文中的 Role 键
	ContextKeyRole = "role"
)

// AuthMiddleware 认证中间件结构体
type AuthMiddleware struct {
	authService *service.AuthService
}

// NewAuthMiddleware 创建认证中间件
func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{authService: authService}
}

// Required 返回需要认证的中间件
func (m *AuthMiddleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthHeader)
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未提供认证信息",
			})
			return
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证格式错误",
			})
			return
		}

		token := strings.TrimPrefix(authHeader, BearerPrefix)
		claims, err := m.authService.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "认证失败: " + err.Error(),
			})
			return
		}

		// 将用户信息存入上下文
		c.Set(ContextKeyClaims, claims)
		c.Set(ContextKeyAdminID, claims.AdminID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Set(ContextKeyRole, claims.Role)

		c.Next()
	}
}

// GetAdminID 从上下文获取管理员 ID
func GetAdminID(c *gin.Context) int64 {
	if id, exists := c.Get(ContextKeyAdminID); exists {
		return id.(int64)
	}
	return 0
}

// GetUsername 从上下文获取用户名
func GetUsername(c *gin.Context) string {
	if username, exists := c.Get(ContextKeyUsername); exists {
		return username.(string)
	}
	return ""
}

// GetClaims 从上下文获取 Claims
func GetClaims(c *gin.Context) *service.Claims {
	if claims, exists := c.Get(ContextKeyClaims); exists {
		return claims.(*service.Claims)
	}
	return nil
}
