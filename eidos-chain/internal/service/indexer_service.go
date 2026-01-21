// ========================================
// IndexerService 索引服务对接说明
// ========================================
//
// ## 功能概述
// IndexerService 监听区块链事件，检测用户充值、提现和结算。
// 采用轮询模式从上次检查点开始扫描区块，解析各类事件。
//
// ## 消息输出 (Kafka Producer)
// - Topic: deposits (发送给 eidos-trading)
// - Topic: settlement-events (发送给 eidos-trading)
// - Topic: withdrawal-events (发送给 eidos-trading)
//
// ## 支持的事件类型
// - Deposit: 用户充值到 Vault 合约
// - Withdraw: 用户提现完成事件
// - SettlementExecuted: 结算批次执行完成
//
// ## 检查点机制
// - 每 checkpointInterval 个区块保存一次检查点
// - 服务重启后从上次检查点继续扫描（断点续扫）
// - 防止重复处理事件（幂等性）
//
// ## 事件幂等处理
// - 使用 tx_hash + log_index 作为唯一标识
// - 先检查数据库是否已存在，再处理事件
// - 防止区块链重组或服务重启导致的重复处理
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
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	withdrawalRepo repository.WithdrawalRepository
	settlementRepo repository.SettlementRepository

	// 合约绑定
	vaultContract    *contract.VaultContract
	exchangeContract *contract.ExchangeContract
	tokenRegistry    *contract.TokenRegistry

	// 配置
	chainID                 int64
	pollInterval            time.Duration
	checkpointInterval      int64 // 每多少个区块保存检查点
	requiredConfirms        int   // 需要的确认数
	vaultContractAddress    common.Address
	exchangeContractAddress common.Address
	maxBlocksPerScan        int64 // 每次扫描的最大区块数

	// 运行状态
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	currentBlock uint64

	// 事件主题
	depositEventTopic    common.Hash
	withdrawEventTopic   common.Hash
	settlementEventTopic common.Hash

	// 事件回调
	onDepositDetected    func(ctx context.Context, deposit *model.DepositEvent) error
	onWithdrawDetected   func(ctx context.Context, withdraw *WithdrawEventData) error
	onSettlementDetected func(ctx context.Context, settlement *SettlementEventData) error
}

// WithdrawEventData 提现事件数据
type WithdrawEventData struct {
	WithdrawID  string          `json:"withdraw_id"`
	User        string          `json:"user"`
	Token       string          `json:"token"`
	Amount      decimal.Decimal `json:"amount"`
	ToAddress   string          `json:"to_address"`
	Nonce       int64           `json:"nonce"`
	TxHash      string          `json:"tx_hash"`
	BlockNumber int64           `json:"block_number"`
	LogIndex    int             `json:"log_index"`
	DetectedAt  int64           `json:"detected_at"`
}

// SettlementEventData 结算事件数据
type SettlementEventData struct {
	BatchID     string   `json:"batch_id"`
	TradeCount  int      `json:"trade_count"`
	TradeIDs    []string `json:"trade_ids,omitempty"`
	TxHash      string   `json:"tx_hash"`
	BlockNumber int64    `json:"block_number"`
	LogIndex    int      `json:"log_index"`
	Timestamp   int64    `json:"timestamp"`
	DetectedAt  int64    `json:"detected_at"`
}

