// Package worker 提供后台任务处理
package worker

import (
	"context"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"go.uber.org/zap"
)

// OrderExpirer 订单过期处理接口
// 用于解耦 worker 和 service 包，避免循环依赖
type OrderExpirer interface {
	// ExpireOrder 过期指定订单
	ExpireOrder(ctx context.Context, orderID string) error
}

// OrderExpiryWorkerConfig 订单过期 Worker 配置
type OrderExpiryWorkerConfig struct {
	CheckInterval time.Duration // 检查间隔，默认 30s
	BatchSize     int           // 每批处理数量，默认 100
}

// DefaultOrderExpiryWorkerConfig 返回默认配置
func DefaultOrderExpiryWorkerConfig() *OrderExpiryWorkerConfig {
	return &OrderExpiryWorkerConfig{
		CheckInterval: 30 * time.Second,
		BatchSize:     100,
	}
}

// OrderExpiryWorker 订单过期处理 Worker
// 定期扫描已过期的 PENDING 订单，直接在 Redis 中取消
// 对于 OPEN/PARTIAL 订单，需要通过撮合引擎处理
type OrderExpiryWorker struct {
	cfg          *OrderExpiryWorkerConfig
	orderRepo    repository.OrderRepository
	orderExpirer OrderExpirer
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewOrderExpiryWorker 创建订单过期 Worker
func NewOrderExpiryWorker(
	cfg *OrderExpiryWorkerConfig,
	orderRepo repository.OrderRepository,
	orderExpirer OrderExpirer,
) *OrderExpiryWorker {
	if cfg == nil {
		cfg = DefaultOrderExpiryWorkerConfig()
	}
	return &OrderExpiryWorker{
		cfg:          cfg,
		orderRepo:    orderRepo,
		orderExpirer: orderExpirer,
	}
}

// Start 启动 Worker
func (w *OrderExpiryWorker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.checkLoop(ctx)

	logger.Info("order expiry worker started",
		zap.Duration("check_interval", w.cfg.CheckInterval),
		zap.Int("batch_size", w.cfg.BatchSize),
	)
}

// Stop 停止 Worker
func (w *OrderExpiryWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	logger.Info("order expiry worker stopped")
}

// checkLoop 检查循环
func (w *OrderExpiryWorker) checkLoop(ctx context.Context) {
	defer w.wg.Done()

	// 启动时立即执行一次
	w.processExpiredOrders(ctx)

	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processExpiredOrders(ctx)
		}
	}
}

// processExpiredOrders 处理过期订单
func (w *OrderExpiryWorker) processExpiredOrders(ctx context.Context) {
	now := time.Now().UnixMilli()

	// 查询已过期但未处理的订单
	// 只查询 PENDING 状态的订单 (OPEN/PARTIAL 需要通过撮合引擎处理)
	orders, err := w.orderRepo.ListExpiredOrders(ctx, now, model.OrderStatusPending, w.cfg.BatchSize)
	if err != nil {
		logger.Error("list expired pending orders failed", zap.Error(err))
		return
	}

	if len(orders) == 0 {
		return
	}

	logger.Info("found expired pending orders",
		zap.Int("count", len(orders)),
	)

	// 处理每个过期订单
	for _, order := range orders {
		if err := w.orderExpirer.ExpireOrder(ctx, order.OrderID); err != nil {
			logger.Error("expire pending order failed",
				zap.String("order_id", order.OrderID),
				zap.Error(err),
			)
			continue
		}

		logger.Debug("pending order expired",
			zap.String("order_id", order.OrderID),
			zap.String("wallet", order.Wallet),
			zap.Int64("expire_at", order.ExpireAt),
		)
	}
}
