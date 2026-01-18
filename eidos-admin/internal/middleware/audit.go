package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/service"
)

// AuditMiddleware 审计中间件
func AuditMiddleware(auditService *service.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过不需要审计的路径
		if shouldSkipAudit(c.Request.URL.Path) {
			c.Next()
			return
		}

		// 读取请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 记录开始时间
		startTime := time.Now()

		// 处理请求
		c.Next()

		// 获取管理员信息
		adminID := GetAdminID(c)
		username := GetUsername(c)

		// 如果未认证，跳过审计
		if adminID == 0 {
			return
		}

		// 构建审计日志
		auditLog := &model.AuditLog{
			AdminID:       adminID,
			AdminUsername: username,
			IPAddress:     c.ClientIP(),
			UserAgent:     c.Request.UserAgent(),
			Action:        determineAction(c.Request.Method),
			ResourceType:  determineResourceType(c.Request.URL.Path),
			ResourceID:    extractResourceID(c),
			Description:   c.Request.Method + " " + c.Request.URL.Path,
			Status:        model.AuditStatusSuccess,
		}

		// 检查是否有错误
		if len(c.Errors) > 0 {
			auditLog.Status = model.AuditStatusFailed
			auditLog.ErrorMessage = c.Errors.String()
		}

		// 检查响应状态码
		if c.Writer.Status() >= 400 {
			auditLog.Status = model.AuditStatusFailed
		}

		// 异步记录审计日志
		go func() {
			_ = auditService.Create(c.Request.Context(), auditLog)
		}()

		// 记录请求耗时
		_ = time.Since(startTime)
	}
}

// shouldSkipAudit 判断是否跳过审计
func shouldSkipAudit(path string) bool {
	skipPaths := []string{
		"/health/live",
		"/health/ready",
		"/admin/v1/stats/overview",
	}

	for _, p := range skipPaths {
		if path == p {
			return true
		}
	}
	return false
}

// determineAction 根据 HTTP 方法确定操作类型
func determineAction(method string) model.AuditAction {
	switch method {
	case "POST":
		return model.AuditActionCreate
	case "PUT", "PATCH":
		return model.AuditActionUpdate
	case "DELETE":
		return model.AuditActionDelete
	default:
		return model.AuditAction(method)
	}
}

// determineResourceType 根据路径确定资源类型
func determineResourceType(path string) model.ResourceType {
	switch {
	case contains(path, "/admins"):
		return model.ResourceTypeAdmin
	case contains(path, "/markets"):
		return model.ResourceTypeMarket
	case contains(path, "/configs"):
		return model.ResourceTypeSystemConfig
	case contains(path, "/orders"):
		return model.ResourceTypeOrder
	case contains(path, "/users"):
		return model.ResourceTypeUser
	case contains(path, "/reload"):
		return model.ResourceTypeService
	default:
		return model.ResourceType("unknown")
	}
}

// extractResourceID 提取资源 ID
func extractResourceID(c *gin.Context) string {
	// 尝试从路径参数获取
	if id := c.Param("id"); id != "" {
		return id
	}
	if symbol := c.Param("symbol"); symbol != "" {
		return symbol
	}
	if wallet := c.Param("wallet"); wallet != "" {
		return wallet
	}
	if key := c.Param("key"); key != "" {
		return key
	}
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
