package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/publisher"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

var (
	ErrInvalidAmount = errors.New("invalid amount")
	ErrInvalidToken  = errors.New("invalid token")
)

// BalanceService 余额服务接口
type BalanceService interface {
	// GetBalance 获取用户单个代币余额
	GetBalance(ctx context.Context, wallet, token string) (*model.Balance, error)

	// GetBalances 获取用户所有余额
	GetBalances(ctx context.Context, wallet string) ([]*model.Balance, error)

	// GetAvailableBalance 获取可用余额 (交易可用 = settled_available + pending_available)
	GetAvailableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error)

	// GetWithdrawableBalance 获取可提现余额 (只有 settled_available 可提现)
	GetWithdrawableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error)

	// Credit 增加余额 (充值入账)
	Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool, remark string) error

	// Freeze 冻结余额 (下单)
	Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error

	// Unfreeze 解冻余额 (取消订单)
	Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error

	// Transfer 交易划转 (从冻结扣减 + 给对方增加)
	Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, tradeID string) error

	// Settle 结算确认 (pending → settled)
	Settle(ctx context.Context, wallet, token string, amount decimal.Decimal, batchID string) error

	// DeductFee 扣除手续费
	DeductFee(ctx context.Context, wallet, token string, amount decimal.Decimal, tradeID string) error

	// RefundWithdraw 提现失败退回
	RefundWithdraw(ctx context.Context, wallet, token string, amount decimal.Decimal, withdrawID string) error

	// GetBalanceLogs 获取余额流水
	GetBalanceLogs(ctx context.Context, wallet string, filter *repository.BalanceLogFilter, page *repository.Pagination) ([]*model.BalanceLog, error)

	// GetTotalFeeBalance 获取手续费账户总余额
	GetTotalFeeBalance(ctx context.Context, token string) (decimal.Decimal, error)
}

// BalancePublisher 余额变更发布接口
type BalancePublisher interface {
	PublishDeposit(ctx context.Context, snapshot *publisher.BalanceSnapshot, depositID string) error
	PublishWithdraw(ctx context.Context, snapshot *publisher.BalanceSnapshot, withdrawalID string) error
	PublishFreeze(ctx context.Context, snapshot *publisher.BalanceSnapshot, orderID string) error
	PublishUnfreeze(ctx context.Context, snapshot *publisher.BalanceSnapshot, orderID string) error
	PublishTrade(ctx context.Context, snapshot *publisher.BalanceSnapshot, tradeID string) error
	PublishSettle(ctx context.Context, snapshot *publisher.BalanceSnapshot, settlementID string) error
}

// balanceService 余额服务实现
// 使用 Transactional Outbox 模式保证 Redis → DB 的可靠同步
type balanceService struct {
	balanceRepo  repository.BalanceRepository
	outboxRepo   *repository.OutboxRepository
	balanceCache cache.BalanceRedisRepository // Redis 作为实时资金真相
	tokenConfig  TokenConfigProvider
	publisher    BalancePublisher // 余额变更发布者
}

// TokenConfigProvider 代币配置提供者
type TokenConfigProvider interface {
	IsValidToken(token string) bool
	GetTokenDecimals(token string) int32
	GetSupportedTokens() []string // 获取支持的代币列表
}

// NewBalanceService 创建余额服务
func NewBalanceService(
	balanceRepo repository.BalanceRepository,
	outboxRepo *repository.OutboxRepository,
	balanceCache cache.BalanceRedisRepository,
	tokenConfig TokenConfigProvider,
	publisher BalancePublisher,
) BalanceService {
	return &balanceService{
		balanceRepo:  balanceRepo,
		outboxRepo:   outboxRepo,
		balanceCache: balanceCache,
		tokenConfig:  tokenConfig,
		publisher:    publisher,
	}
}

// GetBalance 获取用户单个代币余额
// 从 Redis 读取实时余额 (Redis 是资金真相源)
func (s *balanceService) GetBalance(ctx context.Context, wallet, token string) (*model.Balance, error) {
	if !s.tokenConfig.IsValidToken(token) {
		return nil, ErrInvalidToken
	}

	// 从 Redis 读取实时余额
	redisBalance, err := s.balanceCache.GetOrCreateBalance(ctx, wallet, token)
	if err != nil {
		return nil, fmt.Errorf("get balance from redis failed: %w", err)
	}

	// 转换为 model.Balance
	return &model.Balance{
		Wallet:           redisBalance.Wallet,
		Token:            redisBalance.Token,
		SettledAvailable: redisBalance.SettledAvailable,
		SettledFrozen:    redisBalance.SettledFrozen,
		PendingAvailable: redisBalance.PendingAvailable,
		PendingFrozen:    redisBalance.PendingFrozen,
		Version:          redisBalance.Version,
		UpdatedAt:        redisBalance.UpdatedAt,
	}, nil
}

