package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/redis/scripts"
)

var (
	// ErrLockNotHeld 锁未持有
	ErrLockNotHeld = errors.New("lock not held")
	// ErrLockAcquireFailed 获取锁失败
	ErrLockAcquireFailed = errors.New("failed to acquire lock")
	// ErrLockExtendFailed 延长锁失败
	ErrLockExtendFailed = errors.New("failed to extend lock")
	// ErrLockTimeout 获取锁超时
	ErrLockTimeout = errors.New("lock acquire timeout")
)

// LockOptions 锁选项
type LockOptions struct {
	// Expiration 锁过期时间
	Expiration time.Duration

	// RetryInterval 重试间隔
	RetryInterval time.Duration

	// MaxRetries 最大重试次数 (-1 表示无限重试)
	MaxRetries int

	// WatchdogEnabled 是否启用看门狗自动续期
	WatchdogEnabled bool

	// WatchdogInterval 看门狗续期间隔 (默认为 Expiration/3)
	WatchdogInterval time.Duration

	// Metadata 锁元数据 (用于调试)
	Metadata string
}

// DefaultLockOptions 默认锁选项
func DefaultLockOptions() *LockOptions {
	return &LockOptions{
		Expiration:       30 * time.Second,
		RetryInterval:    100 * time.Millisecond,
		MaxRetries:       30,
		WatchdogEnabled:  false,
		WatchdogInterval: 10 * time.Second,
	}
}

// DistributedLock 分布式锁
type DistributedLock struct {
	client       redis.UniversalClient
	key          string
	value        string
	options      *LockOptions
	acquired     int32
	watchdogStop chan struct{}
	mu           sync.Mutex
}

// NewDistributedLock 创建分布式锁
func NewDistributedLock(client redis.UniversalClient, key string, opts *LockOptions) *DistributedLock {
	if opts == nil {
		opts = DefaultLockOptions()
	}

	if opts.WatchdogInterval == 0 {
		opts.WatchdogInterval = opts.Expiration / 3
	}

	return &DistributedLock{
		client:  client,
		key:     key,
		value:   uuid.New().String(),
		options: opts,
	}
}

// TryAcquire 尝试获取锁 (非阻塞)
func (l *DistributedLock) TryAcquire(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if atomic.LoadInt32(&l.acquired) == 1 {
		return true, nil // 已持有
	}

	script := scripts.LockScripts.Acquire
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, l.options.Expiration.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("acquire lock failed: %w", err)
	}

	if result == 1 {
		atomic.StoreInt32(&l.acquired, 1)
		if l.options.WatchdogEnabled {
			l.startWatchdog()
		}
		logger.Debug("lock acquired", zap.String("key", l.key), zap.String("value", l.value))
		return true, nil
	}

	return false, nil
}

// Acquire 获取锁 (带重试)
func (l *DistributedLock) Acquire(ctx context.Context) error {
	retries := 0
	maxRetries := l.options.MaxRetries

	for {
		ok, err := l.TryAcquire(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		retries++
		if maxRetries >= 0 && retries > maxRetries {
			return ErrLockAcquireFailed
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.options.RetryInterval):
			// 继续重试
		}
	}
}

// AcquireWithTimeout 获取锁 (带超时)
func (l *DistributedLock) AcquireWithTimeout(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := l.Acquire(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrLockTimeout
	}
	return err
}

// Release 释放锁
func (l *DistributedLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if atomic.LoadInt32(&l.acquired) == 0 {
		return ErrLockNotHeld
	}

	// 停止看门狗
	l.stopWatchdog()

	script := scripts.LockScripts.Release
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Int64()
	if err != nil {
		return fmt.Errorf("release lock failed: %w", err)
	}

	atomic.StoreInt32(&l.acquired, 0)

	if result == 0 {
		logger.Warn("lock already released or expired",
			zap.String("key", l.key),
			zap.String("value", l.value),
		)
		return ErrLockNotHeld
	}

	logger.Debug("lock released", zap.String("key", l.key), zap.String("value", l.value))
	return nil
}

