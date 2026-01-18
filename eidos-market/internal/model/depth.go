package model

import (
	"sort"

	"github.com/shopspring/decimal"
)

// PriceLevel 价格档位
type PriceLevel struct {
	Price  decimal.Decimal `json:"price"`
	Amount decimal.Decimal `json:"amount"`
}

// Clone 克隆价格档位
func (p *PriceLevel) Clone() *PriceLevel {
	return &PriceLevel{
		Price:  p.Price,
		Amount: p.Amount,
	}
}

// Depth 订单簿深度
type Depth struct {
	Market    string        `json:"market"`
	Bids      []*PriceLevel `json:"bids"` // 买单，价格降序
	Asks      []*PriceLevel `json:"asks"` // 卖单，价格升序
	Sequence  uint64        `json:"sequence"`
	Timestamp int64         `json:"timestamp"`
}

// DepthUpdate 深度增量更新
type DepthUpdate struct {
	Market   string        `json:"market"`
	Bids     []*PriceLevel `json:"bids"`
	Asks     []*PriceLevel `json:"asks"`
	Sequence uint64        `json:"sequence"`
}

// ValidDepthLimits 有效的深度档位数
var ValidDepthLimits = []int{5, 10, 20, 50, 100}

// IsValidDepthLimit 检查是否为有效的深度档位数
func IsValidDepthLimit(limit int) bool {
	for _, v := range ValidDepthLimits {
		if limit == v {
			return true
		}
	}
	return false
}

// NormalizeDepthLimit 标准化深度档位数
func NormalizeDepthLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	for _, v := range ValidDepthLimits {
		if limit <= v {
			return v
		}
	}
	return 100
}

// Clone 克隆深度数据
func (d *Depth) Clone() *Depth {
	clone := &Depth{
		Market:    d.Market,
		Sequence:  d.Sequence,
		Timestamp: d.Timestamp,
		Bids:      make([]*PriceLevel, len(d.Bids)),
		Asks:      make([]*PriceLevel, len(d.Asks)),
	}
	for i, bid := range d.Bids {
		clone.Bids[i] = bid.Clone()
	}
	for i, ask := range d.Asks {
		clone.Asks[i] = ask.Clone()
	}
	return clone
}

// Limit 限制深度档位数
func (d *Depth) Limit(limit int) *Depth {
	limited := &Depth{
		Market:    d.Market,
		Sequence:  d.Sequence,
		Timestamp: d.Timestamp,
	}

	if len(d.Bids) > limit {
		limited.Bids = make([]*PriceLevel, limit)
		copy(limited.Bids, d.Bids[:limit])
	} else {
		limited.Bids = make([]*PriceLevel, len(d.Bids))
		copy(limited.Bids, d.Bids)
	}

	if len(d.Asks) > limit {
		limited.Asks = make([]*PriceLevel, limit)
		copy(limited.Asks, d.Asks[:limit])
	} else {
		limited.Asks = make([]*PriceLevel, len(d.Asks))
		copy(limited.Asks, d.Asks)
	}

	return limited
}

// GetBestBid 获取最优买价
func (d *Depth) GetBestBid() (decimal.Decimal, decimal.Decimal) {
	if len(d.Bids) == 0 {
		return decimal.Zero, decimal.Zero
	}
	return d.Bids[0].Price, d.Bids[0].Amount
}

// GetBestAsk 获取最优卖价
func (d *Depth) GetBestAsk() (decimal.Decimal, decimal.Decimal) {
	if len(d.Asks) == 0 {
		return decimal.Zero, decimal.Zero
	}
	return d.Asks[0].Price, d.Asks[0].Amount
}

// SortBids 对买单按价格降序排序
func SortBids(levels []*PriceLevel) {
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price.GreaterThan(levels[j].Price)
	})
}

// SortAsks 对卖单按价格升序排序
func SortAsks(levels []*PriceLevel) {
	sort.Slice(levels, func(i, j int) bool {
		return levels[i].Price.LessThan(levels[j].Price)
	})
}