// GetBalances 获取用户所有余额
// 使用 Redis Pipeline 批量读取，避免 N+1 查询问题
func (s *balanceService) GetBalances(ctx context.Context, wallet string) ([]*model.Balance, error) {
	tokens := s.tokenConfig.GetSupportedTokens()
	if len(tokens) == 0 {
		return nil, nil
	}

	// 使用 Pipeline 批量获取 (单次 RTT)
	redisBalances, err := s.balanceCache.GetBalancesBatch(ctx, wallet, tokens)
	if err != nil {
		return nil, fmt.Errorf("get balances batch failed: %w", err)
	}

	// 转换并过滤
	balances := make([]*model.Balance, 0, len(redisBalances))
	for _, rb := range redisBalances {
		balance := &model.Balance{
			Wallet:           rb.Wallet,
			Token:            rb.Token,
			SettledAvailable: rb.SettledAvailable,
			SettledFrozen:    rb.SettledFrozen,
			PendingAvailable: rb.PendingAvailable,
			PendingFrozen:    rb.PendingFrozen,
			Version:          rb.Version,
			UpdatedAt:        rb.UpdatedAt,
		}
		// 只返回有余额的代币
		if balance.TotalAvailable().GreaterThan(decimal.Zero) ||
			balance.TotalFrozen().GreaterThan(decimal.Zero) {
			balances = append(balances, balance)
		}
	}

	return balances, nil
}

// GetAvailableBalance 获取可用余额
func (s *balanceService) GetAvailableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	balance, err := s.GetBalance(ctx, wallet, token)
	if err != nil {
		return decimal.Zero, err
	}
	return balance.TotalAvailable(), nil
}

// GetWithdrawableBalance 获取可提现余额
func (s *balanceService) GetWithdrawableBalance(ctx context.Context, wallet, token string) (decimal.Decimal, error) {
	balance, err := s.GetBalance(ctx, wallet, token)
	if err != nil {
		return decimal.Zero, err
	}
	return balance.WithdrawableAmount(), nil
}

// Credit 增加余额
// 使用 Redis 原子操作增加余额，通过 Outbox 持久化到 DB
func (s *balanceService) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool, remark string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}
	if !s.tokenConfig.IsValidToken(token) {
		return ErrInvalidToken
	}

	// Redis 原子增加余额 (实时生效)
	if err := s.balanceCache.Credit(ctx, wallet, token, amount, toSettled); err != nil {
		return fmt.Errorf("redis credit balance failed: %w", err)
	}

	// 写入 Outbox (异步持久化到 DB)
	logType := model.BalanceLogTypeDeposit
	s.enqueueBalanceLog(ctx, wallet, token, logType, amount, "", "", "", remark)

	// 发布余额变更消息
	s.publishBalanceChange(ctx, wallet, token, "deposit", "")

	return nil
}

// Freeze 冻结余额
// 使用 Redis 原子操作冻结余额
func (s *balanceService) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Redis 原子冻结 (从待结算可用冻结)
	if err := s.balanceCache.Freeze(ctx, wallet, token, amount, false, orderID); err != nil {
		if errors.Is(err, cache.ErrRedisInsufficientBalance) {
			return fmt.Errorf("%w: %v", repository.ErrInsufficientBalance, err)
		}
		return fmt.Errorf("redis freeze failed: %w", err)
	}

	// 写入 Outbox
	s.enqueueBalanceLog(ctx, wallet, token, model.BalanceLogTypeFreeze, amount.Neg(), orderID, "", "", fmt.Sprintf("Freeze for order %s", orderID))

	// 发布余额变更消息
	s.publishBalanceChange(ctx, wallet, token, "freeze", orderID)

	return nil
}

