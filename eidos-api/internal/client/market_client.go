// Package client 提供 gRPC 客户端封装
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	commonGrpc "github.com/eidos-exchange/eidos/eidos-common/pkg/grpc"
	pb "github.com/eidos-exchange/eidos/proto/market/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MarketClientInterface 定义 Market 客户端接口（用于测试 mock）
type MarketClientInterface interface {
	Ping(ctx context.Context) error
	ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error)
	GetTicker(ctx context.Context, market string) (*dto.TickerResponse, error)
	ListTickers(ctx context.Context) ([]*dto.TickerResponse, error)
	GetDepth(ctx context.Context, market string, limit int) (*dto.DepthResponse, error)
	GetKlines(ctx context.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error)
	GetRecentTrades(ctx context.Context, market string, limit int) ([]*dto.RecentTradeResponse, error)
}

// 确保 MarketClient 实现 MarketClientInterface
var _ MarketClientInterface = (*MarketClient)(nil)

// MarketClient Market 服务 gRPC 客户端
type MarketClient struct {
	conn   *grpc.ClientConn
	client pb.MarketServiceClient
}

// NewMarketClient 创建 Market 客户端（使用现有连接）
func NewMarketClient(conn *grpc.ClientConn) *MarketClient {
	return &MarketClient{
		conn:   conn,
		client: pb.NewMarketServiceClient(conn),
	}
}

// NewMarketClientWithTarget 创建 Market 客户端（自动创建连接）
// 使用企业级默认配置：keepalive、负载均衡、拦截器（tracing/metrics/logging）
func NewMarketClientWithTarget(target string, enableTracing bool, opts ...grpc.DialOption) (*MarketClient, error) {
	// 获取企业级默认选项
	defaultOpts := commonGrpc.DefaultDialOptions("eidos-api", enableTracing)
	opts = append(defaultOpts, opts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial market service: %w", err)
	}

	return &MarketClient{
		conn:   conn,
		client: pb.NewMarketServiceClient(conn),
	}, nil
}

// Close 关闭连接
func (c *MarketClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping 检查连接状态
func (c *MarketClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 使用 ListMarkets 作为健康检查
	_, err := c.client.ListMarkets(ctx, &pb.ListMarketsRequest{})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unavailable {
			return fmt.Errorf("market service unavailable: %w", err)
		}
	}
	return nil
}

// ListMarkets 获取交易对列表
func (c *MarketClient) ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.ListMarkets(ctx, &pb.ListMarketsRequest{})
	if err != nil {
		return nil, convertMarketError(err)
	}

	markets := make([]*dto.MarketResponse, len(resp.Markets))
	for i, m := range resp.Markets {
		markets[i] = convertProtoMarket(m)
	}

	return markets, nil
}

// GetTicker 获取单个 Ticker
func (c *MarketClient) GetTicker(ctx context.Context, market string) (*dto.TickerResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetTicker(ctx, &pb.GetTickerRequest{
		Market: market,
	})
	if err != nil {
		return nil, convertMarketError(err)
	}

	return convertProtoTicker(resp.Ticker), nil
}

// ListTickers 获取所有 Ticker
func (c *MarketClient) ListTickers(ctx context.Context) ([]*dto.TickerResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.ListTickers(ctx, &pb.ListTickersRequest{})
	if err != nil {
		return nil, convertMarketError(err)
	}

	tickers := make([]*dto.TickerResponse, len(resp.Tickers))
	for i, t := range resp.Tickers {
		tickers[i] = convertProtoTicker(t)
	}

	return tickers, nil
}

