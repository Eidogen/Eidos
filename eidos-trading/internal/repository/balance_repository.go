package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrBalanceNotFound        = errors.New("balance not found")
	ErrInsufficientBalance    = errors.New("insufficient balance")
	ErrBalanceVersionConflict = errors.New("balance version conflict")
)

// BalanceRepository 余额仓储接口
type BalanceRepository interface {
	// Transaction 执行事务
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error

	// GetOrCreate 获取或创建余额记录
	GetOrCreate(ctx context.Context, wallet, token string) (*model.Balance, error)

	// GetByWalletToken 根据钱包和代币查询余额
	GetByWalletToken(ctx context.Context, wallet, token string) (*model.Balance, error)

	// GetByWalletTokenForUpdate 获取余额并加锁 (SELECT FOR UPDATE)
	GetByWalletTokenForUpdate(ctx context.Context, wallet, token string) (*model.Balance, error)

	// ListByWallet 查询用户所有余额
	ListByWallet(ctx context.Context, wallet string) ([]*model.Balance, error)

	// Update 更新余额 (带乐观锁)
	Update(ctx context.Context, balance *model.Balance) error

	// Freeze 冻结余额 (从可用转到冻结)
	// fromSettled: true 表示从已结算可用冻结，false 表示从待结算可用冻结
	Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error

	// Unfreeze 解冻余额 (从冻结转回可用)
	Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error

	// Credit 增加余额 (充值/成交收入)
	Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error

	// Debit 扣减余额 (提现/成交支出)
	// 从冻结部分扣减
	Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error

	// Transfer 转账 (从一个账户到另一个账户)
	Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal) error

	// Settle 结算 (pending → settled)
	Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error

	// CreateBalanceLog 创建余额流水
	CreateBalanceLog(ctx context.Context, log *model.BalanceLog) error

	// ListBalanceLogs 查询余额流水
	ListBalanceLogs(ctx context.Context, wallet string, filter *BalanceLogFilter, page *Pagination) ([]*model.BalanceLog, error)

	// GetFeeAccount 获取手续费账户
	GetFeeAccount(ctx context.Context, bucketID int, token string) (*model.FeeAccount, error)

	// CreditFeeAccount 增加手续费账户余额
	CreditFeeAccount(ctx context.Context, bucketID int, token string, amount decimal.Decimal) error

	// UpsertBalance 插入或更新余额 (用于 Outbox Worker 同步 Redis → DB)
	UpsertBalance(ctx context.Context, balance *model.Balance) error

	// ListBalances 分页查询所有余额记录 (用于对账)
	ListBalances(ctx context.Context, offset, limit int) ([]*model.Balance, error)
}

// BalanceLogFilter 余额流水过滤条件
type BalanceLogFilter struct {
	Token     string                // 代币
	Type      *model.BalanceLogType // 流水类型
	OrderID   string                // 关联订单 ID
	TimeRange *TimeRange            // 时间范围
}

// balanceRepository 余额仓储实现
type balanceRepository struct {
	*Repository
}

// NewBalanceRepository 创建余额仓储
func NewBalanceRepository(db *gorm.DB) BalanceRepository {
	return &balanceRepository{
		Repository: NewRepository(db),
	}
}

// GetOrCreate 获取或创建余额记录
func (r *balanceRepository) GetOrCreate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	var balance model.Balance

	// 先尝试查询
	result := r.DB(ctx).Where("wallet = ? AND token = ?", wallet, token).First(&balance)
	if result.Error == nil {
		return &balance, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("get balance failed: %w", result.Error)
	}

	// 不存在则创建
	balance = model.Balance{
		Wallet:           wallet,
		Token:            token,
		SettledAvailable: decimal.Zero,
		SettledFrozen:    decimal.Zero,
		PendingAvailable: decimal.Zero,
		PendingFrozen:    decimal.Zero,
		PendingTotal:     decimal.Zero,
		Version:          1,
	}

	// 使用 ON CONFLICT 处理并发创建
	result = r.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "wallet"}, {Name: "token"}},
		DoNothing: true,
	}).Create(&balance)

	if result.Error != nil {
		return nil, fmt.Errorf("create balance failed: %w", result.Error)
	}

	// 如果是并发创建导致的冲突，重新查询
	if result.RowsAffected == 0 {
		result = r.DB(ctx).Where("wallet = ? AND token = ?", wallet, token).First(&balance)
		if result.Error != nil {
			return nil, fmt.Errorf("get balance after conflict failed: %w", result.Error)
		}
	}

	return &balance, nil
}

