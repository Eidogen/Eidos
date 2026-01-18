package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	withdrawHistoryKeyPrefix = "risk:withdraw:history:"
	addressWhitelistKeyPrefix = "risk:addr:whitelist:"
	contractAddressKey       = "risk:addr:contracts"
	withdrawTTL              = 24 * time.Hour
)

// WithdrawCache 提现缓存
type WithdrawCache struct {
	client redis.UniversalClient
}

// NewWithdrawCache 创建提现缓存
func NewWithdrawCache(client redis.UniversalClient) *WithdrawCache {
	return &WithdrawCache{client: client}
}

// RecordWithdraw 记录提现
func (c *WithdrawCache) RecordWithdraw(ctx context.Context, wallet, toAddress string, timestamp int64) error {
	key := withdrawHistoryKey(wallet)

	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(timestamp),
		Member: fmt.Sprintf("%d:%s", timestamp, toAddress),
	})
	pipe.Expire(ctx, key, withdrawTTL)

	_, err := pipe.Exec(ctx)
	return err
}

// GetRecentWithdrawCount 获取最近提现次数
func (c *WithdrawCache) GetRecentWithdrawCount(ctx context.Context, wallet string, withinSeconds int64) (int, error) {
	key := withdrawHistoryKey(wallet)
	now := time.Now().UnixMilli()
	start := now - withinSeconds*1000

	count, err := c.client.ZCount(ctx, key, fmt.Sprintf("%d", start), fmt.Sprintf("%d", now)).Result()
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// GetRecentWithdrawAddresses 获取最近提现地址列表
func (c *WithdrawCache) GetRecentWithdrawAddresses(ctx context.Context, wallet string, count int64) ([]string, error) {
	key := withdrawHistoryKey(wallet)

	results, err := c.client.ZRevRange(ctx, key, 0, count-1).Result()
	if err != nil {
		return nil, err
	}

	// 提取地址部分
	addresses := make([]string, 0, len(results))
	for _, r := range results {
		// 格式: timestamp:address
		if len(r) > 14 { // 至少有timestamp:0x...
			addresses = append(addresses, r[14:])
		}
	}

	return addresses, nil
}

// IsNewAddress 检查是否是新地址
func (c *WithdrawCache) IsNewAddress(ctx context.Context, wallet, toAddress string) (bool, error) {
	key := withdrawHistoryKey(wallet)

	// 搜索历史记录中是否有该地址
	iter := c.client.ZScan(ctx, key, 0, "*:"+toAddress, 10).Iterator()

	hasHistory := false
	for iter.Next(ctx) {
		hasHistory = true
		break
	}

	if err := iter.Err(); err != nil {
		return true, err
	}

	return !hasHistory, nil
}

// AddToWhitelist 添加到地址白名单
func (c *WithdrawCache) AddToWhitelist(ctx context.Context, wallet, address string) error {
	key := addressWhitelistKeyPrefix + wallet
	return c.client.SAdd(ctx, key, address).Err()
}

// RemoveFromWhitelist 从地址白名单移除
func (c *WithdrawCache) RemoveFromWhitelist(ctx context.Context, wallet, address string) error {
	key := addressWhitelistKeyPrefix + wallet
	return c.client.SRem(ctx, key, address).Err()
}

// IsWhitelistedAddress 检查地址是否在白名单中
func (c *WithdrawCache) IsWhitelistedAddress(ctx context.Context, wallet, address string) (bool, error) {
	key := addressWhitelistKeyPrefix + wallet
	return c.client.SIsMember(ctx, key, address).Result()
}

// GetWhitelistAddresses 获取白名单地址列表
func (c *WithdrawCache) GetWhitelistAddresses(ctx context.Context, wallet string) ([]string, error) {
	key := addressWhitelistKeyPrefix + wallet
	return c.client.SMembers(ctx, key).Result()
}

// MarkContractAddress 标记为合约地址
func (c *WithdrawCache) MarkContractAddress(ctx context.Context, address string) error {
	return c.client.SAdd(ctx, contractAddressKey, address).Err()
}

// IsContractAddress 检查是否是合约地址
func (c *WithdrawCache) IsContractAddress(ctx context.Context, address string) (bool, error) {
	return c.client.SIsMember(ctx, contractAddressKey, address).Result()
}

// BatchMarkContractAddresses 批量标记合约地址
func (c *WithdrawCache) BatchMarkContractAddresses(ctx context.Context, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}

	members := make([]interface{}, len(addresses))
	for i, addr := range addresses {
		members[i] = addr
	}

	return c.client.SAdd(ctx, contractAddressKey, members...).Err()
}

// ClearWithdrawHistory 清除提现历史
func (c *WithdrawCache) ClearWithdrawHistory(ctx context.Context, wallet string) error {
	key := withdrawHistoryKey(wallet)
	return c.client.Del(ctx, key).Err()
}

// GetWithdrawStats 获取提现统计
func (c *WithdrawCache) GetWithdrawStats(ctx context.Context, wallet string) (map[string]interface{}, error) {
	key := withdrawHistoryKey(wallet)

	// 获取总次数
	totalCount, err := c.client.ZCard(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	// 获取最近1小时次数
	now := time.Now().UnixMilli()
	hourAgo := now - 3600*1000
	recentCount, err := c.client.ZCount(ctx, key, fmt.Sprintf("%d", hourAgo), fmt.Sprintf("%d", now)).Result()
	if err != nil {
		return nil, err
	}

	// 获取白名单数量
	whitelistKey := addressWhitelistKeyPrefix + wallet
	whitelistCount, _ := c.client.SCard(ctx, whitelistKey).Result()

	return map[string]interface{}{
		"total_count":     totalCount,
		"recent_1h_count": recentCount,
		"whitelist_count": whitelistCount,
	}, nil
}

// withdrawHistoryKey 生成提现历史键
func withdrawHistoryKey(wallet string) string {
	return fmt.Sprintf("%s%s", withdrawHistoryKeyPrefix, wallet)
}
