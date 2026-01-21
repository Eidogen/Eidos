package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

var (
	ErrInvalidDeposit      = errors.New("invalid deposit")
	ErrDepositNotFound     = errors.New("deposit not found")
	ErrDuplicateDeposit    = errors.New("duplicate deposit")
	ErrDepositNotConfirmed = errors.New("deposit not confirmed")
)

// DepositService 充值服务接口
type DepositService interface {
	// ProcessDepositEvent 处理链上充值事件
	// 从链服务接收充值事件，创建充值记录
	ProcessDepositEvent(ctx context.Context, event *DepositEvent) error

	// ConfirmDeposit 确认充值
	// 达到确认区块数后调用
	ConfirmDeposit(ctx context.Context, depositID string) error

	// CreditDeposit 充值入账
	// 确认后入账到用户余额
	CreditDeposit(ctx context.Context, depositID string) error

	// GetDeposit 获取充值详情
	GetDeposit(ctx context.Context, depositID string) (*model.Deposit, error)

	// GetDepositByTxHash 通过交易哈希查询充值
	GetDepositByTxHash(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error)

	// ListDeposits 获取用户充值列表
	ListDeposits(ctx context.Context, wallet string, filter *repository.DepositFilter, page *repository.Pagination) ([]*model.Deposit, error)

	// GetPendingDeposits 获取待确认充值列表
	GetPendingDeposits(ctx context.Context, limit int) ([]*model.Deposit, error)

	// GetConfirmedDeposits 获取已确认待入账充值
	GetConfirmedDeposits(ctx context.Context, limit int) ([]*model.Deposit, error)

	// ProcessDeposit 处理来自 Kafka 的充值消息
	ProcessDeposit(ctx context.Context, msg *DepositMessage) error
}

// DepositMessage 充值消息 (来自 Kafka)
type DepositMessage struct {
	TxHash      string
	Wallet      string
	Token       string
	Amount      string
	BlockNumber int64
	Timestamp   int64
}

// DepositEvent 链上充值事件
type DepositEvent struct {
	TxHash     string          // 交易哈希
	LogIndex   uint32          // 日志索引
	BlockNum   int64           // 区块高度
	Wallet     string          // 用户钱包
	Token      string          // 代币
	Amount     decimal.Decimal // 充值金额
	DetectedAt int64           // 检测时间
}

// depositService 充值服务实现
type depositService struct {
	depositRepo  repository.DepositRepository
	balanceRepo  repository.BalanceRepository
	balanceCache cache.BalanceRedisRepository // Redis 作为实时资金真相
	idGenerator  IDGenerator
	tokenConfig  TokenConfigProvider
	asyncTasks   *AsyncTaskManager // 异步任务管理器
}

// NewDepositService 创建充值服务
func NewDepositService(
	depositRepo repository.DepositRepository,
	balanceRepo repository.BalanceRepository,
	balanceCache cache.BalanceRedisRepository,
	idGenerator IDGenerator,
	tokenConfig TokenConfigProvider,
) DepositService {
	return &depositService{
		depositRepo:  depositRepo,
		balanceRepo:  balanceRepo,
		balanceCache: balanceCache,
		idGenerator:  idGenerator,
		tokenConfig:  tokenConfig,
		asyncTasks:   GetAsyncTaskManager(),
	}
}

// ProcessDepositEvent 处理链上充值事件
func (s *depositService) ProcessDepositEvent(ctx context.Context, event *DepositEvent) error {
	// 1. 验证事件
	if err := s.validateDepositEvent(event); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidDeposit, err.Error())
	}

	// 2. 检查是否已存在 (幂等, 基于 tx_hash + log_index)
	_, err := s.depositRepo.GetByTxHashLogIndex(ctx, event.TxHash, event.LogIndex)
	if err == nil {
		return nil // 已处理过，直接返回
	}
	if !errors.Is(err, repository.ErrDepositNotFound) {
		return fmt.Errorf("check deposit exists failed: %w", err)
	}

	// 3. 生成充值 ID
	depositIDInt, err := s.idGenerator.Generate()
	if err != nil {
		return fmt.Errorf("generate deposit id failed: %w", err)
	}
	depositID := fmt.Sprintf("D%d", depositIDInt)

	// 4. 创建充值记录
	now := time.Now().UnixMilli()
	deposit := &model.Deposit{
		DepositID:  depositID,
		Wallet:     event.Wallet,
		Token:      event.Token,
		Amount:     event.Amount,
		TxHash:     event.TxHash,
		LogIndex:   event.LogIndex,
		BlockNum:   event.BlockNum,
		Status:     model.DepositStatusPending,
		DetectedAt: event.DetectedAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.depositRepo.Create(ctx, deposit); err != nil {
		if errors.Is(err, repository.ErrDepositAlreadyExists) {
			return nil // 幂等处理
		}
		return fmt.Errorf("create deposit failed: %w", err)
	}

	// 5. 发送充值事件通知
	// 充值事件通过 AsyncTaskManager 异步处理，由 eidos-api WebSocket 推送给用户
	// 见 eidos-trading/internal/publisher/balance_publisher.go

	return nil
}