// IndexerServiceConfig 配置
type IndexerServiceConfig struct {
	ChainID                 int64
	PollInterval            time.Duration
	CheckpointInterval      int64
	RequiredConfirms        int
	VaultContractAddress    common.Address
	ExchangeContractAddress common.Address
	DepositEventTopic       common.Hash
	WithdrawEventTopic      common.Hash
	SettlementEventTopic    common.Hash
	MaxBlocksPerScan        int64
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

	maxBlocksPerScan := cfg.MaxBlocksPerScan
	if maxBlocksPerScan == 0 {
		maxBlocksPerScan = 1000 // 默认每次扫描最多1000个区块
	}

	return &IndexerService{
		client:                  client,
		checkpointRepo:          checkpointRepo,
		depositRepo:             depositRepo,
		chainID:                 cfg.ChainID,
		pollInterval:            pollInterval,
		checkpointInterval:      checkpointInterval,
		requiredConfirms:        cfg.RequiredConfirms,
		vaultContractAddress:    cfg.VaultContractAddress,
		exchangeContractAddress: cfg.ExchangeContractAddress,
		depositEventTopic:       cfg.DepositEventTopic,
		withdrawEventTopic:      cfg.WithdrawEventTopic,
		settlementEventTopic:    cfg.SettlementEventTopic,
		maxBlocksPerScan:        maxBlocksPerScan,
	}
}

// SetWithdrawalRepository 设置提现仓储
func (s *IndexerService) SetWithdrawalRepository(repo repository.WithdrawalRepository) {
	s.withdrawalRepo = repo
}

// SetSettlementRepository 设置结算仓储
func (s *IndexerService) SetSettlementRepository(repo repository.SettlementRepository) {
	s.settlementRepo = repo
}

// SetVaultContract 设置 Vault 合约绑定
func (s *IndexerService) SetVaultContract(vault *contract.VaultContract) {
	s.vaultContract = vault
	s.depositEventTopic = vault.DepositEventTopic()
	s.withdrawEventTopic = vault.WithdrawEventTopic()
}

// SetExchangeContract 设置 Exchange 合约绑定
func (s *IndexerService) SetExchangeContract(exchange *contract.ExchangeContract) {
	s.exchangeContract = exchange
	s.settlementEventTopic = exchange.SettlementExecutedEventTopic()
}

// SetTokenRegistry 设置 Token 注册表
func (s *IndexerService) SetTokenRegistry(registry *contract.TokenRegistry) {
	s.tokenRegistry = registry
}

// SetOnDepositDetected 设置充值检测回调
func (s *IndexerService) SetOnDepositDetected(fn func(ctx context.Context, deposit *model.DepositEvent) error) {
	s.onDepositDetected = fn
}

// SetOnWithdrawDetected 设置提现检测回调
func (s *IndexerService) SetOnWithdrawDetected(fn func(ctx context.Context, withdraw *WithdrawEventData) error) {
	s.onWithdrawDetected = fn
}

// SetOnSettlementDetected 设置结算检测回调
func (s *IndexerService) SetOnSettlementDetected(fn func(ctx context.Context, settlement *SettlementEventData) error) {
	s.onSettlementDetected = fn
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
		"chain_id", s.chainID,
		"start_block", startBlock)

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

	logger.Info("indexer stopped", "chain_id", s.chainID)

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

// getStartBlock 获取起始区块（断点续扫）
func (s *IndexerService) getStartBlock(ctx context.Context) (uint64, error) {
	// 优先从检查点恢复
	checkpoint, err := s.checkpointRepo.GetByChainID(ctx, s.chainID)
	if err == nil && checkpoint != nil {
		logger.Info("resuming from checkpoint",
			"chain_id", s.chainID,
			"checkpoint_block", checkpoint.BlockNumber,
			"checkpoint_hash", checkpoint.BlockHash)
		return uint64(checkpoint.BlockNumber + 1), nil
	}

	if errors.Is(err, repository.ErrCheckpointNotFound) {
		// 没有检查点，从当前区块开始
		currentBlock, err := s.client.BlockNumber(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get current block: %w", err)
		}
		logger.Info("no checkpoint found, starting from current block",
			"chain_id", s.chainID,
			"current_block", currentBlock)
		return currentBlock, nil
	}

	return 0, fmt.Errorf("failed to get checkpoint: %w", err)
}

