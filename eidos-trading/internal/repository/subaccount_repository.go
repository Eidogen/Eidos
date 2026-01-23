package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrSubAccountNotFound        = errors.New("sub-account not found")
	ErrSubAccountExists          = errors.New("sub-account already exists")
	ErrSubAccountLimitExceeded   = errors.New("sub-account limit exceeded")
	ErrSubAccountFrozen          = errors.New("sub-account is frozen")
	ErrSubAccountBalanceNotFound = errors.New("sub-account balance not found")
	ErrSubAccountVersionConflict = errors.New("sub-account balance version conflict")
)

// SubAccountRepository 子账户仓储接口
type SubAccountRepository interface {
	// Transaction 执行事务
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error

	// --- 子账户管理 ---

	// Create 创建子账户
	Create(ctx context.Context, subAccount *model.SubAccount) error

	// GetByID 根据子账户 ID 查询
	GetByID(ctx context.Context, subAccountID string) (*model.SubAccount, error)

	// GetByWalletAndName 根据钱包和名称查询
	GetByWalletAndName(ctx context.Context, wallet, name string) (*model.SubAccount, error)

	// ListByWallet 查询钱包下所有子账户
	ListByWallet(ctx context.Context, wallet string) ([]*model.SubAccount, error)

	// CountByWallet 统计钱包下子账户数量
	CountByWallet(ctx context.Context, wallet string) (int64, error)

	// Update 更新子账户
	Update(ctx context.Context, subAccount *model.SubAccount) error

	// Delete 删除子账户 (软删除，标记为 DELETED 状态)
	Delete(ctx context.Context, subAccountID string) error

	// GetDefaultByWallet 获取钱包的默认子账户
	GetDefaultByWallet(ctx context.Context, wallet string) (*model.SubAccount, error)

	// SetDefault 设置默认子账户
	SetDefault(ctx context.Context, wallet, subAccountID string) error

	// --- 子账户余额 ---

	// GetBalance 获取子账户余额
	GetBalance(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error)

	// GetBalanceForUpdate 获取子账户余额并加锁
	GetBalanceForUpdate(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error)

	// GetOrCreateBalance 获取或创建子账户余额
	GetOrCreateBalance(ctx context.Context, subAccountID, wallet, token string) (*model.SubAccountBalance, error)

	// ListBalancesBySubAccount 查询子账户所有余额
	ListBalancesBySubAccount(ctx context.Context, subAccountID string) ([]*model.SubAccountBalance, error)

	// ListBalancesByWallet 查询钱包下所有子账户余额
	ListBalancesByWallet(ctx context.Context, wallet string) ([]*model.SubAccountBalance, error)

	// UpdateBalance 更新子账户余额 (带乐观锁)
	UpdateBalance(ctx context.Context, balance *model.SubAccountBalance) error

	// FreezeBalance 冻结子账户余额
	FreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error

	// UnfreezeBalance 解冻子账户余额
	UnfreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error

	// CreditBalance 增加子账户余额
	CreditBalance(ctx context.Context, subAccountID, wallet, token string, amount decimal.Decimal) error

	// DebitBalance 扣减子账户余额 (从冻结部分)
	DebitBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error

	// --- 划转记录 ---

	// CreateTransfer 创建划转记录
	CreateTransfer(ctx context.Context, transfer *model.SubAccountTransfer) error

	// ListTransfers 查询划转记录
	ListTransfers(ctx context.Context, wallet string, filter *SubAccountTransferFilter, page *Pagination) ([]*model.SubAccountTransfer, error)

	// --- 余额流水 ---

	// CreateBalanceLog 创建余额流水
	CreateBalanceLog(ctx context.Context, log *model.SubAccountBalanceLog) error

	// ListBalanceLogs 查询余额流水
	ListBalanceLogs(ctx context.Context, subAccountID string, filter *BalanceLogFilter, page *Pagination) ([]*model.SubAccountBalanceLog, error)
}

// SubAccountTransferFilter 划转记录过滤条件
type SubAccountTransferFilter struct {
	SubAccountID string                         // 子账户 ID
	Token        string                         // 代币
	Type         *model.SubAccountTransferType  // 划转类型
	TimeRange    *TimeRange                     // 时间范围
}

// subAccountRepository 子账户仓储实现
type subAccountRepository struct {
	*Repository
}

// NewSubAccountRepository 创建子账户仓储
func NewSubAccountRepository(db *gorm.DB) SubAccountRepository {
	return &subAccountRepository{
		Repository: NewRepository(db),
	}
}

// --- 子账户管理实现 ---

func (r *subAccountRepository) Create(ctx context.Context, subAccount *model.SubAccount) error {
	// 检查数量限制
	count, err := r.CountByWallet(ctx, subAccount.Wallet)
	if err != nil {
		return err
	}
	if count >= model.MaxSubAccountsPerWallet {
		return ErrSubAccountLimitExceeded
	}

	// 检查名称是否重复
	existing, err := r.GetByWalletAndName(ctx, subAccount.Wallet, subAccount.Name)
	if err != nil && !errors.Is(err, ErrSubAccountNotFound) {
		return err
	}
	if existing != nil {
		return ErrSubAccountExists
	}

	return r.DB(ctx).Create(subAccount).Error
}

