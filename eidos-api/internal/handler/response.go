// Package handler 提供 HTTP 请求处理
package handler

import (
	"net/http"
	"strconv"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/gin-gonic/gin"
)

// Success 返回成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, dto.NewSuccessResponse(data))
}

// SuccessWithPagination 返回分页成功响应
func SuccessWithPagination(c *gin.Context, items interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, dto.NewPagedResponse(items, total, page, pageSize))
}

// SuccessPaginated 返回分页响应（使用 PaginatedResponse 结构）
func SuccessPaginated(c *gin.Context, resp *dto.PaginatedResponse) {
	c.JSON(http.StatusOK, dto.NewPagedResponse(resp.Items, resp.Total, resp.Page, resp.PageSize))
}

// Error 返回业务错误响应
func Error(c *gin.Context, err *dto.BizError) {
	c.JSON(err.HTTPStatus, dto.NewErrorResponse(err))
}

// ErrorWithData 返回带数据的错误响应
func ErrorWithData(c *gin.Context, err *dto.BizError, data interface{}) {
	c.JSON(err.HTTPStatus, &dto.Response{
		Code:    err.Code,
		Message: err.Message,
		Data:    data,
	})
}

// RateLimitError 返回限流错误响应
func RateLimitError(c *gin.Context, limit int, window string, retryAfter int) {
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	c.JSON(http.StatusTooManyRequests, dto.NewRateLimitResponse(limit, window, retryAfter))
}

// BadRequest 返回参数错误响应
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, &dto.Response{
		Code:    dto.ErrInvalidParams.Code,
		Message: message,
	})
}

// NotFound 返回资源不存在响应
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, &dto.Response{
		Code:    dto.ErrOrderNotFound.Code,
		Message: message,
	})
}

// InternalError 返回内部错误响应
func InternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, dto.NewErrorResponse(dto.ErrInternalError))
}

// GetWallet 从 context 获取钱包地址
func GetWallet(c *gin.Context) string {
	wallet, _ := c.Get("wallet")
	if w, ok := wallet.(string); ok {
		return w
	}
	return ""
}

// GetTraceID 从 context 获取 TraceID
func GetTraceID(c *gin.Context) string {
	traceID, _ := c.Get("trace_id")
	if t, ok := traceID.(string); ok {
		return t
	}
	return ""
}
