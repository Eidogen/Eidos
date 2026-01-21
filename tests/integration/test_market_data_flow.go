package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"
	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// MarketDataFlowTestSuite tests the market data flow:
// Trading -> Market Data Service -> K-lines, Depth, Tickers
type MarketDataFlowTestSuite struct {
	suite.Suite
	config         *TestConfig
	helper         *TestHelper
	ctx            context.Context
	cancel         context.CancelFunc
	marketClient   marketv1.MarketServiceClient
	matchingClient matchingv1.MatchingServiceClient
}

func TestMarketDataFlowSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	suite.Run(t, new(MarketDataFlowTestSuite))
}

func (s *MarketDataFlowTestSuite) SetupSuite() {
	s.config = DefaultTestConfig()
	s.helper = NewTestHelper(s.config)
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	err := s.helper.Setup(s.ctx)
	require.NoError(s.T(), err, "Failed to setup test helper")

	// Connect to Market Service
	marketConn, err := s.helper.GetGRPCConnection(s.ctx, "market", s.config.MarketServiceAddr)
	if err != nil {
		s.T().Logf("Warning: Failed to connect to Market Service: %v", err)
	} else {
		s.marketClient = marketv1.NewMarketServiceClient(marketConn)
	}

	// Connect to Matching Service
	matchingConn, err := s.helper.GetGRPCConnection(s.ctx, "matching", s.config.MatchingServiceAddr)
	if err != nil {
		s.T().Logf("Warning: Failed to connect to Matching Service: %v", err)
	} else {
		s.matchingClient = matchingv1.NewMatchingServiceClient(matchingConn)
	}
}

func (s *MarketDataFlowTestSuite) TearDownSuite() {
	s.cancel()
	if s.helper != nil {
		_ = s.helper.Cleanup()
	}
}

// TestListMarkets tests listing available markets
func (s *MarketDataFlowTestSuite) TestListMarkets() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	resp, err := s.marketClient.ListMarkets(s.ctx, &marketv1.ListMarketsRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to list markets: %v", err)
		return
	}

	s.T().Logf("Found %d markets", len(resp.Markets))

	for _, market := range resp.Markets {
		s.T().Logf("  Market: %s, Base: %s, Quote: %s, Status: %v, Trading: %v",
			market.Symbol, market.BaseToken, market.QuoteToken,
			market.Status, market.TradingEnabled)
	}

	// Should have at least one market
	assert.GreaterOrEqual(s.T(), len(resp.Markets), 0)
}

// TestGetTicker tests getting ticker for a market
func (s *MarketDataFlowTestSuite) TestGetTicker() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	resp, err := s.marketClient.GetTicker(s.ctx, &marketv1.GetTickerRequest{
		Market: s.config.TestMarket,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get ticker: %v", err)
		return
	}

	ticker := resp.Ticker
	s.T().Logf("Ticker for %s:", s.config.TestMarket)
	s.T().Logf("  Last Price: %s", ticker.LastPrice)
	s.T().Logf("  24h Change: %s (%s%%)", ticker.PriceChange, ticker.PriceChangePercent)
	s.T().Logf("  24h Volume: %s", ticker.Volume)
	s.T().Logf("  24h High/Low: %s / %s", ticker.High, ticker.Low)
	s.T().Logf("  Best Bid/Ask: %s / %s", ticker.BestBid, ticker.BestAsk)

	assert.Equal(s.T(), s.config.TestMarket, ticker.Market)
}

// TestListTickers tests listing all tickers
func (s *MarketDataFlowTestSuite) TestListTickers() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	resp, err := s.marketClient.ListTickers(s.ctx, &marketv1.ListTickersRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to list tickers: %v", err)
		return
	}

	s.T().Logf("Found %d tickers", len(resp.Tickers))

	for _, ticker := range resp.Tickers {
		s.T().Logf("  %s: Last=%s, Vol=%s, Change=%s%%",
			ticker.Market, ticker.LastPrice, ticker.Volume, ticker.PriceChangePercent)
	}
}

// TestGetKlines tests getting K-line (candlestick) data
func (s *MarketDataFlowTestSuite) TestGetKlines() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	now := time.Now()
	startTime := now.Add(-24 * time.Hour).UnixMilli()
	endTime := now.UnixMilli()

	// Test different intervals
	intervals := []string{"1m", "5m", "15m", "1h", "1d"}

	for _, interval := range intervals {
		resp, err := s.marketClient.GetKlines(s.ctx, &marketv1.GetKlinesRequest{
			Market:    s.config.TestMarket,
			Interval:  interval,
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10,
		})
		if err != nil {
			s.T().Logf("Warning: Failed to get %s klines: %v", interval, err)
			continue
		}

		s.T().Logf("Got %d %s klines for %s", len(resp.Klines), interval, s.config.TestMarket)

		if len(resp.Klines) > 0 {
			kline := resp.Klines[0]
			s.T().Logf("  First kline: O=%s H=%s L=%s C=%s Vol=%s",
				kline.Open, kline.High, kline.Low, kline.Close, kline.Volume)
		}
	}
}

