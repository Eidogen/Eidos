// Package model 价格档位单元测试
package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewPriceLevel(t *testing.T) {
	price := decimal.NewFromFloat(100.5)
	pl := NewPriceLevel(price)

	assert.True(t, price.Equal(pl.Price))
	assert.True(t, pl.TotalSize.IsZero())
	assert.Equal(t, 0, pl.OrderCount)
	assert.Nil(t, pl.Head)
	assert.Nil(t, pl.Tail)
}

func TestPriceLevel_AddOrder(t *testing.T) {
	price := decimal.NewFromFloat(100.0)
	pl := NewPriceLevel(price)

	// 添加第一个订单
	order1 := &Order{
		OrderID:   "order-1",
		Price:     price,
		Remaining: decimal.NewFromFloat(10.0),
	}
	pl.AddOrder(order1)

	assert.Equal(t, 1, pl.OrderCount)
	assert.True(t, decimal.NewFromFloat(10.0).Equal(pl.TotalSize))
	assert.Equal(t, order1, pl.Head)
	assert.Equal(t, order1, pl.Tail)
	assert.Equal(t, pl, order1.PriceLevel)
	assert.Nil(t, order1.Prev)
	assert.Nil(t, order1.Next)

	// 添加第二个订单
	order2 := &Order{
		OrderID:   "order-2",
		Price:     price,
		Remaining: decimal.NewFromFloat(5.0),
	}
	pl.AddOrder(order2)

	assert.Equal(t, 2, pl.OrderCount)
	assert.True(t, decimal.NewFromFloat(15.0).Equal(pl.TotalSize))
	assert.Equal(t, order1, pl.Head)
	assert.Equal(t, order2, pl.Tail)
	assert.Equal(t, order2, order1.Next)
	assert.Equal(t, order1, order2.Prev)
}

func TestPriceLevel_RemoveOrder(t *testing.T) {
	price := decimal.NewFromFloat(100.0)
	pl := NewPriceLevel(price)

	// 创建三个订单
	order1 := &Order{OrderID: "order-1", Price: price, Remaining: decimal.NewFromFloat(10.0)}
	order2 := &Order{OrderID: "order-2", Price: price, Remaining: decimal.NewFromFloat(5.0)}
	order3 := &Order{OrderID: "order-3", Price: price, Remaining: decimal.NewFromFloat(3.0)}

	pl.AddOrder(order1)
	pl.AddOrder(order2)
	pl.AddOrder(order3)

	// 测试移除中间订单
	pl.RemoveOrder(order2)
	assert.Equal(t, 2, pl.OrderCount)
	assert.True(t, decimal.NewFromFloat(13.0).Equal(pl.TotalSize))
	assert.Equal(t, order3, order1.Next)
	assert.Equal(t, order1, order3.Prev)
	assert.Nil(t, order2.Prev)
	assert.Nil(t, order2.Next)
	assert.Nil(t, order2.PriceLevel)

	// 测试移除头部订单
	pl.RemoveOrder(order1)
	assert.Equal(t, 1, pl.OrderCount)
	assert.True(t, decimal.NewFromFloat(3.0).Equal(pl.TotalSize))
	assert.Equal(t, order3, pl.Head)
	assert.Equal(t, order3, pl.Tail)

	// 测试移除最后一个订单
	pl.RemoveOrder(order3)
	assert.Equal(t, 0, pl.OrderCount)
	assert.True(t, pl.TotalSize.IsZero())
	assert.Nil(t, pl.Head)
	assert.Nil(t, pl.Tail)
	assert.True(t, pl.IsEmpty())
}

func TestPriceLevel_UpdateOrderSize(t *testing.T) {
	price := decimal.NewFromFloat(100.0)
	pl := NewPriceLevel(price)

	order := &Order{
		OrderID:   "order-1",
		Price:     price,
		Remaining: decimal.NewFromFloat(10.0),
	}
	pl.AddOrder(order)

	// 部分成交 3.5
	pl.UpdateOrderSize(order, decimal.NewFromFloat(3.5))

	assert.True(t, decimal.NewFromFloat(6.5).Equal(order.Remaining))
	assert.True(t, decimal.NewFromFloat(6.5).Equal(pl.TotalSize))
}

func TestPriceLevel_IsEmpty(t *testing.T) {
	pl := NewPriceLevel(decimal.NewFromFloat(100.0))
	assert.True(t, pl.IsEmpty())

	order := &Order{OrderID: "order-1", Price: pl.Price, Remaining: decimal.NewFromFloat(10.0)}
	pl.AddOrder(order)
	assert.False(t, pl.IsEmpty())

	pl.RemoveOrder(order)
	assert.True(t, pl.IsEmpty())
}

func TestPriceLevel_FirstOrder(t *testing.T) {
	pl := NewPriceLevel(decimal.NewFromFloat(100.0))
	assert.Nil(t, pl.FirstOrder())

	order1 := &Order{OrderID: "order-1", Price: pl.Price, Remaining: decimal.NewFromFloat(10.0)}
	order2 := &Order{OrderID: "order-2", Price: pl.Price, Remaining: decimal.NewFromFloat(5.0)}

	pl.AddOrder(order1)
	pl.AddOrder(order2)

	assert.Equal(t, order1, pl.FirstOrder())
}
