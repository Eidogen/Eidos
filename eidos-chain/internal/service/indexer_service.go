// ========================================
// IndexerService 索引服务对接说明
// ========================================
//
// ## 功能概述
// IndexerService 监听区块链事件，检测用户充值。
// 采用轮询模式从上次检查点开始扫描区块，解析 Deposit 事件。
//
// ## 消息输出 (Kafka Producer)
// - Topic: deposits (发送给 eidos-trading)
// - 消息类型: model.DepositEvent
// - 触发条件: 检测到链上 Deposit 事件后回调 onDepositDetected
//
// ## TODO: eidos-trading 对接
// 1. eidos-trading 需要订阅 deposits 消息
//    - 接收充值通知
//    - 调用 DepositService.CreditDeposit() 入账
//    - 更新用户余额 (增加 available_balance)
//
// ## TODO: 智能合约对接
// - 部署 Vault 合约并获取合约地址
// - 更新 vaultContractAddress 配置
// - depositEventTopic 是 Deposit(address,address,uint256) 事件签名
// - 当前 Mock 模式使用 0x0 地址，不会检测到真实事件
//
// ## 检查点机制
// - 每 checkpointInterval 个区块保存一次检查点
// - 服务重启后从上次检查点继续扫描
// - 防止重复处理事件
//
// ## Arbitrum 特性
// - requiredConfirms = 0 (Arbitrum 快速确认)
// - 可根据安全要求调整确认数
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
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	ErrIndexerAlreadyRunning = errors.New("indexer already running")
	ErrIndexerNotRunning     = errors.New("indexer not running")
)

// IndexerService 链上索引服务
type IndexerService struct {
	client         *blockchain.Client
	checkpointRepo repository.CheckpointRepository
	depositRepo    repository.DepositRepository

	// 配置
	chainID              int64
	pollInterval         time.Duration
	checkpointInterval   int64  // 每多少个区块保存检查点
	requiredConfirms     int    // 需要的确认数
	vaultContractAddress common.Address

	// 运行状态
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	currentBlock uint64

	// 事件主题
	depositEventTopic common.Hash

	// 事件回调
	onDepositDetected func(ctx context.Context, deposit *model.DepositEvent) error
}

// IndexerServiceConfig 配置
type IndexerServiceConfig struct {
	ChainID              int64
	PollInterval         time.Duration
	CheckpointInterval   int64
	RequiredConfirms     int
	VaultContractAddress common.Address
	DepositEventTopic    common.Hash // Deposit(address,address,uint256) 事件签名
}

// NewIndexerService 创建索引服务
func NewIndexerService(
	client *blockchain.Client,
	checkpointRepo repository.CheckpointRepository,
	depositRepo repository.DepositRepository,
	cfg *IndexerServiceConfig,
) *IndexerService {
	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = time.Second
	}

	checkpointInterval := cfg.CheckpointInterval
	if checkpointInterval == 0 {
		checkpointInterval = 10
	}

	return &IndexerService{
		client:               client,
		checkpointRepo:       checkpointRepo,
		depositRepo:          depositRepo,
		chainID:              cfg.ChainID,
		pollInterval:         pollInterval,
		checkpointInterval:   checkpointInterval,
		requiredConfirms:     cfg.RequiredConfirms,
		vaultContractAddress: cfg.VaultContractAddress,
		depositEventTopic:    cfg.DepositEventTopic,
	}
}

// SetOnDepositDetected 设置充值检测回调
func (s *IndexerService) SetOnDepositDetected(fn func(ctx context.Context, deposit *model.DepositEvent) error) {
	s.onDepositDetected = fn
}

// Start 启动索引服务
func (s *IndexerService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrIndexerAlreadyRunning
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	// 获取起始区块
	startBlock, err := s.getStartBlock(ctx)
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	logger.Info("indexer starting",
		zap.Int64("chain_id", s.chainID),
		zap.Uint64("start_block", startBlock))

	go s.runLoop(ctx, startBlock)

	return nil
}