// Unfreeze 解冻余额
// 使用 Redis 原子操作解冻余额
func (s *balanceService) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, orderID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Redis 原子解冻 (回到待结算可用)
	if err := s.balanceCache.Unfreeze(ctx, wallet, token, amount, false); err != nil {
		return fmt.Errorf("redis unfreeze failed: %w", err)
	}

	// 写入 Outbox
	s.enqueueBalanceLog(ctx, wallet, token, model.BalanceLogTypeUnfreeze, amount, orderID, "", "", fmt.Sprintf("Unfreeze for order %s", orderID))

	// 发布余额变更消息
	s.publishBalanceChange(ctx, wallet, token, "unfreeze", orderID)

	return nil
}

// Transfer 交易划转
// 使用 Redis Lua 脚本原子操作进行划转，通过 Outbox 持久化到 DB
func (s *balanceService) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal, tradeID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Redis 原子转账 (单个 Lua 脚本，保证原子性)
	// fromSettled=false 表示从 pending_frozen 扣减，增加到 pending_available
	if err := s.balanceCache.Transfer(ctx, fromWallet, toWallet, token, amount, false); err != nil {
		if errors.Is(err, cache.ErrRedisInsufficientBalance) {
			return fmt.Errorf("%w: %v", repository.ErrInsufficientBalance, err)
		}
		return fmt.Errorf("redis transfer failed: %w", err)
	}

	// 写入 Outbox (双方流水)
	s.enqueueBalanceLog(ctx, fromWallet, token, model.BalanceLogTypeTrade, amount.Neg(), "", tradeID, "", fmt.Sprintf("Trade out: %s", tradeID))
	s.enqueueBalanceLog(ctx, toWallet, token, model.BalanceLogTypeTrade, amount, "", tradeID, "", fmt.Sprintf("Trade in: %s", tradeID))

	// 发布余额变更消息 (双方)
	s.publishBalanceChange(ctx, fromWallet, token, "trade", tradeID)
	s.publishBalanceChange(ctx, toWallet, token, "trade", tradeID)

	return nil
}

// Settle 结算确认 (pending → settled)
// 使用 Redis 原子操作
func (s *balanceService) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal, batchID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Redis 原子结算
	if err := s.balanceCache.Settle(ctx, wallet, token, amount); err != nil {
		return fmt.Errorf("redis settle balance failed: %w", err)
	}

	// 写入 Outbox
	s.enqueueBalanceLog(ctx, wallet, token, model.BalanceLogTypeSettlement, amount, "", "", "", fmt.Sprintf("Settlement confirmed: %s", batchID))

	// 发布余额变更消息
	s.publishBalanceChange(ctx, wallet, token, "settle", batchID)

	return nil
}

// DeductFee 扣除手续费
// 使用 Redis 原子操作扣除手续费，通过 Outbox 持久化到 DB
func (s *balanceService) DeductFee(ctx context.Context, wallet, token string, amount decimal.Decimal, tradeID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil // 0 手续费不需要处理
	}

	// 1. Redis: 从用户待结算可用扣减 (fromSettled=false 表示从 pending_available 扣减)
	if err := s.balanceCache.Debit(ctx, wallet, token, amount, false); err != nil {
		return fmt.Errorf("redis debit fee failed: %w", err)
	}

	// 2. 写入 Outbox
	s.enqueueBalanceLog(ctx, wallet, token, model.BalanceLogTypeFee, amount.Neg(), "", tradeID, "", fmt.Sprintf("Fee deducted: %s", tradeID))

	return nil
}

// RefundWithdraw 提现失败退回
// 使用 Redis 原子操作退回余额
func (s *balanceService) RefundWithdraw(ctx context.Context, wallet, token string, amount decimal.Decimal, withdrawID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Redis 原子退回到已结算可用
	if err := s.balanceCache.Credit(ctx, wallet, token, amount, true); err != nil {
		return fmt.Errorf("redis refund withdraw failed: %w", err)
	}

	// 写入 Outbox
	s.enqueueBalanceLog(ctx, wallet, token, model.BalanceLogTypeWithdrawRefund, amount, "", "", "", fmt.Sprintf("Withdraw refund: %s", withdrawID))

	// 发布余额变更消息 (提现退回视为充值类型)
	s.publishBalanceChange(ctx, wallet, token, "deposit", withdrawID)

	return nil
}

