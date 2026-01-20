// Package client 提供 gRPC 客户端封装
package client

import (
	"context"
	"fmt"
	"time"

	commonpb "github.com/eidos-exchange/eidos/proto/common/v1"
	pb "github.com/eidos-exchange/eidos/proto/risk/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// RiskClientInterface 定义 Risk 客户端接口（用于测试 mock）
type RiskClientInterface interface {
	Ping(ctx context.Context) error
	CheckOrder(ctx context.Context, req *CheckOrderRequest) (*CheckOrderResponse, error)
	CheckWithdraw(ctx context.Context, req *CheckWithdrawRequest) (*CheckWithdrawResponse, error)
	CheckBlacklist(ctx context.Context, wallet string) (*CheckBlacklistResponse, error)
	GetUserLimits(ctx context.Context, wallet string) (*GetUserLimitsResponse, error)
}

// 确保 RiskClient 实现 RiskClientInterface
var _ RiskClientInterface = (*RiskClient)(nil)

// RiskClient Risk 服务 gRPC 客户端
type RiskClient struct {
	conn   *grpc.ClientConn
	client pb.RiskServiceClient
}

// CheckOrderRequest 订单检查请求
type CheckOrderRequest struct {
	Wallet    string
	Market    string
	Side      string // buy, sell
	OrderType string // limit, market
	Price     string
	Amount    string
}

// CheckOrderResponse 订单检查响应
type CheckOrderResponse struct {
	Approved     bool
	RejectReason string
	RejectCode   string
	Warnings     []string
}

// CheckWithdrawRequest 提现检查请求
type CheckWithdrawRequest struct {
	Wallet    string
	Token     string
	Amount    string
	ToAddress string
}

// CheckWithdrawResponse 提现检查响应
type CheckWithdrawResponse struct {
	Approved            bool
	RejectReason        string
	RejectCode          string
	RequireManualReview bool
}

// CheckBlacklistResponse 黑名单检查响应
type CheckBlacklistResponse struct {
	IsBlacklisted bool
	Reason        string
	ExpireAt      int64
}

// UserLimit 用户限额
type UserLimit struct {
	LimitType      string
	Token          string
	MaxValue       string
	UsedValue      string
	RemainingValue string
	ResetAt        int64
}

// GetUserLimitsResponse 获取用户限额响应
type GetUserLimitsResponse struct {
	Wallet string
	Limits []*UserLimit
}

// NewRiskClient 创建 Risk 客户端（使用现有连接）
func NewRiskClient(conn *grpc.ClientConn) *RiskClient {
	return &RiskClient{
		conn:   conn,
		client: pb.NewRiskServiceClient(conn),
	}
}

// NewRiskClientWithTarget 创建 Risk 客户端（自动创建连接）
func NewRiskClientWithTarget(target string, opts ...grpc.DialOption) (*RiskClient, error) {
	// 默认选项
	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	}
	opts = append(defaultOpts, opts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial risk service: %w", err)
	}

	return &RiskClient{
		conn:   conn,
		client: pb.NewRiskServiceClient(conn),
	}, nil
}

// Close 关闭连接
func (c *RiskClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping 检查连接状态
func (c *RiskClient) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// 使用 ListRiskRules 作为健康检查
	_, err := c.client.ListRiskRules(ctx, &pb.ListRiskRulesRequest{})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unavailable {
			return fmt.Errorf("risk service unavailable: %w", err)
		}
	}
	return nil
}

// CheckOrder 检查订单风控
func (c *RiskClient) CheckOrder(ctx context.Context, req *CheckOrderRequest) (*CheckOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// 转换 side
	var side commonpb.OrderSide
	switch req.Side {
	case "buy":
		side = commonpb.OrderSide_ORDER_SIDE_BUY
	case "sell":
		side = commonpb.OrderSide_ORDER_SIDE_SELL
	default:
		side = commonpb.OrderSide_ORDER_SIDE_UNSPECIFIED
	}

	// 转换 order type
	var orderType commonpb.OrderType
	switch req.OrderType {
	case "limit":
		orderType = commonpb.OrderType_ORDER_TYPE_LIMIT
	case "market":
		orderType = commonpb.OrderType_ORDER_TYPE_MARKET
	default:
		orderType = commonpb.OrderType_ORDER_TYPE_UNSPECIFIED
	}

	resp, err := c.client.CheckOrder(ctx, &pb.CheckOrderRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      side,
		OrderType: orderType,
		Price:     req.Price,
		Amount:    req.Amount,
	})
	if err != nil {
		return nil, convertRiskError(err)
	}

	return &CheckOrderResponse{
		Approved:     resp.Approved,
		RejectReason: resp.RejectReason,
		RejectCode:   resp.RejectCode,
		Warnings:     resp.Warnings,
	}, nil
}

// CheckWithdraw 检查提现风控
func (c *RiskClient) CheckWithdraw(ctx context.Context, req *CheckWithdrawRequest) (*CheckWithdrawResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.CheckWithdraw(ctx, &pb.CheckWithdrawRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    req.Amount,
		ToAddress: req.ToAddress,
	})
	if err != nil {
		return nil, convertRiskError(err)
	}

	return &CheckWithdrawResponse{
		Approved:            resp.Approved,
		RejectReason:        resp.RejectReason,
		RejectCode:          resp.RejectCode,
		RequireManualReview: resp.RequireManualReview,
	}, nil
}

// CheckBlacklist 检查黑名单
func (c *RiskClient) CheckBlacklist(ctx context.Context, wallet string) (*CheckBlacklistResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.CheckBlacklist(ctx, &pb.CheckBlacklistRequest{
		Wallet: wallet,
	})
	if err != nil {
		return nil, convertRiskError(err)
	}

	return &CheckBlacklistResponse{
		IsBlacklisted: resp.IsBlacklisted,
		Reason:        resp.Reason,
		ExpireAt:      resp.ExpireAt,
	}, nil
}

// GetUserLimits 获取用户限额
func (c *RiskClient) GetUserLimits(ctx context.Context, wallet string) (*GetUserLimitsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	resp, err := c.client.GetUserLimits(ctx, &pb.GetUserLimitsRequest{
		Wallet: wallet,
	})
	if err != nil {
		return nil, convertRiskError(err)
	}

	limits := make([]*UserLimit, len(resp.Limits))
	for i, l := range resp.Limits {
		limits[i] = &UserLimit{
			LimitType:      l.LimitType,
			Token:          l.Token,
			MaxValue:       l.MaxValue,
			UsedValue:      l.UsedValue,
			RemainingValue: l.RemainingValue,
			ResetAt:        l.ResetAt,
		}
	}

	return &GetUserLimitsResponse{
		Wallet: resp.Wallet,
		Limits: limits,
	}, nil
}

// convertRiskError 转换 gRPC 错误为业务错误
func convertRiskError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("risk service error: %w", err)
	}

	switch st.Code() {
	case codes.NotFound:
		return fmt.Errorf("resource not found: %s", st.Message())
	case codes.InvalidArgument:
		return fmt.Errorf("invalid argument: %s", st.Message())
	case codes.Unavailable:
		return fmt.Errorf("risk service unavailable: %s", st.Message())
	case codes.Internal:
		return fmt.Errorf("internal error: %s", st.Message())
	default:
		return fmt.Errorf("risk service error: %s", st.Message())
	}
}
