package integration

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
)

// TestOrder_CreateBuyOrder 创建买单测试
func TestOrder_CreateBuyOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 先充值 USDT
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建买单
	order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	require.NoError(t, err)
	require.NotNil(t, order)

	// 验证订单
	assert.Equal(t, wallet, order.Wallet)
	assert.Equal(t, market, order.Market)
	assert.Equal(t, model.OrderSideBuy, order.Side)
	assert.Equal(t, model.OrderStatusPending, order.Status)
	assert.True(t, order.Price.Equal(decimal.NewFromFloat(3000)))
	assert.True(t, order.Amount.Equal(decimal.NewFromFloat(1)))

	// 验证冻结金额 (买单冻结 quote token: 3000 USDT)
	balance, err := suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(3000)))
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(7000))) // 10000 - 3000
}

// TestOrder_CreateSellOrder 创建卖单测试
func TestOrder_CreateSellOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 先充值 ETH
	err := suite.DepositForUser(wallet, "ETH", decimal.NewFromFloat(10))
	require.NoError(t, err)

	// 创建卖单
	order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideSell,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3500),
		Amount:        decimal.NewFromFloat(2),
		ClientOrderID: "client-order-2",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	require.NoError(t, err)
	require.NotNil(t, order)

	// 验证订单
	assert.Equal(t, model.OrderSideSell, order.Side)

	// 验证冻结金额 (卖单冻结 base token: 2 ETH)
	balance, err := suite.GetBalance(wallet, "ETH")
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(2)))
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(8))) // 10 - 2
}

// TestOrder_InsufficientBalance 余额不足测试
func TestOrder_InsufficientBalance(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 只充值少量 USDT
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(100))
	require.NoError(t, err)

	// 尝试创建买单 (需要 3000 USDT)
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	assert.Error(t, err)
}

// TestOrder_CancelOrder 取消订单测试
func TestOrder_CancelOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建订单
	order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	require.NoError(t, err)

	// 验证冻结
	balance, err := suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.Equal(decimal.NewFromFloat(3000)))

	// 取消订单
	err = suite.orderSvc.CancelOrder(suite.ctx, wallet, order.OrderID)
	require.NoError(t, err)

	// 验证订单状态
	cancelled, err := suite.orderSvc.GetOrder(suite.ctx, order.OrderID)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusCancelled, cancelled.Status)

	// 验证冻结已释放
	balance, err = suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, balance.SettledFrozen.IsZero())
	assert.True(t, balance.SettledAvailable.Equal(decimal.NewFromFloat(10000)))
}

// TestOrder_GetOrder 查询订单测试
func TestOrder_GetOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建订单
	order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	require.NoError(t, err)

	// 通过 ID 查询
	found, err := suite.orderSvc.GetOrder(suite.ctx, order.OrderID)
	require.NoError(t, err)
	assert.Equal(t, order.OrderID, found.OrderID)

	// 通过 ClientOrderID 查询
	foundByClient, err := suite.orderSvc.GetOrderByClientID(suite.ctx, wallet, "client-order-1")
	require.NoError(t, err)
	assert.Equal(t, order.OrderID, foundByClient.OrderID)
}

// TestOrder_ListOrders 列出订单测试
func TestOrder_ListOrders(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(100000))
	require.NoError(t, err)

	// 创建多个订单
	for i := 0; i < 5; i++ {
		_, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
			Wallet:        wallet,
			Market:        market,
			Side:          model.OrderSideBuy,
			Type:          model.OrderTypeLimit,
			Price:         decimal.NewFromFloat(3000),
			Amount:        decimal.NewFromFloat(0.1),
			ClientOrderID: "",
			Nonce:         uint64(i + 1),
			ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
			Signature:     MockSignature(),
		})
		require.NoError(t, err)
	}

	// 列出订单
	orders, err := suite.orderSvc.ListOrders(suite.ctx, wallet, nil, nil)
	require.NoError(t, err)
	assert.Len(t, orders, 5)
}

// TestOrder_ListOpenOrders 列出活跃订单测试
// 注意: 新创建的订单状态是 PENDING，ListOpenOrders 只返回 OPEN/PARTIAL 状态的订单
// 本测试验证 PENDING 状态订单取消后的行为
func TestOrder_ListOpenOrders(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(100000))
	require.NoError(t, err)

	// 创建多个订单
	var orderIDs []string
	for i := 0; i < 3; i++ {
		order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
			Wallet:        wallet,
			Market:        market,
			Side:          model.OrderSideBuy,
			Type:          model.OrderTypeLimit,
			Price:         decimal.NewFromFloat(3000),
			Amount:        decimal.NewFromFloat(0.1),
			ClientOrderID: "",
			Nonce:         uint64(i + 1),
			ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
			Signature:     MockSignature(),
		})
		require.NoError(t, err)
		orderIDs = append(orderIDs, order.OrderID)
	}

	// 取消一个订单 (PENDING 状态可以直接取消)
	err = suite.orderSvc.CancelOrder(suite.ctx, wallet, orderIDs[0])
	require.NoError(t, err)

	// 列出所有非终态订单 (PENDING 状态)
	// 由于没有真实撮合引擎，订单不会变成 OPEN，所以 ListOpenOrders 返回空
	openOrders, err := suite.orderSvc.ListOpenOrders(suite.ctx, wallet, market)
	require.NoError(t, err)
	assert.Len(t, openOrders, 0) // 没有 OPEN/PARTIAL 状态的订单

	// 使用 ListOrders 验证 PENDING 订单数量 (取消后应该剩 2 个)
	pendingOrders, err := suite.orderSvc.ListOrders(suite.ctx, wallet, &repository.OrderFilter{
		Market:   market,
		Statuses: []model.OrderStatus{model.OrderStatusPending},
	}, nil)
	require.NoError(t, err)
	assert.Len(t, pendingOrders, 2)
}

