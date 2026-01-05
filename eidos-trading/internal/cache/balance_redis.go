package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// BalanceRedisRepository Redis 资金仓储接口
type BalanceRedisRepository interface {
	// GetBalance 获取余额
	GetBalance(ctx context.Context, wallet, token string) (*RedisBalance, error)

	// GetBalancesBatch 批量获取余额 (使用 Pipeline 减少 RTT)
	GetBalancesBatch(ctx context.Context, wallet string, tokens []string) ([]*RedisBalance, error)

	// GetOrCreateBalance 获取或创建余额
	GetOrCreateBalance(ctx context.Context, wallet, token string) (*RedisBalance, error)

	// Freeze 原子冻结余额 (下单时使用)
	// fromSettled: true 从已结算可用冻结，false 从待结算可用冻结
	Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool, orderID string) error

	// FreezeForOrder 原子冻结余额并记录订单冻结信息
	FreezeForOrder(ctx context.Context, req *FreezeForOrderRequest) error

	// Unfreeze 原子解冻余额 (取消订单时使用)
	Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error

	// UnfreezeByOrderID 根据订单ID解冻余额
	UnfreezeByOrderID(ctx context.Context, wallet, token, orderID string) error

	// UnfreezeForCancel 原子解冻余额并写入取消 Outbox (已废弃)
	// 用于取消订单时的原子操作: 解冻余额 + 写入 Cancel Outbox
	UnfreezeForCancel(ctx context.Context, req *UnfreezeForCancelRequest) error

	// WriteCancelOutbox 写入取消 Outbox (不解冻资金)
	// 用于发送取消请求到撮合引擎，资金将在 HandleCancelConfirm 时解冻
	WriteCancelOutbox(ctx context.Context, req *WriteCancelOutboxRequest) error

	// CancelPendingOrder 取消 PENDING 状态订单 (原子操作: 检查状态 + 解冻 + 从 Outbox 移除)
	// PENDING 订单尚未发送到撮合引擎，可以直接在 Redis 中取消
	CancelPendingOrder(ctx context.Context, req *CancelPendingOrderRequest) error

	// Credit 增加余额 (充值/成交收入)
	Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error

	// Debit 从冻结部分扣减余额 (提现/成交支出)
	Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error

	// ClearTrade 清算成交 (原子操作)
	ClearTrade(ctx context.Context, req *ClearTradeRequest) error

	// Transfer 原子转账 (从发送方冻结余额扣减，增加到接收方待结算可用)
	Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, fromSettled bool) error

	// Settle 结算 (pending → settled)
	Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error

	// GetOrderFreeze 获取订单冻结记录
	GetOrderFreeze(ctx context.Context, orderID string) (*OrderFreezeRecord, error)

	// UpdateOrderFreeze 更新订单冻结记录 (部分成交后)
	UpdateOrderFreeze(ctx context.Context, orderID string, settledAmount, pendingAmount decimal.Decimal) error

	// DeleteOrderFreeze 删除订单冻结记录
	DeleteOrderFreeze(ctx context.Context, orderID string) error

	// CreditFeeBucket 增加手续费分桶
	CreditFeeBucket(ctx context.Context, market, token string, bucketID int, amount decimal.Decimal) error

	// CheckTradeProcessed 检查成交是否已处理 (幂等检查)
	CheckTradeProcessed(ctx context.Context, tradeID string) (bool, error)

	// MarkTradeProcessed 标记成交已处理
	MarkTradeProcessed(ctx context.Context, tradeID string, ttl time.Duration) error

	// GetUserPendingTotal 获取用户待结算总额
	GetUserPendingTotal(ctx context.Context, wallet string) (decimal.Decimal, error)

	// GetGlobalPendingTotal 获取全局待结算总额
	GetGlobalPendingTotal(ctx context.Context) (decimal.Decimal, error)

	// DecrUserOpenOrders 减少用户活跃订单计数 (订单进入终态时调用)
	DecrUserOpenOrders(ctx context.Context, wallet string) error

	// GetUserOpenOrders 获取用户活跃订单数
	GetUserOpenOrders(ctx context.Context, wallet string) (int, error)

	// SyncBalanceFromDB 从数据库同步余额到 Redis (恢复时使用)
	SyncBalanceFromDB(ctx context.Context, balance *model.Balance) error

	// RollbackTrade 回滚成交 (结算失败时使用)
	// 将成交的资金变动反向操作: 从 pending 收益扣回, 退回到原来的 frozen
	RollbackTrade(ctx context.Context, req *RollbackTradeRequest) error
}

// balanceRedisRepository Redis 资金仓储实现
type balanceRedisRepository struct {
	rdb *redis.Client
}

// NewBalanceRedisRepository 创建 Redis 资金仓储
func NewBalanceRedisRepository(rdb *redis.Client) BalanceRedisRepository {
	return &balanceRedisRepository{rdb: rdb}
}

// balanceKey 生成余额 key
func balanceKey(wallet, token string) string {
	return fmt.Sprintf(balanceKeyPattern, wallet, token)
}

// orderFreezeKey 生成订单冻结记录 key
func orderFreezeKey(orderID string) string {
	return fmt.Sprintf(orderFreezeKeyPattern, orderID)
}

