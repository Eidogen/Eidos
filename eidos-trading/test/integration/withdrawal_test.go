package integration

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
)

// TestWithdrawal_Basic 基础提现测试
func TestWithdrawal_Basic(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	depositAmount := decimal.NewFromFloat(10000)
	withdrawAmount := decimal.NewFromFloat(5000)

	// 先充值
	err := suite.DepositForUser(wallet, token, depositAmount)
	require.NoError(t, err)

	// 创建提现
	withdrawal, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    withdrawAmount,
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	require.NoError(t, err)
	require.NotNil(t, withdrawal)

	// 验证提现记录
	assert.Equal(t, wallet, withdrawal.Wallet)
	assert.Equal(t, token, withdrawal.Token)
	assert.True(t, withdrawal.Amount.Equal(withdrawAmount))
	assert.Equal(t, model.WithdrawStatusPending, withdrawal.Status)

	// 验证余额被冻结
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.Equal(withdrawAmount))
	assert.True(t, balance.SettledAvailable.Equal(depositAmount.Sub(withdrawAmount)))
}

// TestWithdrawal_InsufficientBalance 余额不足提现测试
func TestWithdrawal_InsufficientBalance(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 充值少量
	err := suite.DepositForUser(wallet, token, decimal.NewFromFloat(100))
	require.NoError(t, err)

	// 尝试提现超过余额
	_, err = suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    decimal.NewFromFloat(1000),
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	assert.Error(t, err)
}

// TestWithdrawal_Confirm 提现确认测试
func TestWithdrawal_Confirm(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	depositAmount := decimal.NewFromFloat(10000)
	withdrawAmount := decimal.NewFromFloat(5000)

	// 充值
	err := suite.DepositForUser(wallet, token, depositAmount)
	require.NoError(t, err)

	// 创建提现
	withdrawal, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    withdrawAmount,
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	require.NoError(t, err)

	// 等待异步 DB 写入完成
	time.Sleep(100 * time.Millisecond)

	// 先审批通过
	err = suite.withdrawSvc.ApproveWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)

	// 提交到链上
	err = suite.withdrawSvc.SubmitWithdrawal(suite.ctx, withdrawal.WithdrawID, "0xtxhash123")
	require.NoError(t, err)

	// 确认提现
	err = suite.withdrawSvc.ConfirmWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)

	// 验证提现状态
	confirmed, err := suite.withdrawSvc.GetWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)
	assert.Equal(t, model.WithdrawStatusConfirmed, confirmed.Status)

	// 验证冻结已释放，余额扣除
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.IsZero())
	assert.True(t, balance.SettledAvailable.Equal(depositAmount.Sub(withdrawAmount)))
}

// TestWithdrawal_Fail 提现失败测试
func TestWithdrawal_Fail(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	depositAmount := decimal.NewFromFloat(10000)
	withdrawAmount := decimal.NewFromFloat(5000)

	// 充值
	err := suite.DepositForUser(wallet, token, depositAmount)
	require.NoError(t, err)

	// 创建提现
	withdrawal, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    withdrawAmount,
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	require.NoError(t, err)

	// 等待异步 DB 写入完成
	time.Sleep(100 * time.Millisecond)

	// 先审批通过
	err = suite.withdrawSvc.ApproveWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)

	// 提交到链上
	err = suite.withdrawSvc.SubmitWithdrawal(suite.ctx, withdrawal.WithdrawID, "0xtxhash456")
	require.NoError(t, err)

	// 提现失败
	err = suite.withdrawSvc.FailWithdrawal(suite.ctx, withdrawal.WithdrawID, "chain transaction failed")
	require.NoError(t, err)

	// 验证提现状态
	failed, err := suite.withdrawSvc.GetWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)
	assert.Equal(t, model.WithdrawStatusFailed, failed.Status)

	// 验证冻结已释放，余额恢复
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.IsZero())
	assert.True(t, balance.SettledAvailable.Equal(depositAmount)) // 全部恢复
}

// TestWithdrawal_InvalidToken 无效代币提现测试
func TestWithdrawal_InvalidToken(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 尝试提现无效代币
	_, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     "INVALID_TOKEN",
		Amount:    decimal.NewFromFloat(1000),
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	assert.Error(t, err)
}

// TestWithdrawal_ZeroAmount 零金额提现测试
func TestWithdrawal_ZeroAmount(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(1000))
	require.NoError(t, err)

	// 尝试零金额提现
	_, err = suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     "USDT",
		Amount:    decimal.Zero,
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	assert.Error(t, err)
}

// TestWithdrawal_GetWithdrawal 查询提现记录测试
func TestWithdrawal_GetWithdrawal(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(1000)

	// 充值
	err := suite.DepositForUser(wallet, token, decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建提现
	withdrawal, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     token,
		Amount:    amount,
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     1,
		Signature: MockSignature(),
	})
	require.NoError(t, err)

	// 等待异步 DB 写入完成
	time.Sleep(100 * time.Millisecond)

	// 查询提现记录
	found, err := suite.withdrawSvc.GetWithdrawal(suite.ctx, withdrawal.WithdrawID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, withdrawal.WithdrawID, found.WithdrawID)
	assert.Equal(t, wallet, found.Wallet)
	assert.True(t, found.Amount.Equal(amount))
}

// TestWithdrawal_ListWithdrawals 列出提现记录测试
func TestWithdrawal_ListWithdrawals(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 充值
	err := suite.DepositForUser(wallet, token, decimal.NewFromFloat(100000))
	require.NoError(t, err)

	// 创建多笔提现
	// 注意: 每笔提现后等待异步 DB 写入完成，避免并发冲突
	for i := 0; i < 5; i++ {
		_, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
			Wallet:    wallet,
			Token:     token,
			Amount:    decimal.NewFromFloat(1000),
			ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			Nonce:     uint64(i + 1),
			Signature: MockSignature(),
		})
		require.NoError(t, err)
		// 等待异步 DB 写入完成
		time.Sleep(100 * time.Millisecond)
	}

	// 列出提现记录
	withdrawals, err := suite.withdrawSvc.ListWithdrawals(suite.ctx, wallet, nil, nil)
	require.NoError(t, err)
	assert.Len(t, withdrawals, 5)
}

// TestWithdrawal_WithPendingOrder 有挂单时提现测试
func TestWithdrawal_WithPendingOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建买单，冻结 3000 USDT
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:    wallet,
		Market:    market,
		Side:      model.OrderSideBuy,
		Type:      model.OrderTypeLimit,
		Price:     decimal.NewFromFloat(3000),
		Amount:    decimal.NewFromFloat(1),
		Nonce:     1,
		ExpireAt:  9999999999999,
		Signature: MockSignature(),
	})
	require.NoError(t, err)

	// 验证可用余额
	balance, err := suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(7000)))
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(3000)))

	// 尝试提现超过可用余额
	_, err = suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(8000), // 超过可用的 7000
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     2,
		Signature: MockSignature(),
	})
	assert.Error(t, err)

	// 提现可用余额内的金额应该成功
	withdrawal, err := suite.withdrawSvc.CreateWithdrawal(suite.ctx, &service.CreateWithdrawalRequest{
		Wallet:    wallet,
		Token:     "USDT",
		Amount:    decimal.NewFromFloat(5000), // 5000 < 7000
		ToAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Nonce:     3,
		Signature: MockSignature(),
	})
	require.NoError(t, err)
	require.NotNil(t, withdrawal)

	// 验证冻结金额累加
	balance, err = suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(2000))) // 7000 - 5000
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(8000)))    // 3000 + 5000
}