// runLoop 主循环
func (s *IndexerService) runLoop(ctx context.Context, startBlock uint64) {
	currentBlock := startBlock
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			// 停止前保存检查点
			s.saveCheckpoint(ctx, currentBlock-1)
			return
		case <-ctx.Done():
			s.saveCheckpoint(ctx, currentBlock-1)
			return
		case <-ticker.C:
			latestBlock, err := s.client.BlockNumber(ctx)
			if err != nil {
				logger.Error("failed to get latest block", "error", err)
				continue
			}

			// 限制每次扫描的区块数，防止卡顿
			endBlock := currentBlock + uint64(s.maxBlocksPerScan)
			if endBlock > latestBlock {
				endBlock = latestBlock
			}

			// 批量处理区块
			if currentBlock <= endBlock {
				if err := s.processBlockRange(ctx, currentBlock, endBlock); err != nil {
					logger.Error("failed to process block range",
						"from", currentBlock,
						"to", endBlock,
						"error", err)
					continue
				}

				// 更新当前区块
				s.mu.Lock()
				s.currentBlock = endBlock
				s.mu.Unlock()

				// 保存检查点
				if endBlock > currentBlock && endBlock%uint64(s.checkpointInterval) == 0 {
					s.saveCheckpoint(ctx, endBlock)
				}

				currentBlock = endBlock + 1
			}
		}
	}
}

// processBlockRange 批量处理区块范围
func (s *IndexerService) processBlockRange(ctx context.Context, fromBlock, toBlock uint64) error {
	// 构建多地址过滤查询
	addresses := []common.Address{}
	topics := [][]common.Hash{nil} // 第一个 topic 列表为所有事件类型

	// 添加合约地址
	if s.vaultContractAddress != (common.Address{}) {
		addresses = append(addresses, s.vaultContractAddress)
	}
	if s.exchangeContractAddress != (common.Address{}) {
		addresses = append(addresses, s.exchangeContractAddress)
	}

	if len(addresses) == 0 {
		return nil // 没有配置合约地址
	}

	// 收集所有事件 topic
	eventTopics := []common.Hash{}
	if s.depositEventTopic != (common.Hash{}) {
		eventTopics = append(eventTopics, s.depositEventTopic)
	}
	if s.withdrawEventTopic != (common.Hash{}) {
		eventTopics = append(eventTopics, s.withdrawEventTopic)
	}
	if s.settlementEventTopic != (common.Hash{}) {
		eventTopics = append(eventTopics, s.settlementEventTopic)
	}

	if len(eventTopics) > 0 {
		topics[0] = eventTopics
	}

	// 过滤日志
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(toBlock)),
		Addresses: addresses,
		Topics:    topics,
	}

	logs, err := s.client.FilterLogs(ctx, query)
	if err != nil {
		return fmt.Errorf("filter logs: %w", err)
	}

	// 处理每个日志
	for _, log := range logs {
		select {
		case <-s.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := s.processLog(ctx, log); err != nil {
			logger.Error("failed to process log",
				"tx_hash", log.TxHash.Hex(),
				"log_index", log.Index,
				"error", err)
			// 继续处理其他日志
		}
	}

	return nil
}

// processLog 处理单个日志
func (s *IndexerService) processLog(ctx context.Context, log types.Log) error {
	if len(log.Topics) == 0 {
		return nil
	}

	topic := log.Topics[0]

	// 检查是否已处理（幂等性）
	exists, err := s.isEventProcessed(ctx, log.TxHash.Hex(), int(log.Index))
	if err != nil {
		return fmt.Errorf("check event processed: %w", err)
	}
	if exists {
		return nil // 已处理，跳过
	}

	// 根据 topic 分发处理
	switch topic {
	case s.depositEventTopic:
		return s.processDepositLog(ctx, log)
	case s.withdrawEventTopic:
		return s.processWithdrawLog(ctx, log)
	case s.settlementEventTopic:
		return s.processSettlementLog(ctx, log)
	default:
		// 未知事件，记录到 chain_events 表
		return s.recordUnknownEvent(ctx, log)
	}
}

