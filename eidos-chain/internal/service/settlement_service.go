// Package service 提供 eidos-chain 的业务逻辑服务
//
// ========================================
// SettlementService 结算服务对接说明
// ========================================
//
// ## 功能概述
// SettlementService 负责将链下成交批量提交到区块链进行结算。
// 采用批次收集 + 定时提交的模式，减少链上交易次数。
//
// ## 消息来源 (Kafka Consumer)
// - Topic: settlements (来自 eidos-trading)
// - 消息类型: model.SettlementTrade
// - 处理流程:
//   1. AddTrade() 接收成交记录
//   2. 达到 batch_size 或 batch_interval 时触发 FlushIfNeeded()
//   3. ProcessPendingBatches() 提交到区块链
//
// ## 消息输出 (Kafka Producer)
// - Topic: settlement-confirmed (发送给 eidos-trading)
// - 消息类型: model.SettlementConfirmation
// - 触发条件: 链上交易确认后回调 onSettlementConfirmed
//
// ## TODO: eidos-trading 对接
// 1. eidos-trading 的清算服务需要发送 settlements 消息
//    - 触发时机: trade-results 处理完成后
//    - 消息格式: model.SettlementTrade (包含 trade_id, maker/taker 信息, 金额等)
//
// 2. eidos-trading 需要订阅 settlement-confirmed 消息
//    - 更新成交状态: MATCHED_OFFCHAIN -> SETTLED_ONCHAIN
//    - 记录 tx_hash, block_number
//
// ## 智能合约对接
// - TODO: 部署 Exchange 合约
// - TODO: 实现 buildSettlementTx() 中的合约调用逻辑
// - 当前 Mock 模式返回模拟 txHash
//
// ========================================
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrBatchNotReady        = errors.New("batch not ready for submission")
	ErrSettlementInProgress = errors.New("settlement already in progress")
	ErrMaxRetriesExceeded   = errors.New("max retries exceeded")
)

// SettlementService 结算服务
type SettlementService struct {
	repo         repository.SettlementRepository
	nonceRepo    repository.NonceRepository
	client       *blockchain.Client
	nonceManager *blockchain.NonceManager

	// 配置
	batchSize     int
	batchInterval time.Duration
	maxRetries    int
	retryBackoff  time.Duration
	chainID       int64

	// 合约地址
	exchangeContract common.Address

	// 批次收集器
	mu            sync.Mutex
	pendingTrades []*model.SettlementTrade
	lastFlushTime time.Time

	// 事件回调
	onSettlementConfirmed func(ctx context.Context, confirmation *model.SettlementConfirmation) error
}

// SettlementServiceConfig 配置
type SettlementServiceConfig struct {
	BatchSize        int
	BatchInterval    time.Duration
	MaxRetries       int
	RetryBackoff     time.Duration
	ChainID          int64
	ExchangeContract common.Address
}

// NewSettlementService 创建结算服务
func NewSettlementService(
	repo repository.SettlementRepository,
	nonceRepo repository.NonceRepository,
	client *blockchain.Client,
	nonceManager *blockchain.NonceManager,
	cfg *SettlementServiceConfig,
) *SettlementService {
	batchSize := cfg.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}

	batchInterval := cfg.BatchInterval
	if batchInterval == 0 {
		batchInterval = 5 * time.Second
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = 30 * time.Second
	}

	return &SettlementService{
		repo:             repo,
		nonceRepo:        nonceRepo,
		client:           client,
		nonceManager:     nonceManager,
		batchSize:        batchSize,
		batchInterval:    batchInterval,
		maxRetries:       maxRetries,
		retryBackoff:     retryBackoff,
		chainID:          cfg.ChainID,
		exchangeContract: cfg.ExchangeContract,
		pendingTrades:    make([]*model.SettlementTrade, 0, batchSize),
		lastFlushTime:    time.Now(),
	}
}

// SetOnSettlementConfirmed 设置结算确认回调
func (s *SettlementService) SetOnSettlementConfirmed(fn func(ctx context.Context, confirmation *model.SettlementConfirmation) error) {
	s.onSettlementConfirmed = fn
}

