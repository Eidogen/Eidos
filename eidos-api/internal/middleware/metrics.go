package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

// Metrics 返回 Prometheus 指标记录中间件
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 metrics 端点自身，避免自循环
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()
		reqSize := c.Request.ContentLength

		c.Next()

		// 计算耗时
		duration := time.Since(start).Seconds()

		// 获取路由路径（使用模板路径而非实际路径，避免高基数）
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		// 获取响应大小
		respSize := int64(c.Writer.Size())
		if respSize < 0 {
			respSize = 0
		}

		// 记录指标
		metrics.RecordHTTPRequest(
			c.Request.Method,
			path,
			strconv.Itoa(c.Writer.Status()),
			duration,
			reqSize,
			respSize,
		)
	}
}
