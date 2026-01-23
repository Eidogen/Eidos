// Package service 子账户服务
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

var (
	ErrSubAccountNameInvalid   = errors.New("sub-account name invalid")
	ErrSubAccountCannotTrade   = errors.New("sub-account cannot trade")
	ErrSubAccountCannotDelete  = errors.New("sub-account cannot be deleted")
	ErrMainAccountNotFound     = errors.New("main account balance not found")
	ErrInsufficientMainBalance = errors.New("insufficient main account balance")
)

// SubAccountService 子账户服务接口
type SubAccountService interface {
	// --- 子账户管理 ---

	// CreateSubAccount 创建子账户
	CreateSubAccount(ctx context.Context, wallet, name string, accountType model.SubAccountType) (*model.SubAccount, error)

	// GetSubAccount 获取子账户
	GetSubAccount(ctx context.Context, subAccountID string) (*model.SubAccount, error)

	// ListSubAccounts 获取钱包下所有子账户
	ListSubAccounts(ctx context.Context, wallet string) ([]*model.SubAccount, error)

	// UpdateSubAccount 更新子账户 (名称、备注)
	UpdateSubAccount(ctx context.Context, subAccountID, name, remark string) error

	// DeleteSubAccount 删除子账户
	DeleteSubAccount(ctx context.Context, subAccountID string) error

	// FreezeSubAccount 冻结子账户
	FreezeSubAccount(ctx context.Context, subAccountID, reason string) error

	// UnfreezeSubAccount 解冻子账户
	UnfreezeSubAccount(ctx context.Context, subAccountID string) error

	// SetDefaultSubAccount 设置默认子账户
	SetDefaultSubAccount(ctx context.Context, wallet, subAccountID string) error

	// GetDefaultSubAccount 获取默认子账户
	GetDefaultSubAccount(ctx context.Context, wallet string) (*model.SubAccount, error)

	// --- 余额管理 ---

	// GetSubAccountBalance 获取子账户单个代币余额
	GetSubAccountBalance(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error)

	// GetSubAccountBalances 获取子账户所有余额
	GetSubAccountBalances(ctx context.Context, subAccountID string) ([]*model.SubAccountBalance, error)

	// GetAllSubAccountBalances 获取钱包下所有子账户的余额
	GetAllSubAccountBalances(ctx context.Context, wallet string) ([]*model.SubAccountBalance, error)

	// --- 划转 ---

	// TransferIn 划入子账户 (主账户 → 子账户)
	TransferIn(ctx context.Context, wallet, subAccountID, token string, amount decimal.Decimal, remark string) error

	// TransferOut 划出子账户 (子账户 → 主账户)
	TransferOut(ctx context.Context, wallet, subAccountID, token string, amount decimal.Decimal, remark string) error

	// GetTransferHistory 获取划转历史
	GetTransferHistory(ctx context.Context, wallet string, filter *repository.SubAccountTransferFilter, page *repository.Pagination) ([]*model.SubAccountTransfer, error)

	// --- 交易操作 (供订单服务调用) ---

	// FreezeBalance 冻结子账户余额 (下单)
	FreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, orderID string) error

	// UnfreezeBalance 解冻子账户余额 (取消订单)
	UnfreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, orderID string) error

	// DebitFrozen 扣减冻结余额 (成交)
	DebitFrozen(ctx context.Context, subAccountID, token string, amount decimal.Decimal, tradeID string) error

	// CreditBalance 增加余额 (成交收入)
	CreditBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, tradeID string) error
}

// subAccountService 子账户服务实现
type subAccountService struct {
	subAccountRepo repository.SubAccountRepository
	balanceRepo    repository.BalanceRepository
	tokenConfig    TokenConfigProvider
}

// NewSubAccountService 创建子账户服务
func NewSubAccountService(
	subAccountRepo repository.SubAccountRepository,
	balanceRepo repository.BalanceRepository,
	tokenConfig TokenConfigProvider,
) SubAccountService {
	return &subAccountService{
		subAccountRepo: subAccountRepo,
		balanceRepo:    balanceRepo,
		tokenConfig:    tokenConfig,
	}
}

