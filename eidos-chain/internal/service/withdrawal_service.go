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
//  1. ProcessWithdrawalRequest() 接收提现请求
//  2. 验证 EIP-712 签名
//  3. 调用 Vault 合约执行提现
//  4. 监听交易确认
//
// ## 消息输出 (Kafka Producer)
// - Topic: withdrawal-confirmed (发送给 eidos-trading)
// - 消息类型: model.WithdrawalConfirmation
// - 触发条件: 链上交易确认后回调 onWithdrawalConfirmed
//
// ## eidos-trading 对接
// 1. eidos-trading 的提现服务需要发送 withdrawals 消息
//   - 触发时机: 提现请求审核通过后
//   - 消息格式: model.WithdrawalRequest (包含 withdraw_id, wallet, to_address, amount, signature)
//   - 提现状态: APPROVED -> PENDING (等待上链)
//
// 2. eidos-trading 需要订阅 withdrawal-confirmed 消息
//   - 更新提现状态: PENDING -> COMPLETED
//   - 记录 tx_hash, block_number
//   - 释放冻结余额
//
// ## 智能合约对接
// - Vault 合约用于执行提现
// - 使用 internal/contract/vault.go 提供的 ABI 绑定
// - EIP-712 签名用于验证用户授权
//
// ========================================
package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/eidos-exchange/eidos/eidos-chain/internal/blockchain"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/contract"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/model"
	"github.com/eidos-exchange/eidos/eidos-chain/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/crypto"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"
)

var (
	ErrWithdrawalNotFound   = errors.New("withdrawal not found")
	ErrWithdrawalInProgress = errors.New("withdrawal already in progress")
	ErrInsufficientBalance  = errors.New("insufficient balance for withdrawal")
	ErrInvalidSignature     = errors.New("invalid withdrawal signature")
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

	// 合约绑定
	vault        *contract.VaultContract
	gasEstimator *contract.GasEstimator
	tokenReg     *contract.TokenRegistry

	// EIP-712 配置
	eip712Domain crypto.EIP712Domain

	// 事件回调
	onWithdrawalConfirmed func(ctx context.Context, confirmation *model.WithdrawalConfirmation) error
}

// WithdrawalServiceConfig 配置
type WithdrawalServiceConfig struct {
	MaxRetries        int
	RetryBackoff      time.Duration
	ChainID           int64
	VaultContract     common.Address
	EIP712DomainName  string
	EIP712Version     string
	SkipSigValidation bool // 用于测试环境跳过签名验证
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

	// 设置 EIP-712 domain
	domainName := cfg.EIP712DomainName
	if domainName == "" {
		domainName = "EidosExchange"
	}
	domainVersion := cfg.EIP712Version
	if domainVersion == "" {
		domainVersion = "1"
	}

	eip712Domain := crypto.EIP712Domain{
		Name:              domainName,
		Version:           domainVersion,
		ChainID:           cfg.ChainID,
		VerifyingContract: cfg.VaultContract.Hex(),
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
		eip712Domain:  eip712Domain,
	}
}

// SetVaultContract 设置 Vault 合约绑定
func (s *WithdrawalService) SetVaultContract(vault *contract.VaultContract) {
	s.vault = vault
}

// SetGasEstimator 设置 Gas 估算器
func (s *WithdrawalService) SetGasEstimator(estimator *contract.GasEstimator) {
	s.gasEstimator = estimator
}

// SetTokenRegistry 设置 Token 注册表
func (s *WithdrawalService) SetTokenRegistry(reg *contract.TokenRegistry) {
	s.tokenReg = reg
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

	// 验证 EIP-712 签名
	if err := s.verifyWithdrawalSignature(req); err != nil {
		logger.Error("withdrawal signature verification failed",
			zap.String("withdraw_id", req.WithdrawID),
			zap.String("wallet", req.Wallet),
			zap.Error(err))
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

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
	return s.submitWithdrawal(ctx, tx, req.Signature)
}

// verifyWithdrawalSignature 验证提现签名
func (s *WithdrawalService) verifyWithdrawalSignature(req *model.WithdrawalRequest) error {
	// 检查 mock 模式
	if crypto.IsMockMode(s.eip712Domain) {
		logger.Warn("skipping signature verification in mock mode",
			zap.String("withdraw_id", req.WithdrawID))
		return nil
	}

	// 检查签名是否为空
	if req.Signature == "" {
		return errors.New("signature is empty")
	}

	// 解析签名
	sig, err := parseSignature(req.Signature)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	// 构建提现数据
	withdrawData := crypto.WithdrawData{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    req.Amount.BigInt(),
		ToAddress: req.ToAddress,
		Nonce:     uint64(req.Nonce),
	}

	// 验证签名
	valid, err := crypto.VerifyWithdrawalSignature(s.eip712Domain, withdrawData, sig)
	if err != nil {
		return fmt.Errorf("signature verification error: %w", err)
	}

	if !valid {
		return errors.New("signature verification failed")
	}

	return nil
}

// parseSignature 解析十六进制签名字符串
func parseSignature(sigHex string) ([]byte, error) {
	sigHex = strings.TrimPrefix(sigHex, "0x")
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex signature: %w", err)
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("invalid signature length: expected 65, got %d", len(sig))
	}
	return sig, nil
}

