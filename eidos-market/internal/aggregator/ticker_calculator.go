package aggregator

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

const (
	// BucketCount 24小时 * 60分钟 = 1440 个桶
	BucketCount = 1440
	// BucketDuration 每个桶的时长（毫秒）
	BucketDuration = 60_000
	// Window24H 24小时窗口（毫秒）
	Window24H = 24 * 60 * 60 * 1000
)

// TickerPublisher Ticker 发布接口
type TickerPublisher interface {
	PublishTicker(ctx context.Context, market string, ticker *model.Ticker) error
}

// TickerCalculatorConfig Ticker 计算器配置
type TickerCalculatorConfig struct {
	PublishInterval time.Duration // 发布间隔
}

// DefaultTickerCalculatorConfig 默认配置
func DefaultTickerCalculatorConfig() TickerCalculatorConfig {
	return TickerCalculatorConfig{
		PublishInterval: time.Second, // 每秒发布一次
	}
}

// TickerCalculator Ticker 计算器
// 使用环形缓冲区存储 1440 个分钟桶，实现 24 小时滑动窗口聚合
type TickerCalculator struct {
	market    string
	config    TickerCalculatorConfig
	buckets   [BucketCount]*model.TickerBucket // 环形缓冲区
	mu        sync.RWMutex

	// 最优买卖价（从深度数据更新）
	bestBid    decimal.Decimal
	bestBidQty decimal.Decimal
	bestAsk    decimal.Decimal
	bestAskQty decimal.Decimal
	priceMu    sync.RWMutex

	// 最新成交价和时间
	lastPrice     decimal.Decimal
	lastTradeTime int64

	publisher TickerPublisher
	logger    *slog.Logger

	closeCh   chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	closed    atomic.Bool

	// 统计指标
	tradeCount   atomic.Int64
	publishCount atomic.Int64
}

// NewTickerCalculator 创建 Ticker 计算器
func NewTickerCalculator(
	market string,
	publisher TickerPublisher,
	logger *slog.Logger,
	config TickerCalculatorConfig,
) *TickerCalculator {
	tc := &TickerCalculator{
		market:    market,
		config:    config,
		publisher: publisher,
		logger:    logger.With("market", market),
		closeCh:   make(chan struct{}),
	}
	return tc
}

// Start 启动 Ticker 计算器
func (tc *TickerCalculator) Start() {
	if tc.publisher != nil {
		tc.wg.Add(1)
		go tc.publishLoop()
	}
	tc.logger.Info("ticker calculator started")
}

// Stop 停止 Ticker 计算器
func (tc *TickerCalculator) Stop() {
	tc.closeOnce.Do(func() {
		tc.closed.Store(true)
		close(tc.closeCh)
		tc.wg.Wait()
		tc.logger.Info("ticker calculator stopped",
			"total_trades", tc.tradeCount.Load(),
			"total_publishes", tc.publishCount.Load())
	})
}

// ProcessTrade 处理成交事件
func (tc *TickerCalculator) ProcessTrade(ctx context.Context, trade *model.TradeEvent) error {
	if tc.closed.Load() {
		return nil
	}

	price, err := trade.GetPrice()
	if err != nil {
		tc.logger.Error("invalid trade price", "price", trade.Price, "error", err)
		return err
	}
	amount, err := trade.GetAmount()
	if err != nil {
		tc.logger.Error("invalid trade amount", "amount", trade.Amount, "error", err)
		return err
	}
	quoteAmount, err := trade.GetQuoteAmount()
	if err != nil {
		tc.logger.Error("invalid trade quote_amount", "quote_amount", trade.QuoteAmount, "error", err)
		return err
	}
	timestamp := trade.Timestamp

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 计算分钟时间戳
	minute := model.CalculateMinute(timestamp)
	idx := tc.bucketIndex(minute)

	// 获取或创建桶
	bucket := tc.buckets[idx]
	if bucket == nil || bucket.Minute != minute {
		// 新分钟，创建新桶
		bucket = model.NewTickerBucket(minute)
		tc.buckets[idx] = bucket
	}

	// 更新桶
	bucket.Update(price, amount, quoteAmount, timestamp)

	// 更新最新成交价
	tc.lastPrice = price
	tc.lastTradeTime = timestamp

	tc.tradeCount.Add(1)

	return nil
}

// bucketIndex 计算桶索引
func (tc *TickerCalculator) bucketIndex(minute int64) int {
	return int((minute / BucketDuration) % BucketCount)
}

// UpdateBestPrices 更新最优买卖价（从深度数据）
func (tc *TickerCalculator) UpdateBestPrices(bestBid, bestBidQty, bestAsk, bestAskQty decimal.Decimal) {
	tc.priceMu.Lock()
	tc.bestBid = bestBid
	tc.bestBidQty = bestBidQty
	tc.bestAsk = bestAsk
	tc.bestAskQty = bestAskQty
	tc.priceMu.Unlock()
}

