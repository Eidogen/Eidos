// Package client 提供外部服务客户端
package client

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	riskpb "github.com/eidos-exchange/eidos/proto/risk/v1"
)

// RiskClient 风控服务客户端
type RiskClient struct {
	conn   *grpc.ClientConn
	client riskpb.RiskServiceClient
}

// RiskClientConfig 客户端配置
type RiskClientConfig struct {
	Addr           string        // gRPC 地址 (host:port)
	ConnectTimeout time.Duration // 连接超时
	RequestTimeout time.Duration // 请求超时
	MaxRetries     int           // 最大重试次数
	RetryBackoff   time.Duration // 重试间隔
}

// DefaultRiskClientConfig 返回默认配置
func DefaultRiskClientConfig(addr string) *RiskClientConfig {
	return &RiskClientConfig{
		Addr:           addr,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 3 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   100 * time.Millisecond,
	}
}

// NewRiskClient 创建风控服务客户端
func NewRiskClient(cfg *RiskClientConfig) (*RiskClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to risk service: %w", err)
	}

	logger.Info("connected to eidos-risk",
		zap.String("addr", cfg.Addr),
	)

	return &RiskClient{
		conn:   conn,
		client: riskpb.NewRiskServiceClient(conn),
	}, nil
}

// CheckOrder 订单风控检查
func (c *RiskClient) CheckOrder(ctx context.Context, req *riskpb.CheckOrderRequest) (*riskpb.CheckOrderResponse, error) {
	return c.client.CheckOrder(ctx, req)
}

// CheckWithdraw 提现风控检查
func (c *RiskClient) CheckWithdraw(ctx context.Context, req *riskpb.CheckWithdrawRequest) (*riskpb.CheckWithdrawResponse, error) {
	return c.client.CheckWithdraw(ctx, req)
}

// CheckBlacklist 检查黑名单
func (c *RiskClient) CheckBlacklist(ctx context.Context, wallet string) (*riskpb.CheckBlacklistResponse, error) {
	return c.client.CheckBlacklist(ctx, &riskpb.CheckBlacklistRequest{
		Wallet: wallet,
	})
}

// GetUserLimits 获取用户限额
func (c *RiskClient) GetUserLimits(ctx context.Context, wallet string) (*riskpb.GetUserLimitsResponse, error) {
	return c.client.GetUserLimits(ctx, &riskpb.GetUserLimitsRequest{
		Wallet: wallet,
	})
}

// Close 关闭连接
func (c *RiskClient) Close() error {
	if c.conn != nil {
		logger.Info("closing risk client connection")
		return c.conn.Close()
	}
	return nil
}
