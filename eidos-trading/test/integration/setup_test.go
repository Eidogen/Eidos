// Package integration 提供集成测试
//
// 运行方式: INTEGRATION_TEST=1 go test ./test/integration/... -v -p=1
// 注意: 使用 -p=1 顺序执行测试以避免 SQLite 并发锁冲突
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/id"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
)

// TestSuite 集成测试套件
type TestSuite struct {
	t   *testing.T
	ctx context.Context

	// 基础设施
	db        *gorm.DB
	rdb       *redis.Client
	minirdb   *miniredis.Miniredis
	idGen     *id.Generator
	marketCfg *MockMarketConfig
	tokenCfg  *MockTokenConfig
	riskCfg   *MockRiskConfig

	// 仓储层
	orderRepo    repository.OrderRepository
	balanceRepo  repository.BalanceRepository
	tradeRepo    repository.TradeRepository
	depositRepo  repository.DepositRepository
	withdrawRepo repository.WithdrawalRepository
	nonceRepo    repository.NonceRepository
	outboxRepo   *repository.OutboxRepository

	// 缓存层
	balanceCache cache.BalanceRedisRepository

	// 服务层
	orderSvc    service.OrderService
	balanceSvc  service.BalanceService
	tradeSvc    service.TradeService
	depositSvc  service.DepositService
	withdrawSvc service.WithdrawalService
	clearingSvc service.ClearingService
}

