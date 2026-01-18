package aggregator

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// KlinePublisher K线发布接口
type KlinePublisher interface {
	PublishKline(ctx context.Context, market string, interval model.KlineInterval, kline *model.Kline) error
}

// KlineRepository K线持久化接口
type KlineRepository interface {
	Upsert(ctx context.Context, kline *model.Kline) error
	BatchUpsert(ctx context.Context, klines []*model.Kline) error
}

// KlineAggregatorConfig 聚合器配置
type KlineAggregatorConfig struct {
	FlushInterval time.Duration // 刷盘间隔
	BufferSize    int           // 缓冲区大小
}

// DefaultKlineAggregatorConfig 默认配置
func DefaultKlineAggregatorConfig() KlineAggregatorConfig {
	return KlineAggregatorConfig{
		FlushInterval: 10 * time.Second,
		BufferSize:    1000,
	}
}

// KlineAggregator K线聚合器
// 每个 market 一个实例，支持所有周期的 K 线聚合
type KlineAggregator struct {
	market    string
	config    KlineAggregatorConfig
	klines    map[model.KlineInterval]*model.Kline // 当前 K 线
	mu        sync.RWMutex
	dirty     map[model.KlineInterval]bool // 标记哪些 K 线有更新
	dirtyMu   sync.Mutex

	repo      KlineRepository
	publisher KlinePublisher
	logger    *zap.Logger

	flushCh   chan struct{}
	closeCh   chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	closed    atomic.Bool

	// 统计指标
	tradeCount   atomic.Int64
	publishCount atomic.Int64
	flushCount   atomic.Int64
}

// NewKlineAggregator 创建 K 线聚合器
func NewKlineAggregator(
	market string,
	repo KlineRepository,
	publisher KlinePublisher,
	logger *zap.Logger,
	config KlineAggregatorConfig,
) *KlineAggregator {
	agg := &KlineAggregator{
		market:    market,
		config:    config,
		klines:    make(map[model.KlineInterval]*model.Kline),
		dirty:     make(map[model.KlineInterval]bool),
		repo:      repo,
		publisher: publisher,
		logger:    logger.With(zap.String("market", market)),
		flushCh:   make(chan struct{}, 1),
		closeCh:   make(chan struct{}),
	}

	// 初始化所有周期的当前 K 线
	now := time.Now()
	for _, interval := range model.AllIntervals {
		agg.klines[interval] = agg.createNewKline(interval, now)
	}

	return agg
}

// Start 启动聚合器
func (a *KlineAggregator) Start() {
	a.wg.Add(1)
	go a.flushLoop()
	a.logger.Info("kline aggregator started")
}

// Stop 停止聚合器
func (a *KlineAggregator) Stop() {
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		close(a.closeCh)
		a.wg.Wait()
		// 最后一次刷盘
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.flushAll(ctx)
		a.logger.Info("kline aggregator stopped",
			zap.Int64("total_trades", a.tradeCount.Load()),
			zap.Int64("total_publishes", a.publishCount.Load()),
			zap.Int64("total_flushes", a.flushCount.Load()))
	})
}

// ProcessTrade 处理成交事件
func (a *KlineAggregator) ProcessTrade(ctx context.Context, trade *model.TradeEvent) error {
	if a.closed.Load() {
		return nil
	}

	price, err := trade.GetPrice()
	if err != nil {
		a.logger.Error("invalid trade price", zap.String("price", trade.Price), zap.Error(err))
		return err
	}
	amount, err := trade.GetAmount()
	if err != nil {
		a.logger.Error("invalid trade amount", zap.String("amount", trade.Amount), zap.Error(err))
		return err
	}
	quoteAmount, err := trade.GetQuoteAmount()
	if err != nil {
		a.logger.Error("invalid trade quote_amount", zap.String("quote_amount", trade.QuoteAmount), zap.Error(err))
		return err
	}
	timestamp := trade.Timestamp

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, interval := range model.AllIntervals {
		kline := a.klines[interval]

		// 检查是否需要切换到新 K 线
		if timestamp >= kline.CloseTime {
			// 保存旧 K 线（异步）
			if kline.TradeCount > 0 {
				a.markDirty(interval)
			}
			// 创建新 K 线
			a.klines[interval] = a.createNewKline(interval, time.UnixMilli(timestamp))
			kline = a.klines[interval]
		}

		// 更新 K 线
		a.updateKline(kline, price, amount, quoteAmount)
		a.markDirty(interval)
	}

	a.tradeCount.Add(1)

	// 发布实时更新
	if a.publisher != nil {
		go a.publishKlineUpdates(ctx)
	}

	return nil
}

