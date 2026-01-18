# eidos-api 对接清单

## 一、服务对接状态总览

| 服务/组件 | 状态 | 优先级 | 负责人 | 预计工时 |
|-----------|------|--------|--------|----------|
| eidos-trading gRPC | ✅ 已完成 | - | - | - |
| eidos-market gRPC | ✅ 已完成 | - | - | - |
| eidos-risk gRPC | ⏳ TODO | P1 | - | 1d |
| Kafka Consumer | ⏳ TODO (eidos-api 不需要直接消费) | P2 | - | - |
| Redis Pub/Sub | ✅ 已完成 | - | - | - |
| Nacos 服务发现 | ⏳ TODO | P1 | - | 1d |
| EIP-712 签名验证 | ⏳ TODO | P0 | - | 2d |
| Prometheus 监控 | ✅ 已完成 | - | - | - |

---

## 二、eidos-market gRPC 对接

### 2.1 接口列表

```protobuf
// api/proto/market/v1/market.proto

service MarketService {
  // 获取所有交易对
  rpc ListMarkets(ListMarketsRequest) returns (ListMarketsResponse);

  // 获取单个 Ticker
  rpc GetTicker(GetTickerRequest) returns (TickerResponse);

  // 获取所有 Tickers
  rpc ListTickers(ListTickersRequest) returns (ListTickersResponse);

  // 获取深度
  rpc GetDepth(GetDepthRequest) returns (DepthResponse);

  // 获取 K 线
  rpc GetKlines(GetKlinesRequest) returns (GetKlinesResponse);

  // 获取最近成交
  rpc GetRecentTrades(GetRecentTradesRequest) returns (GetRecentTradesResponse);
}
```

### 2.2 实现步骤

**步骤 1:** 创建 Market gRPC 客户端

文件: `internal/client/market_client.go`

```go
package client

import (
    "context"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "github.com/eidos-exchange/eidos/eidos-api/api/proto/market/v1"
)

type MarketClient struct {
    conn   *grpc.ClientConn
    client pb.MarketServiceClient
}

func NewMarketClient(addr string) (*MarketClient, error) {
    conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, err
    }

    return &MarketClient{
        conn:   conn,
        client: pb.NewMarketServiceClient(conn),
    }, nil
}

func (c *MarketClient) ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    resp, err := c.client.ListMarkets(ctx, &pb.ListMarketsRequest{})
    if err != nil {
        return nil, err
    }

    return convertMarkets(resp.Markets), nil
}

// TODO: 实现其他方法...
```

**步骤 2:** 更新 MarketService 接口

文件: `internal/service/market_service.go`

```go
package service

// MarketService 市场数据服务接口
type MarketService interface {
    ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error)
    GetTicker(ctx context.Context, market string) (*dto.TickerResponse, error)
    ListTickers(ctx context.Context) ([]*dto.TickerResponse, error)
    GetDepth(ctx context.Context, market string, limit int) (*dto.DepthResponse, error)
    GetKlines(ctx context.Context, market, interval string, startTime, endTime int64, limit int) ([]*dto.KlineResponse, error)
    GetRecentTrades(ctx context.Context, market string, limit int) ([]*dto.RecentTradeResponse, error)
}

// MarketServiceImpl 使用 gRPC 客户端的实现
type MarketServiceImpl struct {
    client *client.MarketClient
}

func NewMarketService(client *client.MarketClient) *MarketServiceImpl {
    return &MarketServiceImpl{client: client}
}

func (s *MarketServiceImpl) ListMarkets(ctx context.Context) ([]*dto.MarketResponse, error) {
    return s.client.ListMarkets(ctx)
}

// TODO: 实现其他方法...
```

**步骤 3:** 更新依赖注入

文件: `internal/app/provider.go`

```go
// 添加 MarketClient 初始化
marketClient, err := client.NewMarketClient(cfg.GRPC.Market.Addr)
if err != nil {
    return nil, fmt.Errorf("create market client: %w", err)
}

marketService := service.NewMarketService(marketClient)
marketHandler := handler.NewMarketHandler(marketService)
```