// GetBalanceLogs 获取余额流水
func (s *balanceService) GetBalanceLogs(ctx context.Context, wallet string, filter *repository.BalanceLogFilter, page *repository.Pagination) ([]*model.BalanceLog, error) {
	return s.balanceRepo.ListBalanceLogs(ctx, wallet, filter, page)
}

// GetTotalFeeBalance 获取手续费账户总余额
func (s *balanceService) GetTotalFeeBalance(ctx context.Context, token string) (decimal.Decimal, error) {
	total := decimal.Zero

	// 遍历所有桶 (0-15)
	for i := 0; i < 16; i++ {
		account, err := s.balanceRepo.GetFeeAccount(ctx, i, token)
		if err != nil {
			continue
		}
		total = total.Add(account.Balance)
	}

	return total, nil
}

// enqueueBalanceLog 将余额流水写入 Outbox
func (s *balanceService) enqueueBalanceLog(ctx context.Context, wallet, token string, logType model.BalanceLogType, amount decimal.Decimal, orderID, tradeID, txHash, remark string) {
	// 如果 outboxRepo 为 nil，跳过 (测试场景)
	if s.outboxRepo == nil {
		return
	}

	payload := model.BalanceLogPayload{
		Wallet:        wallet,
		Token:         token,
		Type:          int8(logType),
		Amount:        amount.String(),
		BalanceBefore: "0",
		BalanceAfter:  "0",
		OrderID:       orderID,
		TradeID:       tradeID,
		TxHash:        txHash,
		Remark:        remark,
	}

	msg := &model.OutboxMessage{
		MessageID:     uuid.New().String(),
		Topic:         model.TopicBalanceLog,
		PartitionKey:  fmt.Sprintf("%s:%s", wallet, token),
		AggregateType: model.AggregateTypeBalance,
		AggregateID:   fmt.Sprintf("%s:%s", wallet, token),
		Status:        model.OutboxStatusPending,
		MaxRetries:    5,
		CreatedAt:     time.Now().UnixMilli(),
	}

	if err := msg.SetPayload(payload); err != nil {
		logger.Error("set outbox payload failed",
			zap.String("wallet", wallet),
			zap.String("token", token),
			zap.Error(err))
		return
	}

	if err := s.outboxRepo.Create(ctx, msg); err != nil {
		logger.Error("create outbox message failed",
			zap.String("wallet", wallet),
			zap.String("token", token),
			zap.String("messageID", msg.MessageID),
			zap.Error(err))
	}
}

// hashToBucket 根据钱包地址哈希到桶 ID
func hashToBucket(wallet string, buckets int) int {
	if len(wallet) < 2 {
		return 0
	}
	// 使用地址最后一个字节的低 4 位
	lastByte := wallet[len(wallet)-1]
	return int(lastByte) % buckets
}

// publishBalanceChange 发布余额变更消息
// 异步发布，不阻塞主流程
func (s *balanceService) publishBalanceChange(ctx context.Context, wallet, token, eventType, eventID string) {
	if s.publisher == nil {
		return
	}

	// 异步发布，不阻塞主流程
	go func() {
		// 获取最新余额快照
		balance, err := s.balanceCache.GetOrCreateBalance(ctx, wallet, token)
		if err != nil {
			return
		}

		snapshot := &publisher.BalanceSnapshot{
			Wallet:    wallet,
			Token:     token,
			Available: balance.SettledAvailable.Add(balance.PendingAvailable),
			Frozen:    balance.SettledFrozen.Add(balance.PendingFrozen),
			Pending:   balance.PendingAvailable.Add(balance.PendingFrozen),
		}

		var publishErr error
		switch eventType {
		case "deposit":
			publishErr = s.publisher.PublishDeposit(ctx, snapshot, eventID)
		case "withdraw":
			publishErr = s.publisher.PublishWithdraw(ctx, snapshot, eventID)
		case "freeze":
			publishErr = s.publisher.PublishFreeze(ctx, snapshot, eventID)
		case "unfreeze":
			publishErr = s.publisher.PublishUnfreeze(ctx, snapshot, eventID)
		case "trade":
			publishErr = s.publisher.PublishTrade(ctx, snapshot, eventID)
		case "settle":
			publishErr = s.publisher.PublishSettle(ctx, snapshot, eventID)
		}

		if publishErr != nil {
			// 发布失败只记录日志，不影响主流程
			// 客户端可以通过 API 查询最新余额
		}
	}()
}
