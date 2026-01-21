// Package jobs 余额扫描取消订单任务
// 定时扫描有挂单的用户的链上余额，如果链上余额不足以支付订单所需金额，自动取消订单
// 适用于离线签名场景：用户签名后余额被转走
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
)

// BalanceScanConfig 余额扫描配置
type BalanceScanConfig struct {
	// ScanInterval 扫描间隔（由 cron 控制，此处仅用于文档）
	ScanInterval time.Duration
	// BatchSize 每批处理的钱包数量
	BatchSize int
	// CancelThresholdPercent 取消阈值百分比
	// 当链上余额 < 冻结金额 * (1 - threshold) 时取消订单
	// 例如：0.05 表示允许 5% 的误差
	CancelThresholdPercent float64
	// MaxConcurrentQueries 最大并发查询数
	MaxConcurrentQueries int
	// EnableNotification 是否发送通知
	EnableNotification bool
}

// DefaultBalanceScanConfig 默认配置
var DefaultBalanceScanConfig = BalanceScanConfig{
	ScanInterval:           5 * time.Minute,
	BatchSize:              100,
	CancelThresholdPercent: 0.01, // 1% 误差容忍
	MaxConcurrentQueries:   10,
	EnableNotification:     true,
}

// WalletOrderInfo 钱包订单信息
type WalletOrderInfo struct {
	Wallet        string
	Token         string
	TotalFrozen   decimal.Decimal // 该钱包在该代币上的总冻结金额
	OrderCount    int             // 订单数量
	OrderIDs      []string        // 订单ID列表
	Markets       []string        // 关联的市场列表
}

// BalanceScanDataProvider 余额扫描数据提供者接口
type BalanceScanDataProvider interface {
	// GetWalletsWithActiveOrders 获取有活跃订单的钱包及其冻结金额
	GetWalletsWithActiveOrders(ctx context.Context, offset, limit int) ([]*WalletOrderInfo, error)
	// GetOnchainBalance 获取链上余额
	GetOnchainBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error)
	// BatchCancelOrders 批量取消订单
	BatchCancelOrders(ctx context.Context, orderIDs []string, reason string) (int, error)
	// SendCancelNotification 发送订单取消通知
	SendCancelNotification(ctx context.Context, wallet string, orderIDs []string, reason string) error
}

// BalanceScanJob 余额扫描取消订单任务
type BalanceScanJob struct {
	scheduler.BaseJob
	dataProvider BalanceScanDataProvider
	config       BalanceScanConfig
	alertFunc    func(ctx context.Context, alert *BalanceScanAlert) error
}

// BalanceScanAlert 余额扫描告警
type BalanceScanAlert struct {
	Wallet           string
	Token            string
	OnchainBalance   decimal.Decimal
	FrozenAmount     decimal.Decimal
	CancelledOrders  int
	CancelledOrderIDs []string
	Timestamp        int64
}

// NewBalanceScanJob 创建余额扫描任务
func NewBalanceScanJob(
	dataProvider BalanceScanDataProvider,
	config *BalanceScanConfig,
) *BalanceScanJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameBalanceScan]

	jobConfig := DefaultBalanceScanConfig
	if config != nil {
		jobConfig = *config
	}

	return &BalanceScanJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameBalanceScan,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		dataProvider: dataProvider,
		config:       jobConfig,
	}
}

// SetAlertFunc 设置告警回调
func (j *BalanceScanJob) SetAlertFunc(f func(ctx context.Context, alert *BalanceScanAlert) error) {
	j.alertFunc = f
}

// Execute 执行余额扫描
func (j *BalanceScanJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	startTime := time.Now()
	logger.Info("starting balance scan job")

	totalScanned := 0
	totalInsufficientWallets := 0
	totalCancelledOrders := 0
	totalErrors := 0

	offset := 0
	thresholdMultiplier := decimal.NewFromFloat(1 - j.config.CancelThresholdPercent)

	for {
		select {
		case <-ctx.Done():
			logger.Warn("balance scan job cancelled",
				zap.Int("scanned", totalScanned),
				zap.Int("cancelled", totalCancelledOrders))
			result.ProcessedCount = totalScanned
			result.AffectedCount = totalCancelledOrders
			result.ErrorCount = totalErrors
			return result, ctx.Err()
		default:
		}

		// 获取有活跃订单的钱包
		walletInfos, err := j.dataProvider.GetWalletsWithActiveOrders(ctx, offset, j.config.BatchSize)
		if err != nil {
			logger.Error("failed to get wallets with active orders",
				zap.Int("offset", offset),
				zap.Error(err))
			totalErrors++
			break
		}

		if len(walletInfos) == 0 {
			logger.Debug("no more wallets to scan")
			break
		}

		// 并发查询链上余额
		insufficientWallets, cancelledCount, batchErrors := j.processBatch(ctx, walletInfos, thresholdMultiplier)

		totalScanned += len(walletInfos)
		totalInsufficientWallets += insufficientWallets
		totalCancelledOrders += cancelledCount
		totalErrors += batchErrors

		logger.Debug("processed batch",
			zap.Int("batch_size", len(walletInfos)),
			zap.Int("insufficient", insufficientWallets),
			zap.Int("cancelled", cancelledCount))

		offset += j.config.BatchSize

		// 如果返回的数量小于批次大小，说明已处理完毕
		if len(walletInfos) < j.config.BatchSize {
			break
		}
	}

	duration := time.Since(startTime)
	result.ProcessedCount = totalScanned
	result.AffectedCount = totalCancelledOrders
	result.ErrorCount = totalErrors
	result.Details["scanned_wallets"] = totalScanned
	result.Details["insufficient_wallets"] = totalInsufficientWallets
	result.Details["cancelled_orders"] = totalCancelledOrders
	result.Details["duration_ms"] = duration.Milliseconds()

	logger.Info("balance scan job completed",
		zap.Int("scanned_wallets", totalScanned),
		zap.Int("insufficient_wallets", totalInsufficientWallets),
		zap.Int("cancelled_orders", totalCancelledOrders),
		zap.Int("errors", totalErrors),
		zap.Duration("duration", duration))

	return result, nil
}

