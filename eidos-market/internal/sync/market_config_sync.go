// Package sync provides market configuration synchronization services.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	"github.com/eidos-exchange/eidos/eidos-market/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/repository"
)

// MarketConfigSyncConfig synchronization configuration
type MarketConfigSyncConfig struct {
	// Database sync interval
	DBSyncInterval time.Duration
	// Nacos config data ID for market configuration
	NacosDataID string
	// Nacos group
	NacosGroup string
	// Enable Nacos config watching
	EnableNacosWatch bool
}

// DefaultMarketConfigSyncConfig returns default configuration
func DefaultMarketConfigSyncConfig() MarketConfigSyncConfig {
	return MarketConfigSyncConfig{
		DBSyncInterval:   5 * time.Minute,
		NacosDataID:      "market-config",
		NacosGroup:       "EIDOS_GROUP",
		EnableNacosWatch: true,
	}
}

// MarketChangeListener listener for market configuration changes
type MarketChangeListener interface {
	// OnMarketAdded called when a new market is added
	OnMarketAdded(ctx context.Context, market *model.Market) error
	// OnMarketUpdated called when a market is updated
	OnMarketUpdated(ctx context.Context, market *model.Market) error
	// OnMarketRemoved called when a market is removed
	OnMarketRemoved(ctx context.Context, symbol string) error
}

// MarketConfigSync synchronizes market configuration from Nacos/Database
type MarketConfigSync struct {
	config       MarketConfigSyncConfig
	repo         repository.MarketRepository
	configCenter *nacos.ConfigCenter // for config listening
	logger       *zap.Logger
	listener     MarketChangeListener

	// Current market configuration cache
	markets   map[string]*model.Market
	marketsMu sync.RWMutex

	// Lifecycle management
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewMarketConfigSync creates a new market configuration synchronizer
func NewMarketConfigSync(
	repo repository.MarketRepository,
	configCenter *nacos.ConfigCenter,
	listener MarketChangeListener,
	logger *zap.Logger,
	config MarketConfigSyncConfig,
) *MarketConfigSync {
	ctx, cancel := context.WithCancel(context.Background())

	return &MarketConfigSync{
		config:       config,
		repo:         repo,
		configCenter: configCenter,
		logger:       logger.Named("market_config_sync"),
		listener:     listener,
		markets:      make(map[string]*model.Market),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the synchronization service
func (s *MarketConfigSync) Start() error {
	// Initial load from database
	if err := s.loadFromDB(s.ctx); err != nil {
		return fmt.Errorf("initial load from db: %w", err)
	}

	// Start Nacos watcher if enabled
	if s.config.EnableNacosWatch && s.configCenter != nil {
		if err := s.startNacosWatcher(); err != nil {
			s.logger.Warn("failed to start nacos watcher, will rely on db sync only",
				zap.Error(err))
		}
	}

	// Start periodic database sync
	s.wg.Add(1)
	go s.dbSyncLoop()

	s.logger.Info("market config sync started",
		zap.Duration("db_sync_interval", s.config.DBSyncInterval),
		zap.Bool("nacos_watch", s.config.EnableNacosWatch))

	return nil
}

// Stop stops the synchronization service
func (s *MarketConfigSync) Stop() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
		s.logger.Info("market config sync stopped")
	})
}

// GetMarket returns a market by symbol
func (s *MarketConfigSync) GetMarket(symbol string) (*model.Market, bool) {
	s.marketsMu.RLock()
	defer s.marketsMu.RUnlock()
	m, ok := s.markets[symbol]
	if ok {
		return m.Clone(), true
	}
	return nil, false
}

// GetAllMarkets returns all markets
func (s *MarketConfigSync) GetAllMarkets() []*model.Market {
	s.marketsMu.RLock()
	defer s.marketsMu.RUnlock()

	result := make([]*model.Market, 0, len(s.markets))
	for _, m := range s.markets {
		result = append(result, m.Clone())
	}
	return result
}

// GetActiveMarkets returns all active markets
func (s *MarketConfigSync) GetActiveMarkets() []*model.Market {
	s.marketsMu.RLock()
	defer s.marketsMu.RUnlock()

	result := make([]*model.Market, 0, len(s.markets))
	for _, m := range s.markets {
		if m.IsActive() {
			result = append(result, m.Clone())
		}
	}
	return result
}

// ForceSync forces a sync from database
func (s *MarketConfigSync) ForceSync(ctx context.Context) error {
	return s.loadFromDB(ctx)
}