// --- 子账户管理实现 ---

func (s *subAccountService) CreateSubAccount(ctx context.Context, wallet, name string, accountType model.SubAccountType) (*model.SubAccount, error) {
	// 验证名称
	name = strings.TrimSpace(name)
	if len(name) < model.SubAccountNameMinLen || len(name) > model.SubAccountNameMaxLen {
		return nil, fmt.Errorf("%w: name length must be between %d and %d",
			ErrSubAccountNameInvalid, model.SubAccountNameMinLen, model.SubAccountNameMaxLen)
	}

	// 生成子账户 ID
	subAccountID := s.generateSubAccountID(wallet)

	subAccount := &model.SubAccount{
		SubAccountID: subAccountID,
		Wallet:       wallet,
		Name:         name,
		Type:         accountType,
		Status:       model.SubAccountStatusActive,
		IsDefault:    false,
	}

	// 检查是否是第一个子账户，如果是则设为默认
	count, err := s.subAccountRepo.CountByWallet(ctx, wallet)
	if err != nil {
		return nil, fmt.Errorf("count sub-accounts: %w", err)
	}
	if count == 0 {
		subAccount.IsDefault = true
	}

	if err := s.subAccountRepo.Create(ctx, subAccount); err != nil {
		if errors.Is(err, repository.ErrSubAccountLimitExceeded) {
			return nil, fmt.Errorf("子账户数量已达上限 (%d)", model.MaxSubAccountsPerWallet)
		}
		if errors.Is(err, repository.ErrSubAccountExists) {
			return nil, fmt.Errorf("子账户名称 '%s' 已存在", name)
		}
		return nil, fmt.Errorf("create sub-account: %w", err)
	}

	logger.Info("子账户创建成功",
		"wallet", wallet,
		"sub_account_id", subAccountID,
		"name", name,
		"type", accountType.String())

	return subAccount, nil
}

func (s *subAccountService) generateSubAccountID(wallet string) string {
	// 格式: 钱包前8位 + UUID
	prefix := wallet
	if len(wallet) > 10 {
		prefix = wallet[2:10] // 去掉 0x 前缀，取前8位
	}
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

func (s *subAccountService) GetSubAccount(ctx context.Context, subAccountID string) (*model.SubAccount, error) {
	return s.subAccountRepo.GetByID(ctx, subAccountID)
}

func (s *subAccountService) ListSubAccounts(ctx context.Context, wallet string) ([]*model.SubAccount, error) {
	return s.subAccountRepo.ListByWallet(ctx, wallet)
}

func (s *subAccountService) UpdateSubAccount(ctx context.Context, subAccountID, name, remark string) error {
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	if name != "" {
		name = strings.TrimSpace(name)
		if len(name) < model.SubAccountNameMinLen || len(name) > model.SubAccountNameMaxLen {
			return fmt.Errorf("%w: name length must be between %d and %d",
				ErrSubAccountNameInvalid, model.SubAccountNameMinLen, model.SubAccountNameMaxLen)
		}
		subAccount.Name = name
	}

	if remark != "" {
		subAccount.Remark = remark
	}

	return s.subAccountRepo.Update(ctx, subAccount)
}

func (s *subAccountService) DeleteSubAccount(ctx context.Context, subAccountID string) error {
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	// 不能删除默认子账户
	if subAccount.IsDefault {
		return fmt.Errorf("%w: cannot delete default sub-account", ErrSubAccountCannotDelete)
	}

	// 检查是否有余额
	balances, err := s.subAccountRepo.ListBalancesBySubAccount(ctx, subAccountID)
	if err != nil {
		return fmt.Errorf("list balances: %w", err)
	}

	for _, balance := range balances {
		if balance.Total().GreaterThan(decimal.Zero) {
			return fmt.Errorf("%w: sub-account has remaining balance", ErrSubAccountCannotDelete)
		}
	}

	return s.subAccountRepo.Delete(ctx, subAccountID)
}

func (s *subAccountService) FreezeSubAccount(ctx context.Context, subAccountID, reason string) error {
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	subAccount.Status = model.SubAccountStatusFrozen
	subAccount.Remark = reason

	logger.Info("子账户已冻结",
		"sub_account_id", subAccountID,
		"reason", reason)

	return s.subAccountRepo.Update(ctx, subAccount)
}

func (s *subAccountService) UnfreezeSubAccount(ctx context.Context, subAccountID string) error {
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	subAccount.Status = model.SubAccountStatusActive
	subAccount.Remark = ""

	logger.Info("子账户已解冻", "sub_account_id", subAccountID)

	return s.subAccountRepo.Update(ctx, subAccount)
}

func (s *subAccountService) SetDefaultSubAccount(ctx context.Context, wallet, subAccountID string) error {
	// 验证子账户存在且属于该钱包
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}
	if subAccount.Wallet != wallet {
		return errors.New("sub-account does not belong to this wallet")
	}

	return s.subAccountRepo.SetDefault(ctx, wallet, subAccountID)
}