// GetByWalletToken 根据钱包和代币查询余额
func (r *balanceRepository) GetByWalletToken(ctx context.Context, wallet, token string) (*model.Balance, error) {
	var balance model.Balance
	result := r.DB(ctx).Where("wallet = ? AND token = ?", wallet, token).First(&balance)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrBalanceNotFound
		}
		return nil, fmt.Errorf("get balance failed: %w", result.Error)
	}
	return &balance, nil
}

// GetByWalletTokenForUpdate 获取余额并加锁
func (r *balanceRepository) GetByWalletTokenForUpdate(ctx context.Context, wallet, token string) (*model.Balance, error) {
	var balance model.Balance
	result := r.DB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("wallet = ? AND token = ?", wallet, token).
		First(&balance)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrBalanceNotFound
		}
		return nil, fmt.Errorf("get balance for update failed: %w", result.Error)
	}
	return &balance, nil
}

// ListByWallet 查询用户所有余额
func (r *balanceRepository) ListByWallet(ctx context.Context, wallet string) ([]*model.Balance, error) {
	var balances []*model.Balance
	result := r.DB(ctx).Where("wallet = ?", wallet).Find(&balances)
	if result.Error != nil {
		return nil, fmt.Errorf("list balances failed: %w", result.Error)
	}
	return balances, nil
}

