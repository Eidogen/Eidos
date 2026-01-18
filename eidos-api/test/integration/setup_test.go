//go:build integration

// Package integration 提供 API 集成测试
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	"github.com/eidos-exchange/eidos/eidos-api/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-api/internal/middleware"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/eidos-exchange/eidos/eidos-api/test/mock"
)

// IntegrationSuite 集成测试套件
type IntegrationSuite struct {
	suite.Suite
	router            *gin.Engine
	mr                *miniredis.Miniredis
	rdb               *redis.Client
	orderService      *mock.OrderService
	balanceService    *mock.BalanceService
	depositService    *mock.DepositService
	withdrawalService *mock.WithdrawalService
	tradeService      *mock.TradeService
	marketService     *mock.MarketService
	healthHandler     *handler.HealthHandler
}

// SetupSuite 初始化测试套件
func (s *IntegrationSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	// 创建 miniredis
	s.mr = miniredis.RunT(s.T())
	s.rdb = redis.NewClient(&redis.Options{
		Addr: s.mr.Addr(),
	})

	// 创建 mock services
	s.orderService = new(mock.OrderService)
	s.balanceService = new(mock.BalanceService)
	s.depositService = new(mock.DepositService)
	s.withdrawalService = new(mock.WithdrawalService)
	s.tradeService = new(mock.TradeService)
	s.marketService = new(mock.MarketService)

	// 创建 handlers
	orderHandler := handler.NewOrderHandler(s.orderService)
	balanceHandler := handler.NewBalanceHandler(s.balanceService)
	depositHandler := handler.NewDepositHandler(s.depositService)
	withdrawalHandler := handler.NewWithdrawalHandler(s.withdrawalService)
	tradeHandler := handler.NewTradeHandler(s.tradeService)
	marketHandler := handler.NewMarketHandler(s.marketService)
	s.healthHandler = handler.NewHealthHandler(&handler.HealthDeps{
		RedisClient: &redisHealthChecker{rdb: s.rdb},
	})

	// 创建限流器
	rateLimiter := ratelimit.NewSlidingWindow(s.rdb)

	// 创建重放保护
	replayGuard := cache.NewReplayGuard(s.rdb)

	// 创建 router
	s.router = gin.New()
	s.router.Use(gin.Recovery())
	s.router.Use(middleware.Trace())
	s.router.Use(middleware.Logger())

	// 健康检查（无需认证）
	s.router.GET("/health/live", s.healthHandler.Live)
	s.router.GET("/health/ready", s.healthHandler.Ready)

	// API 路由组
	api := s.router.Group("/api/v1")

	// 公开接口
	api.GET("/markets", marketHandler.ListMarkets)
	api.GET("/ticker/:market", marketHandler.GetTicker)
	api.GET("/tickers", marketHandler.ListTickers)
	api.GET("/depth/:market", marketHandler.GetDepth)
	api.GET("/klines/:market", marketHandler.GetKlines)
	api.GET("/trades/:market/recent", marketHandler.GetRecentTrades)

	// 认证接口
	authed := api.Group("")
	authed.Use(middleware.RateLimitByIP(rateLimiter, 100, time.Minute))
	// 使用 Mock 模式的认证中间件
	authed.Use(createMockAuthMiddleware(replayGuard))

	// 订单
	authed.POST("/orders/prepare", orderHandler.PrepareOrder)
	authed.POST("/orders", orderHandler.CreateOrder)
	authed.GET("/orders", orderHandler.ListOrders)
	authed.GET("/orders/open", orderHandler.ListOpenOrders)
	authed.GET("/orders/:id", orderHandler.GetOrder)
	authed.DELETE("/orders/:id", orderHandler.CancelOrder)
	authed.DELETE("/orders", orderHandler.BatchCancelOrders)

	// 余额
	authed.GET("/balances", balanceHandler.ListBalances)
	authed.GET("/balances/:token", balanceHandler.GetBalance)
	authed.GET("/transactions", balanceHandler.ListTransactions)

	// 充值
	authed.GET("/deposits", depositHandler.ListDeposits)
	authed.GET("/deposits/:id", depositHandler.GetDeposit)

	// 提现
	authed.POST("/withdrawals", withdrawalHandler.CreateWithdrawal)
	authed.GET("/withdrawals", withdrawalHandler.ListWithdrawals)
	authed.GET("/withdrawals/:id", withdrawalHandler.GetWithdrawal)
	authed.DELETE("/withdrawals/:id", withdrawalHandler.CancelWithdrawal)

	// 成交
	authed.GET("/user/trades", tradeHandler.ListTrades)
	authed.GET("/user/trades/:id", tradeHandler.GetTrade)
}

// redisHealthChecker 实现 HealthDeps 需要的 Ping 接口
type redisHealthChecker struct {
	rdb *redis.Client
}

func (r *redisHealthChecker) Ping() error {
	return r.rdb.Ping(context.Background()).Err()
}

// TearDownSuite 清理测试套件
func (s *IntegrationSuite) TearDownSuite() {
	if s.rdb != nil {
		s.rdb.Close()
	}
	if s.mr != nil {
		s.mr.Close()
	}
}

// SetupTest 每个测试前重置 mock
func (s *IntegrationSuite) SetupTest() {
	s.orderService.ExpectedCalls = nil
	s.orderService.Calls = nil
	s.balanceService.ExpectedCalls = nil
	s.balanceService.Calls = nil
	s.depositService.ExpectedCalls = nil
	s.depositService.Calls = nil
	s.withdrawalService.ExpectedCalls = nil
	s.withdrawalService.Calls = nil
	s.tradeService.ExpectedCalls = nil
	s.tradeService.Calls = nil
	s.marketService.ExpectedCalls = nil
	s.marketService.Calls = nil

	// 重置 miniredis
	s.mr.FlushAll()
}

// Request 发送 HTTP 请求的辅助方法
func (s *IntegrationSuite) Request(method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		jsonData, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonData)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// AuthHeaders 返回认证头
func (s *IntegrationSuite) AuthHeaders(wallet string) map[string]string {
	timestamp := time.Now().UnixMilli()
	return map[string]string{
		"Authorization": "EIP712 " + wallet + ":" + string(rune(timestamp)) + ":0xmocksignature",
	}
}

// ParseResponse 解析响应
func (s *IntegrationSuite) ParseResponse(w *httptest.ResponseRecorder) *dto.Response {
	var resp dto.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	s.NoError(err)
	return &resp
}

// createMockAuthMiddleware 创建 Mock 模式的认证中间件
// 在集成测试中，我们跳过实际签名验证，只设置 wallet
func createMockAuthMiddleware(replayGuard *cache.ReplayGuard) gin.HandlerFunc {
	_ = replayGuard // 保留参数以保持一致的签名
	return func(c *gin.Context) {
		// 从 Authorization header 提取 wallet
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, dto.Response{
				Code:    dto.ErrUnauthorized.Code,
				Message: dto.ErrUnauthorized.Message,
			})
			c.Abort()
			return
		}

		// 解析 "EIP712 wallet:timestamp:signature" 格式
		if len(auth) > 7 && auth[:7] == "EIP712 " {
			parts := bytes.Split([]byte(auth[7:]), []byte(":"))
			if len(parts) >= 1 {
				wallet := string(parts[0])
				c.Set("wallet", wallet)
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, dto.Response{
			Code:    dto.ErrUnauthorized.Code,
			Message: dto.ErrUnauthorized.Message,
		})
		c.Abort()
	}
}

// TestIntegrationSuite 运行集成测试套件
func TestIntegrationSuite(t *testing.T) {
	suite.Run(t, new(IntegrationSuite))
}