// isEventProcessed 检查事件是否已处理
func (s *IndexerService) isEventProcessed(ctx context.Context, txHash string, logIndex int) (bool, error) {
	_, err := s.checkpointRepo.GetEventByTxHashAndLogIndex(ctx, txHash, logIndex)
	if err == nil {
		return true, nil // 已存在
	}
	if errors.Is(err, repository.ErrEventNotFound) {
		return false, nil // 不存在
	}
	return false, err
}

// processDepositLog 处理充值日志
func (s *IndexerService) processDepositLog(ctx context.Context, log types.Log) error {
	// 检查是否已处理
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
		return fmt.Errorf("parse deposit event: %w", err)
	}

	// 获取区块哈希
	blockHash := ""
	block, err := s.client.GetBlockByNumber(ctx, big.NewInt(int64(log.BlockNumber)))
	if err == nil {
		blockHash = block.Hash().Hex()
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

	// 记录事件
	s.recordChainEvent(ctx, log, model.ChainEventTypeDeposit)

	logger.Info("deposit detected",
		"deposit_id", record.DepositID,
		"wallet", deposit.Wallet,
		"token", deposit.Token,
		"amount", deposit.Amount.String(),
		"tx_hash", log.TxHash.Hex())

	// Arbitrum 0 确认，直接处理
	if s.requiredConfirms == 0 {
		return s.processConfirmedDeposit(ctx, record)
	}

	return nil
}

// processWithdrawLog 处理提现日志
func (s *IndexerService) processWithdrawLog(ctx context.Context, log types.Log) error {
	// 解析事件
	var withdrawEvent *contract.WithdrawEvent
	var err error

	if s.vaultContract != nil {
		withdrawEvent, err = s.vaultContract.ParseWithdraw(log)
		if err != nil {
			return fmt.Errorf("parse withdraw event: %w", err)
		}
	} else {
		// 手动解析
		if len(log.Topics) < 3 {
			return fmt.Errorf("invalid withdraw event: not enough topics")
		}
		withdrawEvent = &contract.WithdrawEvent{
			User:  common.HexToAddress(log.Topics[1].Hex()),
			Token: common.HexToAddress(log.Topics[2].Hex()),
			Raw:   log,
		}
		if len(log.Data) >= 96 {
			withdrawEvent.Amount = new(big.Int).SetBytes(log.Data[:32])
			withdrawEvent.To = common.BytesToAddress(log.Data[32:64])
			withdrawEvent.Nonce = new(big.Int).SetBytes(log.Data[64:96])
		}
	}

	// 获取 token 符号
	tokenSymbol := s.getTokenSymbol(withdrawEvent.Token.Hex())

	// 获取 decimals
	var decimals uint8 = 18
	if s.tokenRegistry != nil {
		info, err := s.tokenRegistry.GetByAddress(withdrawEvent.Token)
		if err == nil {
			decimals = info.Decimals
			tokenSymbol = info.Symbol
		}
	}

	amount := decimal.NewFromBigInt(withdrawEvent.Amount, -int32(decimals))

	// 记录事件
	s.recordChainEvent(ctx, log, model.ChainEventTypeWithdraw)

	logger.Info("withdraw event detected",
		"user", withdrawEvent.User.Hex(),
		"token", tokenSymbol,
		"amount", amount.String(),
		"to", withdrawEvent.To.Hex(),
		"tx_hash", log.TxHash.Hex())

	// 回调通知
	if s.onWithdrawDetected != nil {
		data := &WithdrawEventData{
			User:        withdrawEvent.User.Hex(),
			Token:       tokenSymbol,
			Amount:      amount,
			ToAddress:   withdrawEvent.To.Hex(),
			Nonce:       withdrawEvent.Nonce.Int64(),
			TxHash:      log.TxHash.Hex(),
			BlockNumber: int64(log.BlockNumber),
			LogIndex:    int(log.Index),
			DetectedAt:  time.Now().UnixMilli(),
		}
		if err := s.onWithdrawDetected(ctx, data); err != nil {
			logger.Error("withdraw callback failed", "error", err)
		}
	}

	return nil
}