// GetTicker 获取当前 Ticker
func (tc *TickerCalculator) GetTicker() *model.Ticker {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	now := time.Now().UnixMilli()
	windowStart := now - Window24H

	ticker := &model.Ticker{
		Market:    tc.market,
		Timestamp: now,
	}

	// 获取最优买卖价
	tc.priceMu.RLock()
	ticker.BestBid = tc.bestBid
	ticker.BestBidQty = tc.bestBidQty
	ticker.BestAsk = tc.bestAsk
	ticker.BestAskQty = tc.bestAskQty
	tc.priceMu.RUnlock()

	// 聚合所有 24 小时内的桶
	var (
		firstBucket *model.TickerBucket
		lastBucket  *model.TickerBucket
	)

	for _, bucket := range tc.buckets {
		if bucket == nil || bucket.Minute < windowStart || bucket.TradeCount == 0 {
			continue
		}

		// 找到第一个有效桶
		if firstBucket == nil || bucket.FirstTrade < firstBucket.FirstTrade {
			firstBucket = bucket
		}

		// 找到最后一个有效桶
		if lastBucket == nil || bucket.LastTrade > lastBucket.LastTrade {
			lastBucket = bucket
		}

		// 聚合高低价和成交量
		if ticker.High.IsZero() || bucket.High.GreaterThan(ticker.High) {
			ticker.High = bucket.High
		}
		if ticker.Low.IsZero() || bucket.Low.LessThan(ticker.Low) {
			ticker.Low = bucket.Low
		}

		ticker.Volume = ticker.Volume.Add(bucket.Volume)
		ticker.QuoteVolume = ticker.QuoteVolume.Add(bucket.QuoteVolume)
		ticker.TradeCount += bucket.TradeCount
	}

	// 设置开盘价和收盘价
	if firstBucket != nil {
		ticker.Open = firstBucket.Open
	}
	if lastBucket != nil {
		ticker.Close = lastBucket.Close
		ticker.LastPrice = lastBucket.Close
	}

	// 如果有最新成交价，使用它
	if !tc.lastPrice.IsZero() && tc.lastTradeTime > windowStart {
		ticker.LastPrice = tc.lastPrice
	}

	// 计算价格变化
	if !ticker.Open.IsZero() && !ticker.LastPrice.IsZero() {
		ticker.PriceChange = ticker.LastPrice.Sub(ticker.Open)
		ticker.PriceChangePercent = ticker.PriceChange.Div(ticker.Open).Mul(decimal.NewFromInt(100))
	}

	return ticker
}

// publishLoop 定时发布循环
func (tc *TickerCalculator) publishLoop() {
	defer tc.wg.Done()

	ticker := time.NewTicker(tc.config.PublishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tc.publish()
		case <-tc.closeCh:
			return
		}
	}
}

// publish 发布 Ticker
func (tc *TickerCalculator) publish() {
	if tc.publisher == nil {
		return
	}

	t := tc.GetTicker()
	if t.TradeCount == 0 && t.LastPrice.IsZero() {
		return // 没有数据，不发布
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := tc.publisher.PublishTicker(ctx, tc.market, t); err != nil {
		tc.logger.Warn("failed to publish ticker", "error", err)
		metrics.PubSubPublishErrors.WithLabelValues("ticker").Inc()
	} else {
		tc.publishCount.Add(1)
		metrics.TickerUpdatesTotal.WithLabelValues(tc.market).Inc()
		metrics.PubSubPublishTotal.WithLabelValues("ticker").Inc()
	}
}

// GetBucket 获取指定分钟的桶（用于调试）
func (tc *TickerCalculator) GetBucket(minute int64) *model.TickerBucket {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	idx := tc.bucketIndex(minute)
	bucket := tc.buckets[idx]
	if bucket != nil && bucket.Minute == minute {
		return bucket.Clone()
	}
	return nil
}

// CleanExpiredBuckets 清理过期桶
func (tc *TickerCalculator) CleanExpiredBuckets() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now().UnixMilli()
	windowStart := now - Window24H
	cleaned := 0

	for i, bucket := range tc.buckets {
		if bucket != nil && bucket.Minute < windowStart {
			tc.buckets[i] = nil
			cleaned++
		}
	}

	return cleaned
}

// Stats 返回统计信息
func (tc *TickerCalculator) Stats() map[string]int64 {
	tc.mu.RLock()
	activeBuckets := 0
	for _, bucket := range tc.buckets {
		if bucket != nil && bucket.TradeCount > 0 {
			activeBuckets++
		}
	}
	tc.mu.RUnlock()

	// 更新 Prometheus 指标
	metrics.TickerBucketsActive.WithLabelValues(tc.market).Set(float64(activeBuckets))

	return map[string]int64{
		"trade_count":    tc.tradeCount.Load(),
		"publish_count":  tc.publishCount.Load(),
		"active_buckets": int64(activeBuckets),
	}
}
