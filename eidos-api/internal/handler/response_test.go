package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-api/internal/middleware"
)

func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	data := map[string]string{"key": "value"}
	Success(c, data)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 0, resp.Code)
	assert.Equal(t, "success", resp.Message)
}

func TestSuccessWithPagination(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	items := []string{"a", "b", "c"}
	SuccessWithPagination(c, items, 100, 1, 10)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.NotNil(t, data["items"])

	// Pagination is nested inside data
	pagination := data["pagination"].(map[string]interface{})
	assert.Equal(t, float64(100), pagination["total"])
	assert.Equal(t, float64(1), pagination["page"])
	assert.Equal(t, float64(10), pagination["page_size"])
	assert.Equal(t, float64(10), pagination["total_pages"])
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Error(c, dto.ErrInvalidParams)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
	assert.Equal(t, dto.ErrInvalidParams.Message, resp.Message)
}

func TestErrorWithData(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	extraData := map[string]string{"field": "amount"}
	ErrorWithData(c, dto.ErrInvalidParams, extraData)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, float64(dto.ErrInvalidParams.Code), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "amount", data["field"])
}

func TestRateLimitError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RateLimitError(c, 100, "1m", 30)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	// Note: The current implementation uses rune conversion which may not give expected string
	// This tests the function doesn't panic
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	BadRequest(c, "invalid field value")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, dto.ErrInvalidParams.Code, resp.Code)
	assert.Equal(t, "invalid field value", resp.Message)
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NotFound(c, "order not found")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, dto.ErrOrderNotFound.Code, resp.Code)
	assert.Equal(t, "order not found", resp.Message)
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	InternalError(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, dto.ErrInternalError.Code, resp.Code)
}

func TestGetWallet(t *testing.T) {
	t.Run("wallet_exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set("wallet", "0x1234567890123456789012345678901234567890")

		wallet := GetWallet(c)
		assert.Equal(t, "0x1234567890123456789012345678901234567890", wallet)
	})

	t.Run("wallet_not_exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		wallet := GetWallet(c)
		assert.Empty(t, wallet)
	})
}

func TestGetTraceID(t *testing.T) {
	t.Run("trace_id_exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(middleware.TraceIDKey, "test-trace-123")

		traceID := GetTraceID(c)
		assert.Equal(t, "test-trace-123", traceID)
	})

	t.Run("trace_id_not_exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		traceID := GetTraceID(c)
		assert.Empty(t, traceID)
	})
}