func (r *subAccountRepository) GetByID(ctx context.Context, subAccountID string) (*model.SubAccount, error) {
	var subAccount model.SubAccount
	err := r.DB(ctx).Where("sub_account_id = ? AND status != ?", subAccountID, model.SubAccountStatusDeleted).First(&subAccount).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &subAccount, nil
}

func (r *subAccountRepository) GetByWalletAndName(ctx context.Context, wallet, name string) (*model.SubAccount, error) {
	var subAccount model.SubAccount
	err := r.DB(ctx).Where("wallet = ? AND name = ? AND status != ?", wallet, name, model.SubAccountStatusDeleted).First(&subAccount).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &subAccount, nil
}

func (r *subAccountRepository) ListByWallet(ctx context.Context, wallet string) ([]*model.SubAccount, error) {
	var subAccounts []*model.SubAccount
	err := r.DB(ctx).Where("wallet = ? AND status != ?", wallet, model.SubAccountStatusDeleted).
		Order("is_default DESC, created_at ASC").
		Find(&subAccounts).Error
	return subAccounts, err
}

func (r *subAccountRepository) CountByWallet(ctx context.Context, wallet string) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&model.SubAccount{}).
		Where("wallet = ? AND status != ?", wallet, model.SubAccountStatusDeleted).
		Count(&count).Error
	return count, err
}

func (r *subAccountRepository) Update(ctx context.Context, subAccount *model.SubAccount) error {
	return r.DB(ctx).Save(subAccount).Error
}

func (r *subAccountRepository) Delete(ctx context.Context, subAccountID string) error {
	return r.DB(ctx).Model(&model.SubAccount{}).
		Where("sub_account_id = ?", subAccountID).
		Update("status", model.SubAccountStatusDeleted).Error
}

func (r *subAccountRepository) GetDefaultByWallet(ctx context.Context, wallet string) (*model.SubAccount, error) {
	var subAccount model.SubAccount
	err := r.DB(ctx).Where("wallet = ? AND is_default = ? AND status = ?", wallet, true, model.SubAccountStatusActive).First(&subAccount).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &subAccount, nil
}

func (r *subAccountRepository) SetDefault(ctx context.Context, wallet, subAccountID string) error {
	return r.Transaction(ctx, func(ctx context.Context) error {
		// 先取消原来的默认
		if err := r.DB(ctx).Model(&model.SubAccount{}).
			Where("wallet = ? AND is_default = ?", wallet, true).
			Update("is_default", false).Error; err != nil {
			return err
		}

		// 设置新的默认
		return r.DB(ctx).Model(&model.SubAccount{}).
			Where("sub_account_id = ?", subAccountID).
			Update("is_default", true).Error
	})
}

// --- 子账户余额实现 ---

