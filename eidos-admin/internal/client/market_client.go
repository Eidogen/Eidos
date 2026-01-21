package client

import (
	"context"

	"google.golang.org/grpc"

	marketv1 "github.com/eidos-exchange/eidos/proto/market/v1"
)

// MarketClient wraps the market service gRPC client
type MarketClient struct {
	conn   *grpc.ClientConn
	client marketv1.MarketServiceClient
}

// NewMarketClient creates a new market client
func NewMarketClient(ctx context.Context, target string) (*MarketClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &MarketClient{
		conn:   conn,
		client: marketv1.NewMarketServiceClient(conn),
	}, nil
}

// Close closes the connection
func (c *MarketClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ListMarkets retrieves the list of trading pairs
func (c *MarketClient) ListMarkets(ctx context.Context, status marketv1.MarketStatus) (*marketv1.ListMarketsResponse, error) {
	return c.client.ListMarkets(ctx, &marketv1.ListMarketsRequest{
		Status: status,
	})
}

// ListAllMarkets retrieves all trading pairs
func (c *MarketClient) ListAllMarkets(ctx context.Context) (*marketv1.ListMarketsResponse, error) {
	return c.client.ListMarkets(ctx, &marketv1.ListMarketsRequest{})
}

// GetTicker retrieves ticker for a single market
func (c *MarketClient) GetTicker(ctx context.Context, market string) (*marketv1.Ticker, error) {
	resp, err := c.client.GetTicker(ctx, &marketv1.GetTickerRequest{
		Market: market,
	})
	if err != nil {
		return nil, err
	}
	return resp.Ticker, nil
}

// ListTickers retrieves all tickers
func (c *MarketClient) ListTickers(ctx context.Context) (*marketv1.ListTickersResponse, error) {
	return c.client.ListTickers(ctx, &marketv1.ListTickersRequest{})
}

// GetKlines retrieves kline/candlestick data
func (c *MarketClient) GetKlines(ctx context.Context, market, interval string, startTime, endTime int64, limit int32) (*marketv1.GetKlinesResponse, error) {
	return c.client.GetKlines(ctx, &marketv1.GetKlinesRequest{
		Market:    market,
		Interval:  interval,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
	})
}

// GetRecentTrades retrieves recent trades
func (c *MarketClient) GetRecentTrades(ctx context.Context, market string, limit int32) (*marketv1.GetRecentTradesResponse, error) {
	return c.client.GetRecentTrades(ctx, &marketv1.GetRecentTradesRequest{
		Market: market,
		Limit:  limit,
	})
}

// GetDepth retrieves orderbook depth
func (c *MarketClient) GetDepth(ctx context.Context, market string, limit int32) (*marketv1.GetDepthResponse, error) {
	return c.client.GetDepth(ctx, &marketv1.GetDepthRequest{
		Market: market,
		Limit:  limit,
	})
}

// Get24hStats retrieves 24 hour statistics
func (c *MarketClient) Get24hStats(ctx context.Context, market string) (*marketv1.Get24HStatsResponse, error) {
	return c.client.Get24HStats(ctx, &marketv1.Get24HStatsRequest{
		Market: market,
	})
}

// GetTradeHistory retrieves historical trades
func (c *MarketClient) GetTradeHistory(ctx context.Context, market string, startTime, endTime int64, limit int32, fromID string) (*marketv1.GetTradeHistoryResponse, error) {
	return c.client.GetTradeHistory(ctx, &marketv1.GetTradeHistoryRequest{
		Market:    market,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
		FromId:    fromID,
	})
}