// processBatch 处理一批钱包
func (j *BalanceScanJob) processBatch(ctx context.Context, walletInfos []*WalletOrderInfo, thresholdMultiplier decimal.Decimal) (int, int, int) {
	insufficientCount := 0
	cancelledCount := 0
	errorCount := 0

	// 使用 semaphore 控制并发
	sem := make(chan struct{}, j.config.MaxConcurrentQueries)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, info := range walletInfos {
		select {
		case <-ctx.Done():
			return insufficientCount, cancelledCount, errorCount
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(walletInfo *WalletOrderInfo) {
			defer func() {
				<-sem
				wg.Done()
			}()

			insufficient, cancelled, hasError := j.processWallet(ctx, walletInfo, thresholdMultiplier)

			mu.Lock()
			if insufficient {
				insufficientCount++
			}
			cancelledCount += cancelled
			if hasError {
				errorCount++
			}
			mu.Unlock()
		}(info)
	}

	wg.Wait()
	return insufficientCount, cancelledCount, errorCount
}

// processWallet 处理单个钱包
func (j *BalanceScanJob) processWallet(ctx context.Context, info *WalletOrderInfo, thresholdMultiplier decimal.Decimal) (bool, int, bool) {
	// 获取链上余额
	onchainBalance, err := j.dataProvider.GetOnchainBalance(ctx, info.Wallet, info.Token)
	if err != nil {
		logger.Warn("failed to get onchain balance",
			zap.String("wallet", info.Wallet),
			zap.String("token", info.Token),
			zap.Error(err))
		return false, 0, true
	}

	// 计算阈值：冻结金额 * (1 - 阈值百分比)
	requiredBalance := info.TotalFrozen.Mul(thresholdMultiplier)

	// 如果链上余额足够，跳过
	if onchainBalance.GreaterThanOrEqual(requiredBalance) {
		return false, 0, false
	}

	// 余额不足，取消订单
	logger.Warn("insufficient balance detected, cancelling orders",
		zap.String("wallet", info.Wallet),
		zap.String("token", info.Token),
		zap.String("onchain_balance", onchainBalance.String()),
		zap.String("frozen_amount", info.TotalFrozen.String()),
		zap.Int("order_count", info.OrderCount))

	reason := fmt.Sprintf("Insufficient balance: onchain=%s, required=%s",
		onchainBalance.String(), info.TotalFrozen.String())

	// 批量取消订单
	cancelledCount, err := j.dataProvider.BatchCancelOrders(ctx, info.OrderIDs, reason)
	if err != nil {
		logger.Error("failed to cancel orders",
			zap.String("wallet", info.Wallet),
			zap.Strings("order_ids", info.OrderIDs),
			zap.Error(err))
		return true, 0, true
	}

	// 发送通知
	if j.config.EnableNotification {
		if err := j.dataProvider.SendCancelNotification(ctx, info.Wallet, info.OrderIDs, reason); err != nil {
			logger.Warn("failed to send cancel notification",
				zap.String("wallet", info.Wallet),
				zap.Error(err))
		}
	}

	// 发送告警
	if j.alertFunc != nil {
		alert := &BalanceScanAlert{
			Wallet:            info.Wallet,
			Token:             info.Token,
			OnchainBalance:    onchainBalance,
			FrozenAmount:      info.TotalFrozen,
			CancelledOrders:   cancelledCount,
			CancelledOrderIDs: info.OrderIDs,
			Timestamp:         time.Now().UnixMilli(),
		}
		if err := j.alertFunc(ctx, alert); err != nil {
			logger.Warn("failed to send alert",
				zap.String("wallet", info.Wallet),
				zap.Error(err))
		}
	}

	return true, cancelledCount, false
}

// MockBalanceScanDataProvider 模拟数据提供者（用于测试）
type MockBalanceScanDataProvider struct{}

func (p *MockBalanceScanDataProvider) GetWalletsWithActiveOrders(ctx context.Context, offset, limit int) ([]*WalletOrderInfo, error) {
	return nil, nil
}

func (p *MockBalanceScanDataProvider) GetOnchainBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (p *MockBalanceScanDataProvider) BatchCancelOrders(ctx context.Context, orderIDs []string, reason string) (int, error) {
	return len(orderIDs), nil
}

func (p *MockBalanceScanDataProvider) SendCancelNotification(ctx context.Context, wallet string, orderIDs []string, reason string) error {
	return nil
}
