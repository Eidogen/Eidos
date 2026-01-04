package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrNonceAlreadyUsed = errors.New("nonce already used")
)

// NonceRepository Nonce 仓储接口
// 用于防止签名重放攻击
type NonceRepository interface {
	// IsUsed 检查 Nonce 是否已使用
	// 先查 Redis 缓存，再查数据库
	IsUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64) (bool, error)

	// MarkUsed 标记 Nonce 已使用
	// 同时写入 Redis 和数据库
	MarkUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, orderID string) error

	// MarkUsedWithTx 在事务中标记 Nonce 已使用
	// 仅写入数据库，Redis 缓存由调用方负责
	MarkUsedWithTx(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, orderID string) error

	// GetLatestNonce 获取用户最新 Nonce
	// 用于客户端获取下一个可用 Nonce
	GetLatestNonce(ctx context.Context, wallet string, usage model.NonceUsage) (uint64, error)

	// CleanExpired 清理过期的 Nonce 记录
	// 超过 30 天的记录可以清理
	CleanExpired(ctx context.Context, beforeTime int64, batchSize int) (int64, error)
}

// nonceRepository Nonce 仓储实现
type nonceRepository struct {
	*Repository
	rdb *redis.Client
}

// NewNonceRepository 创建 Nonce 仓储
func NewNonceRepository(db *gorm.DB, rdb *redis.Client) NonceRepository {
	return &nonceRepository{
		Repository: NewRepository(db),
		rdb:        rdb,
	}
}

// nonceExpirationDays Nonce 缓存过期天数
const nonceExpirationDays = 7

// IsUsed 检查 Nonce 是否已使用
func (r *nonceRepository) IsUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64) (bool, error) {
	// 先查 Redis 缓存
	key := model.GetNonceRedisKey(wallet, usage, nonce)
	exists, err := r.rdb.Exists(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		// Redis 错误，降级到数据库查询
		return r.isUsedInDB(ctx, wallet, usage, nonce)
	}

	if exists > 0 {
		return true, nil
	}

	// Redis 中不存在，查询数据库
	return r.isUsedInDB(ctx, wallet, usage, nonce)
}

// isUsedInDB 从数据库检查 Nonce 是否已使用
func (r *nonceRepository) isUsedInDB(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64) (bool, error) {
	var count int64
	result := r.DB(ctx).Model(&model.UsedNonce{}).
		Where("wallet = ? AND usage = ? AND nonce = ?", wallet, usage, nonce).
		Count(&count)

	if result.Error != nil {
		return false, fmt.Errorf("check nonce in db failed: %w", result.Error)
	}
	return count > 0, nil
}

// MarkUsed 标记 Nonce 已使用
func (r *nonceRepository) MarkUsed(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, orderID string) error {
	// 先写入数据库
	usedNonce := &model.UsedNonce{
		Wallet:  wallet,
		Usage:   usage,
		Nonce:   nonce,
		OrderID: orderID,
	}

	result := r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet"}, {Name: "usage"}, {Name: "nonce"}},
		DoNothing: true,
	}).Create(usedNonce)

	if result.Error != nil {
		return fmt.Errorf("mark nonce used in db failed: %w", result.Error)
	}

	// 如果是重复插入，说明 Nonce 已被使用
	if result.RowsAffected == 0 {
		return ErrNonceAlreadyUsed
	}

	// 写入 Redis 缓存
	key := model.GetNonceRedisKey(wallet, usage, nonce)
	expiration := time.Duration(nonceExpirationDays) * 24 * time.Hour

	if err := r.rdb.Set(ctx, key, "1", expiration).Err(); err != nil {
		// Redis 写入失败不影响业务，只记录日志
		// 下次查询会从数据库读取
	}

	return nil
}

// MarkUsedWithTx 在事务中标记 Nonce 已使用
func (r *nonceRepository) MarkUsedWithTx(ctx context.Context, wallet string, usage model.NonceUsage, nonce uint64, orderID string) error {
	usedNonce := &model.UsedNonce{
		Wallet:  wallet,
		Usage:   usage,
		Nonce:   nonce,
		OrderID: orderID,
	}

	result := r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet"}, {Name: "usage"}, {Name: "nonce"}},
		DoNothing: true,
	}).Create(usedNonce)

	if result.Error != nil {
		return fmt.Errorf("mark nonce used failed: %w", result.Error)
	}

	// 如果是重复插入，说明 Nonce 已被使用
	if result.RowsAffected == 0 {
		return ErrNonceAlreadyUsed
	}

	return nil
}

// GetLatestNonce 获取用户最新 Nonce
func (r *nonceRepository) GetLatestNonce(ctx context.Context, wallet string, usage model.NonceUsage) (uint64, error) {
	var usedNonce model.UsedNonce
	result := r.DB(ctx).
		Where("wallet = ? AND usage = ?", wallet, usage).
		Order("nonce DESC").
		First(&usedNonce)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return 0, nil // 没有使用过，返回 0
		}
		return 0, fmt.Errorf("get latest nonce failed: %w", result.Error)
	}

	return usedNonce.Nonce, nil
}

// CleanExpired 清理过期的 Nonce 记录
func (r *nonceRepository) CleanExpired(ctx context.Context, beforeTime int64, batchSize int) (int64, error) {
	var totalDeleted int64

	for {
		result := r.DB(ctx).
			Where("created_at < ?", beforeTime).
			Limit(batchSize).
			Delete(&model.UsedNonce{})

		if result.Error != nil {
			return totalDeleted, fmt.Errorf("clean expired nonces failed: %w", result.Error)
		}

		totalDeleted += result.RowsAffected

		// 如果删除数量少于批次大小，说明已经删除完毕
		if result.RowsAffected < int64(batchSize) {
			break
		}
	}

	return totalDeleted, nil
}