// TestOrder_InvalidMarket 无效交易对测试
func TestOrder_InvalidMarket(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 尝试在无效交易对下单
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        "INVALID-MARKET",
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	assert.Error(t, err)
}

// TestOrder_AmountBelowMinimum 数量低于最小值测试
func TestOrder_AmountBelowMinimum(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 尝试下单数量低于最小值 (ETH-USDT 最小 0.001)
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(0.0001),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	// 期望返回错误: 数量低于最小值
	assert.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidOrder)
}

// TestOrder_PriceBelowMinimum 价格低于最小值测试
func TestOrder_PriceBelowMinimum(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 尝试下单价格低于最小值 (ETH-USDT 最小价格 1)
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(0.5),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	// 期望返回错误: 价格低于最小值
	assert.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidOrder)
}

// TestOrder_ExpiredOrder 过期订单测试
func TestOrder_ExpiredOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	// 创建一个已过期的订单
	_, err = suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(-1 * time.Hour).UnixMilli(), // 已过期
		Signature:     MockSignature(),
	})
	// 期望返回错误: 订单已过期
	assert.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidOrder)
}

// TestOrder_CancelNonExistentOrder 取消不存在的订单测试
func TestOrder_CancelNonExistentOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"

	// 尝试取消不存在的订单
	err := suite.orderSvc.CancelOrder(suite.ctx, wallet, "non-existent-order-id")
	assert.Error(t, err)
}

// TestOrder_CancelOtherUserOrder 取消其他用户订单测试
func TestOrder_CancelOtherUserOrder(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet1 := "0x1234567890123456789012345678901234567890"
	wallet2 := "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	market := "ETH-USDT"

	// wallet1 充值并下单
	err := suite.DepositForUser(wallet1, "USDT", decimal.NewFromFloat(10000))
	require.NoError(t, err)

	order, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
		Wallet:        wallet1,
		Market:        market,
		Side:          model.OrderSideBuy,
		Type:          model.OrderTypeLimit,
		Price:         decimal.NewFromFloat(3000),
		Amount:        decimal.NewFromFloat(1),
		ClientOrderID: "client-order-1",
		Nonce:         1,
		ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
		Signature:     MockSignature(),
	})
	require.NoError(t, err)

	// wallet2 尝试取消 wallet1 的订单
	err = suite.orderSvc.CancelOrder(suite.ctx, wallet2, order.OrderID)
	assert.Error(t, err)
}

// TestOrder_MultipleOrders 多订单测试
func TestOrder_MultipleOrders(t *testing.T) {
	suite := NewTestSuite(t)
	defer suite.Cleanup()

	wallet := "0x1234567890123456789012345678901234567890"
	market := "ETH-USDT"

	// 充值 USDT 和 ETH
	err := suite.DepositForUser(wallet, "USDT", decimal.NewFromFloat(100000))
	require.NoError(t, err)
	err = suite.DepositForUser(wallet, "ETH", decimal.NewFromFloat(100))
	require.NoError(t, err)

	// 创建多个买单
	for i := 0; i < 3; i++ {
		_, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
			Wallet:        wallet,
			Market:        market,
			Side:          model.OrderSideBuy,
			Type:          model.OrderTypeLimit,
			Price:         decimal.NewFromFloat(3000),
			Amount:        decimal.NewFromFloat(1),
			ClientOrderID: "",
			Nonce:         uint64(i + 1),
			ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
			Signature:     MockSignature(),
		})
		require.NoError(t, err)
	}

	// 创建多个卖单
	for i := 0; i < 2; i++ {
		_, err := suite.orderSvc.CreateOrder(suite.ctx, &service.CreateOrderRequest{
			Wallet:        wallet,
			Market:        market,
			Side:          model.OrderSideSell,
			Type:          model.OrderTypeLimit,
			Price:         decimal.NewFromFloat(3500),
			Amount:        decimal.NewFromFloat(2),
			ClientOrderID: "",
			Nonce:         uint64(i + 100),
			ExpireAt:      time.Now().Add(24 * time.Hour).UnixMilli(),
			Signature:     MockSignature(),
		})
		require.NoError(t, err)
	}

	// 验证冻结金额
	// USDT: 3 * 3000 = 9000
	usdtBalance, err := suite.GetBalance(wallet, "USDT")
	require.NoError(t, err)
	assert.True(t, usdtBalance.SettledFrozen.Equal(decimal.NewFromFloat(9000)))

	// ETH: 2 * 2 = 4
	ethBalance, err := suite.GetBalance(wallet, "ETH")
	require.NoError(t, err)
	assert.True(t, ethBalance.SettledFrozen.Equal(decimal.NewFromFloat(4)))
}
