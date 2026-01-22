// Package client provides gRPC client implementations for connecting to other Eidos services.
package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"

	commonConfig "github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/discovery"
	commonGrpc "github.com/eidos-exchange/eidos/eidos-common/pkg/grpc"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// ClientManager manages all gRPC client connections
type ClientManager struct {
	cfg   *commonConfig.GRPCClientsConfig
	infra *discovery.Infrastructure // 服务发现基础设施

	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn

	trading  *TradingClient
	matching *MatchingClient
	market   *MarketClient
	chain    *ChainClient
	risk     *RiskClient
	jobs     *JobsClient
}

// NewClientManager creates a new ClientManager
func NewClientManager(cfg *commonConfig.GRPCClientsConfig) *ClientManager {
	return &ClientManager{
		cfg:         cfg,
		connections: make(map[string]*grpc.ClientConn),
	}
}

// NewClientManagerWithDiscovery creates a new ClientManager with service discovery
func NewClientManagerWithDiscovery(cfg *commonConfig.GRPCClientsConfig, infra *discovery.Infrastructure) *ClientManager {
	return &ClientManager{
		cfg:         cfg,
		infra:       infra,
		connections: make(map[string]*grpc.ClientConn),
	}
}

// Connect establishes connections to all services
func (m *ClientManager) Connect(ctx context.Context) error {
	// 如果有服务发现基础设施，使用服务发现模式
	if m.infra != nil {
		return m.connectWithDiscovery(ctx)
	}
	return m.connectDirect(ctx)
}

// connectWithDiscovery 使用服务发现连接
func (m *ClientManager) connectWithDiscovery(ctx context.Context) error {
	mode := m.getServiceDiscoveryMode()

	// Trading
	tradingConn, err := m.infra.GetServiceConnection(ctx, discovery.ServiceTrading)
	if err != nil {
		logger.Warn("failed to connect to trading service via discovery",
			"service", discovery.ServiceTrading,
			"mode", mode,
			"error", err)
	} else {
		m.trading = NewTradingClientFromConn(tradingConn)
		logger.Info("trading client initialized via service discovery",
			"service", discovery.ServiceTrading,
			"mode", mode,
		)
	}

	// Matching
	matchingConn, err := m.infra.GetServiceConnection(ctx, discovery.ServiceMatching)
	if err != nil {
		logger.Warn("failed to connect to matching service via discovery",
			"service", discovery.ServiceMatching,
			"mode", mode,
			"error", err)
	} else {
		m.matching = NewMatchingClientFromConn(matchingConn)
		logger.Info("matching client initialized via service discovery",
			"service", discovery.ServiceMatching,
			"mode", mode,
		)
	}

	// Market
	marketConn, err := m.infra.GetServiceConnection(ctx, discovery.ServiceMarket)
	if err != nil {
		logger.Warn("failed to connect to market service via discovery",
			"service", discovery.ServiceMarket,
			"mode", mode,
			"error", err)
	} else {
		m.market = NewMarketClientFromConn(marketConn)
		logger.Info("market client initialized via service discovery",
			"service", discovery.ServiceMarket,
			"mode", mode,
		)
	}

	// Chain
	chainConn, err := m.infra.GetServiceConnection(ctx, discovery.ServiceChain)
	if err != nil {
		logger.Warn("failed to connect to chain service via discovery",
			"service", discovery.ServiceChain,
			"mode", mode,
			"error", err)
	} else {
		m.chain = NewChainClientFromConn(chainConn)
		logger.Info("chain client initialized via service discovery",
			"service", discovery.ServiceChain,
			"mode", mode,
		)
	}

	// Risk
	riskConn, err := m.infra.GetServiceConnection(ctx, discovery.ServiceRisk)
	if err != nil {
		logger.Warn("failed to connect to risk service via discovery",
			"service", discovery.ServiceRisk,
			"mode", mode,
			"error", err)
	} else {
		m.risk = NewRiskClientFromConn(riskConn)
		logger.Info("risk client initialized via service discovery",
			"service", discovery.ServiceRisk,
			"mode", mode,
		)
	}

	logger.Info("gRPC clients initialized via service discovery", "mode", mode)
	return nil
}