// loadFromDB loads market configuration from database
func (s *MarketConfigSync) loadFromDB(ctx context.Context) error {
	markets, err := s.repo.ListActiveMarkets(ctx)
	if err != nil {
		return fmt.Errorf("list markets: %w", err)
	}

	s.updateMarkets(ctx, markets)
	s.logger.Info("loaded markets from database", zap.Int("count", len(markets)))

	return nil
}

// updateMarkets updates the market cache and notifies listeners
func (s *MarketConfigSync) updateMarkets(ctx context.Context, newMarkets []*model.Market) {
	s.marketsMu.Lock()
	defer s.marketsMu.Unlock()

	// Build new market map
	newMap := make(map[string]*model.Market, len(newMarkets))
	for _, m := range newMarkets {
		newMap[m.Symbol] = m
	}

	// Find added and updated markets
	for symbol, newMarket := range newMap {
		oldMarket, exists := s.markets[symbol]
		if !exists {
			// New market
			s.logger.Info("market added", zap.String("symbol", symbol))
			metrics.ActiveMarkets.Inc()
			if s.listener != nil {
				go func(m *model.Market) {
					if err := s.listener.OnMarketAdded(ctx, m); err != nil {
						s.logger.Error("failed to notify market added",
							zap.String("symbol", m.Symbol),
							zap.Error(err))
					}
				}(newMarket.Clone())
			}
		} else if s.isMarketChanged(oldMarket, newMarket) {
			// Updated market
			s.logger.Info("market updated", zap.String("symbol", symbol))
			if s.listener != nil {
				go func(m *model.Market) {
					if err := s.listener.OnMarketUpdated(ctx, m); err != nil {
						s.logger.Error("failed to notify market updated",
							zap.String("symbol", m.Symbol),
							zap.Error(err))
					}
				}(newMarket.Clone())
			}
		}
	}

	// Find removed markets
	for symbol := range s.markets {
		if _, exists := newMap[symbol]; !exists {
			s.logger.Info("market removed", zap.String("symbol", symbol))
			metrics.ActiveMarkets.Dec()
			if s.listener != nil {
				go func(sym string) {
					if err := s.listener.OnMarketRemoved(ctx, sym); err != nil {
						s.logger.Error("failed to notify market removed",
							zap.String("symbol", sym),
							zap.Error(err))
					}
				}(symbol)
			}
		}
	}

	// Update cache
	s.markets = newMap
}

// isMarketChanged checks if a market has been updated
func (s *MarketConfigSync) isMarketChanged(old, new *model.Market) bool {
	return old.UpdatedAt != new.UpdatedAt ||
		old.Status != new.Status ||
		old.TradingEnabled != new.TradingEnabled ||
		!old.MakerFee.Equal(new.MakerFee) ||
		!old.TakerFee.Equal(new.TakerFee) ||
		!old.MinSize.Equal(new.MinSize) ||
		!old.MaxSize.Equal(new.MaxSize) ||
		!old.MinNotional.Equal(new.MinNotional) ||
		!old.TickSize.Equal(new.TickSize)
}

// dbSyncLoop periodically syncs from database
func (s *MarketConfigSync) dbSyncLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.DBSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			if err := s.loadFromDB(ctx); err != nil {
				s.logger.Error("failed to sync markets from db", zap.Error(err))
			}
			cancel()
		case <-s.ctx.Done():
			return
		}
	}
}

// startNacosWatcher starts watching Nacos configuration changes
func (s *MarketConfigSync) startNacosWatcher() error {
	if s.configCenter == nil {
		return fmt.Errorf("nacos config center is nil")
	}

	// Listen for config changes
	err := s.configCenter.ListenWithGroup(s.config.NacosDataID, s.config.NacosGroup, func(namespace, group, dataId, data string) {
		s.logger.Info("received nacos config update",
			zap.String("data_id", dataId),
			zap.Int("size", len(data)))
		s.handleNacosConfigChange(data)
	})

	if err != nil {
		return fmt.Errorf("listen nacos config: %w", err)
	}

	s.logger.Info("nacos config watcher started",
		zap.String("data_id", s.config.NacosDataID),
		zap.String("group", s.config.NacosGroup))

	return nil
}

// NacosMarketConfig represents market configuration from Nacos
type NacosMarketConfig struct {
	Markets []NacosMarket `json:"markets"`
	Version string        `json:"version"`
}

