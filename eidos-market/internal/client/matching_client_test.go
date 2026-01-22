package client

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	matchingpb "github.com/eidos-exchange/eidos/proto/matching/v1"
)

// mockMatchingServer 模拟 Matching 服务
type mockMatchingServer struct {
	matchingpb.UnimplementedMatchingServiceServer
	getOrderbookFunc func(ctx context.Context, req *matchingpb.GetOrderbookRequest) (*matchingpb.GetOrderbookResponse, error)
	getDepthFunc     func(ctx context.Context, req *matchingpb.GetDepthRequest) (*matchingpb.GetDepthResponse, error)
	healthCheckFunc  func(ctx context.Context, req *matchingpb.HealthCheckRequest) (*matchingpb.HealthCheckResponse, error)
}

func (m *mockMatchingServer) GetOrderbook(ctx context.Context, req *matchingpb.GetOrderbookRequest) (*matchingpb.GetOrderbookResponse, error) {
	if m.getOrderbookFunc != nil {
		return m.getOrderbookFunc(ctx, req)
	}
	return &matchingpb.GetOrderbookResponse{
		Market:    req.Market,
		Sequence:  100,
		Timestamp: time.Now().UnixMilli(),
		Bids: []*matchingpb.PriceLevel{
			{Price: "100.5", Amount: "10.0", OrderCount: 3},
			{Price: "100.0", Amount: "20.0", OrderCount: 5},
		},
		Asks: []*matchingpb.PriceLevel{
			{Price: "101.0", Amount: "15.0", OrderCount: 2},
			{Price: "101.5", Amount: "25.0", OrderCount: 4},
		},
	}, nil
}

func (m *mockMatchingServer) GetDepth(ctx context.Context, req *matchingpb.GetDepthRequest) (*matchingpb.GetDepthResponse, error) {
	if m.getDepthFunc != nil {
		return m.getDepthFunc(ctx, req)
	}
	return &matchingpb.GetDepthResponse{
		Market:    req.Market,
		Timestamp: time.Now().UnixMilli(),
		Bids: []*matchingpb.PriceLevel{
			{Price: "100.5", Amount: "10.0", OrderCount: 3},
		},
		Asks: []*matchingpb.PriceLevel{
			{Price: "101.0", Amount: "15.0", OrderCount: 2},
		},
	}, nil
}

func (m *mockMatchingServer) HealthCheck(ctx context.Context, req *matchingpb.HealthCheckRequest) (*matchingpb.HealthCheckResponse, error) {
	if m.healthCheckFunc != nil {
		return m.healthCheckFunc(ctx, req)
	}
	return &matchingpb.HealthCheckResponse{
		Healthy: true,
		Markets: map[string]*matchingpb.MarketHealth{
			"BTC-USDC": {
				Market:   "BTC-USDC",
				Active:   true,
				BidCount: 100,
				AskCount: 100,
			},
		},
	}, nil
}

func startMockServer(t *testing.T, server *mockMatchingServer) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	matchingpb.RegisterMatchingServiceServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("server stopped: %v", err)
		}
	}()

	return lis.Addr().String(), func() {
		grpcServer.GracefulStop()
	}
}

func TestNewMatchingClient(t *testing.T) {
	mockServer := &mockMatchingServer{}
	addr, cleanup := startMockServer(t, mockServer)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &MatchingClientConfig{
		Addr:           addr,
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 3 * time.Second,
	}

	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)
	require.NotNil(t, client)

	defer client.Close()
}

func TestNewMatchingClient_ConnectionFailed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &MatchingClientConfig{
		Addr:           "127.0.0.1:99999", // 无效端口
		ConnectTimeout: 100 * time.Millisecond,
		RequestTimeout: 100 * time.Millisecond,
	}

	// gRPC 连接是惰性的，创建客户端不会立即失败
	// 只有在实际发起 RPC 请求时才会失败
	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()

	// 实际请求时应该失败
	ctx := context.Background()
	_, err = client.GetSnapshot(ctx, "BTC-USDC")
	assert.Error(t, err)
}

func TestMatchingClient_GetSnapshot(t *testing.T) {
	mockServer := &mockMatchingServer{}
	addr, cleanup := startMockServer(t, mockServer)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := DefaultMatchingClientConfig(addr)

	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	depth, err := client.GetSnapshot(ctx, "BTC-USDC")

	require.NoError(t, err)
	require.NotNil(t, depth)

	assert.Equal(t, "BTC-USDC", depth.Market)
	assert.Equal(t, uint64(100), depth.Sequence)
	assert.Len(t, depth.Bids, 2)
	assert.Len(t, depth.Asks, 2)

	// 验证买单数据
	assert.True(t, depth.Bids[0].Price.Equal(decimal.NewFromFloat(100.5)))
	assert.True(t, depth.Bids[0].Amount.Equal(decimal.NewFromFloat(10.0)))
	assert.True(t, depth.Bids[1].Price.Equal(decimal.NewFromFloat(100.0)))
	assert.True(t, depth.Bids[1].Amount.Equal(decimal.NewFromFloat(20.0)))

	// 验证卖单数据
	assert.True(t, depth.Asks[0].Price.Equal(decimal.NewFromFloat(101.0)))
	assert.True(t, depth.Asks[0].Amount.Equal(decimal.NewFromFloat(15.0)))
	assert.True(t, depth.Asks[1].Price.Equal(decimal.NewFromFloat(101.5)))
	assert.True(t, depth.Asks[1].Amount.Equal(decimal.NewFromFloat(25.0)))
}

func TestMatchingClient_GetDepth(t *testing.T) {
	mockServer := &mockMatchingServer{}
	addr, cleanup := startMockServer(t, mockServer)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := DefaultMatchingClientConfig(addr)

	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.GetDepth(ctx, "BTC-USDC", 10)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "BTC-USDC", resp.Market)
	assert.Len(t, resp.Bids, 1)
	assert.Len(t, resp.Asks, 1)
}

func TestMatchingClient_HealthCheck(t *testing.T) {
	mockServer := &mockMatchingServer{}
	addr, cleanup := startMockServer(t, mockServer)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := DefaultMatchingClientConfig(addr)

	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.HealthCheck(ctx)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Healthy)
	assert.Contains(t, resp.Markets, "BTC-USDC")
}

func TestMatchingClient_Close(t *testing.T) {
	mockServer := &mockMatchingServer{}
	addr, cleanup := startMockServer(t, mockServer)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := DefaultMatchingClientConfig(addr)

	client, err := NewMatchingClient(cfg, logger, false)
	require.NoError(t, err)

	// 关闭连接
	err = client.Close()
	assert.NoError(t, err)

	// 关闭后再次调用应该出错
	ctx := context.Background()
	_, err = client.GetSnapshot(ctx, "BTC-USDC")
	assert.Error(t, err)
}

func TestDefaultMatchingClientConfig(t *testing.T) {
	addr := "localhost:50052"
	cfg := DefaultMatchingClientConfig(addr)

	assert.Equal(t, addr, cfg.Addr)
	assert.Equal(t, 5*time.Second, cfg.ConnectTimeout)
	assert.Equal(t, 3*time.Second, cfg.RequestTimeout)
}
