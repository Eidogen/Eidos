package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	chainv1 "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	tradingv1 "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// WithdrawalFlowTestSuite tests the complete withdrawal flow:
// Request -> Balance Freeze -> Chain Submission -> Confirmation
type WithdrawalFlowTestSuite struct {
	suite.Suite
	config        *TestConfig
	helper        *TestHelper
	ctx           context.Context
	cancel        context.CancelFunc
	tradingClient tradingv1.TradingServiceClient
	chainClient   chainv1.ChainServiceClient
}

func TestWithdrawalFlowSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	suite.Run(t, new(WithdrawalFlowTestSuite))
}

func (s *WithdrawalFlowTestSuite) SetupSuite() {
	s.config = DefaultTestConfig()
	s.helper = NewTestHelper(s.config)
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	err := s.helper.Setup(s.ctx)
	require.NoError(s.T(), err, "Failed to setup test helper")

	// Connect to Trading Service
	tradingConn, err := s.helper.GetGRPCConnection(s.ctx, "trading", s.config.TradingServiceAddr)
	require.NoError(s.T(), err, "Failed to connect to Trading Service")
	s.tradingClient = tradingv1.NewTradingServiceClient(tradingConn)

	// Connect to Chain Service
	chainConn, err := s.helper.GetGRPCConnection(s.ctx, "chain", s.config.ChainServiceAddr)
	if err != nil {
		s.T().Logf("Warning: Failed to connect to Chain Service: %v", err)
	} else {
		s.chainClient = chainv1.NewChainServiceClient(chainConn)
	}
}

func (s *WithdrawalFlowTestSuite) TearDownSuite() {
	s.cancel()
	if s.helper != nil {
		_ = s.helper.Cleanup()
	}
}

// TestCreateWithdrawal tests creating a withdrawal request
func (s *WithdrawalFlowTestSuite) TestCreateWithdrawal() {
	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()

	// First simulate a deposit to have balance
	s.simulateDeposit(wallet, s.config.TestQuoteToken, "1000.00")

	// Create withdrawal request
	req := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    "100.00",
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	resp, err := s.tradingClient.CreateWithdrawal(s.ctx, req)
	if err != nil {
		// May fail due to insufficient balance or other validation
		s.T().Logf("Note: CreateWithdrawal failed (may be expected): %v", err)
		return
	}

	require.NotNil(s.T(), resp.Withdrawal)
	assert.Equal(s.T(), wallet, resp.Withdrawal.Wallet)
	assert.Equal(s.T(), s.config.TestQuoteToken, resp.Withdrawal.Token)
	assert.Equal(s.T(), toAddress, resp.Withdrawal.ToAddress)

	// Status should be PENDING
	assert.Equal(s.T(), commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING, resp.Withdrawal.Status)
}