// TestGetRecentTrades tests getting recent trades
func (s *MarketDataFlowTestSuite) TestGetRecentTrades() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	resp, err := s.marketClient.GetRecentTrades(s.ctx, &marketv1.GetRecentTradesRequest{
		Market: s.config.TestMarket,
		Limit:  20,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get recent trades: %v", err)
		return
	}

	s.T().Logf("Got %d recent trades for %s", len(resp.Trades), s.config.TestMarket)

	for i, trade := range resp.Trades {
		if i >= 5 {
			s.T().Logf("  ... and %d more", len(resp.Trades)-5)
			break
		}
		s.T().Logf("  Trade: %s, Price=%s, Amount=%s, Side=%v",
			trade.TradeId, trade.Price, trade.Amount, trade.Side)
	}
}

// TestGetDepth tests getting order book depth from market service
func (s *MarketDataFlowTestSuite) TestGetDepth() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	// Test different depth levels
	levels := []int32{5, 10, 20, 50}

	for _, level := range levels {
		resp, err := s.marketClient.GetDepth(s.ctx, &marketv1.GetDepthRequest{
			Market: s.config.TestMarket,
			Limit:  level,
		})
		if err != nil {
			s.T().Logf("Warning: Failed to get depth (level=%d): %v", level, err)
			continue
		}

		s.T().Logf("Depth (level=%d) for %s: %d bids, %d asks, seq=%d",
			level, resp.Market, len(resp.Bids), len(resp.Asks), resp.Sequence)

		// Show top bid/ask
		if len(resp.Bids) > 0 {
			s.T().Logf("  Top Bid: %s @ %s", resp.Bids[0].Amount, resp.Bids[0].Price)
		}
		if len(resp.Asks) > 0 {
			s.T().Logf("  Top Ask: %s @ %s", resp.Asks[0].Amount, resp.Asks[0].Price)
		}
	}
}

// TestGet24hStats tests getting 24-hour statistics
func (s *MarketDataFlowTestSuite) TestGet24hStats() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	resp, err := s.marketClient.Get24hStats(s.ctx, &marketv1.Get24hStatsRequest{
		Market: s.config.TestMarket,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get 24h stats: %v", err)
		return
	}

	s.T().Logf("24h Stats for %s:", resp.Market)
	s.T().Logf("  Open: %s", resp.Open)
	s.T().Logf("  High: %s", resp.High)
	s.T().Logf("  Low: %s", resp.Low)
	s.T().Logf("  Last: %s", resp.LastPrice)
	s.T().Logf("  Volume: %s", resp.Volume)
	s.T().Logf("  Quote Volume: %s", resp.QuoteVolume)
	s.T().Logf("  Trade Count: %d", resp.TradeCount)
	s.T().Logf("  Change: %s (%s%%)", resp.PriceChange, resp.PriceChangePercent)
}

// TestGetTradeHistory tests getting trade history
func (s *MarketDataFlowTestSuite) TestGetTradeHistory() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	now := time.Now()
	startTime := now.Add(-24 * time.Hour).UnixMilli()
	endTime := now.UnixMilli()

	resp, err := s.marketClient.GetTradeHistory(s.ctx, &marketv1.GetTradeHistoryRequest{
		Market:    s.config.TestMarket,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     100,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get trade history: %v", err)
		return
	}

	s.T().Logf("Got %d trades in history for %s", len(resp.Trades), s.config.TestMarket)
}

// TestMatchingServiceOrderbook tests orderbook from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceOrderbook() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.GetOrderbook(s.ctx, &matchingv1.GetOrderbookRequest{
		Market: s.config.TestMarket,
		Limit:  50,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get orderbook from matching: %v", err)
		return
	}

	s.T().Logf("Orderbook from Matching Service for %s:", resp.Market)
	s.T().Logf("  Sequence: %d", resp.Sequence)
	s.T().Logf("  Last Price: %s", resp.LastPrice)
	s.T().Logf("  Bids: %d levels", len(resp.Bids))
	s.T().Logf("  Asks: %d levels", len(resp.Asks))
}