// NewTestSuite 创建测试套件
func NewTestSuite(t *testing.T) *TestSuite {
	t.Helper()

	suite := &TestSuite{
		t:   t,
		ctx: context.Background(),
	}

	// 初始化 miniredis
	suite.minirdb = miniredis.RunT(t)
	suite.rdb = redis.NewClient(&redis.Options{
		Addr: suite.minirdb.Addr(),
	})

	// 初始化 SQLite (in-memory)
	// 注意: SQLite 不支持真正的并发写入，集成测试应顺序执行 (-p=1)
	var err error
	suite.db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// 自动迁移
	if err := suite.db.AutoMigrate(
		&model.Order{},
		&model.Balance{},
		&model.BalanceLog{},
		&model.FeeAccount{},
		&model.Trade{},
		&model.Deposit{},
		&model.Withdrawal{},
		&model.UsedNonce{},
		&model.OutboxMessage{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// 初始化 ID 生成器
	suite.idGen, err = id.NewGenerator(1)
	if err != nil {
		t.Fatalf("failed to create id generator: %v", err)
	}

	// 初始化配置提供者
	suite.marketCfg = NewMockMarketConfig()
	suite.tokenCfg = NewMockTokenConfig()
	suite.riskCfg = NewMockRiskConfig()

	// 初始化仓储层
	suite.orderRepo = repository.NewOrderRepository(suite.db)
	suite.balanceRepo = repository.NewBalanceRepository(suite.db)
	suite.tradeRepo = repository.NewTradeRepository(suite.db)
	suite.depositRepo = repository.NewDepositRepository(suite.db)
	suite.withdrawRepo = repository.NewWithdrawalRepository(suite.db)
	suite.nonceRepo = repository.NewNonceRepository(suite.db, suite.rdb)
	suite.outboxRepo = repository.NewOutboxRepository(suite.db)

	// 初始化缓存层
	suite.balanceCache = cache.NewBalanceRedisRepository(suite.rdb)

	// 初始化服务层
	suite.orderSvc = service.NewOrderService(
		suite.orderRepo,
		suite.balanceRepo,
		suite.nonceRepo,
		suite.balanceCache,
		suite.idGen,
		suite.marketCfg,
		suite.riskCfg,
		nil, // no kafka producer in tests
		nil, // no order publisher in tests
	)

	suite.balanceSvc = service.NewBalanceService(
		suite.balanceRepo,
		suite.outboxRepo,
		suite.balanceCache,
		suite.tokenCfg,
		nil, // no balance publisher in tests
	)

	suite.tradeSvc = service.NewTradeService(
		suite.tradeRepo,
		suite.balanceRepo,
		suite.balanceCache,
		suite.idGen,
		suite.marketCfg,
	)

	suite.depositSvc = service.NewDepositService(
		suite.depositRepo,
		suite.balanceRepo,
		suite.balanceCache,
		suite.idGen,
		suite.tokenCfg,
	)

	suite.withdrawSvc = service.NewWithdrawalService(
		suite.withdrawRepo,
		suite.balanceRepo,
		suite.balanceCache,
		suite.nonceRepo,
		suite.idGen,
		suite.tokenCfg,
		nil, // no withdrawal publisher in tests
	)

	suite.clearingSvc = service.NewClearingService(
		suite.db,
		suite.tradeRepo,
		suite.orderRepo,
		suite.balanceRepo,
		suite.balanceCache,
		suite.marketCfg,
		nil, // no order publisher in tests
		nil, // no balance publisher in tests
		nil, // no settlement publisher in tests
	)

	return suite
}

// Cleanup 清理测试资源
func (s *TestSuite) Cleanup() {
	// 等待异步任务完成，避免后台 goroutine 访问已关闭的数据库
	// 使用较长的等待时间确保所有异步 DB 写入完成
	time.Sleep(300 * time.Millisecond)

	if s.rdb != nil {
		s.rdb.Close()
	}
	if s.minirdb != nil {
		s.minirdb.Close()
	}
}

// DepositForUser 为用户充值 (完整流程：创建 -> 确认 -> 入账)
func (s *TestSuite) DepositForUser(wallet, token string, amount decimal.Decimal) error {
	// tx_hash 必须是 66 字符 (0x + 64 hex chars)
	txHash := fmt.Sprintf("0x%064x", time.Now().UnixNano())

	// 1. 创建充值记录
	err := s.depositSvc.ProcessDeposit(s.ctx, &service.DepositMessage{
		TxHash:      txHash,
		Wallet:      wallet,
		Token:       token,
		Amount:      amount.String(),
		BlockNumber: 12345678,
		Timestamp:   time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}

	// 2. 查找刚创建的充值记录
	deposits, err := s.depositSvc.ListDeposits(s.ctx, wallet, nil, nil)
	if err != nil {
		return err
	}
	if len(deposits) == 0 {
		return fmt.Errorf("deposit not found after creation")
	}

	// 找到最新的充值记录 (通过 txHash)
	var depositID string
	for _, d := range deposits {
		if d.TxHash == txHash {
			depositID = d.DepositID
			break
		}
	}
	if depositID == "" {
		return fmt.Errorf("deposit with txHash %s not found", txHash)
	}

	// 3. 确认充值
	if err := s.depositSvc.ConfirmDeposit(s.ctx, depositID); err != nil {
		return err
	}

	// 4. 入账
	if err := s.depositSvc.CreditDeposit(s.ctx, depositID); err != nil {
		return err
	}

	// 5. 等待异步任务完成 (DB 写入)
	time.Sleep(50 * time.Millisecond)

	return nil
}

// GetBalance 获取用户余额
func (s *TestSuite) GetBalance(wallet, token string) (*model.Balance, error) {
	return s.balanceSvc.GetBalance(s.ctx, wallet, token)
}

// GetCacheBalance 从 Redis 获取缓存余额
func (s *TestSuite) GetCacheBalance(wallet, token string) (*cache.RedisBalance, error) {
	return s.balanceCache.GetBalance(s.ctx, wallet, token)
}

// CreateOrder 创建订单（使用 mock 签名）
func (s *TestSuite) CreateOrder(wallet, market string, side model.OrderSide, orderType model.OrderType, price, amount decimal.Decimal, nonce uint64) (*model.Order, error) {
	return s.orderSvc.CreateOrder(s.ctx, &service.CreateOrderRequest{
		Wallet:    wallet,
		Market:    market,
		Side:      side,
		Type:      orderType,
		Price:     price,
		Amount:    amount,
		Nonce:     nonce,
		Signature: MockSignature(),
		ExpireAt:  time.Now().Add(24 * time.Hour).UnixMilli(),
	})
}

// MockSignature 返回一个 65 字节的模拟签名
func MockSignature() []byte {
	sig := make([]byte, 65)
	for i := range sig {
		sig[i] = byte(i)
	}
	return sig
}

// ========== Mock Config Providers ==========

// MockMarketConfig 模拟交易对配置
type MockMarketConfig struct {
	markets map[string]*service.MarketConfig
}

func NewMockMarketConfig() *MockMarketConfig {
	return &MockMarketConfig{
		markets: map[string]*service.MarketConfig{
			"ETH-USDT": {
				Market:          "ETH-USDT",
				BaseToken:       "ETH",
				QuoteToken:      "USDT",
				MinAmount:       decimal.NewFromFloat(0.001),
				MaxAmount:       decimal.NewFromFloat(1000),
				MinPrice:        decimal.NewFromFloat(1),
				MaxPrice:        decimal.NewFromFloat(100000),
				PricePrecision:  2,
				AmountPrecision: 6,
				MakerFeeRate:    decimal.Zero, // 设为 0 简化测试
				TakerFeeRate:    decimal.Zero, // 设为 0 简化测试
				Status:          1,
			},
			"BTC-USDT": {
				Market:          "BTC-USDT",
				BaseToken:       "BTC",
				QuoteToken:      "USDT",
				MinAmount:       decimal.NewFromFloat(0.0001),
				MaxAmount:       decimal.NewFromFloat(100),
				MinPrice:        decimal.NewFromFloat(1),
				MaxPrice:        decimal.NewFromFloat(1000000),
				PricePrecision:  2,
				AmountPrecision: 8,
				MakerFeeRate:    decimal.Zero, // 设为 0 简化测试
				TakerFeeRate:    decimal.Zero, // 设为 0 简化测试
				Status:          1,
			},
		},
	}
}

func (m *MockMarketConfig) GetMarket(market string) (*service.MarketConfig, error) {
	if cfg, ok := m.markets[market]; ok {
		return cfg, nil
	}
	return nil, fmt.Errorf("market not found: %s", market)
}

func (m *MockMarketConfig) IsValidMarket(market string) bool {
	_, ok := m.markets[market]
	return ok
}

// MockTokenConfig 模拟代币配置
type MockTokenConfig struct {
	tokens map[string]tokenInfo
}

type tokenInfo struct {
	Symbol   string
	Name     string
	Decimals int32
	Address  string
}

func NewMockTokenConfig() *MockTokenConfig {
	return &MockTokenConfig{
		tokens: map[string]tokenInfo{
			"ETH":  {Symbol: "ETH", Name: "Ethereum", Decimals: 18, Address: "0x0000000000000000000000000000000000000000"},
			"BTC":  {Symbol: "BTC", Name: "Bitcoin", Decimals: 8, Address: "0x0000000000000000000000000000000000000001"},
			"USDT": {Symbol: "USDT", Name: "Tether USD", Decimals: 6, Address: "0x0000000000000000000000000000000000000002"},
		},
	}
}

func (m *MockTokenConfig) IsValidToken(token string) bool {
	_, ok := m.tokens[token]
	return ok
}

func (m *MockTokenConfig) GetTokenDecimals(token string) int32 {
	if t, ok := m.tokens[token]; ok {
		return t.Decimals
	}
	return 0
}

func (m *MockTokenConfig) GetSupportedTokens() []string {
	result := make([]string, 0, len(m.tokens))
	for k := range m.tokens {
		result = append(result, k)
	}
	return result
}

// MockRiskConfig 模拟风控配置
type MockRiskConfig struct {
	userPendingLimit     decimal.Decimal
	globalPendingLimit   decimal.Decimal
	maxOpenOrdersPerUser int
}

func NewMockRiskConfig() *MockRiskConfig {
	return &MockRiskConfig{
		userPendingLimit:     decimal.NewFromFloat(100000),
		globalPendingLimit:   decimal.NewFromFloat(10000000),
		maxOpenOrdersPerUser: 100,
	}
}

func (m *MockRiskConfig) GetUserPendingLimit() decimal.Decimal {
	return m.userPendingLimit
}

func (m *MockRiskConfig) GetGlobalPendingLimit() decimal.Decimal {
	return m.globalPendingLimit
}

func (m *MockRiskConfig) GetMaxOpenOrdersPerUser() int {
	return m.maxOpenOrdersPerUser
}

// ========== Test Helpers ==========

// TestMain 检查是否运行集成测试
func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		fmt.Println("Skipping integration tests (set INTEGRATION_TEST=1 to run)")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