// AddTrade 添加待结算成交
func (s *SettlementService) AddTrade(ctx context.Context, trade *model.SettlementTrade) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingTrades = append(s.pendingTrades, trade)

	// 检查是否触发批次提交
	if len(s.pendingTrades) >= s.batchSize {
		return s.flushBatch(ctx)
	}

	return nil
}

// FlushIfNeeded 检查并执行定时刷新
func (s *SettlementService) FlushIfNeeded(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pendingTrades) == 0 {
		return nil
	}

	if time.Since(s.lastFlushTime) >= s.batchInterval {
		return s.flushBatch(ctx)
	}

	return nil
}

// flushBatch 刷新批次 (需要持有锁)
func (s *SettlementService) flushBatch(ctx context.Context) error {
	if len(s.pendingTrades) == 0 {
		return nil
	}

	trades := s.pendingTrades
	s.pendingTrades = make([]*model.SettlementTrade, 0, s.batchSize)
	s.lastFlushTime = time.Now()

	// 创建批次
	batch, err := s.createBatch(ctx, trades)
	if err != nil {
		// 回滚：将交易放回队列
		s.pendingTrades = append(trades, s.pendingTrades...)
		return err
	}

	logger.Info("settlement batch created",
		zap.String("batch_id", batch.BatchID),
		zap.Int("trade_count", batch.TradeCount))

	return nil
}

// createBatch 创建结算批次
func (s *SettlementService) createBatch(ctx context.Context, trades []*model.SettlementTrade) (*model.SettlementBatch, error) {
	tradeIDs := make([]string, len(trades))
	for i, t := range trades {
		tradeIDs[i] = t.TradeID
	}

	tradeIDsJSON, err := json.Marshal(tradeIDs)
	if err != nil {
		return nil, err
	}

	batch := &model.SettlementBatch{
		BatchID:    uuid.New().String(),
		TradeCount: len(trades),
		TradeIDs:   string(tradeIDsJSON),
		ChainID:    s.chainID,
		Status:     model.SettlementBatchStatusPending,
	}

	if err := s.repo.Create(ctx, batch); err != nil {
		return nil, err
	}

	return batch, nil
}

