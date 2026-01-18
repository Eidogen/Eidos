package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/repository"
)

// 默认 Outbox 分片数量
const defaultOutboxShards = 8

// calculateShardID 根据钱包地址计算 Outbox 分片 ID
// 使用 FNV-1a 哈希确保相同钱包地址的订单分配到同一分片
// 这保证了同一用户的订单处理有序性
func calculateShardID(wallet string, numShards int) int {
	if numShards <= 0 {
		numShards = defaultOutboxShards
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(wallet)))
	return int(h.Sum32() % uint32(numShards))
}

var (
	ErrInvalidOrder               = errors.New("invalid order")
	ErrOrderNotCancellable        = errors.New("order cannot be cancelled")
	ErrDuplicateOrder             = errors.New("duplicate order")
	ErrInvalidNonce               = errors.New("invalid nonce")
	ErrInsufficientBalance        = errors.New("insufficient balance")
	ErrPendingLimitExceeded       = errors.New("pending limit exceeded")
	ErrGlobalPendingLimitExceeded = errors.New("global pending limit exceeded")
	ErrMaxOpenOrdersExceeded      = errors.New("max open orders exceeded")
	ErrInvalidStateTransition     = errors.New("invalid order state transition")
)

// OrderService 订单服务接口
type OrderService interface {
	// CreateOrder 创建订单
	// 1. 验证订单参数
	// 2. 检查 Nonce 防重放
	// 3. 冻结余额
	// 4. 创建订单记录
	// 5. 发送到撮合引擎
	CreateOrder(ctx context.Context, req *CreateOrderRequest) (*model.Order, error)

	// CancelOrder 取消订单
	// 1. 验证订单状态
	// 2. 解冻余额
	// 3. 更新订单状态
	CancelOrder(ctx context.Context, wallet, orderID string) error

	// CancelOrderByNonce 通过 Nonce 取消订单
	CancelOrderByNonce(ctx context.Context, wallet string, nonce uint64, signature []byte) error

	// GetOrder 获取订单详情
	GetOrder(ctx context.Context, orderID string) (*model.Order, error)

	// GetOrderByClientID 通过客户端 ID 获取订单
	GetOrderByClientID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error)

	// ListOrders 获取订单列表
	ListOrders(ctx context.Context, wallet string, filter *repository.OrderFilter, page *repository.Pagination) ([]*model.Order, error)

	// ListOpenOrders 获取活跃订单
	ListOpenOrders(ctx context.Context, wallet, market string) ([]*model.Order, error)

	// UpdateOrderFilled 更新订单成交信息 (由撮合引擎调用)
	UpdateOrderFilled(ctx context.Context, orderID string, filledAmount, filledQuote decimal.Decimal, newStatus model.OrderStatus) error

	// ExpireOrder 订单过期
	ExpireOrder(ctx context.Context, orderID string) error

	// RejectOrder 拒绝订单 (风控拦截)
	RejectOrder(ctx context.Context, orderID string, reason string) error

	// HandleCancelConfirm 处理撮合引擎的取消确认
	HandleCancelConfirm(ctx context.Context, msg *OrderCancelledMessage) error

	// HandleOrderAccepted 处理订单被撮合引擎接受
	HandleOrderAccepted(ctx context.Context, msg *OrderAcceptedMessage) error
}

// OrderCancelledMessage 订单取消消息
type OrderCancelledMessage struct {
	OrderID       string
	Market        string
	Result        string // success, not_found, already_cancelled
	RemainingSize string
	FilledSize    string
	Timestamp     int64
	Sequence      int64
}

// OrderAcceptedMessage 订单接受消息
type OrderAcceptedMessage struct {
	OrderID   string
	Market    string
	Timestamp int64
	Sequence  int64
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	Wallet        string          // 钱包地址
	Market        string          // 交易对
	Side          model.OrderSide // 买卖方向
	Type          model.OrderType // 订单类型
	Price         decimal.Decimal // 价格 (限价单必填)
	Amount        decimal.Decimal // 数量
	Nonce         uint64          // 用户 Nonce
	Signature     []byte          // EIP-712 签名
	ClientOrderID string          // 客户端订单 ID (可选)
	ExpireAt      int64           // 过期时间 (可选)
}