// NacosMarket represents a single market in Nacos config
type NacosMarket struct {
	Symbol         string `json:"symbol"`
	BaseToken      string `json:"base_token"`
	QuoteToken     string `json:"quote_token"`
	PriceDecimals  int8   `json:"price_decimals"`
	SizeDecimals   int8   `json:"size_decimals"`
	MinSize        string `json:"min_size"`
	MaxSize        string `json:"max_size"`
	MinNotional    string `json:"min_notional"`
	TickSize       string `json:"tick_size"`
	MakerFee       string `json:"maker_fee"`
	TakerFee       string `json:"taker_fee"`
	Status         int8   `json:"status"`
	TradingEnabled bool   `json:"trading_enabled"`
}

// handleNacosConfigChange handles Nacos configuration changes
func (s *MarketConfigSync) handleNacosConfigChange(data string) {
	var cfg NacosMarketConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		s.logger.Error("failed to parse nacos config", zap.Error(err))
		return
	}

	s.logger.Info("applying nacos config update",
		zap.String("version", cfg.Version),
		zap.Int("markets", len(cfg.Markets)))

	// Convert to model.Market and update database
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for _, nm := range cfg.Markets {
		market, err := s.nacosMarketToModel(&nm)
		if err != nil {
			s.logger.Error("failed to convert nacos market",
				zap.String("symbol", nm.Symbol),
				zap.Error(err))
			continue
		}

		// Check if market exists
		existing, err := s.repo.GetBySymbol(ctx, market.Symbol)
		if err != nil {
			s.logger.Error("failed to check existing market",
				zap.String("symbol", market.Symbol),
				zap.Error(err))
			continue
		}

		if existing == nil {
			// Create new market
			if err := s.repo.Create(ctx, market); err != nil {
				s.logger.Error("failed to create market",
					zap.String("symbol", market.Symbol),
					zap.Error(err))
			} else {
				s.logger.Info("created market from nacos",
					zap.String("symbol", market.Symbol))
			}
		} else {
			// Update existing market
			market.ID = existing.ID
			if err := s.repo.Update(ctx, market); err != nil {
				s.logger.Error("failed to update market",
					zap.String("symbol", market.Symbol),
					zap.Error(err))
			} else {
				s.logger.Info("updated market from nacos",
					zap.String("symbol", market.Symbol))
			}
		}
	}

	// Reload from database to get updated data
	if err := s.loadFromDB(ctx); err != nil {
		s.logger.Error("failed to reload after nacos update", zap.Error(err))
	}
}

// nacosMarketToModel converts Nacos market config to model
func (s *MarketConfigSync) nacosMarketToModel(nm *NacosMarket) (*model.Market, error) {
	market := &model.Market{
		Symbol:         nm.Symbol,
		BaseToken:      nm.BaseToken,
		QuoteToken:     nm.QuoteToken,
		PriceDecimals:  nm.PriceDecimals,
		SizeDecimals:   nm.SizeDecimals,
		Status:         model.MarketStatus(nm.Status),
		TradingEnabled: nm.TradingEnabled,
	}

	// Parse decimal fields
	var err error
	if market.MinSize, err = parseDecimal(nm.MinSize, "0.0001"); err != nil {
		return nil, fmt.Errorf("invalid min_size: %w", err)
	}
	if market.MaxSize, err = parseDecimal(nm.MaxSize, "100000"); err != nil {
		return nil, fmt.Errorf("invalid max_size: %w", err)
	}
	if market.MinNotional, err = parseDecimal(nm.MinNotional, "1"); err != nil {
		return nil, fmt.Errorf("invalid min_notional: %w", err)
	}
	if market.TickSize, err = parseDecimal(nm.TickSize, "0.01"); err != nil {
		return nil, fmt.Errorf("invalid tick_size: %w", err)
	}
	if market.MakerFee, err = parseDecimal(nm.MakerFee, "0.001"); err != nil {
		return nil, fmt.Errorf("invalid maker_fee: %w", err)
	}
	if market.TakerFee, err = parseDecimal(nm.TakerFee, "0.001"); err != nil {
		return nil, fmt.Errorf("invalid taker_fee: %w", err)
	}

	return market, nil
}

// parseDecimal parses a decimal string with a default value
func parseDecimal(s, defaultVal string) (d decimal.Decimal, err error) {
	if s == "" {
		s = defaultVal
	}
	return decimal.NewFromString(s)
}
