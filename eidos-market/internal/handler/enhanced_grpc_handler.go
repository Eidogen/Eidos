// Package handler provides gRPC handlers.
package handler

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// EnhancedMarketServiceInterface enhanced service interface with cache support
type EnhancedMarketServiceInterface interface {
	MarketServiceInterface
	GetTickerWithCache(ctx context.Context, market string) (*model.Ticker, error)
	GetDepthWithCache(ctx context.Context, market string, limit int) (*model.Depth, error)
	GetKlinesWithCache(ctx context.Context, market string, interval model.KlineInterval, startTime, endTime int64, limit int) ([]*model.Kline, error)
}

// EnhancedMarketHandler enhanced gRPC handler with caching
type EnhancedMarketHandler struct {
	marketv1.UnimplementedMarketServiceServer
	svc    EnhancedMarketServiceInterface
	logger *zap.Logger
}

// NewEnhancedMarketHandler creates an enhanced gRPC handler
func NewEnhancedMarketHandler(svc *service.EnhancedMarketService, logger *zap.Logger) *EnhancedMarketHandler {
	return &EnhancedMarketHandler{
		svc:    svc,
		logger: logger.Named("grpc_handler"),
	}
}

// ListMarkets returns all markets
func (h *EnhancedMarketHandler) ListMarkets(ctx context.Context, req *marketv1.ListMarketsRequest) (*marketv1.ListMarketsResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("ListMarkets").Observe(time.Since(startTime).Seconds())
	}()

	markets, err := h.svc.GetMarkets(ctx)
	if err != nil {
		metrics.GRPCRequestsTotal.WithLabelValues("ListMarkets", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get markets: %v", err)
	}

	resp := &marketv1.ListMarketsResponse{
		Markets: make([]*marketv1.Market, len(markets)),
	}
	for i, m := range markets {
		resp.Markets[i] = toProtoMarket(m)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("ListMarkets", "ok").Inc()
	return resp, nil
}

// GetTicker returns 24h ticker with caching
func (h *EnhancedMarketHandler) GetTicker(ctx context.Context, req *marketv1.GetTickerRequest) (*marketv1.GetTickerResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("GetTicker").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetTicker", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	ticker, err := h.svc.GetTickerWithCache(ctx, req.Market)
	if err != nil {
		if err == service.ErrMarketNotFound {
			metrics.GRPCRequestsTotal.WithLabelValues("GetTicker", "not_found").Inc()
			return nil, status.Error(codes.NotFound, "market not found")
		}
		metrics.GRPCRequestsTotal.WithLabelValues("GetTicker", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get ticker: %v", err)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("GetTicker", "ok").Inc()
	return &marketv1.GetTickerResponse{
		Ticker: toProtoTicker(ticker),
	}, nil
}

// ListTickers returns all tickers
func (h *EnhancedMarketHandler) ListTickers(ctx context.Context, req *marketv1.ListTickersRequest) (*marketv1.ListTickersResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("ListTickers").Observe(time.Since(startTime).Seconds())
	}()

	tickers, err := h.svc.GetAllTickers(ctx)
	if err != nil {
		metrics.GRPCRequestsTotal.WithLabelValues("ListTickers", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get tickers: %v", err)
	}

	resp := &marketv1.ListTickersResponse{
		Tickers: make([]*marketv1.Ticker, len(tickers)),
	}
	for i, t := range tickers {
		resp.Tickers[i] = toProtoTicker(t)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("ListTickers", "ok").Inc()
	return resp, nil
}

// GetKlines returns klines with caching
func (h *EnhancedMarketHandler) GetKlines(ctx context.Context, req *marketv1.GetKlinesRequest) (*marketv1.GetKlinesResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("GetKlines").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetKlines", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}
	if req.Interval == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetKlines", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "interval is required")
	}
	if !model.IsValidInterval(req.Interval) {
		metrics.GRPCRequestsTotal.WithLabelValues("GetKlines", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "invalid interval")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 500
	}
	if limit > 1500 {
		limit = 1500
	}

	klines, err := h.svc.GetKlinesWithCache(ctx, req.Market, model.KlineInterval(req.Interval),
		req.StartTime, req.EndTime, limit)
	if err != nil {
		metrics.GRPCRequestsTotal.WithLabelValues("GetKlines", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get klines: %v", err)
	}

	resp := &marketv1.GetKlinesResponse{
		Klines: make([]*marketv1.Kline, len(klines)),
	}
	for i, k := range klines {
		resp.Klines[i] = toProtoKline(k)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("GetKlines", "ok").Inc()
	return resp, nil
}

// GetRecentTrades returns recent trades
func (h *EnhancedMarketHandler) GetRecentTrades(ctx context.Context, req *marketv1.GetRecentTradesRequest) (*marketv1.GetRecentTradesResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("GetRecentTrades").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetRecentTrades", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	trades, err := h.svc.GetRecentTrades(ctx, req.Market, limit)
	if err != nil {
		metrics.GRPCRequestsTotal.WithLabelValues("GetRecentTrades", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get trades: %v", err)
	}

	resp := &marketv1.GetRecentTradesResponse{
		Trades: make([]*marketv1.RecentTrade, len(trades)),
	}
	for i, t := range trades {
		resp.Trades[i] = toProtoTrade(t)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("GetRecentTrades", "ok").Inc()
	return resp, nil
}

// GetDepth returns orderbook depth with caching
func (h *EnhancedMarketHandler) GetDepth(ctx context.Context, req *marketv1.GetDepthRequest) (*marketv1.GetDepthResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("GetDepth").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetDepth", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	limit := int(req.Limit)
	if !model.IsValidDepthLimit(limit) {
		limit = model.NormalizeDepthLimit(limit)
	}

	depth, err := h.svc.GetDepthWithCache(ctx, req.Market, limit)
	if err != nil {
		if err == service.ErrMarketNotFound {
			metrics.GRPCRequestsTotal.WithLabelValues("GetDepth", "not_found").Inc()
			return nil, status.Error(codes.NotFound, "market not found")
		}
		metrics.GRPCRequestsTotal.WithLabelValues("GetDepth", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get depth: %v", err)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("GetDepth", "ok").Inc()
	return &marketv1.GetDepthResponse{
		Market:    depth.Market,
		Bids:      toProtoPriceLevels(depth.Bids),
		Asks:      toProtoPriceLevels(depth.Asks),
		Sequence:  depth.Sequence,
		Timestamp: depth.Timestamp,
	}, nil
}

// GetTradeHistory returns historical trades
func (h *EnhancedMarketHandler) GetTradeHistory(ctx context.Context, req *marketv1.GetTradeHistoryRequest) (*marketv1.GetTradeHistoryResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("GetTradeHistory").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("GetTradeHistory", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	trades, err := h.svc.GetRecentTrades(ctx, req.Market, limit)
	if err != nil {
		metrics.GRPCRequestsTotal.WithLabelValues("GetTradeHistory", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get trades: %v", err)
	}

	resp := &marketv1.GetTradeHistoryResponse{
		Trades: make([]*marketv1.RecentTrade, len(trades)),
	}
	for i, t := range trades {
		resp.Trades[i] = toProtoTrade(t)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("GetTradeHistory", "ok").Inc()
	return resp, nil
}

// Get24HStats returns 24h statistics for a market
func (h *EnhancedMarketHandler) Get24HStats(ctx context.Context, req *marketv1.Get24HStatsRequest) (*marketv1.Get24HStatsResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.GRPCRequestDuration.WithLabelValues("Get24hStats").Observe(time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.GRPCRequestsTotal.WithLabelValues("Get24hStats", "invalid").Inc()
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	ticker, err := h.svc.GetTickerWithCache(ctx, req.Market)
	if err != nil {
		if err == service.ErrMarketNotFound {
			metrics.GRPCRequestsTotal.WithLabelValues("Get24hStats", "not_found").Inc()
			return nil, status.Error(codes.NotFound, "market not found")
		}
		metrics.GRPCRequestsTotal.WithLabelValues("Get24hStats", "error").Inc()
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	metrics.GRPCRequestsTotal.WithLabelValues("Get24HStats", "ok").Inc()
	return &marketv1.Get24HStatsResponse{
		Market:             req.Market,
		PriceChange:        ticker.PriceChange.String(),
		PriceChangePercent: ticker.PriceChangePercent.String(),
		High:               ticker.High.String(),
		Low:                ticker.Low.String(),
		Volume:             ticker.Volume.String(),
		QuoteVolume:        ticker.QuoteVolume.String(),
		Open:               ticker.Open.String(),
		LastPrice:          ticker.LastPrice.String(),
		TradeCount:         ticker.TradeCount,
		Timestamp:          ticker.Timestamp,
	}, nil
}