---

## 三、Kafka Consumer 对接

### 3.1 需要消费的 Topic

| Topic | 说明 | 数据格式 | 处理逻辑 |
|-------|------|---------|----------|
| `order-updates` | 订单状态更新 | JSON | WebSocket 推送给用户 |
| `balance-updates` | 余额变动 | JSON | WebSocket 推送给用户 |
| `settlements` | 结算完成通知 | JSON | WebSocket 推送给用户 |

### 3.2 消息格式定义

```go
// internal/kafka/message.go

// OrderUpdateMessage 订单更新消息
type OrderUpdateMessage struct {
    OrderID   string `json:"order_id"`
    Wallet    string `json:"wallet"`
    Market    string `json:"market"`
    Status    string `json:"status"`
    FilledAmt string `json:"filled_amount"`
    UpdatedAt int64  `json:"updated_at"`
}

// BalanceUpdateMessage 余额更新消息
type BalanceUpdateMessage struct {
    Wallet    string `json:"wallet"`
    Token     string `json:"token"`
    Available string `json:"available"`
    Locked    string `json:"locked"`
    UpdatedAt int64  `json:"updated_at"`
}

// SettlementMessage 结算消息
type SettlementMessage struct {
    SettlementID string `json:"settlement_id"`
    OrderID      string `json:"order_id"`
    Wallet       string `json:"wallet"`
    Status       string `json:"status"`
    TxHash       string `json:"tx_hash,omitempty"`
    UpdatedAt    int64  `json:"updated_at"`
}
```

### 3.3 实现步骤

**步骤 1:** 创建 Kafka Consumer

文件: `internal/kafka/consumer.go`

```go
package kafka

import (
    "context"
    "encoding/json"

    "github.com/IBM/sarama"
    "go.uber.org/zap"

    "github.com/eidos-exchange/eidos/eidos-api/internal/ws"
)

type Consumer struct {
    consumer sarama.ConsumerGroup
    hub      *ws.Hub
    logger   *zap.Logger
    topics   []string
}

type Config struct {
    Brokers       []string
    ConsumerGroup string
    Topics        []string
}

func NewConsumer(cfg Config, hub *ws.Hub, logger *zap.Logger) (*Consumer, error) {
    config := sarama.NewConfig()
    config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
        sarama.NewBalanceStrategyRoundRobin(),
    }
    config.Consumer.Offsets.Initial = sarama.OffsetNewest

    consumer, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.ConsumerGroup, config)
    if err != nil {
        return nil, err
    }

    return &Consumer{
        consumer: consumer,
        hub:      hub,
        logger:   logger,
        topics:   cfg.Topics,
    }, nil
}

func (c *Consumer) Start(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return c.consumer.Close()
        default:
            if err := c.consumer.Consume(ctx, c.topics, c); err != nil {
                c.logger.Error("kafka consume error", zap.Error(err))
            }
        }
    }
}

func (c *Consumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        c.handleMessage(msg)
        session.MarkMessage(msg, "")
    }
    return nil
}

func (c *Consumer) handleMessage(msg *sarama.ConsumerMessage) {
    switch msg.Topic {
    case "order-updates":
        c.handleOrderUpdate(msg.Value)
    case "balance-updates":
        c.handleBalanceUpdate(msg.Value)
    case "settlements":
        c.handleSettlement(msg.Value)
    default:
        c.logger.Warn("unknown topic", zap.String("topic", msg.Topic))
    }
}

func (c *Consumer) handleOrderUpdate(data []byte) {
    var msg OrderUpdateMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        c.logger.Error("unmarshal order update", zap.Error(err))
        return
    }

    // 推送给订阅了该钱包订单频道的客户端
    wsMsg := ws.NewUpdateMessage(ws.ChannelOrders, msg.Wallet, msg)
    c.hub.Broadcast(ws.ChannelOrders, msg.Wallet, wsMsg)
}

func (c *Consumer) handleBalanceUpdate(data []byte) {
    var msg BalanceUpdateMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        c.logger.Error("unmarshal balance update", zap.Error(err))
        return
    }

    // 推送给订阅了该钱包的客户端
    wsMsg := ws.NewUpdateMessage(ws.ChannelOrders, msg.Wallet, msg)
    c.hub.Broadcast(ws.ChannelOrders, msg.Wallet, wsMsg)
}

func (c *Consumer) handleSettlement(data []byte) {
    var msg SettlementMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        c.logger.Error("unmarshal settlement", zap.Error(err))
        return
    }

    wsMsg := ws.NewUpdateMessage(ws.ChannelOrders, msg.Wallet, msg)
    c.hub.Broadcast(ws.ChannelOrders, msg.Wallet, wsMsg)
}
```

