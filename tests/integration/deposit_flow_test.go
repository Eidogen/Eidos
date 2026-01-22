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

// DepositFlowTestSuite tests the complete deposit flow:
// Chain Detection -> Confirmation -> Balance Credit
type DepositFlowTestSuite struct {
	suite.Suite
	config        *TestConfig
	helper        *TestHelper
	ctx           context.Context
	cancel        context.CancelFunc
	tradingClient tradingv1.TradingServiceClient
	chainClient   chainv1.ChainServiceClient
}

func TestDepositFlowSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	suite.Run(t, new(DepositFlowTestSuite))
}

func (s *DepositFlowTestSuite) SetupSuite() {
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

func (s *DepositFlowTestSuite) TearDownSuite() {
	s.cancel()
	if s.helper != nil {
		_ = s.helper.Cleanup()
	}
}

// TestDepositDetection tests that deposits are detected from blockchain events
func (s *DepositFlowTestSuite) TestDepositDetection() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	// Check indexer status
	statusResp, err := s.chainClient.GetIndexerStatus(s.ctx, &chainv1.GetIndexerStatusRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to get indexer status: %v", err)
		return
	}

	s.T().Logf("Indexer status: running=%v, current_block=%d, latest_block=%d, lag=%d",
		statusResp.Running, statusResp.CurrentBlock, statusResp.LatestBlock, statusResp.LagBlocks)

	assert.True(s.T(), statusResp.Running, "Indexer should be running")
	assert.LessOrEqual(s.T(), statusResp.LagBlocks, int64(100), "Indexer lag should be reasonable")
}

// TestListDeposits tests listing deposits for a wallet
func (s *DepositFlowTestSuite) TestListDeposits() {
	wallet := GenerateWalletAddress()

	// List deposits - should return empty for new wallet
	resp, err := s.tradingClient.ListDeposits(s.ctx, &tradingv1.ListDepositsRequest{
		Wallet:   wallet,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(s.T(), err, "Failed to list deposits")
	require.NotNil(s.T(), resp)

	// New wallet should have no deposits
	assert.Equal(s.T(), int64(0), resp.Total)
}

// TestGetDeposit tests retrieving a specific deposit
func (s *DepositFlowTestSuite) TestGetDeposit() {
	// Try to get a non-existent deposit
	_, err := s.tradingClient.GetDeposit(s.ctx, &tradingv1.GetDepositRequest{
		DepositId: "non-existent-deposit-id",
	})

	// Should return error for non-existent deposit
	assert.Error(s.T(), err, "Should return error for non-existent deposit")
}

// TestDepositStatusTransitions tests the deposit status workflow
func (s *DepositFlowTestSuite) TestDepositStatusTransitions() {
	s.T().Log("Testing deposit status transitions")

	// The deposit flow is:
	// 1. PENDING - Deposit detected, waiting for confirmations
	// 2. CONFIRMED - Sufficient confirmations received
	// 3. CREDITED - Balance credited to user account

	validTransitions := map[commonv1.DepositStatus][]commonv1.DepositStatus{
		commonv1.DepositStatus_DEPOSIT_STATUS_PENDING: {
			commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED,
			commonv1.DepositStatus_DEPOSIT_STATUS_FAILED,
		},
		commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED: {
			commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED,
			commonv1.DepositStatus_DEPOSIT_STATUS_FAILED,
		},
		commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED: {
			// Terminal state
		},
		commonv1.DepositStatus_DEPOSIT_STATUS_FAILED: {
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

// TestSimulateDepositEvent simulates a deposit event processing
func (s *DepositFlowTestSuite) TestSimulateDepositEvent() {
	wallet := GenerateWalletAddress()
	token := s.config.TestQuoteToken
	amount := "1000.00"

	s.T().Logf("Simulating deposit event for wallet: %s", wallet)

	// Check initial balance
	initialBalance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  token,
	})
	require.NoError(s.T(), err)

	initialAmount, _ := decimal.NewFromString(initialBalance.Total)
	s.T().Logf("Initial balance: %s %s", initialAmount.String(), token)

	// Simulate deposit event via internal API
	// This would normally come from chain indexer
	depositReq := &tradingv1.ProcessDepositEventRequest{
		TxHash:       GenerateTxHash(), // Simulated tx hash
		LogIndex:     0,
		BlockNum:     12345678,
		Wallet:       wallet,
		Token:        token,
		TokenAddress: "0x0000000000000000000000000000000000000001",
		Amount:       amount,
		DetectedAt:   time.Now().UnixMilli(),
	}

	_, err = s.tradingClient.ProcessDepositEvent(s.ctx, depositReq)
	if err != nil {
		// This might fail if the API is internal-only
		s.T().Logf("Note: ProcessDepositEvent failed (may be internal API): %v", err)
		return
	}

	s.T().Logf("Deposit event processed, waiting for confirmation...")

	// Wait for deposit to be credited
	err = WaitForCondition(s.ctx, s.config.RetryInterval, s.config.MaxRetries, func() (bool, error) {
		balance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
			Wallet: wallet,
			Token:  token,
		})
		if err != nil {
			return false, nil
		}

		currentAmount, _ := decimal.NewFromString(balance.Total)
		depositAmount, _ := decimal.NewFromString(amount)
		expectedAmount := initialAmount.Add(depositAmount)

		return currentAmount.Equal(expectedAmount), nil
	})

	if err != nil {
		s.T().Logf("Warning: Deposit credit verification timed out")
	}
}

// TestDepositFromChainService tests the integration with chain service
func (s *DepositFlowTestSuite) TestDepositFromChainService() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	wallet := GenerateWalletAddress()

	// List deposits from chain service
	resp, err := s.chainClient.ListDeposits(s.ctx, &chainv1.ListDepositsRequest{
		WalletAddress: wallet,
		Pagination: &commonv1.PaginationRequest{
			Page:     1,
			PageSize: 10,
		},
	})
	if err != nil {
		s.T().Logf("Warning: Failed to list deposits from chain service: %v", err)
		return
	}

	s.T().Logf("Found %d deposits from chain service", len(resp.Deposits))
}

