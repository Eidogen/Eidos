package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrLockNotHeld 锁未持有
	ErrLockNotHeld = errors.New("lock not held")
	// ErrLockAcquireFailed 获取锁失败
	ErrLockAcquireFailed = errors.New("failed to acquire lock")
)

// RedisLock Redis 分布式锁
type RedisLock struct {
	client     redis.UniversalClient
	key        string
	value      string
	expiration time.Duration
}

// RedisLocker Redis 分布式锁管理器
type RedisLocker struct {
	client     redis.UniversalClient
	keyPrefix  string
	expiration time.Duration
}

// NewRedisLocker 创建 Redis 分布式锁管理器
func NewRedisLocker(client redis.UniversalClient, keyPrefix string, expiration time.Duration) *RedisLocker {
	if expiration == 0 {
		expiration = 30 * time.Second
	}
	return &RedisLocker{
		client:     client,
		keyPrefix:  keyPrefix,
		expiration: expiration,
	}
}

// NewLock 创建一个新锁
func (l *RedisLocker) NewLock(key string) *RedisLock {
	return &RedisLock{
		client:     l.client,
		key:        l.keyPrefix + key,
		value:      uuid.New().String(),
		expiration: l.expiration,
	}
}

// Acquire 获取锁 (非阻塞)
func (lock *RedisLock) Acquire(ctx context.Context) (bool, error) {
	ok, err := lock.client.SetNX(ctx, lock.key, lock.value, lock.expiration).Result()
	if err != nil {
		return false, fmt.Errorf("acquire lock failed: %w", err)
	}
	return ok, nil
}

// AcquireWithRetry 获取锁 (带重试)
func (lock *RedisLock) AcquireWithRetry(ctx context.Context, retryInterval time.Duration, maxRetries int) (bool, error) {
	for i := 0; i < maxRetries; i++ {
		ok, err := lock.Acquire(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(retryInterval):
			// 继续重试
		}
	}
	return false, nil
}

// AcquireOrWait 获取锁 (阻塞等待直到成功或超时)
func (lock *RedisLock) AcquireOrWait(ctx context.Context, retryInterval time.Duration) error {
	for {
		ok, err := lock.Acquire(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			// 继续重试
		}
	}
}

// Release 释放锁 (原子操作，只有持有者才能释放)
func (lock *RedisLock) Release(ctx context.Context) error {
	// Lua 脚本：只有当 key 的值等于 value 时才删除
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, lock.client, []string{lock.key}, lock.value).Int64()
	if err != nil {
		return fmt.Errorf("release lock failed: %w", err)
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// Extend 延长锁的过期时间 (原子操作，只有持有者才能延长)
func (lock *RedisLock) Extend(ctx context.Context, extension time.Duration) error {
	// Lua 脚本：只有当 key 的值等于 value 时才延长过期时间
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, lock.client, []string{lock.key}, lock.value, extension.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("extend lock failed: %w", err)
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// WithLock 在锁保护下执行函数
func (l *RedisLocker) WithLock(ctx context.Context, key string, fn func(ctx context.Context) error) error {
	lock := l.NewLock(key)

	ok, err := lock.Acquire(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLockAcquireFailed
	}

	defer func() {
		// 释放锁，忽略错误 (可能已过期)
		_ = lock.Release(ctx)
	}()

	return fn(ctx)
}

// WithLockRetry 在锁保护下执行函数 (带重试获取锁)
func (l *RedisLocker) WithLockRetry(ctx context.Context, key string, retryInterval time.Duration, maxRetries int, fn func(ctx context.Context) error) error {
	lock := l.NewLock(key)

	ok, err := lock.AcquireWithRetry(ctx, retryInterval, maxRetries)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLockAcquireFailed
	}

	defer func() {
		_ = lock.Release(ctx)
	}()

	return fn(ctx)
}
