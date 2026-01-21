// Package client provides gRPC client implementations for connecting to other Eidos services.
package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// ClientManager manages all gRPC client connections
type ClientManager struct {
	cfg *config.GRPCClientsConfig

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
func NewClientManager(cfg *config.GRPCClientsConfig) *ClientManager {
	return &ClientManager{
		cfg:         cfg,
		connections: make(map[string]*grpc.ClientConn),
	}
}

// Connect establishes connections to all services
func (m *ClientManager) Connect(ctx context.Context) error {
	var err error

	// Connect to trading service
	if m.cfg.Trading != "" {
		m.trading, err = NewTradingClient(ctx, m.cfg.Trading)
		if err != nil {
			logger.Warn("failed to connect to trading service", "error", err)
		}
	}

	// Connect to matching service
	if m.cfg.Matching != "" {
		m.matching, err = NewMatchingClient(ctx, m.cfg.Matching)
		if err != nil {
			logger.Warn("failed to connect to matching service", "error", err)
		}
	}

	// Connect to market service
	if m.cfg.Market != "" {
		m.market, err = NewMarketClient(ctx, m.cfg.Market)
		if err != nil {
			logger.Warn("failed to connect to market service", "error", err)
		}
	}

	// Connect to chain service
	if m.cfg.Chain != "" {
		m.chain, err = NewChainClient(ctx, m.cfg.Chain)
		if err != nil {
			logger.Warn("failed to connect to chain service", "error", err)
		}
	}

	// Connect to risk service
	if m.cfg.Risk != "" {
		m.risk, err = NewRiskClient(ctx, m.cfg.Risk)
		if err != nil {
			logger.Warn("failed to connect to risk service", "error", err)
		}
	}

	return nil
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

// defaultDialOptions returns default gRPC dial options
func defaultDialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
			grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
		),
	}
}

// dial creates a gRPC connection with default options
func dial(ctx context.Context, target string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return grpc.DialContext(ctx, target, defaultDialOptions()...)
}
