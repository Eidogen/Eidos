// Package worker 提供后台任务处理
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
)

// Redis key patterns for cancel outbox
const (
	// Cancel Outbox key: eidos:trading:outbox:cancel:{order_id}
	cancelOutboxKeyPattern = "eidos:trading:outbox:cancel:%s"
	// Cancel Outbox pending list key: eidos:trading:outbox:cancel:pending:{shard_id}
	cancelOutboxPendingKeyPattern = "eidos:trading:outbox:cancel:pending:%d"
	// Cancel Shard lock key: eidos:trading:outbox:cancel:lock:{shard_id}
	cancelShardLockKeyPattern = "eidos:trading:outbox:cancel:lock:%d"
	// Recovery lock key
	cancelRecoveryLockKey = "eidos:trading:outbox:cancel:lock:recovery"
	// Cleanup lock key
	cancelCleanupLockKey = "eidos:trading:outbox:cancel:lock:cleanup"
)

// Cancel Outbox status constants
const (
	CancelOutboxStatusPending    = "PENDING"
	CancelOutboxStatusProcessing = "PROCESSING"
	CancelOutboxStatusSent       = "SENT"
	CancelOutboxStatusFailed     = "FAILED"
)

// CancelOutboxRelayConfig 取消订单 Outbox Relay 配置
type CancelOutboxRelayConfig struct {
	ShardCount        int           // 分片数量，默认 8
	RelayInterval     time.Duration // 轮询间隔，默认 50ms
	BatchSize         int           // 每批处理数量，默认 100
	ProcessTimeout    time.Duration // 单条处理超时，默认 5s
	RetryInterval     time.Duration // 重试间隔，默认 1s
	MaxRetries        int           // 最大重试次数，默认 3
	CleanupInterval   time.Duration // 清理间隔，默认 1h
	StaleThreshold    time.Duration // 卡住消息阈值，默认 5m
	RecoveryInterval  time.Duration // 恢复卡住消息间隔，默认 1m
	LockTTL           time.Duration // 分片锁 TTL，默认 30s
	LockRenewInterval time.Duration // 锁续期间隔，默认 10s
	InstanceID        string        // 实例 ID (用于锁持有者标识)
}

// DefaultCancelOutboxRelayConfig 返回默认配置
func DefaultCancelOutboxRelayConfig() *CancelOutboxRelayConfig {
	return &CancelOutboxRelayConfig{
		ShardCount:        8,
		RelayInterval:     50 * time.Millisecond,
		BatchSize:         100,
		ProcessTimeout:    5 * time.Second,
		RetryInterval:     time.Second,
		MaxRetries:        3,
		CleanupInterval:   time.Hour,
		StaleThreshold:    5 * time.Minute,
		RecoveryInterval:  time.Minute,
		LockTTL:           30 * time.Second,
		LockRenewInterval: 10 * time.Second,
		InstanceID:        fmt.Sprintf("cancel-instance-%d", time.Now().UnixNano()),
	}
}

// CancelOutboxRelay 负责将 Redis Cancel Outbox 消息发送到 Kafka
// 消费 UnfreezeForCancel Lua 脚本写入的 outbox 列表
// 支持多实例部署: 每个分片使用分布式锁，只有持锁实例才能消费
type CancelOutboxRelay struct {
	cfg      *CancelOutboxRelayConfig
	rdb      redis.UniversalClient
	producer *kafka.Producer
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// 分片锁状态
	shardLocks   map[int]bool // 当前实例持有的锁
	shardLocksMu sync.RWMutex

	// 统计
	mu             sync.RWMutex
	processedCount int64
	failedCount    int64
	lastProcessAt  time.Time
}

// NewCancelOutboxRelay 创建取消订单 Outbox Relay
func NewCancelOutboxRelay(cfg *CancelOutboxRelayConfig, rdb redis.UniversalClient, producer *kafka.Producer) *CancelOutboxRelay {
	if cfg == nil {
		cfg = DefaultCancelOutboxRelayConfig()
	}
	return &CancelOutboxRelay{
		cfg:        cfg,
		rdb:        rdb,
		producer:   producer,
		shardLocks: make(map[int]bool),
	}
}

