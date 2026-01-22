package blockchain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/redis/go-redis/v9"
)

var (
	ErrNonceLockFailed   = errors.New("failed to acquire nonce lock")
	ErrNonceLockTimeout  = errors.New("nonce lock timeout")
	ErrNonceNotAcquired  = errors.New("nonce not acquired")
	ErrNonceSyncRequired = errors.New("nonce sync from chain required")
)

// NonceManager Nonce 管理器
// 使用 Redis 分布式锁管理 Nonce，确保并发安全
type NonceManager struct {
	client      *Client
	redis       *redis.Client
	wallet      common.Address
	chainID     int64
	lockTimeout time.Duration

	// 本地缓存
	mu           sync.RWMutex
	localNonce   uint64
	lastSyncTime time.Time
	syncInterval time.Duration

	// 待确认交易追踪
	pendingMu   sync.RWMutex
	pendingTxs  map[uint64]string // nonce -> txHash
	maxPending  int
}

// NonceManagerConfig 配置
type NonceManagerConfig struct {
	Wallet       common.Address
	ChainID      int64
	LockTimeout  time.Duration
	SyncInterval time.Duration
	MaxPending   int
}

// NewNonceManager 创建 Nonce 管理器
func NewNonceManager(client *Client, rdb *redis.Client, cfg *NonceManagerConfig) *NonceManager {
	lockTimeout := cfg.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = 30 * time.Second
	}

	syncInterval := cfg.SyncInterval
	if syncInterval == 0 {
		syncInterval = 5 * time.Minute
	}

	maxPending := cfg.MaxPending
	if maxPending == 0 {
		maxPending = 100
	}

	return &NonceManager{
		client:       client,
		redis:        rdb,
		wallet:       cfg.Wallet,
		chainID:      cfg.ChainID,
		lockTimeout:  lockTimeout,
		syncInterval: syncInterval,
		maxPending:   maxPending,
		pendingTxs:   make(map[uint64]string),
	}
}

// nonceKey 生成 Redis key
func (m *NonceManager) nonceKey() string {
	return fmt.Sprintf("eidos:chain:nonce:%s:%d", m.wallet.Hex(), m.chainID)
}

// lockKey 生成锁 key
func (m *NonceManager) lockKey() string {
	return fmt.Sprintf("eidos:chain:nonce:lock:%s:%d", m.wallet.Hex(), m.chainID)
}

// pendingKey 生成待确认队列 key
func (m *NonceManager) pendingKey() string {
	return fmt.Sprintf("eidos:chain:nonce:pending:%s:%d", m.wallet.Hex(), m.chainID)
}

// AcquireNonce 获取并锁定一个 Nonce
// 返回的 nonce 必须通过 ConfirmNonce 或 ReleaseNonce 处理
func (m *NonceManager) AcquireNonce(ctx context.Context) (uint64, error) {
	// 获取分布式锁
	lockAcquired, err := m.acquireLock(ctx)
	if err != nil {
		return 0, err
	}
	if !lockAcquired {
		return 0, ErrNonceLockFailed
	}
	defer m.releaseLock(ctx)

	// 检查是否需要从链上同步
	if m.needsSync() {
		if err := m.syncFromChain(ctx); err != nil {
			return 0, err
		}
	}

	// 获取当前 nonce
	nonce, err := m.getCurrentNonce(ctx)
	if err != nil {
		return 0, err
	}

	// 递增并保存
	nextNonce := nonce + 1
	if err := m.setCurrentNonce(ctx, nextNonce); err != nil {
		return 0, err
	}

	// 记录到待确认队列
	m.pendingMu.Lock()
	m.pendingTxs[nonce] = "" // 尚未关联 txHash
	m.pendingMu.Unlock()

	return nonce, nil
}

// ConfirmNonce 确认 Nonce 已使用
func (m *NonceManager) ConfirmNonce(ctx context.Context, nonce uint64, txHash string) error {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	if _, exists := m.pendingTxs[nonce]; !exists {
		// nonce 不在待确认列表，可能已被处理
		return nil
	}

	m.pendingTxs[nonce] = txHash

	// 添加到 Redis 待确认队列
	err := m.redis.ZAdd(ctx, m.pendingKey(), redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: fmt.Sprintf("%d:%s", nonce, txHash),
	}).Err()

	return err
}

