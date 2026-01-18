//go:build integration

package integration

import (
	"net/http"

	"github.com/stretchr/testify/mock"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
)

// TestGetBalance_Success 测试获取单个余额成功
func (s *IntegrationSuite) TestGetBalance_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.BalanceResponse{
		Wallet:         wallet,
		Token:          "USDC",
		TotalAvailable: "10000.00",
		SettledFrozen:  "500.00",
		Total:          "10500.00",
	}

	s.balanceService.On("GetBalance", mock.Anything, wallet, "USDC").Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/balances/USDC", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.balanceService.AssertExpectations(s.T())
}

// TestGetBalance_NotFound 测试获取不存在的余额（返回空余额）
func (s *IntegrationSuite) TestGetBalance_NotFound() {
	wallet := "0x1234567890123456789012345678901234567890"

	// 返回余额为 0 的响应（不是 error）
	expectedResp := &dto.BalanceResponse{
		Wallet:         wallet,
		Token:          "UNKNOWN",
		TotalAvailable: "0",
		Total:          "0",
	}

	s.balanceService.On("GetBalance", mock.Anything, wallet, "UNKNOWN").Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/balances/UNKNOWN", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.balanceService.AssertExpectations(s.T())
}

// TestListBalances_Success 测试列出所有余额
func (s *IntegrationSuite) TestListBalances_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := []*dto.BalanceResponse{
		{Wallet: wallet, Token: "USDC", TotalAvailable: "10000.00", SettledFrozen: "500.00", Total: "10500.00"},
		{Wallet: wallet, Token: "BTC", TotalAvailable: "1.5", SettledFrozen: "0.1", Total: "1.6"},
		{Wallet: wallet, Token: "ETH", TotalAvailable: "10.0", SettledFrozen: "0", Total: "10.0"},
	}

	s.balanceService.On("ListBalances", mock.Anything, wallet).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/balances", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.balanceService.AssertExpectations(s.T())
}

// TestListBalances_Unauthorized 测试未认证获取余额
func (s *IntegrationSuite) TestListBalances_Unauthorized() {
	w := s.Request(http.MethodGet, "/api/v1/balances", nil, nil)
	s.Equal(http.StatusUnauthorized, w.Code)
}

// TestListTransactions_Success 测试列出交易记录
func (s *IntegrationSuite) TestListTransactions_Success() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items: []*dto.TransactionResponse{
			{
				ID:        "tx-1",
				Wallet:    wallet,
				Token:     "USDC",
				Type:      "deposit",
				Amount:    "1000.00",
				Balance:   "1000.00",
				CreatedAt: 1704067200000,
			},
			{
				ID:        "tx-2",
				Wallet:    wallet,
				Token:     "USDC",
				Type:      "trade",
				Amount:    "-100.00",
				Balance:   "900.00",
				CreatedAt: 1704153600000,
			},
		},
		Total:      2,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	}

	s.balanceService.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		return req.Wallet == wallet
	})).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/transactions", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.balanceService.AssertExpectations(s.T())
}

// TestListTransactions_WithFilters 测试带过滤条件的交易记录查询
func (s *IntegrationSuite) TestListTransactions_WithFilters() {
	wallet := "0x1234567890123456789012345678901234567890"

	expectedResp := &dto.PaginatedResponse{
		Items:      []*dto.TransactionResponse{},
		Total:      0,
		Page:       1,
		PageSize:   10,
		TotalPages: 0,
	}

	s.balanceService.On("ListTransactions", mock.Anything, mock.MatchedBy(func(req *dto.ListTransactionsRequest) bool {
		return req.Wallet == wallet && req.Token == "USDC" && req.Type == "deposit"
	})).Return(expectedResp, nil)

	w := s.Request(http.MethodGet, "/api/v1/transactions?token=USDC&type=deposit&page_size=10", nil, s.AuthHeaders(wallet))
	s.Equal(http.StatusOK, w.Code)

	resp := s.ParseResponse(w)
	s.Equal(0, resp.Code)

	s.balanceService.AssertExpectations(s.T())
}
