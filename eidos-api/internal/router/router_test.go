package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-api/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
	"github.com/eidos-exchange/eidos/eidos-api/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ratelimit"
	"github.com/eidos-exchange/eidos/eidos-api/internal/ws"
)

func TestNew(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	cfg := &config.Config{}
	logger := zap.NewNop()

	r := New(engine, cfg, logger, nil, nil)

	assert.NotNil(t, r)
	assert.Equal(t, engine, r.engine)
	assert.Equal(t, cfg, r.cfg)
	assert.Equal(t, logger, r.logger)
}

func TestRegisterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	cfg := &config.Config{}
	logger := zap.NewNop()

	r := New(engine, cfg, logger, nil, nil)
	r.RegisterMiddleware()

	// Verify middleware count (5: Recovery, Trace, Logger, CORS, Metrics)
	assert.Len(t, engine.Handlers, 5)
}

func TestRegisterRoutes_HealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled: false,
		},
		WebSocket: config.WebSocketConfig{
			MaxConnections:   1000,
			PingIntervalSec:  30,
			WriteWaitSec:     10,
			PongTimeoutSec:   60,
			MaxMessageSize:   4096,
			MaxSubscriptions: 50,
		},
	}
	logger := zap.NewNop()

	r := New(engine, cfg, logger, nil, nil)
	r.RegisterMiddleware()

	// Create mock handlers
	healthHandler := handler.NewHealthHandler(nil)
	orderHandler := &handler.OrderHandler{}
	balanceHandler := &handler.BalanceHandler{}
	depositHandler := &handler.DepositHandler{}
	withdrawalHandler := &handler.WithdrawalHandler{}
	tradeHandler := &handler.TradeHandler{}
	marketHandler := &handler.MarketHandler{}
	wsHub := ws.NewHub(logger)

	r.RegisterRoutes(
		healthHandler,
		orderHandler,
		balanceHandler,
		depositHandler,
		withdrawalHandler,
		tradeHandler,
		marketHandler,
		wsHub,
	)

	// Test /health/live endpoint
	t.Run("health_live", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Test /health/ready endpoint (initially not ready)
	t.Run("health_ready_not_ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	// Set ready and test again
	t.Run("health_ready_after_set", func(t *testing.T) {
		healthHandler.SetReady(true)
		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Test /metrics endpoint
	t.Run("metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	})
}

func TestRegisterRoutes_WithRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Start miniredis
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	engine := gin.New()
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			PerIP: config.IPRateLimit{
				Total: 100,
			},
			PerWallet: config.WalletRateLimit{
				Queries: 10,
			},
		},
		WebSocket: config.WebSocketConfig{
			MaxConnections:   1000,
			PingIntervalSec:  30,
			WriteWaitSec:     10,
			PongTimeoutSec:   60,
			MaxMessageSize:   4096,
			MaxSubscriptions: 50,
		},
		EIP712: config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 300000,
		},
	}
	logger := zap.NewNop()

	sw := ratelimit.NewSlidingWindow(redisClient)
	rg := cache.NewReplayGuard(redisClient)

	r := New(engine, cfg, logger, sw, rg)
	r.RegisterMiddleware()

	// Create mock handlers
	healthHandler := handler.NewHealthHandler(nil)
	orderHandler := &handler.OrderHandler{}
	balanceHandler := &handler.BalanceHandler{}
	depositHandler := &handler.DepositHandler{}
	withdrawalHandler := &handler.WithdrawalHandler{}
	tradeHandler := &handler.TradeHandler{}
	marketHandler := &handler.MarketHandler{}
	wsHub := ws.NewHub(logger)

	r.RegisterRoutes(
		healthHandler,
		orderHandler,
		balanceHandler,
		depositHandler,
		withdrawalHandler,
		tradeHandler,
		marketHandler,
		wsHub,
	)

	// Test public endpoint (market) without auth
	t.Run("public_endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/markets", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		// Should succeed (200) or return empty data (depends on handler implementation)
		// The important thing is it doesn't require auth
		assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	})

	// Test private endpoint without auth
	t.Run("private_endpoint_no_auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		// Should return 401 Unauthorized
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRegisterRoutes_AllEndpointsExist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	engine := gin.New()
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled: false,
		},
		WebSocket: config.WebSocketConfig{
			MaxConnections:   1000,
			PingIntervalSec:  30,
			WriteWaitSec:     10,
			PongTimeoutSec:   60,
			MaxMessageSize:   4096,
			MaxSubscriptions: 50,
		},
		EIP712: config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 300000,
		},
	}
	logger := zap.NewNop()

	sw := ratelimit.NewSlidingWindow(redisClient)
	rg := cache.NewReplayGuard(redisClient)

	r := New(engine, cfg, logger, sw, rg)
	r.RegisterMiddleware()

	healthHandler := handler.NewHealthHandler(nil)
	orderHandler := &handler.OrderHandler{}
	balanceHandler := &handler.BalanceHandler{}
	depositHandler := &handler.DepositHandler{}
	withdrawalHandler := &handler.WithdrawalHandler{}
	tradeHandler := &handler.TradeHandler{}
	marketHandler := &handler.MarketHandler{}
	wsHub := ws.NewHub(logger)

	r.RegisterRoutes(
		healthHandler,
		orderHandler,
		balanceHandler,
		depositHandler,
		withdrawalHandler,
		tradeHandler,
		marketHandler,
		wsHub,
	)

	// Define expected routes
	expectedRoutes := []struct {
		method string
		path   string
	}{
		// Health endpoints
		{http.MethodGet, "/health/live"},
		{http.MethodGet, "/health/ready"},
		{http.MethodGet, "/metrics"},

		// Public endpoints
		{http.MethodGet, "/api/v1/markets"},
		{http.MethodGet, "/api/v1/ticker/:market"},
		{http.MethodGet, "/api/v1/tickers"},
		{http.MethodGet, "/api/v1/depth/:market"},
		{http.MethodGet, "/api/v1/klines/:market"},
		{http.MethodGet, "/api/v1/trades/:market/recent"},

		// Private endpoints - Orders
		{http.MethodPost, "/api/v1/orders/prepare"},
		{http.MethodPost, "/api/v1/orders"},
		{http.MethodGet, "/api/v1/orders"},
		{http.MethodGet, "/api/v1/orders/open"},
		{http.MethodGet, "/api/v1/orders/:id"},
		{http.MethodDelete, "/api/v1/orders/:id"},
		{http.MethodDelete, "/api/v1/orders"},

		// Private endpoints - Balances
		{http.MethodGet, "/api/v1/balances"},
		{http.MethodGet, "/api/v1/balances/:token"},
		{http.MethodGet, "/api/v1/transactions"},

		// Private endpoints - Deposits
		{http.MethodGet, "/api/v1/deposits"},
		{http.MethodGet, "/api/v1/deposits/:id"},

		// Private endpoints - Withdrawals
		{http.MethodPost, "/api/v1/withdrawals"},
		{http.MethodGet, "/api/v1/withdrawals"},
		{http.MethodGet, "/api/v1/withdrawals/:id"},
		{http.MethodDelete, "/api/v1/withdrawals/:id"},

		// Private endpoints - Trades (user trade history)
		{http.MethodGet, "/api/v1/mytrades"},
		{http.MethodGet, "/api/v1/mytrades/:id"},

		// WebSocket
		{http.MethodGet, "/ws"},
	}

	routes := engine.Routes()
	routeMap := make(map[string]bool)
	for _, r := range routes {
		key := r.Method + ":" + r.Path
		routeMap[key] = true
	}

	for _, expected := range expectedRoutes {
		key := expected.method + ":" + expected.path
		t.Run(key, func(t *testing.T) {
			assert.True(t, routeMap[key], "Route %s %s should be registered", expected.method, expected.path)
		})
	}
}