// ReleaseNonce 释放未使用的 Nonce
// 当交易构建失败时调用
func (m *NonceManager) ReleaseNonce(ctx context.Context, nonce uint64) error {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	if _, exists := m.pendingTxs[nonce]; !exists {
		return ErrNonceNotAcquired
	}

	delete(m.pendingTxs, nonce)

	// 注意：由于其他交易可能已使用更高的 nonce，
	// 我们不能简单地回退 nonce 计数器。
	// 这个 nonce 槽位将成为一个"空洞"，需要后续处理。
	return nil
}

// OnTxConfirmed 交易确认回调
func (m *NonceManager) OnTxConfirmed(ctx context.Context, nonce uint64, txHash string) error {
	m.pendingMu.Lock()
	delete(m.pendingTxs, nonce)
	m.pendingMu.Unlock()

	// 从 Redis 待确认队列移除
	return m.redis.ZRem(ctx, m.pendingKey(), fmt.Sprintf("%d:%s", nonce, txHash)).Err()
}

// OnTxFailed 交易失败回调
func (m *NonceManager) OnTxFailed(ctx context.Context, nonce uint64, txHash string) error {
	m.pendingMu.Lock()
	delete(m.pendingTxs, nonce)
	m.pendingMu.Unlock()

	// 从 Redis 待确认队列移除
	return m.redis.ZRem(ctx, m.pendingKey(), fmt.Sprintf("%d:%s", nonce, txHash)).Err()
}

// SyncFromChain 从链上同步 Nonce
func (m *NonceManager) SyncFromChain(ctx context.Context) error {
	lockAcquired, err := m.acquireLock(ctx)
	if err != nil {
		return err
	}
	if !lockAcquired {
		return ErrNonceLockFailed
	}
	defer m.releaseLock(ctx)

	return m.syncFromChain(ctx)
}

// syncFromChain 内部同步方法 (需要已持有锁)
func (m *NonceManager) syncFromChain(ctx context.Context) error {
	chainNonce, err := m.client.PendingNonceAt(ctx, m.wallet)
	if err != nil {
		return err
	}

	if err := m.setCurrentNonce(ctx, chainNonce); err != nil {
		return err
	}

	m.mu.Lock()
	m.localNonce = chainNonce
	m.lastSyncTime = time.Now()
	m.mu.Unlock()

	return nil
}

// acquireLock 获取分布式锁
func (m *NonceManager) acquireLock(ctx context.Context) (bool, error) {
	// 使用 SET NX 实现分布式锁
	ok, err := m.redis.SetNX(ctx, m.lockKey(), "1", m.lockTimeout).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

// releaseLock 释放分布式锁
func (m *NonceManager) releaseLock(ctx context.Context) error {
	return m.redis.Del(ctx, m.lockKey()).Err()
}

// getCurrentNonce 获取当前 nonce
func (m *NonceManager) getCurrentNonce(ctx context.Context) (uint64, error) {
	val, err := m.redis.Get(ctx, m.nonceKey()).Uint64()
	if err == redis.Nil {
		// 首次使用，从链上获取
		chainNonce, err := m.client.PendingNonceAt(ctx, m.wallet)
		if err != nil {
			return 0, err
		}
		return chainNonce, nil
	}
	if err != nil {
		return 0, err
	}
	return val, nil
}

// setCurrentNonce 设置当前 nonce
func (m *NonceManager) setCurrentNonce(ctx context.Context, nonce uint64) error {
	return m.redis.Set(ctx, m.nonceKey(), nonce, 0).Err()
}

// needsSync 检查是否需要同步
func (m *NonceManager) needsSync() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.lastSyncTime) > m.syncInterval
}

// GetPendingCount 获取待确认交易数量
func (m *NonceManager) GetPendingCount() int {
	m.pendingMu.RLock()
	defer m.pendingMu.RUnlock()
	return len(m.pendingTxs)
}

// GetCurrentNonce 获取当前 nonce (不获取锁，仅用于查询)
func (m *NonceManager) GetCurrentNonce(ctx context.Context) (uint64, error) {
	return m.getCurrentNonce(ctx)
}

// HandleNonceTooLow 处理 nonce too low 错误
func (m *NonceManager) HandleNonceTooLow(ctx context.Context) error {
	return m.SyncFromChain(ctx)
}

// HandleNonceTooHigh 处理 nonce too high 错误
func (m *NonceManager) HandleNonceTooHigh(ctx context.Context, expectedNonce uint64) error {
	// 等待之前的交易确认
	// 或者检查是否有丢失的交易需要重发
	return m.SyncFromChain(ctx)
}