// Stop 停止索引服务
func (s *IndexerService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return ErrIndexerNotRunning
	}

	close(s.stopCh)
	s.running = false

	logger.Info("indexer stopped", zap.Int64("chain_id", s.chainID))

	return nil
}

// IsRunning 检查是否运行中
func (s *IndexerService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetCurrentBlock 获取当前处理的区块
func (s *IndexerService) GetCurrentBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlock
}

// getStartBlock 获取起始区块
func (s *IndexerService) getStartBlock(ctx context.Context) (uint64, error) {
	checkpoint, err := s.checkpointRepo.GetByChainID(ctx, s.chainID)
	if err == nil {
		return uint64(checkpoint.BlockNumber + 1), nil
	}

	if errors.Is(err, repository.ErrCheckpointNotFound) {
		// 从当前区块开始
		currentBlock, err := s.client.BlockNumber(ctx)
		if err != nil {
			return 0, err
		}
		return currentBlock, nil
	}

	return 0, err
}

// runLoop 主循环
func (s *IndexerService) runLoop(ctx context.Context, startBlock uint64) {
	currentBlock := startBlock
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			latestBlock, err := s.client.BlockNumber(ctx)
			if err != nil {
				logger.Error("failed to get latest block", zap.Error(err))
				continue
			}

			// 处理所有新区块
			for currentBlock <= latestBlock {
				select {
				case <-s.stopCh:
					return
				case <-ctx.Done():
					return
				default:
				}

				if err := s.processBlock(ctx, currentBlock); err != nil {
					logger.Error("failed to process block",
						zap.Uint64("block", currentBlock),
						zap.Error(err))
					break
				}

				// 更新当前区块
				s.mu.Lock()
				s.currentBlock = currentBlock
				s.mu.Unlock()

				// 定期保存检查点
				if currentBlock%uint64(s.checkpointInterval) == 0 {
					s.saveCheckpoint(ctx, currentBlock)
				}

				currentBlock++
			}
		}
	}
}

