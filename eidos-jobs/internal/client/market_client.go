// Package client 提供外部服务 gRPC 客户端
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
	marketpb "github.com/eidos-exchange/eidos/proto/market/v1"
)

// MarketClient 行情服务客户端
type MarketClient struct {
	conn   *grpc.ClientConn
	client marketpb.MarketServiceClient
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

	logger.Info("connected to eidos-market", zap.String("addr", addr))

	return &MarketClient{
		conn:   conn,
		client: marketpb.NewMarketServiceClient(conn),
	}, nil
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
// 注意: proto 中没有定义保存 K 线的接口，需要 eidos-market 增加或使用直连数据库
func (c *MarketClient) SaveKline(ctx context.Context, kline *jobs.Kline) error {
	// TODO: 需要 eidos-market 提供 SaveKline gRPC 接口
	// 或者直连 eidos-market 的数据库保存
	logger.Debug("save kline (not implemented via gRPC)",
		zap.String("market", kline.Market),
		zap.String("interval", kline.Interval))
	return nil
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
	if c.conn != nil {
		logger.Info("closing market client connection")
		return c.conn.Close()
	}
	return nil
}

// Ensure MarketClient implements jobs.KlineDataProvider
var _ jobs.KlineDataProvider = (*MarketClient)(nil)