// ConfirmDeposit 确认充值
func (s *depositService) ConfirmDeposit(ctx context.Context, depositID string) error {
	return s.depositRepo.MarkConfirmed(ctx, depositID)
}

// CreditDeposit 充值入账
// 使用 Redis 原子操作入账，DB 异步持久化
func (s *depositService) CreditDeposit(ctx context.Context, depositID string) error {
	// 1. 获取充值记录
	deposit, err := s.depositRepo.GetByDepositID(ctx, depositID)
	if err != nil {
		return err
	}

	// 2. 检查状态
	if deposit.Status != model.DepositStatusConfirmed {
		return ErrDepositNotConfirmed
	}

	// 3. Redis 原子入账 (充值直接进入 settled_available)
	if err := s.balanceCache.Credit(ctx, deposit.Wallet, deposit.Token, deposit.Amount, true); err != nil {
		return fmt.Errorf("redis credit balance failed: %w", err)
	}

	// 4. 异步写入 DB
	s.asyncTasks.Submit("creditDepositDB", depositID, func(taskCtx context.Context) error {
		return s.balanceRepo.Transaction(taskCtx, func(txCtx context.Context) error {
			// 4.1 DB 入账
			if err := s.balanceRepo.Credit(txCtx, deposit.Wallet, deposit.Token, deposit.Amount, true); err != nil {
				return err
			}

			// 4.2 更新充值状态
			if err := s.depositRepo.MarkCredited(txCtx, depositID); err != nil {
				return err
			}

			// 4.3 创建余额流水
			balanceLog := &model.BalanceLog{
				Wallet: deposit.Wallet,
				Token:  deposit.Token,
				Type:   model.BalanceLogTypeDeposit,
				Amount: deposit.Amount,
				TxHash: deposit.TxHash,
				Remark: fmt.Sprintf("Deposit credited: %s", depositID),
			}
			return s.balanceRepo.CreateBalanceLog(txCtx, balanceLog)
		})
	})

	return nil
}

// GetDeposit 获取充值详情
func (s *depositService) GetDeposit(ctx context.Context, depositID string) (*model.Deposit, error) {
	return s.depositRepo.GetByDepositID(ctx, depositID)
}

// GetDepositByTxHash 通过交易哈希查询充值
func (s *depositService) GetDepositByTxHash(ctx context.Context, txHash string, logIndex uint32) (*model.Deposit, error) {
	return s.depositRepo.GetByTxHashLogIndex(ctx, txHash, logIndex)
}

// ListDeposits 获取用户充值列表
func (s *depositService) ListDeposits(ctx context.Context, wallet string, filter *repository.DepositFilter, page *repository.Pagination) ([]*model.Deposit, error) {
	return s.depositRepo.ListByWallet(ctx, wallet, filter, page)
}

// GetPendingDeposits 获取待确认充值列表
func (s *depositService) GetPendingDeposits(ctx context.Context, limit int) ([]*model.Deposit, error) {
	return s.depositRepo.ListPending(ctx, limit)
}

// GetConfirmedDeposits 获取已确认待入账充值
func (s *depositService) GetConfirmedDeposits(ctx context.Context, limit int) ([]*model.Deposit, error) {
	return s.depositRepo.ListConfirmed(ctx, limit)
}

// validateDepositEvent 验证充值事件
func (s *depositService) validateDepositEvent(event *DepositEvent) error {
	if event.TxHash == "" || len(event.TxHash) != 66 {
		return errors.New("invalid tx_hash")
	}
	if event.Wallet == "" || len(event.Wallet) != 42 {
		return errors.New("invalid wallet address")
	}
	if event.Token == "" {
		return errors.New("token is required")
	}
	if !s.tokenConfig.IsValidToken(event.Token) {
		return errors.New("unsupported token")
	}
	if event.Amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("amount must be positive")
	}
	if event.BlockNum <= 0 {
		return errors.New("invalid block_num")
	}
	return nil
}

// ProcessDeposit 处理来自 Kafka 的充值消息
func (s *depositService) ProcessDeposit(ctx context.Context, msg *DepositMessage) error {
	// 解析金额
	amount, err := decimal.NewFromString(msg.Amount)
	if err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	// 转换为内部事件格式
	event := &DepositEvent{
		TxHash:     msg.TxHash,
		LogIndex:   0, // Kafka 消息可能不包含 LogIndex
		BlockNum:   msg.BlockNumber,
		Wallet:     msg.Wallet,
		Token:      msg.Token,
		Amount:     amount,
		DetectedAt: msg.Timestamp,
	}

	// 调用已有的处理方法
	return s.ProcessDepositEvent(ctx, event)
}
