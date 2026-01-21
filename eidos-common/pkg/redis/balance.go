package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/redis/scripts"
)

var (
	// ErrInsufficientBalance 余额不足
	ErrInsufficientBalance = errors.New("insufficient balance")
	// ErrInsufficientFrozen 冻结余额不足
	ErrInsufficientFrozen = errors.New("insufficient frozen balance")
	// ErrInvalidAmount 无效金额
	ErrInvalidAmount = errors.New("invalid amount")
)

// Balance 余额信息
type Balance struct {
	Available decimal.Decimal
	Frozen    decimal.Decimal
}

// Total 总余额
func (b *Balance) Total() decimal.Decimal {
	return b.Available.Add(b.Frozen)
}

// BalanceManager 余额管理器
type BalanceManager struct {
	client       redis.UniversalClient
	keyPrefix    string
	precision    int32 // 小数精度
}

// NewBalanceManager 创建余额管理器
func NewBalanceManager(client redis.UniversalClient, keyPrefix string, precision int32) *BalanceManager {
	if precision <= 0 {
		precision = 8 // 默认 8 位小数
	}
	return &BalanceManager{
		client:    client,
		keyPrefix: keyPrefix,
		precision: precision,
	}
}

// availableKey 可用余额键
func (m *BalanceManager) availableKey(userID, asset string) string {
	return fmt.Sprintf("%s:%s:%s:available", m.keyPrefix, userID, asset)
}

// frozenKey 冻结余额键
func (m *BalanceManager) frozenKey(userID, asset string) string {
	return fmt.Sprintf("%s:%s:%s:frozen", m.keyPrefix, userID, asset)
}

// formatAmount 格式化金额
func (m *BalanceManager) formatAmount(amount decimal.Decimal) string {
	return amount.StringFixed(m.precision)
}

// GetBalance 获取余额
func (m *BalanceManager) GetBalance(ctx context.Context, userID, asset string) (*Balance, error) {
	script := scripts.BalanceScripts.GetBalance
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
		m.frozenKey(userID, asset),
	}).Slice()
	if err != nil {
		return nil, fmt.Errorf("get balance failed: %w", err)
	}

	available, _ := decimal.NewFromString(result[0].(string))
	frozen, _ := decimal.NewFromString(result[1].(string))

	return &Balance{
		Available: available,
		Frozen:    frozen,
	}, nil
}

// SetBalance 设置可用余额 (仅用于初始化)
func (m *BalanceManager) SetBalance(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() {
		return ErrInvalidAmount
	}

	key := m.availableKey(userID, asset)
	return m.client.Set(ctx, key, m.formatAmount(amount), 0).Err()
}

// IncrBalance 增加可用余额
func (m *BalanceManager) IncrBalance(ctx context.Context, userID, asset string, amount decimal.Decimal) (decimal.Decimal, error) {
	if amount.IsNegative() {
		return decimal.Zero, ErrInvalidAmount
	}

	script := scripts.BalanceScripts.IncrBalance
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
	}, m.formatAmount(amount)).Text()
	if err != nil {
		return decimal.Zero, fmt.Errorf("incr balance failed: %w", err)
	}

	newBalance, _ := decimal.NewFromString(result)
	return newBalance, nil
}