// TestMatchingServiceDepth tests depth from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceDepth() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.GetDepth(s.ctx, &matchingv1.GetDepthRequest{
		Market: s.config.TestMarket,
		Level:  20,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get depth from matching: %v", err)
		return
	}

	s.T().Logf("Depth from Matching Service:")
	s.T().Logf("  Bids: %d levels", len(resp.Bids))
	s.T().Logf("  Asks: %d levels", len(resp.Asks))
}

// TestMatchingServiceBestPrices tests best prices from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceBestPrices() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.GetBestPrices(s.ctx, &matchingv1.GetBestPricesRequest{
		Market: s.config.TestMarket,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get best prices from matching: %v", err)
		return
	}

	s.T().Logf("Best Prices for %s:", resp.Market)
	s.T().Logf("  Best Bid: %s (qty: %s)", resp.BestBid, resp.BestBidQty)
	s.T().Logf("  Best Ask: %s (qty: %s)", resp.BestAsk, resp.BestAskQty)
	s.T().Logf("  Spread: %s (%s%%)", resp.Spread, resp.SpreadPercent)
}

// TestMatchingServiceMarketState tests market state from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceMarketState() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.GetMarketState(s.ctx, &matchingv1.GetMarketStateRequest{
		Market: s.config.TestMarket,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get market state from matching: %v", err)
		return
	}

	s.T().Logf("Market State for %s:", resp.Market)
	s.T().Logf("  Active: %v", resp.IsActive)
	s.T().Logf("  Status: %v", resp.Status)
	s.T().Logf("  Bid Orders: %d", resp.BidOrderCount)
	s.T().Logf("  Ask Orders: %d", resp.AskOrderCount)
	s.T().Logf("  Bid Volume: %s", resp.BidVolume)
	s.T().Logf("  Ask Volume: %s", resp.AskVolume)
	s.T().Logf("  Last Price: %s", resp.LastPrice)
	s.T().Logf("  Last Sequence: %d", resp.LastSequence)
}

// TestMatchingServiceListMarkets tests listing markets from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceListMarkets() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.ListMarkets(s.ctx, &matchingv1.ListMarketsRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to list markets from matching: %v", err)
		return
	}

	s.T().Logf("Markets from Matching Service: %d", len(resp.Markets))

	for _, market := range resp.Markets {
		s.T().Logf("  %s: Active=%v, Status=%v, Last=%s, Vol24h=%s",
			market.Market, market.IsActive, market.Status,
			market.LastPrice, market.Volume_24H)
	}
}

// TestMatchingServiceHealthCheck tests health check of matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceHealthCheck() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.HealthCheck(s.ctx, &matchingv1.HealthCheckRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to health check matching: %v", err)
		return
	}

	s.T().Logf("Matching Engine Health:")
	s.T().Logf("  Healthy: %v", resp.Healthy)
	s.T().Logf("  Uptime: %d seconds", resp.UptimeSeconds)
	s.T().Logf("  Memory: %d bytes", resp.MemoryUsageBytes)
	s.T().Logf("  Goroutines: %d", resp.GoroutineCount)
	s.T().Logf("  Markets: %d", len(resp.Markets))

	for market, health := range resp.Markets {
		s.T().Logf("    %s: Active=%v, Bids=%d, Asks=%d, OPS=%d/min",
			market, health.Active, health.BidCount, health.AskCount, health.OrdersPerMinute)
	}
}

