// Package price 外部价格管理
// 实现多数据源聚合的指数价格管理
package price

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	// ErrNoValidPrice 无有效价格
	ErrNoValidPrice = errors.New("no valid price available")
	// ErrSourceNotFound 数据源未找到
	ErrSourceNotFound = errors.New("price source not found")
	// ErrPriceStale 价格过期
	ErrPriceStale = errors.New("price is stale")
)

// PriceSource 价格数据源接口
type PriceSource interface {
	// Name 数据源名称
	Name() string
	// FetchPrice 获取价格
	FetchPrice(ctx context.Context, symbol string) (decimal.Decimal, error)
	// IsHealthy 数据源是否健康
	IsHealthy() bool
	// Priority 优先级 (数字越小优先级越高)
	Priority() int
}

// PriceData 价格数据
type PriceData struct {
	Price       decimal.Decimal `json:"price"`
	Source      string          `json:"source"`
	UpdatedAt   time.Time       `json:"updated_at"`
	IsStale     bool            `json:"is_stale"`
	LatencyMs   int64           `json:"latency_ms"`
}

// IndexPrice 指数价格
type IndexPrice struct {
	Symbol      string          `json:"symbol"`
	Price       decimal.Decimal `json:"price"`
	Sources     []PriceData     `json:"sources"`
	AggMethod   string          `json:"agg_method"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Confidence  float64         `json:"confidence"` // 置信度 0-1
}

// AggregationMethod 聚合方法
type AggregationMethod string

const (
	// AggMedian 中位数聚合
	AggMedian AggregationMethod = "median"
	// AggWeightedAvg 加权平均聚合
	AggWeightedAvg AggregationMethod = "weighted_avg"
	// AggBestSource 最优数据源
	AggBestSource AggregationMethod = "best_source"
)

// IndexPriceManagerConfig 价格管理器配置
type IndexPriceManagerConfig struct {
	// 更新间隔
	UpdateInterval time.Duration
	// 价格过期时间
	StaleThreshold time.Duration
	// 聚合方法
	AggMethod AggregationMethod
	// 最小有效数据源数量
	MinValidSources int
	// 价格偏差阈值 (超过此值认为数据异常)
	DeviationThreshold decimal.Decimal
}

// UpdateCallback 价格更新回调
type UpdateCallback func(symbol string, price *IndexPrice)

// IndexPriceManager 指数价格管理器
type IndexPriceManager struct {
	config   *IndexPriceManagerConfig
	sources  []PriceSource
	prices   map[string]*IndexPrice
	callback UpdateCallback

	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	wg       sync.WaitGroup

	// 统计
	updateCount  int64
	errorCount   int64
	lastError    error
	lastErrorAt  time.Time
}

// NewIndexPriceManager 创建价格管理器
func NewIndexPriceManager(cfg *IndexPriceManagerConfig) *IndexPriceManager {
	if cfg.UpdateInterval <= 0 {
		cfg.UpdateInterval = time.Second
	}
	if cfg.StaleThreshold <= 0 {
		cfg.StaleThreshold = 30 * time.Second
	}
	if cfg.AggMethod == "" {
		cfg.AggMethod = AggMedian
	}
	if cfg.MinValidSources <= 0 {
		cfg.MinValidSources = 1
	}
	if cfg.DeviationThreshold.IsZero() {
		cfg.DeviationThreshold = decimal.NewFromFloat(0.1) // 10%
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &IndexPriceManager{
		config:  cfg,
		sources: make([]PriceSource, 0),
		prices:  make(map[string]*IndexPrice),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AddSource 添加数据源
func (m *IndexPriceManager) AddSource(source PriceSource) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sources = append(m.sources, source)
	// 按优先级排序
	sort.Slice(m.sources, func(i, j int) bool {
		return m.sources[i].Priority() < m.sources[j].Priority()
	})

	zap.L().Info("price source added",
		zap.String("name", source.Name()),
		zap.Int("priority", source.Priority()),
		zap.Int("total_sources", len(m.sources)))
}

// RemoveSource 移除数据源
func (m *IndexPriceManager) RemoveSource(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, s := range m.sources {
		if s.Name() == name {
			m.sources = append(m.sources[:i], m.sources[i+1:]...)
			zap.L().Info("price source removed", zap.String("name", name))
			return
		}
	}
}

// SetCallback 设置价格更新回调
func (m *IndexPriceManager) SetCallback(cb UpdateCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callback = cb
}

// Start 启动价格管理器
func (m *IndexPriceManager) Start(symbols []string) error {
	if len(m.sources) == 0 {
		return errors.New("no price source configured")
	}

	// 初始化价格
	for _, symbol := range symbols {
		m.prices[symbol] = &IndexPrice{
			Symbol:    symbol,
			AggMethod: string(m.config.AggMethod),
			Sources:   make([]PriceData, 0),
		}
	}

	// 启动更新协程
	m.wg.Add(1)
	go m.updateLoop(symbols)

	zap.L().Info("index price manager started",
		zap.Int("symbols", len(symbols)),
		zap.Int("sources", len(m.sources)),
		zap.Duration("interval", m.config.UpdateInterval))

	return nil
}

// Stop 停止价格管理器
func (m *IndexPriceManager) Stop() {
	m.cancel()
	m.wg.Wait()
	zap.L().Info("index price manager stopped")
}

// updateLoop 价格更新循环
func (m *IndexPriceManager) updateLoop(symbols []string) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.UpdateInterval)
	defer ticker.Stop()

	// 首次立即更新
	m.updateAllPrices(symbols)

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateAllPrices(symbols)
		}
	}
}

// updateAllPrices 更新所有价格
func (m *IndexPriceManager) updateAllPrices(symbols []string) {
	for _, symbol := range symbols {
		if err := m.updatePrice(symbol); err != nil {
			m.mu.Lock()
			m.errorCount++
			m.lastError = err
			m.lastErrorAt = time.Now()
			m.mu.Unlock()

			zap.L().Debug("update price failed",
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}
}

// updatePrice 更新单个交易对价格
func (m *IndexPriceManager) updatePrice(symbol string) error {
	m.mu.RLock()
	sources := make([]PriceSource, len(m.sources))
	copy(sources, m.sources)
	m.mu.RUnlock()

	// 从所有数据源获取价格
	priceDataList := make([]PriceData, 0, len(sources))
	for _, source := range sources {
		if !source.IsHealthy() {
			continue
		}

		startTime := time.Now()
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		price, err := source.FetchPrice(ctx, symbol)
		cancel()

		latency := time.Since(startTime).Milliseconds()

		if err != nil {
			zap.L().Debug("fetch price from source failed",
				zap.String("source", source.Name()),
				zap.String("symbol", symbol),
				zap.Error(err))
			continue
		}

		priceDataList = append(priceDataList, PriceData{
			Price:     price,
			Source:    source.Name(),
			UpdatedAt: time.Now(),
			LatencyMs: latency,
		})
	}

	if len(priceDataList) < m.config.MinValidSources {
		return fmt.Errorf("%w: got %d sources, need %d",
			ErrNoValidPrice, len(priceDataList), m.config.MinValidSources)
	}

	// 过滤异常价格
	validPrices := m.filterOutliers(priceDataList)
	if len(validPrices) == 0 {
		return ErrNoValidPrice
	}

	// 聚合价格
	indexPrice := m.aggregatePrices(symbol, validPrices)

	// 更新缓存
	m.mu.Lock()
	oldPrice := m.prices[symbol]
	m.prices[symbol] = indexPrice
	m.updateCount++
	callback := m.callback
	m.mu.Unlock()

	// 触发回调
	if callback != nil && (oldPrice == nil || !oldPrice.Price.Equal(indexPrice.Price)) {
		callback(symbol, indexPrice)
	}

	return nil
}

// filterOutliers 过滤异常价格
func (m *IndexPriceManager) filterOutliers(prices []PriceData) []PriceData {
	if len(prices) <= 2 {
		return prices
	}

	// 计算中位数
	sortedPrices := make([]decimal.Decimal, len(prices))
	for i, p := range prices {
		sortedPrices[i] = p.Price
	}
	sort.Slice(sortedPrices, func(i, j int) bool {
		return sortedPrices[i].LessThan(sortedPrices[j])
	})
	median := sortedPrices[len(sortedPrices)/2]

	// 过滤偏离中位数过大的价格
	result := make([]PriceData, 0, len(prices))
	for _, p := range prices {
		deviation := p.Price.Sub(median).Abs().Div(median)
		if deviation.LessThanOrEqual(m.config.DeviationThreshold) {
			result = append(result, p)
		} else {
			zap.L().Warn("price outlier filtered",
				zap.String("source", p.Source),
				zap.String("price", p.Price.String()),
				zap.String("median", median.String()),
				zap.String("deviation", deviation.String()))
		}
	}

	return result
}

// aggregatePrices 聚合价格
func (m *IndexPriceManager) aggregatePrices(symbol string, prices []PriceData) *IndexPrice {
	var aggPrice decimal.Decimal
	confidence := 1.0

	switch m.config.AggMethod {
	case AggMedian:
		aggPrice = m.calculateMedian(prices)
	case AggWeightedAvg:
		aggPrice = m.calculateWeightedAverage(prices)
	case AggBestSource:
		aggPrice = prices[0].Price // 按优先级排序，取第一个
	default:
		aggPrice = m.calculateMedian(prices)
	}

	// 计算置信度 (基于数据源数量和一致性)
	if len(prices) >= 3 {
		confidence = float64(len(prices)) / float64(len(m.sources))
		// 检查价格一致性
		maxDev := decimal.Zero
		for _, p := range prices {
			dev := p.Price.Sub(aggPrice).Abs().Div(aggPrice)
			if dev.GreaterThan(maxDev) {
				maxDev = dev
			}
		}
		devFloat, _ := maxDev.Float64()
		confidence *= (1.0 - devFloat)
	} else {
		confidence = 0.5 // 数据源少，置信度低
	}

	return &IndexPrice{
		Symbol:     symbol,
		Price:      aggPrice,
		Sources:    prices,
		AggMethod:  string(m.config.AggMethod),
		UpdatedAt:  time.Now(),
		Confidence: confidence,
	}
}

// calculateMedian 计算中位数
func (m *IndexPriceManager) calculateMedian(prices []PriceData) decimal.Decimal {
	sortedPrices := make([]decimal.Decimal, len(prices))
	for i, p := range prices {
		sortedPrices[i] = p.Price
	}
	sort.Slice(sortedPrices, func(i, j int) bool {
		return sortedPrices[i].LessThan(sortedPrices[j])
	})

	n := len(sortedPrices)
	if n%2 == 0 {
		return sortedPrices[n/2-1].Add(sortedPrices[n/2]).Div(decimal.NewFromInt(2))
	}
	return sortedPrices[n/2]
}

// calculateWeightedAverage 计算加权平均
func (m *IndexPriceManager) calculateWeightedAverage(prices []PriceData) decimal.Decimal {
	// 简单实现：权重基于响应延迟的倒数
	totalWeight := decimal.Zero
	weightedSum := decimal.Zero

	for _, p := range prices {
		// 延迟越低，权重越高
		weight := decimal.NewFromInt(1000).Div(decimal.NewFromInt(p.LatencyMs + 1))
		totalWeight = totalWeight.Add(weight)
		weightedSum = weightedSum.Add(p.Price.Mul(weight))
	}

	if totalWeight.IsZero() {
		return m.calculateMedian(prices)
	}

	return weightedSum.Div(totalWeight)
}

// GetPrice 获取指数价格
func (m *IndexPriceManager) GetPrice(symbol string) (*IndexPrice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	price, exists := m.prices[symbol]
	if !exists {
		return nil, fmt.Errorf("symbol %s: %w", symbol, ErrSourceNotFound)
	}

	// 检查是否过期
	if time.Since(price.UpdatedAt) > m.config.StaleThreshold {
		return price, ErrPriceStale
	}

	return price, nil
}

// GetPriceValue 获取价格值
func (m *IndexPriceManager) GetPriceValue(symbol string) (decimal.Decimal, error) {
	price, err := m.GetPrice(symbol)
	if err != nil && !errors.Is(err, ErrPriceStale) {
		return decimal.Zero, err
	}
	if price == nil {
		return decimal.Zero, ErrNoValidPrice
	}
	return price.Price, nil
}

// GetAllPrices 获取所有价格
func (m *IndexPriceManager) GetAllPrices() map[string]*IndexPrice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*IndexPrice, len(m.prices))
	for k, v := range m.prices {
		result[k] = v
	}
	return result
}

// GetStats 获取统计信息
func (m *IndexPriceManager) GetStats() IndexPriceManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sourceNames := make([]string, len(m.sources))
	for i, s := range m.sources {
		sourceNames[i] = s.Name()
	}

	return IndexPriceManagerStats{
		SourceCount:   len(m.sources),
		SourceNames:   sourceNames,
		SymbolCount:   len(m.prices),
		UpdateCount:   m.updateCount,
		ErrorCount:    m.errorCount,
		LastError:     m.lastError,
		LastErrorAt:   m.lastErrorAt,
		UpdateInterval: m.config.UpdateInterval,
	}
}

// IndexPriceManagerStats 管理器统计
type IndexPriceManagerStats struct {
	SourceCount    int           `json:"source_count"`
	SourceNames    []string      `json:"source_names"`
	SymbolCount    int           `json:"symbol_count"`
	UpdateCount    int64         `json:"update_count"`
	ErrorCount     int64         `json:"error_count"`
	LastError      error         `json:"last_error,omitempty"`
	LastErrorAt    time.Time     `json:"last_error_at,omitempty"`
	UpdateInterval time.Duration `json:"update_interval"`
}

// =============================================================================
// 内置价格数据源
// =============================================================================

// StaticPriceSource 静态价格数据源 (测试用)
type StaticPriceSource struct {
	name     string
	priority int
	prices   map[string]decimal.Decimal
	mu       sync.RWMutex
}

// NewStaticPriceSource 创建静态价格数据源
func NewStaticPriceSource(name string, priority int) *StaticPriceSource {
	return &StaticPriceSource{
		name:     name,
		priority: priority,
		prices:   make(map[string]decimal.Decimal),
	}
}

func (s *StaticPriceSource) Name() string     { return s.name }
func (s *StaticPriceSource) Priority() int    { return s.priority }
func (s *StaticPriceSource) IsHealthy() bool  { return true }

func (s *StaticPriceSource) FetchPrice(ctx context.Context, symbol string) (decimal.Decimal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	price, exists := s.prices[symbol]
	if !exists {
		return decimal.Zero, fmt.Errorf("symbol %s not found", symbol)
	}
	return price, nil
}

// SetPrice 设置价格
func (s *StaticPriceSource) SetPrice(symbol string, price decimal.Decimal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices[symbol] = price
}

// SetPrices 批量设置价格
func (s *StaticPriceSource) SetPrices(prices map[string]decimal.Decimal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range prices {
		s.prices[k] = v
	}
}

// HTTPPriceSource HTTP 价格数据源
type HTTPPriceSource struct {
	name       string
	priority   int
	baseURL    string
	apiKey     string
	healthy    bool
	lastCheck  time.Time
	mu         sync.RWMutex
}

// NewHTTPPriceSource 创建 HTTP 价格数据源
func NewHTTPPriceSource(name string, priority int, baseURL string, apiKey string) *HTTPPriceSource {
	return &HTTPPriceSource{
		name:     name,
		priority: priority,
		baseURL:  baseURL,
		apiKey:   apiKey,
		healthy:  true,
	}
}

func (s *HTTPPriceSource) Name() string  { return s.name }
func (s *HTTPPriceSource) Priority() int { return s.priority }

func (s *HTTPPriceSource) IsHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.healthy
}

func (s *HTTPPriceSource) FetchPrice(ctx context.Context, symbol string) (decimal.Decimal, error) {
	// 实际实现需要根据具体的 API 进行 HTTP 请求
	// 这里是框架代码，具体实现需要根据接入的价格源进行定制
	return decimal.Zero, fmt.Errorf("not implemented: fetch price from %s", s.baseURL)
}

// SetHealthy 设置健康状态
func (s *HTTPPriceSource) SetHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = healthy
	s.lastCheck = time.Now()
}