// OrderPublisher 订单状态发布接口
type OrderPublisher interface {
	PublishOrderUpdate(ctx context.Context, order *model.Order) error
}

// orderService 订单服务实现
type orderService struct {
	orderRepo    repository.OrderRepository
	balanceRepo  repository.BalanceRepository
	nonceRepo    repository.NonceRepository
	balanceCache cache.BalanceRedisRepository // Redis 作为实时资金真相
	idGenerator  IDGenerator
	marketConfig MarketConfigProvider
	riskConfig   RiskConfigProvider // 风控配置
	producer     KafkaProducer      // Kafka producer 发送取消请求到撮合引擎
	publisher    OrderPublisher     // 订单状态更新发布者
	asyncTasks   *AsyncTaskManager  // 异步任务管理器
}

// KafkaProducer Kafka 生产者接口
type KafkaProducer interface {
	SendWithContext(ctx context.Context, topic string, key, value []byte) error
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	Generate() (int64, error)
}

// MarketConfigProvider 市场配置提供者接口
type MarketConfigProvider interface {
	GetMarket(market string) (*MarketConfig, error)
	IsValidMarket(market string) bool
}

// RiskConfigProvider 风控配置提供者接口
type RiskConfigProvider interface {
	// GetUserPendingLimit 获取用户待结算限额 (USDT)
	GetUserPendingLimit() decimal.Decimal
	// GetGlobalPendingLimit 获取全局待结算限额 (USDT)
	GetGlobalPendingLimit() decimal.Decimal
	// GetMaxOpenOrdersPerUser 获取用户最大活跃订单数
	GetMaxOpenOrdersPerUser() int
}

// MarketConfig 市场配置
type MarketConfig struct {
	Market          string          // 交易对 (例如 ETH-USDT)
	BaseToken       string          // 基础代币 (例如 ETH)
	QuoteToken      string          // 计价代币 (例如 USDT)
	MinAmount       decimal.Decimal // 最小下单数量
	MaxAmount       decimal.Decimal // 最大下单数量
	MinPrice        decimal.Decimal // 最小价格
	MaxPrice        decimal.Decimal // 最大价格
	PricePrecision  int32           // 价格精度
	AmountPrecision int32           // 数量精度
	MakerFeeRate    decimal.Decimal // Maker 手续费率
	TakerFeeRate    decimal.Decimal // Taker 手续费率
	Status          int8            // 交易对状态 (1=正常, 0=暂停)
}

// NewOrderService 创建订单服务
func NewOrderService(
	orderRepo repository.OrderRepository,
	balanceRepo repository.BalanceRepository,
	nonceRepo repository.NonceRepository,
	balanceCache cache.BalanceRedisRepository,
	idGenerator IDGenerator,
	marketConfig MarketConfigProvider,
	riskConfig RiskConfigProvider,
	producer KafkaProducer,
	publisher OrderPublisher,
) OrderService {
	return &orderService{
		orderRepo:    orderRepo,
		balanceRepo:  balanceRepo,
		nonceRepo:    nonceRepo,
		balanceCache: balanceCache,
		idGenerator:  idGenerator,
		marketConfig: marketConfig,
		riskConfig:   riskConfig,
		producer:     producer,
		publisher:    publisher,
		asyncTasks:   GetAsyncTaskManager(),
	}
}