// connectDirect 直接连接（无服务发现）
func (m *ClientManager) connectDirect(ctx context.Context) error {
	var err error

	// Trading
	if m.cfg.Trading.Addr != "" {
		m.trading, err = NewTradingClient(ctx, m.cfg.Trading.Addr)
		if err != nil {
			logger.Warn("failed to connect to trading service", "error", err)
		}
	}

	// Matching
	if m.cfg.Matching.Addr != "" {
		m.matching, err = NewMatchingClient(ctx, m.cfg.Matching.Addr)
		if err != nil {
			logger.Warn("failed to connect to matching service", "error", err)
		}
	}

	// Market
	if m.cfg.Market.Addr != "" {
		m.market, err = NewMarketClient(ctx, m.cfg.Market.Addr)
		if err != nil {
			logger.Warn("failed to connect to market service", "error", err)
		}
	}

	// Chain
	if m.cfg.Chain.Addr != "" {
		m.chain, err = NewChainClient(ctx, m.cfg.Chain.Addr)
		if err != nil {
			logger.Warn("failed to connect to chain service", "error", err)
		}
	}

	// Risk
	if m.cfg.Risk.Addr != "" {
		m.risk, err = NewRiskClient(ctx, m.cfg.Risk.Addr)
		if err != nil {
			logger.Warn("failed to connect to risk service", "error", err)
		}
	}

	return nil
}

// getServiceDiscoveryMode 返回当前服务发现模式
func (m *ClientManager) getServiceDiscoveryMode() string {
	if m.infra != nil && m.infra.IsNacosEnabled() {
		return "nacos"
	}
	return "static"
}

// ConnectJobs connects to jobs service (optional)
func (m *ClientManager) ConnectJobs(ctx context.Context, addr string) error {
	if addr == "" {
		return nil
	}
	var err error
	m.jobs, err = NewJobsClient(ctx, addr)
	return err
}

// Close closes all connections
func (m *ClientManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, conn := range m.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}

	if m.trading != nil {
		m.trading.Close()
	}
	if m.matching != nil {
		m.matching.Close()
	}
	if m.market != nil {
		m.market.Close()
	}
	if m.chain != nil {
		m.chain.Close()
	}
	if m.risk != nil {
		m.risk.Close()
	}
	if m.jobs != nil {
		m.jobs.Close()
	}

	if len(errs) > 0 {
		return fmt.Errorf("close connections: %v", errs)
	}
	return nil
}

// Trading returns the trading client
func (m *ClientManager) Trading() *TradingClient {
	return m.trading
}

// Matching returns the matching client
func (m *ClientManager) Matching() *MatchingClient {
	return m.matching
}

// Market returns the market client
func (m *ClientManager) Market() *MarketClient {
	return m.market
}

// Chain returns the chain client
func (m *ClientManager) Chain() *ChainClient {
	return m.chain
}

// Risk returns the risk client
func (m *ClientManager) Risk() *RiskClient {
	return m.risk
}

// Jobs returns the jobs client
func (m *ClientManager) Jobs() *JobsClient {
	return m.jobs
}

// dial creates a gRPC connection with enterprise-grade default options
// Uses commonGrpc.DialWithTimeout which includes:
// - keepalive (30s/10s)
// - load balancing (round_robin)
// - message size limits (16MB)
// - interceptors (tracing/metrics/logging)
func dial(ctx context.Context, target string) (*grpc.ClientConn, error) {
	// Note: tracing is disabled here as admin service manages its own tracing
	// through the Infrastructure setup
	return commonGrpc.DialWithTimeout(ctx, target, "eidos-admin", 10*time.Second, false)
}
