// Package orderbook 订单簿性能基准测试
package orderbook

import (
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
)

// BenchmarkOrderBook_AddOrder 测试添加订单性能
func BenchmarkOrderBook_AddOrder(b *testing.B) {
	ob := NewOrderBook("BTC-USDC")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSide(i % 2),
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 + float64(i%1000)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)
	}
}

// BenchmarkOrderBook_RemoveOrder 测试移除订单性能
func BenchmarkOrderBook_RemoveOrder(b *testing.B) {
	ob := NewOrderBook("BTC-USDC")

	// 预先添加订单
	for i := 0; i < b.N; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideBuy,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 + float64(i%1000)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.RemoveOrder("order-" + decimal.NewFromInt(int64(i)).String())
	}
}

// BenchmarkOrderBook_GetOrder 测试获取订单性能
func BenchmarkOrderBook_GetOrder(b *testing.B) {
	ob := NewOrderBook("BTC-USDC")

	// 预先添加订单
	for i := 0; i < 10000; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideBuy,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 + float64(i%1000)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.GetOrder("order-" + decimal.NewFromInt(int64(i%10000)).String())
	}
}

// BenchmarkOrderBook_BestBid 测试获取最佳买价性能
func BenchmarkOrderBook_BestBid(b *testing.B) {
	ob := NewOrderBook("BTC-USDC")

	// 预先添加买单
	for i := 0; i < 1000; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideBuy,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 + float64(i)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.BestBid()
	}
}

// BenchmarkOrderBook_Depth 测试获取深度性能
func BenchmarkOrderBook_Depth(b *testing.B) {
	ob := NewOrderBook("BTC-USDC")

	// 预先添加订单
	for i := 0; i < 1000; i++ {
		order := &model.Order{
			OrderID:   "buy-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideBuy,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 - float64(i)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i * 2),
		}
		ob.AddOrder(order)

		order = &model.Order{
			OrderID:   "sell-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideSell,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(51000.0 + float64(i)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i*2 + 1),
		}
		ob.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ob.Depth(20)
	}
}

// BenchmarkRBTree_Insert 测试红黑树插入性能
func BenchmarkRBTree_Insert(b *testing.B) {
	tree := NewRBTree(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pl := model.NewPriceLevel(decimal.NewFromFloat(float64(i)))
		tree.Insert(pl)
	}
}

// BenchmarkRBTree_Find 测试红黑树查找性能
func BenchmarkRBTree_Find(b *testing.B) {
	tree := NewRBTree(true)

	// 预先插入数据
	for i := 0; i < 10000; i++ {
		pl := model.NewPriceLevel(decimal.NewFromFloat(float64(i)))
		tree.Insert(pl)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.Find(decimal.NewFromFloat(float64(i % 10000)))
	}
}

// BenchmarkRBTree_Delete 测试红黑树删除性能
func BenchmarkRBTree_Delete(b *testing.B) {
	// 每次迭代需要重建树
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tree := NewRBTree(true)
		for j := 0; j < 1000; j++ {
			pl := model.NewPriceLevel(decimal.NewFromFloat(float64(j)))
			tree.Insert(pl)
		}
		b.StartTimer()

		pl := tree.Find(decimal.NewFromFloat(float64(i % 1000)))
		if pl != nil {
			tree.Delete(pl)
		}
	}
}

// BenchmarkPriceLevel_AddOrder 测试价格档位添加订单性能
func BenchmarkPriceLevel_AddOrder(b *testing.B) {
	pl := model.NewPriceLevel(decimal.NewFromFloat(50000.0))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Price:     decimal.NewFromFloat(50000.0),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
		}
		pl.AddOrder(order)
	}
}

// BenchmarkPriceLevel_RemoveOrder 测试价格档位移除订单性能
func BenchmarkPriceLevel_RemoveOrder(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pl := model.NewPriceLevel(decimal.NewFromFloat(50000.0))
		var targetOrder *model.Order
		for j := 0; j < 100; j++ {
			order := &model.Order{
				OrderID:   "order-" + decimal.NewFromInt(int64(j)).String(),
				Price:     decimal.NewFromFloat(50000.0),
				Amount:    decimal.NewFromFloat(1.0),
				Remaining: decimal.NewFromFloat(1.0),
			}
			pl.AddOrder(order)
			if j == 50 {
				targetOrder = order
			}
		}
		b.StartTimer()

		if targetOrder != nil {
			pl.RemoveOrder(targetOrder)
		}
	}
}