// CreateOrder 创建订单
// 使用 Redis Lua 原子操作: 检查 Nonce + 冻结余额 + 写入 Outbox
// 这确保了实时资金的一致性，DB 写入异步完成
func (s *orderService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*model.Order, error) {
	// 1. 验证参数
	if err := s.validateCreateOrderRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidOrder, err.Error())
	}

	// 2. 获取市场配置
	marketCfg, err := s.marketConfig.GetMarket(req.Market)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid market %s", ErrInvalidOrder, req.Market)
	}

	// 3. 检查客户端订单 ID 幂等性 (从 DB 检查)
	if req.ClientOrderID != "" {
		existingOrder, err := s.orderRepo.GetByClientOrderID(ctx, req.Wallet, req.ClientOrderID)
		if err == nil {
			return existingOrder, nil // 返回已存在的订单
		}
		if !errors.Is(err, repository.ErrOrderNotFound) {
			return nil, fmt.Errorf("check client order id failed: %w", err)
		}
	}

	// 4. 生成订单 ID
	orderIDInt, err := s.idGenerator.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate order id failed: %w", err)
	}
	orderID := fmt.Sprintf("O%d", orderIDInt)

	// 5. 计算需要冻结的金额
	freezeToken, freezeAmount := s.calculateFreezeAmount(req, marketCfg)

	// 6. 构建订单对象
	now := time.Now().UnixMilli()
	order := &model.Order{
		OrderID:       orderID,
		Wallet:        req.Wallet,
		Market:        req.Market,
		Side:          req.Side,
		Type:          req.Type,
		Price:         req.Price,
		Amount:        req.Amount,
		FilledAmount:  decimal.Zero,
		FilledQuote:   decimal.Zero,
		Status:        model.OrderStatusPending,
		Nonce:         req.Nonce,
		Signature:     req.Signature,
		ClientOrderID: req.ClientOrderID,
		ExpireAt:      req.ExpireAt,
		FreezeToken:   freezeToken,
		FreezeAmount:  freezeAmount,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// 7. 序列化订单用于 Outbox
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order failed: %w", err)
	}

	// 8. Redis Lua 原子操作: Nonce 检查 + 余额冻结 + Outbox 写入
	// 这是关键的原子操作，确保实时资金一致性
	nonceKey := fmt.Sprintf("nonce:%s:order:%d", req.Wallet, req.Nonce)

	// 获取风控限额配置
	var userPendingLimit, globalPendingLimit decimal.Decimal
	var maxOpenOrders int
	if s.riskConfig != nil {
		userPendingLimit = s.riskConfig.GetUserPendingLimit()
		globalPendingLimit = s.riskConfig.GetGlobalPendingLimit()
		maxOpenOrders = s.riskConfig.GetMaxOpenOrdersPerUser()
	}

	freezeReq := &cache.FreezeForOrderRequest{
		Wallet:             req.Wallet,
		Token:              freezeToken,
		Amount:             freezeAmount,
		OrderID:            orderID,
		FromSettled:        true, // 优先从已结算冻结
		OrderJSON:          string(orderJSON),
		ShardID:            calculateShardID(req.Wallet, defaultOutboxShards), // 根据钱包地址分片
		NonceKey:           nonceKey,
		NonceTTL:           7 * 24 * time.Hour, // Nonce 7 天过期
		UserPendingLimit:   userPendingLimit,   // 用户待结算限额检查
		GlobalPendingLimit: globalPendingLimit, // 全局待结算限额检查
		MaxOpenOrders:      maxOpenOrders,      // 用户最大活跃订单数检查
	}

	if err := s.balanceCache.FreezeForOrder(ctx, freezeReq); err != nil {
		if errors.Is(err, cache.ErrRedisNonceUsed) {
			return nil, ErrInvalidNonce
		}
		if errors.Is(err, cache.ErrRedisInsufficientBalance) {
			metrics.RecordRiskRejection("insufficient_balance")
			return nil, ErrInsufficientBalance
		}
		if errors.Is(err, cache.ErrRedisPendingLimitExceeded) {
			metrics.RecordRiskRejection("pending_limit")
			return nil, ErrPendingLimitExceeded
		}
		if errors.Is(err, cache.ErrRedisGlobalPendingLimitExceeded) {
			metrics.RecordRiskRejection("global_limit")
			return nil, ErrGlobalPendingLimitExceeded
		}
		if errors.Is(err, cache.ErrRedisMaxOpenOrdersExceeded) {
			metrics.RecordRiskRejection("max_orders")
			return nil, ErrMaxOpenOrdersExceeded
		}
		return nil, fmt.Errorf("redis freeze for order failed: %w", err)
	}

	// 9. 记录订单创建成功指标
	sideStr := "buy"
	if req.Side == model.OrderSideSell {
		sideStr = "sell"
	}
	metrics.RecordOrderCreated(req.Market, sideStr)
	metrics.IncTotalOpenOrders()

	// 10. 同步写入 DB (保证数据持久化，避免 Redis 写入成功但 DB 丢单)
	if err := s.persistOrderToDB(ctx, order, req); err != nil {
		// 严重错误：Redis 冻结成功但 DB 写入失败
		// 此时需要回滚 Redis (解锁资金，清除 Outbox)，并返回错误给用户
		// 使用 CancelPendingOrder 逻辑尝试回滚
		shardID := calculateShardID(req.Wallet, defaultOutboxShards)
		rollbackReq := &cache.CancelPendingOrderRequest{
			Wallet:  req.Wallet,
			Token:   freezeToken,
			OrderID: orderID,
			ShardID: shardID,
		}
		_ = s.balanceCache.CancelPendingOrder(ctx, rollbackReq)
		_ = s.balanceCache.DecrUserOpenOrders(ctx, req.Wallet)
		metrics.DecTotalOpenOrders()
		metrics.RecordDataIntegrityCritical("order", "db_persist_failed_rollback")
		return nil, fmt.Errorf("persist order to db failed: %w", err)
	}

	// 11. 发布订单创建消息到 Kafka (供 WebSocket 推送)
	s.publishOrderUpdate(ctx, order)

	return order, nil
}

