# eidos-matching 服务对接说明

## 服务依赖关系

```
                    ┌─────────────────┐
                    │   eidos-api     │
                    │   (HTTP/WS)     │
                    └────────┬────────┘
                             │ gRPC (查询)
                             ▼
┌─────────────┐      ┌───────────────┐      ┌─────────────┐
│eidos-trading│─────▶│eidos-matching │◀─────│ eidos-risk  │
│   (gRPC)    │      │   (核心)       │      │   (gRPC)    │
└──────┬──────┘      └───────┬───────┘      └─────────────┘
       │                     │
       │    Kafka Topics     │
       │  ┌──────────────────┼──────────────────┐
       │  │                  │                  │
       ▼  ▼                  ▼                  ▼
┌──────────────┐    ┌───────────────┐   ┌─────────────┐
│    orders    │    │ trade-results │   │orderbook-   │
│cancel-request│    │order-cancelled│   │  updates    │
└──────────────┘    └───────────────┘   └─────────────┘
       │                  │                    │
       │                  ▼                    ▼
       │           ┌─────────────┐      ┌─────────────┐
       │           │eidos-trading│      │eidos-market │
       │           │  (清算)     │      │  (行情)     │
       │           └─────────────┘      └─────────────┘
       │                  │
       ▼                  ▼
┌─────────────┐    ┌─────────────┐
│ eidos-chain │    │eidos-trading│
│ (链上结算)   │    │  (账户)     │
└─────────────┘    └─────────────┘
```

## Kafka Topic 对接

### 订阅 (Input)

| Topic | 发送方 | 消息格式 | 说明 |
|-------|--------|----------|------|
| `orders` | eidos-trading | OrderMessage | 新订单 |
| `cancel-requests` | eidos-trading | CancelMessage | 取消请求 |

### 发布 (Output)

| Topic | 接收方 | 消息格式 | 说明 |
|-------|--------|----------|------|
| `trade-results` | eidos-trading, eidos-market | TradeResult | 成交结果 |
| `order-cancelled` | eidos-trading | CancelResult | 取消结果 |
| `orderbook-updates` | eidos-market, eidos-api | OrderBookUpdate | 订单簿增量更新 |

## 消息格式定义

### OrderMessage (订阅)

```json
{
  "order_id": "string",       // 订单ID (UUID)
  "wallet": "string",         // 钱包地址 (0x...)
  "market": "string",         // 市场 (BTC-USDC)
  "side": 1,                  // 1=买, 2=卖
  "order_type": 1,            // 1=限价, 2=市价
  "time_in_force": 1,         // 1=GTC, 2=IOC, 3=FOK
  "price": "50000.00",        // 价格 (decimal string)
  "amount": "1.5",            // 数量 (decimal string)
  "timestamp": 1704067200000, // 时间戳 (毫秒)
  "sequence": 12345           // 序列号 (用于幂等)
}
```

### CancelMessage (订阅)

```json
{
  "order_id": "string",       // 要取消的订单ID
  "wallet": "string",         // 钱包地址
  "market": "string",         // 市场
  "timestamp": 1704067200000, // 时间戳
  "sequence": 12346           // 序列号
}
```

### TradeResult (发布)

```json
{
  "trade_id": "string",       // 成交ID
  "market": "string",         // 市场
  "maker_order_id": "string", // Maker 订单ID
  "taker_order_id": "string", // Taker 订单ID
  "maker_wallet": "string",   // Maker 钱包
  "taker_wallet": "string",   // Taker 钱包
  "side": 1,                  // Taker 方向
  "price": "50000.00",        // 成交价
  "amount": "1.5",            // 成交量
  "maker_fee": "0.0015",      // Maker 手续费
  "taker_fee": "0.003",       // Taker 手续费
  "timestamp": 1704067200000, // 成交时间
  "sequence": 1               // 成交序列号
}
```

### CancelResult (发布)

```json
{
  "order_id": "string",       // 订单ID
  "market": "string",         // 市场
  "success": true,            // 是否成功
  "reason": "string",         // 失败原因 (可选)
  "remaining_size": "0.5",    // 剩余数量
  "timestamp": 1704067200000  // 处理时间
}
```

### OrderBookUpdate (发布)

```json
{
  "market": "string",         // 市场
  "type": "ADD",              // ADD/UPDATE/REMOVE
  "side": 1,                  // 1=买, 2=卖
  "price": "50000.00",        // 价格
  "amount": "1.5",            // 变更后数量 (REMOVE 时为 0)
  "timestamp": 1704067200000, // 更新时间
  "sequence": 100             // 更新序列号
}
```

## gRPC 服务对接

### 需要调用的服务

| 服务 | 接口 | 用途 | 调用时机 |
|------|------|------|----------|
| eidos-risk | CheckPreTrade | 交易前风控检查 | 收到订单时 (可选) |

