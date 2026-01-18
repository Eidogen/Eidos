// Package middleware 提供 HTTP 中间件
package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 返回 panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录堆栈
				stack := debug.Stack()
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.ByteString("stack", stack),
				)

				// 返回 500 错误
				c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewErrorResponse(dto.ErrInternalError))
			}
		}()
		c.Next()
	}
}