// processSettlementLog 处理结算日志
func (s *IndexerService) processSettlementLog(ctx context.Context, log types.Log) error {
	// 解析事件
	var settlementEvent *contract.SettlementExecutedEvent
	var err error

	if s.exchangeContract != nil {
		settlementEvent, err = s.exchangeContract.ParseSettlementExecuted(log)
		if err != nil {
			return fmt.Errorf("parse settlement event: %w", err)
		}
	} else {
		// 手动解析
		if len(log.Topics) < 2 {
			return fmt.Errorf("invalid settlement event: not enough topics")
		}
		settlementEvent = &contract.SettlementExecutedEvent{
			Raw: log,
		}
		copy(settlementEvent.BatchID[:], log.Topics[1].Bytes())
		if len(log.Data) >= 64 {
			settlementEvent.TradeCount = new(big.Int).SetBytes(log.Data[:32])
			settlementEvent.Timestamp = new(big.Int).SetBytes(log.Data[32:64])
		}
	}

	batchID := contract.Bytes32ToString(settlementEvent.BatchID)

	// 记录事件
	s.recordChainEvent(ctx, log, model.ChainEventTypeSettlement)

	logger.Info("settlement event detected",
		"batch_id", batchID,
		"trade_count", settlementEvent.TradeCount.Int64(),
		"tx_hash", log.TxHash.Hex())

	// 回调通知
	if s.onSettlementDetected != nil {
		data := &SettlementEventData{
			BatchID:     batchID,
			TradeCount:  int(settlementEvent.TradeCount.Int64()),
			TxHash:      log.TxHash.Hex(),
			BlockNumber: int64(log.BlockNumber),
			LogIndex:    int(log.Index),
			Timestamp:   settlementEvent.Timestamp.Int64(),
			DetectedAt:  time.Now().UnixMilli(),
		}
		if err := s.onSettlementDetected(ctx, data); err != nil {
			logger.Error("settlement callback failed", "error", err)
		}
	}

	return nil
}

// recordUnknownEvent 记录未知事件
func (s *IndexerService) recordUnknownEvent(ctx context.Context, log types.Log) error {
	eventData, _ := json.Marshal(map[string]interface{}{
		"topics": log.Topics,
		"data":   common.Bytes2Hex(log.Data),
	})

	event := &model.ChainEvent{
		ChainID:     s.chainID,
		BlockNumber: int64(log.BlockNumber),
		TxHash:      log.TxHash.Hex(),
		LogIndex:    int(log.Index),
		EventType:   model.ChainEventType("Unknown"),
		EventData:   string(eventData),
		Processed:   false,
	}

	return s.checkpointRepo.CreateEvent(ctx, event)
}

// recordChainEvent 记录链上事件
func (s *IndexerService) recordChainEvent(ctx context.Context, log types.Log, eventType model.ChainEventType) {
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
		Processed:   true,
	}

	if err := s.checkpointRepo.CreateEvent(ctx, event); err != nil {
		logger.Error("failed to record chain event",
			"tx_hash", log.TxHash.Hex(),
			"error", err)
	}
}

