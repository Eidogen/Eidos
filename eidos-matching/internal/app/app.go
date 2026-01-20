// Package app 应用启动和生命周期管理
//
// 服务对接说明 (详见 docs/INTEGRATION.md):
//
// Kafka 订阅 (Input):
//   - orders: 新订单消息，来自 eidos-trading
//   - cancel-requests: 取消请求，来自 eidos-trading
//
// Kafka 发布 (Output):
//   - trade-results: 成交结果，发送给 eidos-trading (清算) 和 eidos-market (行情)
//   - order-cancelled: 取消结果，发送给 eidos-trading
//   - orderbook-updates: 订单簿增量更新，发送给 eidos-market 和 eidos-api
//
// gRPC 调用:
//   - eidos-risk: 交易前风控检查 CheckOrder (已完成)
//
// gRPC 提供:
//   - GetDepth: 供 eidos-api, eidos-market 查询订单簿深度
//   - GetOrderbook: 获取订单簿快照
//   - HealthCheck: 健康检查
//
// Redis:
//   - 快照存储，用于崩溃恢复
//
// 服务注册:
//   - Nacos: 服务注册发现 (已完成)
package app

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
	commonv1 "github.com/eidos-exchange/eidos/proto/common/v1"
	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/config"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/engine"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/snapshot"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// App 撮合引擎应用
type App struct {
	cfg *config.Config

	// 核心组件
	engineManager   *engine.EngineManager
	consumer        *kafka.Consumer
	producer        *kafka.Producer
	snapshotManager *snapshot.Manager
	redisClient     redis.UniversalClient

	// gRPC 服务
	grpcServer   *grpc.Server
	grpcListener net.Listener

	// Nacos 服务注册
	nacosClient   *nacos.Client
	nacosRegistry *nacos.Registry
	serviceInst   *nacos.ServiceInstance

	// eidos-risk 风控客户端
	riskConn   *grpc.ClientConn
	riskClient riskv1.RiskServiceClient

	// 状态
	isRunning atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 快照定时器
	snapshotTicker *time.Ticker
}

// NewApp 创建应用
func NewApp(cfg *config.Config) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// 初始化 Redis
	if err := app.initRedis(); err != nil {
		cancel()
		return nil, fmt.Errorf("init redis: %w", err)
	}

	// 初始化引擎管理器
	if err := app.initEngineManager(); err != nil {
		cancel()
		return nil, fmt.Errorf("init engine manager: %w", err)
	}

	// 初始化快照管理器
	app.initSnapshotManager()

	// 初始化 Kafka
	if err := app.initKafka(); err != nil {
		cancel()
		return nil, fmt.Errorf("init kafka: %w", err)
	}

	// 初始化 gRPC 服务
	if err := app.initGRPC(); err != nil {
		cancel()
		return nil, fmt.Errorf("init grpc: %w", err)
	}

	// 初始化 Nacos 服务注册 (可选，配置为空则跳过)
	if cfg.Nacos.ServerAddr != "" {
		if err := app.initNacos(); err != nil {
			zap.L().Warn("init nacos failed, service discovery disabled", zap.Error(err))
		}
	}

	// 初始化 eidos-risk 风控客户端 (可选，配置为空则跳过)
	if cfg.Risk.Enabled && cfg.Risk.Addr != "" {
		if err := app.initRisk(); err != nil {
			zap.L().Warn("init risk client failed, risk checking disabled", zap.Error(err))
		}
	}

	return app, nil
}

// initRedis 初始化 Redis
func (a *App) initRedis() error {
	opts := &redis.UniversalOptions{
		Addrs:    a.cfg.Redis.Addresses,
		Password: a.cfg.Redis.Password,
		DB:       a.cfg.Redis.DB,
		PoolSize: a.cfg.Redis.PoolSize,
	}

	a.redisClient = redis.NewUniversalClient(opts)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}

	return nil
}

