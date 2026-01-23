package handler

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// MarketServiceInterface 行情服务接口 (用于依赖注入和测试)
type MarketServiceInterface interface {
	GetMarkets(ctx context.Context) ([]*model.Market, error)
	GetTicker(ctx context.Context, market string) (*model.Ticker, error)
	GetAllTickers(ctx context.Context) ([]*model.Ticker, error)
	GetKlines(ctx context.Context, market string, interval model.KlineInterval, startTime, endTime int64, limit int) ([]*model.Kline, error)
	GetDepth(ctx context.Context, market string, limit int) (*model.Depth, error)
	GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error)
	GetTradeHistory(ctx context.Context, market string, startTime, endTime int64, limit int, fromID string) ([]*model.Trade, error)
}

// MarketHandler gRPC 处理器
type MarketHandler struct {
	marketv1.UnimplementedMarketServiceServer
	svc MarketServiceInterface
}

// NewMarketHandler 创建 gRPC 处理器
func NewMarketHandler(svc *service.MarketService) *MarketHandler {
	return &MarketHandler{svc: svc}
}

// ListMarkets 获取市场列表
func (h *MarketHandler) ListMarkets(ctx context.Context, req *marketv1.ListMarketsRequest) (*marketv1.ListMarketsResponse, error) {
	markets, err := h.svc.GetMarkets(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get markets: %v", err)
	}

	resp := &marketv1.ListMarketsResponse{
		Markets: make([]*marketv1.Market, len(markets)),
	}
	for i, m := range markets {
		resp.Markets[i] = toProtoMarket(m)
	}

	return resp, nil
}

// GetTicker 获取 24h Ticker
func (h *MarketHandler) GetTicker(ctx context.Context, req *marketv1.GetTickerRequest) (*marketv1.GetTickerResponse, error) {
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	ticker, err := h.svc.GetTicker(ctx, req.Market)
	if err != nil {
		if err == service.ErrMarketNotFound {
			return nil, status.Error(codes.NotFound, "market not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get ticker: %v", err)
	}

	return &marketv1.GetTickerResponse{
		Ticker: toProtoTicker(ticker),
	}, nil
}

// ListTickers 获取所有 Ticker
func (h *MarketHandler) ListTickers(ctx context.Context, req *marketv1.ListTickersRequest) (*marketv1.ListTickersResponse, error) {
	tickers, err := h.svc.GetAllTickers(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get tickers: %v", err)
	}

	resp := &marketv1.ListTickersResponse{
		Tickers: make([]*marketv1.Ticker, len(tickers)),
	}
	for i, t := range tickers {
		resp.Tickers[i] = toProtoTicker(t)
	}

	return resp, nil
}

// GetKlines 获取 K 线数据
func (h *MarketHandler) GetKlines(ctx context.Context, req *marketv1.GetKlinesRequest) (*marketv1.GetKlinesResponse, error) {
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}
	if req.Interval == "" {
		return nil, status.Error(codes.InvalidArgument, "interval is required")
	}
	if !model.IsValidInterval(req.Interval) {
		return nil, status.Error(codes.InvalidArgument, "invalid interval")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 500
	}
	if limit > 1500 {
		limit = 1500
	}

	klines, err := h.svc.GetKlines(ctx, req.Market, model.KlineInterval(req.Interval),
		req.StartTime, req.EndTime, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get klines: %v", err)
	}

	resp := &marketv1.GetKlinesResponse{
		Klines: make([]*marketv1.Kline, len(klines)),
	}
	for i, k := range klines {
		resp.Klines[i] = toProtoKline(k)
	}

	return resp, nil
}

// GetRecentTrades 获取最新成交
func (h *MarketHandler) GetRecentTrades(ctx context.Context, req *marketv1.GetRecentTradesRequest) (*marketv1.GetRecentTradesResponse, error) {
	if req.Market == "" {
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
		return nil, status.Errorf(codes.Internal, "failed to get trades: %v", err)
	}

	resp := &marketv1.GetRecentTradesResponse{
		Trades: make([]*marketv1.RecentTrade, len(trades)),
	}
	for i, t := range trades {
		resp.Trades[i] = toProtoTrade(t)
	}

	return resp, nil
}