// parseDepositEvent 解析充值事件
func (s *IndexerService) parseDepositEvent(log types.Log) (*model.DepositEvent, error) {
	// 使用合约绑定解析
	if s.vaultContract != nil {
		depositEvent, err := s.vaultContract.ParseDeposit(log)
		if err != nil {
			return nil, err
		}

		tokenSymbol := s.getTokenSymbol(depositEvent.Token.Hex())
		var decimals uint8 = 18
		if s.tokenRegistry != nil {
			info, err := s.tokenRegistry.GetByAddress(depositEvent.Token)
			if err == nil {
				decimals = info.Decimals
				tokenSymbol = info.Symbol
			}
		}

		return &model.DepositEvent{
			DepositID:    uuid.New().String(),
			Wallet:       depositEvent.User.Hex(),
			Token:        tokenSymbol,
			TokenAddress: depositEvent.Token.Hex(),
			Amount:       decimal.NewFromBigInt(depositEvent.Amount, -int32(decimals)),
			TxHash:       log.TxHash.Hex(),
			BlockNumber:  int64(log.BlockNumber),
			LogIndex:     int(log.Index),
			DetectedAt:   time.Now().UnixMilli(),
		}, nil
	}

	// 手动解析
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

	tokenSymbol := s.getTokenSymbol(tokenAddress)
	var decimals uint8 = 18
	if s.tokenRegistry != nil {
		info, err := s.tokenRegistry.GetByAddress(common.HexToAddress(tokenAddress))
		if err == nil {
			decimals = info.Decimals
			tokenSymbol = info.Symbol
		}
	}

	return &model.DepositEvent{
		DepositID:    uuid.New().String(),
		Wallet:       wallet,
		Token:        tokenSymbol,
		TokenAddress: tokenAddress,
		Amount:       decimal.NewFromBigInt(amount, -int32(decimals)),
		TxHash:       log.TxHash.Hex(),
		BlockNumber:  int64(log.BlockNumber),
		LogIndex:     int(log.Index),
		DetectedAt:   time.Now().UnixMilli(),
	}, nil
}

// getTokenSymbol 获取 token 符号
func (s *IndexerService) getTokenSymbol(tokenAddress string) string {
	if s.tokenRegistry != nil {
		info, err := s.tokenRegistry.GetByAddress(common.HexToAddress(tokenAddress))
		if err == nil {
			return info.Symbol
		}
	}
	// 默认返回地址作为符号
	return tokenAddress
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
				"deposit_id", record.DepositID,
				"error", err)
			return err
		}
	}

	// 更新状态为已入账
	return s.depositRepo.UpdateStatus(ctx, record.DepositID, model.DepositRecordStatusCredited)
}

// saveCheckpoint 保存检查点
func (s *IndexerService) saveCheckpoint(ctx context.Context, blockNumber uint64) {
	if blockNumber == 0 {
		return
	}

	block, err := s.client.GetBlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		logger.Error("failed to get block for checkpoint", "error", err)
		return
	}

	checkpoint := &model.BlockCheckpoint{
		ChainID:     s.chainID,
		BlockNumber: int64(blockNumber),
		BlockHash:   block.Hash().Hex(),
	}

	if err := s.checkpointRepo.Upsert(ctx, checkpoint); err != nil {
		logger.Error("failed to save checkpoint", "error", err)
		return
	}

	logger.Debug("checkpoint saved",
		"chain_id", s.chainID,
		"block", blockNumber)
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
				"deposit_id", record.DepositID,
				"error", err)
			continue
		}

		// 检查是否达到确认数
		if confirmations >= record.RequiredConfirmations && record.Status == model.DepositRecordStatusPending {
			if err := s.processConfirmedDeposit(ctx, record); err != nil {
				logger.Error("failed to process confirmed deposit",
					"deposit_id", record.DepositID,
					"error", err)
			}
		}
	}

	return nil
}

// ReprocessFromBlock 从指定区块重新处理（用于修复数据）
func (s *IndexerService) ReprocessFromBlock(ctx context.Context, fromBlock uint64) error {
	latestBlock, err := s.client.BlockNumber(ctx)
	if err != nil {
		return err
	}

	logger.Info("starting reprocess",
		"from_block", fromBlock,
		"latest_block", latestBlock)

	return s.processBlockRange(ctx, fromBlock, latestBlock)
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
		ChainID:      s.chainID,
		Running:      s.IsRunning(),
		CurrentBlock: currentBlock,
		LatestBlock:  latestBlock,
		LagBlocks:    lag,
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

// RecordChainEvent 记录链上事件（公开方法）
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
