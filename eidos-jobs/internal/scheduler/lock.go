package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

const (
	lockPrefix = "eidos:job:lock:"
)

// DistributedLock 分布式锁
type DistributedLock struct {
	client     redis.UniversalClient
	key        string
	value      string
	ttl        time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
	useWatchdog bool
}

// NewDistributedLock 创建分布式锁
func NewDistributedLock(client redis.UniversalClient, jobName string, ttl time.Duration, useWatchdog bool) *DistributedLock {
	return &DistributedLock{
		client:      client,
		key:         lockPrefix + jobName,
		value:       fmt.Sprintf("%d", time.Now().UnixNano()),
		ttl:         ttl,
		stopCh:      make(chan struct{}),
		useWatchdog: useWatchdog,
	}
}

// TryLock 尝试获取锁
func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	// 使用 SET NX 获取锁
	ok, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if ok && l.useWatchdog {
		// 启动 watchdog 协程自动续期
		l.startWatchdog(ctx)
	}

	return ok, nil
}

// Unlock 释放锁
func (l *DistributedLock) Unlock(ctx context.Context) error {
	// 停止 watchdog
	if l.useWatchdog {
		close(l.stopCh)
		l.wg.Wait()
	}

	// 使用 Lua 脚本确保只释放自己持有的锁
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		end
		return 0
	`

	_, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	return nil
}

// startWatchdog 启动 watchdog 自动续期
func (l *DistributedLock) startWatchdog(ctx context.Context) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		// 在 TTL 的 1/3 时间点续期
		renewInterval := l.ttl / 3
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-l.stopCh:
				return
			case <-ticker.C:
				if err := l.renew(ctx); err != nil {
					logger.Warn("failed to renew lock",
						"key", l.key,
						"error", err)
				}
			}
		}
	}()
}

// renew 续期锁
func (l *DistributedLock) renew(ctx context.Context) error {
	// 使用 Lua 脚本确保只续期自己持有的锁
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		end
		return 0
	`

	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, int64(l.ttl.Milliseconds())).Result()
	if err != nil {
		return err
	}

	if result.(int64) == 0 {
		return fmt.Errorf("lock not held")
	}

	return nil
}

// IsHeld 检查锁是否仍被持有
func (l *DistributedLock) IsHeld(ctx context.Context) (bool, error) {
	val, err := l.client.Get(ctx, l.key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == l.value, nil
}

// LockManager 锁管理器
type LockManager struct {
	client redis.UniversalClient
}

// NewLockManager 创建锁管理器
func NewLockManager(client redis.UniversalClient) *LockManager {
	return &LockManager{client: client}
}

// NewLock 创建新锁
func (m *LockManager) NewLock(jobName string, ttl time.Duration, useWatchdog bool) *DistributedLock {
	return NewDistributedLock(m.client, jobName, ttl, useWatchdog)
}

// IsLocked 检查任务是否被锁定
func (m *LockManager) IsLocked(ctx context.Context, jobName string) (bool, error) {
	exists, err := m.client.Exists(ctx, lockPrefix+jobName).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// ForceUnlock 强制解锁 (管理员操作)
func (m *LockManager) ForceUnlock(ctx context.Context, jobName string) error {
	return m.client.Del(ctx, lockPrefix+jobName).Err()
}

// GetLockInfo 获取锁信息
func (m *LockManager) GetLockInfo(ctx context.Context, jobName string) (string, time.Duration, error) {
	key := lockPrefix + jobName

	pipe := m.client.Pipeline()
	valCmd := pipe.Get(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return "", 0, err
	}

	val, _ := valCmd.Result()
	ttl, _ := ttlCmd.Result()

	return val, ttl, nil
}
