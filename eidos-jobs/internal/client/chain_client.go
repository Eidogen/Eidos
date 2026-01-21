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
	chainpb "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingpb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ChainClient 链服务客户端
type ChainClient struct {
	conn   *grpc.ClientConn
	client chainpb.ChainServiceClient
}

// NewChainClient 创建链服务客户端
func NewChainClient(addr string) (*ChainClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to chain service: %w", err)
	}

	logger.Info("connected to eidos-chain", zap.String("addr", addr))

	return &ChainClient{
		conn:   conn,
		client: chainpb.NewChainServiceClient(conn),
	}, nil
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
				zap.String("wallet", wallet),
				zap.Error(err))
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
// TODO: 需要在 chain proto 中定义 GetBalance RPC
func (c *ChainClient) GetBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	return decimal.Zero, fmt.Errorf("not implemented: GetBalance RPC not defined in proto")
}

// BatchGetBalances 批量获取余额
// TODO: 需要在 chain proto 中定义 BatchGetBalances RPC
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
	if c.conn != nil {
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
				zap.String("wallet", wallet),
				zap.Error(err))
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
