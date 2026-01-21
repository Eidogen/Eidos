// Package engine 引擎管理器
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
)

// EngineManager 引擎管理器
// 管理多个市场的撮合引擎，根据 market 路由到对应的引擎
type EngineManager struct {
	engines map[string]*Engine // market -> engine
	configs map[string]*MarketConfig
	mu      sync.RWMutex

	// ID 生成器
	tradeIDGen func() string

	// 配置
	channelSize int
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	Markets     []*MarketConfig
	TradeIDGen  func() string
	ChannelSize int
}

// NewEngineManager 创建引擎管理器
func NewEngineManager(cfg *ManagerConfig) *EngineManager {
	if cfg.ChannelSize <= 0 {
		cfg.ChannelSize = 10000
	}

	m := &EngineManager{
		engines:     make(map[string]*Engine),
		configs:     make(map[string]*MarketConfig),
		tradeIDGen:  cfg.TradeIDGen,
		channelSize: cfg.ChannelSize,
	}

	// 初始化市场配置
	for _, marketCfg := range cfg.Markets {
		m.configs[marketCfg.Symbol] = marketCfg
	}

	return m
}

// Start 启动所有引擎
func (m *EngineManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for symbol, cfg := range m.configs {
		engine := NewEngine(&EngineConfig{
			Market:      cfg,
			TradeIDGen:  m.tradeIDGen,
			ChannelSize: m.channelSize,
		})
		engine.Start()
		m.engines[symbol] = engine
	}

	return nil
}

// Stop 停止所有引擎
func (m *EngineManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, engine := range m.engines {
		engine.Stop()
	}
}

// GetEngine 获取指定市场的引擎
func (m *EngineManager) GetEngine(market string) (*Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	engine, exists := m.engines[market]
	if !exists {
		return nil, fmt.Errorf("engine not found for market: %s", market)
	}
	return engine, nil
}

// ProcessOrder 处理订单 (路由到对应引擎)
func (m *EngineManager) ProcessOrder(ctx context.Context, order *model.Order) (*MatchResult, error) {
	engine, err := m.GetEngine(order.Market)
	if err != nil {
		return nil, err
	}
	return engine.ProcessOrder(ctx, order)
}

// ProcessCancel 处理取消请求 (路由到对应引擎)
func (m *EngineManager) ProcessCancel(ctx context.Context, msg *model.CancelMessage) (*model.CancelResult, error) {
	engine, err := m.GetEngine(msg.Market)
	if err != nil {
		return nil, err
	}
	return engine.ProcessCancel(ctx, msg)
}

// GetDepth 获取深度快照
func (m *EngineManager) GetDepth(market string, levels int) (*orderbook.DepthSnapshot, error) {
	engine, err := m.GetEngine(market)
	if err != nil {
		return nil, err
	}
	return engine.GetDepth(levels)
}

// GetOrder 获取订单
func (m *EngineManager) GetOrder(market, orderID string) *model.Order {
	engine, err := m.GetEngine(market)
	if err != nil {
		return nil
	}
	return engine.GetOrder(orderID)
}

// GetStats 获取所有引擎统计
func (m *EngineManager) GetStats() map[string]*EngineStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]*EngineStats)
	for market, engine := range m.engines {
		stats[market] = engine.GetStats()
	}
	return stats
}

// GetMarkets 获取所有市场
func (m *EngineManager) GetMarkets() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	markets := make([]string, 0, len(m.engines))
	for market := range m.engines {
		markets = append(markets, market)
	}
	return markets
}

// AddMarket 动态添加市场
// 配合配置中心 (Nacos) 实现热加载，参见 eidos-matching/internal/config/nacos_loader.go
func (m *EngineManager) AddMarket(cfg *MarketConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.engines[cfg.Symbol]; exists {
		return fmt.Errorf("market already exists: %s", cfg.Symbol)
	}

	engine := NewEngine(&EngineConfig{
		Market:      cfg,
		TradeIDGen:  m.tradeIDGen,
		ChannelSize: m.channelSize,
	})
	engine.Start()

	m.configs[cfg.Symbol] = cfg
	m.engines[cfg.Symbol] = engine

	return nil
}

// RemoveMarket 移除市场
// 注意: 需要确保订单簿为空
func (m *EngineManager) RemoveMarket(market string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	engine, exists := m.engines[market]
	if !exists {
		return fmt.Errorf("market not found: %s", market)
	}

	// 检查订单簿是否为空
	stats := engine.GetStats()
	if stats.OrderBookStats.OrderCount > 0 {
		return fmt.Errorf("cannot remove market with active orders: %d", stats.OrderBookStats.OrderCount)
	}

	engine.Stop()
	delete(m.engines, market)
	delete(m.configs, market)

	return nil
}