// processBlock 处理单个区块
func (s *IndexerService) processBlock(ctx context.Context, blockNumber uint64) error {
	// 获取区块
	block, err := s.client.GetBlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		return err
	}

	// 过滤 Vault 合约的 Deposit 事件
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(blockNumber)),
		ToBlock:   big.NewInt(int64(blockNumber)),
		Addresses: []common.Address{s.vaultContractAddress},
		Topics:    [][]common.Hash{{s.depositEventTopic}},
	}

	logs, err := s.client.FilterLogs(ctx, query)
	if err != nil {
		return err
	}

	// 处理每个事件
	for _, log := range logs {
		if err := s.processDepositLog(ctx, log, block.Hash().Hex()); err != nil {
			logger.Error("failed to process deposit log",
				zap.String("tx_hash", log.TxHash.Hex()),
				zap.Uint("log_index", log.Index),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// processDepositLog 处理充值日志
func (s *IndexerService) processDepositLog(ctx context.Context, log types.Log, blockHash string) error {
	// 检查是否已处理 (幂等)
	exists, err := s.depositRepo.ExistsByTxHash(ctx, log.TxHash.Hex())
	if err != nil {
		return err
	}
	if exists {
		return nil // 已处理
	}

	// 解析事件数据
	deposit, err := s.parseDepositEvent(log)
	if err != nil {
		return err
	}

	// 创建充值记录
	record := &model.DepositRecord{
		DepositID:             uuid.New().String(),
		WalletAddress:         deposit.Wallet,
		Token:                 deposit.Token,
		TokenAddress:          deposit.TokenAddress,
		Amount:                deposit.Amount,
		ChainID:               s.chainID,
		TxHash:                log.TxHash.Hex(),
		BlockNumber:           int64(log.BlockNumber),
		BlockHash:             blockHash,
		LogIndex:              int(log.Index),
		Confirmations:         0,
		RequiredConfirmations: s.requiredConfirms,
		Status:                model.DepositRecordStatusPending,
	}

	if err := s.depositRepo.Create(ctx, record); err != nil {
		if errors.Is(err, repository.ErrDuplicateDeposit) {
			return nil // 重复，忽略
		}
		return err
	}

	logger.Info("deposit detected",
		zap.String("deposit_id", record.DepositID),
		zap.String("wallet", deposit.Wallet),
		zap.String("token", deposit.Token),
		zap.String("amount", deposit.Amount.String()),
		zap.String("tx_hash", log.TxHash.Hex()))

	// Arbitrum 0 确认，直接处理
	if s.requiredConfirms == 0 {
		return s.processConfirmedDeposit(ctx, record)
	}

	return nil
}

// parseDepositEvent 解析充值事件
func (s *IndexerService) parseDepositEvent(log types.Log) (*model.DepositEvent, error) {
	// TODO: 根据实际合约 ABI 解析
	// 假设事件格式: Deposit(address indexed user, address indexed token, uint256 amount)

	if len(log.Topics) < 3 {
		return nil, fmt.Errorf("invalid deposit event: not enough topics")
	}

	wallet := common.HexToAddress(log.Topics[1].Hex()).Hex()
	tokenAddress := common.HexToAddress(log.Topics[2].Hex()).Hex()

	// 解析 amount 从 data
	amount := new(big.Int)
	if len(log.Data) >= 32 {
		amount.SetBytes(log.Data[:32])
	}

	return &model.DepositEvent{
		DepositID:    uuid.New().String(),
		Wallet:       wallet,
		Token:        s.getTokenSymbol(tokenAddress), // TODO: 实现 token 符号映射
		TokenAddress: tokenAddress,
		Amount:       decimal.NewFromBigInt(amount, -18), // 假设 18 位精度
		TxHash:       log.TxHash.Hex(),
		BlockNumber:  int64(log.BlockNumber),
		LogIndex:     int(log.Index),
		DetectedAt:   time.Now().UnixMilli(),
	}, nil
}

// getTokenSymbol 获取 token 符号
func (s *IndexerService) getTokenSymbol(tokenAddress string) string {
	// TODO: 实现 token 符号映射表
	return "USDC"
}

// processConfirmedDeposit 处理已确认的充值
func (s *IndexerService) processConfirmedDeposit(ctx context.Context, record *model.DepositRecord) error {
	// 更新状态为已确认
	if err := s.depositRepo.UpdateStatus(ctx, record.DepositID, model.DepositRecordStatusConfirmed); err != nil {
		return err
	}

	// 发送充值事件
	if s.onDepositDetected != nil {
		deposit := &model.DepositEvent{
			DepositID:    record.DepositID,
			Wallet:       record.WalletAddress,
			Token:        record.Token,
			TokenAddress: record.TokenAddress,
			Amount:       record.Amount,
			TxHash:       record.TxHash,
			BlockNumber:  record.BlockNumber,
			LogIndex:     record.LogIndex,
			DetectedAt:   time.Now().UnixMilli(),
		}

		if err := s.onDepositDetected(ctx, deposit); err != nil {
			logger.Error("failed to send deposit event",
				zap.String("deposit_id", record.DepositID),
				zap.Error(err))
			return err
		}
	}

	// 更新状态为已入账
	return s.depositRepo.UpdateStatus(ctx, record.DepositID, model.DepositRecordStatusCredited)
}

// saveCheckpoint 保存检查点
func (s *IndexerService) saveCheckpoint(ctx context.Context, blockNumber uint64) {
	block, err := s.client.GetBlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		logger.Error("failed to get block for checkpoint", zap.Error(err))
		return
	}

	checkpoint := &model.BlockCheckpoint{
		ChainID:     s.chainID,
		BlockNumber: int64(blockNumber),
		BlockHash:   block.Hash().Hex(),
	}

	if err := s.checkpointRepo.Upsert(ctx, checkpoint); err != nil {
		logger.Error("failed to save checkpoint", zap.Error(err))
		return
	}

	logger.Debug("checkpoint saved",
		zap.Int64("chain_id", s.chainID),
		zap.Uint64("block", blockNumber))
}

// UpdateConfirmations 更新待确认充值的确认数
func (s *IndexerService) UpdateConfirmations(ctx context.Context) error {
	records, err := s.depositRepo.ListPendingConfirmation(ctx, 100)
	if err != nil {
		return err
	}

	currentBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		return err
	}

	for _, record := range records {
		confirmations := int(currentBlock) - int(record.BlockNumber)
		if confirmations < 0 {
			confirmations = 0
		}

		if err := s.depositRepo.UpdateConfirmations(ctx, record.DepositID, confirmations); err != nil {
			logger.Error("failed to update confirmations",
				zap.String("deposit_id", record.DepositID),
				zap.Error(err))
			continue
		}

		// 检查是否达到确认数
		if confirmations >= record.RequiredConfirmations && record.Status == model.DepositRecordStatusPending {
			if err := s.processConfirmedDeposit(ctx, record); err != nil {
				logger.Error("failed to process confirmed deposit",
					zap.String("deposit_id", record.DepositID),
					zap.Error(err))
			}
		}
	}

	return nil
}

// GetDepositStatus 获取充值状态
func (s *IndexerService) GetDepositStatus(ctx context.Context, depositID string) (*model.DepositRecord, error) {
	return s.depositRepo.GetByDepositID(ctx, depositID)
}

// GetDepositByTxHash 通过交易哈希获取充值
func (s *IndexerService) GetDepositByTxHash(ctx context.Context, txHash string, logIndex int) (*model.DepositRecord, error) {
	return s.depositRepo.GetByTxHashAndLogIndex(ctx, txHash, logIndex)
}

// GetIndexerStatus 获取索引器状态
func (s *IndexerService) GetIndexerStatus(ctx context.Context) (*IndexerStatus, error) {
	latestBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	currentBlock := s.GetCurrentBlock()
	lag := int64(latestBlock) - int64(currentBlock)
	if lag < 0 {
		lag = 0
	}

	checkpoint, _ := s.checkpointRepo.GetByChainID(ctx, s.chainID)

	return &IndexerStatus{
		ChainID:        s.chainID,
		Running:        s.IsRunning(),
		CurrentBlock:   currentBlock,
		LatestBlock:    latestBlock,
		LagBlocks:      lag,
		CheckpointBlock: func() int64 {
			if checkpoint != nil {
				return checkpoint.BlockNumber
			}
			return 0
		}(),
	}, nil
}

// IndexerStatus 索引器状态
type IndexerStatus struct {
	ChainID         int64  `json:"chain_id"`
	Running         bool   `json:"running"`
	CurrentBlock    uint64 `json:"current_block"`
	LatestBlock     uint64 `json:"latest_block"`
	LagBlocks       int64  `json:"lag_blocks"`
	CheckpointBlock int64  `json:"checkpoint_block"`
}

// RecordChainEvent 记录链上事件
func (s *IndexerService) RecordChainEvent(ctx context.Context, log types.Log, eventType model.ChainEventType) error {
	eventData, _ := json.Marshal(map[string]interface{}{
		"topics": log.Topics,
		"data":   common.Bytes2Hex(log.Data),
	})

	event := &model.ChainEvent{
		ChainID:     s.chainID,
		BlockNumber: int64(log.BlockNumber),
		TxHash:      log.TxHash.Hex(),
		LogIndex:    int(log.Index),
		EventType:   eventType,
		EventData:   string(eventData),
		Processed:   false,
	}

	return s.checkpointRepo.CreateEvent(ctx, event)
}