// ProcessPendingBatches 处理待提交的批次
func (s *SettlementService) ProcessPendingBatches(ctx context.Context) error {
	batches, err := s.repo.ListPending(ctx, 10)
	if err != nil {
		return err
	}

	for _, batch := range batches {
		if err := s.submitBatch(ctx, batch); err != nil {
			logger.Error("failed to submit settlement batch",
				zap.String("batch_id", batch.BatchID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// submitBatch 提交批次到链上
func (s *SettlementService) submitBatch(ctx context.Context, batch *model.SettlementBatch) error {
	// 检查重试次数
	if batch.RetryCount >= s.maxRetries {
		return s.markBatchFailed(ctx, batch, "max retries exceeded")
	}

	// 获取 nonce
	nonce, err := s.nonceManager.AcquireNonce(ctx)
	if err != nil {
		return err
	}

	// 构建交易
	tx, err := s.buildSettlementTx(ctx, batch, nonce)
	if err != nil {
		s.nonceManager.ReleaseNonce(ctx, nonce)
		return err
	}

	// 签名交易
	signedTx, err := s.client.SignTransaction(tx)
	if err != nil {
		s.nonceManager.ReleaseNonce(ctx, nonce)
		return err
	}

	// 发送交易
	if err := s.client.SendTransaction(ctx, signedTx); err != nil {
		s.nonceManager.ReleaseNonce(ctx, nonce)

		// 处理特定错误
		errStr := err.Error()
		if contains(errStr, "nonce too low") {
			s.nonceManager.HandleNonceTooLow(ctx)
		}
		if contains(errStr, "insufficient funds") {
			return s.markBatchFailed(ctx, batch, "insufficient funds for gas")
		}

		return s.handleSubmitError(ctx, batch, err)
	}

	txHash := signedTx.Hash().Hex()

	// 确认 nonce 使用
	s.nonceManager.ConfirmNonce(ctx, nonce, txHash)

	// 更新状态为已提交
	if err := s.repo.UpdateStatus(ctx, batch.BatchID, model.SettlementBatchStatusSubmitted, txHash, 0); err != nil {
		logger.Error("failed to update batch status",
			zap.String("batch_id", batch.BatchID),
			zap.Error(err))
	}

	// 记录待确认交易
	pendingTx := &model.PendingTx{
		TxHash:        txHash,
		TxType:        model.PendingTxTypeSettlement,
		RefID:         batch.BatchID,
		WalletAddress: s.client.Address().Hex(),
		ChainID:       s.chainID,
		Nonce:         int64(nonce),
		GasPrice:      tx.GasPrice().String(),
		GasLimit:      int64(tx.Gas()),
		SubmittedAt:   time.Now().UnixMilli(),
		TimeoutAt:     time.Now().Add(10 * time.Minute).UnixMilli(),
		Status:        model.PendingTxStatusPending,
	}
	s.nonceRepo.CreatePendingTx(ctx, pendingTx)

	logger.Info("settlement transaction submitted",
		zap.String("batch_id", batch.BatchID),
		zap.String("tx_hash", txHash),
		zap.Uint64("nonce", nonce))

	return nil
}

// buildSettlementTx 构建结算交易
func (s *SettlementService) buildSettlementTx(ctx context.Context, batch *model.SettlementBatch, nonce uint64) (*types.Transaction, error) {
	// TODO: 根据实际合约 ABI 构建交易数据
	// 这里使用占位实现

	tradeIDs, err := batch.GetTradeIDList()
	if err != nil {
		return nil, err
	}

	// 构造合约调用数据
	// Exchange.batchSettle(tradeIds)
	data, err := s.encodeSettlementData(tradeIDs)
	if err != nil {
		return nil, err
	}

	// 获取 gas 价格
	gasPrice, err := s.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	// 估算 gas
	gasLimit := uint64(300000 + uint64(len(tradeIDs))*50000) // 基础 + 每笔交易

	tx := types.NewTransaction(
		nonce,
		s.exchangeContract,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		data,
	)

	return tx, nil
}

// encodeSettlementData 编码结算数据
func (s *SettlementService) encodeSettlementData(tradeIDs []string) ([]byte, error) {
	// TODO: 使用实际的合约 ABI 编码
	// 这里返回占位数据
	return []byte{}, nil
}

// handleSubmitError 处理提交错误
func (s *SettlementService) handleSubmitError(ctx context.Context, batch *model.SettlementBatch, err error) error {
	logger.Warn("settlement submission failed, will retry",
		zap.String("batch_id", batch.BatchID),
		zap.Int("retry_count", batch.RetryCount),
		zap.Error(err))

	return s.repo.UpdateFailed(ctx, batch.BatchID, err.Error())
}

// markBatchFailed 标记批次失败
func (s *SettlementService) markBatchFailed(ctx context.Context, batch *model.SettlementBatch, reason string) error {
	logger.Error("settlement batch failed",
		zap.String("batch_id", batch.BatchID),
		zap.String("reason", reason))

	if err := s.repo.UpdateFailed(ctx, batch.BatchID, reason); err != nil {
		return err
	}

	// 发送失败通知
	if s.onSettlementConfirmed != nil {
		tradeIDs, _ := batch.GetTradeIDList()
		confirmation := &model.SettlementConfirmation{
			BatchID:     batch.BatchID,
			TradeIDs:    tradeIDs,
			Status:      "FAILED",
			Error:       reason,
			ConfirmedAt: time.Now().UnixMilli(),
		}
		s.onSettlementConfirmed(ctx, confirmation)
	}

	return nil
}

// OnTxConfirmed 交易确认回调
func (s *SettlementService) OnTxConfirmed(ctx context.Context, txHash string, blockNumber uint64, gasUsed uint64) error {
	// 查找对应的批次
	pendingTx, err := s.nonceRepo.GetPendingTxByHash(ctx, txHash)
	if err != nil {
		return err
	}

	if pendingTx.TxType != model.PendingTxTypeSettlement {
		return nil
	}

	batchID := pendingTx.RefID

	// 更新批次状态
	if err := s.repo.UpdateStatus(ctx, batchID, model.SettlementBatchStatusConfirmed, txHash, int64(blockNumber)); err != nil {
		return err
	}

	// 更新待确认交易状态
	s.nonceRepo.UpdatePendingTxStatus(ctx, txHash, model.PendingTxStatusConfirmed)
	s.nonceManager.OnTxConfirmed(ctx, uint64(pendingTx.Nonce), txHash)

	// 发送确认通知
	if s.onSettlementConfirmed != nil {
		batch, err := s.repo.GetByBatchID(ctx, batchID)
		if err != nil {
			return err
		}

		tradeIDs, _ := batch.GetTradeIDList()
		confirmation := &model.SettlementConfirmation{
			BatchID:     batchID,
			TradeIDs:    tradeIDs,
			TxHash:      txHash,
			BlockNumber: int64(blockNumber),
			GasUsed:     int64(gasUsed),
			Status:      "CONFIRMED",
			ConfirmedAt: time.Now().UnixMilli(),
		}

		if err := s.onSettlementConfirmed(ctx, confirmation); err != nil {
			logger.Error("failed to send settlement confirmation",
				zap.String("batch_id", batchID),
				zap.Error(err))
		}
	}

	logger.Info("settlement confirmed",
		zap.String("batch_id", batchID),
		zap.String("tx_hash", txHash),
		zap.Uint64("block_number", blockNumber))

	return nil
}

// OnTxFailed 交易失败回调
func (s *SettlementService) OnTxFailed(ctx context.Context, txHash string, reason string) error {
	pendingTx, err := s.nonceRepo.GetPendingTxByHash(ctx, txHash)
	if err != nil {
		return err
	}

	if pendingTx.TxType != model.PendingTxTypeSettlement {
		return nil
	}

	batchID := pendingTx.RefID

	// 更新待确认交易状态
	s.nonceRepo.UpdatePendingTxStatus(ctx, txHash, model.PendingTxStatusFailed)
	s.nonceManager.OnTxFailed(ctx, uint64(pendingTx.Nonce), txHash)

	// 标记批次失败
	return s.markBatchFailed(ctx, &model.SettlementBatch{BatchID: batchID}, reason)
}

// GetBatchStatus 获取批次状态
func (s *SettlementService) GetBatchStatus(ctx context.Context, batchID string) (*model.SettlementBatch, error) {
	return s.repo.GetByBatchID(ctx, batchID)
}

// RetryBatch 重试批次
func (s *SettlementService) RetryBatch(ctx context.Context, batchID string) error {
	batch, err := s.repo.GetByBatchID(ctx, batchID)
	if err != nil {
		return err
	}

	if batch.Status == model.SettlementBatchStatusConfirmed {
		return errors.New("batch already confirmed")
	}

	// 重置状态
	batch.Status = model.SettlementBatchStatusPending
	batch.ErrorMessage = ""
	if err := s.repo.Update(ctx, batch); err != nil {
		return err
	}

	return s.submitBatch(ctx, batch)
}

// GetPendingTradesCount 获取待处理成交数量
func (s *SettlementService) GetPendingTradesCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pendingTrades)
}

// contains 简单的字符串包含检查
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// SplitAndRetryBatch 拆分并重试失败的批次
func (s *SettlementService) SplitAndRetryBatch(ctx context.Context, batchID string) error {
	batch, err := s.repo.GetByBatchID(ctx, batchID)
	if err != nil {
		return err
	}

	if batch.Status != model.SettlementBatchStatusFailed {
		return fmt.Errorf("batch is not in failed status")
	}

	tradeIDs, err := batch.GetTradeIDList()
	if err != nil {
		return err
	}

	if len(tradeIDs) <= 1 {
		return fmt.Errorf("batch cannot be split further")
	}

	// 二分拆分
	mid := len(tradeIDs) / 2
	batch1IDs := tradeIDs[:mid]
	batch2IDs := tradeIDs[mid:]

	// 创建两个新批次
	for _, ids := range [][]string{batch1IDs, batch2IDs} {
		idsJSON, _ := json.Marshal(ids)
		newBatch := &model.SettlementBatch{
			BatchID:    uuid.New().String(),
			TradeCount: len(ids),
			TradeIDs:   string(idsJSON),
			ChainID:    s.chainID,
			Status:     model.SettlementBatchStatusPending,
		}
		if err := s.repo.Create(ctx, newBatch); err != nil {
			return err
		}

		logger.Info("created split batch",
			zap.String("original_batch", batchID),
			zap.String("new_batch", newBatch.BatchID),
			zap.Int("trade_count", newBatch.TradeCount))
	}

	return nil
}