// initEngineManager 初始化引擎管理器
func (a *App) initEngineManager() error {
	// 转换市场配置
	marketConfigs := make([]*engine.MarketConfig, 0, len(a.cfg.Markets))
	for _, m := range a.cfg.Markets {
		engineCfg := m.ToEngineMarketConfig()
		marketConfigs = append(marketConfigs, &engine.MarketConfig{
			Symbol:        engineCfg.Symbol,
			BaseToken:     engineCfg.BaseToken,
			QuoteToken:    engineCfg.QuoteToken,
			PriceDecimals: engineCfg.PriceDecimals,
			SizeDecimals:  engineCfg.SizeDecimals,
			MinSize:       engineCfg.MinSize,
			TickSize:      engineCfg.TickSize,
			MakerFeeRate:  engineCfg.MakerFeeRate,
			TakerFeeRate:  engineCfg.TakerFeeRate,
			MaxSlippage:   engineCfg.MaxSlippage,
		})
	}

	a.engineManager = engine.NewEngineManager(&engine.ManagerConfig{
		Markets:     marketConfigs,
		TradeIDGen:  generateTradeID,
		ChannelSize: 10000,
	})

	return nil
}

// initSnapshotManager 初始化快照管理器
func (a *App) initSnapshotManager() {
	a.snapshotManager = snapshot.NewManager(a.redisClient, &snapshot.SnapshotConfig{
		Interval:  a.cfg.Snapshot.Interval,
		MaxCount:  a.cfg.Snapshot.MaxCount,
		TwoPhase:  a.cfg.Snapshot.TwoPhase,
		KeyPrefix: "snapshot",
	})
}

// initKafka 初始化 Kafka
func (a *App) initKafka() error {
	// 创建消费者
	a.consumer = kafka.NewConsumer(&kafka.ConsumerConfig{
		Brokers:      a.cfg.Kafka.Brokers,
		GroupID:      a.cfg.Kafka.Consumer.GroupID,
		OrdersTopic:  "orders",
		CancelsTopic: "cancel-requests",
		BatchSize:    a.cfg.Kafka.Consumer.BatchSize,
		LingerMs:     a.cfg.Kafka.Consumer.LingerMs,
		StartOffset:  -1, // latest
		CommitMode:   "manual",
	})

	// 设置处理函数
	a.consumer.SetOrderHandler(a.handleOrder)
	a.consumer.SetCancelHandler(a.handleCancel)

	// 创建生产者
	a.producer = kafka.NewProducer(&kafka.ProducerConfig{
		Brokers:               a.cfg.Kafka.Brokers,
		TradeResultsTopic:     "trade-results",
		OrderCancelledTopic:   "order-cancelled",
		OrderbookUpdatesTopic: "orderbook-updates",
		OrderUpdatesTopic:     "order-updates", // 订单状态更新 (用于风控拒绝等)
		BatchSize:             a.cfg.Kafka.Producer.BatchSize,
		BatchTimeout:          time.Duration(a.cfg.Kafka.Producer.BatchTimeout) * time.Millisecond,
		Compression:           a.cfg.Kafka.Producer.Compression,
		RequiredAcks:          a.cfg.Kafka.Producer.RequiredAcks,
	})

	return nil
}