func (r *subAccountRepository) GetBalance(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error) {
	var balance model.SubAccountBalance
	err := r.DB(ctx).Where("sub_account_id = ? AND token = ?", subAccountID, token).First(&balance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubAccountBalanceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

func (r *subAccountRepository) GetBalanceForUpdate(ctx context.Context, subAccountID, token string) (*model.SubAccountBalance, error) {
	var balance model.SubAccountBalance
	err := r.DB(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("sub_account_id = ? AND token = ?", subAccountID, token).First(&balance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubAccountBalanceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

func (r *subAccountRepository) GetOrCreateBalance(ctx context.Context, subAccountID, wallet, token string) (*model.SubAccountBalance, error) {
	var balance model.SubAccountBalance
	err := r.DB(ctx).Where("sub_account_id = ? AND token = ?", subAccountID, token).First(&balance).Error
	if err == nil {
		return &balance, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 创建新的余额记录
	balance = model.SubAccountBalance{
		SubAccountID: subAccountID,
		Wallet:       wallet,
		Token:        token,
		Available:    decimal.Zero,
		Frozen:       decimal.Zero,
		Version:      1,
	}
	if err := r.DB(ctx).Create(&balance).Error; err != nil {
		// 可能存在并发创建，重新查询
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return r.GetBalance(ctx, subAccountID, token)
		}
		return nil, err
	}
	return &balance, nil
}

func (r *subAccountRepository) ListBalancesBySubAccount(ctx context.Context, subAccountID string) ([]*model.SubAccountBalance, error) {
	var balances []*model.SubAccountBalance
	err := r.DB(ctx).Where("sub_account_id = ?", subAccountID).Find(&balances).Error
	return balances, err
}

func (r *subAccountRepository) ListBalancesByWallet(ctx context.Context, wallet string) ([]*model.SubAccountBalance, error) {
	var balances []*model.SubAccountBalance
	err := r.DB(ctx).Where("wallet = ?", wallet).Find(&balances).Error
	return balances, err
}

func (r *subAccountRepository) UpdateBalance(ctx context.Context, balance *model.SubAccountBalance) error {
	result := r.DB(ctx).Model(balance).
		Where("id = ? AND version = ?", balance.ID, balance.Version).
		Updates(map[string]interface{}{
			"available":  balance.Available,
			"frozen":     balance.Frozen,
			"version":    balance.Version + 1,
			"updated_at": balance.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSubAccountVersionConflict
	}
	balance.Version++
	return nil
}

func (r *subAccountRepository) FreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("freeze amount must be positive")
	}

	balance, err := r.GetBalanceForUpdate(ctx, subAccountID, token)
	if err != nil {
		return err
	}

	if balance.Available.LessThan(amount) {
		return ErrInsufficientBalance
	}

	balance.Available = balance.Available.Sub(amount)
	balance.Frozen = balance.Frozen.Add(amount)

	return r.UpdateBalance(ctx, balance)
}

func (r *subAccountRepository) UnfreezeBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("unfreeze amount must be positive")
	}

	balance, err := r.GetBalanceForUpdate(ctx, subAccountID, token)
	if err != nil {
		return err
	}

	if balance.Frozen.LessThan(amount) {
		return fmt.Errorf("frozen balance insufficient: %s < %s", balance.Frozen.String(), amount.String())
	}

	balance.Frozen = balance.Frozen.Sub(amount)
	balance.Available = balance.Available.Add(amount)

	return r.UpdateBalance(ctx, balance)
}

func (r *subAccountRepository) CreditBalance(ctx context.Context, subAccountID, wallet, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("credit amount must be positive")
	}

	balance, err := r.GetOrCreateBalance(ctx, subAccountID, wallet, token)
	if err != nil {
		return err
	}

	// 重新加锁获取
	balance, err = r.GetBalanceForUpdate(ctx, subAccountID, token)
	if err != nil {
		return err
	}

	balance.Available = balance.Available.Add(amount)

	return r.UpdateBalance(ctx, balance)
}

func (r *subAccountRepository) DebitBalance(ctx context.Context, subAccountID, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("debit amount must be positive")
	}

	balance, err := r.GetBalanceForUpdate(ctx, subAccountID, token)
	if err != nil {
		return err
	}

	if balance.Frozen.LessThan(amount) {
		return fmt.Errorf("frozen balance insufficient: %s < %s", balance.Frozen.String(), amount.String())
	}

	balance.Frozen = balance.Frozen.Sub(amount)

	return r.UpdateBalance(ctx, balance)
}

// --- 划转记录实现 ---

func (r *subAccountRepository) CreateTransfer(ctx context.Context, transfer *model.SubAccountTransfer) error {
	return r.DB(ctx).Create(transfer).Error
}

func (r *subAccountRepository) ListTransfers(ctx context.Context, wallet string, filter *SubAccountTransferFilter, page *Pagination) ([]*model.SubAccountTransfer, error) {
	query := r.DB(ctx).Model(&model.SubAccountTransfer{}).Where("wallet = ?", wallet)

	if filter != nil {
		if filter.SubAccountID != "" {
			query = query.Where("sub_account_id = ?", filter.SubAccountID)
		}
		if filter.Token != "" {
			query = query.Where("token = ?", filter.Token)
		}
		if filter.Type != nil {
			query = query.Where("type = ?", *filter.Type)
		}
		if filter.TimeRange != nil && filter.TimeRange.IsValid() {
			query = query.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
		}
	}

	// 查询总数
	if page != nil {
		if err := query.Count(&page.Total).Error; err != nil {
			return nil, err
		}
		query = query.Offset(page.Offset()).Limit(page.Limit())
	}

	var transfers []*model.SubAccountTransfer
	err := query.Order("created_at DESC").Find(&transfers).Error
	return transfers, err
}

// --- 余额流水实现 ---

func (r *subAccountRepository) CreateBalanceLog(ctx context.Context, log *model.SubAccountBalanceLog) error {
	return r.DB(ctx).Create(log).Error
}

func (r *subAccountRepository) ListBalanceLogs(ctx context.Context, subAccountID string, filter *BalanceLogFilter, page *Pagination) ([]*model.SubAccountBalanceLog, error) {
	query := r.DB(ctx).Model(&model.SubAccountBalanceLog{}).Where("sub_account_id = ?", subAccountID)

	if filter != nil {
		if filter.Token != "" {
			query = query.Where("token = ?", filter.Token)
		}
		if filter.Type != nil {
			query = query.Where("type = ?", *filter.Type)
		}
		if filter.OrderID != "" {
			query = query.Where("order_id = ?", filter.OrderID)
		}
		if filter.TimeRange != nil && filter.TimeRange.IsValid() {
			query = query.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
		}
	}

	// 查询总数
	if page != nil {
		if err := query.Count(&page.Total).Error; err != nil {
			return nil, err
		}
		query = query.Offset(page.Offset()).Limit(page.Limit())
	}

	var logs []*model.SubAccountBalanceLog
	err := query.Order("created_at DESC").Find(&logs).Error
	return logs, err
}
