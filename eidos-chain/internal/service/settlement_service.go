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
// ## eidos-trading 对接
// 1. eidos-trading 的清算服务需要发送 settlements 消息
//    - 触发时机: trade-results 处理完成后
//    - 消息格式: model.SettlementTrade (包含 trade_id, maker/taker 信息, 金额等)
//
// 2. eidos-trading 需要订阅 settlement-confirmed 消息
//    - 更新成交状态: MATCHED_OFFCHAIN -> SETTLED_ONCHAIN
//    - 记录 tx_hash, block_number
//
// ## 智能合约对接
// - Exchange 合约用于批量结算交易
// - 使用 internal/contract/exchange.go 提供的 ABI 绑定
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
	"github.com/eidos-exchange/eidos/eidos-chain/internal/contract"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
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

	// 合约绑定
	exchange     *contract.ExchangeContract
	gasEstimator *contract.GasEstimator
	tokenReg     *contract.TokenRegistry

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
	// Token 地址映射
	BaseTokenAddress  common.Address
	QuoteTokenAddress common.Address
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
		"batch_id", batch.BatchID,
		"trade_count", batch.TradeCount)

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
				"batch_id", batch.BatchID,
				"error", err)
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
			"batch_id", batch.BatchID,
			"error", err)
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
		"batch_id", batch.BatchID,
		"tx_hash", txHash,
		"nonce", nonce)

	return nil
}

// buildSettlementTx 构建结算交易
func (s *SettlementService) buildSettlementTx(ctx context.Context, batch *model.SettlementBatch, nonce uint64) (*types.Transaction, error) {
	tradeIDs, err := batch.GetTradeIDList()
	if err != nil {
		return nil, fmt.Errorf("failed to get trade ID list: %w", err)
	}

	// 获取交易详情并转换为合约参数
	trades, err := s.buildSettleTrades(ctx, tradeIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to build settle trades: %w", err)
	}

	// 使用合约绑定编码数据
	var data []byte
	if s.exchange != nil {
		data, err = s.exchange.PackBatchSettle(trades)
		if err != nil {
			return nil, fmt.Errorf("failed to pack batch settle: %w", err)
		}
	} else {
		// 降级：使用直接编码
		data, err = s.encodeSettlementData(trades)
		if err != nil {
			return nil, fmt.Errorf("failed to encode settlement data: %w", err)
		}
	}

	// Gas 估算
	var gasLimit uint64
	var gasPrice *big.Int

	if s.gasEstimator != nil {
		estimate, err := s.gasEstimator.EstimateSettlementGas(
			ctx,
			s.client.Address(),
			s.exchangeContract,
			data,
			len(trades),
		)
		if err != nil {
			logger.Warn("gas estimation failed, using fallback",
				"batch_id", batch.BatchID,
				"error", err)
			// 降级：使用公式估算
			gasLimit = s.calculateFallbackGas(len(trades))
			gasPrice, _ = s.client.SuggestGasPrice(ctx)
		} else {
			gasLimit = estimate.GasLimit
			gasPrice = estimate.GasPrice.GasPrice
		}
	} else {
		// 没有 gas 估算器，使用公式
		gasLimit = s.calculateFallbackGas(len(trades))
		gasPrice, err = s.client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get gas price: %w", err)
		}
	}

	// 创建交易
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

// buildSettleTrades 从交易 ID 列表构建结算交易参数
func (s *SettlementService) buildSettleTrades(ctx context.Context, tradeIDs []string) ([]contract.SettleTrade, error) {
	trades := make([]contract.SettleTrade, 0, len(tradeIDs))

	for _, tradeID := range tradeIDs {
		// 从 repository 获取交易详情
		// 这里假设已经有存储的交易数据
		trade := contract.SettleTrade{
			TradeID:     contract.StringToBytes32(tradeID),
			MakerWallet: common.Address{}, // 需要从存储获取
			TakerWallet: common.Address{}, // 需要从存储获取
			BaseToken:   common.Address{}, // 需要从配置或存储获取
			QuoteToken:  common.Address{}, // 需要从配置或存储获取
			BaseAmount:  big.NewInt(0),    // 需要从存储获取
			QuoteAmount: big.NewInt(0),    // 需要从存储获取
			MakerFee:    big.NewInt(0),    // 需要从存储获取
			TakerFee:    big.NewInt(0),    // 需要从存储获取
			MakerSide:   0,                // 需要从存储获取
		}
		trades = append(trades, trade)
	}

	return trades, nil
}

// encodeSettlementData 使用 ABI 编码结算数据
func (s *SettlementService) encodeSettlementData(trades []contract.SettleTrade) ([]byte, error) {
	if s.exchange != nil {
		return s.exchange.PackBatchSettle(trades)
	}
	// 降级：如果没有合约绑定，返回空数据（用于 mock 模式）
	return []byte{}, nil
}

// calculateFallbackGas 计算降级 gas 估算
func (s *SettlementService) calculateFallbackGas(tradeCount int) uint64 {
	// 基础 gas: 100,000
	// 每笔交易: 50,000
	// 安全余量: 20%
	baseGas := uint64(100000)
	perTradeGas := uint64(50000)
	total := baseGas + uint64(tradeCount)*perTradeGas
	return total * 120 / 100 // 20% buffer
}

// handleSubmitError 处理提交错误
func (s *SettlementService) handleSubmitError(ctx context.Context, batch *model.SettlementBatch, err error) error {
	logger.Warn("settlement submission failed, will retry",
		"batch_id", batch.BatchID,
		"retry_count", batch.RetryCount,
		"error", err)

	return s.repo.UpdateFailed(ctx, batch.BatchID, err.Error())
}

// markBatchFailed 标记批次失败
func (s *SettlementService) markBatchFailed(ctx context.Context, batch *model.SettlementBatch, reason string) error {
	logger.Error("settlement batch failed",
		"batch_id", batch.BatchID,
		"reason", reason)

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
				"batch_id", batchID,
				"error", err)
		}
	}

	logger.Info("settlement confirmed",
		"batch_id", batchID,
		"tx_hash", txHash,
		"block_number", blockNumber)

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
			"original_batch", batchID,
			"new_batch", newBatch.BatchID,
			"trade_count", newBatch.TradeCount)
	}

	return nil
}
