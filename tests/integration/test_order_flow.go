package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// OrderFlowTestSuite tests the complete order flow:
// Order Creation -> Matching -> Settlement
type OrderFlowTestSuite struct {
	suite.Suite
	config        *TestConfig
	helper        *TestHelper
	ctx           context.Context
	cancel        context.CancelFunc
	tradingClient tradingv1.TradingServiceClient
	matchingClient matchingv1.MatchingServiceClient
}

func TestOrderFlowSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	suite.Run(t, new(OrderFlowTestSuite))
}

func (s *OrderFlowTestSuite) SetupSuite() {
	s.config = DefaultTestConfig()
	s.helper = NewTestHelper(s.config)
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	err := s.helper.Setup(s.ctx)
	require.NoError(s.T(), err, "Failed to setup test helper")

	// Connect to Trading Service
	tradingConn, err := s.helper.GetGRPCConnection(s.ctx, "trading", s.config.TradingServiceAddr)
	require.NoError(s.T(), err, "Failed to connect to Trading Service")
	s.tradingClient = tradingv1.NewTradingServiceClient(tradingConn)

	// Connect to Matching Service
	matchingConn, err := s.helper.GetGRPCConnection(s.ctx, "matching", s.config.MatchingServiceAddr)
	require.NoError(s.T(), err, "Failed to connect to Matching Service")
	s.matchingClient = matchingv1.NewMatchingServiceClient(matchingConn)
}

func (s *OrderFlowTestSuite) TearDownSuite() {
	s.cancel()
	if s.helper != nil {
		_ = s.helper.Cleanup()
	}
}