**步骤 2:** 集成到应用启动流程

文件: `internal/app/app.go`

```go
func (a *App) Run() error {
    // ... 其他初始化 ...

    // 启动 Kafka Consumer
    if a.cfg.Kafka.Enabled {
        kafkaConsumer, err := kafka.NewConsumer(kafka.Config{
            Brokers:       a.cfg.Kafka.Brokers,
            ConsumerGroup: a.cfg.Kafka.ConsumerGroup,
            Topics:        []string{"order-updates", "balance-updates", "settlements"},
        }, a.hub, a.logger)
        if err != nil {
            return fmt.Errorf("create kafka consumer: %w", err)
        }

        go func() {
            if err := kafkaConsumer.Start(context.Background()); err != nil {
                a.logger.Error("kafka consumer error", zap.Error(err))
            }
        }()
    }

    // ... 启动 HTTP 服务 ...
}
```

---

## 四、Redis Pub/Sub 对接

### 4.1 需要订阅的频道

| 频道模式 | 数据源 | 说明 |
|---------|--------|------|
| `eidos:ticker:{market}` | eidos-market | Ticker 实时更新 |
| `eidos:depth:{market}` | eidos-market | 深度实时更新 |
| `eidos:kline:{market}:{interval}` | eidos-market | K 线实时更新 |
| `eidos:trades:{market}` | eidos-market | 成交实时更新 |

### 4.2 完善 Subscriber 实现

文件: `internal/ws/subscriber.go`

```go
// TODO: 完善以下方法

func (s *Subscriber) Start(ctx context.Context) error {
    pubsub := s.rdb.PSubscribe(ctx,
        "eidos:ticker:*",
        "eidos:depth:*",
        "eidos:kline:*",
        "eidos:trades:*",
    )
    defer pubsub.Close()

    ch := pubsub.Channel()
    for {
        select {
        case <-ctx.Done():
            return nil
        case msg := <-ch:
            s.handleMessage(msg.Channel, []byte(msg.Payload))
        }
    }
}

func (s *Subscriber) handleMessage(channel string, payload []byte) {
    // 解析频道名获取 channel 类型和 market
    // eidos:ticker:BTC-USDC -> channel=ticker, market=BTC-USDC
    parts := strings.SplitN(channel, ":", 3)
    if len(parts) < 3 {
        s.logger.Warn("invalid channel format", zap.String("channel", channel))
        return
    }

    channelType := ws.Channel(parts[1])
    market := parts[2]

    // 解析数据
    var data interface{}
    if err := json.Unmarshal(payload, &data); err != nil {
        s.logger.Error("unmarshal payload", zap.Error(err))
        return
    }

    // 广播给订阅者
    msg := ws.NewUpdateMessage(channelType, market, data)
    s.hub.Broadcast(channelType, market, msg)
}
```

---

## 五、Nacos 服务发现对接

### 5.1 配置结构

