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

// ============================================================
// 深度聚合精度功能
// ============================================================

// ValidPrecisions 有效的聚合精度 (价格步进)
// 例如: 0.01 表示聚合到分, 1 表示聚合到元, 10 表示聚合到十元
var ValidPrecisions = []string{"0.001", "0.01", "0.1", "1", "10", "100"}

// IsValidPrecision 检查是否为有效的聚合精度
func IsValidPrecision(precision string) bool {
	for _, v := range ValidPrecisions {
		if precision == v {
			return true
		}
	}
	return false
}

// ParsePrecision 解析精度值
func ParsePrecision(precision string) (decimal.Decimal, bool) {
	if precision == "" {
		return decimal.Zero, false
	}
	p, err := decimal.NewFromString(precision)
	if err != nil || p.IsZero() || p.IsNegative() {
		return decimal.Zero, false
	}
	return p, true
}

// AggregateAtPrecision 按指定精度聚合深度数据
// precision: 价格聚合精度，例如 0.1 会将 100.05 和 100.08 聚合到 100.0
// 买单: 向下取整 (100.08 -> 100.0)
// 卖单: 向上取整 (100.02 -> 100.1)
func (d *Depth) AggregateAtPrecision(precision decimal.Decimal) *Depth {
	if precision.IsZero() {
		return d.Clone()
	}

	aggregated := &Depth{
		Market:    d.Market,
		Sequence:  d.Sequence,
		Timestamp: d.Timestamp,
	}

	// 聚合买单 (向下取整，同价格合并)
	aggregated.Bids = aggregateLevels(d.Bids, precision, false)
	// 排序: 价格降序
	SortBids(aggregated.Bids)

	// 聚合卖单 (向上取整，同价格合并)
	aggregated.Asks = aggregateLevels(d.Asks, precision, true)
	// 排序: 价格升序
	SortAsks(aggregated.Asks)

	return aggregated
}

// aggregateLevels 聚合价格档位
// roundUp: true 向上取整 (卖单), false 向下取整 (买单)
func aggregateLevels(levels []*PriceLevel, precision decimal.Decimal, roundUp bool) []*PriceLevel {
	if len(levels) == 0 {
		return nil
	}

	// 用 map 聚合相同价格的数量
	aggregatedMap := make(map[string]decimal.Decimal)

	for _, level := range levels {
		var roundedPrice decimal.Decimal
		if roundUp {
			// 向上取整: ceil(price / precision) * precision
			roundedPrice = level.Price.Div(precision).Ceil().Mul(precision)
		} else {
			// 向下取整: floor(price / precision) * precision
			roundedPrice = level.Price.Div(precision).Floor().Mul(precision)
		}

		key := roundedPrice.String()
		if amount, exists := aggregatedMap[key]; exists {
			aggregatedMap[key] = amount.Add(level.Amount)
		} else {
			aggregatedMap[key] = level.Amount
		}
	}

	// 转换为 PriceLevel 切片
	result := make([]*PriceLevel, 0, len(aggregatedMap))
	for priceStr, amount := range aggregatedMap {
		price, _ := decimal.NewFromString(priceStr)
		result = append(result, &PriceLevel{
			Price:  price,
			Amount: amount,
		})
	}

	return result
}

// AggregateAndLimit 聚合并限制深度档位
func (d *Depth) AggregateAndLimit(precision decimal.Decimal, limit int) *Depth {
	aggregated := d.AggregateAtPrecision(precision)
	return aggregated.Limit(limit)
}
