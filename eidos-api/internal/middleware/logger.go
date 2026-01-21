package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Logger 返回请求日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算耗时
		latency := time.Since(start)
		status := c.Writer.Status()

		// 获取 TraceID
		traceID, _ := c.Get(TraceIDKey)

		// 基础字段
		args := []any{
			"status", status,
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"ip", c.ClientIP(),
			"latency", latency,
			"user_agent", c.Request.UserAgent(),
		}

		// TraceID
		if tid, ok := traceID.(string); ok && tid != "" {
			args = append(args, "trace_id", tid)
		}

		// 钱包地址
		if wallet, exists := c.Get("wallet"); exists {
			args = append(args, "wallet", wallet.(string))
		}

		// 错误信息
		if len(c.Errors) > 0 {
			args = append(args, "errors", c.Errors.Errors())
		}

		// 根据状态码选择日志级别
		switch {
		case status >= 500:
			logger.Error("request", args...)
		case status >= 400:
			logger.Warn("request", args...)
		default:
			logger.Info("request", args...)
		}
	}
}

// bodyLogWriter 用于记录响应体的 ResponseWriter 包装
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// LoggerWithBody 返回记录请求体和响应体的日志中间件（仅用于调试）
func LoggerWithBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 读取请求体
		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		// 包装 ResponseWriter
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// 处理请求
		c.Next()

		// 记录日志
		latency := time.Since(start)
		logger.Debug("request with body",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", latency,
			"request_body", string(reqBody),
			"response_body", blw.body.String(),
		)
	}
}
