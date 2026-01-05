package cache

import (
	"errors"
	"time"

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
	// 用户活跃订单数 key: eidos:trading:open_orders:{wallet}
	userOpenOrdersKeyPattern = "eidos:trading:open_orders:%s"
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
	ErrRedisBalanceNotFound            = errors.New("redis balance not found")
	ErrRedisInsufficientBalance        = errors.New("redis insufficient balance")
	ErrRedisNonceUsed                  = errors.New("redis nonce already used")
	ErrRedisTradeProcessed             = errors.New("redis trade already processed")
	ErrRedisOutboxExists               = errors.New("redis outbox already exists")
	ErrRedisOrderAlreadySent           = errors.New("redis order already sent to matching")
	ErrRedisPendingLimitExceeded       = errors.New("redis pending limit exceeded")
	ErrRedisGlobalPendingLimitExceeded = errors.New("redis global pending limit exceeded")
	ErrRedisMaxOpenOrdersExceeded      = errors.New("redis max open orders exceeded")
	ErrRedisOrderFreezeNotFound        = errors.New("redis order freeze not found")
)

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
	Wallet             string          // 钱包地址
	Token              string          // 代币
	Amount             decimal.Decimal // 冻结金额
	OrderID            string          // 订单 ID
	FromSettled        bool            // 优先从已结算冻结
	OrderJSON          string          // 订单 JSON (用于 Outbox)
	ShardID            int             // Outbox 分片 ID
	NonceKey           string          // Nonce key (用于标记已使用)
	NonceTTL           time.Duration   // Nonce TTL
	UserPendingLimit   decimal.Decimal // 用户待结算限额 (0 表示不检查)
	GlobalPendingLimit decimal.Decimal // 全局待结算限额 (0 表示不检查)
	MaxOpenOrders      int             // 用户最大活跃订单数 (0 表示不检查)
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