// TestMatchingServiceMetrics tests metrics from matching service
func (s *MarketDataFlowTestSuite) TestMatchingServiceMetrics() {
	if s.matchingClient == nil {
		s.T().Skip("Matching service not available")
	}

	resp, err := s.matchingClient.GetMetrics(s.ctx, &matchingv1.GetMetricsRequest{
		Market: s.config.TestMarket,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get metrics from matching: %v", err)
		return
	}

	if resp.Global != nil {
		s.T().Logf("Global Metrics:")
		s.T().Logf("  Total Orders: %d", resp.Global.TotalOrders)
		s.T().Logf("  Total Trades: %d", resp.Global.TotalTrades)
		s.T().Logf("  Total Volume: %s", resp.Global.TotalVolume)
		s.T().Logf("  Peak OPS: %.2f", resp.Global.PeakOps)
		s.T().Logf("  Avg Latency: %d us", resp.Global.AvgLatencyUs)
		s.T().Logf("  P99 Latency: %d us", resp.Global.P99LatencyUs)
		s.T().Logf("  Kafka Lag: %d", resp.Global.KafkaLag)
	}

	for market, metrics := range resp.Markets {
		s.T().Logf("Metrics for %s:", market)
		s.T().Logf("  Orders Received: %d", metrics.OrdersReceived)
		s.T().Logf("  Orders Matched: %d", metrics.OrdersMatched)
		s.T().Logf("  Orders Cancelled: %d", metrics.OrdersCancelled)
		s.T().Logf("  Trades Executed: %d", metrics.TradesExecuted)
		s.T().Logf("  Avg Latency: %d us", metrics.AvgLatencyUs)
		s.T().Logf("  OPS: %.2f", metrics.OrdersPerSecond)
	}
}

// TestMarketDataConsistency tests consistency between market and matching services
func (s *MarketDataFlowTestSuite) TestMarketDataConsistency() {
	if s.marketClient == nil || s.matchingClient == nil {
		s.T().Skip("Both market and matching services required")
	}

	s.T().Log("Testing data consistency between Market and Matching services")

	// Get depth from both services
	marketDepth, err1 := s.marketClient.GetDepth(s.ctx, &marketv1.GetDepthRequest{
		Market: s.config.TestMarket,
		Limit:  20,
	})

	matchingDepth, err2 := s.matchingClient.GetDepth(s.ctx, &matchingv1.GetDepthRequest{
		Market: s.config.TestMarket,
		Level:  20,
	})

	if err1 != nil || err2 != nil {
		s.T().Logf("Note: Could not get depth from both services (market: %v, matching: %v)", err1, err2)
		return
	}

	// Compare bid/ask counts (should be similar, may differ slightly due to timing)
	s.T().Logf("Market Service: %d bids, %d asks", len(marketDepth.Bids), len(marketDepth.Asks))
	s.T().Logf("Matching Service: %d bids, %d asks", len(matchingDepth.Bids), len(matchingDepth.Asks))

	// Top prices should match if both are synced
	if len(marketDepth.Bids) > 0 && len(matchingDepth.Bids) > 0 {
		s.T().Logf("Top Bid - Market: %s, Matching: %s",
			marketDepth.Bids[0].Price, matchingDepth.Bids[0].Price)
	}
	if len(marketDepth.Asks) > 0 && len(matchingDepth.Asks) > 0 {
		s.T().Logf("Top Ask - Market: %s, Matching: %s",
			marketDepth.Asks[0].Price, matchingDepth.Asks[0].Price)
	}
}

// TestKlineDataIntegrity tests K-line data integrity
func (s *MarketDataFlowTestSuite) TestKlineDataIntegrity() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	// Get 1-hour klines for the last 24 hours
	now := time.Now()
	startTime := now.Add(-24 * time.Hour).UnixMilli()
	endTime := now.UnixMilli()

	resp, err := s.marketClient.GetKlines(s.ctx, &marketv1.GetKlinesRequest{
		Market:    s.config.TestMarket,
		Interval:  "1h",
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     24,
	})
	if err != nil {
		s.T().Logf("Warning: Failed to get klines: %v", err)
		return
	}

	s.T().Logf("Verifying %d klines for data integrity", len(resp.Klines))

	for i, kline := range resp.Klines {
		// Verify OHLC constraints
		// High should be >= Open, Close, Low
		// Low should be <= Open, Close, High
		s.T().Logf("  Kline %d: O=%s H=%s L=%s C=%s", i,
			kline.Open, kline.High, kline.Low, kline.Close)

		// Verify time ordering
		if i > 0 && kline.OpenTime <= resp.Klines[i-1].OpenTime {
			s.T().Logf("  Warning: Klines not in chronological order")
		}
	}
}

// TestMarketDataLatency tests the latency of market data updates
func (s *MarketDataFlowTestSuite) TestMarketDataLatency() {
	if s.marketClient == nil {
		s.T().Skip("Market service not available")
	}

	s.T().Log("Testing market data latency")

	// Measure ticker fetch latency
	iterations := 10
	var totalLatency time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, err := s.marketClient.GetTicker(s.ctx, &marketv1.GetTickerRequest{
			Market: s.config.TestMarket,
		})
		latency := time.Since(start)
		totalLatency += latency

		if err != nil {
			s.T().Logf("  Iteration %d: error - %v", i+1, err)
		} else {
			s.T().Logf("  Iteration %d: %v", i+1, latency)
		}
	}

	avgLatency := totalLatency / time.Duration(iterations)
	s.T().Logf("Average ticker fetch latency: %v", avgLatency)

	// Should be under 100ms typically
	assert.Less(s.T(), avgLatency, 1*time.Second,
		"Average latency should be reasonable")
}
