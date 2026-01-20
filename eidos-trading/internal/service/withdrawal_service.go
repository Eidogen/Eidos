package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

var (
	ErrInvalidWithdrawal      = errors.New("invalid withdrawal")
	ErrWithdrawalNotFound     = errors.New("withdrawal not found")
	ErrWithdrawalNotPending   = errors.New("withdrawal is not pending")
	ErrDuplicateWithdrawal    = errors.New("duplicate withdrawal")
	ErrInvalidWithdrawNonce   = errors.New("invalid withdraw nonce")
	ErrInvalidWithdrawAddress = errors.New("invalid withdraw address")
)

// WithdrawalService 提现服务接口
type WithdrawalService interface {
	// CreateWithdrawal 创建提现请求
	// 1. 验证参数和签名
	// 2. 检查 Nonce 防重放
	// 3. 冻结已结算余额
	// 4. 创建提现记录
	CreateWithdrawal(ctx context.Context, req *CreateWithdrawalRequest) (*model.Withdrawal, error)

	// GetWithdrawal 获取提现详情
	GetWithdrawal(ctx context.Context, withdrawID string) (*model.Withdrawal, error)

	// ListWithdrawals 获取用户提现列表
	ListWithdrawals(ctx context.Context, wallet string, filter *repository.WithdrawalFilter, page *repository.Pagination) ([]*model.Withdrawal, error)

	// ApproveWithdrawal 审批通过提现
	// 风控审核通过后调用
	ApproveWithdrawal(ctx context.Context, withdrawID string) error

	// RejectWithdrawal 拒绝提现
	// 风控审核拒绝，需要解冻余额
	RejectWithdrawal(ctx context.Context, withdrawID, reason string) error

	// SubmitWithdrawal 提交提现到链上
	SubmitWithdrawal(ctx context.Context, withdrawID, txHash string) error

	// ConfirmWithdrawal 确认提现完成
	// 链上交易确认后调用
	ConfirmWithdrawal(ctx context.Context, withdrawID string) error

	// FailWithdrawal 提现失败处理
	// 链上交易失败，需要退回余额
	FailWithdrawal(ctx context.Context, withdrawID, reason string) error

	// CancelWithdrawal 取消提现 (用户主动取消)
	CancelWithdrawal(ctx context.Context, wallet, withdrawID string) error

	// GetPendingWithdrawals 获取待处理提现列表
	GetPendingWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error)

	// GetSubmittedWithdrawals 获取已提交待确认提现
	GetSubmittedWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error)

	// HandleConfirm 处理来自 Kafka 的提现确认消息
	HandleConfirm(ctx context.Context, msg *WithdrawalConfirmedMessage) error
}

// WithdrawalConfirmedMessage 提现确认消息 (来自 Kafka)
type WithdrawalConfirmedMessage struct {
	WithdrawalID string
	TxHash       string
	BlockNumber  int64
	Status       string // confirmed, failed
	Timestamp    int64
}

// CreateWithdrawalRequest 创建提现请求
type CreateWithdrawalRequest struct {
	Wallet    string          // 用户钱包
	Token     string          // 提现代币
	Amount    decimal.Decimal // 提现金额
	ToAddress string          // 目标地址
	Nonce     uint64          // 用户 Nonce
	Signature []byte          // EIP-712 签名
}

// WithdrawalPublisher 提现消息发布接口
type WithdrawalPublisher interface {
	PublishWithdrawalRequest(ctx context.Context, withdrawal *model.Withdrawal) error
}

// withdrawalService 提现服务实现
type withdrawalService struct {
	withdrawRepo         repository.WithdrawalRepository
	balanceRepo          repository.BalanceRepository
	balanceCache         cache.BalanceRedisRepository // Redis 作为实时资金真相
	nonceRepo            repository.NonceRepository
	idGenerator          IDGenerator
	tokenConfig          TokenConfigProvider
	withdrawalPublisher  WithdrawalPublisher // 提现消息发布者 (发送到 eidos-chain)
	asyncTasks           *AsyncTaskManager   // 异步任务管理器
}

// NewWithdrawalService 创建提现服务
func NewWithdrawalService(
	withdrawRepo repository.WithdrawalRepository,
	balanceRepo repository.BalanceRepository,
	balanceCache cache.BalanceRedisRepository,
	nonceRepo repository.NonceRepository,
	idGenerator IDGenerator,
	tokenConfig TokenConfigProvider,
	withdrawalPublisher WithdrawalPublisher,
) WithdrawalService {
	return &withdrawalService{
		withdrawRepo:        withdrawRepo,
		balanceRepo:         balanceRepo,
		balanceCache:        balanceCache,
		nonceRepo:           nonceRepo,
		idGenerator:         idGenerator,
		tokenConfig:         tokenConfig,
		withdrawalPublisher: withdrawalPublisher,
		asyncTasks:          GetAsyncTaskManager(),
	}
}

