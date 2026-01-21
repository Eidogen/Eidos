// Package handler gRPC 服务处理器
package handler

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/service"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	chainv1 "github.com/eidos-exchange/eidos/proto/chain/v1"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCHandler gRPC 服务处理器，实现 chainv1.ChainServiceServer 接口
type GRPCHandler struct {
	chainv1.UnimplementedChainServiceServer
	settlementSvc      *service.SettlementService
	withdrawalSvc      *service.WithdrawalService
	indexerSvc         *service.IndexerService
	reconciliationSvc  *service.ReconciliationService
	settlementRepo     repository.SettlementRepository
	withdrawalRepo     repository.WithdrawalRepository
	depositRepo        repository.DepositRepository
	reconciliationRepo repository.ReconciliationRepository
	blockchainClient   *blockchain.Client
	nonceManager       *blockchain.NonceManager
}

// NewGRPCHandler 创建 gRPC 处理器
func NewGRPCHandler(
	settlementSvc *service.SettlementService,
	withdrawalSvc *service.WithdrawalService,
	indexerSvc *service.IndexerService,
	reconciliationSvc *service.ReconciliationService,
	settlementRepo repository.SettlementRepository,
	withdrawalRepo repository.WithdrawalRepository,
	depositRepo repository.DepositRepository,
	reconciliationRepo repository.ReconciliationRepository,
	blockchainClient *blockchain.Client,
	nonceManager *blockchain.NonceManager,
) *GRPCHandler {
	return &GRPCHandler{
		settlementSvc:      settlementSvc,
		withdrawalSvc:      withdrawalSvc,
		indexerSvc:         indexerSvc,
		reconciliationSvc:  reconciliationSvc,
		settlementRepo:     settlementRepo,
		withdrawalRepo:     withdrawalRepo,
		depositRepo:        depositRepo,
		reconciliationRepo: reconciliationRepo,
		blockchainClient:   blockchainClient,
		nonceManager:       nonceManager,
	}
}

// ========== Settlement 结算相关 ==========

// GetSettlementStatus 获取结算状态
func (h *GRPCHandler) GetSettlementStatus(ctx context.Context, req *chainv1.GetSettlementStatusRequest) (*chainv1.GetSettlementStatusResponse, error) {
	batch, err := h.settlementRepo.GetByBatchID(ctx, req.BatchId)
	if err != nil {
		if err == repository.ErrSettlementBatchNotFound {
			return nil, status.Error(codes.NotFound, "settlement batch not found")
		}
		logger.Error("failed to get settlement batch", zap.Error(err))
		return nil, status.Error(codes.Internal, "internal error")
	}

	tradeIDs, _ := batch.GetTradeIDList()

	return &chainv1.GetSettlementStatusResponse{
		Batch: &chainv1.SettlementBatch{
			BatchId:      batch.BatchID,
			TradeCount:   int32(batch.TradeCount),
			TradeIds:     tradeIDs,
			ChainId:      batch.ChainID,
			TxHash:       batch.TxHash,
			BlockNumber:  batch.BlockNumber,
			GasUsed:      batch.GasUsed,
			GasPrice:     batch.GasPrice,
			Status:       commonv1.BatchStatus(batch.Status),
			ErrorMessage: batch.ErrorMessage,
			RetryCount:   int32(batch.RetryCount),
			SubmittedAt:  batch.SubmittedAt,
			ConfirmedAt:  batch.ConfirmedAt,
			CreatedAt:    batch.CreatedAt,
		},
	}, nil
}

