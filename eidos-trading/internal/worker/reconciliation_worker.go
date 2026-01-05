// Package worker 提供后台任务处理
package worker

import (
	"context"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics" // Added metrics
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/redis/go-redis/v9" // Added redis
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

const (
	// Reconciliation lock key
	reconciliationLockKey = "eidos:trading:reconciliation:lock"
	// Lock TTL
	reconciliationLockTTL = 30 * time.Second
)

// ReconciliationWorkerConfig 对账 Worker 配置
type ReconciliationWorkerConfig struct {
	CheckInterval time.Duration   // 检查间隔，默认 5 分钟
	BatchSize     int             // 每批处理数量，默认 100
	Threshold     decimal.Decimal // 差异阈值，超过此值才报警
}

// DefaultReconciliationWorkerConfig 返回默认配置
func DefaultReconciliationWorkerConfig() *ReconciliationWorkerConfig {
	return &ReconciliationWorkerConfig{
		CheckInterval: 5 * time.Minute,
		BatchSize:     100,
		Threshold:     decimal.NewFromFloat(0.000001), // 忽略微小精度差异
	}
}

// ReconciliationWorker 对账 Worker
// 定期检查 Redis 余额与 DB 余额是否一致
// 如果发现差异，记录告警日志
type ReconciliationWorker struct {
	cfg          *ReconciliationWorkerConfig
	rdb          redis.UniversalClient // Added redis client
	balanceCache cache.BalanceRedisRepository
	balanceRepo  repository.BalanceRepository
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewReconciliationWorker 创建对账 Worker
func NewReconciliationWorker(
	cfg *ReconciliationWorkerConfig,
	rdb redis.UniversalClient, // Added redis client
	balanceCache cache.BalanceRedisRepository,
	balanceRepo repository.BalanceRepository,
) *ReconciliationWorker {
	if cfg == nil {
		cfg = DefaultReconciliationWorkerConfig()
	}
	return &ReconciliationWorker{
		cfg:          cfg,
		rdb:          rdb,
		balanceCache: balanceCache,
		balanceRepo:  balanceRepo,
	}
}

// Start 启动 Worker
func (w *ReconciliationWorker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.checkLoop(ctx)

	logger.Info("reconciliation worker started",
		zap.Duration("check_interval", w.cfg.CheckInterval),
		zap.Int("batch_size", w.cfg.BatchSize),
	)
}

// Stop 停止 Worker
func (w *ReconciliationWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	logger.Info("reconciliation worker stopped")
}

// checkLoop 检查循环
func (w *ReconciliationWorker) checkLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 获取分布式锁，防止多实例并发执行对账
			if w.tryAcquireLock(ctx) {
				logger.Info("acquired reconciliation lock, starting check")
				w.reconcile(ctx)
				// 对账时间可能很长，完成后释放锁 (或者可以让它自然过期，这里选择主动释放)
				w.releaseLock(ctx)
			}
		}
	}
}

// tryAcquireLock 尝试获取分布式锁
func (w *ReconciliationWorker) tryAcquireLock(ctx context.Context) bool {
	ok, err := w.rdb.SetNX(ctx, reconciliationLockKey, "locked", reconciliationLockTTL).Result()
	if err != nil {
		logger.Error("try acquire reconciliation lock failed", zap.Error(err))
		return false
	}
	return ok
}

// releaseLock 释放锁
func (w *ReconciliationWorker) releaseLock(ctx context.Context) {
	w.rdb.Del(ctx, reconciliationLockKey)
}

// reconcile 执行对账
func (w *ReconciliationWorker) reconcile(ctx context.Context) {
	startTime := time.Now()

	// 1. 获取所有活跃用户的余额 (分批处理)
	offset := 0
	totalChecked := 0
	discrepancies := 0

	for {
		// 从数据库获取一批余额记录
		balances, err := w.balanceRepo.ListBalances(ctx, offset, w.cfg.BatchSize)
		if err != nil {
			logger.Error("reconciliation: list balances failed", zap.Error(err))
			return
		}

		if len(balances) == 0 {
			break
		}

		// 检查每个余额
		for _, dbBalance := range balances {
			// 获取 Redis 中的余额
			redisBalance, err := w.balanceCache.GetBalance(ctx, dbBalance.Wallet, dbBalance.Token)
			if err != nil {
				// Redis 中没有记录，可能是正常情况 (用户从未操作过 Redis)
				continue
			}

			// 比较各字段
			if !w.isEqual(dbBalance.SettledAvailable, redisBalance.SettledAvailable) {
				discrepancies++
				logger.Warn("reconciliation: settled_available mismatch",
					zap.String("wallet", dbBalance.Wallet),
					zap.String("token", dbBalance.Token),
					zap.String("db", dbBalance.SettledAvailable.String()),
					zap.String("redis", redisBalance.SettledAvailable.String()),
				)
			}

			if !w.isEqual(dbBalance.SettledFrozen, redisBalance.SettledFrozen) {
				discrepancies++
				logger.Warn("reconciliation: settled_frozen mismatch",
					zap.String("wallet", dbBalance.Wallet),
					zap.String("token", dbBalance.Token),
					zap.String("db", dbBalance.SettledFrozen.String()),
					zap.String("redis", redisBalance.SettledFrozen.String()),
				)
			}

			if !w.isEqual(dbBalance.PendingAvailable, redisBalance.PendingAvailable) {
				discrepancies++
				logger.Warn("reconciliation: pending_available mismatch",
					zap.String("wallet", dbBalance.Wallet),
					zap.String("token", dbBalance.Token),
					zap.String("db", dbBalance.PendingAvailable.String()),
					zap.String("redis", redisBalance.PendingAvailable.String()),
				)
			}

			if !w.isEqual(dbBalance.PendingFrozen, redisBalance.PendingFrozen) {
				discrepancies++
				logger.Warn("reconciliation: pending_frozen mismatch",
					zap.String("wallet", dbBalance.Wallet),
					zap.String("token", dbBalance.Token),
					zap.String("db", dbBalance.PendingFrozen.String()),
					zap.String("redis", redisBalance.PendingFrozen.String()),
				)
			}

			if !w.isEqual(dbBalance.PendingTotal, redisBalance.PendingTotal) {
				discrepancies++
				logger.Warn("reconciliation: pending_total mismatch",
					zap.String("wallet", dbBalance.Wallet),
					zap.String("token", dbBalance.Token),
					zap.String("db", dbBalance.PendingTotal.String()),
					zap.String("redis", redisBalance.PendingTotal.String()),
				)
			}

			totalChecked++
		}

		offset += len(balances)

		// 如果返回的数量少于批大小，说明已经到末尾
		if len(balances) < w.cfg.BatchSize {
			break
		}
	}

	duration := time.Since(startTime)
	logger.Info("reconciliation completed",
		zap.Int("total_checked", totalChecked),
		zap.Int("discrepancies", discrepancies),
		zap.Duration("duration", duration),
	)

	// 如果有差异，可以发送告警 (metrics/alert)
	if discrepancies > 0 {
		metrics.RecordDataIntegrityCritical("reconciliation", "mismatch_found") // Fix arguments
		logger.Error("reconciliation: discrepancies found",
			zap.Int("count", discrepancies),
		)
	}
}

// isEqual 比较两个 Decimal 是否相等 (忽略微小差异)
func (w *ReconciliationWorker) isEqual(a, b decimal.Decimal) bool {
	diff := a.Sub(b).Abs()
	return diff.LessThanOrEqual(w.cfg.Threshold)
}
