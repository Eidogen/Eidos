package client

import (
	"context"

	"google.golang.org/grpc"

	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// MatchingClient wraps the matching service gRPC client
type MatchingClient struct {
	conn   *grpc.ClientConn
	client matchingv1.MatchingServiceClient
}

// NewMatchingClient creates a new matching client
func NewMatchingClient(ctx context.Context, target string) (*MatchingClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &MatchingClient{
		conn:   conn,
		client: matchingv1.NewMatchingServiceClient(conn),
	}, nil
}

// Close closes the connection
func (c *MatchingClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetOrderbook retrieves a snapshot of the orderbook
func (c *MatchingClient) GetOrderbook(ctx context.Context, market string, limit int32) (*matchingv1.GetOrderbookResponse, error) {
	return c.client.GetOrderbook(ctx, &matchingv1.GetOrderbookRequest{
		Market: market,
		Limit:  limit,
	})
}

// GetDepth retrieves aggregated depth at specified levels
func (c *MatchingClient) GetDepth(ctx context.Context, market string, level int32) (*matchingv1.GetDepthResponse, error) {
	return c.client.GetDepth(ctx, &matchingv1.GetDepthRequest{
		Market: market,
		Level:  level,
	})
}

// GetBestPrices retrieves best bid/ask for a market
func (c *MatchingClient) GetBestPrices(ctx context.Context, market string) (*matchingv1.GetBestPricesResponse, error) {
	return c.client.GetBestPrices(ctx, &matchingv1.GetBestPricesRequest{
		Market: market,
	})
}

// GetMarketState retrieves the current state of a market
func (c *MatchingClient) GetMarketState(ctx context.Context, market string) (*matchingv1.GetMarketStateResponse, error) {
	return c.client.GetMarketState(ctx, &matchingv1.GetMarketStateRequest{
		Market: market,
	})
}

// ListMarkets retrieves status of all active markets
func (c *MatchingClient) ListMarkets(ctx context.Context) (*matchingv1.ListMarketsResponse, error) {
	return c.client.ListMarkets(ctx, &matchingv1.ListMarketsRequest{})
}

// HealthCheck checks the health of the matching engine
func (c *MatchingClient) HealthCheck(ctx context.Context) (*matchingv1.HealthCheckResponse, error) {
	return c.client.HealthCheck(ctx, &matchingv1.HealthCheckRequest{})
}

// GetMetrics retrieves matching engine performance metrics
func (c *MatchingClient) GetMetrics(ctx context.Context, market string) (*matchingv1.GetMetricsResponse, error) {
	return c.client.GetMetrics(ctx, &matchingv1.GetMetricsRequest{
		Market: market,
	})
}

// GetAllMetrics retrieves metrics for all markets
func (c *MatchingClient) GetAllMetrics(ctx context.Context) (*matchingv1.GetMetricsResponse, error) {
	return c.client.GetMetrics(ctx, &matchingv1.GetMetricsRequest{})
}
