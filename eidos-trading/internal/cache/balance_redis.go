package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

// Redis key patterns
const (
	// 余额 key: eidos:trading:balance:{wallet}:{token}
	balanceKeyPattern = "eidos:trading:balance:%s:%s"
	// 订单冻结记录 key: eidos:trading:order_freeze:{order_id}
	orderFreezeKeyPattern = "eidos:trading:order_freeze:%s"
	// 成交幂等 key: eidos:trading:trade:{trade_id}
	tradeIdempotentKeyPattern = "eidos:trading:trade:%s"
	// 用户待结算总额 key: eidos:trading:pending:{wallet}
	userPendingKeyPattern = "eidos:trading:pending:%s"
	// 全局待结算总额 key
	globalPendingKey = "eidos:trading:global:pending:total"
	// 手续费分桶 key: eidos:trading:fee_bucket:{market}:{token}:{bucket_id}
	feeBucketKeyPattern = "eidos:trading:fee_bucket:%s:%s:%d"
	// Outbox key patterns (Order)
	outboxOrderKeyPattern   = "eidos:trading:outbox:order:%s"
	outboxPendingKeyPattern = "eidos:trading:outbox:pending:%d"
	// Cancel Outbox key patterns
	outboxCancelKeyPattern        = "eidos:trading:outbox:cancel:%s"
	outboxCancelPendingKeyPattern = "eidos:trading:outbox:cancel:pending:%d"
)

// Balance Redis field names
const (
	fieldSettledAvailable = "settled_available"
	fieldSettledFrozen    = "settled_frozen"
	fieldPendingAvailable = "pending_available"
	fieldPendingFrozen    = "pending_frozen"
	fieldPendingTotal     = "pending_total" // 累计待结算总额 (用于风控限额，只增不减)
	fieldVersion          = "version"
	fieldUpdatedAt        = "updated_at"
)

// OrderFreeze Redis field names
const (
	fieldFreezeSettledAmount = "settled_amount"
	fieldFreezePendingAmount = "pending_amount"
	fieldFreezeToken         = "token"
	fieldFreezeOrderID       = "order_id"
)

// Pending Limit 配置
const (
	// 单用户待结算余额上限 (USDC 等值)
	DefaultUserPendingLimit = 10000
	// 全局待结算余额上限 (USDC 等值)
	DefaultGlobalPendingLimit = 1000000
)

var (
	ErrRedisBalanceNotFound      = errors.New("redis balance not found")
	ErrRedisInsufficientBalance  = errors.New("redis insufficient balance")
	ErrRedisNonceUsed            = errors.New("redis nonce already used")
	ErrRedisTradeProcessed       = errors.New("redis trade already processed")
	ErrRedisOutboxExists         = errors.New("redis outbox already exists")
	ErrRedisOrderAlreadySent     = errors.New("redis order already sent to matching")
	ErrRedisPendingLimitExceeded = errors.New("redis pending limit exceeded")
	ErrRedisOrderFreezeNotFound  = errors.New("redis order freeze not found")
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

	// SyncBalanceFromDB 从数据库同步余额到 Redis (恢复时使用)
	SyncBalanceFromDB(ctx context.Context, balance *model.Balance) error

	// RollbackTrade 回滚成交 (结算失败时使用)
	// 将成交的资金变动反向操作: 从 pending 收益扣回, 退回到原来的 frozen
	RollbackTrade(ctx context.Context, req *RollbackTradeRequest) error
}

// RedisBalance Redis 中的余额结构
type RedisBalance struct {
	Wallet           string          `json:"wallet"`
	Token            string          `json:"token"`
	SettledAvailable decimal.Decimal `json:"settled_available"`
	SettledFrozen    decimal.Decimal `json:"settled_frozen"`
	PendingAvailable decimal.Decimal `json:"pending_available"`
	PendingFrozen    decimal.Decimal `json:"pending_frozen"`
	PendingTotal     decimal.Decimal `json:"pending_total"` // 累计待结算总额 (用于风控限额)
	Version          int64           `json:"version"`
	UpdatedAt        int64           `json:"updated_at"`
}

// TotalAvailable 返回总可用余额
func (b *RedisBalance) TotalAvailable() decimal.Decimal {
	return b.SettledAvailable.Add(b.PendingAvailable)
}

// Total 返回总余额
func (b *RedisBalance) Total() decimal.Decimal {
	return b.SettledAvailable.Add(b.SettledFrozen).Add(b.PendingAvailable).Add(b.PendingFrozen)
}

// ToModel 转换为数据库模型
func (b *RedisBalance) ToModel() *model.Balance {
	return &model.Balance{
		Wallet:           b.Wallet,
		Token:            b.Token,
		SettledAvailable: b.SettledAvailable,
		SettledFrozen:    b.SettledFrozen,
		PendingAvailable: b.PendingAvailable,
		PendingFrozen:    b.PendingFrozen,
		PendingTotal:     b.PendingTotal,
		Version:          b.Version,
		UpdatedAt:        b.UpdatedAt,
	}
}

// OrderFreezeRecord 订单冻结记录
type OrderFreezeRecord struct {
	OrderID       string          `json:"order_id"`
	Token         string          `json:"token"`
	SettledAmount decimal.Decimal `json:"settled_amount"` // 从已结算冻结的金额
	PendingAmount decimal.Decimal `json:"pending_amount"` // 从待结算冻结的金额
}