// Start 启动 Cancel Outbox Relay
func (r *CancelOutboxRelay) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)

	// 为每个分片启动一个消费协程 (会尝试获取锁)
	for shardID := 0; shardID < r.cfg.ShardCount; shardID++ {
		r.wg.Add(1)
		go r.relayLoop(ctx, shardID)
	}

	// 启动恢复协程 (恢复卡住的 PROCESSING 消息)
	r.wg.Add(1)
	go r.recoveryLoop(ctx)

	// 启动清理协程
	r.wg.Add(1)
	go r.cleanupLoop(ctx)

	logger.Info("cancel outbox relay started",
		zap.Int("shard_count", r.cfg.ShardCount),
		zap.Duration("relay_interval", r.cfg.RelayInterval),
		zap.Int("batch_size", r.cfg.BatchSize),
		zap.String("instance_id", r.cfg.InstanceID),
	)
}

// Stop 停止 Cancel Outbox Relay
func (r *CancelOutboxRelay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()

	// 释放所有持有的锁
	r.releaseAllLocks(context.Background())

	logger.Info("cancel outbox relay stopped")
}

// Stats 获取统计信息
func (r *CancelOutboxRelay) Stats() (processed, failed int64, lastProcess time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.processedCount, r.failedCount, r.lastProcessAt
}

// relayLoop 单分片消息发送循环
// 使用分布式锁保证同一分片只有一个实例消费
func (r *CancelOutboxRelay) relayLoop(ctx context.Context, shardID int) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RelayInterval)
	defer ticker.Stop()

	lockRenewTicker := time.NewTicker(r.cfg.LockRenewInterval)
	defer lockRenewTicker.Stop()

	pendingKey := fmt.Sprintf(cancelOutboxPendingKeyPattern, shardID)
	lockKey := fmt.Sprintf(cancelShardLockKeyPattern, shardID)

	for {
		select {
		case <-ctx.Done():
			r.releaseLock(ctx, shardID, lockKey)
			return
		case <-lockRenewTicker.C:
			// 续期锁
			if r.hasLock(shardID) {
				if !r.renewLock(ctx, lockKey) {
					r.setLockStatus(shardID, false)
				}
			}
		case <-ticker.C:
			// 尝试获取锁
			if !r.hasLock(shardID) {
				if r.tryAcquireLock(ctx, lockKey) {
					r.setLockStatus(shardID, true)
					logger.Info("acquired cancel shard lock",
						zap.Int("shard_id", shardID),
						zap.String("instance_id", r.cfg.InstanceID),
					)
				}
			}

			// 只有持锁时才处理消息
			if r.hasLock(shardID) {
				r.processBatch(ctx, shardID, pendingKey)
			}
		}
	}
}

// tryAcquireLock 尝试获取分布式锁
func (r *CancelOutboxRelay) tryAcquireLock(ctx context.Context, lockKey string) bool {
	// 使用 SET NX EX 原子获取锁
	ok, err := r.rdb.SetNX(ctx, lockKey, r.cfg.InstanceID, r.cfg.LockTTL).Result()
	if err != nil {
		logger.Error("try acquire cancel lock failed", zap.String("key", lockKey), zap.Error(err))
		return false
	}
	return ok
}

