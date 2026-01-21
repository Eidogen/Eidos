// Package engine 撮合引擎
//
// 核心组件说明:
//   - Engine: 单市场撮合引擎，每个交易对独立实例
//   - EngineManager: 管理多个 Engine 实例
//   - Matcher: 撮合逻辑，价格-时间优先
//
// 输出通道:
//   - tradesChan: 成交结果，发送到 Kafka trade-results topic
//   - updatesChan: 订单簿更新，发送到 Kafka orderbook-updates topic
//   - cancelsChan: 取消结果，发送到 Kafka order-cancelled topic
//
// 对接清单:
//   - 监控指标: 撮合延迟、吞吐量、订单簿深度
//   - 告警系统: 通道满、延迟过高
//   - SetOrderBook: 支持从快照恢复订单簿
package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
)

// Engine 单市场撮合引擎
// 每个交易对一个独立的 Engine 实例，内部单线程串行处理
type Engine struct {
	market    string
	config    *MarketConfig
	orderBook *orderbook.OrderBook
	matcher   *Matcher

	// 消息通道 (用于异步输出)
	tradesChan  chan *model.TradeResult
	updatesChan chan *model.OrderBookUpdate
	cancelsChan chan *model.CancelResult

	// 序列号管理
	inputSequence  int64 // 最后处理的输入消息序列号
	outputSequence int64 // 输出序列号

	// 统计
	ordersProcessed int64
	tradesGenerated int64
	cancelProcessed int64
	avgLatencyUs    int64 // 平均撮合延迟 (微秒)

	// 状态
	isRunning atomic.Bool
	mu        sync.RWMutex

	// ID 生成器
	tradeIDGen func() string
}

// EngineConfig 引擎配置
type EngineConfig struct {
	Market      *MarketConfig
	TradeIDGen  func() string
	ChannelSize int // 输出通道缓冲大小
}

// NewEngine 创建撮合引擎
func NewEngine(cfg *EngineConfig) *Engine {
	if cfg.ChannelSize <= 0 {
		cfg.ChannelSize = 10000 // 默认缓冲大小
	}

	ob := orderbook.NewOrderBook(cfg.Market.Symbol)

	e := &Engine{
		market:      cfg.Market.Symbol,
		config:      cfg.Market,
		orderBook:   ob,
		tradesChan:  make(chan *model.TradeResult, cfg.ChannelSize),
		updatesChan: make(chan *model.OrderBookUpdate, cfg.ChannelSize),
		cancelsChan: make(chan *model.CancelResult, cfg.ChannelSize),
		tradeIDGen:  cfg.TradeIDGen,
	}

	e.matcher = NewMatcher(cfg.Market, ob, cfg.TradeIDGen)
	return e
}

// Start 启动引擎
func (e *Engine) Start() {
	e.isRunning.Store(true)
}

// Stop 停止引擎
func (e *Engine) Stop() {
	e.isRunning.Store(false)
	close(e.tradesChan)
	close(e.updatesChan)
	close(e.cancelsChan)
}

// IsRunning 检查引擎是否运行中
func (e *Engine) IsRunning() bool {
	return e.isRunning.Load()
}

// ProcessOrder 处理新订单
// 返回撮合结果，同时将成交和更新发送到输出通道
func (e *Engine) ProcessOrder(ctx context.Context, order *model.Order) (*MatchResult, error) {
	if !e.isRunning.Load() {
		return nil, fmt.Errorf("engine not running")
	}

	startTime := time.Now()

	// 幂等性检查: 检查序列号
	if order.Sequence <= e.inputSequence {
		// 已处理过的消息，跳过
		return nil, nil
	}

	// 幂等性检查: 检查订单是否已存在
	if e.orderBook.HasOrder(order.OrderID) {
		return nil, nil
	}

	// 执行撮合
	result, err := e.matcher.Match(order)
	if err != nil {
		return nil, fmt.Errorf("match order failed: %w", err)
	}

	// 更新序列号
	e.inputSequence = order.Sequence

	// 发送成交结果到输出通道
	for _, trade := range result.Trades {
		select {
		case e.tradesChan <- trade:
		default:
			// 通道满了，记录告警指标
			metrics.RecordChannelOverflow(e.market, "trades")
		}
	}

	// 发送订单簿更新到输出通道
	for _, update := range result.OrderUpdates {
		select {
		case e.updatesChan <- update:
		default:
			// 通道满了，记录告警指标
			metrics.RecordChannelOverflow(e.market, "updates")
		}
	}

	// 更新统计
	atomic.AddInt64(&e.ordersProcessed, 1)
	atomic.AddInt64(&e.tradesGenerated, int64(len(result.Trades)))

	// 更新延迟统计
	latencyUs := time.Since(startTime).Microseconds()
	e.updateAvgLatency(latencyUs)

	return result, nil
}

