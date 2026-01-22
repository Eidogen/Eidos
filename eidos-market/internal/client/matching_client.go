// Package client 提供外部服务客户端
package client

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"

	commonGrpc "github.com/eidos-exchange/eidos/eidos-common/pkg/grpc"
	"github.com/eidos-exchange/eidos/eidos-market/internal/aggregator"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	matchingpb "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// MatchingClient 撮合服务客户端
// 实现 aggregator.DepthSnapshotProvider 接口
type MatchingClient struct {
	conn   *grpc.ClientConn
	client matchingpb.MatchingServiceClient
	logger *slog.Logger
	config *MatchingClientConfig
}

// MatchingClientConfig 客户端配置
type MatchingClientConfig struct {
	Addr           string        // gRPC 地址 (host:port)
	ConnectTimeout time.Duration // 连接超时
	RequestTimeout time.Duration // 请求超时
}

// DefaultMatchingClientConfig 返回默认配置
func DefaultMatchingClientConfig(addr string) *MatchingClientConfig {
	return &MatchingClientConfig{
		Addr:           addr,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 3 * time.Second,
	}
}

// NewMatchingClient 创建撮合服务客户端
// 使用企业级默认配置：keepalive、负载均衡、拦截器（tracing/metrics/logging）
func NewMatchingClient(cfg *MatchingClientConfig, logger *slog.Logger, enableTracing bool) (*MatchingClient, error) {
	conn, err := commonGrpc.DialWithTimeout(
		context.Background(),
		cfg.Addr,
		"eidos-market",
		cfg.ConnectTimeout,
		enableTracing,
	)
	if err != nil {
		return nil, fmt.Errorf("connect to matching service: %w", err)
	}

	logger.Info("connected to eidos-matching",
		"addr", cfg.Addr,
	)

	return &MatchingClient{
		conn:   conn,
		client: matchingpb.NewMatchingServiceClient(conn),
		logger: logger,
		config: cfg,
	}, nil
}

// GetSnapshot 获取订单簿快照（实现 aggregator.DepthSnapshotProvider 接口）
func (c *MatchingClient) GetSnapshot(ctx context.Context, market string) (*model.Depth, error) {
	// 设置请求超时
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	resp, err := c.client.GetOrderbook(ctx, &matchingpb.GetOrderbookRequest{
		Market: market,
		Limit:  100, // 获取最大档位数
	})
	if err != nil {
		c.logger.Error("failed to get orderbook snapshot",
			"market", market,
			"error", err)
		return nil, fmt.Errorf("get orderbook: %w", err)
	}

	// 转换为 model.Depth
	depth := &model.Depth{
		Market:    resp.Market,
		Sequence:  resp.Sequence,
		Timestamp: resp.Timestamp,
		Bids:      make([]*model.PriceLevel, 0, len(resp.Bids)),
		Asks:      make([]*model.PriceLevel, 0, len(resp.Asks)),
	}

	for _, bid := range resp.Bids {
		price, _ := decimal.NewFromString(bid.Price)
		amount, _ := decimal.NewFromString(bid.Amount)
		depth.Bids = append(depth.Bids, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	for _, ask := range resp.Asks {
		price, _ := decimal.NewFromString(ask.Price)
		amount, _ := decimal.NewFromString(ask.Amount)
		depth.Asks = append(depth.Asks, &model.PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	c.logger.Debug("orderbook snapshot fetched",
		"market", market,
		"sequence", depth.Sequence,
		"bids", len(depth.Bids),
		"asks", len(depth.Asks))

	return depth, nil
}

// GetDepth 获取订单簿深度
func (c *MatchingClient) GetDepth(ctx context.Context, market string, level int32) (*matchingpb.GetDepthResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	return c.client.GetDepth(ctx, &matchingpb.GetDepthRequest{
		Market: market,
		Level:  level,
	})
}

// HealthCheck 健康检查
func (c *MatchingClient) HealthCheck(ctx context.Context) (*matchingpb.HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()

	return c.client.HealthCheck(ctx, &matchingpb.HealthCheckRequest{})
}

// Close 关闭连接
func (c *MatchingClient) Close() error {
	if c.conn != nil {
		c.logger.Info("closing matching client connection")
		return c.conn.Close()
	}
	return nil
}

// 确保 MatchingClient 实现了 DepthSnapshotProvider 接口
var _ aggregator.DepthSnapshotProvider = (*MatchingClient)(nil)
