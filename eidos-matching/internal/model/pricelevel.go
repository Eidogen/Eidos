package model

import (
	"github.com/shopspring/decimal"
)

// PriceLevel 价格档位
// 包含同一价格的所有订单，按时间优先排序的双向链表
type PriceLevel struct {
	Price      decimal.Decimal // 价格
	TotalSize  decimal.Decimal // 该价位总数量
	OrderCount int             // 订单数量
	Head       *Order          // 链表头 (最早的订单，优先成交)
	Tail       *Order          // 链表尾 (最新的订单)
}

// NewPriceLevel 创建价格档位
func NewPriceLevel(price decimal.Decimal) *PriceLevel {
	return &PriceLevel{
		Price:     price,
		TotalSize: decimal.Zero,
	}
}

// AddOrder 添加订单到链表尾部
// 新订单追加到尾部，保持时间优先
func (pl *PriceLevel) AddOrder(order *Order) {
	order.PriceLevel = pl
	order.Prev = nil
	order.Next = nil

	if pl.Head == nil {
		// 空链表
		pl.Head = order
		pl.Tail = order
	} else {
		// 追加到尾部
		order.Prev = pl.Tail
		pl.Tail.Next = order
		pl.Tail = order
	}

	pl.TotalSize = pl.TotalSize.Add(order.Remaining)
	pl.OrderCount++
}

// RemoveOrder 从链表中移除订单
func (pl *PriceLevel) RemoveOrder(order *Order) {
	if order.Prev != nil {
		order.Prev.Next = order.Next
	} else {
		// order 是 Head
		pl.Head = order.Next
	}

	if order.Next != nil {
		order.Next.Prev = order.Prev
	} else {
		// order 是 Tail
		pl.Tail = order.Prev
	}

	pl.TotalSize = pl.TotalSize.Sub(order.Remaining)
	pl.OrderCount--

	// 清理引用
	order.Prev = nil
	order.Next = nil
	order.PriceLevel = nil
}

// UpdateOrderSize 更新订单剩余数量 (部分成交后调用)
func (pl *PriceLevel) UpdateOrderSize(order *Order, filledAmount decimal.Decimal) {
	pl.TotalSize = pl.TotalSize.Sub(filledAmount)
	order.Remaining = order.Remaining.Sub(filledAmount)
}

// IsEmpty 价格档位是否为空
func (pl *PriceLevel) IsEmpty() bool {
	return pl.Head == nil
}

// FirstOrder 获取第一个订单 (最早的，优先成交)
func (pl *PriceLevel) FirstOrder() *Order {
	return pl.Head
}
