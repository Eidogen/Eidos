package handler

import (
	"context"
	"encoding/json"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ChainHandler gRPC 处理器
type ChainHandler struct {
	settlementSvc     *service.SettlementService
	withdrawalSvc     *service.WithdrawalService
	indexerSvc        *service.IndexerService
	settlementRepo    repository.SettlementRepository
	withdrawalRepo    repository.WithdrawalRepository
	depositRepo       repository.DepositRepository
	reconciliationRepo repository.ReconciliationRepository
	blockchainClient  *blockchain.Client
	nonceManager      *blockchain.NonceManager
}

// NewChainHandler 创建处理器
func NewChainHandler(
	settlementSvc *service.SettlementService,
	withdrawalSvc *service.WithdrawalService,
	indexerSvc *service.IndexerService,
	settlementRepo repository.SettlementRepository,
	withdrawalRepo repository.WithdrawalRepository,
	depositRepo repository.DepositRepository,
	reconciliationRepo repository.ReconciliationRepository,
	blockchainClient *blockchain.Client,
	nonceManager *blockchain.NonceManager,
) *ChainHandler {
	return &ChainHandler{
		settlementSvc:      settlementSvc,
		withdrawalSvc:      withdrawalSvc,
		indexerSvc:         indexerSvc,
		settlementRepo:     settlementRepo,
		withdrawalRepo:     withdrawalRepo,
		depositRepo:        depositRepo,
		reconciliationRepo: reconciliationRepo,
		blockchainClient:   blockchainClient,
		nonceManager:       nonceManager,
	}
}

// SettlementBatchResponse 结算批次响应
type SettlementBatchResponse struct {
	BatchID      string   `json:"batch_id"`
	TradeCount   int      `json:"trade_count"`
	TradeIDs     []string `json:"trade_ids"`
	ChainID      int64    `json:"chain_id"`
	TxHash       string   `json:"tx_hash"`
	BlockNumber  int64    `json:"block_number"`
	GasUsed      int64    `json:"gas_used"`
	GasPrice     string   `json:"gas_price"`
	Status       int32    `json:"status"`
	ErrorMessage string   `json:"error_message"`
	RetryCount   int      `json:"retry_count"`
	SubmittedAt  int64    `json:"submitted_at"`
	ConfirmedAt  int64    `json:"confirmed_at"`
	CreatedAt    int64    `json:"created_at"`
}

// GetSettlementStatus 获取结算状态
func (h *ChainHandler) GetSettlementStatus(ctx context.Context, batchID string) (*SettlementBatchResponse, error) {
	batch, err := h.settlementRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		if err == repository.ErrSettlementBatchNotFound {
			return nil, status.Error(codes.NotFound, "settlement batch not found")
		}
		logger.Error("failed to get settlement batch", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	tradeIDs, _ := batch.GetTradeIDList()

	return &SettlementBatchResponse{
		BatchID:      batch.BatchID,
		TradeCount:   batch.TradeCount,
		TradeIDs:     tradeIDs,
		ChainID:      batch.ChainID,
		TxHash:       batch.TxHash,
		BlockNumber:  batch.BlockNumber,
		GasUsed:      batch.GasUsed,
		GasPrice:     batch.GasPrice,
		Status:       int32(batch.Status),
		ErrorMessage: batch.ErrorMessage,
		RetryCount:   batch.RetryCount,
		SubmittedAt:  batch.SubmittedAt,
		ConfirmedAt:  batch.ConfirmedAt,
		CreatedAt:    batch.CreatedAt,
	}, nil
}

// RetrySettlement 重试结算
func (h *ChainHandler) RetrySettlement(ctx context.Context, batchID string, splitIfFailed bool) ([]string, error) {
	batch, err := h.settlementRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		if err == repository.ErrSettlementBatchNotFound {
			return nil, status.Error(codes.NotFound, "settlement batch not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	if batch.Status == model.SettlementBatchStatusConfirmed {
		return nil, status.Error(codes.FailedPrecondition, "batch already confirmed")
	}

	// 如果需要拆分
	if splitIfFailed && batch.Status == model.SettlementBatchStatusFailed {
		if err := h.settlementSvc.SplitAndRetryBatch(ctx, batchID); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		// 拆分后会生成新批次，当前返回空
		return nil, nil
	}

	// 直接重试
	if err := h.settlementSvc.RetryBatch(ctx, batchID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return nil, nil
}

// ListSettlementBatches 列出结算批次
func (h *ChainHandler) ListSettlementBatches(ctx context.Context, statusFilter int32, page, pageSize int) ([]*SettlementBatchResponse, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	var batches []*model.SettlementBatch
	var err error

	if statusFilter >= 0 {
		batches, err = h.settlementRepo.ListByStatus(ctx, model.SettlementBatchStatus(statusFilter), pagination)
	} else {
		batches, err = h.settlementRepo.ListByTimeRange(ctx, 0, 9999999999999, pagination)
	}

	if err != nil {
		return nil, 0, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*SettlementBatchResponse, len(batches))
	for i, batch := range batches {
		tradeIDs, _ := batch.GetTradeIDList()
		resp[i] = &SettlementBatchResponse{
			BatchID:      batch.BatchID,
			TradeCount:   batch.TradeCount,
			TradeIDs:     tradeIDs,
			ChainID:      batch.ChainID,
			TxHash:       batch.TxHash,
			BlockNumber:  batch.BlockNumber,
			GasUsed:      batch.GasUsed,
			GasPrice:     batch.GasPrice,
			Status:       int32(batch.Status),
			ErrorMessage: batch.ErrorMessage,
			RetryCount:   batch.RetryCount,
			SubmittedAt:  batch.SubmittedAt,
			ConfirmedAt:  batch.ConfirmedAt,
			CreatedAt:    batch.CreatedAt,
		}
	}

	return resp, pagination.Total, nil
}

// DepositRecordResponse 充值记录响应
type DepositRecordResponse struct {
	DepositID             string `json:"deposit_id"`
	WalletAddress         string `json:"wallet_address"`
	Token                 string `json:"token"`
	TokenAddress          string `json:"token_address"`
	Amount                string `json:"amount"`
	ChainID               int64  `json:"chain_id"`
	TxHash                string `json:"tx_hash"`
	BlockNumber           int64  `json:"block_number"`
	LogIndex              int    `json:"log_index"`
	Confirmations         int    `json:"confirmations"`
	RequiredConfirmations int    `json:"required_confirmations"`
	Status                int32  `json:"status"`
	CreditedAt            int64  `json:"credited_at"`
	CreatedAt             int64  `json:"created_at"`
}

// GetDepositStatus 获取充值状态
func (h *ChainHandler) GetDepositStatus(ctx context.Context, depositID string) (*DepositRecordResponse, error) {
	record, err := h.depositRepo.GetByDepositID(ctx, depositID)
	if err != nil {
		if err == repository.ErrDepositRecordNotFound {
			return nil, status.Error(codes.NotFound, "deposit not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &DepositRecordResponse{
		DepositID:             record.DepositID,
		WalletAddress:         record.WalletAddress,
		Token:                 record.Token,
		TokenAddress:          record.TokenAddress,
		Amount:                record.Amount.String(),
		ChainID:               record.ChainID,
		TxHash:                record.TxHash,
		BlockNumber:           record.BlockNumber,
		LogIndex:              record.LogIndex,
		Confirmations:         record.Confirmations,
		RequiredConfirmations: record.RequiredConfirmations,
		Status:                int32(record.Status),
		CreditedAt:            record.CreditedAt,
		CreatedAt:             record.CreatedAt,
	}, nil
}

// ListDeposits 列出充值记录
func (h *ChainHandler) ListDeposits(ctx context.Context, wallet string, statusFilter int32, page, pageSize int) ([]*DepositRecordResponse, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	var records []*model.DepositRecord
	var err error

	if wallet != "" {
		records, err = h.depositRepo.ListByWallet(ctx, wallet, pagination)
	} else if statusFilter >= 0 {
		records, err = h.depositRepo.ListByStatus(ctx, model.DepositRecordStatus(statusFilter), pagination)
	} else {
		// 默认返回最近记录
		pagination.PageSize = 100
		records, err = h.depositRepo.ListPendingConfirmation(ctx, pagination.PageSize)
		pagination.Total = int64(len(records))
	}

	if err != nil {
		return nil, 0, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*DepositRecordResponse, len(records))
	for i, record := range records {
		resp[i] = &DepositRecordResponse{
			DepositID:             record.DepositID,
			WalletAddress:         record.WalletAddress,
			Token:                 record.Token,
			TokenAddress:          record.TokenAddress,
			Amount:                record.Amount.String(),
			ChainID:               record.ChainID,
			TxHash:                record.TxHash,
			BlockNumber:           record.BlockNumber,
			LogIndex:              record.LogIndex,
			Confirmations:         record.Confirmations,
			RequiredConfirmations: record.RequiredConfirmations,
			Status:                int32(record.Status),
			CreditedAt:            record.CreditedAt,
			CreatedAt:             record.CreatedAt,
		}
	}

	return resp, pagination.Total, nil
}

// WithdrawalTxResponse 提现交易响应
type WithdrawalTxResponse struct {
	WithdrawID    string `json:"withdraw_id"`
	WalletAddress string `json:"wallet_address"`
	ToAddress     string `json:"to_address"`
	Token         string `json:"token"`
	TokenAddress  string `json:"token_address"`
	Amount        string `json:"amount"`
	ChainID       int64  `json:"chain_id"`
	TxHash        string `json:"tx_hash"`
	BlockNumber   int64  `json:"block_number"`
	GasUsed       int64  `json:"gas_used"`
	Status        int32  `json:"status"`
	ErrorMessage  string `json:"error_message"`
	RetryCount    int    `json:"retry_count"`
	SubmittedAt   int64  `json:"submitted_at"`
	ConfirmedAt   int64  `json:"confirmed_at"`
	CreatedAt     int64  `json:"created_at"`
}

// GetWithdrawalStatus 获取提现状态
func (h *ChainHandler) GetWithdrawalStatus(ctx context.Context, withdrawID string) (*WithdrawalTxResponse, error) {
	tx, err := h.withdrawalRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		if err == repository.ErrWithdrawalTxNotFound {
			return nil, status.Error(codes.NotFound, "withdrawal not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &WithdrawalTxResponse{
		WithdrawID:    tx.WithdrawID,
		WalletAddress: tx.WalletAddress,
		ToAddress:     tx.ToAddress,
		Token:         tx.Token,
		TokenAddress:  tx.TokenAddress,
		Amount:        tx.Amount.String(),
		ChainID:       tx.ChainID,
		TxHash:        tx.TxHash,
		BlockNumber:   tx.BlockNumber,
		GasUsed:       tx.GasUsed,
		Status:        int32(tx.Status),
		ErrorMessage:  tx.ErrorMessage,
		RetryCount:    tx.RetryCount,
		SubmittedAt:   tx.SubmittedAt,
		ConfirmedAt:   tx.ConfirmedAt,
		CreatedAt:     tx.CreatedAt,
	}, nil
}

// RetryWithdrawal 重试提现
func (h *ChainHandler) RetryWithdrawal(ctx context.Context, withdrawID string) error {
	return h.withdrawalSvc.RetryWithdrawal(ctx, withdrawID)
}

// ListPendingWithdrawals 列出待处理提现
func (h *ChainHandler) ListPendingWithdrawals(ctx context.Context, wallet string, page, pageSize int) ([]*WithdrawalTxResponse, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	txs, err := h.withdrawalRepo.ListByWallet(ctx, wallet, pagination)
	if err != nil {
		return nil, 0, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*WithdrawalTxResponse, len(txs))
	for i, tx := range txs {
		resp[i] = &WithdrawalTxResponse{
			WithdrawID:    tx.WithdrawID,
			WalletAddress: tx.WalletAddress,
			ToAddress:     tx.ToAddress,
			Token:         tx.Token,
			TokenAddress:  tx.TokenAddress,
			Amount:        tx.Amount.String(),
			ChainID:       tx.ChainID,
			TxHash:        tx.TxHash,
			BlockNumber:   tx.BlockNumber,
			GasUsed:       tx.GasUsed,
			Status:        int32(tx.Status),
			ErrorMessage:  tx.ErrorMessage,
			RetryCount:    tx.RetryCount,
			SubmittedAt:   tx.SubmittedAt,
			ConfirmedAt:   tx.ConfirmedAt,
			CreatedAt:     tx.CreatedAt,
		}
	}

	return resp, pagination.Total, nil
}

// IndexerStatusResponse 索引器状态响应
type IndexerStatusResponse struct {
	ChainID         int64  `json:"chain_id"`
	Running         bool   `json:"running"`
	CurrentBlock    uint64 `json:"current_block"`
	LatestBlock     uint64 `json:"latest_block"`
	LagBlocks       int64  `json:"lag_blocks"`
	CheckpointBlock int64  `json:"checkpoint_block"`
}

// GetIndexerStatus 获取索引器状态
func (h *ChainHandler) GetIndexerStatus(ctx context.Context) (*IndexerStatusResponse, error) {
	status, err := h.indexerSvc.GetIndexerStatus(ctx)
	if err != nil {
		return nil, err
	}

	return &IndexerStatusResponse{
		ChainID:         status.ChainID,
		Running:         status.Running,
		CurrentBlock:    status.CurrentBlock,
		LatestBlock:     status.LatestBlock,
		LagBlocks:       status.LagBlocks,
		CheckpointBlock: status.CheckpointBlock,
	}, nil
}

// GetBlockHeight 获取区块高度
func (h *ChainHandler) GetBlockHeight(ctx context.Context) (uint64, error) {
	return h.blockchainClient.BlockNumber(ctx)
}

// ReconciliationRecordResponse 对账记录响应
type ReconciliationRecordResponse struct {
	ID                int64  `json:"id"`
	WalletAddress     string `json:"wallet_address"`
	Token             string `json:"token"`
	OnChainBalance    string `json:"on_chain_balance"`
	OffChainSettled   string `json:"off_chain_settled"`
	OffChainAvailable string `json:"off_chain_available"`
	OffChainFrozen    string `json:"off_chain_frozen"`
	PendingSettle     string `json:"pending_settle"`
	Difference        string `json:"difference"`
	Status            string `json:"status"`
	Resolution        string `json:"resolution"`
	ResolvedBy        string `json:"resolved_by"`
	ResolvedAt        int64  `json:"resolved_at"`
	CheckedAt         int64  `json:"checked_at"`
}

// ListReconciliationRecords 列出对账记录
func (h *ChainHandler) ListReconciliationRecords(ctx context.Context, wallet, statusFilter string, page, pageSize int) ([]*ReconciliationRecordResponse, int64, error) {
	pagination := &repository.Pagination{
		Page:     page,
		PageSize: pageSize,
	}

	var records []*model.ReconciliationRecord
	var err error

	if wallet != "" {
		records, err = h.reconciliationRepo.ListByWallet(ctx, wallet, pagination)
	} else if statusFilter != "" {
		records, err = h.reconciliationRepo.ListByStatus(ctx, model.ReconciliationStatus(statusFilter), pagination)
	} else {
		records, err = h.reconciliationRepo.ListDiscrepancies(ctx, pagination)
	}

	if err != nil {
		return nil, 0, err
	}

	resp := make([]*ReconciliationRecordResponse, len(records))
	for i, record := range records {
		resp[i] = &ReconciliationRecordResponse{
			ID:                record.ID,
			WalletAddress:     record.WalletAddress,
			Token:             record.Token,
			OnChainBalance:    record.OnChainBalance.String(),
			OffChainSettled:   record.OffChainSettled.String(),
			OffChainAvailable: record.OffChainAvailable.String(),
			OffChainFrozen:    record.OffChainFrozen.String(),
			PendingSettle:     record.PendingSettle.String(),
			Difference:        record.Difference.String(),
			Status:            string(record.Status),
			Resolution:        record.Resolution,
			ResolvedBy:        record.ResolvedBy,
			ResolvedAt:        record.ResolvedAt,
			CheckedAt:         record.CheckedAt,
		}
	}

	return resp, pagination.Total, nil
}

// WalletBalanceResponse 钱包余额响应
type WalletBalanceResponse struct {
	WalletAddress string `json:"wallet_address"`
	Balance       string `json:"balance"`
	Token         string `json:"token"`
}

// GetWalletBalance 获取热钱包余额
func (h *ChainHandler) GetWalletBalance(ctx context.Context) (*WalletBalanceResponse, error) {
	balance, err := h.blockchainClient.BalanceAt(ctx, h.blockchainClient.Address(), nil)
	if err != nil {
		return nil, err
	}

	return &WalletBalanceResponse{
		WalletAddress: h.blockchainClient.Address().Hex(),
		Balance:       balance.String(),
		Token:         "ETH",
	}, nil
}

// WalletNonceResponse 钱包 Nonce 响应
type WalletNonceResponse struct {
	WalletAddress string `json:"wallet_address"`
	CurrentNonce  uint64 `json:"current_nonce"`
	PendingCount  int    `json:"pending_count"`
}

// GetWalletNonce 获取热钱包 Nonce
func (h *ChainHandler) GetWalletNonce(ctx context.Context) (*WalletNonceResponse, error) {
	nonce, err := h.nonceManager.GetCurrentNonce(ctx)
	if err != nil {
		return nil, err
	}

	return &WalletNonceResponse{
		WalletAddress: h.blockchainClient.Address().Hex(),
		CurrentNonce:  nonce,
		PendingCount:  h.nonceManager.GetPendingCount(),
	}, nil
}

// convertBatchToJSON 辅助函数：转换批次为 JSON
func convertBatchToJSON(batch *model.SettlementBatch) ([]byte, error) {
	tradeIDs, _ := batch.GetTradeIDList()
	return json.Marshal(map[string]interface{}{
		"batch_id":      batch.BatchID,
		"trade_count":   batch.TradeCount,
		"trade_ids":     tradeIDs,
		"chain_id":      batch.ChainID,
		"tx_hash":       batch.TxHash,
		"block_number":  batch.BlockNumber,
		"status":        batch.Status,
		"error_message": batch.ErrorMessage,
		"retry_count":   batch.RetryCount,
		"created_at":    batch.CreatedAt,
	})
}