// TotalFrozen 返回总冻结金额
func (r *OrderFreezeRecord) TotalFrozen() decimal.Decimal {
	return r.SettledAmount.Add(r.PendingAmount)
}

// FreezeForOrderRequest 下单冻结请求
type FreezeForOrderRequest struct {
	Wallet           string          // 钱包地址
	Token            string          // 代币
	Amount           decimal.Decimal // 冻结金额
	OrderID          string          // 订单 ID
	FromSettled      bool            // 优先从已结算冻结
	OrderJSON        string          // 订单 JSON (用于 Outbox)
	ShardID          int             // Outbox 分片 ID
	NonceKey         string          // Nonce key (用于标记已使用)
	NonceTTL         time.Duration   // Nonce TTL
	UserPendingLimit decimal.Decimal // 用户待结算限额 (0 表示不检查)
}

// ClearTradeRequest 清算请求
type ClearTradeRequest struct {
	TradeID      string          // 成交 ID
	MakerWallet  string          // Maker 钱包
	TakerWallet  string          // Taker 钱包
	MakerOrderID string          // Maker 订单 ID
	TakerOrderID string          // Taker 订单 ID
	BaseToken    string          // 基础代币
	QuoteToken   string          // 计价代币
	Price        decimal.Decimal // 成交价格
	Amount       decimal.Decimal // 成交数量
	QuoteAmount  decimal.Decimal // 成交金额
	MakerFee     decimal.Decimal // Maker 手续费
	TakerFee     decimal.Decimal // Taker 手续费
	FeeToken     string          // 手续费代币
	MakerIsBuy   bool            // Maker 是买方
	Market       string          // 市场
	FeeBucketID  int             // 手续费分桶 ID
}

// UnfreezeForCancelRequest 取消订单解冻请求 (已废弃，使用 WriteCancelOutboxRequest)
type UnfreezeForCancelRequest struct {
	Wallet     string // 钱包地址
	Token      string // 代币
	OrderID    string // 订单 ID
	Market     string // 交易对 (用于 Kafka partition key)
	CancelJSON string // 取消请求 JSON (用于 Outbox)
	ShardID    int    // Outbox 分片 ID
}

// WriteCancelOutboxRequest 写入取消 Outbox 请求 (不解冻资金)
type WriteCancelOutboxRequest struct {
	OrderID    string // 订单 ID
	Market     string // 交易对 (用于 Kafka partition key)
	CancelJSON string // 取消请求 JSON (用于 Outbox)
	ShardID    int    // Outbox 分片 ID
}

// CancelPendingOrderRequest 取消 PENDING 状态订单请求
// PENDING 订单尚未发送到撮合引擎，可以直接在 Redis 中原子取消
type CancelPendingOrderRequest struct {
	Wallet  string // 钱包地址
	Token   string // 冻结的代币
	OrderID string // 订单 ID
	ShardID int    // Outbox 分片 ID
}

