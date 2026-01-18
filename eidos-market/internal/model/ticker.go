package model

import (
	"github.com/shopspring/decimal"
)

// Ticker 24h行情
type Ticker struct {
	Market             string          `json:"market"`
	LastPrice          decimal.Decimal `json:"last_price"`
	PriceChange        decimal.Decimal `json:"price_change"`
	PriceChangePercent decimal.Decimal `json:"price_change_percent"`
	Open               decimal.Decimal `json:"open"`
	High               decimal.Decimal `json:"high"`
	Low                decimal.Decimal `json:"low"`
	Close              decimal.Decimal `json:"close"`
	Volume             decimal.Decimal `json:"volume"`
	QuoteVolume        decimal.Decimal `json:"quote_volume"`
	BestBid            decimal.Decimal `json:"best_bid"`
	BestBidQty         decimal.Decimal `json:"best_bid_qty"`
	BestAsk            decimal.Decimal `json:"best_ask"`
	BestAskQty         decimal.Decimal `json:"best_ask_qty"`
	TradeCount         int32           `json:"trade_count"`
	Timestamp          int64           `json:"timestamp"`
}

// TickerBucket 1分钟桶（用于24h滑动窗口聚合）
type TickerBucket struct {
	Minute      int64           `json:"minute"`       // 分钟时间戳（毫秒，已对齐到分钟）
	Open        decimal.Decimal `json:"open"`         // 开盘价
	High        decimal.Decimal `json:"high"`         // 最高价
	Low         decimal.Decimal `json:"low"`          // 最低价
	Close       decimal.Decimal `json:"close"`        // 收盘价
	Volume      decimal.Decimal `json:"volume"`       // 成交量（Base Token）
	QuoteVolume decimal.Decimal `json:"quote_volume"` // 成交额（Quote Token）
	TradeCount  int32           `json:"trade_count"`  // 成交笔数
	FirstTrade  int64           `json:"first_trade"`  // 第一笔成交时间
	LastTrade   int64           `json:"last_trade"`   // 最后一笔成交时间
}

// NewTickerBucket 创建新的分钟桶
func NewTickerBucket(minute int64) *TickerBucket {
	return &TickerBucket{
		Minute:      minute,
		Open:        decimal.Zero,
		High:        decimal.Zero,
		Low:         decimal.Zero,
		Close:       decimal.Zero,
		Volume:      decimal.Zero,
		QuoteVolume: decimal.Zero,
		TradeCount:  0,
		FirstTrade:  0,
		LastTrade:   0,
	}
}

// IsEmpty 检查桶是否为空
func (b *TickerBucket) IsEmpty() bool {
	return b.TradeCount == 0
}

// Update 更新桶数据
func (b *TickerBucket) Update(price, amount, quoteAmount decimal.Decimal, timestamp int64) {
	if b.TradeCount == 0 {
		b.Open = price
		b.High = price
		b.Low = price
		b.FirstTrade = timestamp
	} else {
		if price.GreaterThan(b.High) {
			b.High = price
		}
		if price.LessThan(b.Low) {
			b.Low = price
		}
	}
	b.Close = price
	b.Volume = b.Volume.Add(amount)
	b.QuoteVolume = b.QuoteVolume.Add(quoteAmount)
	b.TradeCount++
	b.LastTrade = timestamp
}

// Clone 克隆桶数据
func (b *TickerBucket) Clone() *TickerBucket {
	return &TickerBucket{
		Minute:      b.Minute,
		Open:        b.Open,
		High:        b.High,
		Low:         b.Low,
		Close:       b.Close,
		Volume:      b.Volume,
		QuoteVolume: b.QuoteVolume,
		TradeCount:  b.TradeCount,
		FirstTrade:  b.FirstTrade,
		LastTrade:   b.LastTrade,
	}
}

// CalculateMinute 根据时间戳计算对齐到分钟的时间戳
func CalculateMinute(timestamp int64) int64 {
	return (timestamp / 60_000) * 60_000
}

// Clone 克隆 Ticker
func (t *Ticker) Clone() *Ticker {
	return &Ticker{
		Market:             t.Market,
		LastPrice:          t.LastPrice,
		PriceChange:        t.PriceChange,
		PriceChangePercent: t.PriceChangePercent,
		Open:               t.Open,
		High:               t.High,
		Low:                t.Low,
		Close:              t.Close,
		Volume:             t.Volume,
		QuoteVolume:        t.QuoteVolume,
		BestBid:            t.BestBid,
		BestBidQty:         t.BestBidQty,
		BestAsk:            t.BestAsk,
		BestAskQty:         t.BestAskQty,
		TradeCount:         t.TradeCount,
		Timestamp:          t.Timestamp,
	}
}
