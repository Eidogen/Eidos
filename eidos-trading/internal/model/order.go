// Package model 定义交易服务的数据模型
package model

import (
	"github.com/shopspring/decimal"
)

// OrderStatus 订单状态
type OrderStatus int8

const (
	OrderStatusPending    OrderStatus = 0 // 待处理 (刚创建，尚未发送到撮合引擎)
	OrderStatusOpen       OrderStatus = 1 // 挂单中 (已进入订单簿，等待成交)
	OrderStatusPartial    OrderStatus = 2 // 部分成交
	OrderStatusFilled     OrderStatus = 3 // 完全成交
	OrderStatusCancelled  OrderStatus = 4 // 已取消
	OrderStatusExpired    OrderStatus = 5 // 已过期
	OrderStatusRejected   OrderStatus = 6 // 被拒绝
	OrderStatusCancelling OrderStatus = 7 // 取消中 (已发送取消请求，等待撮合引擎确认)
)

// String 返回状态的字符串表示
func (s OrderStatus) String() string {
	switch s {
	case OrderStatusPending:
		return "PENDING"
	case OrderStatusOpen:
		return "OPEN"
	case OrderStatusPartial:
		return "PARTIAL"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusCancelled:
		return "CANCELLED"
	case OrderStatusExpired:
		return "EXPIRED"
	case OrderStatusRejected:
		return "REJECTED"
	case OrderStatusCancelling:
		return "CANCELLING"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal 判断是否为终态
func (s OrderStatus) IsTerminal() bool {
	return s == OrderStatusFilled || s == OrderStatusCancelled ||
		s == OrderStatusExpired || s == OrderStatusRejected
}

// OrderSide 订单方向
type OrderSide int8

const (
	OrderSideBuy  OrderSide = 1 // 买入
	OrderSideSell OrderSide = 2 // 卖出
)

func (s OrderSide) String() string {
	if s == OrderSideBuy {
		return "BUY"
	}
	return "SELL"
}

// OrderType 订单类型
type OrderType int8

const (
	OrderTypeLimit  OrderType = 1 // 限价单
	OrderTypeMarket OrderType = 2 // 市价单
)

func (t OrderType) String() string {
	if t == OrderTypeLimit {
		return "LIMIT"
	}
	return "MARKET"
}

// TimeInForce 订单有效期类型
type TimeInForce int8

const (
	TimeInForceGTC TimeInForce = 1 // Good Till Cancel - 直到取消
	TimeInForceIOC TimeInForce = 2 // Immediate Or Cancel - 立即成交或取消
	TimeInForceFOK TimeInForce = 3 // Fill Or Kill - 全部成交或取消
)

func (t TimeInForce) String() string {
	switch t {
	case TimeInForceGTC:
		return "GTC"
	case TimeInForceIOC:
		return "IOC"
	case TimeInForceFOK:
		return "FOK"
	default:
		return "UNKNOWN"
	}
}

// Order 订单模型
// 对应数据库表 orders
type Order struct {
	ID              int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID         string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"order_id"`                 // 订单 ID (Snowflake)
	Wallet          string          `gorm:"type:varchar(42);index;not null" json:"wallet"`                         // 钱包地址
	Market          string          `gorm:"type:varchar(20);index;not null" json:"market"`                         // 交易对 (如 BTC-USDC)
	Side            OrderSide       `gorm:"type:smallint;not null" json:"side"`                                    // 买/卖
	Type            OrderType       `gorm:"column:order_type;type:smallint;not null" json:"type"`                  // 订单类型
	Price           decimal.Decimal `gorm:"type:decimal(36,18)" json:"price"`                                      // 价格 (市价单为0)
	Amount          decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"amount"`                            // 数量
	FilledAmount    decimal.Decimal `gorm:"type:decimal(36,18);default:0" json:"filled_amount"`                    // 已成交数量
	FilledQuote     decimal.Decimal `gorm:"type:decimal(36,18);default:0" json:"filled_quote"`                     // 已成交金额 (quote token)
	RemainingAmount decimal.Decimal `gorm:"type:decimal(36,18)" json:"remaining_amount"`                           // 剩余数量
	AvgPrice        decimal.Decimal `gorm:"type:decimal(36,18);default:0" json:"avg_price"`                        // 成交均价
	Status          OrderStatus     `gorm:"type:smallint;index;not null;default:0" json:"status"`                  // 订单状态
	TimeInForce     TimeInForce     `gorm:"type:smallint;not null;default:1" json:"time_in_force"`                 // 有效期类型
	Nonce           uint64          `gorm:"type:bigint;not null" json:"nonce"`                                     // 用户 Nonce
	ClientOrderID   string          `gorm:"type:varchar(64);index:idx_wallet_client_order" json:"client_order_id"` // 客户端订单ID (幂等键)
	ExpireAt        int64           `gorm:"type:bigint;not null" json:"expire_at"`                                 // 过期时间 (毫秒)
	Signature       []byte          `gorm:"type:bytea" json:"signature"`                                           // EIP-712 签名
	RejectReason    string          `gorm:"type:varchar(255)" json:"reject_reason"`                                // 拒绝原因
	FreezeToken     string          `gorm:"type:varchar(20);not null" json:"freeze_token"`                         // 冻结的 Token
	FreezeAmount    decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"freeze_amount"`                     // 冻结数量
	AcceptedAt      int64           `gorm:"type:bigint" json:"accepted_at"`                                        // 撮合引擎接受时间 (毫秒)
	CreatedAt       int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`           // 创建时间 (毫秒)
	UpdatedAt       int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`           // 更新时间 (毫秒)
	CreatedBy       string          `gorm:"type:varchar(64)" json:"created_by"`                                    // 创建者
	UpdatedBy       string          `gorm:"type:varchar(64)" json:"updated_by"`                                    // 更新者
}

// TableName 返回表名
func (Order) TableName() string {
	return "trading_orders"
}

// CanTransitionTo 检查状态转换是否合法
func (o *Order) CanTransitionTo(newStatus OrderStatus) bool {
	// 状态转换规则 (参见 04-状态机规范.md)
	transitions := map[OrderStatus][]OrderStatus{
		OrderStatusPending:    {OrderStatusOpen, OrderStatusRejected, OrderStatusCancelled}, // PENDING 可直接取消 (未发送到撮合引擎)
		OrderStatusOpen:       {OrderStatusPartial, OrderStatusFilled, OrderStatusCancelled, OrderStatusExpired, OrderStatusCancelling},
		OrderStatusPartial:    {OrderStatusFilled, OrderStatusCancelled, OrderStatusCancelling},
		OrderStatusCancelling: {OrderStatusCancelled, OrderStatusFilled}, // 取消中可能被成交
	}

	allowed, exists := transitions[o.Status]
	if !exists {
		return false // 终态不能转换
	}

	for _, s := range allowed {
		if s == newStatus {
			return true
		}
	}
	return false
}

// CanCancel 检查订单是否可以取消
// PENDING: 尚未发送到撮合引擎，可直接在 Redis 中取消
// OPEN/PARTIAL: 已在撮合引擎中，需发送取消请求
// CANCELLING: 已在取消中，不需重复取消
func (o *Order) CanCancel() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusOpen || o.Status == OrderStatusPartial
}

// RemainingFreezeAmount 计算剩余冻结金额
// 已成交部分会解冻，这里返回尚未成交部分对应的冻结金额
func (o *Order) RemainingFreezeAmount() decimal.Decimal {
	if o.FreezeAmount.IsZero() {
		return decimal.Zero
	}

	// 根据成交比例计算剩余冻结
	if o.Amount.IsZero() {
		return decimal.Zero
	}

	filledRatio := o.FilledAmount.Div(o.Amount)
	usedFreeze := o.FreezeAmount.Mul(filledRatio)
	return o.FreezeAmount.Sub(usedFreeze)
}

// GetQuoteAmount 计算报价货币数量
// 买单: price * amount
// 卖单: 0 (卖出的是 base token)
func (o *Order) GetQuoteAmount() decimal.Decimal {
	if o.Side == OrderSideBuy && o.Type == OrderTypeLimit {
		return o.Price.Mul(o.Amount)
	}
	return decimal.Zero
}

// GetFreezeToken 获取需要冻结的 Token
// 买单冻结 Quote Token (如 USDC)
// 卖单冻结 Base Token (如 BTC)
func (o *Order) GetFreezeToken() string {
	// market 格式: BASE-QUOTE (如 BTC-USDC)
	if len(o.Market) < 3 {
		return ""
	}

	// 解析交易对
	base, quote := ParseMarket(o.Market)
	if o.Side == OrderSideBuy {
		return quote
	}
	return base
}

// GetFreezeAmount 获取需要冻结的数量
func (o *Order) GetFreezeAmount() decimal.Decimal {
	if o.Side == OrderSideBuy {
		// 买单: 冻结 price * amount (Quote Token)
		if o.Type == OrderTypeLimit {
			return o.Price.Mul(o.Amount)
		}
		// 市价买单暂不支持
		return decimal.Zero
	}
	// 卖单: 冻结 amount (Base Token)
	return o.Amount
}

// ParseMarket 解析交易对
// 输入: BTC-USDC
// 输出: BTC, USDC
func ParseMarket(market string) (base, quote string) {
	for i := 0; i < len(market); i++ {
		if market[i] == '-' {
			return market[:i], market[i+1:]
		}
	}
	return market, ""
}
