package router

import (
	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
)

// Handlers 所有处理器
type Handlers struct {
	Auth       *handler.AuthHandler
	Admin      *handler.AdminHandler
	Market     *handler.MarketHandler
	Stats      *handler.StatsHandler
	Config     *handler.ConfigHandler
	Audit      *handler.AuditHandler
	User       *handler.UserHandler
	Order      *handler.OrderHandler
	Withdrawal *handler.WithdrawalHandler
	Risk       *handler.RiskHandler
}

// SetupRouter 设置路由
func SetupRouter(r *gin.Engine, h *Handlers, authMiddleware *middleware.AuthMiddleware) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API v1
	v1 := r.Group("/admin/v1")
	{
		// 认证相关 (无需登录)
		auth := v1.Group("/auth")
		{
			auth.POST("/login", h.Auth.Login)
		}

		// 需要登录的接口
		authenticated := v1.Group("")
		authenticated.Use(authMiddleware.Required())
		{
			// 认证相关 (需要登录)
			authGroup := authenticated.Group("/auth")
			{
				authGroup.POST("/logout", h.Auth.Logout)
				authGroup.PUT("/password", h.Auth.ChangePassword)
				authGroup.GET("/profile", h.Auth.GetProfile)
			}

			// === 管理员管理 (需要 admin:read/admin:write 权限) ===
			admins := authenticated.Group("/admins")
			{
				admins.GET("", middleware.RequirePermission("admin:read"), h.Admin.List)
				admins.GET("/:id", middleware.RequirePermission("admin:read"), h.Admin.Get)
				admins.POST("", middleware.RequirePermission("admin:write"), h.Admin.Create)
				admins.PUT("/:id", middleware.RequirePermission("admin:write"), h.Admin.Update)
				admins.DELETE("/:id", middleware.RequirePermission("admin:write"), h.Admin.Delete)
				admins.PUT("/:id/password", middleware.RequirePermission("admin:write"), h.Admin.ResetPassword)
			}

			// === 用户管理 (需要 user:read/user:write 权限) ===
			users := authenticated.Group("/users")
			{
				users.GET("/:wallet", middleware.RequirePermission("user:read"), h.User.GetUser)
				users.GET("/:wallet/balances", middleware.RequirePermission("user:read"), h.User.GetUserBalances)
				users.GET("/:wallet/limits", middleware.RequirePermission("user:read"), h.User.GetUserLimits)
				users.GET("/:wallet/rate-limit", middleware.RequirePermission("user:read"), h.User.GetRateLimitStatus)
				users.GET("/:wallet/orders", middleware.RequirePermission("user:read"), h.User.GetUserOrders)
				users.GET("/:wallet/trades", middleware.RequirePermission("user:read"), h.User.GetUserTrades)
				users.GET("/:wallet/deposits", middleware.RequirePermission("user:read"), h.User.GetUserDeposits)
				users.GET("/:wallet/withdrawals", middleware.RequirePermission("user:read"), h.User.GetUserWithdrawals)
				users.POST("/freeze", middleware.RequirePermission("user:write"), h.User.FreezeAccount)
				users.POST("/unfreeze", middleware.RequirePermission("user:write"), h.User.UnfreezeAccount)
				users.POST("/limits", middleware.RequirePermission("user:write"), h.User.SetUserLimits)
				users.POST("/rate-limit/reset", middleware.RequirePermission("user:write"), h.User.ResetRateLimit)
			}

			// === 订单管理 (需要 order:read/order:write 权限) ===
			orders := authenticated.Group("/orders")
			{
				orders.GET("", middleware.RequirePermission("order:read"), h.Order.ListOrders)
				orders.GET("/open", middleware.RequirePermission("order:read"), h.Order.ListOpenOrders)
				orders.GET("/:order_id", middleware.RequirePermission("order:read"), h.Order.GetOrder)
				orders.GET("/:order_id/trades", middleware.RequirePermission("order:read"), h.Order.GetTradesByOrder)
				orders.POST("/cancel", middleware.RequirePermission("order:write"), h.Order.CancelOrder)
				orders.POST("/batch-cancel", middleware.RequirePermission("order:write"), h.Order.BatchCancelOrders)
			}

			// === 成交管理 (需要 trade:read 权限) ===
			trades := authenticated.Group("/trades")
			{
				trades.GET("", middleware.RequirePermission("trade:read"), h.Order.ListTrades)
				trades.GET("/:trade_id", middleware.RequirePermission("trade:read"), h.Order.GetTrade)
			}

			// === 充值管理 (需要 deposit:read 权限) ===
			deposits := authenticated.Group("/deposits")
			{
				deposits.GET("", middleware.RequirePermission("deposit:read"), h.Withdrawal.ListDeposits)
				deposits.GET("/:deposit_id", middleware.RequirePermission("deposit:read"), h.Withdrawal.GetDeposit)
			}

			// === 提现管理 (需要 withdrawal:read/withdrawal:write 权限) ===
			withdrawals := authenticated.Group("/withdrawals")
			{
				withdrawals.GET("", middleware.RequirePermission("withdrawal:read"), h.Withdrawal.ListWithdrawals)
				withdrawals.GET("/pending", middleware.RequirePermission("withdrawal:read"), h.Withdrawal.ListPendingWithdrawals)
				withdrawals.GET("/:withdraw_id", middleware.RequirePermission("withdrawal:read"), h.Withdrawal.GetWithdrawal)
				withdrawals.POST("/reject", middleware.RequirePermission("withdrawal:write"), h.Withdrawal.RejectWithdrawal)
				withdrawals.POST("/retry", middleware.RequirePermission("withdrawal:write"), h.Withdrawal.RetryWithdrawal)
			}

			// === 交易对管理 (需要 market:read/market:write 权限) ===
			markets := authenticated.Group("/markets")
			{
				markets.GET("", middleware.RequirePermission("market:read"), h.Market.List)
				markets.GET("/active", middleware.RequirePermission("market:read"), h.Market.GetAllActive)
				markets.GET("/:id", middleware.RequirePermission("market:read"), h.Market.Get)
				markets.GET("/symbol/:symbol", middleware.RequirePermission("market:read"), h.Market.GetBySymbol)
				markets.POST("", middleware.RequirePermission("market:write"), h.Market.Create)
				markets.PUT("/:id", middleware.RequirePermission("market:write"), h.Market.Update)
				markets.PUT("/:id/status", middleware.RequirePermission("market:write"), h.Market.UpdateStatus)
				markets.DELETE("/:id", middleware.RequirePermission("market:write"), h.Market.Delete)
			}

			// === 风控管理 (需要 risk:read/risk:write 权限) ===
			risk := authenticated.Group("/risk")
			{
				// 风控规则
				risk.GET("/rules", middleware.RequirePermission("risk:read"), h.Risk.ListRiskRules)
				risk.GET("/rules/:rule_id", middleware.RequirePermission("risk:read"), h.Risk.GetRiskRule)
				risk.PUT("/rules", middleware.RequirePermission("risk:write"), h.Risk.UpdateRiskRule)

				// 黑名单
				risk.GET("/blacklist", middleware.RequirePermission("risk:read"), h.Risk.ListBlacklist)
				risk.GET("/blacklist/check/:wallet", middleware.RequirePermission("risk:read"), h.Risk.CheckBlacklist)
				risk.POST("/blacklist", middleware.RequirePermission("risk:write"), h.Risk.AddToBlacklist)
				risk.POST("/blacklist/remove", middleware.RequirePermission("risk:write"), h.Risk.RemoveFromBlacklist)

				// 风险事件
				risk.GET("/events", middleware.RequirePermission("risk:read"), h.Risk.ListRiskEvents)
				risk.GET("/events/:event_id", middleware.RequirePermission("risk:read"), h.Risk.GetRiskEvent)
				risk.POST("/events/acknowledge", middleware.RequirePermission("risk:write"), h.Risk.AcknowledgeRiskEvent)

				// 风控统计
				risk.GET("/stats", middleware.RequirePermission("risk:read"), h.Risk.GetRiskStats)
			}

			// === 统计查询 (需要 stats:read 权限) ===
			stats := authenticated.Group("/stats")
			stats.Use(middleware.RequirePermission("stats:read"))
			{
				stats.GET("/overview", h.Stats.GetOverview)
				stats.GET("/daily", h.Stats.GetDailyStats)
				stats.GET("/daily/list", h.Stats.GetDailyStatsList)
				stats.GET("/trading", h.Stats.GetTradingStats)
				stats.GET("/users", h.Stats.GetUserStats)
				stats.GET("/settlements", h.Stats.GetSettlementStats)
			}

			// === 系统配置管理 (需要 config:read/config:write 权限) ===
			configs := authenticated.Group("/configs")
			{
				configs.GET("", middleware.RequirePermission("config:read"), h.Config.List)
				configs.GET("/version", middleware.RequirePermission("config:read"), h.Config.GetCurrentVersion)
				configs.GET("/versions", middleware.RequirePermission("config:read"), h.Config.GetVersionHistory)
				configs.GET("/category/:category", middleware.RequirePermission("config:read"), h.Config.GetByCategory)
				configs.GET("/key/:key", middleware.RequirePermission("config:read"), h.Config.GetByKey)
				configs.GET("/:id", middleware.RequirePermission("config:read"), h.Config.Get)
				configs.POST("", middleware.RequirePermission("config:write"), h.Config.Create)
				configs.PUT("/batch", middleware.RequirePermission("config:write"), h.Config.BatchUpdate)
				configs.PUT("/:id", middleware.RequirePermission("config:write"), h.Config.Update)
				configs.DELETE("/:id", middleware.RequirePermission("config:write"), h.Config.Delete)
			}

			// === 审计日志查询 (需要 audit:read 权限) ===
			audits := authenticated.Group("/audits")
			audits.Use(middleware.RequirePermission("audit:read"))
			{
				audits.GET("", h.Audit.List)
				audits.GET("/actions", h.Audit.GetActions)
				audits.GET("/resource-types", h.Audit.GetResourceTypes)
				audits.GET("/export", h.Audit.Export)
				audits.GET("/admin/:admin_id", h.Audit.GetByAdmin)
				audits.GET("/resource/:resource_type/:resource_id", h.Audit.GetByResource)
				audits.GET("/:id", h.Audit.Get)
			}
		}
	}
}
