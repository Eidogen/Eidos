// Package config Nacos 配置热加载
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// NacosMarketConfig Nacos 市场配置格式
type NacosMarketConfig struct {
	Markets []MarketConfig `json:"markets" yaml:"markets"`
}

// MarketConfigChangeCallback 市场配置变更回调
type MarketConfigChangeCallback func(added, updated, removed []*EngineMarketConfig)

// NacosConfigLoader Nacos 配置热加载器
type NacosConfigLoader struct {
	client       *nacos.ConfigCenter
	dataID       string
	group        string
	callback     MarketConfigChangeCallback
	currentCfg   map[string]*EngineMarketConfig
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	pollInterval time.Duration

	// 统计
	lastLoadTime  time.Time
	loadCount     int64
	errorCount    int64
}

// NacosLoaderConfig 配置加载器配置
type NacosLoaderConfig struct {
	Client       *nacos.ConfigCenter
	DataID       string // Nacos 配置 ID，如 "eidos-matching-markets"
	Group        string // Nacos 配置组，如 "EIDOS_GROUP"
	Callback     MarketConfigChangeCallback
	PollInterval time.Duration // 轮询间隔，默认 30s
}

// NewNacosConfigLoader 创建 Nacos 配置加载器
func NewNacosConfigLoader(cfg *NacosLoaderConfig) (*NacosConfigLoader, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("nacos client is required")
	}
	if cfg.DataID == "" {
		cfg.DataID = "eidos-matching-markets"
	}
	if cfg.Group == "" {
		cfg.Group = "EIDOS_GROUP"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &NacosConfigLoader{
		client:       cfg.Client,
		dataID:       cfg.DataID,
		group:        cfg.Group,
		callback:     cfg.Callback,
		currentCfg:   make(map[string]*EngineMarketConfig),
		ctx:          ctx,
		cancel:       cancel,
		pollInterval: cfg.PollInterval,
	}, nil
}

// Start 启动配置监听
func (l *NacosConfigLoader) Start() error {
	// 首次加载配置
	if err := l.loadConfig(); err != nil {
		return fmt.Errorf("initial config load: %w", err)
	}

	// 注册配置监听
	err := l.client.ListenWithGroup(l.dataID, l.group, func(namespace, group, dataId, data string) {
		zap.L().Info("nacos config changed",
			zap.String("dataId", dataId),
			zap.String("group", group),
			zap.Int("data_len", len(data)))

		if err := l.handleConfigChange(data); err != nil {
			l.errorCount++
			zap.L().Error("handle config change failed",
				zap.String("dataId", dataId),
				zap.Error(err))
		}
	})
	if err != nil {
		zap.L().Warn("listen config failed, falling back to polling",
			zap.Error(err))
		// 降级到轮询模式
		go l.pollLoop()
	}

	zap.L().Info("nacos config loader started",
		zap.String("dataId", l.dataID),
		zap.String("group", l.group))

	return nil
}

// Stop 停止配置监听
func (l *NacosConfigLoader) Stop() {
	l.cancel()
	zap.L().Info("nacos config loader stopped")
}

// loadConfig 加载配置
func (l *NacosConfigLoader) loadConfig() error {
	content, err := l.client.GetConfigWithGroup(l.dataID, l.group)
	if err != nil {
		return fmt.Errorf("get config from nacos: %w", err)
	}

	return l.handleConfigChange(content)
}

// handleConfigChange 处理配置变更
func (l *NacosConfigLoader) handleConfigChange(data string) error {
	if data == "" {
		return nil
	}

	// 解析新配置
	var nacosCfg NacosMarketConfig
	if err := json.Unmarshal([]byte(data), &nacosCfg); err != nil {
		// 尝试 YAML 格式
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// 转换为引擎配置
	newCfg := make(map[string]*EngineMarketConfig)
	for _, m := range nacosCfg.Markets {
		engineCfg := m.ToEngineMarketConfig()
		newCfg[m.Symbol] = engineCfg
	}

	// 计算差异
	added, updated, removed := l.diffConfigs(newCfg)

	// 更新当前配置
	l.mu.Lock()
	l.currentCfg = newCfg
	l.lastLoadTime = time.Now()
	l.loadCount++
	l.mu.Unlock()

	// 触发回调
	if l.callback != nil && (len(added) > 0 || len(updated) > 0 || len(removed) > 0) {
		zap.L().Info("market config changed",
			zap.Int("added", len(added)),
			zap.Int("updated", len(updated)),
			zap.Int("removed", len(removed)))
		l.callback(added, updated, removed)
	}

	return nil
}

// diffConfigs 计算配置差异
func (l *NacosConfigLoader) diffConfigs(newCfg map[string]*EngineMarketConfig) (added, updated, removed []*EngineMarketConfig) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	added = make([]*EngineMarketConfig, 0)
	updated = make([]*EngineMarketConfig, 0)
	removed = make([]*EngineMarketConfig, 0)

	// 检查新增和更新
	for symbol, cfg := range newCfg {
		oldCfg, exists := l.currentCfg[symbol]
		if !exists {
			added = append(added, cfg)
		} else if !l.configEqual(oldCfg, cfg) {
			updated = append(updated, cfg)
		}
	}

	// 检查删除
	for symbol, cfg := range l.currentCfg {
		if _, exists := newCfg[symbol]; !exists {
			removed = append(removed, cfg)
		}
	}

	return
}

