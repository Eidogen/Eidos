// Package router 提供路由注册
package router

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-api/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ws"
)

// Router 路由管理器
type Router struct {
	engine        *gin.Engine
	cfg           *config.Config
	logger        *slog.Logger
	slidingWindow *ratelimit.SlidingWindow
	replayGuard   *cache.ReplayGuard
}

// New 创建路由管理器
func New(
	engine *gin.Engine,
	cfg *config.Config,
	logger *slog.Logger,
	sw *ratelimit.SlidingWindow,
	rg *cache.ReplayGuard,
) *Router {
	return &Router{
		engine:        engine,
		cfg:           cfg,
		logger:        logger,
		slidingWindow: sw,
		replayGuard:   rg,
	}
}

// RegisterMiddleware 注册全局中间件
func (r *Router) RegisterMiddleware() {
	// 中间件链: Recovery → Trace → Logger → CORS → Metrics
	r.engine.Use(
		middleware.Recovery(),
		middleware.Trace(),
		middleware.Logger(),
		middleware.CORS(),
		middleware.Metrics(),
	)
}

// RegisterRoutes 注册路由
func (r *Router) RegisterRoutes(
	healthHandler *handler.HealthHandler,
	orderHandler *handler.OrderHandler,
	balanceHandler *handler.BalanceHandler,
	depositHandler *handler.DepositHandler,
	withdrawalHandler *handler.WithdrawalHandler,
	tradeHandler *handler.TradeHandler,
	marketHandler *handler.MarketHandler,
	wsHub *ws.Hub,
) {
	// ========== 健康检查（无中间件） ==========
	r.engine.GET("/health/live", healthHandler.Live)
	r.engine.GET("/health/ready", healthHandler.Ready)

	// ========== Prometheus 监控端点 ==========
	r.engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// ========== API v1 ==========
	v1 := r.engine.Group("/api/v1")

	// 限流中间件（全局 IP 限流：每分钟 Total 次）
	if r.cfg.RateLimit.Enabled {
		v1.Use(middleware.RateLimitByIP(
			r.slidingWindow,
			r.cfg.RateLimit.PerIP.Total,
			time.Minute,
		))
	}

	// ========== 公开接口（无需认证） ==========
	public := v1.Group("")
	{
		// 市场信息
		public.GET("/markets", marketHandler.ListMarkets)
		public.GET("/ticker/:market", marketHandler.GetTicker)
		public.GET("/tickers", marketHandler.ListTickers)
		public.GET("/depth/:market", marketHandler.GetDepth)
		public.GET("/klines/:market", marketHandler.GetKlines)
		public.GET("/trades/:market/recent", marketHandler.GetRecentTrades)
	}

	// ========== 认证接口 ==========
	authCfg := &middleware.AuthConfig{
		EIP712Config: &r.cfg.EIP712,
		ReplayGuard:  r.replayGuard,
	}
	authMiddleware := middleware.Auth(authCfg)

	private := v1.Group("")
	private.Use(authMiddleware)

	// 认证接口限流（按钱包：每秒 Queries 次）
	if r.cfg.RateLimit.Enabled {
		private.Use(middleware.RateLimitByWallet(
			r.slidingWindow,
			r.cfg.RateLimit.PerWallet.Queries,
			time.Second,
			"api",
		))
	}

	// 订单相关
	orders := private.Group("/orders")
	{
		orders.POST("/prepare", orderHandler.PrepareOrder)
		orders.POST("", orderHandler.CreateOrder)
		orders.GET("", orderHandler.ListOrders)
		orders.GET("/open", orderHandler.ListOpenOrders)
		orders.GET("/:id", orderHandler.GetOrder)
		orders.DELETE("/:id", orderHandler.CancelOrder)
		orders.DELETE("", orderHandler.BatchCancelOrders)
	}

	// 余额相关
	balances := private.Group("/balances")
	{
		balances.GET("", balanceHandler.ListBalances)
		balances.GET("/:token", balanceHandler.GetBalance)
	}

	// 资金流水
	private.GET("/transactions", balanceHandler.ListTransactions)

	// 充值相关
	deposits := private.Group("/deposits")
	{
		deposits.GET("", depositHandler.ListDeposits)
		deposits.GET("/:id", depositHandler.GetDeposit)
	}

	// 提现相关
	withdrawals := private.Group("/withdrawals")
	{
		withdrawals.POST("", withdrawalHandler.CreateWithdrawal)
		withdrawals.GET("", withdrawalHandler.ListWithdrawals)
		withdrawals.GET("/:id", withdrawalHandler.GetWithdrawal)
		withdrawals.DELETE("/:id", withdrawalHandler.CancelWithdrawal)
	}

	// 成交相关（用户成交历史）
	private.GET("/mytrades", tradeHandler.ListTrades)
	private.GET("/mytrades/:id", tradeHandler.GetTrade)

	// ========== WebSocket ==========
	wsHandler := ws.NewHandler(wsHub, r.logger, r.cfg.WebSocket)
	r.engine.GET("/ws", wsHandler.HandleConnection)
}