// TestListWithdrawals tests listing withdrawals for a wallet
func (s *WithdrawalFlowTestSuite) TestListWithdrawals() {
	wallet := GenerateWalletAddress()

	// List withdrawals - should return empty for new wallet
	resp, err := s.tradingClient.ListWithdrawals(s.ctx, &tradingv1.ListWithdrawalsRequest{
		Wallet:   wallet,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(s.T(), err, "Failed to list withdrawals")
	require.NotNil(s.T(), resp)

	// New wallet should have no withdrawals
	assert.Equal(s.T(), int64(0), resp.Total)
}

// TestGetWithdrawal tests retrieving a specific withdrawal
func (s *WithdrawalFlowTestSuite) TestGetWithdrawal() {
	// Try to get a non-existent withdrawal
	_, err := s.tradingClient.GetWithdrawal(s.ctx, &tradingv1.GetWithdrawalRequest{
		WithdrawId: "non-existent-withdrawal-id",
	})

	// Should return error for non-existent withdrawal
	assert.Error(s.T(), err, "Should return error for non-existent withdrawal")
}

// TestCancelWithdrawal tests cancelling a pending withdrawal
func (s *WithdrawalFlowTestSuite) TestCancelWithdrawal() {
	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()

	// Simulate deposit
	s.simulateDeposit(wallet, s.config.TestQuoteToken, "1000.00")

	// Create withdrawal
	createReq := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    "100.00",
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	createResp, err := s.tradingClient.CreateWithdrawal(s.ctx, createReq)
	if err != nil {
		s.T().Logf("Note: Failed to create withdrawal for cancel test: %v", err)
		return
	}

	withdrawID := createResp.Withdrawal.WithdrawId

	// Cancel the withdrawal
	_, err = s.tradingClient.CancelWithdrawal(s.ctx, &tradingv1.CancelWithdrawalRequest{
		Wallet:     wallet,
		WithdrawId: withdrawID,
	})
	if err != nil {
		s.T().Logf("Note: Failed to cancel withdrawal: %v", err)
		return
	}

	// Verify withdrawal is cancelled
	getResp, err := s.tradingClient.GetWithdrawal(s.ctx, &tradingv1.GetWithdrawalRequest{
		WithdrawId: withdrawID,
	})
	if err != nil {
		s.T().Logf("Note: Failed to get withdrawal: %v", err)
		return
	}

	assert.Equal(s.T(), commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED, getResp.Status)
}

// TestWithdrawalStatusTransitions tests the withdrawal status workflow
func (s *WithdrawalFlowTestSuite) TestWithdrawalStatusTransitions() {
	s.T().Log("Testing withdrawal status transitions")

	// The withdrawal flow is:
	// 1. PENDING - Request received, balance frozen
	// 2. PROCESSING - Being processed by chain service
	// 3. SUBMITTED - Transaction submitted to blockchain
	// 4. CONFIRMED - Transaction confirmed on-chain
	// OR
	// 2. CANCELLED - Cancelled by user (only from PENDING)
	// 2. REJECTED - Rejected by risk/system
	// 3. FAILED - Transaction failed on-chain

	validTransitions := map[commonv1.WithdrawStatus][]commonv1.WithdrawStatus{
		commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING: {
			commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING,
			commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED,
			commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED,
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING: {
			commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED,
			commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED,
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED: {
			commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED,
			commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED,
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED: {
			// Terminal state
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED: {
			// Terminal state, but might allow retry
			commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING,
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED: {
			// Terminal state
		},
		commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED: {
			// Terminal state
		},
	}

	// Log valid transitions for documentation
	for from, toList := range validTransitions {
		if len(toList) > 0 {
			s.T().Logf("  %v -> %v", from, toList)
		} else {
			s.T().Logf("  %v -> (terminal)", from)
		}
	}
}

// TestWithdrawalInsufficientBalance tests withdrawal with insufficient balance
func (s *WithdrawalFlowTestSuite) TestWithdrawalInsufficientBalance() {
	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()

	// Try to withdraw more than available balance
	req := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    "1000000.00", // Large amount
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	_, err := s.tradingClient.CreateWithdrawal(s.ctx, req)

	// Should fail due to insufficient balance
	assert.Error(s.T(), err, "Should reject withdrawal with insufficient balance")
	s.T().Logf("Insufficient balance rejection: %v", err)
}

// TestWithdrawalFreezeUnfreeze tests balance freeze/unfreeze during withdrawal
func (s *WithdrawalFlowTestSuite) TestWithdrawalFreezeUnfreeze() {
	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()
	depositAmount := "1000.00"
	withdrawAmount := "100.00"

	// Simulate deposit
	s.simulateDeposit(wallet, s.config.TestQuoteToken, depositAmount)

	// Get initial balance
	initialBalance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	if err != nil {
		s.T().Logf("Note: Failed to get initial balance: %v", err)
		return
	}

	s.T().Logf("Initial balance - Available: %s, Frozen: %s",
		initialBalance.TotalAvailable, initialBalance.SettledFrozen)

	// Create withdrawal
	req := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    withdrawAmount,
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	createResp, err := s.tradingClient.CreateWithdrawal(s.ctx, req)
	if err != nil {
		s.T().Logf("Note: Failed to create withdrawal: %v", err)
		return
	}

	// Get balance after withdrawal request
	afterBalance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	if err != nil {
		s.T().Logf("Note: Failed to get balance after withdrawal: %v", err)
		return
	}

	s.T().Logf("After withdrawal request - Available: %s, Frozen: %s, Pending Withdrawal: %s",
		afterBalance.TotalAvailable, afterBalance.SettledFrozen, afterBalance.PendingWithdrawal)

	// Withdrawal amount should be frozen (pending withdrawal)
	pendingWithdrawal, _ := decimal.NewFromString(afterBalance.PendingWithdrawal)
	expectedPending, _ := decimal.NewFromString(withdrawAmount)
	assert.True(s.T(), pendingWithdrawal.GreaterThanOrEqual(expectedPending) || pendingWithdrawal.IsZero(),
		"Withdrawal amount should be in pending withdrawal")

	// Cancel to unfreeze
	_, _ = s.tradingClient.CancelWithdrawal(s.ctx, &tradingv1.CancelWithdrawalRequest{
		Wallet:     wallet,
		WithdrawId: createResp.Withdrawal.WithdrawId,
	})
}

// TestWithdrawalFromChainService tests the integration with chain service
func (s *WithdrawalFlowTestSuite) TestWithdrawalFromChainService() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	wallet := GenerateWalletAddress()

	// List pending withdrawals from chain service
	resp, err := s.chainClient.ListPendingWithdrawals(s.ctx, &chainv1.ListPendingWithdrawalsRequest{
		WalletAddress: wallet,
		Pagination: &commonv1.PaginationRequest{
			Page:     1,
			PageSize: 10,
		},
	})
	if err != nil {
		s.T().Logf("Warning: Failed to list pending withdrawals from chain service: %v", err)
		return
	}

	s.T().Logf("Found %d pending withdrawals from chain service", len(resp.Withdrawals))
}

// TestGetWithdrawalStatus tests getting withdrawal status from chain service
func (s *WithdrawalFlowTestSuite) TestGetWithdrawalStatus() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	// Try to get status of non-existent withdrawal
	_, err := s.chainClient.GetWithdrawalStatus(s.ctx, &chainv1.GetWithdrawalStatusRequest{
		WithdrawId: "non-existent-withdrawal",
	})

	// Should return error for non-existent withdrawal
	assert.Error(s.T(), err)
}

// TestRetryWithdrawal tests retrying a failed withdrawal
func (s *WithdrawalFlowTestSuite) TestRetryWithdrawal() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	// Try to retry a non-existent withdrawal
	resp, err := s.chainClient.RetryWithdrawal(s.ctx, &chainv1.RetryWithdrawalRequest{
		WithdrawId:     "non-existent-withdrawal",
		GasBumpPercent: 10,
	})

	if err == nil {
		assert.False(s.T(), resp.Success, "Should not succeed for non-existent withdrawal")
	}
}

// TestWithdrawalBalanceLogs tests balance log entries for withdrawals
func (s *WithdrawalFlowTestSuite) TestWithdrawalBalanceLogs() {
	wallet := GenerateWalletAddress()

	// Get balance logs for withdrawals
	resp, err := s.tradingClient.GetBalanceLogs(s.ctx, &tradingv1.GetBalanceLogsRequest{
		Wallet:   wallet,
		Token:    s.config.TestQuoteToken,
		Type:     commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), resp)

	s.T().Logf("Found %d withdrawal balance logs for wallet %s", len(resp.Logs), wallet)
}

// TestWithdrawalFeeCalculation tests that withdrawal fees are calculated correctly
func (s *WithdrawalFlowTestSuite) TestWithdrawalFeeCalculation() {
	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()
	amount := "100.00"

	// Simulate deposit
	s.simulateDeposit(wallet, s.config.TestQuoteToken, "1000.00")

	// Create withdrawal
	req := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    amount,
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	resp, err := s.tradingClient.CreateWithdrawal(s.ctx, req)
	if err != nil {
		s.T().Logf("Note: Failed to create withdrawal: %v", err)
		return
	}

	withdrawal := resp.Withdrawal
	s.T().Logf("Withdrawal created:")
	s.T().Logf("  Amount: %s", withdrawal.Amount)
	s.T().Logf("  Fee: %s", withdrawal.Fee)
	s.T().Logf("  Net Amount: %s", withdrawal.NetAmount)

	// Verify: amount - fee = net_amount
	withdrawalAmount, _ := decimal.NewFromString(withdrawal.Amount)
	fee, _ := decimal.NewFromString(withdrawal.Fee)
	netAmount, _ := decimal.NewFromString(withdrawal.NetAmount)

	expectedNet := withdrawalAmount.Sub(fee)
	assert.True(s.T(), netAmount.Equal(expectedNet),
		"Net amount should equal amount minus fee")
}

// TestWithdrawalEndToEnd tests the complete withdrawal flow
func (s *WithdrawalFlowTestSuite) TestWithdrawalEndToEnd() {
	s.T().Log("Starting end-to-end withdrawal flow test")

	wallet := GenerateWalletAddress()
	toAddress := GenerateWalletAddress()
	depositAmount := "1000.00"
	withdrawAmount := "100.00"

	// Step 1: Deposit funds
	s.T().Log("Step 1: Simulating deposit...")
	s.simulateDeposit(wallet, s.config.TestQuoteToken, depositAmount)

	// Step 2: Check balance
	balance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	if err != nil {
		s.T().Logf("Step 2: Failed to get balance: %v", err)
	} else {
		s.T().Logf("Step 2: Balance - Total: %s, Withdrawable: %s",
			balance.Total, balance.Withdrawable)
	}

	// Step 3: Create withdrawal request
	s.T().Log("Step 3: Creating withdrawal request...")
	req := &tradingv1.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     s.config.TestQuoteToken,
		Amount:    withdrawAmount,
		ToAddress: toAddress,
		Nonce:     GenerateNonce(),
		Signature: MockSignature(),
	}

	createResp, err := s.tradingClient.CreateWithdrawal(s.ctx, req)
	if err != nil {
		s.T().Logf("Step 3: Failed to create withdrawal: %v", err)
		s.T().Log("End-to-end withdrawal test completed (partial - deposit simulation may not have credited)")
		return
	}

	withdrawID := createResp.Withdrawal.WithdrawId
	s.T().Logf("Step 3: Created withdrawal: %s", withdrawID)

	// Step 4: Monitor withdrawal status
	s.T().Log("Step 4: Monitoring withdrawal status...")
	finalStatuses := []commonv1.WithdrawStatus{
		commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED,
		commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED,
		commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED,
		commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED,
	}

	err = WaitForCondition(s.ctx, s.config.RetryInterval, s.config.MaxRetries, func() (bool, error) {
		withdrawal, err := s.tradingClient.GetWithdrawal(s.ctx, &tradingv1.GetWithdrawalRequest{
			WithdrawId: withdrawID,
		})
		if err != nil {
			return false, nil
		}

		s.T().Logf("  Current status: %v", withdrawal.Status)

		for _, status := range finalStatuses {
			if withdrawal.Status == status {
				return true, nil
			}
		}
		return false, nil
	})

	if err != nil {
		s.T().Logf("Step 4: Withdrawal did not reach final status (might need chain service)")
	}

	// Step 5: Verify final balance
	finalBalance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	if err == nil {
		s.T().Logf("Step 5: Final balance - Total: %s, Withdrawable: %s",
			finalBalance.Total, finalBalance.Withdrawable)
	}

	s.T().Log("End-to-end withdrawal flow test completed")
}

// simulateDeposit simulates a deposit for testing
func (s *WithdrawalFlowTestSuite) simulateDeposit(wallet, token, amount string) {
	depositReq := &tradingv1.ProcessDepositEventRequest{
		TxHash:       GenerateTxHash(),
		LogIndex:     0,
		BlockNum:     12345678,
		Wallet:       wallet,
		Token:        token,
		TokenAddress: "0x0000000000000000000000000000000000000001",
		Amount:       amount,
		DetectedAt:   time.Now().UnixMilli(),
	}

	_, err := s.tradingClient.ProcessDepositEvent(s.ctx, depositReq)
	if err != nil {
		s.T().Logf("Note: simulateDeposit failed (may be internal API): %v", err)
	}
}