// persistOrderToDB 持久化订单到数据库
func (s *orderService) persistOrderToDB(ctx context.Context, order *model.Order, req *CreateOrderRequest) error {
	return s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 1. 记录 Nonce
		if err := s.nonceRepo.MarkUsedWithTx(txCtx, req.Wallet, model.NonceUsageOrder, req.Nonce, order.OrderID); err != nil {
			if errors.Is(err, repository.ErrNonceAlreadyUsed) {
				return nil // 幂等，已处理
			}
			return fmt.Errorf("mark nonce used failed: %w", err)
		}

		// 2. 创建订单
		if err := s.orderRepo.Create(txCtx, order); err != nil {
			if errors.Is(err, repository.ErrOrderAlreadyExists) {
				return nil // 幂等，已存在
			}
			return fmt.Errorf("create order failed: %w", err)
		}

		// 3. 创建余额流水
		balanceLog := &model.BalanceLog{
			Wallet:        order.Wallet,
			Token:         order.FreezeToken,
			Type:          model.BalanceLogTypeFreeze,
			Amount:        order.FreezeAmount.Neg(),
			BalanceBefore: decimal.Zero,
			BalanceAfter:  decimal.Zero,
			OrderID:       order.OrderID,
			Remark:        fmt.Sprintf("Order freeze: %s", order.OrderID),
		}
		if err := s.balanceRepo.CreateBalanceLog(txCtx, balanceLog); err != nil {
			return fmt.Errorf("create balance log failed: %w", err)
		}

		return nil
	})
}

// CancelOrder 取消订单
// 根据订单状态采用不同策略:
// - PENDING: 订单尚未发送到撮合引擎，直接在 Redis 中原子取消 (解冻资金 + 移除 Outbox)
// - OPEN/PARTIAL: 订单已在撮合引擎中，发送取消请求 → 等待 HandleCancelConfirm → 解冻资金
func (s *orderService) CancelOrder(ctx context.Context, wallet, orderID string) error {
	// 1. 获取订单
	order, err := s.orderRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	// 2. 验证钱包地址
	if order.Wallet != wallet {
		return repository.ErrOrderNotFound
	}

	// 3. 检查订单状态是否可取消
	if !order.CanCancel() {
		return ErrOrderNotCancellable
	}

	// 4. 根据订单状态选择取消策略
	shardID := calculateShardID(wallet, defaultOutboxShards)

	if order.Status == model.OrderStatusPending {
		// PENDING 订单: 直接在 Redis 中原子取消
		return s.cancelPendingOrder(ctx, order, shardID)
	}

	// OPEN/PARTIAL 订单: 发送取消请求到撮合引擎
	return s.cancelActiveOrder(ctx, order, shardID)
}