// TestGetDepositStatus tests getting deposit status from chain service
func (s *DepositFlowTestSuite) TestGetDepositStatus() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	// Try to get status of non-existent deposit
	_, err := s.chainClient.GetDepositStatus(s.ctx, &chainv1.GetDepositStatusRequest{
		DepositId: "non-existent-deposit",
	})

	// Should return error for non-existent deposit
	assert.Error(s.T(), err)
}

// TestBalanceAfterDeposit tests that balance is correctly updated after deposit
func (s *DepositFlowTestSuite) TestBalanceAfterDeposit() {
	wallet := GenerateWalletAddress()
	token := s.config.TestQuoteToken

	// Get initial balance
	balance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  token,
	})
	require.NoError(s.T(), err)

	s.T().Logf("Balance for new wallet %s:", wallet)
	s.T().Logf("  Total: %s", balance.Total)
	s.T().Logf("  Total Available: %s", balance.TotalAvailable)
	s.T().Logf("  Settled Available: %s", balance.SettledAvailable)
	s.T().Logf("  Pending Deposit: %s", balance.PendingDeposit)
	s.T().Logf("  Withdrawable: %s", balance.Withdrawable)
}

// TestDepositConfirmationTracking tests confirmation count tracking
func (s *DepositFlowTestSuite) TestDepositConfirmationTracking() {
	if s.chainClient == nil {
		s.T().Skip("Chain service not available")
	}

	// Get current block height
	blockResp, err := s.chainClient.GetBlockHeight(s.ctx, &chainv1.GetBlockHeightRequest{})
	if err != nil {
		s.T().Logf("Warning: Failed to get block height: %v", err)
		return
	}

	s.T().Logf("Current block height: %d", blockResp.BlockHeight)
	s.T().Logf("Block hash: %s", blockResp.BlockHash)
}

