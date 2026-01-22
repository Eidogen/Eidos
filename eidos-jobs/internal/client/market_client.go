// Package client 提供外部服务 gRPC 客户端
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
	marketpb "github.com/eidos-exchange/eidos/proto/market/v1"
)

// MarketClient 行情服务客户端
type MarketClient struct {
	conn     *grpc.ClientConn
	client   marketpb.MarketServiceClient
	ownsConn bool // 是否拥有连接（用于关闭时判断）
}

// NewMarketClient 创建行情服务客户端
func NewMarketClient(addr string) (*MarketClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to market service: %w", err)
	}

	logger.Info("connected to eidos-market", "addr", addr)

	return &MarketClient{
		conn:     conn,
		client:   marketpb.NewMarketServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewMarketClientFromConn 从现有连接创建客户端（服务发现模式）
// 连接由外部管理（如 ServiceDiscovery），客户端不负责关闭
func NewMarketClientFromConn(conn *grpc.ClientConn) *MarketClient {
	return &MarketClient{
		conn:     conn,
		client:   marketpb.NewMarketServiceClient(conn),
		ownsConn: false,
	}
}

// GetKlines 获取 K 线数据
func (c *MarketClient) GetKlines(ctx context.Context, market, interval string, startTime, endTime int64) ([]*jobs.Kline, error) {
	resp, err := c.client.GetKlines(ctx, &marketpb.GetKlinesRequest{
		Market:    market,
		Interval:  interval,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     1500, // 最大返回数
	})
	if err != nil {
		return nil, fmt.Errorf("get klines: %w", err)
	}

	klines := make([]*jobs.Kline, len(resp.Klines))
	for i, k := range resp.Klines {
		open, _ := decimal.NewFromString(k.Open)
		high, _ := decimal.NewFromString(k.High)
		low, _ := decimal.NewFromString(k.Low)
		close, _ := decimal.NewFromString(k.Close)
		volume, _ := decimal.NewFromString(k.Volume)

		klines[i] = &jobs.Kline{
			Market:    k.Market,
			Interval:  k.Interval,
			OpenTime:  k.OpenTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: k.CloseTime,
		}
	}

	return klines, nil
}

// SaveKline 保存 K 线数据
// 需要 market proto UpsertKline RPC
func (c *MarketClient) SaveKline(ctx context.Context, kline *jobs.Kline) error {
	return fmt.Errorf("not implemented: UpsertKline RPC not defined in proto")
}

// BatchSaveKlines 批量保存 K 线数据
// 需要 market proto BatchUpsertKlines RPC
func (c *MarketClient) BatchSaveKlines(ctx context.Context, klines []*jobs.Kline) error {
	return fmt.Errorf("not implemented: BatchUpsertKlines RPC not defined in proto")
}

// RepairKlineHistory 修复 K 线历史数据
// 需要 market proto RepairKlineHistory RPC
func (c *MarketClient) RepairKlineHistory(ctx context.Context, market, interval string, startTime, endTime int64) (int, error) {
	return 0, fmt.Errorf("not implemented: RepairKlineHistory RPC not defined in proto")
}

// GetMarkets 获取所有交易对
func (c *MarketClient) GetMarkets(ctx context.Context) ([]string, error) {
	resp, err := c.client.ListMarkets(ctx, &marketpb.ListMarketsRequest{
		Status: marketpb.MarketStatus_MARKET_STATUS_ACTIVE,
	})
	if err != nil {
		return nil, fmt.Errorf("list markets: %w", err)
	}

	markets := make([]string, len(resp.Markets))
	for i, m := range resp.Markets {
		markets[i] = m.Symbol
	}

	return markets, nil
}

// Close 关闭连接
func (c *MarketClient) Close() error {
	// 只有自己创建的连接才关闭
	if c.ownsConn && c.conn != nil {
		logger.Info("closing market client connection")
		return c.conn.Close()
	}
	return nil
}

// Ensure MarketClient implements jobs.KlineDataProvider
var _ jobs.KlineDataProvider = (*MarketClient)(nil)
