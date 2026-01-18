package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
)

func setupBlacklistTestCache(t *testing.T) (*miniredis.Miniredis, *cache.BlacklistCache) {
	s := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	return s, cache.NewBlacklistCache(client)
}

func TestBlacklistService_CheckBlacklist_FromCache(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// 先加入缓存
	entry := &cache.BlacklistEntry{
		WalletAddress:  wallet,
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 0,
	}
	err := blacklistCache.Set(ctx, entry)
	require.NoError(t, err)

	// 使用仅缓存的服务进行测试
	svc := &BlacklistService{
		cache: blacklistCache,
	}

	// 检查黑名单
	resp, err := svc.CheckBlacklist(ctx, wallet)
	require.NoError(t, err)
	assert.True(t, resp.IsBlacklisted)
	assert.Equal(t, "test reason", resp.Reason)
}

func TestBlacklistService_CheckBlacklist_NotInList(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// 检查黑名单 - 缓存中没有
	// 直接测试缓存逻辑
	result, err := blacklistCache.Get(ctx, wallet)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestBlacklistService_CheckBlacklist_Expired(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)
	ctx := context.Background()

	wallet := "0x1234567890123456789012345678901234567890"

	// 设置一个已过期的条目
	entry := &cache.BlacklistEntry{
		WalletAddress:  wallet,
		ListType:       "full",
		Reason:         "test reason",
		EffectiveUntil: 1000, // 1970年代的时间戳，已过期
	}
	err := blacklistCache.Set(ctx, entry)
	require.NoError(t, err)

	// 使用仅缓存的服务
	svc := &BlacklistService{
		cache: blacklistCache,
	}

	// 检查黑名单
	resp, err := svc.CheckBlacklist(ctx, wallet)
	require.NoError(t, err)
	// 已过期，应该不在黑名单中
	assert.False(t, resp.IsBlacklisted)
}

func TestBlacklistService_SyncCacheFromDB_Integration(t *testing.T) {
	// 这个测试需要数据库，跳过
	t.Skip("Integration test requires database")
}

func TestBlacklistService_AlertCallback(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)

	alertCalled := false
	alertWallet := ""

	svc := &BlacklistService{
		cache: blacklistCache,
	}

	// 设置告警回调
	svc.SetOnRiskAlert(func(ctx context.Context, alert *RiskAlertMessage) error {
		alertCalled = true
		alertWallet = alert.Wallet
		return nil
	})

	// 触发告警
	ctx := context.Background()
	svc.sendAlert(ctx, &RiskAlertMessage{
		AlertID:   "test-alert",
		Wallet:    "0xtest",
		AlertType: "TEST",
		Severity:  "info",
	})

	assert.True(t, alertCalled)
	assert.Equal(t, "0xtest", alertWallet)
}

func TestBlacklistService_AlertCallback_NotSet(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)

	svc := &BlacklistService{
		cache: blacklistCache,
	}

	// 不设置回调，调用 sendAlert 不应该 panic
	ctx := context.Background()
	svc.sendAlert(ctx, &RiskAlertMessage{
		AlertID:   "test-alert",
		Wallet:    "0xtest",
		AlertType: "TEST",
		Severity:  "info",
	})
	// 只要不 panic 就算通过
}

func TestBlacklistService_BatchCheck(t *testing.T) {
	_, blacklistCache := setupBlacklistTestCache(t)
	ctx := context.Background()

	// 添加一些条目到缓存
	wallet1 := "0x1111111111111111111111111111111111111111"
	wallet2 := "0x2222222222222222222222222222222222222222"
	wallet3 := "0x3333333333333333333333333333333333333333"

	err := blacklistCache.Set(ctx, &cache.BlacklistEntry{
		WalletAddress:  wallet1,
		ListType:       "full",
		Reason:         "reason 1",
		EffectiveUntil: 0,
	})
	require.NoError(t, err)

	err = blacklistCache.Set(ctx, &cache.BlacklistEntry{
		WalletAddress:  wallet2,
		ListType:       "trade",
		Reason:         "reason 2",
		EffectiveUntil: 0,
	})
	require.NoError(t, err)

	// wallet3 不在黑名单中

	// 批量检查
	results, err := blacklistCache.BatchCheck(ctx, []string{wallet1, wallet2, wallet3})
	require.NoError(t, err)

	assert.True(t, results[wallet1])
	assert.True(t, results[wallet2])
	assert.False(t, results[wallet3])
}
