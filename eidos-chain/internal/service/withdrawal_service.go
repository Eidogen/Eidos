// ========================================
// WithdrawalService 提现服务对接说明
// ========================================
//
// ## 功能概述
// WithdrawalService 处理用户的链上提现请求。
// 接收审核通过的提现请求，调用智能合约执行转账。
//
// ## 消息来源 (Kafka Consumer)
// - Topic: withdrawals (来自 eidos-trading)
// - 消息类型: model.WithdrawalRequest
// - 处理流程:
//   1. ProcessWithdrawalRequest() 接收提现请求
//   2. 验证签名 (TODO: 实现 EIP-712 签名验证)
//   3. 调用 Vault 合约执行提现
//   4. 监听交易确认
//
// ## 消息输出 (Kafka Producer)
// - Topic: withdrawal-confirmed (发送给 eidos-trading)
// - 消息类型: model.WithdrawalConfirmation
// - 触发条件: 链上交易确认后回调 onWithdrawalConfirmed
//
// ## TODO: eidos-trading 对接
// 1. eidos-trading 的提现服务需要发送 withdrawals 消息
//    - 触发时机: 提现请求审核通过后
//    - 消息格式: model.WithdrawalRequest (包含 withdraw_id, wallet, to_address, amount, signature)
//    - 提现状态: APPROVED -> PENDING (等待上链)
//
// 2. eidos-trading 需要订阅 withdrawal-confirmed 消息
//    - 更新提现状态: PENDING -> COMPLETED
//    - 记录 tx_hash, block_number
//    - 释放冻结余额
//
// ## 智能合约对接
// - TODO: 部署 Vault 合约
// - TODO: 实现 buildWithdrawalTx() 中的合约调用逻辑
// - TODO: 验证提现签名 (verifyWithdrawalSignature)
// - 当前 Mock 模式返回模拟 txHash
//
// ========================================
package service

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"
)

var (
	ErrWithdrawalNotFound    = errors.New("withdrawal not found")
	ErrWithdrawalInProgress  = errors.New("withdrawal already in progress")
	ErrInsufficientBalance   = errors.New("insufficient balance for withdrawal")
	ErrInvalidSignature      = errors.New("invalid withdrawal signature")
)

// WithdrawalService 提现服务
type WithdrawalService struct {
	repo         repository.WithdrawalRepository
	nonceRepo    repository.NonceRepository
	client       *blockchain.Client
	nonceManager *blockchain.NonceManager

	// 配置
	maxRetries   int
	retryBackoff time.Duration
	chainID      int64

	// 合约地址
	vaultContract common.Address

	// 事件回调
	onWithdrawalConfirmed func(ctx context.Context, confirmation *model.WithdrawalConfirmation) error
}

// WithdrawalServiceConfig 配置
type WithdrawalServiceConfig struct {
	MaxRetries    int
	RetryBackoff  time.Duration
	ChainID       int64
	VaultContract common.Address
}

// NewWithdrawalService 创建提现服务
func NewWithdrawalService(
	repo repository.WithdrawalRepository,
	nonceRepo repository.NonceRepository,
	client *blockchain.Client,
	nonceManager *blockchain.NonceManager,
	cfg *WithdrawalServiceConfig,
) *WithdrawalService {
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryBackoff := cfg.RetryBackoff
	if retryBackoff == 0 {
		retryBackoff = 30 * time.Second
	}

	return &WithdrawalService{
		repo:          repo,
		nonceRepo:     nonceRepo,
		client:        client,
		nonceManager:  nonceManager,
		maxRetries:    maxRetries,
		retryBackoff:  retryBackoff,
		chainID:       cfg.ChainID,
		vaultContract: cfg.VaultContract,
	}
}

// SetOnWithdrawalConfirmed 设置提现确认回调
func (s *WithdrawalService) SetOnWithdrawalConfirmed(fn func(ctx context.Context, confirmation *model.WithdrawalConfirmation) error) {
	s.onWithdrawalConfirmed = fn
}