func TestRouter_PublicEndpointsAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	engine := gin.New()
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled: false,
		},
		WebSocket: config.WebSocketConfig{
			MaxConnections:   1000,
			PingIntervalSec:  30,
			WriteWaitSec:     10,
			PongTimeoutSec:   60,
			MaxMessageSize:   4096,
			MaxSubscriptions: 50,
		},
		EIP712: config.EIP712Config{
			MockMode:             true,
			TimestampToleranceMs: 300000,
		},
	}
	logger := zap.NewNop()

	sw := ratelimit.NewSlidingWindow(redisClient)
	rg := cache.NewReplayGuard(redisClient)

	r := New(engine, cfg, logger, sw, rg)
	r.RegisterMiddleware()

	healthHandler := handler.NewHealthHandler(nil)
	orderHandler := &handler.OrderHandler{}
	balanceHandler := &handler.BalanceHandler{}
	depositHandler := &handler.DepositHandler{}
	withdrawalHandler := &handler.WithdrawalHandler{}
	tradeHandler := &handler.TradeHandler{}
	marketHandler := &handler.MarketHandler{}
	wsHub := ws.NewHub(logger)

	r.RegisterRoutes(
		healthHandler,
		orderHandler,
		balanceHandler,
		depositHandler,
		withdrawalHandler,
		tradeHandler,
		marketHandler,
		wsHub,
	)

	publicPaths := []string{
		"/api/v1/markets",
		"/api/v1/ticker/BTC-USDC",
		"/api/v1/tickers",
		"/api/v1/depth/BTC-USDC",
		"/api/v1/klines/BTC-USDC",
		"/api/v1/trades/BTC-USDC/recent",
	}

	for _, path := range publicPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			// Public endpoints should not return 401
			require.NotEqual(t, http.StatusUnauthorized, w.Code, "Public endpoint %s should not require auth", path)
		})
	}
}
