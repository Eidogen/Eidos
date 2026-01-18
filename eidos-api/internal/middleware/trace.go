package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// TraceIDHeader 请求头中的 TraceID 字段名
	TraceIDHeader = "X-Trace-ID"
	// TraceIDKey context 中的 TraceID 键名
	TraceIDKey = "trace_id"
)

// Trace 返回 Trace ID 中间件
// 如果请求头中有 X-Trace-ID，则使用该值，否则生成新的 UUID
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// 设置到 context
		c.Set(TraceIDKey, traceID)

		// 设置到响应头
		c.Header(TraceIDHeader, traceID)

		c.Next()
	}
}
