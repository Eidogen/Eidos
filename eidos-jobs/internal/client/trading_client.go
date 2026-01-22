// Package client 提供外部服务 gRPC 客户端
package client

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"

	commonGrpc "github.com/eidos-exchange/eidos/eidos-common/pkg/grpc"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingpb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// TradingClient 交易服务客户端
type TradingClient struct {
	conn     *grpc.ClientConn
	client   tradingpb.TradingServiceClient
	ownsConn bool // 是否拥有连接（用于关闭时判断）
}

// NewTradingClient 创建交易服务客户端
// 使用企业级默认配置：keepalive、负载均衡、拦截器（tracing/metrics/logging）
func NewTradingClient(addr string, enableTracing bool) (*TradingClient, error) {
	conn, err := commonGrpc.DialWithTimeout(
		context.Background(),
		addr,
		"eidos-jobs",
		10*time.Second,
		enableTracing,
	)
	if err != nil {
		return nil, fmt.Errorf("connect to trading service: %w", err)
	}

	logger.Info("connected to eidos-trading", "addr", addr)

	return &TradingClient{
		conn:     conn,
		client:   tradingpb.NewTradingServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewTradingClientFromConn 从现有连接创建客户端（服务发现模式）
// 连接由外部管理（如 ServiceDiscovery），客户端不负责关闭
func NewTradingClientFromConn(conn *grpc.ClientConn) *TradingClient {
	return &TradingClient{
		conn:     conn,
		client:   tradingpb.NewTradingServiceClient(conn),
		ownsConn: false,
	}
}

// GetExpiredOrders 获取需要过期的订单
// 通过 ListOrders API 查询 OPEN/PARTIAL 状态且 expire_at < beforeTime 的订单
func (c *TradingClient) GetExpiredOrders(ctx context.Context, beforeTime int64, limit int) ([]*jobs.OrderExpireRequest, error) {
	resp, err := c.client.ListOrders(ctx, &tradingpb.ListOrdersRequest{
		Statuses: []commonv1.OrderStatus{
			commonv1.OrderStatus_ORDER_STATUS_OPEN,
			commonv1.OrderStatus_ORDER_STATUS_PARTIAL,
		},
		EndTime:  beforeTime, // 使用 end_time 过滤 expire_at
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}

	requests := make([]*jobs.OrderExpireRequest, 0, len(resp.Orders))
	now := time.Now().UnixMilli()
	for _, order := range resp.Orders {
		// 只返回已过期的订单 (expire_at > 0 且 < now)
		if order.ExpireAt > 0 && order.ExpireAt < now {
			requests = append(requests, &jobs.OrderExpireRequest{
				OrderID: order.OrderId,
				Market:  order.Market,
				Wallet:  order.Wallet,
			})
		}
	}

	return requests, nil
}

// Close 关闭连接
func (c *TradingClient) Close() error {
	// 只有自己创建的连接才关闭
	if c.ownsConn && c.conn != nil {
		logger.Info("closing trading client connection")
		return c.conn.Close()
	}
	return nil
}

// Ensure TradingClient implements jobs.TradingClient
var _ jobs.TradingClient = (*TradingClient)(nil)

// StatsDataProviderImpl 统计数据提供者实现
// 通过 gRPC 调用 trading 服务获取统计数据
type StatsDataProviderImpl struct {
	tradingClient *TradingClient
}

// NewStatsDataProvider 创建统计数据提供者
func NewStatsDataProvider(tradingClient *TradingClient) *StatsDataProviderImpl {
	return &StatsDataProviderImpl{
		tradingClient: tradingClient,
	}
}

// GetHourlyTradeStats 获取小时交易统计
func (p *StatsDataProviderImpl) GetHourlyTradeStats(ctx context.Context, startTime, endTime int64) ([]*jobs.TradeStats, error) {
	// 获取该时段的成交列表
	resp, err := p.tradingClient.client.ListTrades(ctx, &tradingpb.ListTradesRequest{
		StartTime: startTime,
		EndTime:   endTime,
		PageSize:  1000, // 分页获取
	})
	if err != nil {
		return nil, fmt.Errorf("list trades: %w", err)
	}

	// 按市场聚合统计
	statsMap := make(map[string]*jobs.TradeStats)
	for _, trade := range resp.Trades {
		stat, ok := statsMap[trade.Market]
		if !ok {
			stat = &jobs.TradeStats{
				Market:      trade.Market,
				TradeVolume: decimal.Zero,
				FeeTotal:    decimal.Zero,
			}
			statsMap[trade.Market] = stat
		}

		amount, _ := decimal.NewFromString(trade.Amount)
		makerFee, _ := decimal.NewFromString(trade.MakerFee)
		takerFee, _ := decimal.NewFromString(trade.TakerFee)

		stat.TradeVolume = stat.TradeVolume.Add(amount)
		stat.TradeCount++
		stat.FeeTotal = stat.FeeTotal.Add(makerFee).Add(takerFee)
	}

	result := make([]*jobs.TradeStats, 0, len(statsMap))
	for _, stat := range statsMap {
		result = append(result, stat)
	}

	return result, nil
}

// GetDailyTradeStats 获取日交易统计
func (p *StatsDataProviderImpl) GetDailyTradeStats(ctx context.Context, date string) ([]*jobs.TradeStats, error) {
	// 解析日期获取时间范围
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}

	startTime := t.UnixMilli()
	endTime := t.AddDate(0, 0, 1).UnixMilli()

	return p.GetHourlyTradeStats(ctx, startTime, endTime)
}

// GetActiveUserCount 获取活跃用户数
func (p *StatsDataProviderImpl) GetActiveUserCount(ctx context.Context, startTime, endTime int64) (int64, error) {
	// 通过交易列表统计不同钱包数量
	resp, err := p.tradingClient.client.ListTrades(ctx, &tradingpb.ListTradesRequest{
		StartTime: startTime,
		EndTime:   endTime,
		PageSize:  1000,
	})
	if err != nil {
		return 0, fmt.Errorf("list trades: %w", err)
	}

	wallets := make(map[string]struct{})
	for _, trade := range resp.Trades {
		wallets[trade.MakerWallet] = struct{}{}
		wallets[trade.TakerWallet] = struct{}{}
	}

	return int64(len(wallets)), nil
}

// GetNewUserCount 获取新增用户数
func (p *StatsDataProviderImpl) GetNewUserCount(ctx context.Context, date string) (int64, error) {
	// 需要用户表支持，暂时返回 0
	// 实际实现需要查询用户注册表
	return 0, nil
}

// Ensure StatsDataProviderImpl implements jobs.StatsDataProvider
var _ jobs.StatsDataProvider = (*StatsDataProviderImpl)(nil)
