// Package handler gRPC 服务处理器
package handler

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/engine"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MatchingHandler gRPC 处理器
type MatchingHandler struct {
	matchingv1.UnimplementedMatchingServiceServer
	engineManager *engine.EngineManager
}

// NewMatchingHandler 创建处理器
func NewMatchingHandler(engineManager *engine.EngineManager) *MatchingHandler {
	return &MatchingHandler{
		engineManager: engineManager,
	}
}

// GetOrderbook 获取订单簿快照
func (h *MatchingHandler) GetOrderbook(ctx context.Context, req *matchingv1.GetOrderbookRequest) (*matchingv1.GetOrderbookResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("GetOrderbook", "OK", time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.RecordGRPCRequest("GetOrderbook", "InvalidArgument", time.Since(startTime).Seconds())
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	eng, err := h.engineManager.GetEngine(req.Market)
	if err != nil {
		metrics.RecordGRPCRequest("GetOrderbook", "NotFound", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.NotFound, "market not found: %s", req.Market)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	depth, err := eng.GetDepth(limit)
	if err != nil {
		metrics.RecordGRPCRequest("GetOrderbook", "Internal", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.Internal, "get depth failed: %v", err)
	}

	resp := &matchingv1.GetOrderbookResponse{
		Market:    req.Market,
		Bids:      make([]*matchingv1.PriceLevel, 0, len(depth.Bids)),
		Asks:      make([]*matchingv1.PriceLevel, 0, len(depth.Asks)),
		Timestamp: time.Now().UnixMilli(),
		Sequence:  uint64(depth.Sequence),
	}

	for _, bid := range depth.Bids {
		resp.Bids = append(resp.Bids, &matchingv1.PriceLevel{
			Price:      bid.Price.String(),
			Amount:     bid.Size.String(),
			OrderCount: int32(bid.Count),
		})
	}

	for _, ask := range depth.Asks {
		resp.Asks = append(resp.Asks, &matchingv1.PriceLevel{
			Price:      ask.Price.String(),
			Amount:     ask.Size.String(),
			OrderCount: int32(ask.Count),
		})
	}

	return resp, nil
}

// GetDepth 获取订单簿深度
func (h *MatchingHandler) GetDepth(ctx context.Context, req *matchingv1.GetDepthRequest) (*matchingv1.GetDepthResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("GetDepth", "OK", time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.RecordGRPCRequest("GetDepth", "InvalidArgument", time.Since(startTime).Seconds())
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	eng, err := h.engineManager.GetEngine(req.Market)
	if err != nil {
		metrics.RecordGRPCRequest("GetDepth", "NotFound", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.NotFound, "market not found: %s", req.Market)
	}

	// 标准化深度档位
	level := int(req.Level)
	validLevels := []int{5, 10, 20, 50, 100}
	if level <= 0 {
		level = 20
	} else {
		found := false
		for _, v := range validLevels {
			if level <= v {
				level = v
				found = true
				break
			}
		}
		if !found {
			level = 100
		}
	}

	depth, err := eng.GetDepth(level)
	if err != nil {
		metrics.RecordGRPCRequest("GetDepth", "Internal", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.Internal, "get depth failed: %v", err)
	}

	resp := &matchingv1.GetDepthResponse{
		Market:    req.Market,
		Bids:      make([]*matchingv1.PriceLevel, 0, len(depth.Bids)),
		Asks:      make([]*matchingv1.PriceLevel, 0, len(depth.Asks)),
		Timestamp: time.Now().UnixMilli(),
	}

	for _, bid := range depth.Bids {
		resp.Bids = append(resp.Bids, &matchingv1.PriceLevel{
			Price:      bid.Price.String(),
			Amount:     bid.Size.String(),
			OrderCount: int32(bid.Count),
		})
	}

	for _, ask := range depth.Asks {
		resp.Asks = append(resp.Asks, &matchingv1.PriceLevel{
			Price:      ask.Price.String(),
			Amount:     ask.Size.String(),
			OrderCount: int32(ask.Count),
		})
	}

	return resp, nil
}

// HealthCheck 健康检查
func (h *MatchingHandler) HealthCheck(ctx context.Context, req *matchingv1.HealthCheckRequest) (*matchingv1.HealthCheckResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("HealthCheck", "OK", time.Since(startTime).Seconds())
	}()

	markets := h.engineManager.GetMarkets()
	marketStatuses := make(map[string]*matchingv1.MarketHealth, len(markets))

	for _, market := range markets {
		eng, err := h.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		stats := eng.GetStats()
		obStats := stats.OrderBookStats

		marketStatuses[market] = &matchingv1.MarketHealth{
			Market:        market,
			Active:        eng.IsRunning(),
			BidCount:      int64(obStats.BidLevels),
			AskCount:      int64(obStats.AskLevels),
			BestBid:       obStats.BestBid.String(),
			BestAsk:       obStats.BestAsk.String(),
			LastMatchTime: obStats.LastTradeAt,
		}
	}

	return &matchingv1.HealthCheckResponse{
		Healthy: true,
		Markets: marketStatuses,
	}, nil
}

// GetBestPrices 获取最佳买卖价格
func (h *MatchingHandler) GetBestPrices(ctx context.Context, req *matchingv1.GetBestPricesRequest) (*matchingv1.GetBestPricesResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("GetBestPrices", "OK", time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.RecordGRPCRequest("GetBestPrices", "InvalidArgument", time.Since(startTime).Seconds())
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	eng, err := h.engineManager.GetEngine(req.Market)
	if err != nil {
		metrics.RecordGRPCRequest("GetBestPrices", "NotFound", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.NotFound, "market not found: %s", req.Market)
	}

	depth, err := eng.GetDepth(1)
	if err != nil {
		metrics.RecordGRPCRequest("GetBestPrices", "Internal", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.Internal, "get depth failed: %v", err)
	}

	resp := &matchingv1.GetBestPricesResponse{
		Market: req.Market,
	}

	if len(depth.Bids) > 0 {
		resp.BestBid = depth.Bids[0].Price.String()
		resp.BestBidQty = depth.Bids[0].Size.String()
	}

	if len(depth.Asks) > 0 {
		resp.BestAsk = depth.Asks[0].Price.String()
		resp.BestAskQty = depth.Asks[0].Size.String()
	}

	// 计算价差
	if len(depth.Bids) > 0 && len(depth.Asks) > 0 {
		spread := depth.Asks[0].Price.Sub(depth.Bids[0].Price)
		resp.Spread = spread.String()

		midPrice := depth.Bids[0].Price.Add(depth.Asks[0].Price).Div(decimal.NewFromInt(2))
		if !midPrice.IsZero() {
			spreadPercent := spread.Div(midPrice).Mul(decimal.NewFromInt(100))
			resp.SpreadPercent = spreadPercent.String()
		}
	}

	return resp, nil
}

// ListMarkets 列出所有市场
func (h *MatchingHandler) ListMarkets(ctx context.Context, req *matchingv1.ListMarketsRequest) (*matchingv1.ListMarketsResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("ListMarkets", "OK", time.Since(startTime).Seconds())
	}()

	markets := h.engineManager.GetMarkets()
	summaries := make([]*matchingv1.MarketSummary, 0, len(markets))

	for _, market := range markets {
		eng, err := h.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		stats := eng.GetStats()
		obStats := stats.OrderBookStats

		summaries = append(summaries, &matchingv1.MarketSummary{
			Market:    market,
			IsActive:  eng.IsRunning(),
			BestBid:   obStats.BestBid.String(),
			BestAsk:   obStats.BestAsk.String(),
			LastPrice: obStats.LastPrice.String(),
			UpdatedAt: time.Now().UnixMilli(),
		})
	}

	return &matchingv1.ListMarketsResponse{
		Markets: summaries,
	}, nil
}

// GetMarketState 获取市场状态
func (h *MatchingHandler) GetMarketState(ctx context.Context, req *matchingv1.GetMarketStateRequest) (*matchingv1.GetMarketStateResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("GetMarketState", "OK", time.Since(startTime).Seconds())
	}()

	if req.Market == "" {
		metrics.RecordGRPCRequest("GetMarketState", "InvalidArgument", time.Since(startTime).Seconds())
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	eng, err := h.engineManager.GetEngine(req.Market)
	if err != nil {
		metrics.RecordGRPCRequest("GetMarketState", "NotFound", time.Since(startTime).Seconds())
		return nil, status.Errorf(codes.NotFound, "market not found: %s", req.Market)
	}

	stats := eng.GetStats()
	obStats := stats.OrderBookStats

	return &matchingv1.GetMarketStateResponse{
		Market:        req.Market,
		IsActive:      eng.IsRunning(),
		BidOrderCount: int64(obStats.BidLevels),
		AskOrderCount: int64(obStats.AskLevels),
		BestBid:       obStats.BestBid.String(),
		BestAsk:       obStats.BestAsk.String(),
		LastPrice:     obStats.LastPrice.String(),
		LastTradeTime: obStats.LastTradeAt,
		LastSequence:  uint64(stats.OutputSequence),
	}, nil
}

// GetMetrics 获取性能指标
func (h *MatchingHandler) GetMetrics(ctx context.Context, req *matchingv1.GetMetricsRequest) (*matchingv1.GetMetricsResponse, error) {
	startTime := time.Now()
	defer func() {
		metrics.RecordGRPCRequest("GetMetrics", "OK", time.Since(startTime).Seconds())
	}()

	markets := h.engineManager.GetMarkets()
	marketMetrics := make(map[string]*matchingv1.MarketMetrics, len(markets))

	var totalOrders, totalTrades int64

	for _, market := range markets {
		eng, err := h.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		stats := eng.GetStats()
		totalOrders += stats.OrdersProcessed
		totalTrades += stats.TradesGenerated

		marketMetrics[market] = &matchingv1.MarketMetrics{
			Market:          market,
			OrdersReceived:  stats.OrdersProcessed,
			OrdersMatched:   stats.TradesGenerated,
			OrdersCancelled: stats.CancelProcessed,
			TradesExecuted:  stats.TradesGenerated,
			AvgLatencyUs:    stats.AvgLatencyUs,
			P99LatencyUs:    0, // TODO: 添加 P99 延迟统计
			OrdersPerSecond: 0, // TODO: 添加 TPS 统计
		}
	}

	return &matchingv1.GetMetricsResponse{
		Markets: marketMetrics,
		Global: &matchingv1.GlobalMetrics{
			TotalOrders:  totalOrders,
			TotalTrades:  totalTrades,
			AvgLatencyUs: 0, // TODO: 计算全局平均延迟
			P99LatencyUs: 0, // TODO: 计算全局 P99 延迟
			StartTime:    time.Now().Add(-time.Hour).UnixMilli(), // 示例值
		},
	}, nil
}