// initGRPC 初始化 gRPC 服务
func (a *App) initGRPC() error {
	// 创建 gRPC 服务器
	a.grpcServer = grpc.NewServer()

	// 注册 MatchingService
	matchingHandler := handler.NewMatchingHandler(a.engineManager)
	matchingv1.RegisterMatchingServiceServer(a.grpcServer, matchingHandler)

	// 创建监听器
	addr := fmt.Sprintf(":%d", a.cfg.Service.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	a.grpcListener = lis

	return nil
}

// initNacos 初始化 Nacos 服务注册
func (a *App) initNacos() error {
	// 创建 Nacos 客户端
	nacosCfg := &nacos.Config{
		ServerAddr: a.cfg.Nacos.ServerAddr,
		Namespace:  a.cfg.Nacos.Namespace,
		Group:      a.cfg.Nacos.Group,
		LogDir:     "/tmp/nacos/log",
		CacheDir:   "/tmp/nacos/cache",
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}

	client, err := nacos.NewClient(nacosCfg)
	if err != nil {
		return fmt.Errorf("create nacos client: %w", err)
	}
	a.nacosClient = client

	// 创建服务注册器
	a.nacosRegistry = nacos.NewRegistry(client)

	// 准备服务实例信息
	markets := a.engineManager.GetMarkets()
	a.serviceInst = nacos.DefaultServiceInstance(
		a.cfg.Service.Name,
		uint64(a.cfg.Service.GRPCPort),
	)
	a.serviceInst.Metadata = map[string]string{
		"markets":  strings.Join(markets, ","),
		"node_id":  a.cfg.Service.NodeID,
		"env":      a.cfg.Service.Env,
		"protocol": "grpc",
	}

	zap.L().Info("nacos client initialized",
		zap.String("server", a.cfg.Nacos.ServerAddr),
		zap.String("namespace", a.cfg.Nacos.Namespace))

	return nil
}

// initRisk 初始化 eidos-risk 风控客户端
func (a *App) initRisk() error {
	// 创建 gRPC 连接
	conn, err := grpc.NewClient(
		a.cfg.Risk.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to risk service: %w", err)
	}

	a.riskConn = conn
	a.riskClient = riskv1.NewRiskServiceClient(conn)

	zap.L().Info("risk client initialized",
		zap.String("addr", a.cfg.Risk.Addr),
		zap.Int("timeout_ms", a.cfg.Risk.Timeout))

	return nil
}

// Start 启动应用
func (a *App) Start() error {
	if a.isRunning.Load() {
		return fmt.Errorf("app already running")
	}

	// 尝试从快照恢复
	if err := a.recoverFromSnapshot(); err != nil {
		// 恢复失败不阻止启动，记录日志即可
		zap.L().Warn("recover from snapshot failed", zap.Error(err))
	}

	// 启动引擎管理器
	if err := a.engineManager.Start(a.ctx); err != nil {
		return fmt.Errorf("start engine manager: %w", err)
	}

	// 启动 Kafka 消费者
	if err := a.consumer.Start(); err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}

	// 启动输出收集协程
	a.wg.Add(3)
	go a.collectTrades()
	go a.collectUpdates()
	go a.collectCancels()

	// 启动快照定时器
	a.snapshotTicker = time.NewTicker(a.cfg.Snapshot.Interval)
	a.wg.Add(1)
	go a.snapshotLoop()

	// 启动 gRPC 服务
	a.wg.Add(1)
	go a.serveGRPC()

	// 注册服务到 Nacos
	if a.nacosRegistry != nil && a.serviceInst != nil {
		if err := a.nacosRegistry.Register(a.serviceInst); err != nil {
			zap.L().Error("register service to nacos failed", zap.Error(err))
		} else {
			zap.L().Info("service registered to nacos",
				zap.String("service", a.serviceInst.ServiceName),
				zap.String("ip", a.serviceInst.IP),
				zap.Uint64("port", a.serviceInst.Port))
		}
	}

	a.isRunning.Store(true)
	zap.L().Info("eidos-matching started",
		zap.Int("grpc_port", a.cfg.Service.GRPCPort),
		zap.Strings("markets", a.engineManager.GetMarkets()))
	return nil
}