func (s *subAccountService) GetDefaultSubAccount(ctx context.Context, wallet string) (*model.SubAccount, error) {
	return s.subAccountRepo.GetDefaultByWallet(ctx, wallet)
}

// --- 余额管理实现 ---

func (s *subAccountService) GetSubAccountBalance(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error) {
	return s.subAccountRepo.GetBalance(ctx, subAccountID, token)
}

func (s *subAccountService) GetSubAccountBalances(ctx context.Context, subAccountID string) ([]*model.SubAccountBalance, error) {
	return s.subAccountRepo.ListBalancesBySubAccount(ctx, subAccountID)
}

func (s *subAccountService) GetAllSubAccountBalances(ctx context.Context, wallet string) ([]*model.SubAccountBalance, error) {
	return s.subAccountRepo.ListBalancesByWallet(ctx, wallet)
}

// --- 划转实现 ---

func (s *subAccountService) TransferIn(ctx context.Context, wallet, subAccountID, token string, amount decimal.Decimal, remark string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	if !s.tokenConfig.IsValidToken(token) {
		return ErrInvalidToken
	}

	// 验证子账户
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}
	if subAccount.Wallet != wallet {
		return errors.New("sub-account does not belong to this wallet")
	}
	if !subAccount.CanTransfer() {
		return ErrSubAccountCannotTrade
	}

	transferID := uuid.New().String()

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		// 从主账户扣减 (使用 settled_available)
		mainBalance, err := s.balanceRepo.GetByWalletTokenForUpdate(ctx, wallet, token)
		if err != nil {
			if errors.Is(err, repository.ErrBalanceNotFound) {
				return ErrMainAccountNotFound
			}
			return fmt.Errorf("get main balance: %w", err)
		}

		if mainBalance.SettledAvailable.LessThan(amount) {
			return ErrInsufficientMainBalance
		}

		mainBalance.SettledAvailable = mainBalance.SettledAvailable.Sub(amount)
		if err := s.balanceRepo.Update(ctx, mainBalance); err != nil {
			return fmt.Errorf("update main balance: %w", err)
		}

		// 增加子账户余额
		if err := s.subAccountRepo.CreditBalance(ctx, subAccountID, wallet, token, amount); err != nil {
			return fmt.Errorf("credit sub-account: %w", err)
		}

		// 创建划转记录
		transfer := &model.SubAccountTransfer{
			TransferID:   transferID,
			Wallet:       wallet,
			SubAccountID: subAccountID,
			Type:         model.SubAccountTransferTypeIn,
			Token:        token,
			Amount:       amount,
			Remark:       remark,
		}
		if err := s.subAccountRepo.CreateTransfer(ctx, transfer); err != nil {
			return fmt.Errorf("create transfer record: %w", err)
		}

		// 创建主账户流水
		mainLog := &model.BalanceLog{
			Wallet:        wallet,
			Token:         token,
			Type:          model.BalanceLogTypeWithdraw, // 划出视为提现类型
			Amount:        amount.Neg(),
			BalanceBefore: mainBalance.SettledAvailable.Add(amount),
			BalanceAfter:  mainBalance.SettledAvailable,
			Remark:        fmt.Sprintf("划转到子账户 %s", subAccountID),
		}
		if err := s.balanceRepo.CreateBalanceLog(ctx, mainLog); err != nil {
			return fmt.Errorf("create main balance log: %w", err)
		}

		// 创建子账户流水
		subBalance, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)
		subLog := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        wallet,
			Token:         token,
			Type:          model.BalanceLogTypeDeposit, // 划入视为充值类型
			Amount:        amount,
			BalanceBefore: subBalance.Available.Sub(amount),
			BalanceAfter:  subBalance.Available,
			TransferID:    transferID,
			Remark:        remark,
		}
		if err := s.subAccountRepo.CreateBalanceLog(ctx, subLog); err != nil {
			return fmt.Errorf("create sub-account balance log: %w", err)
		}

		logger.Info("资金划入子账户",
			"wallet", wallet,
			"sub_account_id", subAccountID,
			"token", token,
			"amount", amount.String(),
			"transfer_id", transferID)

		return nil
	})
}

