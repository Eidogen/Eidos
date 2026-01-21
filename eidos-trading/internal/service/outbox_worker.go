package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

const (
	// 默认批次大小
	defaultBatchSize = 100
	// 默认轮询间隔
	defaultPollInterval = 100 * time.Millisecond
	// 空闲时轮询间隔
	idlePollInterval = 500 * time.Millisecond
	// 处理超时
	processTimeout = 10 * time.Second
	// 清理间隔
	cleanInterval = 1 * time.Hour
	// 保留已处理消息时间
	retentionPeriod = 24 * time.Hour
)

// OutboxWorker Outbox 消息处理 Worker
type OutboxWorker interface {
	// Start 启动 Worker
	Start(ctx context.Context) error
	// Stop 停止 Worker
	Stop() error
	// Stats 获取统计信息
	Stats() OutboxWorkerStats
}

// OutboxWorkerStats Worker 统计信息
type OutboxWorkerStats struct {
	ProcessedCount int64
	FailedCount    int64
	LastProcessAt  time.Time
	IsRunning      bool
}

// outboxWorker Outbox Worker 实现
type outboxWorker struct {
	outboxRepo  *repository.OutboxRepository
	balanceRepo repository.BalanceRepository

	batchSize    int
	pollInterval time.Duration

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup // 等待 goroutine 退出
	stats   OutboxWorkerStats
}

// OutboxWorkerConfig Worker 配置
type OutboxWorkerConfig struct {
	BatchSize    int
	PollInterval time.Duration
}

// NewOutboxWorker 创建 Outbox Worker
func NewOutboxWorker(
	outboxRepo *repository.OutboxRepository,
	balanceRepo repository.BalanceRepository,
	cfg *OutboxWorkerConfig,
) OutboxWorker {
	batchSize := defaultBatchSize
	pollInterval := defaultPollInterval

	if cfg != nil {
		if cfg.BatchSize > 0 {
			batchSize = cfg.BatchSize
		}
		if cfg.PollInterval > 0 {
			pollInterval = cfg.PollInterval
		}
	}

	return &outboxWorker{
		outboxRepo:   outboxRepo,
		balanceRepo:  balanceRepo,
		batchSize:    batchSize,
		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
	}
}

// Start 启动 Worker
func (w *outboxWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("outbox worker already running")
	}
	w.running = true
	w.stats.IsRunning = true
	w.mu.Unlock()

	logger.Info("outbox worker started",
		"batchSize", w.batchSize,
		"pollInterval", w.pollInterval)

	// 启动 goroutines
	w.wg.Add(2)

	// 主处理循环
	go w.processLoop(ctx)

	// 清理循环
	go w.cleanLoop(ctx)

	return nil
}

// Stop 停止 Worker
func (w *outboxWorker) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}

	close(w.stopCh)
	w.running = false
	w.stats.IsRunning = false
	w.mu.Unlock()

	// 等待所有 goroutines 退出
	w.wg.Wait()

	logger.Info("outbox worker stopped")
	return nil
}

// Stats 获取统计信息
func (w *outboxWorker) Stats() OutboxWorkerStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.stats
}

// processLoop 主处理循环
func (w *outboxWorker) processLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			processed := w.processBatch(ctx)
			if processed == 0 {
				// 空闲时降低轮询频率
				ticker.Reset(idlePollInterval)
			} else {
				ticker.Reset(w.pollInterval)
			}
		}
	}
}

// cleanLoop 清理循环
func (w *outboxWorker) cleanLoop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(cleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.cleanOldMessages(ctx)
		}
	}
}

// processBatch 处理一批消息
func (w *outboxWorker) processBatch(ctx context.Context) int {
	processCtx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()

	messages, err := w.outboxRepo.FetchPending(processCtx, w.batchSize)
	if err != nil {
		logger.Error("fetch pending outbox messages failed", "error", err)
		return 0
	}

	if len(messages) == 0 {
		return 0
	}

	processed := 0
	for _, msg := range messages {
		if err := w.processMessage(processCtx, msg); err != nil {
			logger.Error("process outbox message failed",
				"id", msg.ID,
				"messageID", msg.MessageID,
				"topic", msg.Topic,
				"error", err)

			if markErr := w.outboxRepo.MarkFailed(processCtx, msg.ID, err); markErr != nil {
				logger.Error("mark outbox message failed", "error", markErr)
			}

			w.mu.Lock()
			w.stats.FailedCount++
			w.mu.Unlock()
		} else {
			if markErr := w.outboxRepo.MarkSent(processCtx, msg.ID); markErr != nil {
				logger.Error("mark outbox message sent failed", "error", markErr)
			}

			w.mu.Lock()
			w.stats.ProcessedCount++
			w.stats.LastProcessAt = time.Now()
			w.mu.Unlock()
			processed++
		}
	}

	return processed
}