// renewLock 续期锁
func (r *CancelOutboxRelay) renewLock(ctx context.Context, lockKey string) bool {
	// 使用 Lua 脚本确保只有锁持有者才能续期
	script := redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('PEXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`)

	result, err := script.Run(ctx, r.rdb, []string{lockKey}, r.cfg.InstanceID, r.cfg.LockTTL.Milliseconds()).Int()
	if err != nil {
		logger.Error("renew cancel lock failed", zap.String("key", lockKey), zap.Error(err))
		return false
	}
	return result == 1
}

// releaseLock 释放锁
func (r *CancelOutboxRelay) releaseLock(ctx context.Context, shardID int, lockKey string) {
	if !r.hasLock(shardID) {
		return
	}

	// 使用 Lua 脚本确保只有锁持有者才能释放
	script := redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`)

	if _, err := script.Run(ctx, r.rdb, []string{lockKey}, r.cfg.InstanceID).Result(); err != nil {
		logger.Error("release cancel lock failed", zap.String("key", lockKey), zap.Error(err))
	}

	r.setLockStatus(shardID, false)
	logger.Info("released cancel shard lock",
		zap.Int("shard_id", shardID),
		zap.String("instance_id", r.cfg.InstanceID),
	)
}

// releaseAllLocks 释放所有锁
func (r *CancelOutboxRelay) releaseAllLocks(ctx context.Context) {
	r.shardLocksMu.RLock()
	lockedShards := make([]int, 0)
	for shardID, locked := range r.shardLocks {
		if locked {
			lockedShards = append(lockedShards, shardID)
		}
	}
	r.shardLocksMu.RUnlock()

	for _, shardID := range lockedShards {
		lockKey := fmt.Sprintf(cancelShardLockKeyPattern, shardID)
		r.releaseLock(ctx, shardID, lockKey)
	}
}

// hasLock 检查是否持有锁
func (r *CancelOutboxRelay) hasLock(shardID int) bool {
	r.shardLocksMu.RLock()
	defer r.shardLocksMu.RUnlock()
	return r.shardLocks[shardID]
}

// setLockStatus 设置锁状态
func (r *CancelOutboxRelay) setLockStatus(shardID int, locked bool) {
	r.shardLocksMu.Lock()
	defer r.shardLocksMu.Unlock()
	r.shardLocks[shardID] = locked
}

// processBatch 处理一批消息
func (r *CancelOutboxRelay) processBatch(ctx context.Context, shardID int, pendingKey string) {
	// 使用 RPOP 从列表右侧弹出 (FIFO)
	for i := 0; i < r.cfg.BatchSize; i++ {
		// 弹出一个订单 ID
		orderID, err := r.rdb.RPop(ctx, pendingKey).Result()
		if err != nil {
			if err == redis.Nil {
				return // 队列为空
			}
			logger.Error("rpop cancel outbox failed",
				zap.Int("shard_id", shardID),
				zap.Error(err),
			)
			return
		}

		// 处理该取消请求
		if err := r.processCancel(ctx, orderID); err != nil {
			logger.Error("process cancel outbox failed",
				zap.String("order_id", orderID),
				zap.Error(err),
			)
			// 处理失败，重新放回队列左侧 (会在下一轮重试)
			if pushErr := r.rdb.LPush(ctx, pendingKey, orderID).Err(); pushErr != nil {
				logger.Error("lpush cancel back to outbox failed",
					zap.String("order_id", orderID),
					zap.Error(pushErr),
				)
			}

			r.mu.Lock()
			r.failedCount++
			r.mu.Unlock()
			continue
		}

		r.mu.Lock()
		r.processedCount++
		r.lastProcessAt = time.Now()
		r.mu.Unlock()
	}
}

// processCancel 处理单个取消请求
func (r *CancelOutboxRelay) processCancel(ctx context.Context, orderID string) error {
	outboxKey := fmt.Sprintf(cancelOutboxKeyPattern, orderID)

	// 1. 获取 outbox 记录
	result, err := r.rdb.HGetAll(ctx, outboxKey).Result()
	if err != nil {
		return fmt.Errorf("hgetall cancel outbox: %w", err)
	}
	if len(result) == 0 {
		// 记录不存在，可能已被处理
		logger.Warn("cancel outbox record not found, may already processed",
			zap.String("order_id", orderID))
		return nil
	}

	// 2. 检查状态
	status := result["status"]
	if status == CancelOutboxStatusSent {
		// 已发送，跳过
		return nil
	}

	// 3. 更新状态为 PROCESSING (使用 Lua 保证原子性)
	script := redis.NewScript(`
		local key = KEYS[1]
		local status = redis.call('HGET', key, 'status')
		if status == 'SENT' then
			return 0  -- 已发送，跳过
		end
		redis.call('HSET', key, 'status', 'PROCESSING', 'updated_at', ARGV[1])
		return 1
	`)

	updated, err := script.Run(ctx, r.rdb, []string{outboxKey}, time.Now().UnixMilli()).Int()
	if err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}
	if updated == 0 {
		return nil // 已被其他实例处理
	}

	// 4. 获取取消请求 JSON 并解析 Market
	cancelJSON := result["cancel_json"]
	if cancelJSON == "" {
		return fmt.Errorf("cancel_json is empty")
	}

	// 解析取消请求获取 Market (用作 Kafka partition key)
	var cancelMsg CancelMessage
	if err := json.Unmarshal([]byte(cancelJSON), &cancelMsg); err != nil {
		return fmt.Errorf("unmarshal cancel: %w", err)
	}

	// 5. 发送到 Kafka
	// 使用 Market 作为 partition key，保证同一交易对的取消请求有序
	if err := r.producer.SendWithContext(ctx, kafka.TopicCancelRequests, []byte(cancelMsg.Market), []byte(cancelJSON)); err != nil {
		// 发送失败，更新状态
		retryCount := 0
		if rc, ok := result["retry_count"]; ok {
			fmt.Sscanf(rc, "%d", &retryCount)
		}
		retryCount++

		newStatus := CancelOutboxStatusPending
		if retryCount >= r.cfg.MaxRetries {
			newStatus = CancelOutboxStatusFailed
		}

		r.rdb.HSet(ctx, outboxKey,
			"status", newStatus,
			"retry_count", retryCount,
			"last_error", err.Error(),
			"updated_at", time.Now().UnixMilli(),
		)
		return fmt.Errorf("send to kafka: %w", err)
	}

	// 6. 发送成功，更新状态
	if err := r.rdb.HSet(ctx, outboxKey,
		"status", CancelOutboxStatusSent,
		"sent_at", time.Now().UnixMilli(),
		"updated_at", time.Now().UnixMilli(),
	).Err(); err != nil {
		logger.Warn("update cancel outbox status to sent failed",
			zap.String("order_id", orderID),
			zap.Error(err))
	}

	logger.Debug("cancel request sent to matching engine",
		zap.String("order_id", orderID),
		zap.String("market", cancelMsg.Market),
	)

	return nil
}

// recoveryLoop 恢复卡住消息的循环
func (r *CancelOutboxRelay) recoveryLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 获取分布式锁
			if r.tryAcquireLock(ctx, cancelRecoveryLockKey) {
				logger.Debug("acquired cancel recovery lock")
				r.recoverStaleMessages(ctx)
				r.releaseLockSimple(ctx, cancelRecoveryLockKey)
			}
		}
	}
}

// recoverStaleMessages 恢复卡住的消息
func (r *CancelOutboxRelay) recoverStaleMessages(ctx context.Context) {
	// 使用 SCAN 遍历所有 cancel outbox 记录
	var cursor uint64
	pattern := "eidos:trading:outbox:cancel:O*" // 以 O 开头的是订单ID
	staleThreshold := time.Now().Add(-r.cfg.StaleThreshold).UnixMilli()

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			logger.Error("scan cancel outbox keys failed", zap.Error(err))
			return
		}

		for _, key := range keys {
			result, err := r.rdb.HMGet(ctx, key, "status", "updated_at", "shard").Result()
			if err != nil {
				continue
			}

			status, _ := result[0].(string)
			updatedAtStr, _ := result[1].(string)
			shardStr, _ := result[2].(string)

			if status != CancelOutboxStatusProcessing {
				continue
			}

			var updatedAt int64
			fmt.Sscanf(updatedAtStr, "%d", &updatedAt)

			if updatedAt > staleThreshold {
				continue // 还没超时
			}

			// 恢复到 PENDING 并重新入队
			var shardID int
			fmt.Sscanf(shardStr, "%d", &shardID)
			pendingKey := fmt.Sprintf(cancelOutboxPendingKeyPattern, shardID)

			// 提取 orderID
			orderID := key[len("eidos:trading:outbox:cancel:"):]

			// 使用 Lua 脚本原子更新状态并入队
			script := redis.NewScript(`
				local outbox_key = KEYS[1]
				local pending_key = KEYS[2]
				local order_id = ARGV[1]
				local now = ARGV[2]

				local status = redis.call('HGET', outbox_key, 'status')
				if status ~= 'PROCESSING' then
					return 0
				end

				redis.call('HSET', outbox_key, 'status', 'PENDING', 'updated_at', now)
				redis.call('LPUSH', pending_key, order_id)
				return 1
			`)

			recovered, err := script.Run(ctx, r.rdb, []string{key, pendingKey}, orderID, time.Now().UnixMilli()).Int()
			if err != nil {
				logger.Error("recover stale cancel failed",
					zap.String("order_id", orderID),
					zap.Error(err))
				continue
			}

			if recovered == 1 {
				logger.Info("recovered stale processing cancel",
					zap.String("order_id", orderID),
					zap.Int("shard_id", shardID),
				)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// cleanupLoop 清理循环
func (r *CancelOutboxRelay) cleanupLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 获取分布式锁
			if r.tryAcquireLock(ctx, cancelCleanupLockKey) {
				logger.Debug("acquired cancel cleanup lock")
				r.cleanupSentMessages(ctx)
				r.alertFailedMessages(ctx)
				r.releaseLockSimple(ctx, cancelCleanupLockKey)
			}
		}
	}
}

// releaseLockSimple 简单释放非分片锁
func (r *CancelOutboxRelay) releaseLockSimple(ctx context.Context, lockKey string) {
	// 使用 Lua 脚本确保只有锁持有者才能释放
	script := redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`)

	if _, err := script.Run(ctx, r.rdb, []string{lockKey}, r.cfg.InstanceID).Result(); err != nil {
		logger.Error("release simple lock failed", zap.String("key", lockKey), zap.Error(err))
	}
}