// ProcessWithdrawalRequest 处理提现请求
func (s *WithdrawalService) ProcessWithdrawalRequest(ctx context.Context, req *model.WithdrawalRequest) error {
	// 检查是否已存在
	existing, err := s.repo.GetByWithdrawID(ctx, req.WithdrawID)
	if err == nil && existing != nil {
		if existing.Status == model.WithdrawalTxStatusConfirmed {
			return nil // 已完成
		}
		if existing.Status == model.WithdrawalTxStatusPending || existing.Status == model.WithdrawalTxStatusSubmitted {
			return ErrWithdrawalInProgress
		}
	}

	// 验证签名
	// TODO: 实现 EIP-712 签名验证
	// if !s.verifySignature(req) {
	// 	return ErrInvalidSignature
	// }

	// 创建提现记录
	tx := &model.WithdrawalTx{
		WithdrawID:    req.WithdrawID,
		WalletAddress: req.Wallet,
		ToAddress:     req.ToAddress,
		Token:         req.Token,
		TokenAddress:  req.TokenAddress,
		Amount:        req.Amount,
		ChainID:       s.chainID,
		Status:        model.WithdrawalTxStatusPending,
	}

	if err := s.repo.Create(ctx, tx); err != nil {
		return err
	}

	logger.Info("withdrawal request received",
		zap.String("withdraw_id", req.WithdrawID),
		zap.String("wallet", req.Wallet),
		zap.String("token", req.Token),
		zap.String("amount", req.Amount.String()))

	// 立即尝试提交
	return s.submitWithdrawal(ctx, tx)
}