// tradeIdempotentKey 生成成交幂等 key
func tradeIdempotentKey(tradeID string) string {
	return fmt.Sprintf(tradeIdempotentKeyPattern, tradeID)
}

// feeBucketKey 生成手续费分桶 key
func feeBucketKey(market, token string, bucketID int) string {
	return fmt.Sprintf(feeBucketKeyPattern, market, token, bucketID)
}

// GetBalance 获取余额
func (r *balanceRedisRepository) GetBalance(ctx context.Context, wallet, token string) (*RedisBalance, error) {
	key := balanceKey(wallet, token)
	result, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis hgetall failed: %w", err)
	}

	if len(result) == 0 {
		return nil, ErrRedisBalanceNotFound
	}

	return parseRedisBalance(wallet, token, result)
}

// GetBalancesBatch 批量获取余额 (使用 Pipeline 减少 RTT)
// 返回所有查询到的余额，跳过不存在的 key
func (r *balanceRedisRepository) GetBalancesBatch(ctx context.Context, wallet string, tokens []string) ([]*RedisBalance, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	// 使用 Pipeline 批量获取
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(tokens))

	for i, token := range tokens {
		key := balanceKey(wallet, token)
		cmds[i] = pipe.HGetAll(ctx, key)
	}

	// 执行 Pipeline (单次 RTT)
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis pipeline exec failed: %w", err)
	}

	// 解析结果
	balances := make([]*RedisBalance, 0, len(tokens))
	for i, cmd := range cmds {
		result, err := cmd.Result()
		if err != nil {
			continue // 跳过错误的 key
		}
		if len(result) == 0 {
			continue // 跳过不存在的 key
		}

		balance, err := parseRedisBalance(wallet, tokens[i], result)
		if err != nil {
			continue // 跳过解析失败的 key
		}
		balances = append(balances, balance)
	}

	return balances, nil
}

// GetOrCreateBalance 获取或创建余额
func (r *balanceRedisRepository) GetOrCreateBalance(ctx context.Context, wallet, token string) (*RedisBalance, error) {
	balance, err := r.GetBalance(ctx, wallet, token)
	if err == nil {
		return balance, nil
	}
	if !errors.Is(err, ErrRedisBalanceNotFound) {
		return nil, err
	}

	// 创建新余额 (使用 HSETNX 确保原子性)
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	// 使用 Lua 脚本原子创建
	result, err := luaGetOrCreateBalance.Run(ctx, r.rdb, []string{key}, now).Slice()
	if err != nil {
		return nil, fmt.Errorf("create balance failed: %w", err)
	}

	// 解析结果
	resultMap := make(map[string]string)
	for i := 0; i < len(result); i += 2 {
		resultMap[result[i].(string)] = result[i+1].(string)
	}

	return parseRedisBalance(wallet, token, resultMap)
}

// SyncBalanceFromDB 从数据库同步余额到 Redis
func (r *balanceRedisRepository) SyncBalanceFromDB(ctx context.Context, balance *model.Balance) error {
	key := balanceKey(balance.Wallet, balance.Token)
	return r.rdb.HSet(ctx, key,
		fieldSettledAvailable, balance.SettledAvailable.String(),
		fieldSettledFrozen, balance.SettledFrozen.String(),
		fieldPendingAvailable, balance.PendingAvailable.String(),
		fieldPendingFrozen, balance.PendingFrozen.String(),
		fieldPendingTotal, balance.PendingTotal.String(),
		fieldVersion, balance.Version,
		fieldUpdatedAt, balance.UpdatedAt,
	).Err()
}

// parseRedisBalance 解析 Redis 余额数据
func parseRedisBalance(wallet, token string, data map[string]string) (*RedisBalance, error) {
	settledAvailable, err := decimal.NewFromString(data[fieldSettledAvailable])
	if err != nil {
		settledAvailable = decimal.Zero
	}

	settledFrozen, err := decimal.NewFromString(data[fieldSettledFrozen])
	if err != nil {
		settledFrozen = decimal.Zero
	}

	pendingAvailable, err := decimal.NewFromString(data[fieldPendingAvailable])
	if err != nil {
		pendingAvailable = decimal.Zero
	}

	pendingFrozen, err := decimal.NewFromString(data[fieldPendingFrozen])
	if err != nil {
		pendingFrozen = decimal.Zero
	}

	pendingTotal, err := decimal.NewFromString(data[fieldPendingTotal])
	if err != nil {
		pendingTotal = decimal.Zero
	}

	var version int64 = 1
	if v, ok := data[fieldVersion]; ok {
		fmt.Sscanf(v, "%d", &version)
	}

	var updatedAt int64
	if v, ok := data[fieldUpdatedAt]; ok {
		fmt.Sscanf(v, "%d", &updatedAt)
	}

	return &RedisBalance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: settledAvailable,
		SettledFrozen:    settledFrozen,
		PendingAvailable: pendingAvailable,
		PendingFrozen:    pendingFrozen,
		PendingTotal:     pendingTotal,
		Version:          version,
		UpdatedAt:        updatedAt,
	}, nil
}