// RollbackTradeRequest 回滚成交请求
// 结算失败时，将成交的资金变动反向操作
type RollbackTradeRequest struct {
	TradeID     string          // 成交 ID
	MakerWallet string          // Maker 钱包
	TakerWallet string          // Taker 钱包
	BaseToken   string          // 基础代币
	QuoteToken  string          // 计价代币
	Amount      decimal.Decimal // 成交数量 (base)
	QuoteAmount decimal.Decimal // 成交金额 (quote)
	MakerFee    decimal.Decimal // Maker 手续费
	TakerFee    decimal.Decimal // Taker 手续费
	MakerIsBuy  bool            // Maker 是买方
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
	script := redis.NewScript(`
		local key = KEYS[1]
		local exists = redis.call('EXISTS', key)
		if exists == 1 then
			return redis.call('HGETALL', key)
		end
		redis.call('HSET', key,
			'settled_available', '0',
			'settled_frozen', '0',
			'pending_available', '0',
			'pending_frozen', '0',
			'pending_total', '0',
			'version', '1',
			'updated_at', ARGV[1]
		)
		return redis.call('HGETALL', key)
	`)

	result, err := script.Run(ctx, r.rdb, []string{key}, now).Slice()
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

	// Lua 脚本原子冻结
	script := redis.NewScript(`
		local key = KEYS[1]
		local available_field = ARGV[1]
		local frozen_field = ARGV[2]
		local amount = ARGV[3]
		local now = ARGV[4]

		local available = redis.call('HGET', key, available_field)
		if not available then
			return {err = 'balance not found'}
		end

		available = tonumber(available)
		local freeze_amount = tonumber(amount)

		if available < freeze_amount then
			return {err = 'insufficient balance'}
		end

		redis.call('HINCRBYFLOAT', key, available_field, -freeze_amount)
		redis.call('HINCRBYFLOAT', key, frozen_field, freeze_amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	result, err := script.Run(ctx, r.rdb, []string{key}, field, frozenField, amount.String(), now).Result()
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
// 包含待结算限额检查: 如果使用待结算余额冻结，检查用户待结算总额是否超限
func (r *balanceRedisRepository) FreezeForOrder(ctx context.Context, req *FreezeForOrderRequest) error {
	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxOrderKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxPendingKeyPattern, req.ShardID)
	userPendingKeyStr := fmt.Sprintf(userPendingKeyPattern, req.Wallet)
	now := time.Now().UnixMilli()

	// Lua 脚本: 原子执行冻结 + Nonce标记 + Outbox写入 + 待结算限额检查
	script := redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local nonce_key = KEYS[3]
		local outbox_key = KEYS[4]
		local outbox_pending_key = KEYS[5]
		local user_pending_key = KEYS[6]

		local wallet = ARGV[1]
		local token = ARGV[2]
		local amount = tonumber(ARGV[3])
		local order_id = ARGV[4]
		local from_settled = ARGV[5] == '1'
		local order_json = ARGV[6]
		local shard_id = ARGV[7]
		local nonce_ttl = tonumber(ARGV[8])
		local now = ARGV[9]
		local user_pending_limit = tonumber(ARGV[10])

		-- 1. 检查 Nonce 是否已使用
		if redis.call('EXISTS', nonce_key) == 1 then
			return {'err', 'nonce_used'}
		end

		-- 2. 检查余额
		local exists = redis.call('EXISTS', balance_key)
		if exists == 0 then
			-- 创建新余额
			redis.call('HSET', balance_key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		local settled_available = tonumber(redis.call('HGET', balance_key, 'settled_available') or '0')
		local pending_available = tonumber(redis.call('HGET', balance_key, 'pending_available') or '0')
		local pending_frozen = tonumber(redis.call('HGET', balance_key, 'pending_frozen') or '0')

		-- 3. 计算冻结来源
		local settled_freeze = 0
		local pending_freeze = 0

		if from_settled then
			-- 优先从已结算冻结
			if settled_available >= amount then
				settled_freeze = amount
			elseif settled_available + pending_available >= amount then
				settled_freeze = settled_available
				pending_freeze = amount - settled_available
			else
				return {'err', 'insufficient_balance'}
			end
		else
			-- 优先从待结算冻结
			if pending_available >= amount then
				pending_freeze = amount
			elseif settled_available + pending_available >= amount then
				pending_freeze = pending_available
				settled_freeze = amount - pending_available
			else
				return {'err', 'insufficient_balance'}
			end
		end

		-- 4. 待结算限额检查 (如果使用了待结算余额)
		if user_pending_limit > 0 and pending_freeze > 0 then
			-- 使用 pending_total 字段进行限额检查
			-- pending_total 是累计待结算总额 (只增不减)，用于风控限额
			-- 与 pending_available + pending_frozen 不同，结算后不会减少
			local pending_total = tonumber(redis.call('HGET', balance_key, 'pending_total') or '0')
			if pending_total > user_pending_limit then
				return {'err', 'pending_limit_exceeded'}
			end
		end

		-- 5. 执行冻结
		if settled_freeze > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', -settled_freeze)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', settled_freeze)
		end
		if pending_freeze > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', -pending_freeze)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', pending_freeze)
		end
		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 6. 记录订单冻结信息
		redis.call('HSET', order_freeze_key,
			'order_id', order_id,
			'token', token,
			'settled_amount', tostring(settled_freeze),
			'pending_amount', tostring(pending_freeze)
		)

		-- 7. 标记 Nonce 已使用
		redis.call('SETEX', nonce_key, nonce_ttl, '1')

		-- 8. 写入 Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'order_json', order_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		return {'ok', 'success', tostring(settled_freeze), tostring(pending_freeze)}
	`)

	fromSettledFlag := "0"
	if req.FromSettled {
		fromSettledFlag = "1"
	}

	// 用户待结算限额，0 表示不检查
	userPendingLimit := req.UserPendingLimit
	if userPendingLimit.IsZero() {
		userPendingLimit = decimal.NewFromInt(DefaultUserPendingLimit)
	}

	result, err := script.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr, req.NonceKey, outboxKeyStr, outboxPendingKeyStr, userPendingKeyStr},
		req.Wallet, req.Token, req.Amount.String(), req.OrderID, fromSettledFlag,
		req.OrderJSON, req.ShardID, int64(req.NonceTTL.Seconds()), now, userPendingLimit.String(),
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
			default:
				return errors.New(errStr)
			}
		}
	}

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

	script := redis.NewScript(`
		local key = KEYS[1]
		local frozen_field = ARGV[1]
		local available_field = ARGV[2]
		local amount = tonumber(ARGV[3])
		local now = ARGV[4]

		local frozen = tonumber(redis.call('HGET', key, frozen_field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		redis.call('HINCRBYFLOAT', key, frozen_field, -amount)
		redis.call('HINCRBYFLOAT', key, available_field, amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb, []string{key}, frozenField, availableField, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("unfreeze failed: %w", err)
	}

	return nil
}

// UnfreezeByOrderID 根据订单ID解冻余额
func (r *balanceRedisRepository) UnfreezeByOrderID(ctx context.Context, wallet, token, orderID string) error {
	balanceKeyStr := balanceKey(wallet, token)
	orderFreezeKeyStr := orderFreezeKey(orderID)
	now := time.Now().UnixMilli()

	script := redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local now = ARGV[1]

		-- 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data == 0 then
			return {err = 'order freeze not found'}
		end

		local freeze = {}
		for i = 1, #freeze_data, 2 do
			freeze[freeze_data[i]] = freeze_data[i+1]
		end

		local settled_amount = tonumber(freeze['settled_amount'] or '0')
		local pending_amount = tonumber(freeze['pending_amount'] or '0')

		-- 解冻
		if settled_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
		end
		if pending_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
		end

		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 删除订单冻结记录
		redis.call('DEL', order_freeze_key)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb, []string{balanceKeyStr, orderFreezeKeyStr}, now).Result()
	if err != nil {
		return fmt.Errorf("unfreeze by order id failed: %w", err)
	}

	return nil
}

// WriteCancelOutbox 写入取消 Outbox (不解冻资金)
// 用于发送取消请求到撮合引擎，资金将在 HandleCancelConfirm 时解冻
func (r *balanceRedisRepository) WriteCancelOutbox(ctx context.Context, req *WriteCancelOutboxRequest) error {
	outboxKeyStr := fmt.Sprintf(outboxCancelKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxCancelPendingKeyPattern, req.ShardID)
	now := time.Now().UnixMilli()

	// Lua 脚本: 幂等写入 Cancel Outbox
	script := redis.NewScript(`
		local outbox_key = KEYS[1]
		local outbox_pending_key = KEYS[2]

		local order_id = ARGV[1]
		local cancel_json = ARGV[2]
		local shard_id = ARGV[3]
		local now = ARGV[4]

		-- 幂等检查: 如果已存在则跳过
		if redis.call('EXISTS', outbox_key) == 1 then
			return {ok = 'already_exists'}
		end

		-- 写入 Cancel Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'cancel_json', cancel_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb,
		[]string{outboxKeyStr, outboxPendingKeyStr},
		req.OrderID, req.CancelJSON, req.ShardID, now,
	).Result()

	if err != nil {
		return fmt.Errorf("write cancel outbox failed: %w", err)
	}

	return nil
}