// Stop 停止应用
func (a *App) Stop() error {
	if !a.isRunning.Load() {
		return nil
	}

	a.isRunning.Store(false)
	a.cancel()

	// 从 Nacos 注销服务
	if a.nacosRegistry != nil && a.serviceInst != nil {
		if err := a.nacosRegistry.Deregister(a.serviceInst); err != nil {
			zap.L().Error("deregister service from nacos failed", zap.Error(err))
		} else {
			zap.L().Info("service deregistered from nacos",
				zap.String("service", a.serviceInst.ServiceName))
		}
	}

	// 关闭 Nacos 客户端
	if a.nacosClient != nil {
		a.nacosClient.Close()
	}

	// 关闭 Risk gRPC 连接
	if a.riskConn != nil {
		if err := a.riskConn.Close(); err != nil {
			zap.L().Error("close risk connection failed", zap.Error(err))
		}
	}

	// 停止 gRPC 服务
	if a.grpcServer != nil {
		a.grpcServer.GracefulStop()
	}

	// 停止快照定时器
	if a.snapshotTicker != nil {
		a.snapshotTicker.Stop()
	}

	// 停止消费者
	if err := a.consumer.Stop(); err != nil {
		zap.L().Error("stop consumer failed", zap.Error(err))
	}

	// 停止引擎
	a.engineManager.Stop()

	// 等待所有协程结束
	a.wg.Wait()

	// 关闭生产者
	if err := a.producer.Close(); err != nil {
		zap.L().Error("close producer failed", zap.Error(err))
	}

	// 关闭 Redis
	if err := a.redisClient.Close(); err != nil {
		zap.L().Error("close redis failed", zap.Error(err))
	}

	// 保存最终快照
	a.saveAllSnapshots()

	return nil
}

// serveGRPC 启动 gRPC 服务
func (a *App) serveGRPC() {
	defer a.wg.Done()

	if err := a.grpcServer.Serve(a.grpcListener); err != nil {
		// GracefulStop 会导致 Serve 返回 nil，其他错误需要记录
		if a.isRunning.Load() {
			zap.L().Error("gRPC server error", zap.Error(err))
		}
	}
}

// checkOrderRisk 调用风控服务检查订单
// 返回: (是否通过, 拒绝原因, 错误)
func (a *App) checkOrderRisk(ctx context.Context, order *model.Order) (bool, string, error) {
	// 创建带超时的 context
	timeout := time.Duration(a.cfg.Risk.Timeout) * time.Millisecond
	riskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 转换订单方向和类型为 proto enum
	var side commonv1.OrderSide
	if order.Side == model.OrderSideBuy {
		side = commonv1.OrderSide_ORDER_SIDE_BUY
	} else {
		side = commonv1.OrderSide_ORDER_SIDE_SELL
	}

	var orderType commonv1.OrderType
	if order.Type == model.OrderTypeLimit {
		orderType = commonv1.OrderType_ORDER_TYPE_LIMIT
	} else {
		orderType = commonv1.OrderType_ORDER_TYPE_MARKET
	}

	// 调用风控服务
	resp, err := a.riskClient.CheckOrder(riskCtx, &riskv1.CheckOrderRequest{
		Wallet:    order.Wallet,
		Market:    order.Market,
		Side:      side,
		OrderType: orderType,
		Price:     order.Price.String(),
		Amount:    order.Amount.String(),
	})
	if err != nil {
		return false, "", fmt.Errorf("call risk service: %w", err)
	}

	// 记录警告 (如果有)
	if len(resp.Warnings) > 0 {
		zap.L().Warn("risk check warnings",
			zap.String("order_id", order.OrderID),
			zap.Strings("warnings", resp.Warnings))
	}

	return resp.Approved, resp.RejectReason, nil
}