// Extend 延长锁过期时间
func (l *DistributedLock) Extend(ctx context.Context, extension time.Duration) error {
	if atomic.LoadInt32(&l.acquired) == 0 {
		return ErrLockNotHeld
	}

	script := scripts.LockScripts.Extend
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, extension.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("extend lock failed: %w", err)
	}

	if result == 0 {
		atomic.StoreInt32(&l.acquired, 0)
		return ErrLockExtendFailed
	}

	logger.Debug("lock extended", zap.String("key", l.key), zap.Duration("extension", extension))
	return nil
}

// startWatchdog 启动看门狗
func (l *DistributedLock) startWatchdog() {
	l.watchdogStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(l.options.WatchdogInterval)
		defer ticker.Stop()

		for {
			select {
			case <-l.watchdogStop:
				return
			case <-ticker.C:
				if atomic.LoadInt32(&l.acquired) == 0 {
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := l.Extend(ctx, l.options.Expiration)
				cancel()

				if err != nil {
					logger.Error("watchdog extend failed",
						zap.String("key", l.key),
						zap.Error(err),
					)
					return
				}
			}
		}
	}()
}

// stopWatchdog 停止看门狗
func (l *DistributedLock) stopWatchdog() {
	if l.watchdogStop != nil {
		close(l.watchdogStop)
		l.watchdogStop = nil
	}
}

// IsHeld 检查是否持有锁
func (l *DistributedLock) IsHeld() bool {
	return atomic.LoadInt32(&l.acquired) == 1
}

// Key 获取锁键
func (l *DistributedLock) Key() string {
	return l.key
}

// Value 获取锁值
func (l *DistributedLock) Value() string {
	return l.value
}

// LockManager 锁管理器
type LockManager struct {
	client     redis.UniversalClient
	keyPrefix  string
	options    *LockOptions
	activeLocks sync.Map // key -> *DistributedLock
}

// NewLockManager 创建锁管理器
func NewLockManager(client redis.UniversalClient, keyPrefix string, opts *LockOptions) *LockManager {
	if opts == nil {
		opts = DefaultLockOptions()
	}
	return &LockManager{
		client:    client,
		keyPrefix: keyPrefix,
		options:   opts,
	}
}

// NewLock 创建锁
func (m *LockManager) NewLock(key string) *DistributedLock {
	fullKey := m.keyPrefix + key
	return NewDistributedLock(m.client, fullKey, m.options)
}

// NewLockWithOptions 创建锁 (自定义选项)
func (m *LockManager) NewLockWithOptions(key string, opts *LockOptions) *DistributedLock {
	fullKey := m.keyPrefix + key
	return NewDistributedLock(m.client, fullKey, opts)
}

// WithLock 在锁保护下执行函数
func (m *LockManager) WithLock(ctx context.Context, key string, fn func(ctx context.Context) error) error {
	lock := m.NewLock(key)

	if err := lock.Acquire(ctx); err != nil {
		return err
	}
	defer lock.Release(ctx)

	return fn(ctx)
}

// WithLockTimeout 在锁保护下执行函数 (带超时)
func (m *LockManager) WithLockTimeout(ctx context.Context, key string, timeout time.Duration, fn func(ctx context.Context) error) error {
	lock := m.NewLock(key)

	if err := lock.AcquireWithTimeout(ctx, timeout); err != nil {
		return err
	}
	defer lock.Release(ctx)

	return fn(ctx)
}

// TryWithLock 尝试在锁保护下执行函数 (非阻塞)
func (m *LockManager) TryWithLock(ctx context.Context, key string, fn func(ctx context.Context) error) (bool, error) {
	lock := m.NewLock(key)

	ok, err := lock.TryAcquire(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	defer lock.Release(ctx)

	return true, fn(ctx)
}

// ReentrantLock 可重入锁
type ReentrantLock struct {
	client   redis.UniversalClient
	key      string
	owner    string
	options  *LockOptions
	count    int32
	mu       sync.Mutex
}

// NewReentrantLock 创建可重入锁
func NewReentrantLock(client redis.UniversalClient, key, owner string, opts *LockOptions) *ReentrantLock {
	if opts == nil {
		opts = DefaultLockOptions()
	}
	return &ReentrantLock{
		client:  client,
		key:     key,
		owner:   owner,
		options: opts,
	}
}

// Acquire 获取可重入锁
func (l *ReentrantLock) Acquire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	script := scripts.LockScripts.ReentrantAcquire
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.owner, l.options.Expiration.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("acquire reentrant lock failed: %w", err)
	}

	if result == 0 {
		return ErrLockAcquireFailed
	}

	atomic.StoreInt32(&l.count, int32(result))
	logger.Debug("reentrant lock acquired",
		zap.String("key", l.key),
		zap.String("owner", l.owner),
		zap.Int64("count", result),
	)
	return nil
}

