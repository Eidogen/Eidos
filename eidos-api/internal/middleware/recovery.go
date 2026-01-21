// Package middleware 提供 HTTP 中间件
package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Recovery 返回 panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录堆栈
				stack := debug.Stack()
				logger.Error("panic recovered",
					"error", err,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"stack", string(stack),
				)

				// 返回 500 错误
				c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewErrorResponse(dto.ErrInternalError))
			}
		}()
		c.Next()
	}
}