// updateKline 更新 K 线数据
func (a *KlineAggregator) updateKline(kline *model.Kline, price, amount, quoteAmount decimal.Decimal) {
	if kline.TradeCount == 0 {
		kline.Open = price
		kline.High = price
		kline.Low = price
	} else {
		if price.GreaterThan(kline.High) {
			kline.High = price
		}
		if price.LessThan(kline.Low) {
			kline.Low = price
		}
	}
	kline.Close = price
	kline.Volume = kline.Volume.Add(amount)
	kline.QuoteVolume = kline.QuoteVolume.Add(quoteAmount)
	kline.TradeCount++
}

// createNewKline 创建新 K 线
func (a *KlineAggregator) createNewKline(interval model.KlineInterval, t time.Time) *model.Kline {
	openTime := model.CalculateOpenTime(t.UnixMilli(), interval)
	closeTime := model.CalculateCloseTime(openTime, interval)

	return &model.Kline{
		Market:      a.market,
		Interval:    interval,
		OpenTime:    openTime,
		CloseTime:   closeTime,
		Open:        decimal.Zero,
		High:        decimal.Zero,
		Low:         decimal.Zero,
		Close:       decimal.Zero,
		Volume:      decimal.Zero,
		QuoteVolume: decimal.Zero,
		TradeCount:  0,
	}
}

// markDirty 标记 K 线为脏（需要持久化）
func (a *KlineAggregator) markDirty(interval model.KlineInterval) {
	a.dirtyMu.Lock()
	a.dirty[interval] = true
	a.dirtyMu.Unlock()
}

// getDirtyAndClear 获取并清除脏标记
func (a *KlineAggregator) getDirtyAndClear() map[model.KlineInterval]bool {
	a.dirtyMu.Lock()
	defer a.dirtyMu.Unlock()
	dirty := a.dirty
	a.dirty = make(map[model.KlineInterval]bool)
	return dirty
}

// flushLoop 定时刷盘循环
func (a *KlineAggregator) flushLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			a.flushAll(ctx)
			cancel()
		case <-a.flushCh:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			a.flushAll(ctx)
			cancel()
		case <-a.closeCh:
			return
		}
	}
}

// flushAll 刷新所有脏 K 线到数据库
func (a *KlineAggregator) flushAll(ctx context.Context) {
	if a.repo == nil {
		return
	}

	dirty := a.getDirtyAndClear()
	if len(dirty) == 0 {
		return
	}

	startTime := time.Now()

	a.mu.RLock()
	klines := make([]*model.Kline, 0, len(dirty))
	for interval := range dirty {
		if kline, ok := a.klines[interval]; ok && kline.TradeCount > 0 {
			klines = append(klines, kline.Clone())
		}
	}
	a.mu.RUnlock()

	if len(klines) == 0 {
		return
	}

	if err := a.repo.BatchUpsert(ctx, klines); err != nil {
		a.logger.Error("failed to flush klines", zap.Error(err))
		metrics.KlineFlushErrors.WithLabelValues(a.market, "db_error").Inc()
		// 重新标记为脏，下次重试
		for _, kline := range klines {
			a.markDirty(kline.Interval)
		}
		return
	}

	a.flushCount.Add(1)
	metrics.KlineFlushDuration.WithLabelValues(a.market).Observe(time.Since(startTime).Seconds())

	// 记录每个周期的 K 线生成数
	for _, kline := range klines {
		metrics.KlinesGeneratedTotal.WithLabelValues(a.market, string(kline.Interval)).Inc()
	}

	a.logger.Debug("flushed klines", zap.Int("count", len(klines)))
}

// Flush 手动触发刷盘
func (a *KlineAggregator) Flush() {
	select {
	case a.flushCh <- struct{}{}:
	default:
	}
}

// publishKlineUpdates 发布 K 线更新
func (a *KlineAggregator) publishKlineUpdates(ctx context.Context) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for interval, kline := range a.klines {
		if kline.TradeCount > 0 {
			if err := a.publisher.PublishKline(ctx, a.market, interval, kline.Clone()); err != nil {
				a.logger.Warn("failed to publish kline",
					zap.String("interval", string(interval)),
					zap.Error(err))
				metrics.PubSubPublishErrors.WithLabelValues("kline").Inc()
			} else {
				a.publishCount.Add(1)
				metrics.PubSubPublishTotal.WithLabelValues("kline").Inc()
			}
		}
	}
}

// GetKline 获取当前 K 线快照
func (a *KlineAggregator) GetKline(interval model.KlineInterval) *model.Kline {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if kline, ok := a.klines[interval]; ok {
		return kline.Clone()
	}
	return nil
}

// GetAllKlines 获取所有周期的当前 K 线
func (a *KlineAggregator) GetAllKlines() map[model.KlineInterval]*model.Kline {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[model.KlineInterval]*model.Kline, len(a.klines))
	for interval, kline := range a.klines {
		result[interval] = kline.Clone()
	}
	return result
}

// Stats 返回统计信息
func (a *KlineAggregator) Stats() map[string]int64 {
	return map[string]int64{
		"trade_count":   a.tradeCount.Load(),
		"publish_count": a.publishCount.Load(),
		"flush_count":   a.flushCount.Load(),
	}
}
