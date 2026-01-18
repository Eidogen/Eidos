# eidos-api 架构文档

## 一、服务概述

eidos-api 是 Eidos 交易系统的 REST + WebSocket 网关服务，负责：
1. 提供 RESTful API 供前端调用
2. 提供 WebSocket 实时推送服务
3. EIP-712 签名认证与重放攻击保护
4. 请求限流与安全防护

## 二、水平扩展方案

### 2.1 无状态设计

eidos-api 采用无状态设计，支持水平扩展：

```
                    ┌──────────────────────────────────────────┐
                    │              Load Balancer               │
                    │         (Nginx / AWS ALB / K8s)          │
                    └──────────────────────────────────────────┘
                           │         │         │         │
                    ┌──────┴──┐ ┌────┴────┐ ┌──┴──────┐ ┌┴────────┐
                    │ api-1  │ │ api-2   │ │ api-3  │ │ api-N   │
                    │ :8080  │ │ :8080   │ │ :8080  │ │ :8080   │
                    └────────┘ └─────────┘ └────────┘ └─────────┘
                           │         │         │         │
                    ┌──────┴─────────┴─────────┴─────────┴──────┐
                    │               Shared Infrastructure        │
                    │  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐  │
                    │  │ Redis │ │ Kafka │ │ gRPC  │ │ Nacos │  │
                    │  │Cluster│ │Cluster│ │Services│ │       │  │
                    │  └───────┘ └───────┘ └───────┘ └───────┘  │
                    └───────────────────────────────────────────┘
```

### 2.2 扩展关键点

| 组件 | 扩展策略 | 说明 |
|------|---------|------|
| **API 实例** | 水平扩展 | 无状态，可任意增加实例 |
| **WebSocket** | Sticky Session + Redis Pub/Sub | 连接保持 + 跨实例消息广播 |
| **认证** | Redis 存储签名 nonce | 防重放攻击，跨实例共享 |
| **限流** | Redis 滑动窗口 | 分布式限流，跨实例共享计数 |
| **服务发现** | Nacos | 自动注册与健康检查 |

### 2.3 WebSocket 扩展架构

```
┌─────────────────────────────────────────────────────────────┐
│                     WebSocket Connections                    │
│  User A ────► api-1     User B ────► api-2                  │
│  User C ────► api-1     User D ────► api-3                  │
└─────────────────────────────────────────────────────────────┘
                    │                    │
                    ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│                    Redis Pub/Sub                             │
│  Channel: eidos:ticker:{market}                              │
│  Channel: eidos:depth:{market}                               │
│  Channel: eidos:kline:{market}:{interval}                    │
│  Channel: eidos:trades:{market}                              │
│  Channel: eidos:orders:{wallet}  (私有频道)                  │
└─────────────────────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────────────────────┐
│                 eidos-market (数据源)                        │
│  - 计算并推送 Ticker/Depth/Kline/Trades                     │
└─────────────────────────────────────────────────────────────┘
```

### 2.4 部署建议

**开发环境:**
- 单实例部署
- 本地 Redis + Kafka

**测试环境:**
- 2-3 实例
- Redis Sentinel
- Kafka 3 节点

**生产环境:**
- 最少 3 实例 (建议 5+)
- Redis Cluster (6 节点)
- Kafka Cluster (3+ brokers)
- K8s HPA 自动伸缩

```yaml
# Kubernetes HPA 配置示例
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: eidos-api
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: eidos-api
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

## 三、服务对接 TODO 清单

### 3.1 需要对接的服务

| 服务 | 协议 | 端口 | 状态 | 说明 |
|------|------|------|------|------|
| eidos-trading | gRPC | 50051 | ✅ 已实现 | 订单、余额、充值、提现、成交 |
| eidos-market | gRPC | 50053 | ⏳ TODO | 行情数据 (Ticker/Depth/Kline) |
| eidos-risk | gRPC | 50055 | ⏳ TODO | 风控检查 |
| Redis | TCP | 6379 | ✅ 已实现 | 限流、重放保护、Session |
| Kafka | TCP | 9092 | ⏳ TODO | 事件订阅 |
| Nacos | HTTP | 8848 | ⏳ TODO | 服务发现与配置中心 |

### 3.2 eidos-trading 对接 (已完成)

**已实现接口:**
```protobuf
// 订单服务
rpc CreateOrder(CreateOrderRequest) returns (OrderResponse)
rpc CancelOrder(CancelOrderRequest) returns (OrderResponse)
rpc BatchCancelOrders(BatchCancelRequest) returns (BatchCancelResponse)
rpc GetOrder(GetOrderRequest) returns (OrderResponse)
rpc ListOrders(ListOrdersRequest) returns (ListOrdersResponse)
rpc ListOpenOrders(ListOpenOrdersRequest) returns (ListOpenOrdersResponse)
rpc PrepareOrder(PrepareOrderRequest) returns (PrepareOrderResponse)

