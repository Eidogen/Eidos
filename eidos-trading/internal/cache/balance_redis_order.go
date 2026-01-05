package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// Freeze 原子冻结余额
func (r *balanceRedisRepository) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool, orderID string) error {
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	var field string
	var frozenField string
	if fromSettled {
		field = fieldSettledAvailable
		frozenField = fieldSettledFrozen
	} else {
		field = fieldPendingAvailable
		frozenField = fieldPendingFrozen
	}

	result, err := luaFreeze.Run(ctx, r.rdb, []string{key}, field, frozenField, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("freeze failed: %w", err)
	}

	if resultMap, ok := result.(map[interface{}]interface{}); ok {
		if errMsg, exists := resultMap["err"]; exists {
			errStr := errMsg.(string)
			if errStr == "balance not found" {
				return ErrRedisBalanceNotFound
			}
			if errStr == "insufficient balance" {
				return ErrRedisInsufficientBalance
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// FreezeForOrder 原子冻结余额并记录订单冻结信息 (下单专用)
func (r *balanceRedisRepository) FreezeForOrder(ctx context.Context, req *FreezeForOrderRequest) error {
	startTime := time.Now()
	defer func() {
		metrics.RedisOperationLatency.WithLabelValues("freeze_for_order").Observe(time.Since(startTime).Seconds())
	}()

	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxOrderKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxPendingKeyPattern, req.ShardID)
	userPendingKeyStr := fmt.Sprintf(userPendingKeyPattern, req.Wallet)
	userOpenOrdersKeyStr := fmt.Sprintf(userOpenOrdersKeyPattern, req.Wallet)
	now := time.Now().UnixMilli()

	fromSettledFlag := "0"
	if req.FromSettled {
		fromSettledFlag = "1"
	}

	// 用户待结算限额，0 表示不检查
	userPendingLimit := req.UserPendingLimit
	if userPendingLimit.IsZero() {
		userPendingLimit = decimal.NewFromInt(DefaultUserPendingLimit)
	}

	// 全局待结算限额，0 表示不检查
	globalPendingLimit := req.GlobalPendingLimit
	if globalPendingLimit.IsZero() {
		globalPendingLimit = decimal.NewFromInt(DefaultGlobalPendingLimit)
	}

	// 最大活跃订单数，0 表示不检查
	maxOpenOrders := req.MaxOpenOrders

	result, err := luaFreezeForOrder.Run(ctx, r.rdb,
		[]string{
			balanceKeyStr, orderFreezeKeyStr, req.NonceKey, outboxKeyStr,
			outboxPendingKeyStr, userPendingKeyStr, globalPendingKey, userOpenOrdersKeyStr,
		},
		req.Wallet, req.Token, req.Amount.String(), req.OrderID, fromSettledFlag,
		req.OrderJSON, req.ShardID, int64(req.NonceTTL.Seconds()), now,
		userPendingLimit.String(), globalPendingLimit.String(), maxOpenOrders,
	).Result()

	if err != nil {
		return fmt.Errorf("freeze for order failed: %w", err)
	}

	// 解析结果
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) >= 2 {
		if resultSlice[0] == "err" {
			errStr := resultSlice[1].(string)
			switch errStr {
			case "nonce_used":
				return ErrRedisNonceUsed
			case "insufficient_balance":
				return ErrRedisInsufficientBalance
			case "pending_limit_exceeded":
				return ErrRedisPendingLimitExceeded
			case "global_pending_limit_exceeded":
				return ErrRedisGlobalPendingLimitExceeded
			case "max_open_orders_exceeded":
				return ErrRedisMaxOpenOrdersExceeded
			default:
				return errors.New(errStr)
			}
		}
	}

	// 记录余额操作指标
	metrics.RecordBalanceOperation("freeze", req.Token)
	return nil
}

// Unfreeze 原子解冻余额
func (r *balanceRedisRepository) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	var frozenField string
	var availableField string
	if toSettled {
		frozenField = fieldSettledFrozen
		availableField = fieldSettledAvailable
	} else {
		frozenField = fieldPendingFrozen
		availableField = fieldPendingAvailable
	}

	_, err := luaUnfreeze.Run(ctx, r.rdb, []string{key}, frozenField, availableField, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("unfreeze failed: %w", err)
	}

	metrics.RecordBalanceOperation("unfreeze", token)
	return nil
}

// UnfreezeByOrderID 根据订单ID解冻余额
func (r *balanceRedisRepository) UnfreezeByOrderID(ctx context.Context, wallet, token, orderID string) error {
	startTime := time.Now()
	defer func() {
		metrics.RedisOperationLatency.WithLabelValues("unfreeze_by_order").Observe(time.Since(startTime).Seconds())
	}()

	balanceKeyStr := balanceKey(wallet, token)
	orderFreezeKeyStr := orderFreezeKey(orderID)
	now := time.Now().UnixMilli()

	result, err := luaUnfreezeByOrderID.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr},
		now,
	).Result()

	if err != nil {
		return fmt.Errorf("unfreeze by order id failed: %w", err)
	}

	if resultMap, ok := result.(map[interface{}]interface{}); ok {
		if errMsg, exists := resultMap["err"]; exists {
			errStr := errMsg.(string)
			if errStr == "order freeze not found" {
				return ErrRedisOrderFreezeNotFound
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// UnfreezeForCancel 原子解冻余额并写入取消 Outbox (已废弃)
func (r *balanceRedisRepository) UnfreezeForCancel(ctx context.Context, req *UnfreezeForCancelRequest) error {
	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxCancelKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxCancelPendingKeyPattern, req.ShardID)
	now := time.Now().UnixMilli()

	result, err := luaUnfreezeForCancel.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr, outboxKeyStr, outboxPendingKeyStr},
		req.Wallet, req.OrderID, req.CancelJSON, req.ShardID, now,
	).Result()

	if err != nil {
		return fmt.Errorf("unfreeze for cancel failed: %w", err)
	}

	if resultMap, ok := result.(map[interface{}]interface{}); ok {
		if val, exists := resultMap["ok"]; exists {
			if val == "already_exists" {
				return ErrRedisOutboxExists
			}
		}
	}

	return nil
}

