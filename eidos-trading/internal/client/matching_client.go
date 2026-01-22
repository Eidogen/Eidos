// Package client 提供外部服务客户端
package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	matchingpb "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// MatchingClient 撮合服务客户端
type MatchingClient struct {
	conn      *grpc.ClientConn
	client    matchingpb.MatchingServiceClient
	ownsConn  bool // 是否拥有连接（用于关闭时判断）
}

// MatchingClientConfig 客户端配置
type MatchingClientConfig struct {
	Addr            string        // gRPC 地址 (host:port)
	ConnectTimeout  time.Duration // 连接超时
	RequestTimeout  time.Duration // 请求超时
	MaxRetries      int           // 最大重试次数
	RetryBackoff    time.Duration // 重试间隔
}

// DefaultMatchingClientConfig 返回默认配置
func DefaultMatchingClientConfig(addr string) *MatchingClientConfig {
	return &MatchingClientConfig{
		Addr:           addr,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 3 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   100 * time.Millisecond,
	}
}

// NewMatchingClient 创建撮合服务客户端（自行管理连接）
func NewMatchingClient(cfg *MatchingClientConfig) (*MatchingClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to matching service: %w", err)
	}

	logger.Info("connected to eidos-matching",
		"addr", cfg.Addr,
	)

	return &MatchingClient{
		conn:     conn,
		client:   matchingpb.NewMatchingServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewMatchingClientFromConn 从现有连接创建客户端（服务发现模式）
// 连接由外部管理（如 ServiceDiscovery），客户端不负责关闭
func NewMatchingClientFromConn(conn *grpc.ClientConn) *MatchingClient {
	return &MatchingClient{
		conn:     conn,
		client:   matchingpb.NewMatchingServiceClient(conn),
		ownsConn: false,
	}
}

// GetOrderbook 获取订单簿快照
func (c *MatchingClient) GetOrderbook(ctx context.Context, market string, limit int32) (*matchingpb.GetOrderbookResponse, error) {
	return c.client.GetOrderbook(ctx, &matchingpb.GetOrderbookRequest{
		Market: market,
		Limit:  limit,
	})
}

// GetDepth 获取订单簿深度
func (c *MatchingClient) GetDepth(ctx context.Context, market string, level int32) (*matchingpb.GetDepthResponse, error) {
	return c.client.GetDepth(ctx, &matchingpb.GetDepthRequest{
		Market: market,
		Level:  level,
	})
}

// HealthCheck 健康检查
func (c *MatchingClient) HealthCheck(ctx context.Context) (*matchingpb.HealthCheckResponse, error) {
	return c.client.HealthCheck(ctx, &matchingpb.HealthCheckRequest{})
}

// Close 关闭连接
func (c *MatchingClient) Close() error {
	// 只有自己创建的连接才关闭
	if c.ownsConn && c.conn != nil {
		logger.Info("closing matching client connection")
		return c.conn.Close()
	}
	return nil
}
