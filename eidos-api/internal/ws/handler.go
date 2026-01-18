package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

// Handler WebSocket 处理器
type Handler struct {
	hub      *Hub
	logger   *zap.Logger
	cfg      config.WebSocketConfig
	upgrader websocket.Upgrader
}

// NewHandler 创建 WebSocket 处理器
func NewHandler(hub *Hub, logger *zap.Logger, cfg config.WebSocketConfig) *Handler {
	return &Handler{
		hub:    hub,
		logger: logger,
		cfg:    cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				// TODO: 生产环境应该验证 Origin
				return true
			},
		},
	}
}

// HandleConnection 处理 WebSocket 连接
// GET /ws
func (h *Handler) HandleConnection(c *gin.Context) {
	// 检查连接数限制
	if h.hub.ClientCount() >= h.cfg.MaxConnections {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"code":    20002,
			"message": "too many connections",
		})
		return
	}

	// 升级连接
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", zap.Error(err))
		return
	}

	// 创建客户端
	client := NewClient(h.hub, conn, h.logger, h.cfg)

	// 检查是否有认证信息（可选）
	if wallet, exists := c.Get("wallet"); exists {
		client.SetWallet(wallet.(string))
	}

	h.logger.Info("websocket client connected",
		zap.String("client", client.ID()),
		zap.String("remote", c.Request.RemoteAddr),
	)

	// 注册到 Hub
	h.hub.Register(client)

	// 启动读写协程
	go client.WritePump()
	go client.ReadPump()
}
