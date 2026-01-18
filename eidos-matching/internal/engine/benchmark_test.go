// Package engine 撮合引擎性能基准测试
package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
)

// BenchmarkMatcher_Match 测试撮合性能
func BenchmarkMatcher_Match(b *testing.B) {
	cfg := &MarketConfig{
		Symbol:        "BTC-USDC",
		BaseToken:     "BTC",
		QuoteToken:    "USDC",
		PriceDecimals: 2,
		SizeDecimals:  4,
		MinSize:       decimal.NewFromFloat(0.0001),
		TickSize:      decimal.NewFromFloat(0.01),
		MakerFeeRate:  decimal.NewFromFloat(0.001),
		TakerFeeRate:  decimal.NewFromFloat(0.002),
	}
	ob := orderbook.NewOrderBook("BTC-USDC")
	m := NewMatcher(cfg, ob, func() string { return "trade-1" })

	// 预先添加卖单
	for i := 0; i < 1000; i++ {
		order := &model.Order{
			OrderID:     "sell-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0xmaker",
			Market:      "BTC-USDC",
			Side:        model.OrderSideSell,
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50000.0 + float64(i)),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(i),
		}
		m.orderBook.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buyOrder := &model.Order{
			OrderID:     "buy-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0xtaker",
			Market:      "BTC-USDC",
			Side:        model.OrderSideBuy,
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50500.0),
			Amount:      decimal.NewFromFloat(0.1),
			Remaining:   decimal.NewFromFloat(0.1),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(1000 + i),
		}
		m.Match(buyOrder)
	}
}

// BenchmarkMatcher_NoMatch 测试无撮合情况性能
func BenchmarkMatcher_NoMatch(b *testing.B) {
	cfg := &MarketConfig{
		Symbol:        "BTC-USDC",
		BaseToken:     "BTC",
		QuoteToken:    "USDC",
		PriceDecimals: 2,
		SizeDecimals:  4,
		MinSize:       decimal.NewFromFloat(0.0001),
		TickSize:      decimal.NewFromFloat(0.01),
		MakerFeeRate:  decimal.NewFromFloat(0.001),
		TakerFeeRate:  decimal.NewFromFloat(0.002),
	}
	ob := orderbook.NewOrderBook("BTC-USDC")
	m := NewMatcher(cfg, ob, func() string { return "trade-1" })

	// 预先添加卖单（价格较高）
	for i := 0; i < 100; i++ {
		order := &model.Order{
			OrderID:     "sell-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0xmaker",
			Market:      "BTC-USDC",
			Side:        model.OrderSideSell,
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(60000.0 + float64(i)),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(i),
		}
		m.orderBook.AddOrder(order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 买单价格低于卖单，不会成交
		buyOrder := &model.Order{
			OrderID:     "buy-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0xtaker",
			Market:      "BTC-USDC",
			Side:        model.OrderSideBuy,
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50000.0),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(100 + i),
		}
		m.Match(buyOrder)
	}
}

// BenchmarkEngine_ProcessOrder 测试引擎处理订单性能
func BenchmarkEngine_ProcessOrder(b *testing.B) {
	var counter int64
	cfg := &EngineConfig{
		Market: &MarketConfig{
			Symbol:        "BTC-USDC",
			BaseToken:     "BTC",
			QuoteToken:    "USDC",
			PriceDecimals: 2,
			SizeDecimals:  4,
			MinSize:       decimal.NewFromFloat(0.0001),
			TickSize:      decimal.NewFromFloat(0.01),
			MakerFeeRate:  decimal.NewFromFloat(0.001),
			TakerFeeRate:  decimal.NewFromFloat(0.002),
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 10000,
	}

	engine := NewEngine(cfg)
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := &model.Order{
			OrderID:     "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0x123",
			Market:      "BTC-USDC",
			Side:        model.OrderSide(i % 2),
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50000.0 + float64(i%100)),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(i + 1),
		}
		engine.ProcessOrder(ctx, order)
	}
}

// BenchmarkEngine_GetDepth 测试获取深度性能
func BenchmarkEngine_GetDepth(b *testing.B) {
	var counter int64
	cfg := &EngineConfig{
		Market: &MarketConfig{
			Symbol:        "BTC-USDC",
			BaseToken:     "BTC",
			QuoteToken:    "USDC",
			PriceDecimals: 2,
			SizeDecimals:  4,
			MinSize:       decimal.NewFromFloat(0.0001),
			TickSize:      decimal.NewFromFloat(0.01),
			MakerFeeRate:  decimal.NewFromFloat(0.001),
			TakerFeeRate:  decimal.NewFromFloat(0.002),
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 10000,
	}

	engine := NewEngine(cfg)
	engine.Start()
	defer engine.Stop()

	ctx := context.Background()

	// 预先添加订单
	for i := 0; i < 1000; i++ {
		order := &model.Order{
			OrderID:     "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0x123",
			Market:      "BTC-USDC",
			Side:        model.OrderSide(i % 2),
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50000.0 + float64(i%100)),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(i + 1),
		}
		engine.ProcessOrder(ctx, order)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.GetDepth(20)
	}
}

// BenchmarkEngineManager_ProcessOrder 测试引擎管理器处理订单性能
func BenchmarkEngineManager_ProcessOrder(b *testing.B) {
	var counter int64
	cfg := &ManagerConfig{
		Markets: []*MarketConfig{
			{
				Symbol:        "BTC-USDC",
				BaseToken:     "BTC",
				QuoteToken:    "USDC",
				PriceDecimals: 2,
				SizeDecimals:  4,
				MinSize:       decimal.NewFromFloat(0.0001),
				TickSize:      decimal.NewFromFloat(0.01),
				MakerFeeRate:  decimal.NewFromFloat(0.001),
				TakerFeeRate:  decimal.NewFromFloat(0.002),
			},
		},
		TradeIDGen: func() string {
			return "trade-" + decimal.NewFromInt(atomic.AddInt64(&counter, 1)).String()
		},
		ChannelSize: 10000,
	}

	manager := NewEngineManager(cfg)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := &model.Order{
			OrderID:     "order-" + decimal.NewFromInt(int64(i)).String(),
			Wallet:      "0x123",
			Market:      "BTC-USDC",
			Side:        model.OrderSide(i % 2),
			Type:        model.OrderTypeLimit,
			TimeInForce: model.TimeInForceGTC,
			Price:       decimal.NewFromFloat(50000.0 + float64(i%100)),
			Amount:      decimal.NewFromFloat(1.0),
			Remaining:   decimal.NewFromFloat(1.0),
			Timestamp:   time.Now().UnixNano(),
			Sequence:    int64(i + 1),
		}
		manager.ProcessOrder(ctx, order)
	}
}