// Update 更新余额 (带乐观锁)
func (r *balanceRepository) Update(ctx context.Context, balance *model.Balance) error {
	oldVersion := balance.Version
	balance.Version++
	balance.UpdatedAt = time.Now().UnixMilli()

	result := r.DB(ctx).Model(balance).
		Where("id = ? AND version = ?", balance.ID, oldVersion).
		Updates(balance)

	if result.Error != nil {
		return fmt.Errorf("update balance failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrBalanceVersionConflict
	}
	return nil
}

// Freeze 冻结余额
func (r *balanceRepository) Freeze(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("freeze amount must be positive")
	}

	var sql string
	if fromSettled {
		// 从已结算可用冻结到已结算冻结
		sql = `UPDATE trading_balances
			   SET settled_available = settled_available - ?,
				   settled_frozen = settled_frozen + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND settled_available >= ?
				 AND version = (SELECT version FROM trading_balances WHERE wallet = ? AND token = ?)`
	} else {
		// 从待结算可用冻结到待结算冻结
		sql = `UPDATE trading_balances
			   SET pending_available = pending_available - ?,
				   pending_frozen = pending_frozen + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND pending_available >= ?
				 AND version = (SELECT version FROM trading_balances WHERE wallet = ? AND token = ?)`
	}

	result := r.DB(ctx).Exec(sql, amount, amount, time.Now().UnixMilli(), wallet, token, amount, wallet, token)
	if result.Error != nil {
		return fmt.Errorf("freeze balance failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

// Unfreeze 解冻余额
func (r *balanceRepository) Unfreeze(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("unfreeze amount must be positive")
	}

	var sql string
	if toSettled {
		// 从已结算冻结解冻到已结算可用
		sql = `UPDATE trading_balances
			   SET settled_frozen = settled_frozen - ?,
				   settled_available = settled_available + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND settled_frozen >= ?`
	} else {
		// 从待结算冻结解冻到待结算可用
		sql = `UPDATE trading_balances
			   SET pending_frozen = pending_frozen - ?,
				   pending_available = pending_available + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND pending_frozen >= ?`
	}

	result := r.DB(ctx).Exec(sql, amount, amount, time.Now().UnixMilli(), wallet, token, amount)
	if result.Error != nil {
		return fmt.Errorf("unfreeze balance failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

// Credit 增加余额
func (r *balanceRepository) Credit(ctx context.Context, wallet, token string, amount decimal.Decimal, toSettled bool) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("credit amount must be positive")
	}

	// 先确保记录存在
	_, err := r.GetOrCreate(ctx, wallet, token)
	if err != nil {
		return err
	}

	var sql string
	if toSettled {
		// 增加已结算可用
		sql = `UPDATE trading_balances
			   SET settled_available = settled_available + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?`
	} else {
		// 增加待结算可用
		sql = `UPDATE trading_balances
			   SET pending_available = pending_available + ?,
				   pending_total = pending_total + ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?`
	}

	var result *gorm.DB
	if toSettled {
		result = r.DB(ctx).Exec(sql, amount, time.Now().UnixMilli(), wallet, token)
	} else {
		result = r.DB(ctx).Exec(sql, amount, amount, time.Now().UnixMilli(), wallet, token)
	}

	if result.Error != nil {
		return fmt.Errorf("credit balance failed: %w", result.Error)
	}
	return nil
}

// Debit 扣减余额 (从冻结部分)
func (r *balanceRepository) Debit(ctx context.Context, wallet, token string, amount decimal.Decimal, fromSettled bool) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("debit amount must be positive")
	}

	var sql string
	if fromSettled {
		// 从已结算冻结扣减
		sql = `UPDATE trading_balances
			   SET settled_frozen = settled_frozen - ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND settled_frozen >= ?`
	} else {
		// 从待结算冻结扣减
		sql = `UPDATE trading_balances
			   SET pending_frozen = pending_frozen - ?,
				   pending_total = pending_total - ?,
				   version = version + 1,
				   updated_at = ?
			   WHERE wallet = ? AND token = ?
				 AND pending_frozen >= ?`
	}

	var result *gorm.DB
	if fromSettled {
		result = r.DB(ctx).Exec(sql, amount, time.Now().UnixMilli(), wallet, token, amount)
	} else {
		result = r.DB(ctx).Exec(sql, amount, amount, time.Now().UnixMilli(), wallet, token, amount)
	}

	if result.Error != nil {
		return fmt.Errorf("debit balance failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

// Transfer 转账
func (r *balanceRepository) Transfer(ctx context.Context, fromWallet, toWallet, token string, amount decimal.Decimal) error {
	return r.Transaction(ctx, func(txCtx context.Context) error {
		// 从发送方扣减
		if err := r.Debit(txCtx, fromWallet, token, amount, true); err != nil {
			return fmt.Errorf("debit from sender failed: %w", err)
		}

		// 增加到接收方
		if err := r.Credit(txCtx, toWallet, token, amount, true); err != nil {
			return fmt.Errorf("credit to receiver failed: %w", err)
		}

		return nil
	})
}

// Settle 结算 (pending → settled)
func (r *balanceRepository) Settle(ctx context.Context, wallet, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("settle amount must be positive")
	}

	// 从待结算可用转移到已结算可用
	sql := `UPDATE trading_balances
		   SET pending_available = pending_available - ?,
			   pending_total = pending_total - ?,
			   settled_available = settled_available + ?,
			   version = version + 1,
			   updated_at = ?
		   WHERE wallet = ? AND token = ?
			 AND pending_available >= ?`

	result := r.DB(ctx).Exec(sql, amount, amount, amount, time.Now().UnixMilli(), wallet, token, amount)
	if result.Error != nil {
		return fmt.Errorf("settle balance failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInsufficientBalance
	}
	return nil
}

// CreateBalanceLog 创建余额流水
func (r *balanceRepository) CreateBalanceLog(ctx context.Context, log *model.BalanceLog) error {
	result := r.DB(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(log)

	if result.Error != nil {
		return fmt.Errorf("create balance log failed: %w", result.Error)
	}
	return nil
}

// ListBalanceLogs 查询余额流水
func (r *balanceRepository) ListBalanceLogs(ctx context.Context, wallet string, filter *BalanceLogFilter, page *Pagination) ([]*model.BalanceLog, error) {
	db := r.DB(ctx).Where("wallet = ?", wallet)

	if filter != nil {
		if filter.Token != "" {
			db = db.Where("token = ?", filter.Token)
		}
		if filter.Type != nil {
			db = db.Where("type = ?", *filter.Type)
		}
		if filter.OrderID != "" {
			db = db.Where("order_id = ?", filter.OrderID)
		}
		if filter.TimeRange != nil && filter.TimeRange.IsValid() {
			db = db.Where("created_at >= ? AND created_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
		}
	}

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.BalanceLog{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count balance logs failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var logs []*model.BalanceLog
	db = db.Order("created_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("list balance logs failed: %w", err)
	}
	return logs, nil
}

// GetFeeAccount 获取手续费账户
func (r *balanceRepository) GetFeeAccount(ctx context.Context, bucketID int, token string) (*model.FeeAccount, error) {
	var account model.FeeAccount
	result := r.DB(ctx).Where("bucket_id = ? AND token = ?", bucketID, token).First(&account)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 创建新账户
			account = model.FeeAccount{
				BucketID: bucketID,
				Token:    token,
				Balance:  decimal.Zero,
				Version:  1,
			}
			if err := r.DB(ctx).Create(&account).Error; err != nil {
				return nil, fmt.Errorf("create fee account failed: %w", err)
			}
			return &account, nil
		}
		return nil, fmt.Errorf("get fee account failed: %w", result.Error)
	}
	return &account, nil
}

// CreditFeeAccount 增加手续费账户余额
func (r *balanceRepository) CreditFeeAccount(ctx context.Context, bucketID int, token string, amount decimal.Decimal) error {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil
	}

	sql := `UPDATE trading_fee_accounts
		   SET balance = balance + ?,
			   version = version + 1,
			   updated_at = ?
		   WHERE bucket_id = ? AND token = ?`

	result := r.DB(ctx).Exec(sql, amount, time.Now().UnixMilli(), bucketID, token)
	if result.Error != nil {
		return fmt.Errorf("credit fee account failed: %w", result.Error)
	}

	// 如果没有更新到行，创建新账户
	if result.RowsAffected == 0 {
		_, err := r.GetFeeAccount(ctx, bucketID, token)
		if err != nil {
			return err
		}
		// 重试更新
		result = r.DB(ctx).Exec(sql, amount, time.Now().UnixMilli(), bucketID, token)
		if result.Error != nil {
			return fmt.Errorf("credit fee account retry failed: %w", result.Error)
		}
	}

	return nil
}

// UpsertBalance 插入或更新余额 (用于 Outbox Worker 同步 Redis → DB)
func (r *balanceRepository) UpsertBalance(ctx context.Context, balance *model.Balance) error {
	now := time.Now().UnixMilli()
	balance.UpdatedAt = now

	// 使用 UPSERT，如果存在则更新，不存在则插入
	result := r.DB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "wallet"}, {Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"settled_available",
			"settled_frozen",
			"pending_available",
			"pending_frozen",
			"pending_total",
			"version",
			"updated_at",
		}),
	}).Create(balance)

	if result.Error != nil {
		return fmt.Errorf("upsert balance failed: %w", result.Error)
	}
	return nil
}

// ListBalances 分页查询所有余额记录 (用于对账)
func (r *balanceRepository) ListBalances(ctx context.Context, offset, limit int) ([]*model.Balance, error) {
	var balances []*model.Balance
	result := r.DB(ctx).
		Order("id ASC").
		Offset(offset).
		Limit(limit).
		Find(&balances)

	if result.Error != nil {
		return nil, fmt.Errorf("list balances failed: %w", result.Error)
	}
	return balances, nil
}
