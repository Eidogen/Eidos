// Package service provides balance monitoring for insufficient balance order cancellation
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// Balance monitor errors
var (
	ErrMonitorNotRunning = errors.New("balance monitor is not running")
	ErrInvalidThreshold  = errors.New("invalid balance threshold")
)

// BalanceMonitorConfig 余额监控配置
type BalanceMonitorConfig struct {
	// CheckInterval 检查间隔
	CheckInterval time.Duration

	// BatchSize 每批处理的用户数量
	BatchSize int

	// BalanceThreshold 余额阈值 (低于此值时检查订单)
	// 设置为 0 表示只检查无法支付冻结金额的情况
	BalanceThreshold decimal.Decimal

	// MaxCancelPerBatch 每批最多取消的订单数
	MaxCancelPerBatch int

	// DryRun 干跑模式 (只记录不实际取消)
	DryRun bool
}

// DefaultBalanceMonitorConfig 默认配置
func DefaultBalanceMonitorConfig() *BalanceMonitorConfig {
	return &BalanceMonitorConfig{
		CheckInterval:     5 * time.Minute,
		BatchSize:         100,
		BalanceThreshold:  decimal.Zero,
		MaxCancelPerBatch: 50,
		DryRun:            false,
	}
}

// BalanceCheckResult 余额检查结果
type BalanceCheckResult struct {
	Wallet          string          `json:"wallet"`
	Token           string          `json:"token"`
	TotalFrozen     decimal.Decimal `json:"total_frozen"`
	AvailableAmount decimal.Decimal `json:"available_amount"`
	ShortfallAmount decimal.Decimal `json:"shortfall_amount"` // 缺口金额
	OrdersToCancel  []string        `json:"orders_to_cancel"`
	CancelledOrders []string        `json:"cancelled_orders"`
	FailedOrders    []string        `json:"failed_orders"`
}

// BalanceMonitorService 余额监控服务接口
type BalanceMonitorService interface {
	// CheckAndCancelInsufficientOrders 检查并取消余额不足的订单
	// 供 eidos-jobs 调用
	CheckAndCancelInsufficientOrders(ctx context.Context) ([]BalanceCheckResult, error)

	// CheckWalletBalance 检查指定钱包的余额
	CheckWalletBalance(ctx context.Context, wallet, token string) (*BalanceCheckResult, error)

	// GetInsufficientBalanceWallets 获取余额不足的钱包列表
	GetInsufficientBalanceWallets(ctx context.Context, limit int) ([]string, error)

	// Start 启动后台监控 (可选)
	Start(ctx context.Context)

	// Stop 停止后台监控
	Stop()

	// IsRunning 检查监控是否运行中
	IsRunning() bool
}

// balanceMonitorService 余额监控服务实现
type balanceMonitorService struct {
	cfg          *BalanceMonitorConfig
	orderRepo    repository.OrderRepository
	balanceCache cache.BalanceRedisRepository
	orderService OrderService
	marketConfig MarketConfigProvider

	running    bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
}

// NewBalanceMonitorService 创建余额监控服务
func NewBalanceMonitorService(
	cfg *BalanceMonitorConfig,
	orderRepo repository.OrderRepository,
	balanceCache cache.BalanceRedisRepository,
	orderService OrderService,
	marketConfig MarketConfigProvider,
) BalanceMonitorService {
	if cfg == nil {
		cfg = DefaultBalanceMonitorConfig()
	}
	return &balanceMonitorService{
		cfg:          cfg,
		orderRepo:    orderRepo,
		balanceCache: balanceCache,
		orderService: orderService,
		marketConfig: marketConfig,
	}
}

