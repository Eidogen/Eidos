// Package handler 提供 HTTP 处理器
package handler

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	ready atomic.Bool
	deps  *HealthDeps
}

// HealthDeps 健康检查依赖
type HealthDeps struct {
	TradingClient interface{ Ping() error }
	MarketClient  interface{ Ping() error }
	RedisClient   interface{ Ping() error }
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(deps *HealthDeps) *HealthHandler {
	h := &HealthHandler{deps: deps}
	h.ready.Store(false)
	return h
}

// SetReady 设置就绪状态
func (h *HealthHandler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Live 存活探针
// GET /health/live
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// Ready 就绪探针
// GET /health/ready
func (h *HealthHandler) Ready(c *gin.Context) {
	if !h.ready.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "service initializing",
		})
		return
	}

	// 检查依赖
	checks := make(map[string]string)
	allOK := true

	if h.deps != nil {
		if h.deps.TradingClient != nil {
			if err := h.deps.TradingClient.Ping(); err != nil {
				checks["trading"] = err.Error()
				allOK = false
			} else {
				checks["trading"] = "ok"
			}
		}

		if h.deps.MarketClient != nil {
			if err := h.deps.MarketClient.Ping(); err != nil {
				checks["market"] = err.Error()
				allOK = false
			} else {
				checks["market"] = "ok"
			}
		}

		if h.deps.RedisClient != nil {
			if err := h.deps.RedisClient.Ping(); err != nil {
				checks["redis"] = err.Error()
				allOK = false
			} else {
				checks["redis"] = "ok"
			}
		}
	}

	if !allOK {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"checks": checks,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"checks": checks,
	})
}
