// Package cache 提供风控服务的缓存层
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	blacklistKeyPrefix = "risk:blacklist:"
	blacklistSetKey    = "risk:blacklist:all"
	blacklistTTL       = 24 * time.Hour
)

// BlacklistEntry 黑名单缓存条目
type BlacklistEntry struct {
	WalletAddress  string `json:"wallet_address"`
	ListType       string `json:"list_type"` // trade, withdraw, full
	Reason         string `json:"reason"`
	EffectiveUntil int64  `json:"effective_until"` // 0 表示永久
}

// BlacklistCache 黑名单缓存
type BlacklistCache struct {
	client redis.UniversalClient
}

// NewBlacklistCache 创建黑名单缓存
func NewBlacklistCache(client redis.UniversalClient) *BlacklistCache {
	return &BlacklistCache{client: client}
}

// Set 设置黑名单条目
func (c *BlacklistCache) Set(ctx context.Context, entry *BlacklistEntry) error {
	key := blacklistKeyPrefix + entry.WalletAddress

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	pipe := c.client.Pipeline()

	// 设置详情
	pipe.Set(ctx, key, data, blacklistTTL)

	// 添加到集合
	pipe.SAdd(ctx, blacklistSetKey, entry.WalletAddress)

	_, err = pipe.Exec(ctx)
	return err
}

// Get 获取黑名单条目
func (c *BlacklistCache) Get(ctx context.Context, wallet string) (*BlacklistEntry, error) {
	key := blacklistKeyPrefix + wallet

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var entry BlacklistEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

// Remove 移除黑名单条目
func (c *BlacklistCache) Remove(ctx context.Context, wallet string) error {
	key := blacklistKeyPrefix + wallet

	pipe := c.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, blacklistSetKey, wallet)

	_, err := pipe.Exec(ctx)
	return err
}

// Exists 检查是否在黑名单中
func (c *BlacklistCache) Exists(ctx context.Context, wallet string) (bool, error) {
	return c.client.SIsMember(ctx, blacklistSetKey, wallet).Result()
}

// GetAll 获取所有黑名单地址
func (c *BlacklistCache) GetAll(ctx context.Context) ([]string, error) {
	return c.client.SMembers(ctx, blacklistSetKey).Result()
}

// LoadFromDB 从数据库加载黑名单到缓存
func (c *BlacklistCache) LoadFromDB(ctx context.Context, entries []*BlacklistEntry) error {
	if len(entries) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()

	for _, entry := range entries {
		key := blacklistKeyPrefix + entry.WalletAddress
		data, _ := json.Marshal(entry)
		pipe.Set(ctx, key, data, blacklistTTL)
		pipe.SAdd(ctx, blacklistSetKey, entry.WalletAddress)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// Clear 清空黑名单缓存
func (c *BlacklistCache) Clear(ctx context.Context) error {
	// 获取所有钱包地址
	wallets, err := c.GetAll(ctx)
	if err != nil {
		return err
	}

	if len(wallets) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()

	// 删除所有详情
	for _, wallet := range wallets {
		pipe.Del(ctx, blacklistKeyPrefix+wallet)
	}

	// 删除集合
	pipe.Del(ctx, blacklistSetKey)

	_, err = pipe.Exec(ctx)
	return err
}

// Count 获取黑名单数量
func (c *BlacklistCache) Count(ctx context.Context) (int64, error) {
	return c.client.SCard(ctx, blacklistSetKey).Result()
}

// IsBlacklisted 检查钱包是否被禁止特定操作
func (c *BlacklistCache) IsBlacklisted(ctx context.Context, wallet, action string) (bool, string, error) {
	entry, err := c.Get(ctx, wallet)
	if err != nil {
		return false, "", err
	}

	if entry == nil {
		return false, "", nil
	}

	// 检查是否过期
	now := time.Now().UnixMilli()
	if entry.EffectiveUntil > 0 && now > entry.EffectiveUntil {
		return false, "", nil
	}

	// 检查黑名单类型
	switch entry.ListType {
	case "full":
		return true, entry.Reason, nil
	case "trade":
		if action == "trade" {
			return true, entry.Reason, nil
		}
	case "withdraw":
		if action == "withdraw" {
			return true, entry.Reason, nil
		}
	}

	return false, "", nil
}

// BatchCheck 批量检查地址是否在黑名单中
func (c *BlacklistCache) BatchCheck(ctx context.Context, wallets []string) (map[string]bool, error) {
	if len(wallets) == 0 {
		return make(map[string]bool), nil
	}

	// 使用 pipeline 批量检查
	pipe := c.client.Pipeline()
	cmds := make(map[string]*redis.BoolCmd)

	for _, wallet := range wallets {
		cmds[wallet] = pipe.SIsMember(ctx, blacklistSetKey, wallet)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for wallet, cmd := range cmds {
		result[wallet], _ = cmd.Result()
	}

	return result, nil
}

// Refresh 刷新单个条目的 TTL
func (c *BlacklistCache) Refresh(ctx context.Context, wallet string) error {
	key := blacklistKeyPrefix + wallet
	return c.client.Expire(ctx, key, blacklistTTL).Err()
}

// GetExpiringSoon 获取即将过期的黑名单条目
func (c *BlacklistCache) GetExpiringSoon(ctx context.Context, withinSeconds int64) ([]*BlacklistEntry, error) {
	wallets, err := c.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var expiring []*BlacklistEntry
	now := time.Now().UnixMilli()
	threshold := now + withinSeconds*1000

	for _, wallet := range wallets {
		entry, err := c.Get(ctx, wallet)
		if err != nil || entry == nil {
			continue
		}

		if entry.EffectiveUntil > 0 && entry.EffectiveUntil <= threshold {
			expiring = append(expiring, entry)
		}
	}

	return expiring, nil
}

// Stats 获取缓存统计信息
func (c *BlacklistCache) Stats(ctx context.Context) (map[string]interface{}, error) {
	count, err := c.Count(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_count": count,
		"key_prefix":  blacklistKeyPrefix,
		"ttl_seconds": int64(blacklistTTL.Seconds()),
	}, nil
}

// blacklistKey 生成缓存键
func blacklistKey(wallet string) string {
	return fmt.Sprintf("%s%s", blacklistKeyPrefix, wallet)
}
