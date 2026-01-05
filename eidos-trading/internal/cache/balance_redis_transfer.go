package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// Credit 增加余额
func (r *balanceRedisRepository) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	var field string
	if toSettled {
		field = fieldSettledAvailable
	} else {
		field = fieldPendingAvailable
	}

	// 是否增加 pending_total (只有增加到 pending_available 时才增加)
	updatePendingTotal := "0"
	if !toSettled {
		updatePendingTotal = "1"
	}

	_, err := luaCredit.Run(ctx, r.rdb, []string{key}, field, amount.String(), now, updatePendingTotal).Result()
	if err != nil {
		return fmt.Errorf("credit failed: %w", err)
	}

	metrics.RecordBalanceOperation("credit", token)
	return nil
}

// Debit 从冻结部分扣减余额
func (r *balanceRedisRepository) Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error {
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	var field string
	if fromSettled {
		field = fieldSettledFrozen
	} else {
		field = fieldPendingFrozen
	}

	_, err := luaDebit.Run(ctx, r.rdb, []string{key}, field, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("debit failed: %w", err)
	}

	metrics.RecordBalanceOperation("debit", token)
	return nil
}

// Transfer 原子转账 (从发送方冻结余额扣减，增加到接收方待结算可用)
func (r *balanceRedisRepository) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, fromSettled bool) error {
	fromKey := balanceKey(fromWallet, token)
	toKey := balanceKey(toWallet, token)
	now := time.Now().UnixMilli()

	var frozenField string
	if fromSettled {
		frozenField = fieldSettledFrozen
	} else {
		frozenField = fieldPendingFrozen
	}

	// Lua 脚本原子转账: 从发送方冻结扣减 + 增加到接收方待结算可用
	result, err := luaTransfer.Run(ctx, r.rdb, []string{fromKey, toKey}, frozenField, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) >= 2 {
		if resultSlice[0] == "err" {
			errStr := resultSlice[1].(string)
			if errStr == "insufficient frozen balance" {
				return ErrRedisInsufficientBalance
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// Settle 结算 (pending → settled)
func (r *balanceRedisRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	startTime := time.Now()
	defer func() {
		metrics.RedisOperationLatency.WithLabelValues("settle").Observe(time.Since(startTime).Seconds())
	}()

	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	_, err := luaSettle.Run(ctx, r.rdb, []string{key}, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("settle failed: %w", err)
	}

	metrics.RecordBalanceOperation("settle", token)
	return nil
}

// GetUserPendingTotal 获取用户待结算总额
func (r *balanceRedisRepository) GetUserPendingTotal(ctx context.Context, wallet string) (decimal.Decimal, error) {
	key := fmt.Sprintf(userPendingKeyPattern, wallet)
	result, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return decimal.Zero, nil
		}
		return decimal.Zero, fmt.Errorf("get user pending total failed: %w", err)
	}
	return decimal.NewFromString(result)
}

// GetGlobalPendingTotal 获取全局待结算总额
func (r *balanceRedisRepository) GetGlobalPendingTotal(ctx context.Context) (decimal.Decimal, error) {
	result, err := r.rdb.Get(ctx, globalPendingKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return decimal.Zero, nil
		}
		return decimal.Zero, fmt.Errorf("get global pending total failed: %w", err)
	}
	return decimal.NewFromString(result)
}