// handleOrder 处理订单消息
func (a *App) handleOrder(ctx context.Context, order *model.Order) error {
	startTime := time.Now()

	// 记录接收到的订单
	metrics.RecordOrderReceived(order.Market, order.Side.String(), order.Type.String())

	// 风控检查 (如果已启用)
	if a.riskClient != nil {
		approved, rejectReason, err := a.checkOrderRisk(ctx, order)
		if err != nil {
			// 风控服务调用失败，根据配置决定是否放行
			// 默认策略: 风控服务不可用时放行，避免阻塞交易
			zap.L().Warn("risk check failed, order allowed by default",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
		} else if !approved {
			// 风控拒绝
			metrics.RecordOrderProcessed(order.Market, "rejected", time.Since(startTime).Seconds())
			zap.L().Info("order rejected by risk check",
				zap.String("order_id", order.OrderID),
				zap.String("wallet", order.Wallet),
				zap.String("reason", rejectReason))

			// 发送 order-rejected 消息到 Kafka
			rejectedMsg := &model.OrderRejectedMessage{
				OrderID:      order.OrderID,
				Wallet:       order.Wallet,
				Market:       order.Market,
				Side:         int8(order.Side),
				OrderType:    int8(order.Type),
				Price:        order.Price.String(),
				Amount:       order.Amount.String(),
				Status:       "rejected",
				RejectReason: rejectReason,
				Timestamp:    time.Now().UnixMilli(),
			}
			if err := a.producer.SendOrderRejected(ctx, rejectedMsg); err != nil {
				zap.L().Error("send order rejected message failed",
					zap.String("order_id", order.OrderID),
					zap.Error(err))
			}
			return nil
		}
	}

	result, err := a.engineManager.ProcessOrder(ctx, order)
	if err != nil {
		metrics.RecordOrderProcessed(order.Market, "error", time.Since(startTime).Seconds())
		return err
	}

	if result == nil {
		metrics.RecordOrderProcessed(order.Market, "skipped", time.Since(startTime).Seconds())
		return nil // 幂等跳过
	}

	// 记录处理成功
	status := "queued"
	if len(result.Trades) > 0 {
		status = "matched"
	}
	metrics.RecordOrderProcessed(order.Market, status, time.Since(startTime).Seconds())

	// 发送成交结果并记录指标
	for _, trade := range result.Trades {
		// 记录撮合指标
		volume, _ := trade.Amount.Float64()
		value, _ := trade.QuoteAmount.Float64()
		metrics.RecordMatch(trade.Market, volume, value, 0) // 单次撮合延迟在引擎内部记录

		if err := a.producer.SendTradeResult(ctx, trade); err != nil {
			metrics.RecordKafkaError("trade-results", "send")
		} else {
			metrics.RecordKafkaMessage("trade-results", false)
		}
	}

	// 发送订单簿更新
	for _, update := range result.OrderUpdates {
		if err := a.producer.SendOrderBookUpdate(ctx, update); err != nil {
			metrics.RecordKafkaError("orderbook-updates", "send")
		} else {
			metrics.RecordKafkaMessage("orderbook-updates", false)
		}
	}

	return nil
}

// handleCancel 处理取消消息
func (a *App) handleCancel(ctx context.Context, cancel *model.CancelMessage) error {
	// 记录接收到的取消请求
	metrics.RecordCancelReceived(cancel.Market)

	result, err := a.engineManager.ProcessCancel(ctx, cancel)
	if err != nil {
		metrics.RecordCancelProcessed(cancel.Market, "error")
		return err
	}

	if result == nil {
		metrics.RecordCancelProcessed(cancel.Market, "not_found")
		return nil
	}

	// 记录取消成功
	metrics.RecordCancelProcessed(cancel.Market, "success")

	// 发送取消结果
	if err := a.producer.SendCancelResult(ctx, result); err != nil {
		metrics.RecordKafkaError("order-cancelled", "send")
		return err
	}
	metrics.RecordKafkaMessage("order-cancelled", false)
	return nil
}

// collectTrades 收集成交结果
func (a *App) collectTrades() {
	defer a.wg.Done()

	a.engineManager.CollectTrades(a.ctx, func(trade *model.TradeResult) error {
		return a.producer.SendTradeResult(a.ctx, trade)
	})
}

// collectUpdates 收集订单簿更新
func (a *App) collectUpdates() {
	defer a.wg.Done()

	a.engineManager.CollectUpdates(a.ctx, func(update *model.OrderBookUpdate) error {
		return a.producer.SendOrderBookUpdate(a.ctx, update)
	})
}

// collectCancels 收集取消结果
func (a *App) collectCancels() {
	defer a.wg.Done()

	a.engineManager.CollectCancels(a.ctx, func(cancel *model.CancelResult) error {
		return a.producer.SendCancelResult(a.ctx, cancel)
	})
}

// snapshotLoop 快照定时循环
func (a *App) snapshotLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.snapshotTicker.C:
			a.saveAllSnapshots()
		}
	}
}