```yaml
# config/config.yaml
nacos:
  enabled: true
  server_addr: "127.0.0.1:8848"
  namespace: "eidos-dev"
  service:
    name: "eidos-api"
    group: "DEFAULT_GROUP"
    cluster: "DEFAULT"
    weight: 1.0
    metadata:
      version: "1.0.0"
      protocol: "http"
```

### 5.2 实现步骤

**步骤 1:** 使用 eidos-common nacos 包

文件: `internal/app/app.go`

```go
import (
    "github.com/eidos-exchange/eidos/eidos-common/pkg/nacos"
)

func (a *App) initNacos() error {
    if !a.cfg.Nacos.Enabled {
        return nil
    }

    // 创建 Nacos 客户端
    client, err := nacos.NewClient(nacos.Config{
        ServerAddr: a.cfg.Nacos.ServerAddr,
        Namespace:  a.cfg.Nacos.Namespace,
    })
    if err != nil {
        return err
    }

    // 注册服务
    err = client.RegisterInstance(nacos.Instance{
        ServiceName: a.cfg.Nacos.Service.Name,
        IP:          getLocalIP(),
        Port:        a.cfg.Server.Port,
        Weight:      a.cfg.Nacos.Service.Weight,
        Metadata:    a.cfg.Nacos.Service.Metadata,
    })
    if err != nil {
        return err
    }

    a.nacosClient = client
    return nil
}
```

**步骤 2:** 动态获取服务地址

```go
// 获取 eidos-trading 服务地址
func (a *App) getTradingAddr() (string, error) {
    if a.nacosClient == nil {
        return a.cfg.GRPC.Trading.Addr, nil
    }

    instance, err := a.nacosClient.SelectOneHealthyInstance("eidos-trading")
    if err != nil {
        return "", err
    }

    return fmt.Sprintf("%s:%d", instance.IP, instance.Port), nil
}
```

---

## 六、EIP-712 签名验证对接

### 6.1 当前状态

当前使用 Mock 模式，跳过签名验证。需要完善以下功能：

1. 实际签名验证
2. 时间戳校验
3. Nonce 重放保护

### 6.2 实现步骤

**步骤 1:** 添加依赖

```bash
go get github.com/ethereum/go-ethereum
```

**步骤 2:** 实现签名验证

文件: `internal/middleware/auth.go`

```go
import (
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// verifySignature 验证 EIP-712 签名
func (m *AuthMiddleware) verifySignature(wallet string, timestamp int64, message string, signature string) error {
    // 1. 检查时间戳
    now := time.Now().UnixMilli()
    if abs(now-timestamp) > m.cfg.TimestampTolerance.Milliseconds() {
        return fmt.Errorf("timestamp expired")
    }

    // 2. 检查 nonce 重放
    nonce := fmt.Sprintf("%s:%d:%s", wallet, timestamp, message)
    used, err := m.replayGuard.CheckAndMark(context.Background(), nonce)
    if err != nil {
        return fmt.Errorf("check replay: %w", err)
    }
    if used {
        return fmt.Errorf("signature already used")
    }

    // 3. 构建 EIP-712 TypedData
    typedData := apitypes.TypedData{
        Types: apitypes.Types{
            "EIP712Domain": {
                {Name: "name", Type: "string"},
                {Name: "version", Type: "string"},
                {Name: "chainId", Type: "uint256"},
                {Name: "verifyingContract", Type: "address"},
            },
            "Authentication": {
                {Name: "wallet", Type: "address"},
                {Name: "timestamp", Type: "uint256"},
                {Name: "message", Type: "string"},
            },
        },
        PrimaryType: "Authentication",
        Domain: apitypes.TypedDataDomain{
            Name:              m.cfg.EIP712.Name,
            Version:           m.cfg.EIP712.Version,
            ChainId:           math.NewHexOrDecimal256(m.cfg.EIP712.ChainID),
            VerifyingContract: m.cfg.EIP712.VerifyingContract,
        },
        Message: apitypes.TypedDataMessage{
            "wallet":    wallet,
            "timestamp": fmt.Sprintf("%d", timestamp),
            "message":   message,
        },
    }

    // 4. 计算 hash
    hash, _, err := apitypes.TypedDataAndHash(typedData)
    if err != nil {
        return fmt.Errorf("compute hash: %w", err)
    }

    // 5. 解码签名
    sig := common.FromHex(signature)
    if len(sig) != 65 {
        return fmt.Errorf("invalid signature length")
    }

    // 调整 v 值
    if sig[64] >= 27 {
        sig[64] -= 27
    }

    // 6. 恢复公钥
    pubkey, err := crypto.SigToPub(hash, sig)
    if err != nil {
        return fmt.Errorf("recover pubkey: %w", err)
    }

    // 7. 验证地址
    recovered := crypto.PubkeyToAddress(*pubkey)
    expected := common.HexToAddress(wallet)

    if recovered != expected {
        return fmt.Errorf("signature mismatch: expected %s, got %s", expected.Hex(), recovered.Hex())
    }

    return nil
}
```

