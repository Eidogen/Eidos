package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockPinger implements the Pinger interface for testing
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping() error {
	return m.err
}

func TestNewHealthHandler(t *testing.T) {
	pinger := &mockPinger{}
	deps := &HealthDeps{
		RedisClient: pinger,
	}
	handler := NewHealthHandler(deps)
	assert.NotNil(t, handler)
}

func TestNewHealthHandler_NilDeps(t *testing.T) {
	handler := NewHealthHandler(nil)
	assert.NotNil(t, handler)
}

func TestHealthHandler_SetReady(t *testing.T) {
	handler := NewHealthHandler(nil)

	// Default state
	handler.SetReady(true)
	// No panic means success

	handler.SetReady(false)
	// No panic means success
}

func TestHealthHandler_Live(t *testing.T) {
	handler := NewHealthHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/live", nil)

	handler.Live(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthHandler_Ready_NotReady(t *testing.T) {
	handler := NewHealthHandler(nil)
	handler.SetReady(false)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHandler_Ready_Ready(t *testing.T) {
	pinger := &mockPinger{}
	deps := &HealthDeps{
		RedisClient: pinger,
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthHandler_Ready_RedisError(t *testing.T) {
	pinger := &mockPinger{err: errors.New("redis connection failed")}
	deps := &HealthDeps{
		RedisClient: pinger,
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHandler_Ready_NoRedisClient(t *testing.T) {
	handler := NewHealthHandler(nil)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	// Should still be OK if no redis client is configured
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthHandler_Ready_TradingClientOK(t *testing.T) {
	deps := &HealthDeps{
		TradingClient: &mockPinger{},
		RedisClient:   &mockPinger{},
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthHandler_Ready_TradingClientError(t *testing.T) {
	deps := &HealthDeps{
		TradingClient: &mockPinger{err: errors.New("trading service unavailable")},
		RedisClient:   &mockPinger{},
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHandler_Ready_MarketClientOK(t *testing.T) {
	deps := &HealthDeps{
		TradingClient: &mockPinger{},
		MarketClient:  &mockPinger{},
		RedisClient:   &mockPinger{},
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHealthHandler_Ready_MarketClientError(t *testing.T) {
	deps := &HealthDeps{
		TradingClient: &mockPinger{},
		MarketClient:  &mockPinger{err: errors.New("market service unavailable")},
		RedisClient:   &mockPinger{},
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHandler_Ready_AllClientsOK(t *testing.T) {
	deps := &HealthDeps{
		TradingClient: &mockPinger{},
		MarketClient:  &mockPinger{},
		RedisClient:   &mockPinger{},
	}
	handler := NewHealthHandler(deps)
	handler.SetReady(true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/health/ready", nil)

	handler.Ready(c)

	assert.Equal(t, http.StatusOK, w.Code)
}