// processMessage 处理单条消息
func (w *outboxWorker) processMessage(ctx context.Context, msg *model.OutboxMessage) error {
	switch msg.Topic {
	case model.TopicBalanceUpdate:
		return w.processBalanceUpdate(ctx, msg)
	case model.TopicBalanceLog:
		return w.processBalanceLog(ctx, msg)
	default:
		// 对于 Kafka 消息类型，直接标记成功 (由 Kafka producer 处理)
		return nil
	}
}

// processBalanceUpdate 处理余额更新
func (w *outboxWorker) processBalanceUpdate(ctx context.Context, msg *model.OutboxMessage) error {
	var payload model.BalanceUpdatePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal balance update payload: %w", err)
	}

	settledAvailable, err := decimal.NewFromString(payload.SettledAvailable)
	if err != nil {
		return fmt.Errorf("parse settled_available: %w", err)
	}
	settledFrozen, err := decimal.NewFromString(payload.SettledFrozen)
	if err != nil {
		return fmt.Errorf("parse settled_frozen: %w", err)
	}
	pendingAvailable, err := decimal.NewFromString(payload.PendingAvailable)
	if err != nil {
		return fmt.Errorf("parse pending_available: %w", err)
	}
	pendingFrozen, err := decimal.NewFromString(payload.PendingFrozen)
	if err != nil {
		return fmt.Errorf("parse pending_frozen: %w", err)
	}
	pendingTotal, err := decimal.NewFromString(payload.PendingTotal)
	if err != nil {
		return fmt.Errorf("parse pending_total: %w", err)
	}

	balance := &model.Balance{
		Wallet:           payload.Wallet,
		Token:            payload.Token,
		SettledAvailable: settledAvailable,
		SettledFrozen:    settledFrozen,
		PendingAvailable: pendingAvailable,
		PendingFrozen:    pendingFrozen,
		PendingTotal:     pendingTotal,
		Version:          payload.Version,
		UpdatedAt:        time.Now().UnixMilli(),
	}

	return w.balanceRepo.UpsertBalance(ctx, balance)
}

// processBalanceLog 处理余额流水
func (w *outboxWorker) processBalanceLog(ctx context.Context, msg *model.OutboxMessage) error {
	var payload model.BalanceLogPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal balance log payload: %w", err)
	}

	amount, err := decimal.NewFromString(payload.Amount)
	if err != nil {
		return fmt.Errorf("parse amount: %w", err)
	}
	balanceBefore, err := decimal.NewFromString(payload.BalanceBefore)
	if err != nil {
		return fmt.Errorf("parse balance_before: %w", err)
	}
	balanceAfter, err := decimal.NewFromString(payload.BalanceAfter)
	if err != nil {
		return fmt.Errorf("parse balance_after: %w", err)
	}

	balanceLog := &model.BalanceLog{
		Wallet:        payload.Wallet,
		Token:         payload.Token,
		Type:          model.BalanceLogType(payload.Type),
		Amount:        amount,
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceAfter,
		OrderID:       payload.OrderID,
		TradeID:       payload.TradeID,
		TxHash:        payload.TxHash,
		Remark:        payload.Remark,
		CreatedAt:     time.Now().UnixMilli(),
	}

	return w.balanceRepo.CreateBalanceLog(ctx, balanceLog)
}

// cleanOldMessages 清理旧消息
func (w *outboxWorker) cleanOldMessages(ctx context.Context) {
	cleanCtx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()

	beforeTime := time.Now().Add(-retentionPeriod).UnixMilli()
	deleted, err := w.outboxRepo.CleanSent(cleanCtx, beforeTime, 1000)
	if err != nil {
		logger.Error("clean old outbox messages failed", "error", err)
		return
	}

	if deleted > 0 {
		logger.Info("cleaned old outbox messages", "deleted", deleted)
	}
}
