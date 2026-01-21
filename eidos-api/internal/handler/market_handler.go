package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// MarketService 行情服务接口
// 通过 gRPC 调用 eidos-market 服务，实现见 eidos-api/internal/client/market_client.go
type MarketService interface {
	ListMarkets(ctx *gin.Context) ([]*dto.MarketResponse, error)
	GetTicker(ctx *gin.Context, market string) (*dto.TickerResponse, error)
	ListTickers(ctx *gin.Context) ([]*dto.TickerResponse, error)
	GetDepth(ctx *gin.Context, market string, limit int) (*dto.DepthResponse, error)
	GetKlines(ctx *gin.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error)
	GetRecentTrades(ctx *gin.Context, market string, limit int) ([]*dto.RecentTradeResponse, error)
}

// MarketHandler 行情处理器
type MarketHandler struct {
	svc MarketService
}

// NewMarketHandler 创建行情处理器
func NewMarketHandler(svc MarketService) *MarketHandler {
	return &MarketHandler{svc: svc}
}

// ListMarkets 获取交易对列表
// GET /api/v1/markets
func (h *MarketHandler) ListMarkets(c *gin.Context) {
	if h.svc == nil {
		// 返回静态 Mock 数据
		Success(c, getMockMarkets())
		return
	}

	markets, err := h.svc.ListMarkets(c)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, markets)
}

// GetTicker 获取单个 Ticker
// GET /api/v1/ticker/:market
func (h *MarketHandler) GetTicker(c *gin.Context) {
	market := c.Param("market")
	if market == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("market is required"))
		return
	}

	if h.svc == nil {
		// 返回静态 Mock 数据
		Success(c, getMockTicker(market))
		return
	}

	ticker, err := h.svc.GetTicker(c, market)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, ticker)
}

// ListTickers 获取所有 Ticker
// GET /api/v1/tickers
func (h *MarketHandler) ListTickers(c *gin.Context) {
	if h.svc == nil {
		// 返回静态 Mock 数据
		Success(c, getMockTickers())
		return
	}

	tickers, err := h.svc.ListTickers(c)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, tickers)
}

// GetDepth 获取深度
// GET /api/v1/depth/:market
func (h *MarketHandler) GetDepth(c *gin.Context) {
	market := c.Param("market")
	if market == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("market is required"))
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if h.svc == nil {
		// 返回空深度
		Success(c, &dto.DepthResponse{
			Market:    market,
			Bids:      [][]string{},
			Asks:      [][]string{},
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	depth, err := h.svc.GetDepth(c, market, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, depth)
}

// GetKlines 获取 K 线
// GET /api/v1/klines/:market
func (h *MarketHandler) GetKlines(c *gin.Context) {
	market := c.Param("market")
	if market == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("market is required"))
		return
	}

	interval := c.DefaultQuery("interval", "1m")
	limit := 100

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	var startTime, endTime int64
	if st := c.Query("start_time"); st != "" {
		if parsed, err := strconv.ParseInt(st, 10, 64); err == nil {
			startTime = parsed
		}
	}
	if et := c.Query("end_time"); et != "" {
		if parsed, err := strconv.ParseInt(et, 10, 64); err == nil {
			endTime = parsed
		}
	}

	if h.svc == nil {
		// 返回空数据
		Success(c, []*dto.KlineResponse{})
		return
	}

	klines, err := h.svc.GetKlines(c, market, interval, startTime, endTime, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, klines)
}

// GetRecentTrades 获取最近成交
// GET /api/v1/trades/:market/recent
func (h *MarketHandler) GetRecentTrades(c *gin.Context) {
	market := c.Param("market")
	if market == "" {
		Error(c, dto.ErrInvalidParams.WithMessage("market is required"))
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if h.svc == nil {
		// 返回空数据
		Success(c, []*dto.RecentTradeResponse{})
		return
	}

	trades, err := h.svc.GetRecentTrades(c, market, limit)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	Success(c, trades)
}

// ========== Mock 数据 ==========

func getMockMarkets() []*dto.MarketResponse {
	return []*dto.MarketResponse{
		{
			Symbol:         "BTC-USDC",
			BaseToken:      "BTC",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 6,
			MinAmount:      "0.0001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
		{
			Symbol:         "ETH-USDC",
			BaseToken:      "ETH",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 5,
			MinAmount:      "0.001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
	}
}

func getMockTicker(market string) *dto.TickerResponse {
	return &dto.TickerResponse{
		Market:             market,
		LastPrice:          "0",
		PriceChange:        "0",
		PriceChangePercent: "0",
		High:               "0",
		Low:                "0",
		Volume:             "0",
		QuoteVolume:        "0",
		Open:               "0",
		BestBid:            "0",
		BestAsk:            "0",
		Timestamp:          time.Now().UnixMilli(),
	}
}

func getMockTickers() []*dto.TickerResponse {
	markets := []string{"BTC-USDC", "ETH-USDC"}
	tickers := make([]*dto.TickerResponse, len(markets))
	for i, m := range markets {
		tickers[i] = getMockTicker(m)
	}
	return tickers
}