// CancelPendingOrder 取消 PENDING 状态订单 (原子操作)
// PENDING 订单尚未发送到撮合引擎，可以直接在 Redis 中取消:
// 1. 检查 Order Outbox 状态是否为 PENDING (未发送)
// 2. 从 Outbox pending 队列移除
// 3. 删除 Outbox Hash
// 4. 解冻余额
// 5. 删除订单冻结记录
func (r *balanceRedisRepository) CancelPendingOrder(ctx context.Context, req *CancelPendingOrderRequest) error {
	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxOrderKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxPendingKeyPattern, req.ShardID)
	now := time.Now().UnixMilli()

	// Lua 脚本: 原子取消 PENDING 订单
	script := redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local outbox_key = KEYS[3]
		local outbox_pending_key = KEYS[4]

		local order_id = ARGV[1]
		local now = ARGV[2]

		-- 1. 检查 Outbox 状态
		local outbox_status = redis.call('HGET', outbox_key, 'status')
		if not outbox_status then
			-- Outbox 不存在，可能已经被处理或从未创建
			return {err = 'outbox_not_found'}
		end

		if outbox_status ~= 'PENDING' then
			-- 订单已发送到撮合引擎，不能直接取消
			return {err = 'order_already_sent'}
		end

		-- 2. 从 pending 队列移除
		redis.call('LREM', outbox_pending_key, 0, order_id)

		-- 3. 删除 Outbox Hash
		redis.call('DEL', outbox_key)

		-- 4. 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data > 0 then
			local freeze = {}
			for i = 1, #freeze_data, 2 do
				freeze[freeze_data[i]] = freeze_data[i+1]
			end

			local settled_amount = tonumber(freeze['settled_amount'] or '0')
			local pending_amount = tonumber(freeze['pending_amount'] or '0')

			-- 5. 解冻余额
			if settled_amount > 0 then
				redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
				redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
			end
			if pending_amount > 0 then
				redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
				redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
			end

			redis.call('HINCRBY', balance_key, 'version', 1)
			redis.call('HSET', balance_key, 'updated_at', now)

			-- 6. 删除订单冻结记录
			redis.call('DEL', order_freeze_key)
		end

		return {ok = 'success'}
	`)

	result, err := script.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr, outboxKeyStr, outboxPendingKeyStr},
		req.OrderID, now,
	).Result()

	if err != nil {
		return fmt.Errorf("cancel pending order failed: %w", err)
	}

	// 解析结果
	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) >= 2 {
		if resultSlice[0] == "err" {
			errStr := resultSlice[1].(string)
			switch errStr {
			case "outbox_not_found":
				return ErrRedisOrderFreezeNotFound
			case "order_already_sent":
				return ErrRedisOrderAlreadySent
			default:
				return errors.New(errStr)
			}
		}
	}

	return nil
}

// UnfreezeForCancel 原子解冻余额并写入取消 Outbox (已废弃，保留向后兼容)
// 用于取消订单时: 解冻余额 + 写入 Cancel Outbox (保证原子性)
func (r *balanceRedisRepository) UnfreezeForCancel(ctx context.Context, req *UnfreezeForCancelRequest) error {
	balanceKeyStr := balanceKey(req.Wallet, req.Token)
	orderFreezeKeyStr := orderFreezeKey(req.OrderID)
	outboxKeyStr := fmt.Sprintf(outboxCancelKeyPattern, req.OrderID)
	outboxPendingKeyStr := fmt.Sprintf(outboxCancelPendingKeyPattern, req.ShardID)
	now := time.Now().UnixMilli()

	// Lua 脚本: 原子执行解冻 + Outbox写入
	script := redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local outbox_key = KEYS[3]
		local outbox_pending_key = KEYS[4]

		local order_id = ARGV[1]
		local cancel_json = ARGV[2]
		local shard_id = ARGV[3]
		local now = ARGV[4]

		-- 1. 检查是否已经有取消 outbox (幂等检查)
		if redis.call('EXISTS', outbox_key) == 1 then
			return {ok = 'already_exists'}
		end

		-- 2. 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data == 0 then
			-- 没有冻结记录，可能已被处理，仍然写入 outbox
			redis.call('HSET', outbox_key,
				'status', 'PENDING',
				'cancel_json', cancel_json,
				'shard', shard_id,
				'retry_count', '0',
				'created_at', now,
				'updated_at', now
			)
			redis.call('LPUSH', outbox_pending_key, order_id)
			return {ok = 'success_no_freeze'}
		end

		local freeze = {}
		for i = 1, #freeze_data, 2 do
			freeze[freeze_data[i]] = freeze_data[i+1]
		end

		local settled_amount = tonumber(freeze['settled_amount'] or '0')
		local pending_amount = tonumber(freeze['pending_amount'] or '0')

		-- 3. 解冻余额
		if settled_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
		end
		if pending_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
		end

		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 4. 删除订单冻结记录
		redis.call('DEL', order_freeze_key)

		-- 5. 写入 Cancel Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'cancel_json', cancel_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb,
		[]string{balanceKeyStr, orderFreezeKeyStr, outboxKeyStr, outboxPendingKeyStr},
		req.OrderID, req.CancelJSON, req.ShardID, now,
	).Result()

	if err != nil {
		return fmt.Errorf("unfreeze for cancel failed: %w", err)
	}

	return nil
}

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

	script := redis.NewScript(`
		local key = KEYS[1]
		local field = ARGV[1]
		local amount = ARGV[2]
		local now = ARGV[3]
		local update_pending_total = ARGV[4] == '1'

		-- 确保余额记录存在
		local exists = redis.call('EXISTS', key)
		if exists == 0 then
			redis.call('HSET', key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		redis.call('HINCRBYFLOAT', key, field, amount)

		-- 如果是增加到 pending_available，同时增加 pending_total
		if update_pending_total then
			redis.call('HINCRBYFLOAT', key, 'pending_total', amount)
		end

		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb, []string{key}, field, amount.String(), now, updatePendingTotal).Result()
	if err != nil {
		return fmt.Errorf("credit failed: %w", err)
	}

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

	script := redis.NewScript(`
		local key = KEYS[1]
		local field = ARGV[1]
		local amount = tonumber(ARGV[2])
		local now = ARGV[3]

		local frozen = tonumber(redis.call('HGET', key, field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		redis.call('HINCRBYFLOAT', key, field, -amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb, []string{key}, field, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("debit failed: %w", err)
	}

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
	script := redis.NewScript(`
		local from_key = KEYS[1]
		local to_key = KEYS[2]
		local frozen_field = ARGV[1]
		local amount = tonumber(ARGV[2])
		local now = ARGV[3]

		-- 1. 检查发送方冻结余额
		local frozen = tonumber(redis.call('HGET', from_key, frozen_field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		-- 2. 从发送方冻结余额扣减
		redis.call('HINCRBYFLOAT', from_key, frozen_field, -amount)
		redis.call('HINCRBY', from_key, 'version', 1)
		redis.call('HSET', from_key, 'updated_at', now)

		-- 3. 确保接收方余额记录存在
		local to_exists = redis.call('EXISTS', to_key)
		if to_exists == 0 then
			redis.call('HSET', to_key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		-- 4. 增加到接收方待结算可用，同时增加 pending_total
		redis.call('HINCRBYFLOAT', to_key, 'pending_available', amount)
		redis.call('HINCRBYFLOAT', to_key, 'pending_total', amount)
		redis.call('HINCRBY', to_key, 'version', 1)
		redis.call('HSET', to_key, 'updated_at', now)

		return {ok = 'success'}
	`)

	result, err := script.Run(ctx, r.rdb, []string{fromKey, toKey}, frozenField, amount.String(), now).Result()
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

// ClearTrade 清算成交 (原子操作)
func (r *balanceRedisRepository) ClearTrade(ctx context.Context, req *ClearTradeRequest) error {
	tradeKey := tradeIdempotentKey(req.TradeID)
	makerBaseKey := balanceKey(req.MakerWallet, req.BaseToken)
	makerQuoteKey := balanceKey(req.MakerWallet, req.QuoteToken)
	takerBaseKey := balanceKey(req.TakerWallet, req.BaseToken)
	takerQuoteKey := balanceKey(req.TakerWallet, req.QuoteToken)
	makerOrderFreezeKey := orderFreezeKey(req.MakerOrderID)
	takerOrderFreezeKey := orderFreezeKey(req.TakerOrderID)
	feeBucketKeyStr := feeBucketKey(req.Market, req.FeeToken, req.FeeBucketID)
	now := time.Now().UnixMilli()

	script := redis.NewScript(`
		local trade_key = KEYS[1]
		local maker_base_key = KEYS[2]
		local maker_quote_key = KEYS[3]
		local taker_base_key = KEYS[4]
		local taker_quote_key = KEYS[5]
		local maker_order_freeze_key = KEYS[6]
		local taker_order_freeze_key = KEYS[7]
		local fee_bucket_key = KEYS[8]

		local base_amount = tonumber(ARGV[1])
		local quote_amount = tonumber(ARGV[2])
		local maker_fee = tonumber(ARGV[3])
		local taker_fee = tonumber(ARGV[4])
		local maker_is_buy = ARGV[5] == '1'
		local now = ARGV[6]

		-- 辅助函数: 更新订单冻结记录
		local function update_order_freeze(freeze_key, deduct_amount)
			local settled = tonumber(redis.call('HGET', freeze_key, 'settled_amount') or '0')
			local pending = tonumber(redis.call('HGET', freeze_key, 'pending_amount') or '0')
			local total = settled + pending
			if total <= 0 then return end

			-- 按比例从 settled 和 pending 扣减
			local settled_ratio = settled / total
			local settled_deduct = deduct_amount * settled_ratio
			local pending_deduct = deduct_amount - settled_deduct

			local new_settled = settled - settled_deduct
			local new_pending = pending - pending_deduct

			-- 防止浮点精度问题导致负数
			if new_settled < 0.000001 then new_settled = 0 end
			if new_pending < 0.000001 then new_pending = 0 end

			if new_settled == 0 and new_pending == 0 then
				-- 全部释放，删除记录
				redis.call('DEL', freeze_key)
			else
				redis.call('HSET', freeze_key,
					'settled_amount', tostring(new_settled),
					'pending_amount', tostring(new_pending))
			end
		end

		-- 1. 幂等检查
		if redis.call('EXISTS', trade_key) == 1 then
			return {err = 'trade_already_processed'}
		end

		-- 2. 清算逻辑
		if maker_is_buy then
			-- Maker 是买方: Maker 冻结 quote, 获得 base; Taker 冻结 base, 获得 quote
			-- Maker: 解冻并扣减 quote, 增加 base (pending_available)
			local maker_quote_frozen = tonumber(redis.call('HGET', maker_quote_key, 'settled_frozen') or '0')
			local maker_quote_pending_frozen = tonumber(redis.call('HGET', maker_quote_key, 'pending_frozen') or '0')
			local total_frozen = maker_quote_frozen + maker_quote_pending_frozen
			if total_frozen < quote_amount then
				return {err = 'maker insufficient frozen'}
			end

			-- 按比例扣减
			local settled_ratio = maker_quote_frozen / total_frozen
			local settled_deduct = quote_amount * settled_ratio
			local pending_deduct = quote_amount - settled_deduct

			local maker_receive = base_amount - maker_fee
			redis.call('HINCRBYFLOAT', maker_quote_key, 'settled_frozen', -settled_deduct)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_frozen', -pending_deduct)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_available', maker_receive)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_total', maker_receive)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HSET', maker_quote_key, 'updated_at', now)
			redis.call('HSET', maker_base_key, 'updated_at', now)

			-- 更新 Maker 订单冻结记录
			update_order_freeze(maker_order_freeze_key, quote_amount)

			-- Taker: 解冻并扣减 base, 增加 quote (pending_available)
			local taker_base_frozen = tonumber(redis.call('HGET', taker_base_key, 'settled_frozen') or '0')
			local taker_base_pending_frozen = tonumber(redis.call('HGET', taker_base_key, 'pending_frozen') or '0')
			local taker_total_frozen = taker_base_frozen + taker_base_pending_frozen
			if taker_total_frozen < base_amount then
				return {err = 'taker insufficient frozen'}
			end

			local taker_settled_ratio = taker_base_frozen / taker_total_frozen
			local taker_settled_deduct = base_amount * taker_settled_ratio
			local taker_pending_deduct = base_amount - taker_settled_deduct

			local taker_receive = quote_amount - taker_fee
			redis.call('HINCRBYFLOAT', taker_base_key, 'settled_frozen', -taker_settled_deduct)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_frozen', -taker_pending_deduct)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_available', taker_receive)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_total', taker_receive)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HSET', taker_base_key, 'updated_at', now)
			redis.call('HSET', taker_quote_key, 'updated_at', now)

			-- 更新 Taker 订单冻结记录
			update_order_freeze(taker_order_freeze_key, base_amount)
		else
			-- Maker 是卖方: Maker 冻结 base, 获得 quote; Taker 冻结 quote, 获得 base
			-- Maker: 解冻并扣减 base, 增加 quote (pending_available)
			local maker_base_frozen = tonumber(redis.call('HGET', maker_base_key, 'settled_frozen') or '0')
			local maker_base_pending_frozen = tonumber(redis.call('HGET', maker_base_key, 'pending_frozen') or '0')
			local total_frozen = maker_base_frozen + maker_base_pending_frozen
			if total_frozen < base_amount then
				return {err = 'maker insufficient frozen'}
			end

			local settled_ratio = maker_base_frozen / total_frozen
			local settled_deduct = base_amount * settled_ratio
			local pending_deduct = base_amount - settled_deduct

			local maker_receive = quote_amount - maker_fee
			redis.call('HINCRBYFLOAT', maker_base_key, 'settled_frozen', -settled_deduct)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_frozen', -pending_deduct)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_available', maker_receive)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_total', maker_receive)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HSET', maker_base_key, 'updated_at', now)
			redis.call('HSET', maker_quote_key, 'updated_at', now)

			-- 更新 Maker 订单冻结记录
			update_order_freeze(maker_order_freeze_key, base_amount)

			-- Taker: 解冻并扣减 quote, 增加 base (pending_available)
			local taker_quote_frozen = tonumber(redis.call('HGET', taker_quote_key, 'settled_frozen') or '0')
			local taker_quote_pending_frozen = tonumber(redis.call('HGET', taker_quote_key, 'pending_frozen') or '0')
			local taker_total_frozen = taker_quote_frozen + taker_quote_pending_frozen
			if taker_total_frozen < quote_amount then
				return {err = 'taker insufficient frozen'}
			end

			local taker_settled_ratio = taker_quote_frozen / taker_total_frozen
			local taker_settled_deduct = quote_amount * taker_settled_ratio
			local taker_pending_deduct = quote_amount - taker_settled_deduct

			local taker_receive = base_amount - taker_fee
			redis.call('HINCRBYFLOAT', taker_quote_key, 'settled_frozen', -taker_settled_deduct)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_frozen', -taker_pending_deduct)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_available', taker_receive)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_total', taker_receive)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
			redis.call('HSET', taker_base_key, 'updated_at', now)

			-- 更新 Taker 订单冻结记录
			update_order_freeze(taker_order_freeze_key, quote_amount)
		end

		-- 3. 增加手续费分桶
		local total_fee = maker_fee + taker_fee
		if total_fee > 0 then
			redis.call('INCRBYFLOAT', fee_bucket_key, total_fee)
		end

		-- 4. 标记成交已处理 (7 天 TTL)
		redis.call('SETEX', trade_key, 604800, '1')

		return {ok = 'success'}
	`)

	makerIsBuyFlag := "0"
	if req.MakerIsBuy {
		makerIsBuyFlag = "1"
	}

	result, err := script.Run(ctx, r.rdb,
		[]string{tradeKey, makerBaseKey, makerQuoteKey, takerBaseKey, takerQuoteKey,
			makerOrderFreezeKey, takerOrderFreezeKey, feeBucketKeyStr},
		req.Amount.String(), req.QuoteAmount.String(), req.MakerFee.String(), req.TakerFee.String(),
		makerIsBuyFlag, now,
	).Result()

	if err != nil {
		return fmt.Errorf("clear trade failed: %w", err)
	}

	if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) >= 2 {
		if resultSlice[0] == "err" {
			errStr := resultSlice[1].(string)
			if errStr == "trade_already_processed" {
				return ErrRedisTradeProcessed
			}
			return errors.New(errStr)
		}
	}

	return nil
}

// Settle 结算 (pending → settled)
func (r *balanceRedisRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	key := balanceKey(wallet, token)
	now := time.Now().UnixMilli()

	script := redis.NewScript(`
		local key = KEYS[1]
		local amount = tonumber(ARGV[1])
		local now = ARGV[2]

		local pending_available = tonumber(redis.call('HGET', key, 'pending_available') or '0')
		if pending_available < amount then
			return {err = 'insufficient pending balance'}
		end

		redis.call('HINCRBYFLOAT', key, 'pending_available', -amount)
		redis.call('HINCRBYFLOAT', key, 'settled_available', amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	_, err := script.Run(ctx, r.rdb, []string{key}, amount.String(), now).Result()
	if err != nil {
		return fmt.Errorf("settle failed: %w", err)
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

// CreditFeeBucket 增加手续费分桶
func (r *balanceRedisRepository) CreditFeeBucket(ctx context.Context, market, token string, bucketID int, amount decimal.Decimal) error {
	key := feeBucketKey(market, token, bucketID)
	return r.rdb.IncrByFloat(ctx, key, amount.InexactFloat64()).Err()
}

// CheckTradeProcessed 检查成交是否已处理
func (r *balanceRedisRepository) CheckTradeProcessed(ctx context.Context, tradeID string) (bool, error) {
	key := tradeIdempotentKey(tradeID)
	result, err := r.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check trade processed failed: %w", err)
	}
	return result > 0, nil
}

// MarkTradeProcessed 标记成交已处理
func (r *balanceRedisRepository) MarkTradeProcessed(ctx context.Context, tradeID string, ttl time.Duration) error {
	key := tradeIdempotentKey(tradeID)
	return r.rdb.SetEx(ctx, key, "1", ttl).Err()
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

// RollbackTrade 回滚成交 (结算失败时使用)
// 将成交的资金变动反向操作:
// - Maker 收益从 pending_available 扣回
// - Taker 收益从 pending_available 扣回
// - 双方原来扣除的冻结退回 pending_frozen (因为已经从 frozen 扣除，无法区分是 settled 还是 pending)
func (r *balanceRedisRepository) RollbackTrade(ctx context.Context, req *RollbackTradeRequest) error {
	rollbackKey := fmt.Sprintf("eidos:trading:rollback:%s", req.TradeID)
	makerBaseKey := balanceKey(req.MakerWallet, req.BaseToken)
	makerQuoteKey := balanceKey(req.MakerWallet, req.QuoteToken)
	takerBaseKey := balanceKey(req.TakerWallet, req.BaseToken)
	takerQuoteKey := balanceKey(req.TakerWallet, req.QuoteToken)
	now := time.Now().UnixMilli()

	script := redis.NewScript(`
		local rollback_key = KEYS[1]
		local maker_base_key = KEYS[2]
		local maker_quote_key = KEYS[3]
		local taker_base_key = KEYS[4]
		local taker_quote_key = KEYS[5]

		local base_amount = tonumber(ARGV[1])
		local quote_amount = tonumber(ARGV[2])
		local maker_fee = tonumber(ARGV[3])
		local taker_fee = tonumber(ARGV[4])
		local maker_is_buy = ARGV[5] == '1'
		local now = ARGV[6]

		-- 1. 幂等检查
		if redis.call('EXISTS', rollback_key) == 1 then
			return {ok = 'already_rolled_back'}
		end

		-- 2. 回滚逻辑 (ClearTrade 的反向操作)
		if maker_is_buy then
			-- 原操作: Maker 扣 quote_frozen, 得 base_pending_available
			-- 回滚: Maker 扣 base_pending_available (maker_receive = base_amount - maker_fee)
			--       Maker 加 quote_pending_frozen (因为无法区分来源，统一加到 pending)
			local maker_receive = base_amount - maker_fee
			local maker_pending = tonumber(redis.call('HGET', maker_base_key, 'pending_available') or '0')

			-- 扣减 Maker 收益 (可能不足，记录实际扣减量)
			local actual_deduct = math.min(maker_pending, maker_receive)
			if actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', maker_base_key, 'pending_available', -actual_deduct)
				redis.call('HINCRBYFLOAT', maker_base_key, 'pending_total', -actual_deduct)
			end
			-- 退回 Maker 冻结 (退到 pending_frozen)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_frozen', quote_amount)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HSET', maker_base_key, 'updated_at', now)
			redis.call('HSET', maker_quote_key, 'updated_at', now)

			-- 原操作: Taker 扣 base_frozen, 得 quote_pending_available
			-- 回滚: Taker 扣 quote_pending_available (taker_receive = quote_amount - taker_fee)
			--       Taker 加 base_pending_frozen
			local taker_receive = quote_amount - taker_fee
			local taker_pending = tonumber(redis.call('HGET', taker_quote_key, 'pending_available') or '0')

			local taker_actual_deduct = math.min(taker_pending, taker_receive)
			if taker_actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_available', -taker_actual_deduct)
				redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_total', -taker_actual_deduct)
			end
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_frozen', base_amount)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
			redis.call('HSET', taker_base_key, 'updated_at', now)
		else
			-- Maker 是卖方
			-- 原操作: Maker 扣 base_frozen, 得 quote_pending_available
			-- 回滚: Maker 扣 quote_pending_available, 加 base_pending_frozen
			local maker_receive = quote_amount - maker_fee
			local maker_pending = tonumber(redis.call('HGET', maker_quote_key, 'pending_available') or '0')

			local actual_deduct = math.min(maker_pending, maker_receive)
			if actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_available', -actual_deduct)
				redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_total', -actual_deduct)
			end
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_frozen', base_amount)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HSET', maker_quote_key, 'updated_at', now)
			redis.call('HSET', maker_base_key, 'updated_at', now)

			-- 原操作: Taker 扣 quote_frozen, 得 base_pending_available
			-- 回滚: Taker 扣 base_pending_available, 加 quote_pending_frozen
			local taker_receive = base_amount - taker_fee
			local taker_pending = tonumber(redis.call('HGET', taker_base_key, 'pending_available') or '0')

			local taker_actual_deduct = math.min(taker_pending, taker_receive)
			if taker_actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', taker_base_key, 'pending_available', -taker_actual_deduct)
				redis.call('HINCRBYFLOAT', taker_base_key, 'pending_total', -taker_actual_deduct)
			end
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_frozen', quote_amount)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HSET', taker_base_key, 'updated_at', now)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
		end

		-- 3. 标记已回滚 (7 天 TTL)
		redis.call('SETEX', rollback_key, 604800, '1')

		return {ok = 'success'}
	`)

	makerIsBuyFlag := "0"
	if req.MakerIsBuy {
		makerIsBuyFlag = "1"
	}

	_, err := script.Run(ctx, r.rdb,
		[]string{rollbackKey, makerBaseKey, makerQuoteKey, takerBaseKey, takerQuoteKey},
		req.Amount.String(), req.QuoteAmount.String(), req.MakerFee.String(), req.TakerFee.String(),
		makerIsBuyFlag, now,
	).Result()

	if err != nil {
		return fmt.Errorf("rollback trade failed: %w", err)
	}

	return nil
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