// Release 释放可重入锁
func (l *ReentrantLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	script := scripts.LockScripts.ReentrantRelease
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.owner).Int64()
	if err != nil {
		return fmt.Errorf("release reentrant lock failed: %w", err)
	}

	if result == 0 {
		return ErrLockNotHeld
	}

	if result == -1 {
		atomic.StoreInt32(&l.count, 0)
		logger.Debug("reentrant lock fully released",
			zap.String("key", l.key),
			zap.String("owner", l.owner),
		)
	} else {
		atomic.StoreInt32(&l.count, int32(result))
		logger.Debug("reentrant lock partially released",
			zap.String("key", l.key),
			zap.String("owner", l.owner),
			zap.Int64("remaining", result),
		)
	}

	return nil
}

// GetCount 获取重入次数
func (l *ReentrantLock) GetCount() int32 {
	return atomic.LoadInt32(&l.count)
}

// ReadWriteLock 读写锁
type ReadWriteLock struct {
	client      redis.UniversalClient
	readKey     string
	writeKey    string
	options     *LockOptions
}

// NewReadWriteLock 创建读写锁
func NewReadWriteLock(client redis.UniversalClient, key string, opts *LockOptions) *ReadWriteLock {
	if opts == nil {
		opts = DefaultLockOptions()
	}
	return &ReadWriteLock{
		client:   client,
		readKey:  key + ":readers",
		writeKey: key + ":writer",
		options:  opts,
	}
}

// RLock 获取读锁
func (l *ReadWriteLock) RLock(ctx context.Context, readerID string) error {
	script := scripts.ReadWriteLockScripts.AcquireRead
	result, err := l.client.Eval(ctx, script, []string{l.readKey, l.writeKey}, readerID, l.options.Expiration.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("acquire read lock failed: %w", err)
	}

	if result == 0 {
		return ErrLockAcquireFailed
	}

	logger.Debug("read lock acquired", zap.String("reader", readerID))
	return nil
}

// RUnlock 释放读锁
func (l *ReadWriteLock) RUnlock(ctx context.Context, readerID string) error {
	script := scripts.ReadWriteLockScripts.ReleaseRead
	_, err := l.client.Eval(ctx, script, []string{l.readKey}, readerID).Int64()
	if err != nil {
		return fmt.Errorf("release read lock failed: %w", err)
	}

	logger.Debug("read lock released", zap.String("reader", readerID))
	return nil
}

// Lock 获取写锁
func (l *ReadWriteLock) Lock(ctx context.Context, writerID string) error {
	script := scripts.ReadWriteLockScripts.AcquireWrite
	result, err := l.client.Eval(ctx, script, []string{l.readKey, l.writeKey}, writerID, l.options.Expiration.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("acquire write lock failed: %w", err)
	}

	if result == 0 {
		return ErrLockAcquireFailed
	}

	logger.Debug("write lock acquired", zap.String("writer", writerID))
	return nil
}

// Unlock 释放写锁
func (l *ReadWriteLock) Unlock(ctx context.Context, writerID string) error {
	script := scripts.ReadWriteLockScripts.ReleaseWrite
	result, err := l.client.Eval(ctx, script, []string{l.writeKey}, writerID).Int64()
	if err != nil {
		return fmt.Errorf("release write lock failed: %w", err)
	}

	if result == 0 {
		return ErrLockNotHeld
	}

	logger.Debug("write lock released", zap.String("writer", writerID))
	return nil
}
