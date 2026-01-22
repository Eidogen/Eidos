package client

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	pb "github.com/eidos-exchange/eidos/proto/market/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// mockMarketServer 模拟 Market gRPC 服务
type mockMarketServer struct {
	pb.UnimplementedMarketServiceServer
	listMarketsFunc     func(ctx context.Context, req *pb.ListMarketsRequest) (*pb.ListMarketsResponse, error)
	getTickerFunc       func(ctx context.Context, req *pb.GetTickerRequest) (*pb.GetTickerResponse, error)
	listTickersFunc     func(ctx context.Context, req *pb.ListTickersRequest) (*pb.ListTickersResponse, error)
	getDepthFunc        func(ctx context.Context, req *pb.GetDepthRequest) (*pb.GetDepthResponse, error)
	getKlinesFunc       func(ctx context.Context, req *pb.GetKlinesRequest) (*pb.GetKlinesResponse, error)
	getRecentTradesFunc func(ctx context.Context, req *pb.GetRecentTradesRequest) (*pb.GetRecentTradesResponse, error)
}

func (m *mockMarketServer) ListMarkets(ctx context.Context, req *pb.ListMarketsRequest) (*pb.ListMarketsResponse, error) {
	if m.listMarketsFunc != nil {
		return m.listMarketsFunc(ctx, req)
	}
	return &pb.ListMarketsResponse{}, nil
}

func (m *mockMarketServer) GetTicker(ctx context.Context, req *pb.GetTickerRequest) (*pb.GetTickerResponse, error) {
	if m.getTickerFunc != nil {
		return m.getTickerFunc(ctx, req)
	}
	return &pb.GetTickerResponse{}, nil
}

func (m *mockMarketServer) ListTickers(ctx context.Context, req *pb.ListTickersRequest) (*pb.ListTickersResponse, error) {
	if m.listTickersFunc != nil {
		return m.listTickersFunc(ctx, req)
	}
	return &pb.ListTickersResponse{}, nil
}

func (m *mockMarketServer) GetDepth(ctx context.Context, req *pb.GetDepthRequest) (*pb.GetDepthResponse, error) {
	if m.getDepthFunc != nil {
		return m.getDepthFunc(ctx, req)
	}
	return &pb.GetDepthResponse{}, nil
}

func (m *mockMarketServer) GetKlines(ctx context.Context, req *pb.GetKlinesRequest) (*pb.GetKlinesResponse, error) {
	if m.getKlinesFunc != nil {
		return m.getKlinesFunc(ctx, req)
	}
	return &pb.GetKlinesResponse{}, nil
}

func (m *mockMarketServer) GetRecentTrades(ctx context.Context, req *pb.GetRecentTradesRequest) (*pb.GetRecentTradesResponse, error) {
	if m.getRecentTradesFunc != nil {
		return m.getRecentTradesFunc(ctx, req)
	}
	return &pb.GetRecentTradesResponse{}, nil
}