### 提供的服务

| 接口 | 调用方 | 用途 |
|------|--------|------|
| GetDepth | eidos-api, eidos-market | 获取订单簿深度 |
| GetOrder | eidos-trading | 查询订单状态 |
| GetStats | eidos-admin | 获取引擎统计 |

## Redis 依赖

### Key 结构

| Key Pattern | 用途 | 过期时间 |
|-------------|------|----------|
| `snapshot:{market}:latest` | 最新快照 | 永久 |
| `snapshot:{market}:preparing` | 预写快照 | 1小时 |
| `snapshot:{market}:{timestamp}` | 历史快照 | 24小时 |
| `snapshot:{market}:history` | 历史快照索引 (ZSet) | 永久 |

### 数据格式

快照数据为 JSON 格式，包含:
- 订单簿状态 (买卖订单)
- Kafka 偏移量
- 最新价和序列号
- 校验和

## TODO: 待实现的对接点

### 1. 风控服务对接 (eidos-risk)

```go
// internal/app/app.go
// TODO: 对接 eidos-risk 服务进行交易前检查
// 位置: handleOrder 函数开头
//
// riskClient.CheckPreTrade(ctx, &risk.PreTradeRequest{
//     OrderID:   order.OrderID,
//     Wallet:    order.Wallet,
//     Market:    order.Market,
//     Side:      order.Side,
//     Price:     order.Price,
//     Amount:    order.Amount,
// })
```

### 2. 日志系统对接

```go
// internal/app/app.go, internal/kafka/consumer.go
// TODO: 对接统一日志系统 (如 eidos-common/logger)
//
// logger.Error("process order failed",
//     zap.String("order_id", order.OrderID),
//     zap.Error(err))
```

### 3. 监控指标上报

```go
// internal/engine/engine.go
// TODO: 对接 Prometheus 指标上报
//
// metrics.OrdersProcessed.Inc()
// metrics.TradesGenerated.Add(float64(len(trades)))
// metrics.MatchLatency.Observe(latencyUs)
```

### 4. 服务注册发现 (Nacos)

```go
// internal/app/app.go
// TODO: 注册服务到 Nacos
//
// nacos.RegisterService(&nacos.ServiceInstance{
//     ServiceName: "eidos-matching",
//     IP:          localIP,
//     Port:        grpcPort,
//     Metadata: map[string]string{
//         "markets": "BTC-USDC,ETH-USDC",
//     },
// })
```

### 5. gRPC 服务暴露

```go
// internal/handler/grpc.go (待创建)
// TODO: 实现 gRPC 服务接口
//
// service MatchingService {
//     rpc GetDepth(GetDepthRequest) returns (GetDepthResponse);
//     rpc GetOrder(GetOrderRequest) returns (GetOrderResponse);
//     rpc GetStats(GetStatsRequest) returns (GetStatsResponse);
// }
```

### 6. 死信队列处理

```go
// internal/kafka/consumer.go
// TODO: 解析失败的消息发送到死信队列
//
// if err := producer.SendToDLQ(ctx, &DLQMessage{
//     OriginalTopic: topic,
//     OriginalKey:   key,
//     OriginalValue: value,
//     Error:         err.Error(),
//     Timestamp:     time.Now(),
// }); err != nil {
//     logger.Error("send to DLQ failed", zap.Error(err))
// }
```

### 7. 订单簿恢复完善

```go
// internal/engine/engine.go
// TODO: 添加 SetOrderBook 方法用于快照恢复
//
// func (e *Engine) SetOrderBook(ob *orderbook.OrderBook) {
//     e.orderBook = ob
//     e.matcher = NewMatcher(e.config.Market, ob, e.config.TradeIDGen)
// }
```

### 8. 健康检查接口

```go
// internal/handler/health.go (待创建)
// TODO: 实现健康检查接口供 K8s 探针使用
//
// func (h *HealthHandler) Liveness(ctx context.Context) error {
//     return nil // 进程存活即可
// }
//
// func (h *HealthHandler) Readiness(ctx context.Context) error {
//     // 检查 Kafka 连接
//     // 检查 Redis 连接
//     // 检查引擎是否运行
//     return nil
// }
```

## 环境变量

| 变量 | 说明 | 示例 |
|------|------|------|
| `CONFIG_PATH` | 配置文件路径 | `/etc/eidos/config.yaml` |
| `KAFKA_BROKERS` | Kafka 地址 | `kafka-1:9092,kafka-2:9092` |
| `REDIS_ADDRS` | Redis 地址 | `redis-1:6379,redis-2:6379` |
| `REDIS_PASSWORD` | Redis 密码 | `${REDIS_PASSWORD}` |
| `NACOS_ADDR` | Nacos 地址 | `nacos:8848` |
| `LOG_LEVEL` | 日志级别 | `info` |
