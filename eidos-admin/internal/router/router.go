package router

import (
	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/middleware"
)

// Handlers 所有处理器
type Handlers struct {
	Auth   *handler.AuthHandler
	Admin  *handler.AdminHandler
	Market *handler.MarketHandler
	Stats  *handler.StatsHandler
	Config *handler.ConfigHandler
	Audit  *handler.AuditHandler
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

			// 管理员管理 (需要 admin:read/admin:write 权限)
			admins := authenticated.Group("/admins")
			{
				admins.GET("", middleware.RequirePermission("admin:read"), h.Admin.List)
				admins.GET("/:id", middleware.RequirePermission("admin:read"), h.Admin.Get)
				admins.POST("", middleware.RequirePermission("admin:write"), h.Admin.Create)
				admins.PUT("/:id", middleware.RequirePermission("admin:write"), h.Admin.Update)
				admins.DELETE("/:id", middleware.RequirePermission("admin:write"), h.Admin.Delete)
				admins.PUT("/:id/password", middleware.RequirePermission("admin:write"), h.Admin.ResetPassword)
			}

			// 市场配置管理 (需要 market:read/market:write 权限)
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

			// 统计查询 (需要 stats:read 权限)
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

			// 系统配置管理 (需要 config:read/config:write 权限)
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

			// 审计日志查询 (需要 audit:read 权限)
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