// ProcessCancel 处理取消请求
func (e *Engine) ProcessCancel(ctx context.Context, msg *model.CancelMessage) (*model.CancelResult, error) {
	if !e.isRunning.Load() {
		return nil, fmt.Errorf("engine not running")
	}

	// 幂等性检查
	if msg.Sequence <= e.inputSequence {
		return nil, nil
	}

	result := &model.CancelResult{
		OrderID:   msg.OrderID,
		Market:    msg.Market,
		Wallet:    msg.Wallet,
		Timestamp: time.Now().UnixNano(),
		Sequence:  e.orderBook.NextSequence(),
	}

	// 从订单簿移除
	order := e.orderBook.RemoveOrder(msg.OrderID)
	if order == nil {
		// 订单不存在
		result.Success = false
		result.Reason = "ORDER_NOT_FOUND"
		result.RemainingSize = decimal.Zero
	} else {
		result.Success = true
		result.RemainingSize = order.Remaining

		// 发送订单簿更新
		update := &model.OrderBookUpdate{
			Market:     order.Market,
			UpdateType: "REMOVE",
			Side:       order.Side,
			Price:      order.Price,
			Size:       decimal.Zero,
			Timestamp:  result.Timestamp,
			Sequence:   result.Sequence,
		}
		select {
		case e.updatesChan <- update:
		default:
			metrics.RecordChannelOverflow(e.market, "updates")
		}
	}

	// 更新序列号
	e.inputSequence = msg.Sequence

	// 发送取消结果
	select {
	case e.cancelsChan <- result:
	default:
		metrics.RecordChannelOverflow(e.market, "cancels")
	}

	atomic.AddInt64(&e.cancelProcessed, 1)

	return result, nil
}

// GetOrder 获取订单
func (e *Engine) GetOrder(orderID string) *model.Order {
	return e.orderBook.GetOrder(orderID)
}

// GetDepth 获取深度快照
func (e *Engine) GetDepth(levels int) (*orderbook.DepthSnapshot, error) {
	return e.orderBook.Depth(levels)
}

// GetStats 获取引擎统计
func (e *Engine) GetStats() *EngineStats {
	obStats := e.orderBook.Stats()
	matcherStats := e.matcher.GetStats()

	return &EngineStats{
		Market:          e.market,
		OrdersProcessed: atomic.LoadInt64(&e.ordersProcessed),
		TradesGenerated: atomic.LoadInt64(&e.tradesGenerated),
		CancelProcessed: atomic.LoadInt64(&e.cancelProcessed),
		AvgLatencyUs:    atomic.LoadInt64(&e.avgLatencyUs),
		InputSequence:   e.inputSequence,
		OutputSequence:  e.outputSequence,
		OrderBookStats:  obStats,
		MatcherStats:    matcherStats,
	}
}

// TradesChan 获取成交输出通道
func (e *Engine) TradesChan() <-chan *model.TradeResult {
	return e.tradesChan
}

// UpdatesChan 获取订单簿更新通道
func (e *Engine) UpdatesChan() <-chan *model.OrderBookUpdate {
	return e.updatesChan
}

// CancelsChan 获取取消结果通道
func (e *Engine) CancelsChan() <-chan *model.CancelResult {
	return e.cancelsChan
}

// SetInputSequence 设置输入序列号 (恢复时使用)
func (e *Engine) SetInputSequence(seq int64) {
	e.inputSequence = seq
}

// GetInputSequence 获取当前输入序列号
func (e *Engine) GetInputSequence() int64 {
	return e.inputSequence
}

// GetOrderBook 获取订单簿 (用于快照)
func (e *Engine) GetOrderBook() *orderbook.OrderBook {
	return e.orderBook
}

// SetOrderBook 设置订单簿 (用于从快照恢复)
//
// [快照恢复]: 此方法用于从 Redis 快照恢复订单簿状态
// 调用时机: 在 Engine.Start() 之前
// 注意事项:
//   - 需要同时恢复 inputSequence
//   - 恢复后需要从正确的 Kafka offset 继续消费
//
// 使用示例:
//
//	snapshot, _ := snapshotManager.LoadSnapshot(ctx, market, offsets)
//	ob := snapshotManager.RestoreOrderBook(snapshot)
//	engine.SetOrderBook(ob)
//	engine.SetInputSequence(snapshot.Sequence)
//	engine.Start()
func (e *Engine) SetOrderBook(ob *orderbook.OrderBook) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.orderBook = ob
	e.matcher = NewMatcher(e.config, ob, e.tradeIDGen)
}

// updateAvgLatency 更新平均延迟 (指数移动平均)
func (e *Engine) updateAvgLatency(latencyUs int64) {
	// 使用 EMA 计算: new_avg = old_avg * 0.9 + new_value * 0.1
	oldAvg := atomic.LoadInt64(&e.avgLatencyUs)
	if oldAvg == 0 {
		atomic.StoreInt64(&e.avgLatencyUs, latencyUs)
		return
	}
	newAvg := (oldAvg*9 + latencyUs) / 10
	atomic.StoreInt64(&e.avgLatencyUs, newAvg)
}

// UpdateConfig 更新市场配置 (热更新)
func (e *Engine) UpdateConfig(cfg *MarketConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 更新配置
	e.config = cfg

	// 更新撮合器配置
	e.matcher.UpdateConfig(cfg)
}

// UpdateIndexPrice 更新外部指数价格
func (e *Engine) UpdateIndexPrice(price decimal.Decimal) {
	e.matcher.UpdateIndexPrice(price)
}

// EngineStats 引擎统计
type EngineStats struct {
	Market          string                   `json:"market"`
	OrdersProcessed int64                    `json:"orders_processed"`
	TradesGenerated int64                    `json:"trades_generated"`
	CancelProcessed int64                    `json:"cancel_processed"`
	AvgLatencyUs    int64                    `json:"avg_latency_us"`
	InputSequence   int64                    `json:"input_sequence"`
	OutputSequence  int64                    `json:"output_sequence"`
	OrderBookStats  orderbook.OrderBookStats `json:"orderbook_stats"`
	MatcherStats    MatcherStats             `json:"matcher_stats"`
}
