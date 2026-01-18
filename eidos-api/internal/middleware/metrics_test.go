package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

func TestMetricsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试路由
	r := gin.New()
	r.Use(Metrics())
	r.GET("/api/v1/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 获取初始计数
	initialCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/test", "200"))

	// 发送请求
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证指标被记录
	newCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/test", "200"))
	assert.Equal(t, initialCount+1, newCount)
}

func TestMetricsMiddleware_SkipsMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试路由
	r := gin.New()
	r.Use(Metrics())
	r.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, "prometheus metrics")
	})

	// 获取初始计数 - metrics 端点不应该被跟踪
	initialCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200"))

	// 发送请求到 /metrics
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证 /metrics 端点的请求不被记录（避免自循环）
	newCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/metrics", "200"))
	assert.Equal(t, initialCount, newCount)
}

func TestMetricsMiddleware_RecordsDifferentStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试路由
	r := gin.New()
	r.Use(Metrics())
	r.GET("/api/v1/success", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/api/v1/notfound", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
	r.GET("/api/v1/error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	})

	tests := []struct {
		path   string
		status string
	}{
		{"/api/v1/success", "200"},
		{"/api/v1/notfound", "404"},
		{"/api/v1/error", "500"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			initialCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", tt.path, tt.status))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			newCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", tt.path, tt.status))
			assert.Equal(t, initialCount+1, newCount, "Counter for %s should increment", tt.path)
		})
	}
}

func TestMetricsMiddleware_UnknownPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试路由（没有定义的路由）
	r := gin.New()
	r.Use(Metrics())

	// 获取初始计数
	initialCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "unknown", "404"))

	// 发送请求到未定义的路由
	req := httptest.NewRequest(http.MethodGet, "/undefined/path", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 验证未定义路由使用 "unknown" 作为路径标签
	newCount := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "unknown", "404"))
	assert.Equal(t, initialCount+1, newCount)
}