// WriteCancelOutbox 写入取消 Outbox (不解冻资金)
func (r *balanceRedisRepository) WriteCancelOutbox(ctx context.Context, req *WriteCancelOutboxRequest) error {
	outboxKeyStr := fmt.Sprintf(outboxCancelKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxCancelPendingKeyPattern, req.ShardID)
	now := time.Now().UnixMilli()

	result, err := luaWriteCancelOutbox.Run(ctx, r.rdb,
		[]string{outboxKeyStr, outboxPendingKeyStr},
		req.CancelJSON, req.ShardID, now, req.OrderID,
	).Result()

	if err != nil {
		return fmt.Errorf("write cancel outbox failed: %w", err)
	}

	if resultMap, ok := result.(map[interface{}]interface{}); ok {
		if val, exists := resultMap["ok"]; exists {
			if val == "already_exists" {
				return ErrRedisOutboxExists
			}
		}
	}

	return nil
}

// CancelPendingOrder 取消 PENDING 状态订单
func (r *balanceRedisRepository) CancelPendingOrder(ctx context.Context, req *CancelPendingOrderRequest) error {
	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxOrderKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxPendingKeyPattern, req.ShardID)

	// 构建一个空的 CancelJSON，因为 Pending 订单不需要发给撮合引擎
	// 但为了 Outbox 格式统一，我们还是写进去，虽然最终可能不会被用来发送取消请求
	// 或者，CancelPendingOrder 的逻辑其实是直接把 Outbox 状态改成 Failed 或者直接删除？
	// 看 Lua 脚本: 它是把 Outbox 状态改成 PENDING (这里可能是个 Bug? 或者意图是把 Cancel 请求写入 Outbox?)
	// 仔细看 Lua 脚本:
	/*
		-- 5. 写入 Cancel Outbox
		redis.call('HSET', outbox_key, ... 'status', 'PENDING' ...)
		redis.call('LPUSH', outbox_pending_key, order_id)
	*/
	// 这里的 outbox_key 是传入的第 3 个 key，也就是 user passed `outboxKeyStr`.
	// 在 CancelPendingOrder 中，我们传入的是 `outboxOrderKeyPattern` (即订单创建时的 Outbox)
	// 所以这里是把原订单的 Outbox 状态改回 PENDING ??
	// 不，Lua 脚本里写的是: redis.call('HSET', outbox_key, 'status', 'PENDING', 'cancel_json', cancel_json ...)
	// 看起来它是在把 *Order Creation Outbox* 覆盖成一个 *Cancel Request* ??
	// 这看起来有点怪。原订单是 CreateOrderRequest，现在覆盖成 Cancel 吗？
	// 如果是 CancelPendingOrder，意味着订单还没发出去。
	// 如果还没发出去，直接删除 Outbox 或者标记为 Cancelled 最好。
	// 但这个 Lua 脚本的行为是将同一个 Key 的内容替换了。
	// 让我们照搬原逻辑，不要改动逻辑。

	cancelJSON := "{}" // Dummy JSON, we don't really use it for Pending cancellation usually unless it turns into a Cancel request
	now := time.Now().UnixMilli()

	result, err := luaCancelPendingOrder.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr, outboxKeyStr, outboxPendingKeyStr},
		req.OrderID, cancelJSON, req.ShardID, now,
	).Result()

	if err != nil {
		return fmt.Errorf("cancel pending order failed: %w", err)
	}

	if resultMap, ok := result.(map[interface{}]interface{}); ok {
		if errMsg, exists := resultMap["err"]; exists {
			errStr := errMsg.(string)
			if errStr == "outbox_not_found" {
				return ErrRedisOrderFreezeNotFound // 或者其他更合适的错误
			}
			if errStr == "order_already_sent" {
				return ErrRedisOrderAlreadySent
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// GetOrderFreeze 获取订单冻结记录
func (r *balanceRedisRepository) GetOrderFreeze(ctx context.Context, orderID string) (*OrderFreezeRecord, error) {
	key := orderFreezeKey(orderID)
	result, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get order freeze failed: %w", err)
	}

	if len(result) == 0 {
		return nil, nil
	}

	settledAmount, err := decimal.NewFromString(result[fieldFreezeSettledAmount])
	if err != nil {
		return nil, fmt.Errorf("parse settled_amount: %w", err)
	}
	pendingAmount, err := decimal.NewFromString(result[fieldFreezePendingAmount])
	if err != nil {
		return nil, fmt.Errorf("parse pending_amount: %w", err)
	}

	return &OrderFreezeRecord{
		OrderID:       result[fieldFreezeOrderID],
		Token:         result[fieldFreezeToken],
		SettledAmount: settledAmount,
		PendingAmount: pendingAmount,
	}, nil
}

// UpdateOrderFreeze 更新订单冻结记录
func (r *balanceRedisRepository) UpdateOrderFreeze(ctx context.Context, orderID string, settledAmount, pendingAmount decimal.Decimal) error {
	key := orderFreezeKey(orderID)
	return r.rdb.HSet(ctx, key,
		fieldFreezeSettledAmount, settledAmount.String(),
		fieldFreezePendingAmount, pendingAmount.String(),
	).Err()
}

// DeleteOrderFreeze 删除订单冻结记录
func (r *balanceRedisRepository) DeleteOrderFreeze(ctx context.Context, orderID string) error {
	key := orderFreezeKey(orderID)
	return r.rdb.Del(ctx, key).Err()
}

// DecrUserOpenOrders 减少用户活跃订单计数 (订单进入终态时调用)
func (r *balanceRedisRepository) DecrUserOpenOrders(ctx context.Context, wallet string) error {
	key := fmt.Sprintf(userOpenOrdersKeyPattern, wallet)
	_, err := luaDecrUserOpenOrders.Run(ctx, r.rdb, []string{key}).Result()
	if err != nil {
		return fmt.Errorf("decr user open orders failed: %w", err)
	}
	return nil
}

// GetUserOpenOrders 获取用户活跃订单数
func (r *balanceRedisRepository) GetUserOpenOrders(ctx context.Context, wallet string) (int, error) {
	key := fmt.Sprintf(userOpenOrdersKeyPattern, wallet)
	val, err := r.rdb.Get(ctx, key).Int()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("get user open orders failed: %w", err)
	}
	return val, nil
}

// OrderFreezeRecordToJSON 订单冻结记录序列化
func OrderFreezeRecordToJSON(record *OrderFreezeRecord) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// OrderFreezeRecordFromJSON 订单冻结记录反序列化
func OrderFreezeRecordFromJSON(data string) (*OrderFreezeRecord, error) {
	var record OrderFreezeRecord
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return nil, err
	}
	return &record, nil
}