// cleanupSentMessages 清理已发送的消息
func (r *CancelOutboxRelay) cleanupSentMessages(ctx context.Context) {
	var cursor uint64
	pattern := "eidos:trading:outbox:cancel:O*"
	retention := 24 * time.Hour
	retentionThreshold := time.Now().Add(-retention).UnixMilli()
	deleted := 0

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			logger.Error("scan cancel outbox keys for cleanup failed", zap.Error(err))
			return
		}

		for _, key := range keys {
			result, err := r.rdb.HMGet(ctx, key, "status", "sent_at").Result()
			if err != nil {
				continue
			}

			status, _ := result[0].(string)
			sentAtStr, _ := result[1].(string)

			if status != CancelOutboxStatusSent {
				continue
			}

			var sentAt int64
			fmt.Sscanf(sentAtStr, "%d", &sentAt)

			if sentAt > retentionThreshold {
				continue // 还在保留期内
			}

			// 删除记录
			if err := r.rdb.Del(ctx, key).Err(); err != nil {
				logger.Error("delete sent cancel outbox record failed",
					zap.String("key", key),
					zap.Error(err))
				continue
			}
			deleted++
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if deleted > 0 {
		logger.Info("cleaned up sent cancel outbox records",
			zap.Int("count", deleted),
		)
	}
}

// alertFailedMessages 告警失败消息
func (r *CancelOutboxRelay) alertFailedMessages(ctx context.Context) {
	var cursor uint64
	pattern := "eidos:trading:outbox:cancel:O*"
	failedCount := 0

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return
		}

		for _, key := range keys {
			status, err := r.rdb.HGet(ctx, key, "status").Result()
			if err != nil {
				continue
			}
			if status == CancelOutboxStatusFailed {
				failedCount++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if failedCount > 0 {
		logger.Warn("cancel outbox has failed messages",
			zap.Int("count", failedCount),
		)
		// TODO: 发送告警通知
	}
}

// CancelMessage 取消请求消息结构 (用于 Kafka)
type CancelMessage struct {
	OrderID   string `json:"order_id"`
	Market    string `json:"market"`
	Wallet    string `json:"wallet"`
	Timestamp int64  `json:"timestamp"`
}

// ParseCancelMessage 解析取消消息
func ParseCancelMessage(data []byte) (*CancelMessage, error) {
	var msg CancelMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