// GetDepth 获取深度
func (h *MarketHandler) GetDepth(ctx context.Context, req *marketv1.GetDepthRequest) (*marketv1.GetDepthResponse, error) {
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	limit := int(req.Limit)
	if !model.IsValidDepthLimit(limit) {
		limit = model.NormalizeDepthLimit(limit)
	}

	depth, err := h.svc.GetDepth(ctx, req.Market, limit)
	if err != nil {
		if err == service.ErrMarketNotFound {
			return nil, status.Error(codes.NotFound, "market not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get depth: %v", err)
	}

	return &marketv1.GetDepthResponse{
		Market:    depth.Market,
		Bids:      toProtoPriceLevels(depth.Bids),
		Asks:      toProtoPriceLevels(depth.Asks),
		Sequence:  depth.Sequence,
		Timestamp: depth.Timestamp,
	}, nil
}

// Get24HStats 获取 24 小时统计数据
func (h *MarketHandler) Get24HStats(ctx context.Context, req *marketv1.Get24HStatsRequest) (*marketv1.Get24HStatsResponse, error) {
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	ticker, err := h.svc.GetTicker(ctx, req.Market)
	if err != nil {
		if err == service.ErrMarketNotFound {
			return nil, status.Error(codes.NotFound, "market not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get 24h stats: %v", err)
	}

	return &marketv1.Get24HStatsResponse{
		Market:             ticker.Market,
		PriceChange:        ticker.PriceChange.String(),
		PriceChangePercent: ticker.PriceChangePercent.String(),
		High:               ticker.High.String(),
		Low:                ticker.Low.String(),
		Volume:             ticker.Volume.String(),
		QuoteVolume:        ticker.QuoteVolume.String(),
		Open:               ticker.Open.String(),
		LastPrice:          ticker.LastPrice.String(),
		TradeCount:         int32(ticker.TradeCount),
		Timestamp:          ticker.Timestamp,
	}, nil
}

// GetTradeHistory 获取历史成交记录
func (h *MarketHandler) GetTradeHistory(ctx context.Context, req *marketv1.GetTradeHistoryRequest) (*marketv1.GetTradeHistoryResponse, error) {
	if req.Market == "" {
		return nil, status.Error(codes.InvalidArgument, "market is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	trades, err := h.svc.GetTradeHistory(ctx, req.Market, req.StartTime, req.EndTime, limit, req.FromId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get trade history: %v", err)
	}

	resp := &marketv1.GetTradeHistoryResponse{
		Trades: make([]*marketv1.RecentTrade, len(trades)),
	}
	for i, t := range trades {
		resp.Trades[i] = toProtoTrade(t)
	}

	return resp, nil
}

// toProtoMarket 转换 Market 模型到 Proto
func toProtoMarket(m *model.Market) *marketv1.Market {
	return &marketv1.Market{
		Symbol:         m.Symbol,
		BaseToken:      m.BaseToken,
		QuoteToken:     m.QuoteToken,
		PriceDecimals:  int32(m.PriceDecimals),
		SizeDecimals:   int32(m.SizeDecimals),
		MinSize:        m.MinSize.String(),
		MaxSize:        m.MaxSize.String(),
		MinNotional:    m.MinNotional.String(),
		TickSize:       m.TickSize.String(),
		MakerFee:       m.MakerFee.String(),
		TakerFee:       m.TakerFee.String(),
		Status:         marketv1.MarketStatus(m.Status),
		TradingEnabled: m.TradingEnabled,
	}
}

// toProtoTicker 转换 Ticker 模型到 Proto
func toProtoTicker(t *model.Ticker) *marketv1.Ticker {
	return &marketv1.Ticker{
		Market:             t.Market,
		LastPrice:          t.LastPrice.String(),
		PriceChange:        t.PriceChange.String(),
		PriceChangePercent: t.PriceChangePercent.String(),
		High:               t.High.String(),
		Low:                t.Low.String(),
		Volume:             t.Volume.String(),
		QuoteVolume:        t.QuoteVolume.String(),
		Open:               t.Open.String(),
		BestBid:            t.BestBid.String(),
		BestBidQty:         t.BestBidQty.String(),
		BestAsk:            t.BestAsk.String(),
		BestAskQty:         t.BestAskQty.String(),
		TradeCount:         t.TradeCount,
		Timestamp:          t.Timestamp,
	}
}

// toProtoKline 转换 Kline 模型到 Proto
func toProtoKline(k *model.Kline) *marketv1.Kline {
	return &marketv1.Kline{
		Market:      k.Market,
		Interval:    string(k.Interval),
		OpenTime:    k.OpenTime,
		Open:        k.Open.String(),
		High:        k.High.String(),
		Low:         k.Low.String(),
		Close:       k.Close.String(),
		Volume:      k.Volume.String(),
		QuoteVolume: k.QuoteVolume.String(),
		CloseTime:   k.CloseTime,
		TradeCount:  k.TradeCount,
	}
}

// toProtoTrade 转换 Trade 模型到 Proto
func toProtoTrade(t *model.Trade) *marketv1.RecentTrade {
	// 模型: TradeSideBuy=0, TradeSideSell=1
	// Proto: TRADE_SIDE_UNSPECIFIED=0, TRADE_SIDE_BUY=1, TRADE_SIDE_SELL=2
	// 转换: 0 -> 1 (BUY), 1 -> 2 (SELL)
	side := marketv1.TradeSide(int32(t.Side) + 1)
	return &marketv1.RecentTrade{
		TradeId:   t.TradeID,
		Market:    t.Market,
		Price:     t.Price.String(),
		Amount:    t.Amount.String(),
		Side:      side,
		Timestamp: t.Timestamp,
	}
}

// toProtoPriceLevels 转换价格档位到 Proto
func toProtoPriceLevels(levels []*model.PriceLevel) []*marketv1.PriceLevel {
	result := make([]*marketv1.PriceLevel, len(levels))
	for i, l := range levels {
		result[i] = &marketv1.PriceLevel{
			Price:  l.Price.String(),
			Amount: l.Amount.String(),
		}
	}
	return result
}