// saveAllSnapshots 保存所有市场的快照
func (a *App) saveAllSnapshots() {
	offsets := a.consumer.GetOffsets()

	for _, market := range a.engineManager.GetMarkets() {
		eng, err := a.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		ob := eng.GetOrderBook()
		startTime := time.Now()
		err = a.snapshotManager.SaveSnapshot(a.ctx, market, ob, offsets)
		metrics.RecordSnapshotSave(market, time.Since(startTime).Seconds(), err)
		if err != nil {
			continue
		}

		// 两阶段快照: 确认
		if a.cfg.Snapshot.TwoPhase {
			if err := a.snapshotManager.ConfirmSnapshot(a.ctx, market); err != nil {
				metrics.SnapshotErrors.WithLabelValues(market, "confirm").Inc()
			}
		}

		// 更新订单簿指标
		a.updateOrderBookMetrics(market, eng)
	}
}

// updateOrderBookMetrics 更新订单簿指标
func (a *App) updateOrderBookMetrics(market string, eng *engine.Engine) {
	stats := eng.GetStats()
	obStats := stats.OrderBookStats

	// 计算价差百分比
	spreadPercent := 0.0
	bestBid, _ := obStats.BestBid.Float64()
	bestAsk, _ := obStats.BestAsk.Float64()
	if bestBid > 0 && bestAsk > 0 {
		spreadPercent = (bestAsk - bestBid) / bestBid * 100
	}

	// OrderCount 是总订单数，按买卖档位比例估算
	bidOrders := obStats.OrderCount / 2
	askOrders := obStats.OrderCount - bidOrders

	metrics.UpdateOrderBookMetrics(
		market,
		obStats.BidLevels,
		obStats.AskLevels,
		bidOrders,
		askOrders,
		spreadPercent,
		bestBid,
		bestAsk,
	)

	// 更新引擎状态
	metrics.SetEngineRunning(market, eng.IsRunning())
}

// recoverFromSnapshot 从快照恢复
func (a *App) recoverFromSnapshot() error {
	offsets := a.consumer.GetOffsets()
	recoveredCount := 0

	for _, market := range a.engineManager.GetMarkets() {
		snap, err := a.snapshotManager.LoadSnapshot(a.ctx, market, offsets)
		if err != nil {
			// 无快照，从头开始
			continue
		}

		// 恢复订单簿
		ob := a.snapshotManager.RestoreOrderBook(snap)
		if ob == nil {
			continue
		}

		// 获取引擎并替换订单簿
		eng, err := a.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		// 恢复订单簿状态
		eng.SetOrderBook(ob)

		// 恢复输入序列号
		eng.SetInputSequence(snap.Sequence)

		recoveredCount++
		orderCount := 0
		for _, pl := range snap.Bids {
			orderCount += len(pl.Orders)
		}
		for _, pl := range snap.Asks {
			orderCount += len(pl.Orders)
		}
		zap.L().Info("recovered snapshot",
			zap.String("market", market),
			zap.Int64("sequence", snap.Sequence),
			zap.Int("orders", orderCount))
	}

	if recoveredCount > 0 {
		zap.L().Info("snapshot recovery completed",
			zap.Int("markets_recovered", recoveredCount))
	}

	return nil
}

// GetStats 获取统计信息
func (a *App) GetStats() map[string]interface{} {
	engineStats := a.engineManager.GetStats()
	producerStats := a.producer.GetStats()

	return map[string]interface{}{
		"engines":  engineStats,
		"producer": producerStats,
		"running":  a.isRunning.Load(),
	}
}

// generateTradeID 生成交易 ID
func generateTradeID() string {
	return uuid.New().String()
}
