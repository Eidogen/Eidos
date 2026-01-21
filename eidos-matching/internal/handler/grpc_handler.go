// Package handler gRPC 服务处理器
package handler

import (
	"context"
	"time"

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
