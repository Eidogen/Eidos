// Package cache 提供缓存相关功能
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// ReplayKeyPrefix 重放保护的 Redis Key 前缀
	ReplayKeyPrefix = "eidos:api:replay:"
	// DefaultReplayTTL 默认重放保护 TTL
	DefaultReplayTTL = 5 * time.Minute
)

// ReplayGuard 签名重放保护
type ReplayGuard struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewReplayGuard 创建重放保护实例
func NewReplayGuard(rdb *redis.Client) *ReplayGuard {
	return &ReplayGuard{
		rdb: rdb,
		ttl: DefaultReplayTTL,
	}
}

// NewReplayGuardWithTTL 创建带自定义 TTL 的重放保护实例
func NewReplayGuardWithTTL(rdb *redis.Client, ttl time.Duration) *ReplayGuard {
	return &ReplayGuard{
		rdb: rdb,
		ttl: ttl,
	}
}

// CheckAndMark 检查签名是否已使用，如果未使用则标记
// 返回 true 表示可以使用（未重放），false 表示重放攻击
func (g *ReplayGuard) CheckAndMark(ctx context.Context, wallet, timestamp, signature string) (bool, error) {
	// 计算签名哈希
	hash := g.computeHash(wallet, timestamp, signature)
	key := ReplayKeyPrefix + hash

	// 使用 SETNX 原子操作
	ok, err := g.rdb.SetNX(ctx, key, "1", g.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}

	return ok, nil
}

// Check 仅检查签名是否已使用（不标记）
func (g *ReplayGuard) Check(ctx context.Context, wallet, timestamp, signature string) (bool, error) {
	hash := g.computeHash(wallet, timestamp, signature)
	key := ReplayKeyPrefix + hash

	exists, err := g.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists: %w", err)
	}

	return exists == 0, nil
}

// Mark 标记签名为已使用
func (g *ReplayGuard) Mark(ctx context.Context, wallet, timestamp, signature string) error {
	hash := g.computeHash(wallet, timestamp, signature)
	key := ReplayKeyPrefix + hash

	return g.rdb.Set(ctx, key, "1", g.ttl).Err()
}

// computeHash 计算签名的唯一哈希
func (g *ReplayGuard) computeHash(wallet, timestamp, signature string) string {
	data := wallet + ":" + timestamp + ":" + signature
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
