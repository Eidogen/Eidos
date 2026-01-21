// Package client 提供外部服务客户端
package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	riskpb "github.com/eidos-exchange/eidos/proto/risk/v1"
)

// Risk check errors
var (
	ErrRiskOrderRejected      = errors.New("order rejected by risk service")
	ErrRiskWithdrawalRejected = errors.New("withdrawal rejected by risk service")
	ErrRiskBlacklisted        = errors.New("wallet is blacklisted")
	ErrRiskLimitExceeded      = errors.New("risk limit exceeded")
	ErrRiskServiceUnavailable = errors.New("risk service unavailable")
)

// RiskClient 风控服务客户端
type RiskClient struct {
	conn           *grpc.ClientConn
	client         riskpb.RiskServiceClient
	cfg            *RiskClientConfig
}

// RiskClientConfig 客户端配置
type RiskClientConfig struct {
	Addr           string        // gRPC 地址 (host:port)
	ConnectTimeout time.Duration // 连接超时
	RequestTimeout time.Duration // 请求超时
	MaxRetries     int           // 最大重试次数
	RetryBackoff   time.Duration // 重试间隔
	FailOpen       bool          // 失败开放模式 (服务不可用时允许通过)
}

// DefaultRiskClientConfig 返回默认配置
func DefaultRiskClientConfig(addr string) *RiskClientConfig {
	return &RiskClientConfig{
		Addr:           addr,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 3 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   100 * time.Millisecond,
		FailOpen:       false, // 默认严格模式
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
		"addr", cfg.Addr,
	)

	return &RiskClient{
		conn:   conn,
		client: riskpb.NewRiskServiceClient(conn),
		cfg:    cfg,
	}, nil
}

// CheckOrderForService 订单风控检查 (实现 service.RiskClient 接口)
// 封装原始 gRPC 调用，提供友好的接口
func (c *RiskClient) CheckOrder(ctx context.Context, wallet, market string, side model.OrderSide, orderType model.OrderType, price, amount decimal.Decimal) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	pbSide := commonv1.OrderSide_ORDER_SIDE_BUY
	if side == model.OrderSideSell {
		pbSide = commonv1.OrderSide_ORDER_SIDE_SELL
	}

	pbType := commonv1.OrderType_ORDER_TYPE_LIMIT
	if orderType == model.OrderTypeMarket {
		pbType = commonv1.OrderType_ORDER_TYPE_MARKET
	}

	req := &riskpb.CheckOrderRequest{
		Wallet:    wallet,
		Market:    market,
		Side:      pbSide,
		OrderType: pbType,
		Price:     price.String(),
		Amount:    amount.String(),
	}

	var lastErr error
	for i := 0; i <= c.cfg.MaxRetries; i++ {
		resp, err := c.client.CheckOrder(ctx, req)
		if err != nil {
			lastErr = err
			// 检查是否为不可用错误
			if isUnavailableError(err) {
				if c.cfg.FailOpen {
					logger.Warn("risk service unavailable, fail-open mode enabled",
						"wallet", wallet,
						"error", err)
					return nil
				}
				// 重试
				if i < c.cfg.MaxRetries {
					time.Sleep(c.cfg.RetryBackoff * time.Duration(i+1))
					continue
				}
				return fmt.Errorf("%w: %v", ErrRiskServiceUnavailable, err)
			}
			return fmt.Errorf("check order failed: %w", err)
		}

		if !resp.Approved {
			return fmt.Errorf("%w: %s (code: %s)", ErrRiskOrderRejected, resp.RejectReason, resp.RejectCode)
		}

		return nil
	}

	return fmt.Errorf("%w: %v", ErrRiskServiceUnavailable, lastErr)
}

// CheckWithdrawal 提现风控检查 (实现 service.RiskClient 接口)
func (c *RiskClient) CheckWithdrawal(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	req := &riskpb.CheckWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    amount.String(),
	}

	var lastErr error
	for i := 0; i <= c.cfg.MaxRetries; i++ {
		resp, err := c.client.CheckWithdrawal(ctx, req)
		if err != nil {
			lastErr = err
			if isUnavailableError(err) {
				if c.cfg.FailOpen {
					logger.Warn("risk service unavailable for withdrawal check, fail-open mode enabled",
						"wallet", wallet,
						"error", err)
					return nil
				}
				if i < c.cfg.MaxRetries {
					time.Sleep(c.cfg.RetryBackoff * time.Duration(i+1))
					continue
				}
				return fmt.Errorf("%w: %v", ErrRiskServiceUnavailable, err)
			}
			return fmt.Errorf("check withdrawal failed: %w", err)
		}

		if !resp.Approved {
			return fmt.Errorf("%w: %s (code: %s)", ErrRiskWithdrawalRejected, resp.RejectReason, resp.RejectCode)
		}

		return nil
	}

	return fmt.Errorf("%w: %v", ErrRiskServiceUnavailable, lastErr)
}

// CheckOrderRaw 原始订单风控检查 (直接传递 protobuf)
func (c *RiskClient) CheckOrderRaw(ctx context.Context, req *riskpb.CheckOrderRequest) (*riskpb.CheckOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	return c.client.CheckOrder(ctx, req)
}

// CheckWithdrawalRaw 原始提现风控检查 (直接传递 protobuf)
func (c *RiskClient) CheckWithdrawalRaw(ctx context.Context, req *riskpb.CheckWithdrawalRequest) (*riskpb.CheckWithdrawalResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	return c.client.CheckWithdrawal(ctx, req)
}

// CheckBlacklist 检查黑名单
func (c *RiskClient) CheckBlacklist(ctx context.Context, wallet string) (*riskpb.CheckBlacklistResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	return c.client.CheckBlacklist(ctx, &riskpb.CheckBlacklistRequest{
		Wallet: wallet,
	})
}

// IsBlacklisted 检查钱包是否在黑名单中
func (c *RiskClient) IsBlacklisted(ctx context.Context, wallet string) (bool, error) {
	resp, err := c.CheckBlacklist(ctx, wallet)
	if err != nil {
		if isUnavailableError(err) && c.cfg.FailOpen {
			return false, nil
		}
		return false, err
	}
	return resp.IsBlacklisted, nil
}

// GetUserLimits 获取用户限额
func (c *RiskClient) GetUserLimits(ctx context.Context, wallet string) (*riskpb.GetUserLimitsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
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

// isUnavailableError 检查是否为服务不可用错误
func isUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded
}