// Freeze 冻结余额
func (m *BalanceManager) Freeze(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.Freeze
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
		m.frozenKey(userID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("freeze failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientBalance
	default:
		return ErrInvalidAmount
	}
}

// Unfreeze 解冻余额
func (m *BalanceManager) Unfreeze(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.Unfreeze
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
		m.frozenKey(userID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("unfreeze failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientFrozen
	default:
		return ErrInvalidAmount
	}
}

// Deduct 从冻结余额中扣减
func (m *BalanceManager) Deduct(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.Deduct
	result, err := m.client.Eval(ctx, script, []string{
		m.frozenKey(userID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("deduct failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientFrozen
	default:
		return ErrInvalidAmount
	}
}

// DeductAvailable 从可用余额中直接扣减
func (m *BalanceManager) DeductAvailable(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.DeductAvailable
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("deduct available failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientBalance
	default:
		return ErrInvalidAmount
	}
}

// FreezeAndDeduct 冻结并扣减 (一步完成)
func (m *BalanceManager) FreezeAndDeduct(ctx context.Context, userID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.FreezeAndDeduct
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("freeze and deduct failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientBalance
	default:
		return ErrInvalidAmount
	}
}

// Transfer 原子转账
func (m *BalanceManager) Transfer(ctx context.Context, fromUserID, toUserID, asset string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.Transfer
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(fromUserID, asset),
		m.availableKey(toUserID, asset),
	}, m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientBalance
	default:
		return ErrInvalidAmount
	}
}

// BatchFreeze 批量冻结
func (m *BalanceManager) BatchFreeze(ctx context.Context, operations []FreezeOperation) (int, error) {
	if len(operations) == 0 {
		return 0, nil
	}

	// 构建键和参数
	keys := make([]string, 0, len(operations)*2)
	args := make([]interface{}, 0, len(operations))

	for _, op := range operations {
		if op.Amount.IsNegative() || op.Amount.IsZero() {
			continue
		}
		keys = append(keys, m.availableKey(op.UserID, op.Asset))
		keys = append(keys, m.frozenKey(op.UserID, op.Asset))
		args = append(args, m.formatAmount(op.Amount))
	}

	if len(keys) == 0 {
		return 0, nil
	}

	script := scripts.BalanceScripts.BatchFreeze
	result, err := m.client.Eval(ctx, script, keys, args...).Int64()
	if err != nil {
		return 0, fmt.Errorf("batch freeze failed: %w", err)
	}

	return int(result), nil
}

// FreezeOperation 冻结操作
type FreezeOperation struct {
	UserID string
	Asset  string
	Amount decimal.Decimal
}

// CompareAndDeduct 比较并扣减
func (m *BalanceManager) CompareAndDeduct(ctx context.Context, userID, asset string, minBalance, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return ErrInvalidAmount
	}

	script := scripts.BalanceScripts.CompareAndDeduct
	result, err := m.client.Eval(ctx, script, []string{
		m.availableKey(userID, asset),
	}, m.formatAmount(minBalance), m.formatAmount(amount)).Int64()
	if err != nil {
		return fmt.Errorf("compare and deduct failed: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrInsufficientBalance
	default:
		return ErrInvalidAmount
	}
}

// MultiAssetBalanceManager 多资产余额管理器
type MultiAssetBalanceManager struct {
	*BalanceManager
}

// NewMultiAssetBalanceManager 创建多资产余额管理器
func NewMultiAssetBalanceManager(client redis.UniversalClient, keyPrefix string, precision int32) *MultiAssetBalanceManager {
	return &MultiAssetBalanceManager{
		BalanceManager: NewBalanceManager(client, keyPrefix, precision),
	}
}

// GetBalances 获取多个资产的余额
func (m *MultiAssetBalanceManager) GetBalances(ctx context.Context, userID string, assets []string) (map[string]*Balance, error) {
	if len(assets) == 0 {
		return make(map[string]*Balance), nil
	}

	pipe := m.client.Pipeline()
	cmds := make(map[string][2]*redis.StringCmd)

	for _, asset := range assets {
		availableCmd := pipe.Get(ctx, m.availableKey(userID, asset))
		frozenCmd := pipe.Get(ctx, m.frozenKey(userID, asset))
		cmds[asset] = [2]*redis.StringCmd{availableCmd, frozenCmd}
	}

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("get balances failed: %w", err)
	}

	balances := make(map[string]*Balance, len(assets))
	for asset, cmd := range cmds {
		availableStr, _ := cmd[0].Result()
		frozenStr, _ := cmd[1].Result()

		available, _ := decimal.NewFromString(availableStr)
		frozen, _ := decimal.NewFromString(frozenStr)

		balances[asset] = &Balance{
			Available: available,
			Frozen:    frozen,
		}
	}

	return balances, nil
}

// SetBalances 批量设置余额
func (m *MultiAssetBalanceManager) SetBalances(ctx context.Context, userID string, balances map[string]decimal.Decimal) error {
	if len(balances) == 0 {
		return nil
	}

	pipe := m.client.Pipeline()

	for asset, amount := range balances {
		if amount.IsNegative() {
			return ErrInvalidAmount
		}
		pipe.Set(ctx, m.availableKey(userID, asset), m.formatAmount(amount), 0)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set balances failed: %w", err)
	}

	return nil
}
