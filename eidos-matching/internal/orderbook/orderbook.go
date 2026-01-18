package orderbook

import (
	"sync"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
)

// OrderBook 订单簿
// 使用红黑树按价格索引，每个价格档位内使用双向链表按时间排序
type OrderBook struct {
	Market    string
	Bids      *RBTree                 // 买单树 (价格降序)
	Asks      *RBTree                 // 卖单树 (价格升序)
	Orders    map[string]*model.Order // 订单ID -> 订单 (快速查找)
	LastPrice decimal.Decimal         // 最新成交价

	// 统计信息
	OrderCount  int   // 订单总数
	TradeCount  int64 // 成交笔数
	Sequence    int64 // 输出序列号
	LastTradeAt int64 // 最后成交时间

	mu sync.RWMutex // 读写锁 (单线程撮合时不需要，但保留用于快照读取)
}

// NewOrderBook 创建订单簿
func NewOrderBook(market string) *OrderBook {
	return &OrderBook{
		Market:    market,
		Bids:      NewRBTree(false), // 降序 (高价优先)
		Asks:      NewRBTree(true),  // 升序 (低价优先)
		Orders:    make(map[string]*model.Order),
		LastPrice: decimal.Zero,
	}
}

// AddOrder 添加订单到订单簿
// 返回是否成功添加
func (ob *OrderBook) AddOrder(order *model.Order) bool {
	// 检查订单是否已存在
	if _, exists := ob.Orders[order.OrderID]; exists {
		return false
	}

	// 选择对应的树
	var tree *RBTree
	if order.Side == model.OrderSideBuy {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	// 查找或创建价格档位
	pl := tree.Find(order.Price)
	if pl == nil {
		pl = model.NewPriceLevel(order.Price)
		tree.Insert(pl)
	}

	// 添加订单到价格档位
	pl.AddOrder(order)
	ob.Orders[order.OrderID] = order
	ob.OrderCount++

	return true
}

// RemoveOrder 从订单簿中移除订单
func (ob *OrderBook) RemoveOrder(orderID string) *model.Order {
	order, exists := ob.Orders[orderID]
	if !exists {
		return nil
	}

	// 从价格档位移除
	pl := order.PriceLevel
	if pl != nil {
		pl.RemoveOrder(order)

		// 如果价格档位为空，从树中删除
		if pl.IsEmpty() {
			var tree *RBTree
			if order.Side == model.OrderSideBuy {
				tree = ob.Bids
			} else {
				tree = ob.Asks
			}
			tree.Delete(pl)
		}
	}

	delete(ob.Orders, orderID)
	ob.OrderCount--

	return order
}

// GetOrder 获取订单
func (ob *OrderBook) GetOrder(orderID string) *model.Order {
	return ob.Orders[orderID]
}

// HasOrder 检查订单是否存在
func (ob *OrderBook) HasOrder(orderID string) bool {
	_, exists := ob.Orders[orderID]
	return exists
}

// BestBid 获取最优买价
func (ob *OrderBook) BestBid() *model.PriceLevel {
	return ob.Bids.First()
}

// BestAsk 获取最优卖价
func (ob *OrderBook) BestAsk() *model.PriceLevel {
	return ob.Asks.First()
}

// BestBidPrice 获取最优买价价格
func (ob *OrderBook) BestBidPrice() decimal.Decimal {
	if pl := ob.BestBid(); pl != nil {
		return pl.Price
	}
	return decimal.Zero
}

// BestAskPrice 获取最优卖价价格
func (ob *OrderBook) BestAskPrice() decimal.Decimal {
	if pl := ob.BestAsk(); pl != nil {
		return pl.Price
	}
	return decimal.Zero
}

// Spread 买卖价差
func (ob *OrderBook) Spread() decimal.Decimal {
	bestBid := ob.BestBidPrice()
	bestAsk := ob.BestAskPrice()
	if bestBid.IsZero() || bestAsk.IsZero() {
		return decimal.Zero
	}
	return bestAsk.Sub(bestBid)
}

// Depth 获取深度快照
// levels: 返回的价格档位数量，0 表示全部
func (ob *OrderBook) Depth(levels int) (*DepthSnapshot, error) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	snapshot := &DepthSnapshot{
		Market:    ob.Market,
		Bids:      make([]DepthLevel, 0),
		Asks:      make([]DepthLevel, 0),
		LastPrice: ob.LastPrice,
		Sequence:  ob.Sequence,
	}

	// 遍历买单
	count := 0
	ob.Bids.InOrder(func(pl *model.PriceLevel) bool {
		if levels > 0 && count >= levels {
			return false
		}
		snapshot.Bids = append(snapshot.Bids, DepthLevel{
			Price: pl.Price,
			Size:  pl.TotalSize,
			Count: pl.OrderCount,
		})
		count++
		return true
	})

	// 遍历卖单
	count = 0
	ob.Asks.InOrder(func(pl *model.PriceLevel) bool {
		if levels > 0 && count >= levels {
			return false
		}
		snapshot.Asks = append(snapshot.Asks, DepthLevel{
			Price: pl.Price,
			Size:  pl.TotalSize,
			Count: pl.OrderCount,
		})
		count++
		return true
	})

	return snapshot, nil
}