// CreateWithdrawal 创建提现请求
// 使用 Redis 原子操作冻结余额，确保实时资金一致性
func (s *withdrawalService) CreateWithdrawal(ctx context.Context, req *CreateWithdrawalRequest) (*model.Withdrawal, error) {
	// 1. 验证参数
	if err := s.validateWithdrawalRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidWithdrawal, err.Error())
	}

	// 2. 验证目标地址
	if err := model.ValidateWithdrawAddress(req.ToAddress); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidWithdrawAddress, err.Error())
	}

	// 3. 检查 Nonce 幂等性
	existingWithdraw, err := s.withdrawRepo.GetByWalletNonce(ctx, req.Wallet, req.Nonce)
	if err == nil {
		return existingWithdraw, nil // 返回已存在的提现
	}
	if !errors.Is(err, repository.ErrWithdrawalNotFound) {
		return nil, fmt.Errorf("check existing withdrawal failed: %w", err)
	}

	// 4. 从 Redis 检查可提现余额 (只有 settled_available 可提现)
	balance, err := s.balanceCache.GetBalance(ctx, req.Wallet, req.Token)
	if err != nil {
		if errors.Is(err, cache.ErrRedisBalanceNotFound) {
			return nil, ErrInsufficientBalance
		}
		return nil, fmt.Errorf("get balance from redis failed: %w", err)
	}

	if balance.SettledAvailable.LessThan(req.Amount) {
		return nil, ErrInsufficientBalance
	}

	// 5. 生成提现 ID
	withdrawIDInt, err := s.idGenerator.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate withdraw id failed: %w", err)
	}
	withdrawID := fmt.Sprintf("W%d", withdrawIDInt)

	// 6. 创建提现记录
	now := time.Now().UnixMilli()
	withdrawal := &model.Withdrawal{
		WithdrawID: withdrawID,
		Wallet:     req.Wallet,
		Token:      req.Token,
		Amount:     req.Amount,
		ToAddress:  req.ToAddress,
		Nonce:      req.Nonce,
		Signature:  req.Signature,
		Status:     model.WithdrawStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// 7. Redis 原子冻结已结算余额
	if err := s.balanceCache.Freeze(ctx, req.Wallet, req.Token, req.Amount, true, withdrawID); err != nil {
		if errors.Is(err, cache.ErrRedisInsufficientBalance) {
			return nil, ErrInsufficientBalance
		}
		return nil, fmt.Errorf("redis freeze balance failed: %w", err)
	}

	// 8. 异步写入 DB (Redis 已完成实时冻结，DB 用于持久化)
	s.asyncTasks.Submit("createWithdrawalDB", withdrawID, func(taskCtx context.Context) error {
		return s.balanceRepo.Transaction(taskCtx, func(txCtx context.Context) error {
			// 8.1 DB 冻结余额
			if err := s.balanceRepo.Freeze(txCtx, req.Wallet, req.Token, req.Amount, true); err != nil {
				return err
			}

			// 8.2 记录 Nonce
			if err := s.nonceRepo.MarkUsedWithTx(txCtx, req.Wallet, model.NonceUsageWithdraw, req.Nonce, withdrawID); err != nil {
				if errors.Is(err, repository.ErrNonceAlreadyUsed) {
					return nil // 幂等
				}
				return err
			}

			// 8.3 创建提现记录
			if err := s.withdrawRepo.Create(txCtx, withdrawal); err != nil {
				if errors.Is(err, repository.ErrWithdrawalAlreadyExists) {
					return nil // 幂等
				}
				return err
			}

			// 8.4 创建余额流水
			balanceLog := &model.BalanceLog{
				Wallet: req.Wallet,
				Token:  req.Token,
				Type:   model.BalanceLogTypeWithdraw,
				Amount: req.Amount.Neg(),
				Remark: fmt.Sprintf("Withdraw freeze: %s", withdrawID),
			}
			return s.balanceRepo.CreateBalanceLog(txCtx, balanceLog)
		})
	})

	// 9. TODO: 发送提现请求到风控服务

	return withdrawal, nil
}

