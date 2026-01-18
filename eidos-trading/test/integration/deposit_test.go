package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
)

// TestDeposit_Basic 基础充值测试
func TestDeposit_Basic(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(1000)

	// 使用完整的充值流程 (创建 -> 确认 -> 入账)
	err := suite.DepositForUser(wallet, token, amount)
	require.NoError(t, err)

	// 验证 DB 余额 (充值进入 SettledAvailable)
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(amount))
	assert.True(t, balance.SettledFrozen.IsZero())

	// 验证 Redis 缓存余额
	cacheBalance, err := suite.GetCacheBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, cacheBalance.SettledAvailable.Equal(amount))
	assert.True(t, cacheBalance.SettledFrozen.IsZero())
}

// TestDeposit_MultipleDeposits 多次充值累加测试
func TestDeposit_MultipleDeposits(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"

	// 第一次充值
	amount1 := decimal.NewFromFloat(1000)
	err := suite.DepositForUser(wallet, token, amount1)
	require.NoError(t, err)

	// 第二次充值
	amount2 := decimal.NewFromFloat(2000)
	err = suite.DepositForUser(wallet, token, amount2)
	require.NoError(t, err)

	// 验证总余额
	expectedTotal := amount1.Add(amount2)
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(expectedTotal))

	// 验证 Redis 缓存
	cacheBalance, err := suite.GetCacheBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, cacheBalance.SettledAvailable.Equal(expectedTotal))
}

// TestDeposit_MultipleTokens 多币种充值测试
func TestDeposit_MultipleTokens(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 充值 USDT
	usdtAmount := decimal.NewFromFloat(1000)
	err := suite.DepositForUser(wallet, "USDT", usdtAmount)
	require.NoError(t, err)

	// 充值 ETH
	ethAmount := decimal.NewFromFloat(10)
	err = suite.DepositForUser(wallet, "ETH", ethAmount)
	require.NoError(t, err)

	// 验证 USDT 余额
	usdtBalance, err := suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, usdtBalance.SettledAvailable.Equal(usdtAmount))

	// 验证 ETH 余额
	ethBalance, err := suite.GetBalance(wallet, "ETH")
	require.NoError(t, err)
	assert.True(t, ethBalance.SettledAvailable.Equal(ethAmount))
}

// TestDeposit_DuplicateTxHash 重复交易哈希测试
func TestDeposit_DuplicateTxHash(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(1000)
	// tx_hash 必须是 66 字符
	txHash := "0x" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	// 第一次充值 (完整流程)
	err := suite.depositSvc.ProcessDeposit(suite.ctx, &service.DepositMessage{
		TxHash:      txHash,
		Wallet:      wallet,
		Token:       token,
		Amount:      amount.String(),
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	// 查找充值记录并完成流程
	deposits, err := suite.depositSvc.ListDeposits(suite.ctx, wallet, nil, nil)
	require.NoError(t, err)
	require.Len(t, deposits, 1)
	depositID := deposits[0].DepositID

	err = suite.depositSvc.ConfirmDeposit(suite.ctx, depositID)
	require.NoError(t, err)
	err = suite.depositSvc.CreditDeposit(suite.ctx, depositID)
	require.NoError(t, err)

	// 等待异步任务完成
	time.Sleep(50 * time.Millisecond)

	// 重复充值应该幂等，不增加余额
	err = suite.depositSvc.ProcessDeposit(suite.ctx, &service.DepositMessage{
		TxHash:      txHash,
		Wallet:      wallet,
		Token:       token,
		Amount:      amount.String(),
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	require.NoError(t, err) // 幂等处理，不报错

	// 余额应该只有一次充值的金额
	balance, err := suite.GetBalance(wallet, token)
	require.NoError(t, err)
	assert.True(t, balance.SettledAvailable.Equal(amount), "balance should be %s but got %s", amount, balance.SettledAvailable)
}

// TestDeposit_InvalidToken 无效代币测试
func TestDeposit_InvalidToken(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 尝试充值无效代币
	err := suite.depositSvc.ProcessDeposit(suite.ctx, &service.DepositMessage{
		TxHash:      fmt.Sprintf("0x%064x", time.Now().UnixNano()),
		Wallet:      wallet,
		Token:       "INVALID_TOKEN",
		Amount:      "1000",
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	assert.Error(t, err)
}

// TestDeposit_ZeroAmount 零金额充值测试
func TestDeposit_ZeroAmount(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 尝试零金额充值
	err := suite.depositSvc.ProcessDeposit(suite.ctx, &service.DepositMessage{
		TxHash:      fmt.Sprintf("0x%064x", time.Now().UnixNano()),
		Wallet:      wallet,
		Token:       "USDT",
		Amount:      "0",
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	assert.Error(t, err)
}

// TestDeposit_NegativeAmount 负金额充值测试
func TestDeposit_NegativeAmount(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 尝试负金额充值
	err := suite.depositSvc.ProcessDeposit(suite.ctx, &service.DepositMessage{
		TxHash:      fmt.Sprintf("0x%064x", time.Now().UnixNano()),
		Wallet:      wallet,
		Token:       "USDT",
		Amount:      "-1000",
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	assert.Error(t, err)
}

// TestDeposit_GetDeposit 查询充值记录测试
func TestDeposit_GetDeposit(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	token := "USDT"
	amount := decimal.NewFromFloat(1000)

	// 充值
	err := suite.DepositForUser(wallet, token, amount)
	require.NoError(t, err)

	// 列出充值记录验证
	deposits, err := suite.depositSvc.ListDeposits(suite.ctx, wallet, nil, nil)
	require.NoError(t, err)
	require.Len(t, deposits, 1)
	assert.Equal(t, wallet, deposits[0].Wallet)
	assert.True(t, deposits[0].Amount.Equal(amount))
}

// TestDeposit_ListDeposits 列出充值记录测试
func TestDeposit_ListDeposits(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 创建多笔充值
	for i := 0; i < 5; i++ {
		err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(float64(100*(i+1))))
		require.NoError(t, err)
	}

	// 列出充值记录
	deposits, err := suite.depositSvc.ListDeposits(suite.ctx, wallet, nil, nil)
	require.NoError(t, err)
	assert.Len(t, deposits, 5)
}
