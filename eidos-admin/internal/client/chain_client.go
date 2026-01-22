package client

import (
	"context"

	"google.golang.org/grpc"

	chainv1 "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
)

// ChainClient wraps the chain service gRPC client
type ChainClient struct {
	conn     *grpc.ClientConn
	client   chainv1.ChainServiceClient
	ownsConn bool // 是否拥有连接（用于关闭时判断）
}

// NewChainClient creates a new chain client
func NewChainClient(ctx context.Context, target string) (*ChainClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &ChainClient{
		conn:     conn,
		client:   chainv1.NewChainServiceClient(conn),
		ownsConn: true,
	}, nil
}

// NewChainClientFromConn creates a chain client from an existing connection (service discovery mode)
func NewChainClientFromConn(conn *grpc.ClientConn) *ChainClient {
	return &ChainClient{
		conn:     conn,
		client:   chainv1.NewChainServiceClient(conn),
		ownsConn: false,
	}
}

// Close closes the connection
func (c *ChainClient) Close() error {
	if c.ownsConn && c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetIndexerStatus retrieves the status of the chain indexer
func (c *ChainClient) GetIndexerStatus(ctx context.Context, chainID int64) (*chainv1.GetIndexerStatusResponse, error) {
	return c.client.GetIndexerStatus(ctx, &chainv1.GetIndexerStatusRequest{
		ChainId: chainID,
	})
}

// GetBlockHeight retrieves the current block height
func (c *ChainClient) GetBlockHeight(ctx context.Context, chainID int64) (*chainv1.GetBlockHeightResponse, error) {
	return c.client.GetBlockHeight(ctx, &chainv1.GetBlockHeightRequest{
		ChainId: chainID,
	})
}

// GetDepositStatus retrieves status of a deposit
func (c *ChainClient) GetDepositStatus(ctx context.Context, depositID string) (*chainv1.DepositRecord, error) {
	resp, err := c.client.GetDepositStatus(ctx, &chainv1.GetDepositStatusRequest{
		DepositId: depositID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Deposit, nil
}

// ListDeposits retrieves deposits with filters
func (c *ChainClient) ListDeposits(ctx context.Context, req *chainv1.ListDepositsRequest) (*chainv1.ListDepositsResponse, error) {
	return c.client.ListDeposits(ctx, req)
}

// GetWithdrawalStatus retrieves status of a withdrawal transaction
func (c *ChainClient) GetWithdrawalStatus(ctx context.Context, withdrawID string) (*chainv1.WithdrawalTx, error) {
	resp, err := c.client.GetWithdrawalStatus(ctx, &chainv1.GetWithdrawalStatusRequest{
		WithdrawId: withdrawID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Withdrawal, nil
}

// ListPendingWithdrawals retrieves pending withdrawal transactions
func (c *ChainClient) ListPendingWithdrawals(ctx context.Context, wallet, token string, page, pageSize int32) (*chainv1.ListPendingWithdrawalsResponse, error) {
	return c.client.ListPendingWithdrawals(ctx, &chainv1.ListPendingWithdrawalsRequest{
		WalletAddress: wallet,
		Token:         token,
		Pagination: &commonv1.PaginationRequest{
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// RetryWithdrawal retries a failed withdrawal
func (c *ChainClient) RetryWithdrawal(ctx context.Context, withdrawID string, gasBumpPercent int32) (*chainv1.RetryWithdrawalResponse, error) {
	return c.client.RetryWithdrawal(ctx, &chainv1.RetryWithdrawalRequest{
		WithdrawId:     withdrawID,
		GasBumpPercent: gasBumpPercent,
	})
}

// GetSettlementStatus retrieves status of a settlement batch
func (c *ChainClient) GetSettlementStatus(ctx context.Context, batchID string) (*chainv1.SettlementBatch, error) {
	resp, err := c.client.GetSettlementStatus(ctx, &chainv1.GetSettlementStatusRequest{
		BatchId: batchID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Batch, nil
}

// ListSettlementBatches retrieves settlement batches with filters
func (c *ChainClient) ListSettlementBatches(ctx context.Context, req *chainv1.ListSettlementBatchesRequest) (*chainv1.ListSettlementBatchesResponse, error) {
	return c.client.ListSettlementBatches(ctx, req)
}

// RetrySettlement retries a failed settlement batch
func (c *ChainClient) RetrySettlement(ctx context.Context, batchID string, splitIfFailed bool, gasBumpPercent int32) (*chainv1.RetrySettlementResponse, error) {
	return c.client.RetrySettlement(ctx, &chainv1.RetrySettlementRequest{
		BatchId:        batchID,
		SplitIfFailed:  splitIfFailed,
		GasBumpPercent: gasBumpPercent,
	})
}

// TriggerReconciliation triggers a reconciliation check
func (c *ChainClient) TriggerReconciliation(ctx context.Context, wallet, token string, recType commonv1.ReconciliationType) (*chainv1.TriggerReconciliationResponse, error) {
	return c.client.TriggerReconciliation(ctx, &chainv1.TriggerReconciliationRequest{
		WalletAddress: wallet,
		Token:         token,
		Type:          recType,
	})
}

// GetReconciliationStatus retrieves status of a reconciliation task
func (c *ChainClient) GetReconciliationStatus(ctx context.Context, taskID string) (*chainv1.GetReconciliationStatusResponse, error) {
	return c.client.GetReconciliationStatus(ctx, &chainv1.GetReconciliationStatusRequest{
		TaskId: taskID,
	})
}

// ListReconciliationRecords retrieves reconciliation records
func (c *ChainClient) ListReconciliationRecords(ctx context.Context, req *chainv1.ListReconciliationRecordsRequest) (*chainv1.ListReconciliationRecordsResponse, error) {
	return c.client.ListReconciliationRecords(ctx, req)
}

// GetReconciliationReport retrieves a reconciliation summary report
func (c *ChainClient) GetReconciliationReport(ctx context.Context, startTime, endTime int64) (*chainv1.GetReconciliationReportResponse, error) {
	return c.client.GetReconciliationReport(ctx, &chainv1.GetReconciliationReportRequest{
		StartTime: startTime,
		EndTime:   endTime,
	})
}

// GetWalletBalance retrieves the hot wallet balance
func (c *ChainClient) GetWalletBalance(ctx context.Context, chainID int64, tokenAddress string) (*chainv1.GetWalletBalanceResponse, error) {
	return c.client.GetWalletBalance(ctx, &chainv1.GetWalletBalanceRequest{
		ChainId:      chainID,
		TokenAddress: tokenAddress,
	})
}

// GetWalletNonce retrieves the current nonce for the hot wallet
func (c *ChainClient) GetWalletNonce(ctx context.Context, chainID int64) (*chainv1.GetWalletNonceResponse, error) {
	return c.client.GetWalletNonce(ctx, &chainv1.GetWalletNonceRequest{
		ChainId: chainID,
	})
}

// GetChainStatus retrieves overall chain service status
func (c *ChainClient) GetChainStatus(ctx context.Context) (*chainv1.GetChainStatusResponse, error) {
	return c.client.GetChainStatus(ctx, &chainv1.GetChainStatusRequest{})
}
