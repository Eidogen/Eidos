// Package ratelimit provides rate limiting functionality
package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// ConcurrencyLimiter limits concurrent requests using Redis
type ConcurrencyLimiter struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// NewConcurrencyLimiter creates a new concurrency limiter
func NewConcurrencyLimiter(client *redis.Client) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		client: client,
		prefix: "eidos:api:concurrent:",
		ttl:    30 * time.Second, // Auto-release after 30 seconds
	}
}

// Acquire tries to acquire a slot for concurrent requests
// Returns true if acquired, false if limit reached
func (l *ConcurrencyLimiter) Acquire(ctx context.Context, key string, limit int) (bool, error) {
	fullKey := l.prefix + key

	// Use Lua script for atomic increment with limit check
	script := redis.NewScript(`
		local current = redis.call('INCR', KEYS[1])
		if current == 1 then
			redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		if current > tonumber(ARGV[1]) then
			redis.call('DECR', KEYS[1])
			return 0
		end
		return 1
	`)

	result, err := script.Run(ctx, l.client, []string{fullKey}, limit, int(l.ttl.Seconds())).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Release releases a slot
func (l *ConcurrencyLimiter) Release(ctx context.Context, key string) error {
	fullKey := l.prefix + key
	return l.client.Decr(ctx, fullKey).Err()
}

// Current returns current concurrent count
func (l *ConcurrencyLimiter) Current(ctx context.Context, key string) (int, error) {
	fullKey := l.prefix + key
	val, err := l.client.Get(ctx, fullKey).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}