func (s *subAccountService) TransferOut(ctx context.Context, wallet, subAccountID, token string, amount decimal.Decimal, remark string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	if !s.tokenConfig.IsValidToken(token) {
		return ErrInvalidToken
	}

	// 验证子账户
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}
	if subAccount.Wallet != wallet {
		return errors.New("sub-account does not belong to this wallet")
	}
	if !subAccount.CanTransfer() {
		return ErrSubAccountCannotTrade
	}

	transferID := uuid.New().String()

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		// 获取子账户余额并检查
		subBalance, err := s.subAccountRepo.GetBalanceForUpdate(ctx, subAccountID, token)
		if err != nil {
			if errors.Is(err, repository.ErrSubAccountBalanceNotFound) {
				return repository.ErrInsufficientBalance
			}
			return fmt.Errorf("get sub-account balance: %w", err)
		}

		if subBalance.Available.LessThan(amount) {
			return repository.ErrInsufficientBalance
		}

		// 扣减子账户余额
		subBalance.Available = subBalance.Available.Sub(amount)
		if err := s.subAccountRepo.UpdateBalance(ctx, subBalance); err != nil {
			return fmt.Errorf("update sub-account balance: %w", err)
		}

		// 增加主账户余额
		if err := s.balanceRepo.Credit(ctx, wallet, token, amount, true); err != nil {
			return fmt.Errorf("credit main account: %w", err)
		}

		// 创建划转记录
		transfer := &model.SubAccountTransfer{
			TransferID:   transferID,
			Wallet:       wallet,
			SubAccountID: subAccountID,
			Type:         model.SubAccountTransferTypeOut,
			Token:        token,
			Amount:       amount,
			Remark:       remark,
		}
		if err := s.subAccountRepo.CreateTransfer(ctx, transfer); err != nil {
			return fmt.Errorf("create transfer record: %w", err)
		}

		// 创建子账户流水
		subLog := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        wallet,
			Token:         token,
			Type:          model.BalanceLogTypeWithdraw,
			Amount:        amount.Neg(),
			BalanceBefore: subBalance.Available.Add(amount),
			BalanceAfter:  subBalance.Available,
			TransferID:    transferID,
			Remark:        remark,
		}
		if err := s.subAccountRepo.CreateBalanceLog(ctx, subLog); err != nil {
			return fmt.Errorf("create sub-account balance log: %w", err)
		}

		logger.Info("资金划出子账户",
			"wallet", wallet,
			"sub_account_id", subAccountID,
			"token", token,
			"amount", amount.String(),
			"transfer_id", transferID)

		return nil
	})
}

