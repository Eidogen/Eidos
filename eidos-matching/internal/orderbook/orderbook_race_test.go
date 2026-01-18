// Package orderbook 订单簿竞态测试
// 使用 go test -race 运行
package orderbook

import (
	"sync"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestOrderBook_ConcurrentReads 测试并发读取
func TestOrderBook_ConcurrentReads(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 添加一些初始订单
	for i := 0; i < 100; i++ {
		price := 50000.0 + float64(i)
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSideBuy,
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(price),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)
	}

	var wg sync.WaitGroup
	concurrency := 10
	iterations := 100

	// 并发读取操作
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = ob.BestBid()
				_ = ob.BestAsk()
				_ = ob.BestBidPrice()
				_ = ob.BestAskPrice()
				_ = ob.Spread()
				_ = ob.GetOrder("order-50")
				_ = ob.HasOrder("order-50")
				_, _ = ob.Depth(10)
				_ = ob.Stats()
			}
		}()
	}

	wg.Wait()
}

// TestOrderBook_SequentialModifications 测试顺序修改（订单簿设计为单线程写入）
func TestOrderBook_SequentialModifications(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	// 模拟 Kafka 消费者的顺序处理
	for i := 0; i < 1000; i++ {
		// 添加订单
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Market:    "BTC-USDC",
			Side:      model.OrderSide(i % 2),
			Type:      model.OrderTypeLimit,
			Price:     decimal.NewFromFloat(50000.0 + float64(i%100)),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Timestamp: time.Now().UnixNano(),
			Sequence:  int64(i),
		}
		ob.AddOrder(order)

		// 每10个订单移除一个
		if i > 0 && i%10 == 0 {
			removeID := "order-" + decimal.NewFromInt(int64(i-5)).String()
			ob.RemoveOrder(removeID)
		}
	}

	// 验证统计
	stats := ob.Stats()
	assert.Greater(t, stats.BidLevels+stats.AskLevels, 0)
}

// TestPriceLevel_ConcurrentOrderManagement 测试价格档位的并发操作
// 注意：实际使用中 PriceLevel 由 OrderBook 管理，不会并发访问
func TestPriceLevel_SequentialOrderManagement(t *testing.T) {
	pl := model.NewPriceLevel(decimal.NewFromFloat(50000.0))

	// 顺序添加订单
	for i := 0; i < 100; i++ {
		order := &model.Order{
			OrderID:   "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:    "0x123",
			Price:     decimal.NewFromFloat(50000.0),
			Amount:    decimal.NewFromFloat(1.0),
			Remaining: decimal.NewFromFloat(1.0),
			Sequence:  int64(i),
		}
		pl.AddOrder(order)
	}

	assert.Equal(t, 100, pl.OrderCount)

	// 顺序移除订单（移除链表头部的订单）
	for i := 0; i < 50; i++ {
		if order := pl.Head; order != nil {
			pl.RemoveOrder(order)
		}
	}

	assert.Equal(t, 50, pl.OrderCount)
}

// TestRBTree_SequentialOperations 测试红黑树顺序操作
func TestRBTree_SequentialOperations(t *testing.T) {
	tree := NewRBTree(true) // ascending order

	// 顺序插入
	for i := 0; i < 1000; i++ {
		pl := model.NewPriceLevel(decimal.NewFromFloat(float64(i)))
		tree.Insert(pl)
	}

	assert.Equal(t, 1000, tree.Len())

	// 顺序删除
	for i := 0; i < 500; i++ {
		pl := tree.Find(decimal.NewFromFloat(float64(i)))
		if pl != nil {
			tree.Delete(pl)
		}
	}

	assert.Equal(t, 500, tree.Len())

	// 顺序遍历
	count := 0
	tree.InOrder(func(pl *model.PriceLevel) bool {
		count++
		return true
	})
	assert.Equal(t, 500, count)
}

// TestOrderBook_NextSequence_Sequential 测试序列号生成的顺序性
func TestOrderBook_NextSequence_Sequential(t *testing.T) {
	ob := NewOrderBook("BTC-USDC")

	sequences := make([]int64, 1000)
	for i := 0; i < 1000; i++ {
		sequences[i] = ob.NextSequence()
	}

	// 验证序列号严格递增
	for i := 1; i < len(sequences); i++ {
		assert.Equal(t, sequences[i-1]+1, sequences[i])
	}
}