// CheckAndCancelInsufficientOrders 检查并取消余额不足的订单
func (s *balanceMonitorService) CheckAndCancelInsufficientOrders(ctx context.Context) ([]BalanceCheckResult, error) {
	startTime := time.Now()
	logger.Info("starting balance check for insufficient orders")

	// 1. 获取所有活跃订单
	openOrders, err := s.orderRepo.ListOpenOrders(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("list open orders: %w", err)
	}

	if len(openOrders) == 0 {
		logger.Info("no open orders to check")
		return nil, nil
	}

	// 2. 按钱包+代币分组统计冻结金额
	type walletToken struct {
		wallet string
		token  string
	}
	frozenByWalletToken := make(map[walletToken]decimal.Decimal)
	ordersByWalletToken := make(map[walletToken][]*model.Order)

	for _, order := range openOrders {
		key := walletToken{wallet: order.Wallet, token: order.FreezeToken}
		frozenByWalletToken[key] = frozenByWalletToken[key].Add(order.FreezeAmount)
		ordersByWalletToken[key] = append(ordersByWalletToken[key], order)
	}

	// 3. 检查每个钱包+代币组合的余额
	var results []BalanceCheckResult
	totalCancelled := 0
	totalFailed := 0

	for key, totalFrozen := range frozenByWalletToken {
		// 获取实际余额
		balance, err := s.balanceCache.GetBalance(ctx, key.wallet, key.token)
		if err != nil {
			if errors.Is(err, cache.ErrRedisBalanceNotFound) {
				// 余额不存在，需要取消所有订单
				logger.Warn("balance not found for wallet with open orders",
					"wallet", key.wallet,
					"token", key.token,
					"order_count", len(ordersByWalletToken[key]))
			} else {
				logger.Error("get balance failed",
					"wallet", key.wallet,
					"token", key.token,
					"error", err)
				continue
			}
		}

		// 计算可用余额 (已结算 + 待结算可用)
		var availableAmount decimal.Decimal
		if balance != nil {
			availableAmount = balance.TotalAvailable().Add(balance.SettledFrozen).Add(balance.PendingFrozen)
		}

		// 检查是否有缺口
		shortfall := totalFrozen.Sub(availableAmount)
		if shortfall.LessThanOrEqual(decimal.Zero) {
			continue // 余额充足
		}

		// 需要取消部分订单以释放冻结
		orders := ordersByWalletToken[key]
		result := s.processInsufficientBalance(ctx, key.wallet, key.token, totalFrozen, availableAmount, shortfall, orders)
		results = append(results, result)
		totalCancelled += len(result.CancelledOrders)
		totalFailed += len(result.FailedOrders)
	}

	duration := time.Since(startTime)
	logger.Info("balance check completed",
		"wallets_with_issues", len(results),
		"orders_cancelled", totalCancelled,
		"orders_failed", totalFailed,
		"duration", duration)

	metrics.RecordBalanceMonitorRun(len(results), totalCancelled)

	return results, nil
}

// processInsufficientBalance 处理余额不足的情况
func (s *balanceMonitorService) processInsufficientBalance(
	ctx context.Context,
	wallet, token string,
	totalFrozen, availableAmount, shortfall decimal.Decimal,
	orders []*model.Order,
) BalanceCheckResult {
	result := BalanceCheckResult{
		Wallet:          wallet,
		Token:           token,
		TotalFrozen:     totalFrozen,
		AvailableAmount: availableAmount,
		ShortfallAmount: shortfall,
	}

	logger.Warn("insufficient balance detected",
		"wallet", wallet,
		"token", token,
		"total_frozen", totalFrozen.String(),
		"available", availableAmount.String(),
		"shortfall", shortfall.String(),
		"order_count", len(orders))

	// 按创建时间排序 (FIFO 取消最新的订单)
	// 在实际场景中可能需要考虑其他因素如订单价值、用户等级等
	sortOrdersByCreatedAt(orders)

	// 计算需要取消多少订单才能覆盖缺口
	var toCancel []*model.Order
	cumulativeFree := decimal.Zero

	for i := len(orders) - 1; i >= 0 && cumulativeFree.LessThan(shortfall); i-- {
		toCancel = append(toCancel, orders[i])
		cumulativeFree = cumulativeFree.Add(orders[i].FreezeAmount)
	}

	// 限制每批取消数量
	if len(toCancel) > s.cfg.MaxCancelPerBatch {
		toCancel = toCancel[:s.cfg.MaxCancelPerBatch]
	}

	// 记录需要取消的订单
	for _, order := range toCancel {
		result.OrdersToCancel = append(result.OrdersToCancel, order.OrderID)
	}

	// 干跑模式不实际取消
	if s.cfg.DryRun {
		logger.Info("dry run: would cancel orders",
			"wallet", wallet,
			"token", token,
			"orders", result.OrdersToCancel)
		return result
	}

	// 取消订单
	for _, order := range toCancel {
		err := s.orderService.CancelOrder(ctx, wallet, order.OrderID)
		if err != nil {
			logger.Error("cancel order failed",
				"wallet", wallet,
				"order_id", order.OrderID,
				"error", err)
			result.FailedOrders = append(result.FailedOrders, order.OrderID)
		} else {
			logger.Info("order cancelled due to insufficient balance",
				"wallet", wallet,
				"order_id", order.OrderID,
				"freeze_amount", order.FreezeAmount.String())
			result.CancelledOrders = append(result.CancelledOrders, order.OrderID)
		}
	}

	return result
}