// ProcessPendingWithdrawals 处理待提交的提现
func (s *WithdrawalService) ProcessPendingWithdrawals(ctx context.Context) error {
	txs, err := s.repo.ListPending(ctx, 10)
	if err != nil {
		return err
	}

	for _, tx := range txs {
		// 从数据库获取原始请求的签名
		// 注意：签名需要在创建时存储或从其他地方获取
		if err := s.submitWithdrawal(ctx, tx, ""); err != nil {
			logger.Error("failed to submit withdrawal",
				zap.String("withdraw_id", tx.WithdrawID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// submitWithdrawal 提交提现到链上
func (s *WithdrawalService) submitWithdrawal(ctx context.Context, withdrawTx *model.WithdrawalTx, signature string) error {
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
	tx, err := s.buildWithdrawalTx(ctx, withdrawTx, nonce, signature)
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
func (s *WithdrawalService) buildWithdrawalTx(ctx context.Context, withdrawTx *model.WithdrawalTx, nonce uint64, signature string) (*types.Transaction, error) {
	// 构建提现参数
	params := &contract.WithdrawParams{
		User:   common.HexToAddress(withdrawTx.WalletAddress),
		Token:  common.HexToAddress(withdrawTx.TokenAddress),
		Amount: withdrawTx.Amount.BigInt(),
		To:     common.HexToAddress(withdrawTx.ToAddress),
		Nonce:  big.NewInt(0), // 从请求中获取的 nonce
	}

	// 解析签名
	if signature != "" {
		sig, err := parseSignature(signature)
		if err != nil {
			return nil, fmt.Errorf("failed to parse signature: %w", err)
		}
		params.Signature = sig
	} else {
		// 如果没有签名，使用空签名（仅适用于测试环境）
		params.Signature = make([]byte, 65)
	}

	// 编码数据
	var data []byte
	var err error

	if s.vault != nil {
		data, err = s.vault.PackWithdraw(params)
		if err != nil {
			return nil, fmt.Errorf("failed to pack withdrawal: %w", err)
		}
	} else {
		// 降级：直接使用 ABI 编码
		data, err = s.encodeWithdrawalData(params)
		if err != nil {
			return nil, fmt.Errorf("failed to encode withdrawal: %w", err)
		}
	}

	// Gas 估算
	var gasLimit uint64
	var gasPrice *big.Int

	if s.gasEstimator != nil {
		estimate, err := s.gasEstimator.EstimateWithdrawalGas(
			ctx,
			s.client.Address(),
			s.vaultContract,
			data,
		)
		if err != nil {
			logger.Warn("gas estimation failed, using fallback",
				zap.String("withdraw_id", withdrawTx.WithdrawID),
				zap.Error(err))
			gasLimit = s.calculateFallbackGas()
			gasPrice, _ = s.client.SuggestGasPrice(ctx)
		} else {
			gasLimit = estimate.GasLimit
			gasPrice = estimate.GasPrice.GasPrice
		}
	} else {
		gasLimit = s.calculateFallbackGas()
		gasPrice, err = s.client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get gas price: %w", err)
		}
	}

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

// encodeWithdrawalData 使用 ABI 编码提现数据
func (s *WithdrawalService) encodeWithdrawalData(params *contract.WithdrawParams) ([]byte, error) {
	if s.vault != nil {
		return s.vault.PackWithdraw(params)
	}
	// 降级：如果没有合约绑定，返回空数据（用于 mock 模式）
	return []byte{}, nil
}

// calculateFallbackGas 计算降级 gas 估算
func (s *WithdrawalService) calculateFallbackGas() uint64 {
	// 基础 gas: 80,000
	// 安全余量: 20%
	baseGas := uint64(80000)
	return baseGas * 120 / 100 // 20% buffer
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

	return s.submitWithdrawal(ctx, withdrawTx, "")
}

// ListPendingWithdrawals 列出待处理提现
func (s *WithdrawalService) ListPendingWithdrawals(ctx context.Context, wallet string, page *repository.Pagination) ([]*model.WithdrawalTx, error) {
	return s.repo.ListByWallet(ctx, wallet, page)
}

// CheckOnChainWithdrawalStatus 检查链上提现状态
func (s *WithdrawalService) CheckOnChainWithdrawalStatus(ctx context.Context, user common.Address, nonce *big.Int) (bool, error) {
	if s.vault == nil {
		return false, errors.New("vault contract not configured")
	}

	return s.vault.IsWithdrawalProcessed(ctx, user, nonce)
}

// GetOnChainBalance 获取用户链上余额
func (s *WithdrawalService) GetOnChainBalance(ctx context.Context, user, token common.Address) (*big.Int, error) {
	if s.vault == nil {
		return nil, errors.New("vault contract not configured")
	}

	return s.vault.GetBalance(ctx, user, token)
}