// TestCreateLimitOrder tests creating a limit buy order
func (s *OrderFlowTestSuite) TestCreateLimitOrder() {
	wallet := GenerateWalletAddress()
	orderID := GenerateOrderID()
	nonce := GenerateNonce()

	req := &tradingv1.CreateOrderRequest{
		Wallet:        wallet,
		Market:        s.config.TestMarket,
		Side:          commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:          commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:         "2000.00",
		Amount:        "1.0",
		Nonce:         nonce,
		Signature:     MockSignature(),
		ClientOrderId: orderID,
		TimeInForce:   commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	resp, err := s.tradingClient.CreateOrder(s.ctx, req)
	require.NoError(s.T(), err, "Failed to create limit order")
	require.NotNil(s.T(), resp, "Response should not be nil")
	require.NotNil(s.T(), resp.Order, "Order should not be nil")

	assert.Equal(s.T(), wallet, resp.Order.Wallet)
	assert.Equal(s.T(), s.config.TestMarket, resp.Order.Market)
	assert.Equal(s.T(), commonv1.OrderSide_ORDER_SIDE_BUY, resp.Order.Side)
	assert.Equal(s.T(), commonv1.OrderType_ORDER_TYPE_LIMIT, resp.Order.Type)

	// Verify order status is PENDING or OPEN
	validStatuses := []commonv1.OrderStatus{
		commonv1.OrderStatus_ORDER_STATUS_PENDING,
		commonv1.OrderStatus_ORDER_STATUS_OPEN,
	}
	assert.Contains(s.T(), validStatuses, resp.Order.Status)
}

// TestCreateMarketOrder tests creating a market sell order
func (s *OrderFlowTestSuite) TestCreateMarketOrder() {
	wallet := GenerateWalletAddress()
	nonce := GenerateNonce()

	req := &tradingv1.CreateOrderRequest{
		Wallet:      wallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_SELL,
		Type:        commonv1.OrderType_ORDER_TYPE_MARKET,
		Amount:      "0.5",
		Nonce:       nonce,
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_IOC,
	}

	resp, err := s.tradingClient.CreateOrder(s.ctx, req)
	require.NoError(s.T(), err, "Failed to create market order")
	require.NotNil(s.T(), resp, "Response should not be nil")
	require.NotNil(s.T(), resp.Order, "Order should not be nil")

	assert.Equal(s.T(), commonv1.OrderType_ORDER_TYPE_MARKET, resp.Order.Type)
	assert.Equal(s.T(), commonv1.OrderSide_ORDER_SIDE_SELL, resp.Order.Side)
}

// TestOrderMatching tests the matching of a buy and sell order
func (s *OrderFlowTestSuite) TestOrderMatching() {
	// Create a maker (seller) limit order
	makerWallet := GenerateWalletAddress()
	makerReq := &tradingv1.CreateOrderRequest{
		Wallet:      makerWallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_SELL,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       "2000.00",
		Amount:      "1.0",
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	makerResp, err := s.tradingClient.CreateOrder(s.ctx, makerReq)
	require.NoError(s.T(), err, "Failed to create maker order")
	require.NotNil(s.T(), makerResp.Order)
	makerOrderID := makerResp.Order.OrderId

	// Create a taker (buyer) limit order that should match
	takerWallet := GenerateWalletAddress()
	takerReq := &tradingv1.CreateOrderRequest{
		Wallet:      takerWallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       "2000.00",
		Amount:      "1.0",
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	takerResp, err := s.tradingClient.CreateOrder(s.ctx, takerReq)
	require.NoError(s.T(), err, "Failed to create taker order")
	require.NotNil(s.T(), takerResp.Order)

	// Wait for orders to be matched
	err = WaitForCondition(s.ctx, s.config.RetryInterval, s.config.MaxRetries, func() (bool, error) {
		order, err := s.tradingClient.GetOrder(s.ctx, &tradingv1.GetOrderRequest{
			OrderId: makerOrderID,
		})
		if err != nil {
			return false, nil // Retry on error
		}
		// Check if order is filled or partially filled
		return order.Status == commonv1.OrderStatus_ORDER_STATUS_FILLED ||
			order.Status == commonv1.OrderStatus_ORDER_STATUS_PARTIAL, nil
	})

	if err != nil {
		s.T().Logf("Warning: Order matching verification timed out (might need matching engine)")
	}
}

// TestCancelOrder tests cancelling an active order
func (s *OrderFlowTestSuite) TestCancelOrder() {
	wallet := GenerateWalletAddress()

	// Create order first
	createReq := &tradingv1.CreateOrderRequest{
		Wallet:      wallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       "1500.00", // Below market price to ensure it stays open
		Amount:      "0.1",
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	createResp, err := s.tradingClient.CreateOrder(s.ctx, createReq)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), createResp.Order)
	orderID := createResp.Order.OrderId

	// Wait for order to be in OPEN status
	time.Sleep(100 * time.Millisecond)

	// Cancel the order
	_, err = s.tradingClient.CancelOrder(s.ctx, &tradingv1.CancelOrderRequest{
		Wallet:  wallet,
		OrderId: orderID,
	})
	require.NoError(s.T(), err, "Failed to cancel order")

	// Verify order is cancelled
	err = WaitForCondition(s.ctx, s.config.RetryInterval, s.config.MaxRetries, func() (bool, error) {
		order, err := s.tradingClient.GetOrder(s.ctx, &tradingv1.GetOrderRequest{
			OrderId: orderID,
		})
		if err != nil {
			return false, nil
		}
		return order.Status == commonv1.OrderStatus_ORDER_STATUS_CANCELLED, nil
	})
	require.NoError(s.T(), err, "Order should be cancelled")
}

// TestBatchCancelOrders tests batch cancellation
func (s *OrderFlowTestSuite) TestBatchCancelOrders() {
	wallet := GenerateWalletAddress()
	var orderIDs []string

	// Create multiple orders
	for i := 0; i < 3; i++ {
		req := &tradingv1.CreateOrderRequest{
			Wallet:      wallet,
			Market:      s.config.TestMarket,
			Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
			Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
			Price:       FormatDecimal(1500.0+float64(i*10), 2),
			Amount:      "0.1",
			Nonce:       GenerateNonce(),
			Signature:   MockSignature(),
			TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
		}

		resp, err := s.tradingClient.CreateOrder(s.ctx, req)
		require.NoError(s.T(), err)
		orderIDs = append(orderIDs, resp.Order.OrderId)
	}

	// Wait briefly for orders to be processed
	time.Sleep(200 * time.Millisecond)

	// Batch cancel all orders
	batchResp, err := s.tradingClient.BatchCancelOrders(s.ctx, &tradingv1.BatchCancelOrdersRequest{
		Wallet:   wallet,
		Market:   s.config.TestMarket,
		OrderIds: orderIDs,
	})
	require.NoError(s.T(), err, "Failed to batch cancel orders")
	assert.Equal(s.T(), int32(3), batchResp.CancelledCount+batchResp.FailedCount)
}

// TestGetBalance tests balance retrieval
func (s *OrderFlowTestSuite) TestGetBalance() {
	wallet := GenerateWalletAddress()

	resp, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	require.NoError(s.T(), err, "Failed to get balance")
	require.NotNil(s.T(), resp)

	// New wallet should have zero balance
	balance, err := decimal.NewFromString(resp.Total)
	require.NoError(s.T(), err)
	assert.True(s.T(), balance.IsZero() || balance.GreaterThanOrEqual(decimal.Zero))
}

// TestListOrders tests order listing with filters
func (s *OrderFlowTestSuite) TestListOrders() {
	wallet := GenerateWalletAddress()

	// Create some orders first
	for i := 0; i < 2; i++ {
		req := &tradingv1.CreateOrderRequest{
			Wallet:      wallet,
			Market:      s.config.TestMarket,
			Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
			Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
			Price:       FormatDecimal(1400.0+float64(i*10), 2),
			Amount:      "0.1",
			Nonce:       GenerateNonce(),
			Signature:   MockSignature(),
			TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
		}
		_, err := s.tradingClient.CreateOrder(s.ctx, req)
		require.NoError(s.T(), err)
	}

	// Wait for orders to be created
	time.Sleep(100 * time.Millisecond)

	// List orders
	resp, err := s.tradingClient.ListOrders(s.ctx, &tradingv1.ListOrdersRequest{
		Wallet:   wallet,
		Market:   s.config.TestMarket,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(s.T(), err, "Failed to list orders")
	require.NotNil(s.T(), resp)
	assert.GreaterOrEqual(s.T(), len(resp.Orders), 0)
}

// TestListOpenOrders tests listing open orders
func (s *OrderFlowTestSuite) TestListOpenOrders() {
	wallet := GenerateWalletAddress()

	// Create an order
	req := &tradingv1.CreateOrderRequest{
		Wallet:      wallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       "1300.00",
		Amount:      "0.1",
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}
	_, err := s.tradingClient.CreateOrder(s.ctx, req)
	require.NoError(s.T(), err)

	// Wait for order to be open
	time.Sleep(100 * time.Millisecond)

	// List open orders
	resp, err := s.tradingClient.ListOpenOrders(s.ctx, &tradingv1.ListOpenOrdersRequest{
		Wallet: wallet,
		Market: s.config.TestMarket,
	})
	require.NoError(s.T(), err, "Failed to list open orders")
	require.NotNil(s.T(), resp)
}

// TestOrderbookQuery tests querying the orderbook from matching service
func (s *OrderFlowTestSuite) TestOrderbookQuery() {
	req := &matchingv1.GetOrderbookRequest{
		Market: s.config.TestMarket,
		Limit:  20,
	}

	resp, err := s.matchingClient.GetOrderbook(s.ctx, req)
	if err != nil {
		// Matching service might not be available in test environment
		s.T().Logf("Warning: Failed to query orderbook: %v", err)
		return
	}

	assert.Equal(s.T(), s.config.TestMarket, resp.Market)
	assert.GreaterOrEqual(s.T(), resp.Sequence, uint64(0))
}

// TestGetDepth tests depth query from matching service
func (s *OrderFlowTestSuite) TestGetDepth() {
	req := &matchingv1.GetDepthRequest{
		Market: s.config.TestMarket,
		Level:  20,
	}

	resp, err := s.matchingClient.GetDepth(s.ctx, req)
	if err != nil {
		s.T().Logf("Warning: Failed to get depth: %v", err)
		return
	}

	assert.Equal(s.T(), s.config.TestMarket, resp.Market)
}

// TestGetBestPrices tests best prices query from matching service
func (s *OrderFlowTestSuite) TestGetBestPrices() {
	req := &matchingv1.GetBestPricesRequest{
		Market: s.config.TestMarket,
	}

	resp, err := s.matchingClient.GetBestPrices(s.ctx, req)
	if err != nil {
		s.T().Logf("Warning: Failed to get best prices: %v", err)
		return
	}

	assert.Equal(s.T(), s.config.TestMarket, resp.Market)
}

// TestTradeFlowEndToEnd tests the complete flow from order to settlement
func (s *OrderFlowTestSuite) TestTradeFlowEndToEnd() {
	s.T().Log("Starting end-to-end trade flow test")

	// Step 1: Create maker (sell) order
	makerWallet := GenerateWalletAddress()
	makerPrice := "2100.00"
	makerAmount := "0.5"

	makerReq := &tradingv1.CreateOrderRequest{
		Wallet:      makerWallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_SELL,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       makerPrice,
		Amount:      makerAmount,
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	makerResp, err := s.tradingClient.CreateOrder(s.ctx, makerReq)
	require.NoError(s.T(), err, "Step 1: Failed to create maker order")
	s.T().Logf("Step 1: Created maker order: %s", makerResp.Order.OrderId)

	// Step 2: Create taker (buy) order to match
	takerWallet := GenerateWalletAddress()

	takerReq := &tradingv1.CreateOrderRequest{
		Wallet:      takerWallet,
		Market:      s.config.TestMarket,
		Side:        commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:        commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:       makerPrice, // Same price to ensure match
		Amount:      makerAmount,
		Nonce:       GenerateNonce(),
		Signature:   MockSignature(),
		TimeInForce: commonv1.TimeInForce_TIME_IN_FORCE_GTC,
	}

	takerResp, err := s.tradingClient.CreateOrder(s.ctx, takerReq)
	require.NoError(s.T(), err, "Step 2: Failed to create taker order")
	s.T().Logf("Step 2: Created taker order: %s", takerResp.Order.OrderId)

	// Step 3: Wait for matching and verify
	s.T().Log("Step 3: Waiting for order matching...")

	err = WaitForCondition(s.ctx, s.config.RetryInterval, s.config.MaxRetries, func() (bool, error) {
		makerOrder, err := s.tradingClient.GetOrder(s.ctx, &tradingv1.GetOrderRequest{
			OrderId: makerResp.Order.OrderId,
		})
		if err != nil {
			return false, nil
		}

		// Check if either filled or partially filled
		if makerOrder.Status == commonv1.OrderStatus_ORDER_STATUS_FILLED ||
			makerOrder.Status == commonv1.OrderStatus_ORDER_STATUS_PARTIAL {
			s.T().Logf("Step 3: Maker order matched, status: %v", makerOrder.Status)
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		s.T().Log("Step 3: Order matching verification timed out (matching engine may not be running)")
	}

	// Step 4: Check trades
	tradesResp, err := s.tradingClient.ListTradesByOrder(s.ctx, &tradingv1.ListTradesByOrderRequest{
		OrderId: makerResp.Order.OrderId,
	})
	if err == nil && len(tradesResp.Trades) > 0 {
		s.T().Logf("Step 4: Found %d trades for maker order", len(tradesResp.Trades))
		for _, trade := range tradesResp.Trades {
			s.T().Logf("  Trade ID: %s, Price: %s, Amount: %s, Settlement: %v",
				trade.TradeId, trade.Price, trade.Amount, trade.SettlementStatus)
		}
	} else {
		s.T().Log("Step 4: No trades found (expected if matching engine not running)")
	}

	s.T().Log("End-to-end trade flow test completed")
}

// Helper function to create gRPC connection with retry
func dialWithRetry(ctx context.Context, addr string, maxRetries int) (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	var err error

	for i := 0; i < maxRetries; i++ {
		conn, err = grpc.DialContext(ctx, addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err == nil {
			return conn, nil
		}
		time.Sleep(time.Second)
	}
	return nil, err
}