// 余额服务
rpc GetBalance(GetBalanceRequest) returns (BalanceResponse)
rpc ListBalances(ListBalancesRequest) returns (ListBalancesResponse)
rpc ListTransactions(ListTransactionsRequest) returns (ListTransactionsResponse)

// 充值服务
rpc GetDeposit(GetDepositRequest) returns (DepositResponse)
rpc ListDeposits(ListDepositsRequest) returns (ListDepositsResponse)

// 提现服务
rpc CreateWithdrawal(CreateWithdrawalRequest) returns (WithdrawalResponse)
rpc CancelWithdrawal(CancelWithdrawalRequest) returns (WithdrawalResponse)
rpc GetWithdrawal(GetWithdrawalRequest) returns (WithdrawalResponse)
rpc ListWithdrawals(ListWithdrawalsRequest) returns (ListWithdrawalsResponse)

// 成交服务
rpc GetTrade(GetTradeRequest) returns (TradeResponse)
rpc ListTrades(ListTradesRequest) returns (ListTradesResponse)
```

**实现文件:**
- `internal/client/trading_client.go` - gRPC 客户端封装

### 3.3 eidos-market 对接 (TODO)

**需要实现的接口:**
```protobuf
service MarketService {
  // 交易对列表
  rpc ListMarkets(ListMarketsRequest) returns (ListMarketsResponse);

  // Ticker
  rpc GetTicker(GetTickerRequest) returns (TickerResponse);
  rpc ListTickers(ListTickersRequest) returns (ListTickersResponse);

  // 深度
  rpc GetDepth(GetDepthRequest) returns (DepthResponse);

  // K 线
  rpc GetKlines(GetKlinesRequest) returns (GetKlinesResponse);

  // 最近成交
  rpc GetRecentTrades(GetRecentTradesRequest) returns (GetRecentTradesResponse);
}
```

**TODO 实现步骤:**
1. 创建 `internal/client/market_client.go`
2. 实现 MarketServiceClient 接口
3. 更新 MarketHandler 使用真实客户端
4. 添加 Mock 模式支持

### 3.4 Kafka 订阅 (TODO)

**需要订阅的 Topic:**

| Topic | 消费组 | 说明 | 处理逻辑 |
|-------|--------|------|---------|
| `order-updates` | eidos-api | 订单状态更新 | 推送给订阅者 |
| `balance-updates` | eidos-api | 余额变动 | 推送给订阅者 |
| `settlements` | eidos-api | 结算完成 | 推送给订阅者 |

**实现文件:** `internal/kafka/consumer.go`

```go
// TODO: Kafka Consumer 实现
type KafkaConsumer struct {
    consumer sarama.ConsumerGroup
    hub      *ws.Hub
    logger   *zap.Logger
}

func (c *KafkaConsumer) Start(ctx context.Context) error {
    topics := []string{"order-updates", "balance-updates", "settlements"}

    for {
        select {
        case <-ctx.Done():
            return nil
        default:
            err := c.consumer.Consume(ctx, topics, c)
            if err != nil {
                c.logger.Error("kafka consume error", zap.Error(err))
            }
        }
    }
}

func (c *KafkaConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for msg := range claim.Messages() {
        switch msg.Topic {
        case "order-updates":
            c.handleOrderUpdate(msg)
        case "balance-updates":
            c.handleBalanceUpdate(msg)
        case "settlements":
            c.handleSettlement(msg)
        }
        session.MarkMessage(msg, "")
    }
    return nil
}
```

### 3.5 Redis Pub/Sub 订阅 (TODO)

**需要订阅的频道:**

| 频道模式 | 说明 | 数据源 |
|----------|------|--------|
| `eidos:ticker:*` | Ticker 更新 | eidos-market |
| `eidos:depth:*` | 深度更新 | eidos-market |
| `eidos:kline:*` | K 线更新 | eidos-market |
| `eidos:trades:*` | 成交更新 | eidos-market |

**实现文件:** `internal/ws/subscriber.go` (已有框架，需要完善)

```go
// TODO: 完善 Redis 订阅处理
func (s *Subscriber) handleMessage(payload []byte) {
    var msg struct {
        Channel Channel `json:"channel"`
        Market  string  `json:"market"`
        Data    interface{} `json:"data"`
    }

    if err := json.Unmarshal(payload, &msg); err != nil {
        s.logger.Error("unmarshal message failed", zap.Error(err))
        return
    }

    // 广播给订阅该频道的所有客户端
    s.hub.Broadcast(msg.Channel, msg.Market, ws.NewUpdateMessage(msg.Channel, msg.Market, msg.Data))
}
```

### 3.6 Nacos 服务发现 (TODO)

**配置示例:**
```yaml
nacos:
  server_addr: "127.0.0.1:8848"
  namespace: "eidos-dev"
  service:
    name: "eidos-api"
    group: "DEFAULT_GROUP"
    weight: 1.0
    metadata:
      version: "1.0.0"
      protocol: "http"
