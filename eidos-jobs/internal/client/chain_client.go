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
	chainpb "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingpb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ChainClient 链服务客户端
type ChainClient struct {
	conn     *grpc.ClientConn
	client   chainpb.ChainServiceClient
	ownsConn bool // 是否拥有连接（用于关闭时判断）
}

// NewChainClient 创建链服务客户端
// 使用企业级默认配置：keepalive、负载均衡、拦截器（tracing/metrics/logging）
func NewChainClient(addr string, enableTracing bool) (*ChainClient, error) {
	conn, err := commonGrpc.DialWithTimeout(
		context.Background(),
		addr,
		"eidos-jobs",
		10*time.Second,
		enableTracing,
	)
	if err != nil {
		return nil, fmt.Errorf("connect to chain service: %w", err)
	}

	logger.Info("connected to eidos-chain", "addr", addr)

	return &ChainClient{
		conn:     conn,
		client:   chainpb.NewChainServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewChainClientFromConn 从现有连接创建客户端（服务发现模式）
// 连接由外部管理（如 ServiceDiscovery），客户端不负责关闭
func NewChainClientFromConn(conn *grpc.ClientConn) *ChainClient {
	return &ChainClient{
		conn:     conn,
		client:   chainpb.NewChainServiceClient(conn),
		ownsConn: false,
	}
}

// GetLatestBlockNumber 获取最新区块号
func (c *ChainClient) GetLatestBlockNumber(ctx context.Context) (int64, error) {
	resp, err := c.client.GetBlockHeight(ctx, &chainpb.GetBlockHeightRequest{
		ChainId: 42161, // Arbitrum mainnet
	})
	if err != nil {
		return 0, fmt.Errorf("get block height: %w", err)
	}

	return int64(resp.BlockHeight), nil
}

// GetOnchainBalances 获取链上余额
func (c *ChainClient) GetOnchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	// 通过 ListReconciliationRecords 获取链上余额数据
	result := make(map[string]map[string]decimal.Decimal)

	for _, wallet := range wallets {
		resp, err := c.client.ListReconciliationRecords(ctx, &chainpb.ListReconciliationRecordsRequest{
			WalletAddress: wallet,
			Pagination: &commonv1.PaginationRequest{
				Page:     1,
				PageSize: 100,
			},
		})
		if err != nil {
			logger.Warn("failed to get reconciliation records",
				"wallet", wallet,
				"error", err)
			continue
		}

		balances := make(map[string]decimal.Decimal)
		for _, record := range resp.Records {
			balance, _ := decimal.NewFromString(record.OnChainBalance)
			balances[record.Token] = balance
		}

		result[wallet] = balances
	}

	return result, nil
}

// GetBalance 获取单个钱包的单个代币余额
// 需要 chain proto GetBalance RPC
func (c *ChainClient) GetBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	return decimal.Zero, fmt.Errorf("not implemented: GetBalance RPC not defined in proto")
}

// BatchGetBalances 批量获取余额
// 需要 chain proto BatchGetBalances RPC
func (c *ChainClient) BatchGetBalances(ctx context.Context, requests []*BalanceRequest) (map[string]map[string]decimal.Decimal, error) {
	return nil, fmt.Errorf("not implemented: BatchGetBalances RPC not defined in proto")
}

// BalanceRequest 余额查询请求
type BalanceRequest struct {
	Wallet string
	Token  string
}

// Close 关闭连接
func (c *ChainClient) Close() error {
	// 只有自己创建的连接才关闭
	if c.ownsConn && c.conn != nil {
		logger.Info("closing chain client connection")
		return c.conn.Close()
	}
	return nil
}

// ReconciliationDataProviderImpl 对账数据提供者实现
type ReconciliationDataProviderImpl struct {
	chainClient   *ChainClient
	tradingClient *TradingClient
}

// NewReconciliationDataProvider 创建对账数据提供者
func NewReconciliationDataProvider(chainClient *ChainClient, tradingClient *TradingClient) *ReconciliationDataProviderImpl {
	return &ReconciliationDataProviderImpl{
		chainClient:   chainClient,
		tradingClient: tradingClient,
	}
}

// GetOffchainBalances 获取链下余额
func (p *ReconciliationDataProviderImpl) GetOffchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	result := make(map[string]map[string]decimal.Decimal)

	for _, wallet := range wallets {
		resp, err := p.tradingClient.client.GetBalances(ctx, &tradingpb.GetBalancesRequest{
			Wallet: wallet,
		})
		if err != nil {
			logger.Warn("failed to get balances",
				"wallet", wallet,
				"error", err)
			continue
		}

		balances := make(map[string]decimal.Decimal)
		for _, balance := range resp.Balances {
			// settled_available 代表可用的已结算余额
			settled, _ := decimal.NewFromString(balance.SettledAvailable)
			balances[balance.Token] = settled
		}

		result[wallet] = balances
	}

	return result, nil
}

// GetOnchainBalances 获取链上余额
func (p *ReconciliationDataProviderImpl) GetOnchainBalances(ctx context.Context, wallets []string) (map[string]map[string]decimal.Decimal, error) {
	return p.chainClient.GetOnchainBalances(ctx, wallets)
}

// GetChangedWallets 获取在指定时间范围内有变动的钱包
func (p *ReconciliationDataProviderImpl) GetChangedWallets(ctx context.Context, startTime, endTime int64) ([]string, error) {
	// 通过查询余额流水获取有变动的钱包
	// 注意: 这需要 trading 服务提供批量查询余额流水的接口
	// 暂时返回空列表，让对账任务做全量对账
	logger.Debug("GetChangedWallets not fully implemented, returning empty list")
	return nil, nil
}

// GetAllWallets 获取所有有余额的钱包
func (p *ReconciliationDataProviderImpl) GetAllWallets(ctx context.Context) ([]string, error) {
	// 通过 chain 服务的对账记录获取所有钱包
	resp, err := p.chainClient.client.ListReconciliationRecords(ctx, &chainpb.ListReconciliationRecordsRequest{
		Pagination: &commonv1.PaginationRequest{
			Page:     1,
			PageSize: 1000,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list reconciliation records: %w", err)
	}

	walletSet := make(map[string]struct{})
	for _, record := range resp.Records {
		walletSet[record.WalletAddress] = struct{}{}
	}

	wallets := make([]string, 0, len(walletSet))
	for wallet := range walletSet {
		wallets = append(wallets, wallet)
	}

	return wallets, nil
}

// GetLatestBlockNumber 获取最新区块号
func (p *ReconciliationDataProviderImpl) GetLatestBlockNumber(ctx context.Context) (int64, error) {
	return p.chainClient.GetLatestBlockNumber(ctx)
}

// Ensure ReconciliationDataProviderImpl implements jobs.ReconciliationDataProvider
var _ jobs.ReconciliationDataProvider = (*ReconciliationDataProviderImpl)(nil)