// UpdateOrderRemaining 更新订单剩余数量
// 成交后调用，减少订单的剩余数量
func (ob *OrderBook) UpdateOrderRemaining(order *model.Order, filledAmount decimal.Decimal) {
	if order.PriceLevel != nil {
		order.PriceLevel.UpdateOrderSize(order, filledAmount)
	}
}

// RemoveOrderIfFilled 如果订单已完全成交则移除
func (ob *OrderBook) RemoveOrderIfFilled(order *model.Order) bool {
	if order.IsFilled() {
		ob.RemoveOrder(order.OrderID)
		return true
	}
	return false
}

// NextSequence 获取下一个输出序列号
func (ob *OrderBook) NextSequence() int64 {
	ob.Sequence++
	return ob.Sequence
}

// SetLastPrice 设置最新成交价
func (ob *OrderBook) SetLastPrice(price decimal.Decimal, timestamp int64) {
	ob.LastPrice = price
	ob.LastTradeAt = timestamp
}

// Stats 获取订单簿统计信息
func (ob *OrderBook) Stats() OrderBookStats {
	return OrderBookStats{
		Market:       ob.Market,
		BidLevels:    ob.Bids.Len(),
		AskLevels:    ob.Asks.Len(),
		OrderCount:   ob.OrderCount,
		TradeCount:   ob.TradeCount,
		BestBid:      ob.BestBidPrice(),
		BestAsk:      ob.BestAskPrice(),
		Spread:       ob.Spread(),
		LastPrice:    ob.LastPrice,
		LastTradeAt:  ob.LastTradeAt,
		LastSequence: ob.Sequence,
	}
}

// DepthSnapshot 深度快照
type DepthSnapshot struct {
	Market    string          `json:"market"`
	Bids      []DepthLevel    `json:"bids"`
	Asks      []DepthLevel    `json:"asks"`
	LastPrice decimal.Decimal `json:"last_price"`
	Sequence  int64           `json:"sequence"`
	Timestamp int64           `json:"timestamp"`
}

// DepthLevel 深度档位
type DepthLevel struct {
	Price decimal.Decimal `json:"price"`
	Size  decimal.Decimal `json:"size"`
	Count int             `json:"count"`
}

// OrderBookStats 订单簿统计信息
type OrderBookStats struct {
	Market       string          `json:"market"`
	BidLevels    int             `json:"bid_levels"`
	AskLevels    int             `json:"ask_levels"`
	OrderCount   int             `json:"order_count"`
	TradeCount   int64           `json:"trade_count"`
	BestBid      decimal.Decimal `json:"best_bid"`
	BestAsk      decimal.Decimal `json:"best_ask"`
	Spread       decimal.Decimal `json:"spread"`
	LastPrice    decimal.Decimal `json:"last_price"`
	LastTradeAt  int64           `json:"last_trade_at"`
	LastSequence int64           `json:"last_sequence"`
}