// RetrySettlement 重试结算
func (h *GRPCHandler) RetrySettlement(ctx context.Context, req *chainv1.RetrySettlementRequest) (*chainv1.RetrySettlementResponse, error) {
	batch, err := h.settlementRepo.GetByBatchID(ctx, req.BatchId)
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
	if req.SplitIfFailed && batch.Status == model.SettlementBatchStatusFailed {
		if err := h.settlementSvc.SplitAndRetryBatch(ctx, req.BatchId); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		// TODO: 返回新批次 ID
		return &chainv1.RetrySettlementResponse{Success: true, Message: "batch split and retrying"}, nil
	}

	// 直接重试
	if err := h.settlementSvc.RetryBatch(ctx, req.BatchId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.RetrySettlementResponse{Success: true, Message: "batch retry scheduled"}, nil
}

// ListSettlementBatches 列出结算批次
func (h *GRPCHandler) ListSettlementBatches(ctx context.Context, req *chainv1.ListSettlementBatchesRequest) (*chainv1.ListSettlementBatchesResponse, error) {
	pagination := &repository.Pagination{
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	}

	var batches []*model.SettlementBatch
	var err error

	if req.Status >= 0 {
		batches, err = h.settlementRepo.ListByStatus(ctx, model.SettlementBatchStatus(req.Status), pagination)
	} else {
		batches, err = h.settlementRepo.ListByTimeRange(ctx, 0, 9999999999999, pagination)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*chainv1.SettlementBatch, len(batches))
	for i, batch := range batches {
		tradeIDs, _ := batch.GetTradeIDList()
		resp[i] = &chainv1.SettlementBatch{
			BatchId:      batch.BatchID,
			TradeCount:   int32(batch.TradeCount),
			TradeIds:     tradeIDs,
			ChainId:      batch.ChainID,
			TxHash:       batch.TxHash,
			BlockNumber:  batch.BlockNumber,
			GasUsed:      batch.GasUsed,
			GasPrice:     batch.GasPrice,
			Status:       commonv1.BatchStatus(batch.Status),
			ErrorMessage: batch.ErrorMessage,
			RetryCount:   int32(batch.RetryCount),
			SubmittedAt:  batch.SubmittedAt,
			ConfirmedAt:  batch.ConfirmedAt,
			CreatedAt:    batch.CreatedAt,
		}
	}

	return &chainv1.ListSettlementBatchesResponse{
		Batches: resp,
		Pagination: &commonv1.PaginationResponse{
			Page:     int32(pagination.Page),
			PageSize: int32(pagination.PageSize),
			Total:    pagination.Total,
		},
	}, nil
}

// ========== Deposit 充值相关 ==========

// GetDepositStatus 获取充值状态
func (h *GRPCHandler) GetDepositStatus(ctx context.Context, req *chainv1.GetDepositStatusRequest) (*chainv1.GetDepositStatusResponse, error) {
	record, err := h.depositRepo.GetByDepositID(ctx, req.DepositId)
	if err != nil {
		if err == repository.ErrDepositRecordNotFound {
			return nil, status.Error(codes.NotFound, "deposit not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chainv1.GetDepositStatusResponse{
		Deposit: &chainv1.DepositRecord{
			DepositId:             record.DepositID,
			WalletAddress:         record.WalletAddress,
			Token:                 record.Token,
			TokenAddress:          record.TokenAddress,
			Amount:                record.Amount.String(),
			ChainId:               record.ChainID,
			TxHash:                record.TxHash,
			BlockNumber:           record.BlockNumber,
			LogIndex:              int32(record.LogIndex),
			Confirmations:         int32(record.Confirmations),
			RequiredConfirmations: int32(record.RequiredConfirmations),
			Status:                commonv1.DepositStatus(record.Status),
			CreditedAt:            record.CreditedAt,
			CreatedAt:             record.CreatedAt,
		},
	}, nil
}

// ListDeposits 列出充值记录
func (h *GRPCHandler) ListDeposits(ctx context.Context, req *chainv1.ListDepositsRequest) (*chainv1.ListDepositsResponse, error) {
	pagination := &repository.Pagination{
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	}

	var records []*model.DepositRecord
	var err error

	if req.WalletAddress != "" {
		records, err = h.depositRepo.ListByWallet(ctx, req.WalletAddress, pagination)
	} else if req.Status >= 0 {
		records, err = h.depositRepo.ListByStatus(ctx, model.DepositRecordStatus(req.Status), pagination)
	} else {
		pagination.PageSize = 100
		records, err = h.depositRepo.ListPendingConfirmation(ctx, pagination.PageSize)
		pagination.Total = int64(len(records))
	}

	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*chainv1.DepositRecord, len(records))
	for i, record := range records {
		resp[i] = &chainv1.DepositRecord{
			DepositId:             record.DepositID,
			WalletAddress:         record.WalletAddress,
			Token:                 record.Token,
			TokenAddress:          record.TokenAddress,
			Amount:                record.Amount.String(),
			ChainId:               record.ChainID,
			TxHash:                record.TxHash,
			BlockNumber:           record.BlockNumber,
			LogIndex:              int32(record.LogIndex),
			Confirmations:         int32(record.Confirmations),
			RequiredConfirmations: int32(record.RequiredConfirmations),
			Status:                commonv1.DepositStatus(record.Status),
			CreditedAt:            record.CreditedAt,
			CreatedAt:             record.CreatedAt,
		}
	}

	return &chainv1.ListDepositsResponse{
		Deposits: resp,
		Pagination: &commonv1.PaginationResponse{
			Page:     int32(pagination.Page),
			PageSize: int32(pagination.PageSize),
			Total:    pagination.Total,
		},
	}, nil
}

// ========== Withdrawal 提现相关 ==========

// GetWithdrawalStatus 获取提现状态
func (h *GRPCHandler) GetWithdrawalStatus(ctx context.Context, req *chainv1.GetWithdrawalStatusRequest) (*chainv1.GetWithdrawalStatusResponse, error) {
	tx, err := h.withdrawalRepo.GetByWithdrawID(ctx, req.WithdrawId)
	if err != nil {
		if err == repository.ErrWithdrawalTxNotFound {
			return nil, status.Error(codes.NotFound, "withdrawal not found")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chainv1.GetWithdrawalStatusResponse{
		Withdrawal: &chainv1.WithdrawalTx{
			WithdrawId:    tx.WithdrawID,
			WalletAddress: tx.WalletAddress,
			ToAddress:     tx.ToAddress,
			Token:         tx.Token,
			TokenAddress:  tx.TokenAddress,
			Amount:        tx.Amount.String(),
			ChainId:       tx.ChainID,
			TxHash:        tx.TxHash,
			BlockNumber:   tx.BlockNumber,
			GasUsed:       tx.GasUsed,
			Status:        commonv1.WithdrawStatus(tx.Status),
			ErrorMessage:  tx.ErrorMessage,
			RetryCount:    int32(tx.RetryCount),
			SubmittedAt:   tx.SubmittedAt,
			ConfirmedAt:   tx.ConfirmedAt,
			CreatedAt:     tx.CreatedAt,
		},
	}, nil
}

// RetryWithdrawal 重试提现
func (h *GRPCHandler) RetryWithdrawal(ctx context.Context, req *chainv1.RetryWithdrawalRequest) (*chainv1.RetryWithdrawalResponse, error) {
	if err := h.withdrawalSvc.RetryWithdrawal(ctx, req.WithdrawId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &chainv1.RetryWithdrawalResponse{Success: true, Message: "withdrawal retry scheduled"}, nil
}

// ListPendingWithdrawals 列出待处理提现
func (h *GRPCHandler) ListPendingWithdrawals(ctx context.Context, req *chainv1.ListPendingWithdrawalsRequest) (*chainv1.ListPendingWithdrawalsResponse, error) {
	pagination := &repository.Pagination{
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	}

	txs, err := h.withdrawalRepo.ListByWallet(ctx, req.WalletAddress, pagination)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	resp := make([]*chainv1.WithdrawalTx, len(txs))
	for i, tx := range txs {
		resp[i] = &chainv1.WithdrawalTx{
			WithdrawId:    tx.WithdrawID,
			WalletAddress: tx.WalletAddress,
			ToAddress:     tx.ToAddress,
			Token:         tx.Token,
			TokenAddress:  tx.TokenAddress,
			Amount:        tx.Amount.String(),
			ChainId:       tx.ChainID,
			TxHash:        tx.TxHash,
			BlockNumber:   tx.BlockNumber,
			GasUsed:       tx.GasUsed,
			Status:        commonv1.WithdrawStatus(tx.Status),
			ErrorMessage:  tx.ErrorMessage,
			RetryCount:    int32(tx.RetryCount),
			SubmittedAt:   tx.SubmittedAt,
			ConfirmedAt:   tx.ConfirmedAt,
			CreatedAt:     tx.CreatedAt,
		}
	}

	return &chainv1.ListPendingWithdrawalsResponse{
		Withdrawals: resp,
		Pagination: &commonv1.PaginationResponse{
			Page:     int32(pagination.Page),
			PageSize: int32(pagination.PageSize),
			Total:    pagination.Total,
		},
	}, nil
}

// ========== Indexer 索引器相关 ==========

// GetIndexerStatus 获取索引器状态
func (h *GRPCHandler) GetIndexerStatus(ctx context.Context, req *chainv1.GetIndexerStatusRequest) (*chainv1.GetIndexerStatusResponse, error) {
	indexerStatus, err := h.indexerSvc.GetIndexerStatus(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.GetIndexerStatusResponse{
		ChainId:         indexerStatus.ChainID,
		Running:         indexerStatus.Running,
		CurrentBlock:    indexerStatus.CurrentBlock,
		LatestBlock:     indexerStatus.LatestBlock,
		LagBlocks:       indexerStatus.LagBlocks,
		CheckpointBlock: indexerStatus.CheckpointBlock,
	}, nil
}

// GetBlockHeight 获取区块高度
func (h *GRPCHandler) GetBlockHeight(ctx context.Context, req *chainv1.GetBlockHeightRequest) (*chainv1.GetBlockHeightResponse, error) {
	blockNumber, err := h.blockchainClient.BlockNumber(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.GetBlockHeightResponse{
		BlockHeight: blockNumber,
	}, nil
}

// ========== Reconciliation 对账相关 ==========

// TriggerReconciliation 触发对账
func (h *GRPCHandler) TriggerReconciliation(ctx context.Context, req *chainv1.TriggerReconciliationRequest) (*chainv1.TriggerReconciliationResponse, error) {
	if h.reconciliationSvc == nil {
		return nil, status.Error(codes.Unavailable, "reconciliation service not configured")
	}

	taskID, err := h.reconciliationSvc.TriggerReconciliation(ctx, req.WalletAddress, req.Token)
	if err != nil {
		logger.Error("trigger reconciliation failed", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.TriggerReconciliationResponse{
		Success: true,
		TaskId:  taskID,
		Message: "reconciliation triggered",
	}, nil
}

// GetReconciliationStatus 获取对账状态
func (h *GRPCHandler) GetReconciliationStatus(ctx context.Context, req *chainv1.GetReconciliationStatusRequest) (*chainv1.GetReconciliationStatusResponse, error) {
	if h.reconciliationSvc == nil {
		return nil, status.Error(codes.Unavailable, "reconciliation service not configured")
	}

	task, err := h.reconciliationSvc.GetTaskStatus(req.TaskId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &chainv1.GetReconciliationStatusResponse{
		TaskId:        task.TaskID,
		Status:        task.Status,
		TotalChecked:  task.TotalChecked,
		Discrepancies: task.Discrepancies,
		StartedAt:     task.StartedAt,
		CompletedAt:   task.CompletedAt,
	}, nil
}

// ListReconciliationRecords 列出对账记录
func (h *GRPCHandler) ListReconciliationRecords(ctx context.Context, req *chainv1.ListReconciliationRecordsRequest) (*chainv1.ListReconciliationRecordsResponse, error) {
	pagination := &repository.Pagination{
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	}

	var records []*model.ReconciliationRecord
	var err error

	if req.WalletAddress != "" {
		records, err = h.reconciliationRepo.ListByWallet(ctx, req.WalletAddress, pagination)
	} else if req.Status != commonv1.ReconciliationStatus_RECONCILIATION_STATUS_UNSPECIFIED {
		records, err = h.reconciliationRepo.ListByStatus(ctx, model.ReconciliationStatus(req.Status), pagination)
	} else {
		records, err = h.reconciliationRepo.ListDiscrepancies(ctx, pagination)
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := make([]*chainv1.ReconciliationRecord, len(records))
	for i, record := range records {
		resp[i] = &chainv1.ReconciliationRecord{
			Id:                record.ID,
			WalletAddress:     record.WalletAddress,
			Token:             record.Token,
			OnChainBalance:    record.OnChainBalance.String(),
			OffChainSettled:   record.OffChainSettled.String(),
			OffChainAvailable: record.OffChainAvailable.String(),
			OffChainFrozen:    record.OffChainFrozen.String(),
			PendingSettle:     record.PendingSettle.String(),
			Difference:        record.Difference.String(),
			Status:            modelToProtoReconciliationStatus(record.Status),
			Resolution:        record.Resolution,
			ResolvedBy:        record.ResolvedBy,
			ResolvedAt:        record.ResolvedAt,
			CheckedAt:         record.CheckedAt,
		}
	}

	return &chainv1.ListReconciliationRecordsResponse{
		Records: resp,
		Pagination: &commonv1.PaginationResponse{
			Page:     int32(pagination.Page),
			PageSize: int32(pagination.PageSize),
			Total:    pagination.Total,
		},
	}, nil
}

// ========== Wallet 热钱包相关 ==========

// GetWalletBalance 获取热钱包余额
func (h *GRPCHandler) GetWalletBalance(ctx context.Context, req *chainv1.GetWalletBalanceRequest) (*chainv1.GetWalletBalanceResponse, error) {
	balance, err := h.blockchainClient.BalanceAt(ctx, h.blockchainClient.Address(), nil)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.GetWalletBalanceResponse{
		WalletAddress: h.blockchainClient.Address().Hex(),
		Balance:       balance.String(),
		Token:         "ETH",
	}, nil
}

// GetWalletNonce 获取热钱包 Nonce
func (h *GRPCHandler) GetWalletNonce(ctx context.Context, req *chainv1.GetWalletNonceRequest) (*chainv1.GetWalletNonceResponse, error) {
	nonce, err := h.nonceManager.GetCurrentNonce(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &chainv1.GetWalletNonceResponse{
		WalletAddress: h.blockchainClient.Address().Hex(),
		CurrentNonce:  nonce,
		PendingCount:  int32(h.nonceManager.GetPendingCount()),
	}, nil
}

// modelToProtoReconciliationStatus converts model ReconciliationStatus to proto enum
func modelToProtoReconciliationStatus(s model.ReconciliationStatus) commonv1.ReconciliationStatus {
	switch s {
	case model.ReconciliationStatusOK:
		return commonv1.ReconciliationStatus_RECONCILIATION_STATUS_MATCHED
	case model.ReconciliationStatusDiscrepancy:
		return commonv1.ReconciliationStatus_RECONCILIATION_STATUS_DISCREPANCY
	case model.ReconciliationStatusResolved:
		return commonv1.ReconciliationStatus_RECONCILIATION_STATUS_RESOLVED
	case model.ReconciliationStatusIgnored:
		return commonv1.ReconciliationStatus_RECONCILIATION_STATUS_IGNORED
	default:
		return commonv1.ReconciliationStatus_RECONCILIATION_STATUS_UNSPECIFIED
	}
}
