package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Logger 日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 记录日志
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		userAgent := c.Request.UserAgent()

		fields := []any{
			"status", status,
			"method", method,
			"path", path,
			"query", query,
			"ip", clientIP,
			"user_agent", userAgent,
			"latency", latency,
		}

		// 添加管理员信息 (如果有)
		if adminID := GetAdminID(c); adminID > 0 {
			fields = append(fields, "admin_id", adminID)
		}
		if username := GetUsername(c); username != "" {
			fields = append(fields, "username", username)
		}

		// 添加错误信息 (如果有)
		if len(c.Errors) > 0 {
			fields = append(fields, "errors", c.Errors.String())
		}

		// 根据状态码选择日志级别
		switch {
		case status >= 500:
			logger.Error("request completed", fields...)
		case status >= 400:
			logger.Warn("request completed", fields...)
		default:
			logger.Info("request completed", fields...)
		}
	}
}
