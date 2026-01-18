package aggregator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// raceTestKlineRepo 竞态测试用的 K 线仓储 mock (简化版)
type raceTestKlineRepo struct{}

func (m *raceTestKlineRepo) Upsert(ctx context.Context, kline *model.Kline) error         { return nil }
func (m *raceTestKlineRepo) BatchUpsert(ctx context.Context, klines []*model.Kline) error { return nil }

// raceTestKlinePublisher 竞态测试用的 K 线发布器 mock (简化版)
type raceTestKlinePublisher struct{}

func (m *raceTestKlinePublisher) PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error {
	return nil
}

// raceTestTickerPublisher 竞态测试用的 Ticker 发布器 mock (简化版)
type raceTestTickerPublisher struct{}

func (m *raceTestTickerPublisher) PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error {
	return nil
}

// raceTestDepthPublisher 竞态测试用的深度发布器 mock (简化版)
type raceTestDepthPublisher struct{}

func (m *raceTestDepthPublisher) PublishDepth(ctx context.Context, market string, depth *model.Depth) error {
	return nil
}

// raceTestSnapshotProvider 竞态测试用的快照提供者 mock
type raceTestSnapshotProvider struct{}

func (m *raceTestSnapshotProvider) GetSnapshot(ctx context.Context, market string) (*model.Depth, error) {
	// 返回一个空的但有效的深度快照，避免 nil pointer
	return &model.Depth{
		Market:   market,
		Sequence: 0,
		Bids:     []*model.PriceLevel{},
		Asks:     []*model.PriceLevel{},
	}, nil
}

func newRaceTestKlineAggregator(market string) *KlineAggregator {
	return NewKlineAggregator(
		market,
		&raceTestKlineRepo{},
		&raceTestKlinePublisher{},
		zap.NewNop(),
		DefaultKlineAggregatorConfig(),
	)
}

func newRaceTestTickerCalculator(market string) *TickerCalculator {
	return NewTickerCalculator(
		market,
		&raceTestTickerPublisher{},
		zap.NewNop(),
		DefaultTickerCalculatorConfig(),
	)
}

func newRaceTestDepthManager(market string) *DepthManager {
	return NewDepthManager(
		market,
		&raceTestDepthPublisher{},
		&raceTestSnapshotProvider{},
		zap.NewNop(),
		DefaultDepthManagerConfig(),
	)
}

// TestKlineAggregator_ConcurrentProcessTrade 测试并发成交处理
func TestKlineAggregator_ConcurrentProcessTrade(t *testing.T) {
	agg := newRaceTestKlineAggregator("BTC-USDC")
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 100
	tradesPerGoroutine := 100
	baseTime := time.Now().UnixMilli()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < tradesPerGoroutine; j++ {
				price := decimal.NewFromFloat(50000.0 + float64(goroutineID))
				amount := decimal.NewFromFloat(1.0)
				quoteAmount := decimal.NewFromFloat(50000.0)
				trade := &model.TradeEvent{
					Market:      "BTC-USDC",
					Price:       price.String(),
					Amount:      amount.String(),
					QuoteAmount: quoteAmount.String(),
					Timestamp:   baseTime + int64(j),
				}
				agg.ProcessTrade(ctx, trade)
			}
		}(i)
	}

	wg.Wait()

	// 验证数据一致性
	stats := agg.Stats()
	expectedTrades := int64(numGoroutines * tradesPerGoroutine)
	require.Equal(t, expectedTrades, stats["trade_count"])
}

// TestKlineAggregator_ConcurrentGetKline 测试并发读写
func TestKlineAggregator_ConcurrentGetKline(t *testing.T) {
	agg := newRaceTestKlineAggregator("BTC-USDC")
	agg.Start()
	defer agg.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	baseTime := time.Now().UnixMilli()

	// 启动多个写入者
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				trade := &model.TradeEvent{
					Market:      "BTC-USDC",
					Price:       "50000",
					Amount:      "1.0",
					QuoteAmount: "50000",
					Timestamp:   baseTime,
				}
				agg.ProcessTrade(ctx, trade)
			}
		}()
	}

	// 启动多个读取者
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = agg.GetKline(model.Interval1m)
				_ = agg.GetAllKlines()
				_ = agg.Stats()
			}
		}()
	}

	wg.Wait()
}

// TestDepthManager_ConcurrentApplyUpdate 测试并发深度更新
func TestDepthManager_ConcurrentApplyUpdate(t *testing.T) {
	dm := newRaceTestDepthManager("BTC-USDC")
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 50
	updatesPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				seq := uint64(goroutineID*updatesPerGoroutine + j + 1)
				bidPrice := decimal.NewFromFloat(49900.0 - float64(goroutineID))
				askPrice := decimal.NewFromFloat(50100.0 + float64(goroutineID))
				update := &model.DepthUpdate{
					Market:   "BTC-USDC",
					Sequence: seq,
					Bids: []*model.PriceLevel{
						{Price: bidPrice, Amount: decimal.NewFromFloat(10.0)},
					},
					Asks: []*model.PriceLevel{
						{Price: askPrice, Amount: decimal.NewFromFloat(5.0)},
					},
				}
				dm.ApplyUpdate(ctx, update)
			}
		}(i)
	}

	wg.Wait()

	// 验证数据可以正常读取
	depth := dm.GetDepth(100)
	require.NotNil(t, depth)
	require.Equal(t, "BTC-USDC", depth.Market)
}