// cancelPendingOrder 取消 PENDING 状态订单
// PENDING 订单尚未发送到撮合引擎，可以直接在 Redis 中原子取消
func (s *orderService) cancelPendingOrder(ctx context.Context, order *model.Order, shardID int) error {
	// 1. Redis 原子取消: 检查 Outbox 状态 + 解冻余额 + 删除 Outbox
	cancelReq := &cache.CancelPendingOrderRequest{
		Wallet:  order.Wallet,
		Token:   order.FreezeToken,
		OrderID: order.OrderID,
		ShardID: shardID,
	}

	err := s.balanceCache.CancelPendingOrder(ctx, cancelReq)
	if err != nil {
		if errors.Is(err, cache.ErrRedisOrderAlreadySent) {
			// 订单已发送到撮合引擎，转为取消 OPEN 订单流程
			return s.cancelActiveOrder(ctx, order, shardID)
		}
		if errors.Is(err, cache.ErrRedisOrderFreezeNotFound) {
			// Outbox 不存在，可能已经被处理
			// 尝试作为 OPEN 订单取消 (保险起见)
			return s.cancelActiveOrder(ctx, order, shardID)
		}
		return fmt.Errorf("cancel pending order failed: %w", err)
	}

	// 2. 同步更新 DB
	if err := s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新订单状态
		if err := s.orderRepo.UpdateStatus(txCtx, order.OrderID, order.Status, model.OrderStatusCancelled); err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		// 创建余额流水
		if order.FreezeAmount.GreaterThan(decimal.Zero) {
			balanceLog := &model.BalanceLog{
				Wallet:        order.Wallet,
				Token:         order.FreezeToken,
				Type:          model.BalanceLogTypeUnfreeze,
				Amount:        order.FreezeAmount,
				BalanceBefore: decimal.Zero,
				BalanceAfter:  decimal.Zero,
				OrderID:       order.OrderID,
				Remark:        fmt.Sprintf("Pending order cancelled: %s", order.OrderID),
			}
			if err := s.balanceRepo.CreateBalanceLog(txCtx, balanceLog); err != nil {
				return fmt.Errorf("create balance log: %w", err)
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// 3. 发布订单取消消息
	order.Status = model.OrderStatusCancelled
	order.UpdatedAt = time.Now().UnixMilli()
	s.publishOrderUpdate(ctx, order)

	return nil
}

// cancelActiveOrder 取消 OPEN/PARTIAL 状态订单
// 发送取消请求到撮合引擎，资金将在 HandleCancelConfirm 时解冻
func (s *orderService) cancelActiveOrder(ctx context.Context, order *model.Order, shardID int) error {
	// 1. 构建取消请求 JSON
	cancelReq := &CancelOrderRequest{
		OrderID:   order.OrderID,
		Market:    order.Market,
		Wallet:    order.Wallet,
		Timestamp: time.Now().UnixMilli(),
	}
	cancelJSON, err := json.Marshal(cancelReq)
	if err != nil {
		return fmt.Errorf("marshal cancel request failed: %w", err)
	}

	// 2. 写入 Cancel Outbox (只发送取消请求，不解冻资金)
	writeReq := &cache.WriteCancelOutboxRequest{
		OrderID:    order.OrderID,
		Market:     order.Market,
		CancelJSON: string(cancelJSON),
		ShardID:    shardID,
	}

	if err := s.balanceCache.WriteCancelOutbox(ctx, writeReq); err != nil {
		return fmt.Errorf("write cancel outbox failed: %w", err)
	}

	// 3. 更新订单状态为 CANCELLING (同步)
	return s.orderRepo.UpdateStatus(ctx, order.OrderID, order.Status, model.OrderStatusCancelling)

}

// CancelOrderRequest 取消订单请求 (发送到 Kafka)
type CancelOrderRequest struct {
	OrderID   string `json:"order_id"`
	Market    string `json:"market"`
	Wallet    string `json:"wallet"`
	Timestamp int64  `json:"timestamp"`
}

// CancelOrderByNonce 通过 Nonce 取消订单
func (s *orderService) CancelOrderByNonce(ctx context.Context, wallet string, nonce uint64, signature []byte) error {
	// 1. 查找订单
	order, err := s.orderRepo.GetByWalletNonce(ctx, wallet, nonce)
	if err != nil {
		return err
	}

	// 2. 验证签名 (TODO: 实现 EIP-712 签名验证)

	// 3. 取消订单
	return s.CancelOrder(ctx, wallet, order.OrderID)
}

// GetOrder 获取订单详情
func (s *orderService) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	return s.orderRepo.GetByOrderID(ctx, orderID)
}

// GetOrderByClientID 通过客户端 ID 获取订单
func (s *orderService) GetOrderByClientID(ctx context.Context, wallet, clientOrderID string) (*model.Order, error) {
	return s.orderRepo.GetByClientOrderID(ctx, wallet, clientOrderID)
}

// ListOrders 获取订单列表
func (s *orderService) ListOrders(ctx context.Context, wallet string, filter *repository.OrderFilter, page *repository.Pagination) ([]*model.Order, error) {
	return s.orderRepo.ListByWallet(ctx, wallet, filter, page)
}

// ListOpenOrders 获取活跃订单
func (s *orderService) ListOpenOrders(ctx context.Context, wallet, market string) ([]*model.Order, error) {
	return s.orderRepo.ListOpenOrders(ctx, wallet, market)
}

// UpdateOrderFilled 更新订单成交信息
func (s *orderService) UpdateOrderFilled(ctx context.Context, orderID string, filledAmount, filledQuote decimal.Decimal, newStatus model.OrderStatus) error {
	return s.orderRepo.UpdateFilled(ctx, orderID, filledAmount.String(), filledQuote.String(), newStatus)
}

// ExpireOrder 订单过期
// 使用 Redis 解冻资金，保证实时资金一致性
func (s *orderService) ExpireOrder(ctx context.Context, orderID string) error {
	order, err := s.orderRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	if !order.CanCancel() {
		return nil // 已经是终态，不需要处理
	}

	// 使用 Redis 解冻资金
	if err := s.balanceCache.UnfreezeByOrderID(ctx, order.Wallet, order.FreezeToken, orderID); err != nil {
		// logger.Warn("expire order unfreeze failed", zap.Error(err), zap.String("order_id", orderID))
	}

	// 减少用户活跃订单计数
	_ = s.balanceCache.DecrUserOpenOrders(ctx, order.Wallet)
	metrics.DecTotalOpenOrders()

	// 记录订单过期指标
	sideStr := "buy"
	if order.Side == model.OrderSideSell {
		sideStr = "sell"
	}
	metrics.RecordOrderExpired(order.Market, sideStr)

	// 同步更新 DB
	unfreezeAmount := order.RemainingFreezeAmount()
	if err := s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		if err := s.orderRepo.UpdateStatus(txCtx, orderID, order.Status, model.OrderStatusExpired); err != nil {
			return err
		}

		if unfreezeAmount.GreaterThan(decimal.Zero) {
			balanceLog := &model.BalanceLog{
				Wallet:        order.Wallet,
				Token:         order.FreezeToken,
				Type:          model.BalanceLogTypeUnfreeze,
				Amount:        unfreezeAmount,
				BalanceBefore: decimal.Zero,
				BalanceAfter:  decimal.Zero,
				OrderID:       orderID,
				Remark:        fmt.Sprintf("Order expired: %s", orderID),
			}
			if err := s.balanceRepo.CreateBalanceLog(txCtx, balanceLog); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// 发布订单过期消息
	order.Status = model.OrderStatusExpired
	order.UpdatedAt = time.Now().UnixMilli()
	s.publishOrderUpdate(ctx, order)

	return nil
}

// RejectOrder 拒绝订单 (风控拦截)
// 使用 Redis 解冻资金，保证实时资金一致性
func (s *orderService) RejectOrder(ctx context.Context, orderID string, reason string) error {
	order, err := s.orderRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	if order.Status != model.OrderStatusPending {
		return nil
	}

	// 使用 Redis 解冻资金
	if err := s.balanceCache.UnfreezeByOrderID(ctx, order.Wallet, order.FreezeToken, orderID); err != nil {
		// logger.Warn("reject order unfreeze failed", zap.Error(err), zap.String("order_id", orderID))
	}

	// 减少用户活跃订单计数
	_ = s.balanceCache.DecrUserOpenOrders(ctx, order.Wallet)
	metrics.DecTotalOpenOrders()

	// 同步更新 DB
	unfreezeAmount := order.FreezeAmount
	order.Status = model.OrderStatusRejected
	order.RejectReason = reason
	order.UpdatedAt = time.Now().UnixMilli()

	if err := s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
		// 更新订单状态和拒绝原因
		if err := s.orderRepo.Update(txCtx, order); err != nil {
			return err
		}

		// 创建余额流水
		if unfreezeAmount.GreaterThan(decimal.Zero) {
			balanceLog := &model.BalanceLog{
				Wallet:        order.Wallet,
				Token:         order.FreezeToken,
				Type:          model.BalanceLogTypeUnfreeze,
				Amount:        unfreezeAmount,
				BalanceBefore: decimal.Zero,
				BalanceAfter:  decimal.Zero,
				OrderID:       orderID,
				Remark:        fmt.Sprintf("Order rejected: %s, reason: %s", orderID, reason),
			}
			if err := s.balanceRepo.CreateBalanceLog(txCtx, balanceLog); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// 发布订单拒绝消息
	s.publishOrderUpdate(ctx, order)

	return nil
}

// validateCreateOrderRequest 验证创建订单请求
func (s *orderService) validateCreateOrderRequest(req *CreateOrderRequest) error {
	if req.Wallet == "" || len(req.Wallet) != 42 {
		return errors.New("invalid wallet address")
	}
	if !strings.HasPrefix(req.Wallet, "0x") {
		return errors.New("wallet address must start with 0x")
	}
	if req.Market == "" {
		return errors.New("market is required")
	}
	if req.Side != model.OrderSideBuy && req.Side != model.OrderSideSell {
		return errors.New("invalid order side")
	}
	if req.Type != model.OrderTypeLimit && req.Type != model.OrderTypeMarket {
		return errors.New("invalid order type")
	}
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return errors.New("amount must be positive")
	}
	if req.Type == model.OrderTypeLimit && req.Price.LessThanOrEqual(decimal.Zero) {
		return errors.New("price must be positive for limit order")
	}
	if len(req.Signature) == 0 {
		return errors.New("signature is required")
	}
	return nil
}

// calculateFreezeAmount 计算需要冻结的金额
func (s *orderService) calculateFreezeAmount(req *CreateOrderRequest, cfg *MarketConfig) (string, decimal.Decimal) {
	if req.Side == model.OrderSideBuy {
		// 买单冻结计价代币 (Quote Token)
		// 冻结金额 = 价格 * 数量 * (1 + 手续费率)
		quoteAmount := req.Price.Mul(req.Amount)
		feeAmount := quoteAmount.Mul(cfg.TakerFeeRate)
		return cfg.QuoteToken, quoteAmount.Add(feeAmount)
	} else {
		// 卖单冻结基础代币 (Base Token)
		return cfg.BaseToken, req.Amount
	}
}

// HandleCancelConfirm 处理撮合引擎的取消确认
// 这是取消订单的关键步骤: 收到撮合引擎确认后才解冻资金
func (s *orderService) HandleCancelConfirm(ctx context.Context, msg *OrderCancelledMessage) error {
	order, err := s.orderRepo.GetByOrderID(ctx, msg.OrderID)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}

	switch msg.Result {
	case "success":
		// 成功取消 - 使用 Redis 解冻资金 (保证实时资金一致性)
		if err := s.balanceCache.UnfreezeByOrderID(ctx, order.Wallet, order.FreezeToken, msg.OrderID); err != nil {
			// 解冻失败，但不应该阻止状态更新
			// 这种情况可能是订单冻结记录已不存在 (已被部分成交释放)
			// logger.Warn("unfreeze by order id failed", zap.Error(err), zap.String("order_id", msg.OrderID))
		}

		// 减少用户活跃订单计数
		_ = s.balanceCache.DecrUserOpenOrders(ctx, order.Wallet)
		metrics.DecTotalOpenOrders()

		// 记录订单取消指标
		sideStr := "buy"
		if order.Side == model.OrderSideSell {
			sideStr = "sell"
		}
		metrics.RecordOrderCancelled(order.Market, sideStr)

		// 同步更新 DB
		remainingSizeStr := msg.RemainingSize
		remainingSize, err := decimal.NewFromString(remainingSizeStr)
		if err != nil {
			return fmt.Errorf("parse remaining size: %w", err)
		}

		if err := s.balanceRepo.Transaction(ctx, func(txCtx context.Context) error {
			// 更新订单状态
			if err := s.orderRepo.UpdateStatus(txCtx, msg.OrderID, order.Status, model.OrderStatusCancelled); err != nil {
				return fmt.Errorf("update order status: %w", err)
			}

			// 创建余额流水
			unfreezeAmount := s.calculateUnfreezeAmount(order, remainingSize)
			if unfreezeAmount.GreaterThan(decimal.Zero) {
				balanceLog := &model.BalanceLog{
					Wallet:        order.Wallet,
					Token:         order.FreezeToken,
					Type:          model.BalanceLogTypeUnfreeze,
					Amount:        unfreezeAmount,
					BalanceBefore: decimal.Zero,
					BalanceAfter:  decimal.Zero,
					OrderID:       msg.OrderID,
					Remark:        fmt.Sprintf("Order cancel confirmed: %s", msg.OrderID),
				}
				if err := s.balanceRepo.CreateBalanceLog(txCtx, balanceLog); err != nil {
					return fmt.Errorf("create balance log: %w", err)
				}
			}
			return nil
		}); err != nil {
			metrics.RecordDataIntegrityCritical("order", "cancel_db_update_failed")
			return fmt.Errorf("cancel confirm db update failed: %w", err)
		}

		// 发布订单取消消息
		order.Status = model.OrderStatusCancelled
		order.UpdatedAt = time.Now().UnixMilli()
		s.publishOrderUpdate(ctx, order)

		return nil

	case "not_found":
		// 订单不在撮合引擎，可能已全部成交
		newStatus := model.OrderStatusCancelled
		if order.FilledAmount.Equal(order.Amount) {
			newStatus = model.OrderStatusFilled
		} else {
			// 异常情况，需要解冻剩余资金
			if err := s.balanceCache.UnfreezeByOrderID(ctx, order.Wallet, order.FreezeToken, msg.OrderID); err != nil {
				// logger.Warn("unfreeze by order id failed (not_found)", zap.Error(err), zap.String("order_id", msg.OrderID))
			}
		}

		// 减少用户活跃订单计数
		_ = s.balanceCache.DecrUserOpenOrders(ctx, order.Wallet)
		metrics.DecTotalOpenOrders()

		// 同步更新 DB
		finalStatus := newStatus
		if err := s.orderRepo.UpdateStatus(ctx, msg.OrderID, order.Status, finalStatus); err != nil {
			metrics.RecordDataIntegrityCritical("order", "cancel_not_found_db_failed")
			return fmt.Errorf("update order status failed: %w", err)
		}

		// 发布订单状态变更消息
		order.Status = finalStatus
		order.UpdatedAt = time.Now().UnixMilli()
		s.publishOrderUpdate(ctx, order)

		return nil

	case "already_cancelled":
		// 幂等处理，忽略
		return nil

	default:
		return fmt.Errorf("unknown cancel result: %s", msg.Result)
	}
}

// calculateUnfreezeAmount 计算需要解冻的金额
func (s *orderService) calculateUnfreezeAmount(order *model.Order, remainingSize decimal.Decimal) decimal.Decimal {
	if order.Side == model.OrderSideBuy {
		// 买单：解冻剩余的 quote 金额
		return remainingSize.Mul(order.Price)
	}
	// 卖单：解冻剩余的 base 数量
	return remainingSize
}

// HandleOrderAccepted 处理订单被撮合引擎接受
func (s *orderService) HandleOrderAccepted(ctx context.Context, msg *OrderAcceptedMessage) error {
	order, err := s.orderRepo.GetByOrderID(ctx, msg.OrderID)
	if err != nil {
		return fmt.Errorf("get order: %w", err)
	}

	// 只更新 PENDING 状态的订单
	if order.Status != model.OrderStatusPending {
		return nil
	}

	order.Status = model.OrderStatusOpen
	order.AcceptedAt = msg.Timestamp
	order.UpdatedAt = time.Now().UnixMilli()

	if err := s.orderRepo.Update(ctx, order); err != nil {
		return err
	}

	// 发布订单状态变更消息 (PENDING → OPEN)
	s.publishOrderUpdate(ctx, order)

	return nil
}

// validateStateTransition 验证订单状态转换是否合法
// 返回 nil 表示转换合法，否则返回 ErrInvalidStateTransition
func (s *orderService) validateStateTransition(order *model.Order, newStatus model.OrderStatus) error {
	if !order.CanTransitionTo(newStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, order.Status.String(), newStatus.String())
	}
	return nil
}

// publishOrderUpdate 发布订单状态更新消息
// 异步发布，不阻塞主流程
func (s *orderService) publishOrderUpdate(ctx context.Context, order *model.Order) {
	if s.publisher == nil {
		return
	}
	// 异步发布，不阻塞主流程
	go func() {
		if err := s.publisher.PublishOrderUpdate(ctx, order); err != nil {
			// 发布失败只记录日志，不影响主流程
			// 客户端可以通过 API 查询最新状态
		}
	}()
}
