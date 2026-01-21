package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewHandler(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	cfg := config.WebSocketConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		MaxConnections:  100,
	}

	handler := NewHandler(hub, logger, cfg)
	assert.NotNil(t, handler)
	assert.Equal(t, hub, handler.hub)
	assert.Equal(t, logger, handler.logger)
}

func TestHandler_HandleConnection_MaxConnectionsExceeded(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		MaxConnections: 0, // 设置为 0，任何连接都会超过限制
	}

	handler := NewHandler(hub, logger, cfg)

	// 创建测试请求
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ws", nil)

	// 调用处理函数
	handler.HandleConnection(c)

	// 验证响应
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandler_HandleConnection_Upgrade_NoWebSocket(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		MaxConnections:  100,
	}

	handler := NewHandler(hub, logger, cfg)

	// 创建普通 HTTP 请求（不是 WebSocket 升级请求）
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ws", nil)

	// 调用处理函数
	handler.HandleConnection(c)

	// 由于没有 WebSocket 升级头，升级会失败
	// 但我们不会收到 HTTP 响应，因为 gorilla/websocket 会直接写入连接
	// 这里我们只验证没有 panic
}

func TestHandler_HandleConnection_WithWallet(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()
	defer hub.Stop()

	cfg := config.WebSocketConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		MaxConnections:  100,
	}

	handler := NewHandler(hub, logger, cfg)

	// 创建带 wallet 的请求
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ws", nil)
	c.Set("wallet", "0x1234567890123456789012345678901234567890")

	// 调用处理函数（升级会失败，因为不是真正的 WebSocket 请求）
	handler.HandleConnection(c)
	// 验证没有 panic
}

func TestHandler_Upgrader_CheckOrigin(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	cfg := config.WebSocketConfig{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		AllowAllOrigins:  true, // 启用允许所有 origin
	}

	handler := NewHandler(hub, logger, cfg)

	// 测试 CheckOrigin 回调（AllowAllOrigins=true 时应返回 true）
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://example.com")

	result := handler.upgrader.CheckOrigin(req)
	assert.True(t, result, "CheckOrigin should return true when AllowAllOrigins is enabled")
}
