// Package worker 提供后台任务处理
package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/alert"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// OutboxRelayConfig Outbox Relay 配置
type OutboxRelayConfig struct {
	RelayInterval    time.Duration // 轮询间隔，默认 100ms
	BatchSize        int           // 每批处理数量，默认 100
	CleanupInterval  time.Duration // 清理间隔，默认 1h
	Retention        time.Duration // 已发送消息保留时间，默认 24h
	RecoveryInterval time.Duration // 恢复卡住消息的间隔，默认 5m
	StaleThreshold   time.Duration // 消息被视为卡住的时间阈值，默认 5m
}

// DefaultOutboxRelayConfig 返回默认配置
func DefaultOutboxRelayConfig() *OutboxRelayConfig {
	return &OutboxRelayConfig{
		RelayInterval:    100 * time.Millisecond,
		BatchSize:        100,
		CleanupInterval:  time.Hour,
		Retention:        24 * time.Hour,
		RecoveryInterval: 5 * time.Minute,
		StaleThreshold:   5 * time.Minute,
	}
}

// OutboxRelay 负责将 outbox 消息发送到 Kafka
type OutboxRelay struct {
	cfg      *OutboxRelayConfig
	repo     *repository.OutboxRepository
	producer *kafka.Producer
	alerter  alert.Alerter
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewOutboxRelay 创建 Outbox Relay
func NewOutboxRelay(cfg *OutboxRelayConfig, repo *repository.OutboxRepository, producer *kafka.Producer, alerter alert.Alerter) *OutboxRelay {
	if cfg == nil {
		cfg = DefaultOutboxRelayConfig()
	}
	return &OutboxRelay{
		cfg:      cfg,
		repo:     repo,
		producer: producer,
		alerter:  alerter,
	}
}

// Start 启动 Outbox Relay
func (r *OutboxRelay) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)

	// 启动消息发送协程
	r.wg.Add(1)
	go r.relayLoop(ctx)

	// 启动清理协程
	r.wg.Add(1)
	go r.cleanupLoop(ctx)

	// 启动恢复协程 (恢复卡住的 processing 消息)
	r.wg.Add(1)
	go r.recoveryLoop(ctx)

	logger.Info("outbox relay started",
		"relay_interval", r.cfg.RelayInterval,
		"batch_size", r.cfg.BatchSize,
	)
}

// Stop 停止 Outbox Relay
func (r *OutboxRelay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	logger.Info("outbox relay stopped")
}

// relayLoop 消息发送循环
func (r *OutboxRelay) relayLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RelayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

// processBatch 处理一批消息
func (r *OutboxRelay) processBatch(ctx context.Context) {
	messages, err := r.repo.FetchPending(ctx, r.cfg.BatchSize)
	if err != nil {
		logger.Error("fetch pending messages failed", "error", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	for _, msg := range messages {
		if err := r.sendMessage(ctx, msg); err != nil {
			logger.Error("send message failed",
				"id", msg.ID,
				"message_id", msg.MessageID,
				"topic", msg.Topic,
				"error", err,
			)
			// 标记失败
			if markErr := r.repo.MarkFailed(ctx, msg.ID, err); markErr != nil {
				logger.Error("mark message failed error", "error", markErr)
			}
			continue
		}

		// 标记已发送
		if err := r.repo.MarkSent(ctx, msg.ID); err != nil {
			logger.Error("mark message sent error", "error", err)
		}
	}
}

// sendMessage 发送单条消息到 Kafka
func (r *OutboxRelay) sendMessage(ctx context.Context, msg *model.OutboxMessage) error {
	return r.producer.SendWithContext(ctx, msg.Topic, []byte(msg.PartitionKey), msg.Payload)
}

// cleanupLoop 清理循环
func (r *OutboxRelay) cleanupLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.cleanup(ctx)
			r.alertFailedMessages(ctx)
		}
	}
}

// cleanup 清理已发送的消息
func (r *OutboxRelay) cleanup(ctx context.Context) {
	beforeTime := time.Now().Add(-r.cfg.Retention).UnixMilli()
	deleted, err := r.repo.CleanSent(ctx, beforeTime, r.cfg.BatchSize)
	if err != nil {
		logger.Error("cleanup sent messages failed", "error", err)
		return
	}

	if deleted > 0 {
		logger.Info("cleaned up sent messages", "count", deleted)
	}
}

// alertFailedMessages 告警失败消息
func (r *OutboxRelay) alertFailedMessages(ctx context.Context) {
	count, err := r.repo.CountFailed(ctx)
	if err != nil {
		logger.Error("count failed messages error", "error", err)
		return
	}

	if count > 0 {
		logger.Warn("outbox has failed messages",
			"count", count,
		)
		// 发送告警通知
		if r.alerter != nil {
			r.alerter.SendAsync(ctx, &alert.Alert{
				Title:    "Outbox 消息发送失败",
				Message:  fmt.Sprintf("当前有 %d 条消息发送失败，请检查 Kafka 连接状态和消息内容", count),
				Severity: alert.SeverityWarning,
				Tags: map[string]string{
					"component":    "outbox_relay",
					"failed_count": fmt.Sprintf("%d", count),
				},
			})
		}
	}
}

// recoveryLoop 恢复卡住消息的循环
// 当实例崩溃时，processing 状态的消息需要被恢复到 pending 状态
func (r *OutboxRelay) recoveryLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.recoverStaleMessages(ctx)
		}
	}
}

// recoverStaleMessages 恢复卡住的消息
func (r *OutboxRelay) recoverStaleMessages(ctx context.Context) {
	recovered, err := r.repo.RecoverStaleProcessing(ctx, r.cfg.StaleThreshold)
	if err != nil {
		logger.Error("recover stale processing messages failed", "error", err)
		return
	}

	if recovered > 0 {
		logger.Info("recovered stale processing messages",
			"count", recovered,
			"threshold", r.cfg.StaleThreshold,
		)
	}
}
