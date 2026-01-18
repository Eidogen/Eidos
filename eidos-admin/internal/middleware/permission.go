package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// RequirePermission 权限检查中间件
func RequirePermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未认证",
			})
			return
		}

		// 检查是否有任一所需权限
		hasPermission := false
		for _, perm := range permissions {
			if claims.Role.HasPermission(perm) {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "权限不足",
			})
			return
		}

		c.Next()
	}
}

// RequireRole 角色检查中间件
func RequireRole(roles ...model.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未认证",
			})
			return
		}

		hasRole := false
		for _, role := range roles {
			if claims.Role == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "角色权限不足",
			})
			return
		}

		c.Next()
	}
}

// RequireSuperAdmin 超级管理员检查
func RequireSuperAdmin() gin.HandlerFunc {
	return RequireRole(model.RoleSuperAdmin)
}