// GetDepth 获取深度数据
func (c *MarketClient) GetDepth(ctx context.Context, market string, limit int) (*dto.DepthResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetDepth(ctx, &pb.GetDepthRequest{
		Market: market,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, convertMarketError(err)
	}

	return convertProtoDepth(resp), nil
}

// GetKlines 获取 K 线数据
func (c *MarketClient) GetKlines(ctx context.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetKlines(ctx, &pb.GetKlinesRequest{
		Market:    market,
		Interval:  interval,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, convertMarketError(err)
	}

	klines := make([]*dto.KlineResponse, len(resp.Klines))
	for i, k := range resp.Klines {
		klines[i] = convertProtoKline(k)
	}

	return klines, nil
}

// GetRecentTrades 获取最近成交
func (c *MarketClient) GetRecentTrades(ctx context.Context, market string, limit int) ([]*dto.RecentTradeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetRecentTrades(ctx, &pb.GetRecentTradesRequest{
		Market: market,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, convertMarketError(err)
	}

	trades := make([]*dto.RecentTradeResponse, len(resp.Trades))
	for i, t := range resp.Trades {
		trades[i] = convertProtoRecentTrade(t)
	}

	return trades, nil
}

// ========== Proto 转换函数 ==========

// convertProtoMarket 转换 Proto Market 到 DTO
func convertProtoMarket(m *pb.Market) *dto.MarketResponse {
	return &dto.MarketResponse{
		Symbol:         m.Symbol,
		BaseToken:      m.BaseToken,
		QuoteToken:     m.QuoteToken,
		PriceDecimals:  int(m.PriceDecimals),
		AmountDecimals: int(m.SizeDecimals),
		MinAmount:      m.MinSize,
		MaxAmount:      m.MaxSize,
		MinNotional:    m.MinNotional,
		TickSize:       m.TickSize,
		MakerFeeRate:   m.MakerFee,
		TakerFeeRate:   m.TakerFee,
		IsActive:       m.TradingEnabled,
	}
}

// convertProtoTicker 转换 Proto Ticker 到 DTO
func convertProtoTicker(t *pb.Ticker) *dto.TickerResponse {
	return &dto.TickerResponse{
		Market:             t.Market,
		LastPrice:          t.LastPrice,
		PriceChange:        t.PriceChange,
		PriceChangePercent: t.PriceChangePercent,
		Open:               t.Open,
		High:               t.High,
		Low:                t.Low,
		Volume:             t.Volume,
		QuoteVolume:        t.QuoteVolume,
		BestBid:            t.BestBid,
		BestBidQty:         t.BestBidQty,
		BestAsk:            t.BestAsk,
		BestAskQty:         t.BestAskQty,
		TradeCount:         int(t.TradeCount),
		Timestamp:          t.Timestamp,
	}
}

// convertProtoDepth 转换 Proto Depth 到 DTO
func convertProtoDepth(d *pb.GetDepthResponse) *dto.DepthResponse {
	bids := make([][]string, len(d.Bids))
	for i, b := range d.Bids {
		bids[i] = []string{b.Price, b.Amount}
	}

	asks := make([][]string, len(d.Asks))
	for i, a := range d.Asks {
		asks[i] = []string{a.Price, a.Amount}
	}

	return &dto.DepthResponse{
		Market:    d.Market,
		Bids:      bids,
		Asks:      asks,
		Sequence:  d.Sequence,
		Timestamp: d.Timestamp,
	}
}

// convertProtoKline 转换 Proto Kline 到 DTO
func convertProtoKline(k *pb.Kline) *dto.KlineResponse {
	return &dto.KlineResponse{
		Market:      k.Market,
		Interval:    k.Interval,
		OpenTime:    k.OpenTime,
		Open:        k.Open,
		High:        k.High,
		Low:         k.Low,
		Close:       k.Close,
		Volume:      k.Volume,
		QuoteVolume: k.QuoteVolume,
		TradeCount:  int(k.TradeCount),
		CloseTime:   k.CloseTime,
	}
}

// convertProtoRecentTrade 转换 Proto RecentTrade 到 DTO
func convertProtoRecentTrade(t *pb.RecentTrade) *dto.RecentTradeResponse {
	side := "buy"
	if t.Side == pb.TradeSide_TRADE_SIDE_SELL {
		side = "sell"
	}

	return &dto.RecentTradeResponse{
		TradeID:   t.TradeId,
		Market:    t.Market,
		Price:     t.Price,
		Amount:    t.Amount,
		Side:      side,
		Timestamp: t.Timestamp,
	}
}

// convertMarketError 转换 gRPC 错误到业务错误
func convertMarketError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return dto.ErrInternalError
	}

	switch st.Code() {
	case codes.NotFound:
		return dto.ErrMarketNotFound
	case codes.InvalidArgument:
		return dto.ErrInvalidParams.WithMessage(st.Message())
	case codes.Unavailable:
		return dto.ErrServiceUnavailable
	default:
		return dto.ErrInternalError
	}
}
