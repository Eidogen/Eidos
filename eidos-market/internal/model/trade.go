package model

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// TradeSide 成交方向
type TradeSide int8

const (
	TradeSideBuy  TradeSide = 0 // 买方主动成交
	TradeSideSell TradeSide = 1 // 卖方主动成交
)

func (s TradeSide) String() string {
	switch s {
	case TradeSideBuy:
		return "BUY"
	case TradeSideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

// Trade 成交记录（行情服务使用的简化版本）
type Trade struct {
	TradeID     string          `json:"trade_id" gorm:"type:varchar(64);primaryKey"`
	Market      string          `json:"market" gorm:"type:varchar(20);index;not null"`
	Price       decimal.Decimal `json:"price" gorm:"type:decimal(36,18);not null"`
	Amount      decimal.Decimal `json:"amount" gorm:"type:decimal(36,18);not null"`
	QuoteAmount decimal.Decimal `json:"quote_amount" gorm:"type:decimal(36,18);not null"`
	Side        TradeSide       `json:"side" gorm:"type:smallint;not null"` // Taker 方向
	Timestamp   int64           `json:"timestamp" gorm:"type:bigint;index;not null"`
	CreatedAt   int64           `json:"created_at" gorm:"type:bigint;not null;autoCreateTime:milli"`
}

// TableName GORM表名
func (Trade) TableName() string {
	return "eidos_market_trades"
}

// Clone 克隆成交记录
func (t *Trade) Clone() *Trade {
	return &Trade{
		TradeID:     t.TradeID,
		Market:      t.Market,
		Price:       t.Price,
		Amount:      t.Amount,
		QuoteAmount: t.QuoteAmount,
		Side:        t.Side,
		Timestamp:   t.Timestamp,
		CreatedAt:   t.CreatedAt,
	}
}

// TradeEvent Kafka 成交事件（来自 eidos-matching）
type TradeEvent struct {
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
	MakerSide    int8   `json:"maker_side"` // 0=买, 1=卖
	Timestamp    int64  `json:"timestamp"`
}

// ToTrade 转换为 Trade 模型
// 返回错误如果价格/数量格式无效
func (e *TradeEvent) ToTrade() (*Trade, error) {
	price, err := decimal.NewFromString(e.Price)
	if err != nil {
		return nil, fmt.Errorf("invalid price format %q: %w", e.Price, err)
	}
	amount, err := decimal.NewFromString(e.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount format %q: %w", e.Amount, err)
	}
	quoteAmount, err := decimal.NewFromString(e.QuoteAmount)
	if err != nil {
		return nil, fmt.Errorf("invalid quote_amount format %q: %w", e.QuoteAmount, err)
	}

	// Taker 方向与 Maker 相反
	side := TradeSideBuy
	if e.MakerSide == 0 { // Maker 是买方，则 Taker 是卖方
		side = TradeSideSell
	}

	return &Trade{
		TradeID:     e.TradeID,
		Market:      e.Market,
		Price:       price,
		Amount:      amount,
		QuoteAmount: quoteAmount,
		Side:        side,
		Timestamp:   e.Timestamp,
	}, nil
}

// GetPrice 获取价格（decimal）
func (e *TradeEvent) GetPrice() (decimal.Decimal, error) {
	return decimal.NewFromString(e.Price)
}

// GetAmount 获取数量（decimal）
func (e *TradeEvent) GetAmount() (decimal.Decimal, error) {
	return decimal.NewFromString(e.Amount)
}

// GetQuoteAmount 获取成交额（decimal）
func (e *TradeEvent) GetQuoteAmount() (decimal.Decimal, error) {
	return decimal.NewFromString(e.QuoteAmount)
}
