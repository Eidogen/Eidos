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
//
// 新增功能:
//   - 市场配置热加载: 从 Nacos 动态加载市场配置
//   - 外部指数价格: 多数据源聚合的指数价格管理
//   - 高可用: Leader 选举和 Standby 模式
//   - 完整监控: 撮合延迟、订单簿深度、吞吐量指标
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
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	matchingv1 "github.com/eidos-exchange/eidos/proto/matching/v1"
	riskv1 "github.com/eidos-exchange/eidos/proto/risk/v1"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/config"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/engine"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/ha"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/handler"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/metrics"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/price"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/snapshot"
	"log/slog"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
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

	// Nacos 服务注册和配置
	nacosClient       *nacos.Client
	nacosConfigCenter *nacos.ConfigCenter
	nacosRegistry     *nacos.Registry
	nacosConfigLoader *config.NacosConfigLoader
	serviceInst       *nacos.ServiceInstance

	// eidos-risk 风控客户端
	riskConn   *grpc.ClientConn
	riskClient riskv1.RiskServiceClient

	// 高可用组件
	leaderElection *ha.LeaderElection
	standbyManager *ha.StandbyManager

	// 指数价格管理器
	indexPriceManager *price.IndexPriceManager

	// 状态
	isRunning atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 定时器
	snapshotTicker *time.Ticker
	metricsTicker  *time.Ticker

	// 吞吐量统计
	lastOrderCount int64
	lastTradeCount int64
	lastStatTime   time.Time
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
			slog.Warn("init nacos failed, service discovery disabled", "error", err)
		}
	}

	// 初始化 eidos-risk 风控客户端 (可选，配置为空则跳过)
	if cfg.Risk.Enabled && cfg.Risk.Addr != "" {
		if err := app.initRisk(); err != nil {
			slog.Warn("init risk client failed, risk checking disabled", "error", err)
		}
	}

	// 初始化高可用组件 (可选)
	if cfg.HA.Enabled {
		if err := app.initHA(); err != nil {
			slog.Warn("init HA failed, running in standalone mode", "error", err)
		}
	}

	// 初始化指数价格管理器
	app.initIndexPriceManager()

	// 初始化 Nacos 配置热加载 (可选)
	if app.nacosClient != nil {
		if err := app.initNacosConfigLoader(); err != nil {
			slog.Warn("init nacos config loader failed", "error", err)
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

	// 创建配置中心客户端
	configCenterCfg := &nacos.ConfigCenterConfig{
		ServerAddr: a.cfg.Nacos.ServerAddr,
		Namespace:  a.cfg.Nacos.Namespace,
		Group:      a.cfg.Nacos.Group,
		LogDir:     "/tmp/nacos/log",
		CacheDir:   "/tmp/nacos/cache",
		LogLevel:   "warn",
		TimeoutMs:  5000,
	}
	configCenter, err := nacos.NewConfigCenter(configCenterCfg)
	if err != nil {
		return fmt.Errorf("create nacos config center: %w", err)
	}
	a.nacosConfigCenter = configCenter

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

	slog.Info("nacos client initialized",
		"server", a.cfg.Nacos.ServerAddr,
		"namespace", a.cfg.Nacos.Namespace)

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

	slog.Info("risk client initialized",
		"addr", a.cfg.Risk.Addr,
		"timeout_ms", a.cfg.Risk.Timeout)

	return nil
}

// initHA 初始化高可用组件
func (a *App) initHA() error {
	// 创建 Leader 选举器
	leaderCfg := &ha.LeaderElectionConfig{
		RedisClient:   a.redisClient,
		KeyPrefix:     "eidos:matching:leader",
		NodeID:        a.cfg.Service.NodeID,
		LeaseDuration: a.cfg.HA.FailoverTimeout * 3, // 租约时间是超时时间的3倍
		RenewInterval: a.cfg.HA.HeartbeatInterval,
		RetryInterval: a.cfg.HA.HeartbeatInterval,
		OnLeaderChange: func(isLeader bool, oldState, newState ha.LeaderState) {
			a.handleLeaderChange(isLeader, oldState, newState)
		},
	}

	var err error
	a.leaderElection, err = ha.NewLeaderElection(leaderCfg)
	if err != nil {
		return fmt.Errorf("create leader election: %w", err)
	}

	// 创建 Standby 管理器
	standbyyCfg := &ha.StandbyManagerConfig{
		RedisClient:       a.redisClient,
		NodeID:            a.cfg.Service.NodeID,
		KeyPrefix:         "eidos:matching:ha",
		SyncInterval:      time.Second,
		HeartbeatInterval: a.cfg.HA.HeartbeatInterval,
		FailoverTimeout:   a.cfg.HA.FailoverTimeout,
		OnStateChange: func(oldMode, newMode ha.StandbyMode) {
			slog.Info("standby mode changed",
				"old_mode", oldMode.String(),
				"new_mode", newMode.String())
			metrics.SetLeaderState(a.cfg.Service.NodeID, int(newMode))
		},
	}

	a.standbyManager, err = ha.NewStandbyManager(standbyyCfg, a.leaderElection)
	if err != nil {
		return fmt.Errorf("create standby manager: %w", err)
	}

	slog.Info("HA components initialized",
		"node_id", a.cfg.Service.NodeID,
		"heartbeat_interval", a.cfg.HA.HeartbeatInterval,
		"failover_timeout", a.cfg.HA.FailoverTimeout)

	return nil
}

// handleLeaderChange 处理 Leader 变更
func (a *App) handleLeaderChange(isLeader bool, oldState, newState ha.LeaderState) {
	slog.Info("leader state changed",
		"is_leader", isLeader,
		"old_state", oldState.String(),
		"new_state", newState.String())

	metrics.SetLeaderState(a.cfg.Service.NodeID, int(newState))

	if isLeader && oldState != ha.StateLeader {
		// 成为 Leader，记录故障转移
		metrics.RecordFailover(a.cfg.Service.NodeID)
		slog.Info("this node became the leader",
			"node_id", a.cfg.Service.NodeID)
	}
}

// initIndexPriceManager 初始化指数价格管理器
func (a *App) initIndexPriceManager() {
	cfg := &price.IndexPriceManagerConfig{
		UpdateInterval:     time.Second,
		StaleThreshold:     30 * time.Second,
		AggMethod:          price.AggMedian,
		MinValidSources:    1,
		DeviationThreshold: decimal.NewFromFloat(0.1), // 10%
	}

	a.indexPriceManager = price.NewIndexPriceManager(cfg)

	// 设置价格更新回调
	a.indexPriceManager.SetCallback(func(symbol string, indexPrice *price.IndexPrice) {
		// 将指数价格更新到对应的撮合引擎
		if err := a.engineManager.UpdateIndexPrice(symbol, indexPrice.Price); err != nil {
			slog.Debug("update index price to engine failed",
				"symbol", symbol,
				"error", err)
		}

		// 计算与最新成交价的偏差
		eng, err := a.engineManager.GetEngine(symbol)
		if err == nil {
			lastPrice := eng.GetStats().MatcherStats.LastPrice
			if !lastPrice.IsZero() {
				deviation := indexPrice.Price.Sub(lastPrice).Abs().Div(lastPrice).Mul(decimal.NewFromInt(100))
				deviationFloat, _ := deviation.Float64()
				metrics.UpdateIndexPriceDeviation(symbol, deviationFloat)
			}
		}
	})

	// 添加一个静态价格源作为默认实现 (生产环境应该接入真实的价格源)
	staticSource := price.NewStaticPriceSource("static", 100)
	a.indexPriceManager.AddSource(staticSource)

	slog.Info("index price manager initialized")
}

// initNacosConfigLoader 初始化 Nacos 配置热加载
func (a *App) initNacosConfigLoader() error {
	loaderCfg := &config.NacosLoaderConfig{
		Client:       a.nacosConfigCenter,
		DataID:       "eidos-matching-markets",
		Group:        a.cfg.Nacos.Group,
		PollInterval: 30 * time.Second,
		Callback:     a.handleMarketConfigChange,
	}

	var err error
	a.nacosConfigLoader, err = config.NewNacosConfigLoader(loaderCfg)
	if err != nil {
		return fmt.Errorf("create nacos config loader: %w", err)
	}

	slog.Info("nacos config loader initialized",
		"dataId", loaderCfg.DataID,
		"group", loaderCfg.Group)

	return nil
}

// handleMarketConfigChange 处理市场配置变更
func (a *App) handleMarketConfigChange(added, updated, removed []*config.EngineMarketConfig) {
	// 处理新增的市场
	for _, cfg := range added {
		if err := config.ValidateMarketConfig(cfg); err != nil {
			slog.Warn("invalid market config, skipping",
				"symbol", cfg.Symbol,
				"error", err)
			continue
		}

		engineCfg := &engine.MarketConfig{
			Symbol:        cfg.Symbol,
			BaseToken:     cfg.BaseToken,
			QuoteToken:    cfg.QuoteToken,
			PriceDecimals: cfg.PriceDecimals,
			SizeDecimals:  cfg.SizeDecimals,
			MinSize:       cfg.MinSize,
			TickSize:      cfg.TickSize,
			MakerFeeRate:  cfg.MakerFeeRate,
			TakerFeeRate:  cfg.TakerFeeRate,
			MaxSlippage:   cfg.MaxSlippage,
		}

		if err := a.engineManager.AddMarket(engineCfg); err != nil {
			slog.Error("add market failed",
				"symbol", cfg.Symbol,
				"error", err)
		} else {
			slog.Info("market added via hot reload",
				"symbol", cfg.Symbol)
		}
	}

	// 处理更新的市场
	for _, cfg := range updated {
		if err := config.ValidateMarketConfig(cfg); err != nil {
			slog.Warn("invalid market config, skipping update",
				"symbol", cfg.Symbol,
				"error", err)
			continue
		}

		engineCfg := &engine.MarketConfig{
			Symbol:        cfg.Symbol,
			BaseToken:     cfg.BaseToken,
			QuoteToken:    cfg.QuoteToken,
			PriceDecimals: cfg.PriceDecimals,
			SizeDecimals:  cfg.SizeDecimals,
			MinSize:       cfg.MinSize,
			TickSize:      cfg.TickSize,
			MakerFeeRate:  cfg.MakerFeeRate,
			TakerFeeRate:  cfg.TakerFeeRate,
			MaxSlippage:   cfg.MaxSlippage,
		}

		if err := a.engineManager.UpdateMarket(engineCfg); err != nil {
			slog.Error("update market failed",
				"symbol", cfg.Symbol,
				"error", err)
		} else {
			slog.Info("market updated via hot reload",
				"symbol", cfg.Symbol)
		}
	}

	// 处理删除的市场 (通常不建议动态删除，记录警告)
	for _, cfg := range removed {
		slog.Warn("market removal requested via hot reload",
			"symbol", cfg.Symbol,
			"note", "market removal is not recommended, manual restart required")
	}
}

// Start 启动应用
func (a *App) Start() error {
	if a.isRunning.Load() {
		return fmt.Errorf("app already running")
	}

	// 尝试从快照恢复
	if err := a.recoverFromSnapshot(); err != nil {
		// 恢复失败不阻止启动，记录日志即可
		slog.Warn("recover from snapshot failed", "error", err)
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

	// 启动指标收集
	a.metricsTicker = time.NewTicker(time.Second)
	a.lastStatTime = time.Now()
	a.wg.Add(1)
	go a.metricsLoop()

	// 启动 gRPC 服务
	a.wg.Add(1)
	go a.serveGRPC()

	// 启动高可用组件
	if a.leaderElection != nil {
		if err := a.leaderElection.Start(); err != nil {
			slog.Error("start leader election failed", "error", err)
		}
	}
	if a.standbyManager != nil {
		if err := a.standbyManager.Start(a.engineManager.GetMarkets()); err != nil {
			slog.Error("start standby manager failed", "error", err)
		}
	}

	// 启动指数价格管理器
	if a.indexPriceManager != nil {
		if err := a.indexPriceManager.Start(a.engineManager.GetMarkets()); err != nil {
			slog.Warn("start index price manager failed", "error", err)
		}
	}

	// 启动 Nacos 配置热加载
	if a.nacosConfigLoader != nil {
		if err := a.nacosConfigLoader.Start(); err != nil {
			slog.Warn("start nacos config loader failed", "error", err)
		}
	}

	// 注册服务到 Nacos
	if a.nacosRegistry != nil && a.serviceInst != nil {
		if err := a.nacosRegistry.Register(a.serviceInst); err != nil {
			slog.Error("register service to nacos failed", "error", err)
		} else {
			slog.Info("service registered to nacos",
				"service", a.serviceInst.ServiceName,
				"ip", a.serviceInst.IP,
				"port", a.serviceInst.Port)
		}
	}

	a.isRunning.Store(true)
	slog.Info("eidos-matching started",
		"grpc_port", a.cfg.Service.GRPCPort,
		"markets", a.engineManager.GetMarkets())
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
			slog.Error("deregister service from nacos failed", "error", err)
		} else {
			slog.Info("service deregistered from nacos",
				"service", a.serviceInst.ServiceName)
		}
	}

	// 关闭 Nacos 客户端
	if a.nacosClient != nil {
		a.nacosClient.Close()
	}

	// 关闭 Risk gRPC 连接
	if a.riskConn != nil {
		if err := a.riskConn.Close(); err != nil {
			slog.Error("close risk connection failed", "error", err)
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

	// 停止指标定时器
	if a.metricsTicker != nil {
		a.metricsTicker.Stop()
	}

	// 停止 Nacos 配置热加载
	if a.nacosConfigLoader != nil {
		a.nacosConfigLoader.Stop()
	}

	// 停止指数价格管理器
	if a.indexPriceManager != nil {
		a.indexPriceManager.Stop()
	}

	// 停止高可用组件
	if a.standbyManager != nil {
		a.standbyManager.Stop()
	}
	if a.leaderElection != nil {
		a.leaderElection.Stop()
	}

	// 停止消费者
	if err := a.consumer.Stop(); err != nil {
		slog.Error("stop consumer failed", "error", err)
	}

	// 停止引擎
	a.engineManager.Stop()

	// 等待所有协程结束
	a.wg.Wait()

	// 关闭生产者
	if err := a.producer.Close(); err != nil {
		slog.Error("close producer failed", "error", err)
	}

	// 关闭 Redis
	if err := a.redisClient.Close(); err != nil {
		slog.Error("close redis failed", "error", err)
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
			slog.Error("gRPC server error", "error", err)
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
		slog.Warn("risk check warnings",
			"order_id", order.OrderID,
			"warnings", resp.Warnings)
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
			slog.Warn("risk check failed, order allowed by default",
				"order_id", order.OrderID,
				"error", err)
		} else if !approved {
			// 风控拒绝
			metrics.RecordOrderProcessed(order.Market, "rejected", time.Since(startTime).Seconds())
			slog.Info("order rejected by risk check",
				"order_id", order.OrderID,
				"wallet", order.Wallet,
				"reason", rejectReason)

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
				slog.Error("send order rejected message failed",
					"order_id", order.OrderID,
					"error", err)
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

// metricsLoop 指标收集循环
func (a *App) metricsLoop() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.metricsTicker.C:
			a.collectMetrics()
		}
	}
}

// collectMetrics 收集性能指标
func (a *App) collectMetrics() {
	now := time.Now()
	elapsed := now.Sub(a.lastStatTime).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	for _, market := range a.engineManager.GetMarkets() {
		eng, err := a.engineManager.GetEngine(market)
		if err != nil {
			continue
		}

		stats := eng.GetStats()
		obStats := stats.OrderBookStats

		// 计算吞吐量
		currentOrders := stats.OrdersProcessed
		currentTrades := stats.TradesGenerated
		ordersPerSec := float64(currentOrders-a.lastOrderCount) / elapsed
		tradesPerSec := float64(currentTrades-a.lastTradeCount) / elapsed
		metrics.UpdateThroughputMetrics(market, ordersPerSec, tradesPerSec)

		// 记录撮合延迟 (微秒)
		metrics.RecordMatchLatencyMicros(market, float64(stats.AvgLatencyUs))

		// 更新订单簿深度指标
		bestBid, _ := obStats.BestBid.Float64()
		bestAsk, _ := obStats.BestAsk.Float64()

		// 计算中间价
		midPrice := 0.0
		if bestBid > 0 && bestAsk > 0 {
			midPrice = (bestBid + bestAsk) / 2
		}

		// 计算订单簿不平衡度 (这里简化为档位数量比例)
		imbalance := 0.0
		totalLevels := obStats.BidLevels + obStats.AskLevels
		if totalLevels > 0 {
			imbalance = float64(obStats.BidLevels-obStats.AskLevels) / float64(totalLevels)
		}

		metrics.UpdateOrderBookVolumeMetrics(market, 0, 0, midPrice, imbalance) // 实际 volume 需要从订单簿计算

		// 更新最优价位数量 (简化实现)
		metrics.UpdateTopOfBookMetrics(market, float64(obStats.BidLevels), float64(obStats.AskLevels))

		// 更新订单簿内存估算 (每个订单约 200 字节)
		estimatedMemory := int64(obStats.OrderCount * 200)
		metrics.UpdateOrderBookMemory(market, estimatedMemory)

		// 更新通道缓冲区使用率
		// 注意: 通道长度获取需要在 Engine 中暴露，这里简化处理
		metrics.UpdateChannelBufferUsage(market, "trades", 0)
		metrics.UpdateChannelBufferUsage(market, "updates", 0)
		metrics.UpdateChannelBufferUsage(market, "cancels", 0)
	}

	// 更新统计基准
	engineStats := a.engineManager.GetStats()
	totalOrders := int64(0)
	totalTrades := int64(0)
	for _, stat := range engineStats {
		totalOrders += stat.OrdersProcessed
		totalTrades += stat.TradesGenerated
	}
	a.lastOrderCount = totalOrders
	a.lastTradeCount = totalTrades
	a.lastStatTime = now

	// 更新全局消息吞吐量
	metrics.UpdateMessageThroughput("orders", float64(totalOrders-a.lastOrderCount)/elapsed)
	metrics.UpdateMessageThroughput("trades", float64(totalTrades-a.lastTradeCount)/elapsed)
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
		slog.Info("recovered snapshot",
			"market", market,
			"sequence", snap.Sequence,
			"orders", orderCount)
	}

	if recoveredCount > 0 {
		slog.Info("snapshot recovery completed",
			"markets_recovered", recoveredCount)
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