// GetWithdrawal 获取提现详情
func (s *withdrawalService) GetWithdrawal(ctx context.Context, withdrawID string) (*model.Withdrawal, error) {
	return s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
}

// ListWithdrawals 获取用户提现列表
func (s *withdrawalService) ListWithdrawals(ctx context.Context, wallet string, filter *repository.WithdrawalFilter, page *repository.Pagination) ([]*model.Withdrawal, error) {
	return s.withdrawRepo.ListByWallet(ctx, wallet, filter, page)
}

// ApproveWithdrawal 审批通过提现
// 风控审核通过后：更新状态为 Processing，并发送 Kafka 消息到 eidos-chain 执行链上提现
func (s *withdrawalService) ApproveWithdrawal(ctx context.Context, withdrawID string) error {
	// 1. 获取提现详情
	withdrawal, err := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("get withdrawal: %w", err)
	}

	// 2. 更新状态为 Processing
	if err := s.withdrawRepo.MarkProcessing(ctx, withdrawID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	// 3. 发送提现请求到 eidos-chain
	if s.withdrawalPublisher != nil {
		if err := s.withdrawalPublisher.PublishWithdrawalRequest(ctx, withdrawal); err != nil {
			// 发送失败不影响审批结果，记录日志后继续
			// eidos-jobs 定时任务会扫描 Processing 状态的提现并重新发送
			logger.Warn("publish withdrawal request failed, will be retried by jobs",
				zap.String("withdrawal_id", withdrawID),
				zap.Error(err))
		}
	}

	return nil
}

// RejectWithdrawal 拒绝提现
// 风控审核拒绝，使用 Redis 解冻余额 + 同步 DB 持久化
// 支持幂等：如果已被拒绝，返回成功
func (s *withdrawalService) RejectWithdrawal(ctx context.Context, withdrawID, reason string) error {
	withdrawal, err := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return err
	}

	// 幂等检查：已是拒绝状态，直接返回成功
	if withdrawal.Status == model.WithdrawStatusRejected {
		return nil
	}

	if withdrawal.Status != model.WithdrawStatusPending {
		return ErrWithdrawalNotPending
	}

	// 1. 先尝试 DB 更新 (乐观锁保证并发安全)
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新状态 (WHERE status = PENDING)
		if err := s.withdrawRepo.MarkRejected(txCtx, withdrawID, reason); err != nil {
			return err
		}

		// 解冻余额 (回到 settled_available)
		if err := s.balanceRepo.Unfreeze(txCtx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); err != nil {
			return fmt.Errorf("unfreeze balance failed: %w", err)
		}

		// 记录流水
		balanceLog := &model.BalanceLog{
			Wallet: withdrawal.Wallet,
			Token:  withdrawal.Token,
			Type:   model.BalanceLogTypeWithdrawRefund,
			Amount: withdrawal.Amount,
			Remark: fmt.Sprintf("Withdraw rejected: %s - %s", withdrawID, reason),
		}
		return s.balanceRepo.CreateBalanceLog(txCtx, balanceLog)
	})

	// 处理乐观锁冲突 - 检查是否已被其他实例处理
	if errors.Is(err, repository.ErrOptimisticLock) {
		// 重新加载检查状态
		updated, reloadErr := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
		if reloadErr != nil {
			return fmt.Errorf("reload withdrawal after conflict: %w", reloadErr)
		}
		// 已被拒绝，幂等返回成功
		if updated.Status == model.WithdrawStatusRejected {
			return nil
		}
		// 状态变为其他，返回原错误
		return ErrWithdrawalNotPending
	}

	if err != nil {
		return err
	}

	// 2. DB 成功后，同步 Redis 解冻 (确保一致性)
	if unfreezeErr := s.balanceCache.Unfreeze(ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); unfreezeErr != nil {
		// Redis 解冻失败不影响业务，DB 已是最终状态，后续对账可修复
		if !errors.Is(unfreezeErr, cache.ErrRedisBalanceNotFound) {
			logger.Warn("redis unfreeze failed after db update, will be fixed by reconciliation",
				zap.String("withdraw_id", withdrawID),
				zap.String("wallet", withdrawal.Wallet),
				zap.Error(unfreezeErr))
		}
	}

	return nil
}

// SubmitWithdrawal 提交提现到链上
func (s *withdrawalService) SubmitWithdrawal(ctx context.Context, withdrawID, txHash string) error {
	return s.withdrawRepo.MarkSubmitted(ctx, withdrawID, txHash)
}

