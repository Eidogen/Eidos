package ws

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

// Handler WebSocket 处理器
type Handler struct {
	hub      *Hub
	logger   *slog.Logger
	cfg      config.WebSocketConfig
	upgrader websocket.Upgrader
}

// NewHandler 创建 WebSocket 处理器
func NewHandler(hub *Hub, logger *slog.Logger, cfg config.WebSocketConfig) *Handler {
	h := &Handler{
		hub:    hub,
		logger: logger,
		cfg:    cfg,
	}

	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     h.checkOrigin,
	}

	return h
}

// checkOrigin validates the Origin header for WebSocket connections
func (h *Handler) checkOrigin(r *http.Request) bool {
	// If allow all origins is enabled (dev mode), accept all
	if h.cfg.AllowAllOrigins {
		return true
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		// No origin header - could be a same-origin request or non-browser client
		// For security, reject requests without Origin header in production
		h.logger.Debug("websocket connection without origin header",
			"remote", r.RemoteAddr,
		)
		return false
	}

	// Parse the origin URL
	originURL, err := url.Parse(origin)
	if err != nil {
		h.logger.Warn("invalid origin URL",
			"origin", origin,
			"error", err,
		)
		return false
	}

	// Extract the host (domain:port or just domain)
	originHost := originURL.Host

	// Check against whitelist
	for _, allowed := range h.cfg.AllowedOrigins {
		if matchOrigin(originHost, allowed) {
			return true
		}
	}

	h.logger.Warn("websocket origin rejected",
		"origin", origin,
		"remote", r.RemoteAddr,
	)
	return false
}

// matchOrigin checks if the origin matches the allowed pattern
// Supports:
// - Exact match: "example.com"
// - Wildcard subdomain: "*.example.com"
// - With port: "example.com:8080"
func matchOrigin(origin, pattern string) bool {
	// Normalize: remove trailing slashes
	origin = strings.TrimSuffix(origin, "/")
	pattern = strings.TrimSuffix(pattern, "/")

	// Exact match
	if origin == pattern {
		return true
	}

	// Wildcard subdomain match
	if strings.HasPrefix(pattern, "*.") {
		// Pattern: *.example.com
		// Should match: sub.example.com, deep.sub.example.com
		suffix := pattern[1:] // Remove the *, keep .example.com
		if strings.HasSuffix(origin, suffix) {
			return true
		}
	}

	return false
}

// HandleConnection 处理 WebSocket 连接
// GET /ws
func (h *Handler) HandleConnection(c *gin.Context) {
	// Check connection limit
	if h.hub.ClientCount() >= h.cfg.MaxConnections {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    20002,
			"message": "too many connections",
		})
		return
	}

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	// Create client
	client := NewClient(h.hub, conn, h.logger, h.cfg)

	// Check if authentication info exists (optional for public channels)
	if wallet, exists := c.Get("wallet"); exists {
		client.SetWallet(wallet.(string))
	}

	h.logger.Info("websocket client connected",
		"client", client.ID(),
		"remote", c.Request.RemoteAddr,
		"origin", c.Request.Header.Get("Origin"),
	)

	// Register to Hub
	h.hub.Register(client)

	// Start read/write goroutines
	go client.WritePump()
	go client.ReadPump()
}

// HandleAuthenticatedConnection 处理需要认证的 WebSocket 连接
// GET /ws/private
func (h *Handler) HandleAuthenticatedConnection(c *gin.Context) {
	// Check authentication
	wallet, exists := c.Get("wallet")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    10004,
			"message": "authentication required",
		})
		return
	}

	// Check connection limit
	if h.hub.ClientCount() >= h.cfg.MaxConnections {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    20002,
			"message": "too many connections",
		})
		return
	}

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	// Create authenticated client
	client := NewClient(h.hub, conn, h.logger, h.cfg)
	client.SetWallet(wallet.(string))
	client.SetAuthenticated(true)

	h.logger.Info("authenticated websocket client connected",
		"client", client.ID(),
		"wallet", wallet.(string),
		"remote", c.Request.RemoteAddr,
	)

	// Register to Hub
	h.hub.Register(client)

	// Start read/write goroutines
	go client.WritePump()
	go client.ReadPump()
}
