package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GinMiddleware 返回 Gin 中间件用于收集 HTTP 请求指标
func GinMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 /metrics 和 /health 端点自身的指标收集
		if c.Request.URL.Path == "/metrics" || c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
			c.Next()
			return
		}

		// 增加正在处理的请求数
		RequestsInFlight.WithLabelValues(serviceName, c.Request.Method).Inc()
		defer RequestsInFlight.WithLabelValues(serviceName, c.Request.Method).Dec()

		// 开始计时
		start := time.Now()

		// 记录请求大小
		RequestSize.WithLabelValues(serviceName, c.Request.Method).Observe(float64(c.Request.ContentLength))

		// 处理请求
		c.Next()

		// 计算耗时
		duration := time.Since(start).Seconds()

		// 获取状态码
		statusCode := strconv.Itoa(c.Writer.Status())

		// 获取路径模板 (如果有的话，否则使用原始路径)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// 记录指标
		RecordRequest(serviceName, c.Request.Method+" "+path, statusCode, duration)

		// 记录响应大小
		ResponseSize.WithLabelValues(serviceName, c.Request.Method).Observe(float64(c.Writer.Size()))
	}
}

// GinMetricsHandler 返回 Gin 处理器用于暴露 /metrics 端点
func GinMetricsHandler() gin.HandlerFunc {
	h := Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
