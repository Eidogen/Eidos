// Package model 定义撮合引擎的数据模型
package model

import (
	"github.com/shopspring/decimal"
)

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

// Opposite 返回相反方向
func (s OrderSide) Opposite() OrderSide {
	if s == OrderSideBuy {
		return OrderSideSell
	}
	return OrderSideBuy
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

// Order 撮合引擎中的订单
// 使用双向链表组织同价格的订单
type Order struct {
	OrderID     string          // 订单 ID
	Wallet      string          // 钱包地址
	Market      string          // 交易对 (如 BTC-USDC)
	Side        OrderSide       // 买/卖
	Type        OrderType       // 订单类型
	TimeInForce TimeInForce     // 有效期类型
	Price       decimal.Decimal // 价格 (使用 decimal 精确计算)
	Amount      decimal.Decimal // 原始数量
	Remaining   decimal.Decimal // 剩余数量
	Timestamp   int64           // 纳秒时间戳 (用于时间优先排序)
	Sequence    int64           // 消息序列号 (用于幂等性检查)

	// 双向链表指针 (同价格内按时间排序)
	Prev *Order
	Next *Order

	// 所属价格档位的引用
	PriceLevel *PriceLevel
}

// IsFilled 订单是否已完全成交
func (o *Order) IsFilled() bool {
	return o.Remaining.IsZero() || o.Remaining.IsNegative()
}

// FilledAmount 已成交数量
func (o *Order) FilledAmount() decimal.Decimal {
	return o.Amount.Sub(o.Remaining)
}

// OrderMessage Kafka 订单消息格式
// 从 eidos-trading 发送到 eidos-matching
type OrderMessage struct {
	OrderID     string `json:"order_id"`
	Wallet      string `json:"wallet"`
	Market      string `json:"market"`
	Side        int8   `json:"side"`          // 1=买, 2=卖
	OrderType   int8   `json:"order_type"`    // 1=限价, 2=市价
	TimeInForce int8   `json:"time_in_force"` // 1=GTC, 2=IOC, 3=FOK
	Price       string `json:"price"`         // 价格 (字符串避免精度丢失)
	Amount      string `json:"amount"`        // 数量
	Timestamp   int64  `json:"timestamp"`     // 纳秒时间戳
	Sequence    int64  `json:"sequence"`      // 消息序列号
}

// CancelMessage Kafka 取消请求消息格式
type CancelMessage struct {
	OrderID   string `json:"order_id"`
	Wallet    string `json:"wallet"`
	Market    string `json:"market"`
	Timestamp int64  `json:"timestamp"`
	Sequence  int64  `json:"sequence"`
}

// TradeResult 成交结果
// 用于引擎内部处理
type TradeResult struct {
	TradeID          string          `json:"trade_id"`
	Market           string          `json:"market"`
	MakerOrderID     string          `json:"maker_order_id"`
	TakerOrderID     string          `json:"taker_order_id"`
	MakerWallet      string          `json:"maker_wallet"`
	TakerWallet      string          `json:"taker_wallet"`
	Price            decimal.Decimal `json:"price"`              // 成交价格 (Maker 价格)
	Amount           decimal.Decimal `json:"amount"`             // 成交数量
	QuoteAmount      decimal.Decimal `json:"quote_amount"`       // 成交金额 (价格 * 数量)
	MakerSide        OrderSide       `json:"maker_side"`         // Maker 方向
	MakerFee         decimal.Decimal `json:"maker_fee"`          // Maker 手续费
	TakerFee         decimal.Decimal `json:"taker_fee"`          // Taker 手续费
	FeeToken         string          `json:"fee_token"`          // 手续费 Token
	MakerOrderFilled bool            `json:"maker_order_filled"` // Maker 订单是否完全成交
	TakerOrderFilled bool            `json:"taker_order_filled"` // Taker 订单是否完全成交
	Timestamp        int64           `json:"timestamp"`          // 成交时间 (纳秒)
	Sequence         int64           `json:"sequence"`           // 输出序列号
}

// TradeResultMessage Kafka 成交消息格式
// 发送到 trade-results topic，供 eidos-market 消费
// 字段使用 string 类型确保跨服务兼容性
type TradeResultMessage struct {
	TradeID      string `json:"trade_id"`
	Market       string `json:"market"`
	MakerOrderID string `json:"maker_order_id"`
	TakerOrderID string `json:"taker_order_id"`
	MakerWallet  string `json:"maker_wallet"`
	TakerWallet  string `json:"taker_wallet"`
	Price        string `json:"price"`
	Amount       string `json:"amount"`
	QuoteAmount  string `json:"quote_amount"`
	MakerFee     string `json:"maker_fee"`
	TakerFee     string `json:"taker_fee"`
	FeeToken     string `json:"fee_token"`
	MakerSide    int8   `json:"maker_side"` // 0=买, 1=卖 (与 eidos-market 对齐)
	Timestamp    int64  `json:"timestamp"`
}

// ToMessage 转换为 Kafka 消息格式
func (t *TradeResult) ToMessage() *TradeResultMessage {
	// MakerSide 转换: OrderSideBuy(1) -> 0, OrderSideSell(2) -> 1
	makerSide := int8(0)
	if t.MakerSide == OrderSideSell {
		makerSide = 1
	}

	return &TradeResultMessage{
		TradeID:      t.TradeID,
		Market:       t.Market,
		MakerOrderID: t.MakerOrderID,
		TakerOrderID: t.TakerOrderID,
		MakerWallet:  t.MakerWallet,
		TakerWallet:  t.TakerWallet,
		Price:        t.Price.String(),
		Amount:       t.Amount.String(),
		QuoteAmount:  t.QuoteAmount.String(),
		MakerFee:     t.MakerFee.String(),
		TakerFee:     t.TakerFee.String(),
		FeeToken:     t.FeeToken,
		MakerSide:    makerSide,
		Timestamp:    t.Timestamp,
	}
}

// CancelResult 取消结果
type CancelResult struct {
	OrderID       string          `json:"order_id"`
	Market        string          `json:"market"`
	Wallet        string          `json:"wallet"`
	Success       bool            `json:"success"`
	Reason        string          `json:"reason,omitempty"`
	RemainingSize decimal.Decimal `json:"remaining_size"` // 取消时剩余数量
	Timestamp     int64           `json:"timestamp"`
	Sequence      int64           `json:"sequence"`
}

// OrderBookUpdate 订单簿增量更新 (单个价位)
// 用于引擎内部处理
type OrderBookUpdate struct {
	Market     string          `json:"market"`
	UpdateType string          `json:"update_type"` // ADD, REMOVE, UPDATE
	Side       OrderSide       `json:"side"`
	Price      decimal.Decimal `json:"price"`
	Size       decimal.Decimal `json:"size"` // 0 表示删除该价位
	Timestamp  int64           `json:"timestamp"`
	Sequence   int64           `json:"sequence"`
}

// PriceLevelMessage Kafka 价格档位消息格式
type PriceLevelMessage struct {
	Price  string `json:"price"`
	Amount string `json:"amount"`
}

// OrderBookUpdateMessage Kafka 订单簿更新消息格式
// 发送到 orderbook-updates topic，供 eidos-market 消费
// 支持批量更新多个价格档位
type OrderBookUpdateMessage struct {
	Market   string               `json:"market"`
	Bids     []*PriceLevelMessage `json:"bids"`
	Asks     []*PriceLevelMessage `json:"asks"`
	Sequence uint64               `json:"sequence"`
}

// NewOrderBookUpdateMessage 创建订单簿更新消息
func NewOrderBookUpdateMessage(market string, sequence uint64) *OrderBookUpdateMessage {
	return &OrderBookUpdateMessage{
		Market:   market,
		Bids:     make([]*PriceLevelMessage, 0),
		Asks:     make([]*PriceLevelMessage, 0),
		Sequence: sequence,
	}
}

// AddBid 添加买单更新
func (m *OrderBookUpdateMessage) AddBid(price, amount decimal.Decimal) {
	m.Bids = append(m.Bids, &PriceLevelMessage{
		Price:  price.String(),
		Amount: amount.String(),
	})
}

// AddAsk 添加卖单更新
func (m *OrderBookUpdateMessage) AddAsk(price, amount decimal.Decimal) {
	m.Asks = append(m.Asks, &PriceLevelMessage{
		Price:  price.String(),
		Amount: amount.String(),
	})
}

// IsEmpty 检查消息是否为空
func (m *OrderBookUpdateMessage) IsEmpty() bool {
	return len(m.Bids) == 0 && len(m.Asks) == 0
}

// OrderRejectedMessage 订单被拒绝消息
// 发送到 order-updates topic，通知 eidos-trading 和 eidos-api
type OrderRejectedMessage struct {
	OrderID      string `json:"order_id"`
	Wallet       string `json:"wallet"`
	Market       string `json:"market"`
	Side         int8   `json:"side"`          // 1=买, 2=卖
	OrderType    int8   `json:"order_type"`    // 1=限价, 2=市价
	Price        string `json:"price"`
	Amount       string `json:"amount"`
	Status       string `json:"status"`        // rejected
	RejectReason string `json:"reject_reason"` // 拒绝原因
	Timestamp    int64  `json:"timestamp"`
}

// NewOrderRejectedMessage 从订单消息创建拒绝消息
func NewOrderRejectedMessage(order *OrderMessage, reason string, timestamp int64) *OrderRejectedMessage {
	return &OrderRejectedMessage{
		OrderID:      order.OrderID,
		Wallet:       order.Wallet,
		Market:       order.Market,
		Side:         order.Side,
		OrderType:    order.OrderType,
		Price:        order.Price,
		Amount:       order.Amount,
		Status:       "rejected",
		RejectReason: reason,
		Timestamp:    timestamp,
	}
}