func (s *subAccountService) GetTransferHistory(ctx context.Context, wallet string, filter *repository.SubAccountTransferFilter, page *repository.Pagination) ([]*model.SubAccountTransfer, error) {
	return s.subAccountRepo.ListTransfers(ctx, wallet, filter, page)
}

// --- 交易操作实现 ---

func (s *subAccountService) FreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, orderID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// 验证子账户状态
	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}
	if !subAccount.CanTrade() {
		return ErrSubAccountCannotTrade
	}

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		// 冻结余额
		if err := s.subAccountRepo.FreezeBalance(ctx, subAccountID, token, amount); err != nil {
			return err
		}

		// 记录流水
		balance, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)
		log := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        subAccount.Wallet,
			Token:         token,
			Type:          model.BalanceLogTypeFreeze,
			Amount:        amount.Neg(),
			BalanceBefore: balance.Available.Add(amount),
			BalanceAfter:  balance.Available,
			OrderID:       orderID,
			Remark:        "下单冻结",
		}
		return s.subAccountRepo.CreateBalanceLog(ctx, log)
	})
}

func (s *subAccountService) UnfreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, orderID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		// 解冻余额
		if err := s.subAccountRepo.UnfreezeBalance(ctx, subAccountID, token, amount); err != nil {
			return err
		}

		// 记录流水
		balance, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)
		log := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        subAccount.Wallet,
			Token:         token,
			Type:          model.BalanceLogTypeUnfreeze,
			Amount:        amount,
			BalanceBefore: balance.Available.Sub(amount),
			BalanceAfter:  balance.Available,
			OrderID:       orderID,
			Remark:        "取消订单解冻",
		}
		return s.subAccountRepo.CreateBalanceLog(ctx, log)
	})
}

func (s *subAccountService) DebitFrozen(ctx context.Context, subAccountID, token string, amount decimal.Decimal, tradeID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		balanceBefore, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)

		// 从冻结部分扣减
		if err := s.subAccountRepo.DebitBalance(ctx, subAccountID, token, amount); err != nil {
			return err
		}

		balanceAfter, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)

		// 记录流水
		log := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        subAccount.Wallet,
			Token:         token,
			Type:          model.BalanceLogTypeTrade,
			Amount:        amount.Neg(),
			BalanceBefore: balanceBefore.Total(),
			BalanceAfter:  balanceAfter.Total(),
			TradeID:       tradeID,
			Remark:        "成交扣款",
		}
		return s.subAccountRepo.CreateBalanceLog(ctx, log)
	})
}

func (s *subAccountService) CreditBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal, tradeID string) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	subAccount, err := s.subAccountRepo.GetByID(ctx, subAccountID)
	if err != nil {
		return err
	}

	return s.subAccountRepo.Transaction(ctx, func(ctx context.Context) error {
		balanceBefore, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)
		beforeTotal := decimal.Zero
		if balanceBefore != nil {
			beforeTotal = balanceBefore.Total()
		}

		// 增加可用余额
		if err := s.subAccountRepo.CreditBalance(ctx, subAccountID, subAccount.Wallet, token, amount); err != nil {
			return err
		}

		balanceAfter, _ := s.subAccountRepo.GetBalance(ctx, subAccountID, token)

		// 记录流水
		log := &model.SubAccountBalanceLog{
			SubAccountID:  subAccountID,
			Wallet:        subAccount.Wallet,
			Token:         token,
			Type:          model.BalanceLogTypeTrade,
			Amount:        amount,
			BalanceBefore: beforeTotal,
			BalanceAfter:  balanceAfter.Total(),
			TradeID:       tradeID,
			Remark:        "成交收入",
			CreatedAt:     time.Now().UnixMilli(),
		}
		return s.subAccountRepo.CreateBalanceLog(ctx, log)
	})
}