// ProcessPendingWithdrawals 处理待提交的提现
func (s *WithdrawalService) ProcessPendingWithdrawals(ctx context.Context) error {
	txs, err := s.repo.ListPending(ctx, 10)
	if err != nil {
		return err
	}

	for _, tx := range txs {
		if err := s.submitWithdrawal(ctx, tx); err != nil {
			logger.Error("failed to submit withdrawal",
				zap.String("withdraw_id", tx.WithdrawID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// submitWithdrawal 提交提现到链上
func (s *WithdrawalService) submitWithdrawal(ctx context.Context, withdrawTx *model.WithdrawalTx) error {
	// 检查重试次数
	if withdrawTx.RetryCount >= s.maxRetries {
		return s.markWithdrawalFailed(ctx, withdrawTx, "max retries exceeded")
	}

	// 获取 nonce
	nonce, err := s.nonceManager.AcquireNonce(ctx)
	if err != nil {
		return err
	}

	// 构建交易
	tx, err := s.buildWithdrawalTx(ctx, withdrawTx, nonce)
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

		errStr := err.Error()
		if contains(errStr, "nonce too low") {
			s.nonceManager.HandleNonceTooLow(ctx)
		}
		if contains(errStr, "insufficient funds") {
			return s.markWithdrawalFailed(ctx, withdrawTx, "insufficient funds for gas")
		}

		return s.handleSubmitError(ctx, withdrawTx, err)
	}

	txHash := signedTx.Hash().Hex()

	// 确认 nonce 使用
	s.nonceManager.ConfirmNonce(ctx, nonce, txHash)

	// 更新状态为已提交
	if err := s.repo.UpdateStatus(ctx, withdrawTx.WithdrawID, model.WithdrawalTxStatusSubmitted, txHash, 0); err != nil {
		logger.Error("failed to update withdrawal status",
			zap.String("withdraw_id", withdrawTx.WithdrawID),
			zap.Error(err))
	}

	// 记录待确认交易
	pendingTx := &model.PendingTx{
		TxHash:        txHash,
		TxType:        model.PendingTxTypeWithdrawal,
		RefID:         withdrawTx.WithdrawID,
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

	logger.Info("withdrawal transaction submitted",
		zap.String("withdraw_id", withdrawTx.WithdrawID),
		zap.String("tx_hash", txHash),
		zap.Uint64("nonce", nonce))

	return nil
}

// buildWithdrawalTx 构建提现交易
func (s *WithdrawalService) buildWithdrawalTx(ctx context.Context, withdrawTx *model.WithdrawalTx, nonce uint64) (*types.Transaction, error) {
	// TODO: 根据实际合约 ABI 构建交易数据
	// Vault.withdraw(user, token, amount, to)

	data, err := s.encodeWithdrawalData(withdrawTx)
	if err != nil {
		return nil, err
	}

	gasPrice, err := s.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	gasLimit := uint64(150000) // 提现预估 gas

	tx := types.NewTransaction(
		nonce,
		s.vaultContract,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		data,
	)

	return tx, nil
}

// encodeWithdrawalData 编码提现数据
func (s *WithdrawalService) encodeWithdrawalData(withdrawTx *model.WithdrawalTx) ([]byte, error) {
	// TODO: 使用实际的合约 ABI 编码
	return []byte{}, nil
}

// handleSubmitError 处理提交错误
func (s *WithdrawalService) handleSubmitError(ctx context.Context, withdrawTx *model.WithdrawalTx, err error) error {
	logger.Warn("withdrawal submission failed, will retry",
		zap.String("withdraw_id", withdrawTx.WithdrawID),
		zap.Int("retry_count", withdrawTx.RetryCount),
		zap.Error(err))

	return s.repo.UpdateFailed(ctx, withdrawTx.WithdrawID, err.Error())
}

// markWithdrawalFailed 标记提现失败
func (s *WithdrawalService) markWithdrawalFailed(ctx context.Context, withdrawTx *model.WithdrawalTx, reason string) error {
	logger.Error("withdrawal failed",
		zap.String("withdraw_id", withdrawTx.WithdrawID),
		zap.String("reason", reason))

	if err := s.repo.UpdateFailed(ctx, withdrawTx.WithdrawID, reason); err != nil {
		return err
	}

	// 发送失败通知
	if s.onWithdrawalConfirmed != nil {
		confirmation := &model.WithdrawalConfirmation{
			WithdrawID:  withdrawTx.WithdrawID,
			Wallet:      withdrawTx.WalletAddress,
			Status:      "FAILED",
			Error:       reason,
			ConfirmedAt: time.Now().UnixMilli(),
		}
		s.onWithdrawalConfirmed(ctx, confirmation)
	}

	return nil
}

// OnTxConfirmed 交易确认回调
func (s *WithdrawalService) OnTxConfirmed(ctx context.Context, txHash string, blockNumber uint64, gasUsed uint64) error {
	pendingTx, err := s.nonceRepo.GetPendingTxByHash(ctx, txHash)
	if err != nil {
		return err
	}

	if pendingTx.TxType != model.PendingTxTypeWithdrawal {
		return nil
	}

	withdrawID := pendingTx.RefID

	// 更新提现状态
	if err := s.repo.UpdateStatus(ctx, withdrawID, model.WithdrawalTxStatusConfirmed, txHash, int64(blockNumber)); err != nil {
		return err
	}

	// 更新待确认交易状态
	s.nonceRepo.UpdatePendingTxStatus(ctx, txHash, model.PendingTxStatusConfirmed)
	s.nonceManager.OnTxConfirmed(ctx, uint64(pendingTx.Nonce), txHash)

	// 发送确认通知
	if s.onWithdrawalConfirmed != nil {
		withdrawTx, _ := s.repo.GetByWithdrawID(ctx, withdrawID)
		confirmation := &model.WithdrawalConfirmation{
			WithdrawID:  withdrawID,
			Wallet:      withdrawTx.WalletAddress,
			TxHash:      txHash,
			BlockNumber: int64(blockNumber),
			GasUsed:     int64(gasUsed),
			Status:      "CONFIRMED",
			ConfirmedAt: time.Now().UnixMilli(),
		}

		if err := s.onWithdrawalConfirmed(ctx, confirmation); err != nil {
			logger.Error("failed to send withdrawal confirmation",
				zap.String("withdraw_id", withdrawID),
				zap.Error(err))
		}
	}

	logger.Info("withdrawal confirmed",
		zap.String("withdraw_id", withdrawID),
		zap.String("tx_hash", txHash),
		zap.Uint64("block_number", blockNumber))

	return nil
}

// OnTxFailed 交易失败回调
func (s *WithdrawalService) OnTxFailed(ctx context.Context, txHash string, reason string) error {
	pendingTx, err := s.nonceRepo.GetPendingTxByHash(ctx, txHash)
	if err != nil {
		return err
	}

	if pendingTx.TxType != model.PendingTxTypeWithdrawal {
		return nil
	}

	withdrawID := pendingTx.RefID

	// 更新待确认交易状态
	s.nonceRepo.UpdatePendingTxStatus(ctx, txHash, model.PendingTxStatusFailed)
	s.nonceManager.OnTxFailed(ctx, uint64(pendingTx.Nonce), txHash)

	// 标记提现失败
	withdrawTx, _ := s.repo.GetByWithdrawID(ctx, withdrawID)
	return s.markWithdrawalFailed(ctx, withdrawTx, reason)
}

// GetWithdrawalStatus 获取提现状态
func (s *WithdrawalService) GetWithdrawalStatus(ctx context.Context, withdrawID string) (*model.WithdrawalTx, error) {
	return s.repo.GetByWithdrawID(ctx, withdrawID)
}

// RetryWithdrawal 重试提现
func (s *WithdrawalService) RetryWithdrawal(ctx context.Context, withdrawID string) error {
	withdrawTx, err := s.repo.GetByWithdrawID(ctx, withdrawID)
	if err != nil {
		return err
	}

	if withdrawTx.Status == model.WithdrawalTxStatusConfirmed {
		return errors.New("withdrawal already confirmed")
	}

	// 重置状态
	withdrawTx.Status = model.WithdrawalTxStatusPending
	withdrawTx.ErrorMessage = ""
	if err := s.repo.Update(ctx, withdrawTx); err != nil {
		return err
	}

	return s.submitWithdrawal(ctx, withdrawTx)
}

// ListPendingWithdrawals 列出待处理提现
func (s *WithdrawalService) ListPendingWithdrawals(ctx context.Context, wallet string, page *repository.Pagination) ([]*model.WithdrawalTx, error) {
	return s.repo.ListByWallet(ctx, wallet, page)
}