```

**TODO 实现步骤:**
1. 使用 eidos-common 的 nacos 包
2. 应用启动时注册服务
3. 使用 Nacos 获取 eidos-trading/eidos-market 地址
4. 监听服务变更事件

## 四、EIP-712 签名验证 (TODO)

**当前状态:** Mock 模式，跳过实际签名验证

**TODO 完善步骤:**
1. 引入 `go-ethereum/crypto` 包
2. 实现 `recoverPubkey()` 函数
3. 验证签名时间戳 (±5 分钟)
4. 检查 nonce 重放攻击

```go
// TODO: internal/middleware/auth.go 中完善
func recoverPubkey(typedData []byte, signature []byte) (common.Address, error) {
    // 1. 计算 EIP-712 hash
    hash := crypto.Keccak256(typedData)

    // 2. 从签名恢复公钥
    pubkey, err := crypto.SigToPub(hash, signature)
    if err != nil {
        return common.Address{}, err
    }

    // 3. 返回地址
    return crypto.PubkeyToAddress(*pubkey), nil
}
```

## 五、配置说明

### 5.1 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_PORT` | 8080 | HTTP 服务端口 |
| `TRADING_GRPC_ADDR` | localhost:50051 | Trading 服务地址 |
| `MARKET_GRPC_ADDR` | localhost:50053 | Market 服务地址 |
| `REDIS_ADDR` | localhost:6379 | Redis 地址 |
| `REDIS_PASSWORD` | - | Redis 密码 |
| `KAFKA_BROKERS` | localhost:9092 | Kafka brokers |
| `NACOS_ADDR` | localhost:8848 | Nacos 地址 |
| `LOG_LEVEL` | info | 日志级别 |
| `ENV` | dev | 环境 (dev/test/prod) |

### 5.2 配置文件示例

```yaml
# config/config.yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s

grpc:
  trading:
    addr: "localhost:50051"
    timeout: 5s
  market:
    addr: "localhost:50053"
    timeout: 5s

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
  pool_size: 100

kafka:
  brokers:
    - "localhost:9092"
  consumer_group: "eidos-api"

websocket:
  read_buffer_size: 1024
  write_buffer_size: 1024
  ping_interval: 30s
  pong_timeout: 60s
  max_message_size: 65536
  max_subscriptions: 50

ratelimit:
  ip:
    limit: 100
    window: 1m
  wallet:
    order: 10
    query: 60
    window: 1m

auth:
  mock_mode: true  # 开发模式跳过签名验证
  timestamp_tolerance: 5m
  replay_window: 10m

eip712:
  name: "EidosExchange"
  version: "1"
  chain_id: 31337
  verifying_contract: "0x0000000000000000000000000000000000000000"
```

## 六、监控指标

### 6.1 Prometheus 指标

```go
// TODO: 添加到 internal/metrics/metrics.go

// HTTP 请求指标
var (
    HTTPRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "eidos_api_http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    HTTPRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "eidos_api_http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
)

// WebSocket 指标
var (
    WSConnectionsTotal = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "eidos_api_ws_connections_total",
            Help: "Total WebSocket connections",
        },
    )

    WSSubscriptionsTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "eidos_api_ws_subscriptions_total",
            Help: "Total WebSocket subscriptions",
        },
        []string{"channel"},
    )
)

// gRPC 客户端指标
var (
    GRPCRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "eidos_api_grpc_requests_total",
            Help: "Total gRPC requests",
        },
        []string{"service", "method", "status"},
    )
)
```

### 6.2 健康检查

- `GET /health/live` - 存活探针
- `GET /health/ready` - 就绪探针 (检查 Redis、gRPC 连接)

## 七、错误码体系

| 范围 | 类型 | 说明 |
|------|------|------|
| 10000-10999 | 通用错误 | 签名、参数、认证 |
| 11000-11999 | 订单错误 | 订单创建、取消等 |
| 12000-12999 | 资产错误 | 余额、充值、提现 |
| 13000-13999 | 市场错误 | 行情数据 |
| 20000-20999 | 系统错误 | 内部错误、超时等 |

详见 `internal/dto/errors.go`