---

## 七、Prometheus 监控对接

### 7.1 需要暴露的指标

| 指标名 | 类型 | 标签 | 说明 |
|--------|------|------|------|
| `eidos_api_http_requests_total` | Counter | method, path, status | HTTP 请求总数 |
| `eidos_api_http_request_duration_seconds` | Histogram | method, path | HTTP 请求耗时 |
| `eidos_api_ws_connections_total` | Gauge | - | WebSocket 连接数 |
| `eidos_api_ws_subscriptions_total` | Gauge | channel | WebSocket 订阅数 |
| `eidos_api_grpc_requests_total` | Counter | service, method, status | gRPC 请求数 |
| `eidos_api_grpc_request_duration_seconds` | Histogram | service, method | gRPC 请求耗时 |

### 7.2 实现步骤

**步骤 1:** 创建 metrics 包

文件: `internal/metrics/metrics.go`

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    HTTPRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "eidos_api_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "eidos_api_http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )

    WSConnectionsTotal = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "eidos_api_ws_connections_total",
            Help: "Total WebSocket connections",
        },
    )

    WSSubscriptionsTotal = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "eidos_api_ws_subscriptions_total",
            Help: "Total WebSocket subscriptions",
        },
        []string{"channel"},
    )
)
```

**步骤 2:** 添加 metrics 中间件

文件: `internal/middleware/metrics.go`

```go
package middleware

import (
    "strconv"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/eidos-exchange/eidos/eidos-api/internal/metrics"
)

func Metrics() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        c.Next()

        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())
        path := c.FullPath()
        if path == "" {
            path = "unknown"
        }

        metrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
        metrics.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
    }
}
```

**步骤 3:** 暴露 /metrics 端点

文件: `internal/router/router.go`

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

func SetupRouter(r *gin.Engine) {
    // Prometheus metrics
    r.GET("/metrics", gin.WrapH(promhttp.Handler()))

    // ... 其他路由 ...
}
```

---

## 八、测试覆盖率目标

| 模块 | 当前覆盖率 | 目标覆盖率 |
|------|-----------|-----------|
| middleware | ~60% | 80% |
| handler | ~50% | 75% |
| client | ~30% | 70% |
| ws | ~40% | 70% |
| cache | ~80% | 85% |
| ratelimit | ~70% | 80% |
| **总体** | **~35%** | **≥75%** |

---

## 九、待办事项优先级

### P0 - 必须完成 (上线阻塞)

- [ ] eidos-market gRPC 对接
- [ ] Kafka Consumer 实现
- [ ] Redis Pub/Sub 订阅
- [ ] EIP-712 签名验证

### P1 - 重要 (上线后优化)

- [ ] Nacos 服务发现
- [ ] Prometheus 监控
- [ ] 测试覆盖率提升到 75%
- [ ] 性能压测与优化

### P2 - 可选 (后续迭代)

- [ ] 请求日志采样
- [ ] 灰度发布支持
- [ ] A/B 测试支持