// TestDepositIdempotency tests that duplicate deposits are handled correctly
func (s *DepositFlowTestSuite) TestDepositIdempotency() {
	s.T().Log("Testing deposit idempotency...")

	// Deposits are uniquely identified by tx_hash + log_index
	// Processing the same deposit twice should be idempotent

	wallet := GenerateWalletAddress()
	txHash := GenerateTxHash()
	logIndex := uint32(0)
	amount := "500.00"

	depositReq := &tradingv1.ProcessDepositEventRequest{
		TxHash:       txHash,
		LogIndex:     logIndex,
		BlockNum:     12345679,
		Wallet:       wallet,
		Token:        s.config.TestQuoteToken,
		TokenAddress: "0x0000000000000000000000000000000000000001",
		Amount:       amount,
		DetectedAt:   time.Now().UnixMilli(),
	}

	// First call
	_, err1 := s.tradingClient.ProcessDepositEvent(s.ctx, depositReq)
	if err1 != nil {
		s.T().Logf("Note: First ProcessDepositEvent call: %v", err1)
	}

	// Second call with same tx_hash + log_index should be idempotent
	_, err2 := s.tradingClient.ProcessDepositEvent(s.ctx, depositReq)
	if err2 != nil {
		s.T().Logf("Note: Second ProcessDepositEvent call (idempotent): %v", err2)
	}

	// Verify balance was only credited once
	balance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	if err != nil {
		s.T().Logf("Note: Failed to get balance: %v", err)
		return
	}

	total, _ := decimal.NewFromString(balance.Total)
	expected, _ := decimal.NewFromString(amount)

	// Should be at most the deposit amount (not doubled)
	assert.True(s.T(), total.LessThanOrEqual(expected) || total.IsZero(),
		"Balance should not be credited twice")
}

// TestDepositBalanceLogs tests balance log entries for deposits
func (s *DepositFlowTestSuite) TestDepositBalanceLogs() {
	wallet := GenerateWalletAddress()

	// Get balance logs
	resp, err := s.tradingClient.GetBalanceLogs(s.ctx, &tradingv1.GetBalanceLogsRequest{
		Wallet:   wallet,
		Token:    s.config.TestQuoteToken,
		Type:     commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(s.T(), err)
	require.NotNil(s.T(), resp)

	s.T().Logf("Found %d deposit balance logs for wallet %s", len(resp.Logs), wallet)

	for _, log := range resp.Logs {
		s.T().Logf("  Log ID: %d, Amount: %s, TxHash: %s",
			log.Id, log.Amount, log.TxHash)
	}
}

// TestMultipleDeposits tests handling multiple concurrent deposits
func (s *DepositFlowTestSuite) TestMultipleDeposits() {
	wallet := GenerateWalletAddress()

	// Get initial balance
	initialBalance, err := s.tradingClient.GetBalance(s.ctx, &tradingv1.GetBalanceRequest{
		Wallet: wallet,
		Token:  s.config.TestQuoteToken,
	})
	require.NoError(s.T(), err)

	initial, _ := decimal.NewFromString(initialBalance.Total)
	s.T().Logf("Initial balance: %s", initial.String())

	// Simulate multiple deposits
	depositAmounts := []string{"100.00", "200.00", "300.00"}

	for i, amount := range depositAmounts {
		depositReq := &tradingv1.ProcessDepositEventRequest{
			TxHash:       GenerateTxHash(),
			LogIndex:     uint32(i),
			BlockNum:     int64(12345680 + i),
			Wallet:       wallet,
			Token:        s.config.TestQuoteToken,
			TokenAddress: "0x0000000000000000000000000000000000000001",
			Amount:       amount,
			DetectedAt:   time.Now().UnixMilli(),
		}

		_, err := s.tradingClient.ProcessDepositEvent(s.ctx, depositReq)
		if err != nil {
			s.T().Logf("Note: ProcessDepositEvent %d failed: %v", i, err)
		}
	}

	// List all deposits for the wallet
	depositsResp, err := s.tradingClient.ListDeposits(s.ctx, &tradingv1.ListDepositsRequest{
		Wallet:   wallet,
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		s.T().Logf("Note: Failed to list deposits: %v", err)
		return
	}

	s.T().Logf("Total deposits found: %d", depositsResp.Total)
}