// TestDepthManager_ConcurrentReadWrite 测试并发读写深度
func TestDepthManager_ConcurrentReadWrite(t *testing.T) {
	dm := newRaceTestDepthManager("BTC-USDC")
	dm.Start()
	defer dm.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	done := make(chan struct{})

	// 启动写入者
	wg.Add(1)
	go func() {
		defer wg.Done()
		seq := uint64(1)
		for {
			select {
			case <-done:
				return
			default:
				update := &model.DepthUpdate{
					Market:   "BTC-USDC",
					Sequence: seq,
					Bids: []*model.PriceLevel{
						{Price: decimal.NewFromFloat(49900.0), Amount: decimal.NewFromFloat(10.0)},
					},
					Asks: []*model.PriceLevel{
						{Price: decimal.NewFromFloat(50100.0), Amount: decimal.NewFromFloat(5.0)},
					},
				}
				dm.ApplyUpdate(ctx, update)
				seq++
			}
		}
	}()

	// 启动多个读取者
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = dm.GetDepth(20)
					_, _, _, _ = dm.GetBestPrices()
					_ = dm.Stats()
				}
			}
		}()
	}

	// 运行一段时间后停止
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestTickerCalculator_ConcurrentProcessTrade 测试并发 Ticker 计算
func TestTickerCalculator_ConcurrentProcessTrade(t *testing.T) {
	tc := newRaceTestTickerCalculator("BTC-USDC")
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 100
	tradesPerGoroutine := 100
	baseTime := time.Now().UnixMilli()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < tradesPerGoroutine; j++ {
				price := decimal.NewFromFloat(50000.0 + float64(j))
				amount := decimal.NewFromFloat(1.0)
				quoteAmount := decimal.NewFromFloat(50000.0)
				trade := &model.TradeEvent{
					Market:      "BTC-USDC",
					Price:       price.String(),
					Amount:      amount.String(),
					QuoteAmount: quoteAmount.String(),
					Timestamp:   baseTime + int64(j*1000), // 每笔间隔1秒
				}
				tc.ProcessTrade(ctx, trade)
			}
		}(i)
	}

	wg.Wait()

	// 验证数据一致性
	stats := tc.Stats()
	expectedTrades := int64(numGoroutines * tradesPerGoroutine)
	require.Equal(t, expectedTrades, stats["trade_count"])
}

// TestTickerCalculator_ConcurrentReadWrite 测试并发读写 Ticker
func TestTickerCalculator_ConcurrentReadWrite(t *testing.T) {
	tc := newRaceTestTickerCalculator("BTC-USDC")
	tc.Start()
	defer tc.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	done := make(chan struct{})
	baseTime := time.Now().UnixMilli()

	// 启动写入者
	wg.Add(1)
	go func() {
		defer wg.Done()
		seq := int64(0)
		for {
			select {
			case <-done:
				return
			default:
				trade := &model.TradeEvent{
					Market:      "BTC-USDC",
					Price:       "50000",
					Amount:      "1.0",
					QuoteAmount: "50000",
					Timestamp:   baseTime + seq*1000,
				}
				tc.ProcessTrade(ctx, trade)
				seq++
			}
		}
	}()

	// 启动更新最优价格的协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				tc.UpdateBestPrices(
					decimal.NewFromFloat(49900.0),
					decimal.NewFromFloat(10.0),
					decimal.NewFromFloat(50100.0),
					decimal.NewFromFloat(5.0),
				)
			}
		}
	}()

	// 启动多个读取者
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = tc.GetTicker()
					_ = tc.Stats()
				}
			}
		}()
	}

	// 运行一段时间后停止
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestAllAggregators_ConcurrentMixedOperations 综合并发测试
func TestAllAggregators_ConcurrentMixedOperations(t *testing.T) {
	klineAgg := newRaceTestKlineAggregator("BTC-USDC")
	depthMgr := newRaceTestDepthManager("BTC-USDC")
	tickerCalc := newRaceTestTickerCalculator("BTC-USDC")

	klineAgg.Start()
	depthMgr.Start()
	tickerCalc.Start()

	defer klineAgg.Stop()
	defer depthMgr.Stop()
	defer tickerCalc.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup
	done := make(chan struct{})
	baseTime := time.Now().UnixMilli()

	// K线写入
	wg.Add(1)
	go func() {
		defer wg.Done()
		seq := int64(0)
		for {
			select {
			case <-done:
				return
			default:
				price := decimal.NewFromFloat(50000.0 + float64(seq%100))
				trade := &model.TradeEvent{
					Market:      "BTC-USDC",
					Price:       price.String(),
					Amount:      "1.0",
					QuoteAmount: "50000",
					Timestamp:   baseTime + seq,
				}
				klineAgg.ProcessTrade(ctx, trade)
				tickerCalc.ProcessTrade(ctx, trade)
				seq++
			}
		}
	}()

	// 深度更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		seq := uint64(1)
		for {
			select {
			case <-done:
				return
			default:
				update := &model.DepthUpdate{
					Market:   "BTC-USDC",
					Sequence: seq,
					Bids: []*model.PriceLevel{
						{Price: decimal.NewFromFloat(49900.0), Amount: decimal.NewFromFloat(10.0)},
					},
					Asks: []*model.PriceLevel{
						{Price: decimal.NewFromFloat(50100.0), Amount: decimal.NewFromFloat(5.0)},
					},
				}
				depthMgr.ApplyUpdate(ctx, update)
				seq++
			}
		}
	}()

	// 并发读取所有聚合器
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = klineAgg.GetKline(model.Interval1m)
					_ = depthMgr.GetDepth(20)
					_ = tickerCalc.GetTicker()
				}
			}
		}()
	}

	// 运行一段时间
	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()

	// 验证最终状态一致性
	require.True(t, klineAgg.Stats()["trade_count"] > 0)
	require.True(t, depthMgr.Stats()["bid_levels"] > 0 || depthMgr.Stats()["ask_levels"] > 0)
	require.True(t, tickerCalc.Stats()["trade_count"] > 0)
}