// ConfirmWithdrawal 确认提现完成
// 链上交易确认后调用，DB 优先 + Redis 同步
// 支持幂等：如果已确认，返回成功
func (s *withdrawalService) ConfirmWithdrawal(ctx context.Context, withdrawID string) error {
	withdrawal, err := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return err
	}

	// 幂等检查：已是确认状态，直接返回成功
	if withdrawal.Status == model.WithdrawStatusConfirmed {
		return nil
	}

	// 状态检查：只有 SUBMITTED 状态可以确认
	if withdrawal.Status != model.WithdrawStatusSubmitted {
		return fmt.Errorf("withdrawal status is %s, expected SUBMITTED", withdrawal.Status)
	}

	// 1. 先尝试 DB 更新 (乐观锁保证并发安全)
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新状态 (WHERE status = SUBMITTED)
		if err := s.withdrawRepo.MarkConfirmed(txCtx, withdrawID); err != nil {
			return err
		}

		// 从冻结余额中扣减 (真正扣款)
		if err := s.balanceRepo.Debit(txCtx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); err != nil {
			return fmt.Errorf("debit balance failed: %w", err)
		}

		return nil
	})

	// 处理乐观锁冲突
	if errors.Is(err, repository.ErrOptimisticLock) {
		updated, reloadErr := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
		if reloadErr != nil {
			return fmt.Errorf("reload withdrawal after conflict: %w", reloadErr)
		}
		if updated.Status == model.WithdrawStatusConfirmed {
			return nil // 幂等返回成功
		}
		return fmt.Errorf("withdrawal status changed to %s during confirm", updated.Status)
	}

	if err != nil {
		return err
	}

	// 2. DB 成功后，同步 Redis 扣款
	if debitErr := s.balanceCache.Debit(ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); debitErr != nil {
		if !errors.Is(debitErr, cache.ErrRedisBalanceNotFound) {
			logger.Warn("redis debit failed after db update, will be fixed by reconciliation",
				zap.String("withdraw_id", withdrawID),
				zap.String("wallet", withdrawal.Wallet),
				zap.Error(debitErr))
		}
	}

	return nil
}

// FailWithdrawal 提现失败处理
// 链上交易失败，DB 优先 + Redis 同步
// 支持幂等：如果已失败，返回成功
func (s *withdrawalService) FailWithdrawal(ctx context.Context, withdrawID, reason string) error {
	withdrawal, err := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return err
	}

	// 幂等检查：已是失败状态，直接返回成功
	if withdrawal.Status == model.WithdrawStatusFailed {
		return nil
	}

	// 状态检查：只有 PROCESSING 或 SUBMITTED 状态可以标记失败
	if withdrawal.Status != model.WithdrawStatusProcessing && withdrawal.Status != model.WithdrawStatusSubmitted {
		return fmt.Errorf("withdrawal status is %s, cannot mark as failed", withdrawal.Status)
	}

	// 1. 先尝试 DB 更新 (乐观锁保证并发安全)
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新状态
		if err := s.withdrawRepo.MarkFailed(txCtx, withdrawID, reason); err != nil {
			return err
		}

		// 解冻余额回到 settled_available
		if err := s.balanceRepo.Unfreeze(txCtx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); err != nil {
			return fmt.Errorf("unfreeze balance failed: %w", err)
		}

		// 标记已退回
		if err := s.withdrawRepo.MarkRefunded(txCtx, withdrawID); err != nil {
			return err
		}

		// 记录流水
		balanceLog := &model.BalanceLog{
			Wallet: withdrawal.Wallet,
			Token:  withdrawal.Token,
			Type:   model.BalanceLogTypeWithdrawRefund,
			Amount: withdrawal.Amount,
			Remark: fmt.Sprintf("Withdraw failed refund: %s - %s", withdrawID, reason),
		}
		return s.balanceRepo.CreateBalanceLog(txCtx, balanceLog)
	})

	// 处理乐观锁冲突
	if errors.Is(err, repository.ErrOptimisticLock) {
		updated, reloadErr := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
		if reloadErr != nil {
			return fmt.Errorf("reload withdrawal after conflict: %w", reloadErr)
		}
		if updated.Status == model.WithdrawStatusFailed {
			return nil // 幂等返回成功
		}
		return fmt.Errorf("withdrawal status changed to %s during fail", updated.Status)
	}

	if err != nil {
		return err
	}

	// 2. DB 成功后，同步 Redis 解冻
	if unfreezeErr := s.balanceCache.Unfreeze(ctx, withdrawal.Wallet, withdrawal.Token, withdrawal.Amount, true); unfreezeErr != nil {
		if !errors.Is(unfreezeErr, cache.ErrRedisBalanceNotFound) {
			logger.Warn("redis unfreeze failed after db update, will be fixed by reconciliation",
				zap.String("withdraw_id", withdrawID),
				zap.String("wallet", withdrawal.Wallet),
				zap.Error(unfreezeErr))
		}
	}

	return nil
}

