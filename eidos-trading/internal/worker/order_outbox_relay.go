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
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// Redis key patterns for order outbox
const (
	// Outbox order key: eidos:trading:outbox:order:{order_id}
	outboxOrderKeyPattern = "eidos:trading:outbox:order:%s"
	// Outbox pending list key: eidos:trading:outbox:pending:{shard_id}
	outboxPendingKeyPattern = "eidos:trading:outbox:pending:%d"
	// Shard lock key: eidos:trading:outbox:lock:{shard_id}
	shardLockKeyPattern = "eidos:trading:outbox:lock:%d"
	// Recovery lock key: eidos:trading:outbox:lock:recovery
	recoveryLockKey = "eidos:trading:outbox:lock:recovery"
	// Cleanup lock key: eidos:trading:outbox:lock:cleanup
	cleanupLockKey = "eidos:trading:outbox:lock:cleanup"
)

// Outbox status constants
const (
	OutboxStatusPending    = "PENDING"
	OutboxStatusProcessing = "PROCESSING"
	OutboxStatusSent       = "SENT"
	OutboxStatusFailed     = "FAILED"
)

// OrderOutboxRelayConfig 订单 Outbox Relay 配置
type OrderOutboxRelayConfig struct {
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

// DefaultOrderOutboxRelayConfig 返回默认配置
func DefaultOrderOutboxRelayConfig() *OrderOutboxRelayConfig {
	return &OrderOutboxRelayConfig{
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
		InstanceID:        fmt.Sprintf("instance-%d", time.Now().UnixNano()),
	}
}

// OrderOutboxRelay 负责将 Redis Order Outbox 消息发送到 Kafka
// 消费 FreezeForOrder Lua 脚本写入的 outbox 列表
// 支持多实例部署: 每个分片使用分布式锁，只有持锁实例才能消费
type OrderOutboxRelay struct {
	cfg      *OrderOutboxRelayConfig
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

// NewOrderOutboxRelay 创建订单 Outbox Relay
func NewOrderOutboxRelay(cfg *OrderOutboxRelayConfig, rdb redis.UniversalClient, producer *kafka.Producer) *OrderOutboxRelay {
	if cfg == nil {
		cfg = DefaultOrderOutboxRelayConfig()
	}
	return &OrderOutboxRelay{
		cfg:        cfg,
		rdb:        rdb,
		producer:   producer,
		shardLocks: make(map[int]bool),
	}
}

// Start 启动 Order Outbox Relay
func (r *OrderOutboxRelay) Start(ctx context.Context) {
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

	logger.Info("order outbox relay started",
		zap.Int("shard_count", r.cfg.ShardCount),
		zap.Duration("relay_interval", r.cfg.RelayInterval),
		zap.Int("batch_size", r.cfg.BatchSize),
		zap.String("instance_id", r.cfg.InstanceID),
	)
}

// Stop 停止 Order Outbox Relay
func (r *OrderOutboxRelay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()

	// 释放所有持有的锁
	r.releaseAllLocks(context.Background())

	logger.Info("order outbox relay stopped")
}

// Stats 获取统计信息
func (r *OrderOutboxRelay) Stats() (processed, failed int64, lastProcess time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.processedCount, r.failedCount, r.lastProcessAt
}

// relayLoop 单分片消息发送循环
// 使用分布式锁保证同一分片只有一个实例消费
func (r *OrderOutboxRelay) relayLoop(ctx context.Context, shardID int) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RelayInterval)
	defer ticker.Stop()

	lockRenewTicker := time.NewTicker(r.cfg.LockRenewInterval)
	defer lockRenewTicker.Stop()

	pendingKey := fmt.Sprintf(outboxPendingKeyPattern, shardID)
	lockKey := fmt.Sprintf(shardLockKeyPattern, shardID)

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
					logger.Info("acquired shard lock",
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
func (r *OrderOutboxRelay) tryAcquireLock(ctx context.Context, lockKey string) bool {
	// 使用 SET NX EX 原子获取锁
	ok, err := r.rdb.SetNX(ctx, lockKey, r.cfg.InstanceID, r.cfg.LockTTL).Result()
	if err != nil {
		logger.Error("try acquire lock failed", zap.String("key", lockKey), zap.Error(err))
		return false
	}
	return ok
}

// renewLock 续期锁
func (r *OrderOutboxRelay) renewLock(ctx context.Context, lockKey string) bool {
	// 使用 Lua 脚本确保只有锁持有者才能续期
	script := redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('PEXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`)

	result, err := script.Run(ctx, r.rdb, []string{lockKey}, r.cfg.InstanceID, r.cfg.LockTTL.Milliseconds()).Int()
	if err != nil {
		logger.Error("renew lock failed", zap.String("key", lockKey), zap.Error(err))
		return false
	}
	return result == 1
}

// releaseLock 释放锁
func (r *OrderOutboxRelay) releaseLock(ctx context.Context, shardID int, lockKey string) {
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
		logger.Error("release lock failed", zap.String("key", lockKey), zap.Error(err))
	}

	r.setLockStatus(shardID, false)
	logger.Info("released shard lock",
		zap.Int("shard_id", shardID),
		zap.String("instance_id", r.cfg.InstanceID),
	)
}

// releaseAllLocks 释放所有锁
func (r *OrderOutboxRelay) releaseAllLocks(ctx context.Context) {
	r.shardLocksMu.RLock()
	lockedShards := make([]int, 0)
	for shardID, locked := range r.shardLocks {
		if locked {
			lockedShards = append(lockedShards, shardID)
		}
	}
	r.shardLocksMu.RUnlock()

	for _, shardID := range lockedShards {
		lockKey := fmt.Sprintf(shardLockKeyPattern, shardID)
		r.releaseLock(ctx, shardID, lockKey)
	}
}

// hasLock 检查是否持有锁
func (r *OrderOutboxRelay) hasLock(shardID int) bool {
	r.shardLocksMu.RLock()
	defer r.shardLocksMu.RUnlock()
	return r.shardLocks[shardID]
}

// setLockStatus 设置锁状态
func (r *OrderOutboxRelay) setLockStatus(shardID int, locked bool) {
	r.shardLocksMu.Lock()
	defer r.shardLocksMu.Unlock()
	r.shardLocks[shardID] = locked
}

// processBatch 处理一批消息
func (r *OrderOutboxRelay) processBatch(ctx context.Context, shardID int, pendingKey string) {
	// 使用 RPOP 从列表右侧弹出 (FIFO)
	// 注意: FreezeForOrder 使用 LPUSH，所以这里用 RPOP 保证顺序
	for i := 0; i < r.cfg.BatchSize; i++ {
		// 弹出一个订单 ID
		orderID, err := r.rdb.RPop(ctx, pendingKey).Result()
		if err != nil {
			if err == redis.Nil {
				return // 队列为空
			}
			logger.Error("rpop order outbox failed",
				zap.Int("shard_id", shardID),
				zap.Error(err),
			)
			return
		}

		// 处理该订单
		if err := r.processOrder(ctx, orderID); err != nil {
			logger.Error("process order outbox failed",
				zap.String("order_id", orderID),
				zap.Error(err),
			)
			// 处理失败，重新放回队列左侧 (会在下一轮重试)
			if pushErr := r.rdb.LPush(ctx, pendingKey, orderID).Err(); pushErr != nil {
				logger.Error("lpush order back to outbox failed",
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

// processOrder 处理单个订单
func (r *OrderOutboxRelay) processOrder(ctx context.Context, orderID string) error {
	outboxKey := fmt.Sprintf(outboxOrderKeyPattern, orderID)

	// 1. 获取 outbox 记录
	result, err := r.rdb.HGetAll(ctx, outboxKey).Result()
	if err != nil {
		return fmt.Errorf("hgetall outbox: %w", err)
	}
	if len(result) == 0 {
		// 记录不存在，可能已被处理
		logger.Warn("order outbox record not found, may already processed",
			zap.String("order_id", orderID))
		return nil
	}

	// 2. 检查状态
	status := result["status"]
	if status == OutboxStatusSent {
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

	// 4. 获取订单 JSON 并解析 Market
	orderJSON := result["order_json"]
	if orderJSON == "" {
		return fmt.Errorf("order_json is empty")
	}

	// 解析订单获取 Market (用作 Kafka partition key)
	var orderMsg OrderMessage
	if err := json.Unmarshal([]byte(orderJSON), &orderMsg); err != nil {
		return fmt.Errorf("unmarshal order: %w", err)
	}

	// 5. 发送到 Kafka
	// 使用 Market 作为 partition key，保证同一交易对的订单有序
	if err := r.producer.SendWithContext(ctx, kafka.TopicOrders, []byte(orderMsg.Market), []byte(orderJSON)); err != nil {
		// 发送失败，更新状态
		retryCount := 0
		if rc, ok := result["retry_count"]; ok {
			fmt.Sscanf(rc, "%d", &retryCount)
		}
		retryCount++

		newStatus := OutboxStatusPending
		if retryCount >= r.cfg.MaxRetries {
			newStatus = OutboxStatusFailed
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
		"status", OutboxStatusSent,
		"sent_at", time.Now().UnixMilli(),
		"updated_at", time.Now().UnixMilli(),
	).Err(); err != nil {
		logger.Warn("update outbox status to sent failed",
			zap.String("order_id", orderID),
			zap.Error(err))
	}

	logger.Debug("order sent to matching engine",
		zap.String("order_id", orderID),
		zap.String("market", orderMsg.Market),
	)

	return nil
}

// recoveryLoop 恢复卡住消息的循环
// 当实例崩溃时，PROCESSING 状态的消息需要被恢复到 pending 队列
// 增加分布式锁，避免多实例并发全表扫描
func (r *OrderOutboxRelay) recoveryLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 尝试获取锁 (只持有很短时间)
			if r.tryAcquireLock(ctx, recoveryLockKey) {
				logger.Info("acquired recovery lock, starting recovery")
				r.recoverStaleMessages(ctx)
				// 任务完成后立即释放，不需要一直持有
				r.releaseLockSimple(ctx, recoveryLockKey)
			}
		}
	}
}

// recoverStaleMessages 恢复卡住的消息
func (r *OrderOutboxRelay) recoverStaleMessages(ctx context.Context) {
	// 使用 SCAN 遍历所有 outbox 记录
	var cursor uint64
	pattern := "eidos:trading:outbox:order:*"
	staleThreshold := time.Now().Add(-r.cfg.StaleThreshold).UnixMilli()

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			logger.Error("scan outbox keys failed", zap.Error(err))
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

			if status != OutboxStatusProcessing {
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
			pendingKey := fmt.Sprintf(outboxPendingKeyPattern, shardID)

			// 提取 orderID
			orderID := key[len("eidos:trading:outbox:order:"):]

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
				logger.Error("recover stale order failed",
					zap.String("order_id", orderID),
					zap.Error(err))
				continue
			}

			if recovered == 1 {
				logger.Info("recovered stale processing order",
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
// 增加分布式锁，避免多实例并发扫描
func (r *OrderOutboxRelay) cleanupLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 尝试获取锁
			if r.tryAcquireLock(ctx, cleanupLockKey) {
				logger.Info("acquired cleanup lock, starting cleanup")
				r.cleanupSentMessages(ctx)
				r.alertFailedMessages(ctx)
				r.releaseLockSimple(ctx, cleanupLockKey)
			}
		}
	}
}

// releaseLockSimple 简单释放非分片锁
func (r *OrderOutboxRelay) releaseLockSimple(ctx context.Context, lockKey string) {
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
func (r *OrderOutboxRelay) cleanupSentMessages(ctx context.Context) {
	var cursor uint64
	pattern := "eidos:trading:outbox:order:*"
	retention := 24 * time.Hour
	retentionThreshold := time.Now().Add(-retention).UnixMilli()
	deleted := 0

	for {
		keys, nextCursor, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			logger.Error("scan outbox keys for cleanup failed", zap.Error(err))
			return
		}

		for _, key := range keys {
			result, err := r.rdb.HMGet(ctx, key, "status", "sent_at").Result()
			if err != nil {
				continue
			}

			status, _ := result[0].(string)
			sentAtStr, _ := result[1].(string)

			if status != OutboxStatusSent {
				continue
			}

			var sentAt int64
			fmt.Sscanf(sentAtStr, "%d", &sentAt)

			if sentAt > retentionThreshold {
				continue // 还在保留期内
			}

			// 删除记录
			if err := r.rdb.Del(ctx, key).Err(); err != nil {
				logger.Error("delete sent outbox record failed",
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
		logger.Info("cleaned up sent order outbox records",
			zap.Int("count", deleted),
		)
	}
}

// alertFailedMessages 告警失败消息
func (r *OrderOutboxRelay) alertFailedMessages(ctx context.Context) {
	var cursor uint64
	pattern := "eidos:trading:outbox:order:*"
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
			if status == OutboxStatusFailed {
				failedCount++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if failedCount > 0 {
		logger.Warn("order outbox has failed messages",
			zap.Int("count", failedCount),
		)
		// 发送告警通知 (Metrics)
		metrics.RecordOutboxError("order", "processing_failed_in_db")
	}
}

// OrderMessage 订单消息结构 (用于 Kafka)
type OrderMessage struct {
	OrderID       string `json:"order_id"`
	Wallet        string `json:"wallet"`
	Market        string `json:"market"`
	Side          int8   `json:"side"`
	Type          int8   `json:"type"`
	Price         string `json:"price"`
	Amount        string `json:"amount"`
	FilledAmount  string `json:"filled_amount"`
	FilledQuote   string `json:"filled_quote"`
	Status        int8   `json:"status"`
	Nonce         uint64 `json:"nonce"`
	Signature     []byte `json:"signature"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	ExpireAt      int64  `json:"expire_at,omitempty"`
	FreezeToken   string `json:"freeze_token"`
	FreezeAmount  string `json:"freeze_amount"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// ParseOrderMessage 解析订单消息
func ParseOrderMessage(data []byte) (*OrderMessage, error) {
	var msg OrderMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
