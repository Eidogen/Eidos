// Package client 余额扫描数据提供者实现
package client

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/jobs"
)

// BalanceScanDataProviderImpl 余额扫描数据提供者实现
// TODO: 完成 proto 定义后实现实际功能
type BalanceScanDataProviderImpl struct {
	chainClient   *ChainClient
	tradingClient *TradingClient
}

// NewBalanceScanDataProvider 创建余额扫描数据提供者
func NewBalanceScanDataProvider(chainClient *ChainClient, tradingClient *TradingClient) *BalanceScanDataProviderImpl {
	return &BalanceScanDataProviderImpl{
		chainClient:   chainClient,
		tradingClient: tradingClient,
	}
}

// GetWalletsWithActiveOrders 获取有活跃订单的钱包及其冻结金额
// TODO: 需要在 trading proto 中定义 ListWalletsWithFrozenBalance RPC
func (p *BalanceScanDataProviderImpl) GetWalletsWithActiveOrders(ctx context.Context, offset, limit int) ([]*jobs.WalletOrderInfo, error) {
	return nil, fmt.Errorf("not implemented: ListWalletsWithFrozenBalance RPC not defined in proto")
}

// GetOnchainBalance 获取链上余额
// TODO: 需要在 chain proto 中定义 GetBalance RPC
func (p *BalanceScanDataProviderImpl) GetOnchainBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	return decimal.Zero, fmt.Errorf("not implemented: GetBalance RPC not defined in proto")
}

// BatchCancelOrders 批量取消订单
// TODO: 需要在 trading proto 中完善 BatchCancelOrders 请求字段
func (p *BalanceScanDataProviderImpl) BatchCancelOrders(ctx context.Context, orderIDs []string, reason string) (int, error) {
	return 0, fmt.Errorf("not implemented: BatchCancelOrders fields not complete in proto")
}

// SendCancelNotification 发送订单取消通知
// TODO: 需要在 trading proto 中定义 SendNotification RPC
func (p *BalanceScanDataProviderImpl) SendCancelNotification(ctx context.Context, wallet string, orderIDs []string, reason string) error {
	return fmt.Errorf("not implemented: SendNotification RPC not defined in proto")
}

// Ensure BalanceScanDataProviderImpl implements jobs.BalanceScanDataProvider
var _ jobs.BalanceScanDataProvider = (*BalanceScanDataProviderImpl)(nil)