// CollectTrades 收集所有引擎的成交 (用于 Kafka 生产者)
func (m *EngineManager) CollectTrades(ctx context.Context, handler func(*model.TradeResult) error) {
	m.mu.RLock()
	engines := make([]*Engine, 0, len(m.engines))
	for _, e := range m.engines {
		engines = append(engines, e)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, engine := range engines {
		wg.Add(1)
		go func(e *Engine) {
			defer wg.Done()
			market := e.market
			retryCount := 0
			maxRetries := 3
			retryBackoff := 100 * time.Millisecond

			for {
				select {
				case <-ctx.Done():
					return
				case trade, ok := <-e.TradesChan():
					if !ok {
						return
					}
					// 带重试的错误处理
					for attempt := 0; attempt <= maxRetries; attempt++ {
						if err := handler(trade); err != nil {
							retryCount++
							metrics.RecordKafkaError("trade-results", "send")
							slog.Warn("send trade result failed",
								"market", market,
								"trade_id", trade.TradeID,
								"attempt", attempt+1,
								"error", err)

							if attempt < maxRetries {
								select {
								case <-ctx.Done():
									return
								case <-time.After(retryBackoff * time.Duration(attempt+1)):
									continue
								}
							}
							// 达到最大重试次数，记录并继续
							slog.Error("send trade result max retries exceeded",
								"market", market,
								"trade_id", trade.TradeID,
								"total_retries", retryCount)
						} else {
							break // 成功
						}
					}
				}
			}
		}(engine)
	}
	wg.Wait()
}

// CollectUpdates 收集所有引擎的订单簿更新
func (m *EngineManager) CollectUpdates(ctx context.Context, handler func(*model.OrderBookUpdate) error) {
	m.mu.RLock()
	engines := make([]*Engine, 0, len(m.engines))
	for _, e := range m.engines {
		engines = append(engines, e)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, engine := range engines {
		wg.Add(1)
		go func(e *Engine) {
			defer wg.Done()
			market := e.market
			retryCount := 0
			maxRetries := 2 // 订单簿更新容忍度更高，重试次数较少
			retryBackoff := 50 * time.Millisecond

			for {
				select {
				case <-ctx.Done():
					return
				case update, ok := <-e.UpdatesChan():
					if !ok {
						return
					}
					// 带重试的错误处理
					for attempt := 0; attempt <= maxRetries; attempt++ {
						if err := handler(update); err != nil {
							retryCount++
							metrics.RecordKafkaError("orderbook-updates", "send")
							slog.Debug("send orderbook update failed",
								"market", market,
								"update_type", update.UpdateType,
								"attempt", attempt+1,
								"error", err)

							if attempt < maxRetries {
								select {
								case <-ctx.Done():
									return
								case <-time.After(retryBackoff * time.Duration(attempt+1)):
									continue
								}
							}
							// 订单簿更新丢失可接受（客户端可以重新订阅），记录日志即可
							slog.Warn("send orderbook update dropped",
								"market", market,
								"price", update.Price.String())
						} else {
							break // 成功
						}
					}
				}
			}
		}(engine)
	}
	wg.Wait()
}

// CollectCancels 收集所有引擎的取消结果
func (m *EngineManager) CollectCancels(ctx context.Context, handler func(*model.CancelResult) error) {
	m.mu.RLock()
	engines := make([]*Engine, 0, len(m.engines))
	for _, e := range m.engines {
		engines = append(engines, e)
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, engine := range engines {
		wg.Add(1)
		go func(e *Engine) {
			defer wg.Done()
			market := e.market
			retryCount := 0
			maxRetries := 3
			retryBackoff := 100 * time.Millisecond

			for {
				select {
				case <-ctx.Done():
					return
				case cancel, ok := <-e.CancelsChan():
					if !ok {
						return
					}
					// 带重试的错误处理
					for attempt := 0; attempt <= maxRetries; attempt++ {
						if err := handler(cancel); err != nil {
							retryCount++
							metrics.RecordKafkaError("order-cancelled", "send")
							slog.Warn("send cancel result failed",
								"market", market,
								"order_id", cancel.OrderID,
								"attempt", attempt+1,
								"error", err)

							if attempt < maxRetries {
								select {
								case <-ctx.Done():
									return
								case <-time.After(retryBackoff * time.Duration(attempt+1)):
									continue
								}
							}
							// 达到最大重试次数
							slog.Error("send cancel result max retries exceeded",
								"market", market,
								"order_id", cancel.OrderID,
								"total_retries", retryCount)
						} else {
							break // 成功
						}
					}
				}
			}
		}(engine)
	}
	wg.Wait()
}

// UpdateMarket 更新市场配置 (热更新)
func (m *EngineManager) UpdateMarket(cfg *MarketConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	engine, exists := m.engines[cfg.Symbol]
	if !exists {
		return fmt.Errorf("market not found: %s", cfg.Symbol)
	}

	// 更新配置 (费率等可以热更新)
	m.configs[cfg.Symbol] = cfg
	engine.UpdateConfig(cfg)

	slog.Info("market config updated",
		"market", cfg.Symbol,
		"maker_fee", cfg.MakerFeeRate.String(),
		"taker_fee", cfg.TakerFeeRate.String())

	return nil
}

// UpdateIndexPrice 更新外部指数价格
func (m *EngineManager) UpdateIndexPrice(market string, price decimal.Decimal) error {
	engine, err := m.GetEngine(market)
	if err != nil {
		return err
	}
	engine.UpdateIndexPrice(price)
	return nil
}

// GetAllEngines 获取所有引擎 (用于快照等)
func (m *EngineManager) GetAllEngines() map[string]*Engine {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Engine, len(m.engines))
	for k, v := range m.engines {
		result[k] = v
	}
	return result
}