// sortOrdersByCreatedAt 按创建时间排序 (升序)
func sortOrdersByCreatedAt(orders []*model.Order) {
	// 简单冒泡排序 (订单数通常不多)
	for i := 0; i < len(orders)-1; i++ {
		for j := i + 1; j < len(orders); j++ {
			if orders[i].CreatedAt > orders[j].CreatedAt {
				orders[i], orders[j] = orders[j], orders[i]
			}
		}
	}
}

// CheckWalletBalance 检查指定钱包的余额
func (s *balanceMonitorService) CheckWalletBalance(ctx context.Context, wallet, token string) (*BalanceCheckResult, error) {
	// 获取该钱包+代币的所有活跃订单
	orders, err := s.orderRepo.ListOpenOrders(ctx, wallet, "")
	if err != nil {
		return nil, fmt.Errorf("list open orders: %w", err)
	}

	// 筛选指定代币的订单
	var tokenOrders []*model.Order
	var totalFrozen decimal.Decimal
	for _, order := range orders {
		if order.FreezeToken == token {
			tokenOrders = append(tokenOrders, order)
			totalFrozen = totalFrozen.Add(order.FreezeAmount)
		}
	}

	// 获取余额
	balance, err := s.balanceCache.GetBalance(ctx, wallet, token)
	if err != nil && !errors.Is(err, cache.ErrRedisBalanceNotFound) {
		return nil, fmt.Errorf("get balance: %w", err)
	}

	var availableAmount decimal.Decimal
	if balance != nil {
		availableAmount = balance.TotalAvailable().Add(balance.SettledFrozen).Add(balance.PendingFrozen)
	}

	shortfall := totalFrozen.Sub(availableAmount)
	if shortfall.LessThan(decimal.Zero) {
		shortfall = decimal.Zero
	}

	result := &BalanceCheckResult{
		Wallet:          wallet,
		Token:           token,
		TotalFrozen:     totalFrozen,
		AvailableAmount: availableAmount,
		ShortfallAmount: shortfall,
	}

	// 如果有缺口，列出需要取消的订单
	if shortfall.GreaterThan(decimal.Zero) {
		sortOrdersByCreatedAt(tokenOrders)
		cumulativeFree := decimal.Zero
		for i := len(tokenOrders) - 1; i >= 0 && cumulativeFree.LessThan(shortfall); i-- {
			result.OrdersToCancel = append(result.OrdersToCancel, tokenOrders[i].OrderID)
			cumulativeFree = cumulativeFree.Add(tokenOrders[i].FreezeAmount)
		}
	}

	return result, nil
}

// GetInsufficientBalanceWallets 获取余额不足的钱包列表
func (s *balanceMonitorService) GetInsufficientBalanceWallets(ctx context.Context, limit int) ([]string, error) {
	// 获取所有活跃订单
	orders, err := s.orderRepo.ListOpenOrders(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("list open orders: %w", err)
	}

	// 按钱包分组统计
	walletTokens := make(map[string]map[string]decimal.Decimal)
	for _, order := range orders {
		if _, ok := walletTokens[order.Wallet]; !ok {
			walletTokens[order.Wallet] = make(map[string]decimal.Decimal)
		}
		walletTokens[order.Wallet][order.FreezeToken] = walletTokens[order.Wallet][order.FreezeToken].Add(order.FreezeAmount)
	}

	// 检查每个钱包的余额
	var insufficientWallets []string
	for wallet, tokens := range walletTokens {
		for token, frozen := range tokens {
			balance, err := s.balanceCache.GetBalance(ctx, wallet, token)
			if err != nil {
				if errors.Is(err, cache.ErrRedisBalanceNotFound) {
					// 余额不存在，肯定不足
					insufficientWallets = append(insufficientWallets, wallet)
					break
				}
				continue
			}

			available := balance.TotalAvailable().Add(balance.SettledFrozen).Add(balance.PendingFrozen)
			if frozen.GreaterThan(available) {
				insufficientWallets = append(insufficientWallets, wallet)
				break
			}
		}

		if len(insufficientWallets) >= limit {
			break
		}
	}

	return insufficientWallets, nil
}

// Start 启动后台监控
func (s *balanceMonitorService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runLoop(ctx)
	}()

	logger.Info("balance monitor service started",
		"check_interval", s.cfg.CheckInterval,
		"batch_size", s.cfg.BatchSize,
		"dry_run", s.cfg.DryRun)
}

// runLoop 后台监控循环
func (s *balanceMonitorService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := s.CheckAndCancelInsufficientOrders(ctx)
			if err != nil {
				logger.Error("balance monitor check failed", "error", err)
			}
		}
	}
}

// Stop 停止后台监控
func (s *balanceMonitorService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	logger.Info("balance monitor service stopped")
}

// IsRunning 检查监控是否运行中
func (s *balanceMonitorService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
