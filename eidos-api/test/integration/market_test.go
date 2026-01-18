//go:build integration

package integration

import (
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// TestListMarkets_Success 测试列出所有交易对
func (s *IntegrationSuite) TestListMarkets_Success() {
	expectedResp := []*dto.MarketResponse{
		{
			Symbol:         "BTC-USDC",
			BaseToken:      "BTC",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 8,
			MinAmount:      "0.0001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
		{
			Symbol:         "ETH-USDC",
			BaseToken:      "ETH",
			QuoteToken:     "USDC",
			PriceDecimals:  2,
			AmountDecimals: 8,
			MinAmount:      "0.001",
			MinNotional:    "10",
			MakerFeeRate:   "0.001",
			TakerFeeRate:   "0.002",
			IsActive:       true,
		},
	}

	s.marketService.On("ListMarkets", mock.Anything).Return(expectedResp, nil)

	// 公开接口，无需认证
	w := s.Request(http.MethodGet, "/api/v1/markets", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetTicker_Success 测试获取单个 Ticker
func (s *IntegrationSuite) TestGetTicker_Success() {
	expectedResp := &dto.TickerResponse{
		Market:             "BTC-USDC",
		LastPrice:          "42000.00",
		High:               "43000.00",
		Low:                "41000.00",
		Volume:             "1000.5",
		PriceChangePercent: "2.5",
		Timestamp:          1704067200000,
	}

	s.marketService.On("GetTicker", mock.Anything, "BTC-USDC").Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/ticker/BTC-USDC", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetTicker_NotFound 测试获取不存在的市场 Ticker
func (s *IntegrationSuite) TestGetTicker_NotFound() {
	s.marketService.On("GetTicker", mock.Anything, "UNKNOWN-PAIR").Return(nil, dto.ErrMarketNotFound)

	w := s.Request(http.MethodGet, "/api/v1/ticker/UNKNOWN-PAIR", nil, nil)
	s.Equal(http.StatusNotFound, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(dto.ErrMarketNotFound.Code, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestListTickers_Success 测试列出所有 Tickers
func (s *IntegrationSuite) TestListTickers_Success() {
	expectedResp := []*dto.TickerResponse{
		{
			Market:    "BTC-USDC",
			LastPrice: "42000.00",
			High:      "43000.00",
			Low:       "41000.00",
		},
		{
			Market:    "ETH-USDC",
			LastPrice: "2200.00",
			High:      "2300.00",
			Low:       "2100.00",
		},
	}

	s.marketService.On("ListTickers", mock.Anything).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/tickers", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetDepth_Success 测试获取深度
func (s *IntegrationSuite) TestGetDepth_Success() {
	expectedResp := &dto.DepthResponse{
		Market:    "BTC-USDC",
		Bids:      [][]string{{"41999.00", "1.5"}, {"41998.00", "2.0"}},
		Asks:      [][]string{{"42001.00", "0.8"}, {"42002.00", "1.2"}},
		Timestamp: 1704067200000,
	}

	s.marketService.On("GetDepth", mock.Anything, "BTC-USDC", 20).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/depth/BTC-USDC", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetDepth_WithLimit 测试获取指定深度
func (s *IntegrationSuite) TestGetDepth_WithLimit() {
	expectedResp := &dto.DepthResponse{
		Market:    "BTC-USDC",
		Bids:      [][]string{{"41999.00", "1.5"}},
		Asks:      [][]string{{"42001.00", "0.8"}},
		Timestamp: 1704067200000,
	}

	s.marketService.On("GetDepth", mock.Anything, "BTC-USDC", 5).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/depth/BTC-USDC?limit=5", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetKlines_Success 测试获取 K 线
func (s *IntegrationSuite) TestGetKlines_Success() {
	expectedResp := []*dto.KlineResponse{
		{
			OpenTime:    1704067200000,
			Open:        "42000.00",
			High:        "42100.00",
			Low:         "41900.00",
			Close:       "42050.00",
			Volume:      "100.5",
			QuoteVolume: "4210000.00",
			CloseTime:   1704070800000,
			TradeCount:  150,
		},
	}

	s.marketService.On("GetKlines", mock.Anything, "BTC-USDC", "1h", int64(0), int64(0), 100).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/klines/BTC-USDC?interval=1h", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}

// TestGetRecentTrades_Success 测试获取最近成交
func (s *IntegrationSuite) TestGetRecentTrades_Success() {
	expectedResp := []*dto.RecentTradeResponse{
		{
			TradeID:     "trade-1",
			Price:       "42000.00",
			Amount:      "0.5",
			QuoteAmount: "21000.00",
			Side:        "buy",
			Timestamp:   1704067200000,
		},
		{
			TradeID:     "trade-2",
			Price:       "42001.00",
			Amount:      "0.3",
			QuoteAmount: "12600.30",
			Side:        "sell",
			Timestamp:   1704067201000,
		},
	}

	s.marketService.On("GetRecentTrades", mock.Anything, "BTC-USDC", 50).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/trades/BTC-USDC/recent", nil, nil)
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.marketService.AssertExpectations(s.T())
}