// configEqual 比较两个配置是否相等
func (l *NacosConfigLoader) configEqual(a, b *EngineMarketConfig) bool {
	if a.Symbol != b.Symbol {
		return false
	}
	if a.BaseToken != b.BaseToken || a.QuoteToken != b.QuoteToken {
		return false
	}
	if a.PriceDecimals != b.PriceDecimals || a.SizeDecimals != b.SizeDecimals {
		return false
	}
	if !a.MinSize.Equal(b.MinSize) || !a.TickSize.Equal(b.TickSize) {
		return false
	}
	if !a.MakerFeeRate.Equal(b.MakerFeeRate) || !a.TakerFeeRate.Equal(b.TakerFeeRate) {
		return false
	}
	if !a.MaxSlippage.Equal(b.MaxSlippage) {
		return false
	}
	return true
}

// pollLoop 轮询配置变更
func (l *NacosConfigLoader) pollLoop() {
	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			if err := l.loadConfig(); err != nil {
				l.errorCount++
				zap.L().Warn("poll config failed", zap.Error(err))
			}
		}
	}
}

// GetCurrentConfigs 获取当前配置
func (l *NacosConfigLoader) GetCurrentConfigs() map[string]*EngineMarketConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[string]*EngineMarketConfig, len(l.currentCfg))
	for k, v := range l.currentCfg {
		result[k] = v
	}
	return result
}

// GetStats 获取加载器统计
func (l *NacosConfigLoader) GetStats() NacosLoaderStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return NacosLoaderStats{
		DataID:       l.dataID,
		Group:        l.group,
		LastLoadTime: l.lastLoadTime,
		LoadCount:    l.loadCount,
		ErrorCount:   l.errorCount,
		MarketCount:  len(l.currentCfg),
	}
}

// NacosLoaderStats 加载器统计
type NacosLoaderStats struct {
	DataID       string    `json:"data_id"`
	Group        string    `json:"group"`
	LastLoadTime time.Time `json:"last_load_time"`
	LoadCount    int64     `json:"load_count"`
	ErrorCount   int64     `json:"error_count"`
	MarketCount  int       `json:"market_count"`
}

// ParseMarketConfigFromJSON 从 JSON 解析市场配置
func ParseMarketConfigFromJSON(data string) ([]*EngineMarketConfig, error) {
	var nacosCfg NacosMarketConfig
	if err := json.Unmarshal([]byte(data), &nacosCfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	configs := make([]*EngineMarketConfig, 0, len(nacosCfg.Markets))
	for _, m := range nacosCfg.Markets {
		configs = append(configs, m.ToEngineMarketConfig())
	}
	return configs, nil
}

// MarketConfigToJSON 将市场配置转换为 JSON
func MarketConfigToJSON(configs []*EngineMarketConfig) (string, error) {
	markets := make([]MarketConfig, 0, len(configs))
	for _, cfg := range configs {
		markets = append(markets, MarketConfig{
			Symbol:        cfg.Symbol,
			BaseToken:     cfg.BaseToken,
			QuoteToken:    cfg.QuoteToken,
			PriceDecimals: cfg.PriceDecimals,
			SizeDecimals:  cfg.SizeDecimals,
			MinSize:       cfg.MinSize.String(),
			TickSize:      cfg.TickSize.String(),
			MakerFeeRate:  cfg.MakerFeeRate.String(),
			TakerFeeRate:  cfg.TakerFeeRate.String(),
			MaxSlippage:   cfg.MaxSlippage.String(),
		})
	}

	nacosCfg := NacosMarketConfig{Markets: markets}
	data, err := json.MarshalIndent(nacosCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(data), nil
}

// ValidateMarketConfig 验证市场配置
func ValidateMarketConfig(cfg *EngineMarketConfig) error {
	if cfg.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if cfg.BaseToken == "" {
		return fmt.Errorf("base_token is required")
	}
	if cfg.QuoteToken == "" {
		return fmt.Errorf("quote_token is required")
	}
	if cfg.PriceDecimals < 0 || cfg.PriceDecimals > 18 {
		return fmt.Errorf("price_decimals must be between 0 and 18")
	}
	if cfg.SizeDecimals < 0 || cfg.SizeDecimals > 18 {
		return fmt.Errorf("size_decimals must be between 0 and 18")
	}
	if cfg.MinSize.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("min_size must be positive")
	}
	if cfg.TickSize.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("tick_size must be positive")
	}
	if cfg.MakerFeeRate.LessThan(decimal.Zero) {
		return fmt.Errorf("maker_fee_rate cannot be negative")
	}
	if cfg.TakerFeeRate.LessThan(decimal.Zero) {
		return fmt.Errorf("taker_fee_rate cannot be negative")
	}
	if cfg.MaxSlippage.LessThan(decimal.Zero) || cfg.MaxSlippage.GreaterThan(decimal.NewFromFloat(0.5)) {
		return fmt.Errorf("max_slippage must be between 0 and 0.5")
	}
	return nil
}