// CancelWithdrawal 取消提现
// DB 优先 + Redis 同步，支持幂等
func (s *withdrawalService) CancelWithdrawal(ctx context.Context, wallet, withdrawID string) error {
	withdrawal, err := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return err
	}

	// 验证钱包
	if withdrawal.Wallet != wallet {
		return ErrWithdrawalNotFound
	}

	// 幂等检查：已是取消状态，直接返回成功
	if withdrawal.Status == model.WithdrawStatusCancelled {
		return nil
	}

	// 只有 PENDING 状态可以取消
	if withdrawal.Status != model.WithdrawStatusPending {
		return ErrWithdrawalNotPending
	}

	// 1. 先尝试 DB 更新 (乐观锁保证并发安全)
	err = s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		if err := s.withdrawRepo.MarkCancelled(txCtx, withdrawID); err != nil {
			return err
		}
		if err := s.balanceRepo.Unfreeze(txCtx, wallet, withdrawal.Token, withdrawal.Amount, true); err != nil {
			return err
		}
		balanceLog := &model.BalanceLog{
			Wallet: wallet,
			Token:  withdrawal.Token,
			Type:   model.BalanceLogTypeWithdrawRefund,
			Amount: withdrawal.Amount,
			Remark: fmt.Sprintf("Withdraw cancelled: %s", withdrawID),
		}
		return s.balanceRepo.CreateBalanceLog(txCtx, balanceLog)
	})

	// 处理乐观锁冲突
	if errors.Is(err, repository.ErrOptimisticLock) {
		updated, reloadErr := s.withdrawRepo.GetByWithdrawID(ctx, withdrawID)
		if reloadErr != nil {
			return fmt.Errorf("reload withdrawal after conflict: %w", reloadErr)
		}
		if updated.Status == model.WithdrawStatusCancelled {
			return nil // 幂等返回成功
		}
		return ErrWithdrawalNotPending
	}

	if err != nil {
		return err
	}

	// 2. DB 成功后，同步 Redis 解冻
	if unfreezeErr := s.balanceCache.Unfreeze(ctx, wallet, withdrawal.Token, withdrawal.Amount, true); unfreezeErr != nil {
		if !errors.Is(unfreezeErr, cache.ErrRedisBalanceNotFound) {
			logger.Warn("redis unfreeze failed after db update, will be fixed by reconciliation",
				zap.String("withdraw_id", withdrawID),
				zap.String("wallet", wallet),
				zap.Error(unfreezeErr))
		}
	}

	return nil
}

// GetPendingWithdrawals 获取待处理提现列表
func (s *withdrawalService) GetPendingWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	return s.withdrawRepo.ListPending(ctx, limit)
}

// GetSubmittedWithdrawals 获取已提交待确认提现
func (s *withdrawalService) GetSubmittedWithdrawals(ctx context.Context, limit int) ([]*model.Withdrawal, error) {
	return s.withdrawRepo.ListSubmitted(ctx, limit)
}

// validateWithdrawalRequest 验证提现请求
func (s *withdrawalService) validateWithdrawalRequest(req *CreateWithdrawalRequest) error {
	if req.Wallet == "" || len(req.Wallet) != 42 {
		return errors.New("invalid wallet address")
	}
	if req.Token == "" {
		return errors.New("token is required")
	}
	if !s.tokenConfig.IsValidToken(req.Token) {
		return errors.New("unsupported token")
	}
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("amount must be positive")
	}
	if req.ToAddress == "" {
		return errors.New("to_address is required")
	}
	if len(req.Signature) == 0 {
		return errors.New("signature is required")
	}
	return nil
}

// HandleConfirm 处理来自 Kafka 的提现确认消息
func (s *withdrawalService) HandleConfirm(ctx context.Context, msg *WithdrawalConfirmedMessage) error {
	switch msg.Status {
	case "confirmed":
		// 提现成功确认
		return s.ConfirmWithdrawal(ctx, msg.WithdrawalID)
	case "failed":
		// 提现失败，需要退款
		return s.FailWithdrawal(ctx, msg.WithdrawalID, "chain transaction failed")
	default:
		return fmt.Errorf("unknown withdrawal status: %s", msg.Status)
	}
}