// setupMarketTestServer 创建测试用 gRPC 服务器和客户端
func setupMarketTestServer(t *testing.T, srv *mockMarketServer) (*MarketClient, func()) {
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	pb.RegisterMarketServiceServer(grpcServer, srv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	client := NewMarketClient(conn)

	cleanup := func() {
		client.Close()
		grpcServer.Stop()
		lis.Close()
	}

	return client, cleanup
}

// ========== ListMarkets 测试 ==========

func TestMarketClient_ListMarkets_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		listMarketsFunc: func(ctx context.Context, req *pb.ListMarketsRequest) (*pb.ListMarketsResponse, error) {
			return &pb.ListMarketsResponse{
				Markets: []*pb.Market{
					{
						Symbol:         "BTC-USDC",
						BaseToken:      "BTC",
						QuoteToken:     "USDC",
						PriceDecimals:  2,
						SizeDecimals:   6,
						MinSize:        "0.001",
						MaxSize:        "100",
						MinNotional:    "10",
						TickSize:       "0.01",
						MakerFee:       "0.001",
						TakerFee:       "0.002",
						TradingEnabled: true,
					},
					{
						Symbol:         "ETH-USDC",
						BaseToken:      "ETH",
						QuoteToken:     "USDC",
						PriceDecimals:  2,
						SizeDecimals:   6,
						MinSize:        "0.01",
						MaxSize:        "1000",
						MinNotional:    "10",
						TickSize:       "0.01",
						MakerFee:       "0.001",
						TakerFee:       "0.002",
						TradingEnabled: true,
					},
				},
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	markets, err := client.ListMarkets(context.Background())
	require.NoError(t, err)
	assert.Len(t, markets, 2)

	// 验证第一个市场
	assert.Equal(t, "BTC-USDC", markets[0].Symbol)
	assert.Equal(t, "BTC", markets[0].BaseToken)
	assert.Equal(t, "USDC", markets[0].QuoteToken)
	assert.Equal(t, 2, markets[0].PriceDecimals)
	assert.Equal(t, 6, markets[0].AmountDecimals)
	assert.Equal(t, "0.001", markets[0].MinAmount)
	assert.Equal(t, "100", markets[0].MaxAmount)
	assert.Equal(t, "10", markets[0].MinNotional)
	assert.Equal(t, "0.01", markets[0].TickSize)
	assert.Equal(t, "0.001", markets[0].MakerFeeRate)
	assert.Equal(t, "0.002", markets[0].TakerFeeRate)
	assert.True(t, markets[0].IsActive)
}

func TestMarketClient_ListMarkets_Empty(t *testing.T) {
	mockSrv := &mockMarketServer{
		listMarketsFunc: func(ctx context.Context, req *pb.ListMarketsRequest) (*pb.ListMarketsResponse, error) {
			return &pb.ListMarketsResponse{Markets: []*pb.Market{}}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	markets, err := client.ListMarkets(context.Background())
	require.NoError(t, err)
	assert.Empty(t, markets)
}

// ========== GetTicker 测试 ==========

func TestMarketClient_GetTicker_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		getTickerFunc: func(ctx context.Context, req *pb.GetTickerRequest) (*pb.GetTickerResponse, error) {
			assert.Equal(t, "BTC-USDC", req.Market)
			return &pb.GetTickerResponse{
				Ticker: &pb.Ticker{
					Market:             "BTC-USDC",
					LastPrice:          "42000.50",
					PriceChange:        "1500.00",
					PriceChangePercent: "3.70",
					Open:               "40500.50",
					High:               "43000.00",
					Low:                "40000.00",
					Volume:             "1234.567",
					QuoteVolume:        "51851414.50",
					BestBid:            "42000.00",
					BestBidQty:         "0.5",
					BestAsk:            "42001.00",
					BestAskQty:         "0.3",
					TradeCount:         5678,
					Timestamp:          1704067200000,
				},
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	ticker, err := client.GetTicker(context.Background(), "BTC-USDC")
	require.NoError(t, err)

	assert.Equal(t, "BTC-USDC", ticker.Market)
	assert.Equal(t, "42000.50", ticker.LastPrice)
	assert.Equal(t, "1500.00", ticker.PriceChange)
	assert.Equal(t, "3.70", ticker.PriceChangePercent)
	assert.Equal(t, "40500.50", ticker.Open)
	assert.Equal(t, "43000.00", ticker.High)
	assert.Equal(t, "40000.00", ticker.Low)
	assert.Equal(t, "1234.567", ticker.Volume)
	assert.Equal(t, "51851414.50", ticker.QuoteVolume)
	assert.Equal(t, "42000.00", ticker.BestBid)
	assert.Equal(t, "0.5", ticker.BestBidQty)
	assert.Equal(t, "42001.00", ticker.BestAsk)
	assert.Equal(t, "0.3", ticker.BestAskQty)
	assert.Equal(t, 5678, ticker.TradeCount)
	assert.Equal(t, int64(1704067200000), ticker.Timestamp)
}

func TestMarketClient_GetTicker_NotFound(t *testing.T) {
	mockSrv := &mockMarketServer{
		getTickerFunc: func(ctx context.Context, req *pb.GetTickerRequest) (*pb.GetTickerResponse, error) {
			return nil, status.Error(codes.NotFound, "market not found")
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	ticker, err := client.GetTicker(context.Background(), "INVALID")
	assert.Nil(t, ticker)
	assert.Equal(t, dto.ErrMarketNotFound, err)
}

// ========== ListTickers 测试 ==========

func TestMarketClient_ListTickers_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		listTickersFunc: func(ctx context.Context, req *pb.ListTickersRequest) (*pb.ListTickersResponse, error) {
			return &pb.ListTickersResponse{
				Tickers: []*pb.Ticker{
					{
						Market:    "BTC-USDC",
						LastPrice: "42000.50",
						Timestamp: 1704067200000,
					},
					{
						Market:    "ETH-USDC",
						LastPrice: "2500.00",
						Timestamp: 1704067200000,
					},
				},
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	tickers, err := client.ListTickers(context.Background())
	require.NoError(t, err)
	assert.Len(t, tickers, 2)
	assert.Equal(t, "BTC-USDC", tickers[0].Market)
	assert.Equal(t, "ETH-USDC", tickers[1].Market)
}

// ========== GetDepth 测试 ==========

func TestMarketClient_GetDepth_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		getDepthFunc: func(ctx context.Context, req *pb.GetDepthRequest) (*pb.GetDepthResponse, error) {
			assert.Equal(t, "BTC-USDC", req.Market)
			assert.Equal(t, int32(20), req.Limit)
			return &pb.GetDepthResponse{
				Market: "BTC-USDC",
				Bids: []*pb.PriceLevel{
					{Price: "42000.00", Amount: "0.5"},
					{Price: "41999.00", Amount: "1.2"},
				},
				Asks: []*pb.PriceLevel{
					{Price: "42001.00", Amount: "0.3"},
					{Price: "42002.00", Amount: "0.8"},
				},
				Sequence:  12345,
				Timestamp: 1704067200000,
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	depth, err := client.GetDepth(context.Background(), "BTC-USDC", 20)
	require.NoError(t, err)

	assert.Equal(t, "BTC-USDC", depth.Market)
	assert.Len(t, depth.Bids, 2)
	assert.Len(t, depth.Asks, 2)

	// 验证 bids
	assert.Equal(t, []string{"42000.00", "0.5"}, depth.Bids[0])
	assert.Equal(t, []string{"41999.00", "1.2"}, depth.Bids[1])

	// 验证 asks
	assert.Equal(t, []string{"42001.00", "0.3"}, depth.Asks[0])
	assert.Equal(t, []string{"42002.00", "0.8"}, depth.Asks[1])

	assert.Equal(t, uint64(12345), depth.Sequence)
	assert.Equal(t, int64(1704067200000), depth.Timestamp)
}

// ========== GetKlines 测试 ==========

func TestMarketClient_GetKlines_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		getKlinesFunc: func(ctx context.Context, req *pb.GetKlinesRequest) (*pb.GetKlinesResponse, error) {
			assert.Equal(t, "BTC-USDC", req.Market)
			assert.Equal(t, "1h", req.Interval)
			assert.Equal(t, int64(1704000000000), req.StartTime)
			assert.Equal(t, int64(1704067200000), req.EndTime)
			assert.Equal(t, int32(100), req.Limit)

			return &pb.GetKlinesResponse{
				Klines: []*pb.Kline{
					{
						Market:      "BTC-USDC",
						Interval:    "1h",
						OpenTime:    1704000000000,
						Open:        "40000.00",
						High:        "41000.00",
						Low:         "39500.00",
						Close:       "40500.00",
						Volume:      "100.5",
						QuoteVolume: "4050000.00",
						TradeCount:  500,
						CloseTime:   1704003600000,
					},
					{
						Market:      "BTC-USDC",
						Interval:    "1h",
						OpenTime:    1704003600000,
						Open:        "40500.00",
						High:        "42000.00",
						Low:         "40000.00",
						Close:       "41500.00",
						Volume:      "150.3",
						QuoteVolume: "6237450.00",
						TradeCount:  750,
						CloseTime:   1704007200000,
					},
				},
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	klines, err := client.GetKlines(context.Background(), "BTC-USDC", "1h", 1704000000000, 1704067200000, 100)
	require.NoError(t, err)
	assert.Len(t, klines, 2)

	// 验证第一根 K 线
	assert.Equal(t, "BTC-USDC", klines[0].Market)
	assert.Equal(t, "1h", klines[0].Interval)
	assert.Equal(t, int64(1704000000000), klines[0].OpenTime)
	assert.Equal(t, "40000.00", klines[0].Open)
	assert.Equal(t, "41000.00", klines[0].High)
	assert.Equal(t, "39500.00", klines[0].Low)
	assert.Equal(t, "40500.00", klines[0].Close)
	assert.Equal(t, "100.5", klines[0].Volume)
	assert.Equal(t, "4050000.00", klines[0].QuoteVolume)
	assert.Equal(t, 500, klines[0].TradeCount)
	assert.Equal(t, int64(1704003600000), klines[0].CloseTime)
}

// ========== GetRecentTrades 测试 ==========

func TestMarketClient_GetRecentTrades_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		getRecentTradesFunc: func(ctx context.Context, req *pb.GetRecentTradesRequest) (*pb.GetRecentTradesResponse, error) {
			assert.Equal(t, "BTC-USDC", req.Market)
			assert.Equal(t, int32(50), req.Limit)

			return &pb.GetRecentTradesResponse{
				Trades: []*pb.RecentTrade{
					{
						TradeId:   "trade-001",
						Market:    "BTC-USDC",
						Price:     "42000.00",
						Amount:    "0.5",
						Side:      pb.TradeSide_TRADE_SIDE_BUY,
						Timestamp: 1704067200000,
					},
					{
						TradeId:   "trade-002",
						Market:    "BTC-USDC",
						Price:     "42001.00",
						Amount:    "0.3",
						Side:      pb.TradeSide_TRADE_SIDE_SELL,
						Timestamp: 1704067201000,
					},
				},
			}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	trades, err := client.GetRecentTrades(context.Background(), "BTC-USDC", 50)
	require.NoError(t, err)
	assert.Len(t, trades, 2)

	// 验证第一笔成交
	assert.Equal(t, "trade-001", trades[0].TradeID)
	assert.Equal(t, "BTC-USDC", trades[0].Market)
	assert.Equal(t, "42000.00", trades[0].Price)
	assert.Equal(t, "0.5", trades[0].Amount)
	assert.Equal(t, "buy", trades[0].Side)
	assert.Equal(t, int64(1704067200000), trades[0].Timestamp)

	// 验证第二笔成交
	assert.Equal(t, "trade-002", trades[1].TradeID)
	assert.Equal(t, "sell", trades[1].Side)
}

// ========== Ping 测试 ==========

func TestMarketClient_Ping_Success(t *testing.T) {
	mockSrv := &mockMarketServer{
		listMarketsFunc: func(ctx context.Context, req *pb.ListMarketsRequest) (*pb.ListMarketsResponse, error) {
			return &pb.ListMarketsResponse{}, nil
		},
	}

	client, cleanup := setupMarketTestServer(t, mockSrv)
	defer cleanup()

	err := client.Ping(context.Background())
	assert.NoError(t, err)
}

func TestMarketClient_Ping_Unavailable(t *testing.T) {
	// 创建一个立即关闭的服务器来模拟不可用
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	lis.Close()

	// 创建连接到不存在的服务器
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := NewMarketClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = client.Ping(ctx)
	// 预期会有错误（连接失败或超时）
	assert.Error(t, err)
}

// ========== Error Handling 测试 ==========

func TestMarketClient_ErrorConversion(t *testing.T) {
	tests := []struct {
		name        string
		grpcCode    codes.Code
		expectedErr error
	}{
		{
			name:        "NotFound",
			grpcCode:    codes.NotFound,
			expectedErr: dto.ErrMarketNotFound,
		},
		{
			name:        "InvalidArgument",
			grpcCode:    codes.InvalidArgument,
			expectedErr: dto.ErrInvalidParams,
		},
		{
			name:        "Unavailable",
			grpcCode:    codes.Unavailable,
			expectedErr: dto.ErrServiceUnavailable,
		},
		{
			name:        "Internal",
			grpcCode:    codes.Internal,
			expectedErr: dto.ErrInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSrv := &mockMarketServer{
				getTickerFunc: func(ctx context.Context, req *pb.GetTickerRequest) (*pb.GetTickerResponse, error) {
					return nil, status.Error(tt.grpcCode, "test error")
				},
			}

			client, cleanup := setupMarketTestServer(t, mockSrv)
			defer cleanup()

			_, err := client.GetTicker(context.Background(), "BTC-USDC")

			if tt.grpcCode == codes.InvalidArgument {
				// InvalidArgument 会创建带自定义消息的错误
				bizErr, ok := err.(*dto.BizError)
				require.True(t, ok)
				assert.Equal(t, dto.ErrInvalidParams.Code, bizErr.Code)
			} else {
				assert.Equal(t, tt.expectedErr, err)
			}
		})
	}
}

// ========== NewMarketClientWithTarget 测试 ==========

func TestNewMarketClientWithTarget_Success(t *testing.T) {
	// 创建临时服务器
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	pb.RegisterMarketServiceServer(grpcServer, &mockMarketServer{})

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	// 使用 WithTarget 创建客户端（禁用 tracing 用于测试）
	client, err := NewMarketClientWithTarget(lis.Addr().String(), false)
	require.NoError(t, err)
	defer client.Close()

	assert.NotNil(t, client)
}

// ========== Close 测试 ==========

func TestMarketClient_Close_NilConn(t *testing.T) {
	client := &MarketClient{conn: nil}
	err := client.Close()
	assert.NoError(t, err)
}
