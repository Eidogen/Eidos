# eidos-market 服务集成文档

本文档记录 eidos-market 行情服务与其他服务的集成依赖项，供各服务开发时参考。

---

## 一、概述

eidos-market 是行情数据服务，负责：
- K 线聚合（1m, 5m, 15m, 30m, 1h, 4h, 1d, 1w）
- Ticker 计算（24h 滚动窗口统计）
- 订单簿深度管理
- 成交流处理与分发

**数据流向：**
```
┌─────────────────┐     Kafka      ┌─────────────────┐
│  eidos-matching │ ─────────────▶ │  eidos-market   │
│   (撮合引擎)    │  trade-results │   (行情服务)    │
│                 │  orderbook-    │                 │
└─────────────────┘    updates     └────────┬────────┘
                                            │
                       ┌────────────────────┼────────────────────┐
                       │                    │                    │
                       ▼                    ▼                    ▼
              ┌─────────────┐      ┌─────────────┐      ┌─────────────┐
              │   gRPC API  │      │ Redis Cache │      │ Redis PubSub│
              │ (eidos-api) │      │  (查询缓存) │      │ (实时推送)  │
              └─────────────┘      └─────────────┘      └─────────────┘
                                                               │
                                                               ▼
                                                      ┌─────────────┐
                                                      │  eidos-api  │
                                                      │ (WebSocket) │
                                                      └─────────────┘
```

---

## 二、上游依赖 (eidos-market 消费)

### 2.1 eidos-matching - Kafka: trade-results

**状态**: 🔴 待实现

**Topic**: `trade-results`

**说明**: eidos-matching 撮合成功后，需要将成交结果发送到 Kafka，供 eidos-market 消费并更新 K 线、Ticker。

**消息格式**:
```json
{
  "trade_id": "string",
  "market": "BTC-USDC",
  "maker_order_id": "string",
  "taker_order_id": "string",
  "price": "50000.00",
  "amount": "1.5",
  "quote_amount": "75000.00",
  "side": 0,
  "timestamp": 1700000000000
}
```

**字段说明**:
| 字段 | 类型 | 说明 |
|------|------|------|
| trade_id | string | 成交 ID（唯一） |
| market | string | 交易对，如 "BTC-USDC" |
| maker_order_id | string | Maker 订单 ID |
| taker_order_id | string | Taker 订单 ID |
| price | string | 成交价格（decimal string） |
| amount | string | 成交数量（Base Token） |
| quote_amount | string | 成交金额（Quote Token） |
| side | int | Taker 方向：0=买, 1=卖 |
| timestamp | int64 | 成交时间（毫秒时间戳） |

**eidos-matching 实现要点**:
1. 撮合成功后立即发送消息（低延迟）
2. 保证消息顺序（同一市场的成交按时间顺序）
3. 使用 market 作为 Kafka partition key
4. 建议批量发送（每 10ms 或累积 100 条）

---

### 2.2 eidos-matching - Kafka: orderbook-updates

**状态**: 🔴 待实现

**Topic**: `orderbook-updates`

**说明**: eidos-matching 订单簿变更后，需要发送增量更新到 Kafka，供 eidos-market 维护深度快照。

**消息格式**:
```json
{
  "market": "BTC-USDC",
  "bids": [
    {"price": "49900.00", "amount": "10.5"},
    {"price": "49800.00", "amount": "0"}
  ],
  "asks": [
    {"price": "50100.00", "amount": "5.2"}
  ],
  "sequence": 12345,
  "timestamp": 1700000000000
}
```

**字段说明**:
| 字段 | 类型 | 说明 |
|------|------|------|
| market | string | 交易对 |
| bids | array | 买单变更列表 |
| asks | array | 卖单变更列表 |
| bids[].price | string | 价格档位 |
| bids[].amount | string | 新数量（0 表示删除该档位） |
| sequence | uint64 | 序列号（递增） |
| timestamp | int64 | 时间戳（毫秒） |

**eidos-matching 实现要点**:
1. 每次订单簿变更后发送增量更新
2. sequence 必须严格递增（用于检测消息丢失）
3. amount = "0" 表示删除该档位
4. 使用 market 作为 Kafka partition key
5. 建议合并同一价格档位的多次更新（防止消息膨胀）

**重要**: eidos-market 检测到 sequence 缺口时，会请求全量快照（见 2.3）。

---

### 2.3 eidos-matching - gRPC: GetOrderbook

**状态**: ✅ 已实现

**说明**: 当 eidos-market 检测到 sequence 缺口或服务重启时，需要从 eidos-matching 获取订单簿全量快照。

**Proto 定义** (proto/matching/v1/matching.proto):
```protobuf
service MatchingService {
  // 获取订单簿快照
  rpc GetOrderbook(GetOrderbookRequest) returns (GetOrderbookResponse);
}

message GetOrderbookRequest {
  string market = 1;
  int32 limit = 2;  // 每边数量限制，默认 100
}

message GetOrderbookResponse {
  string market = 1;
  repeated PriceLevel bids = 2;
  repeated PriceLevel asks = 3;
  int64 timestamp = 4;
  uint64 sequence = 5;
}

message PriceLevel {
  string price = 1;
  string amount = 2;
  int32 order_count = 3;
}
```

**eidos-matching 实现**: `eidos-matching/internal/handler/grpc_handler.go:GetOrderbook`

**eidos-market 客户端**: `eidos-market/internal/client/matching_client.go`
- 实现 `aggregator.DepthSnapshotProvider` 接口
- 通过 `GetSnapshot()` 方法调用 eidos-matching 的 `GetOrderbook` gRPC 接口

**配置** (config.yaml 或环境变量):
```yaml
matching:
  enabled: true           # MATCHING_ENABLED
  addr: "eidos-matching:50052"  # MATCHING_ADDR
  connect_timeout: 5      # MATCHING_CONNECT_TIMEOUT (秒)
  request_timeout: 3      # MATCHING_REQUEST_TIMEOUT (秒)
```

**eidos-market 调用时机**:
1. 检测到 sequence 缺口时（自动触发）
2. 可用于服务启动时的初始化同步

---

### 2.4 eidos-admin - 交易对配置同步

**状态**: 🟡 可选

**说明**: eidos-market 需要获取交易对配置（精度、最小下单量等）。可通过以下方式之一：

**方式一：共享数据库表**
- eidos-market 直接读取 `eidos_market_markets` 表
- eidos-admin 负责写入和维护

**方式二：Kafka 事件**
- Topic: `market-config-updates`
- eidos-admin 修改配置后发送事件

**方式三：gRPC 接口**
- eidos-admin 提供 `ListMarkets` gRPC 接口
- eidos-market 启动时调用

**当前实现**: eidos-market 使用本地数据库表，假设由 eidos-admin 或手动管理。

---

## 三、下游消费 (其他服务消费 eidos-market)

### 3.1 eidos-api - gRPC 查询接口

**状态**: 🟢 已实现

**说明**: eidos-api 通过 gRPC 调用 eidos-market 获取行情数据。

**服务定义**:
```protobuf
service MarketService {
  // 获取交易对列表
  rpc ListMarkets(ListMarketsRequest) returns (ListMarketsResponse);

  // 获取单个 Ticker
  rpc GetTicker(GetTickerRequest) returns (GetTickerResponse);

  // 获取所有 Ticker
  rpc ListTickers(ListTickersRequest) returns (ListTickersResponse);

  // 获取 K 线数据
  rpc GetKlines(GetKlinesRequest) returns (GetKlinesResponse);

  // 获取最近成交
  rpc GetRecentTrades(GetRecentTradesRequest) returns (GetRecentTradesResponse);

  // 获取订单簿深度
  rpc GetDepth(GetDepthRequest) returns (GetDepthResponse);
}
```

**eidos-api 实现要点**:
1. 通过 Nacos 发现 eidos-market 服务
2. 使用连接池和负载均衡
3. 设置合理超时（建议 500ms）
4. 实现熔断和重试

---

### 3.2 eidos-api - Redis Pub/Sub 订阅

**状态**: 🔴 待实现

**说明**: eidos-api 订阅 Redis Pub/Sub 频道，接收实时行情推送，转发给 WebSocket 客户端。

**频道列表**:
| 频道模式 | 说明 | 消息频率 |
|----------|------|----------|
| `eidos:ticker:{market}` | Ticker 更新 | ~1s |
| `eidos:depth:{market}` | 深度更新 | ~100ms |
| `eidos:trades:{market}` | 成交流 | 每笔成交 |
| `eidos:kline:{market}:{interval}` | K 线更新 | ~1s |

**消息格式**:

**Ticker 消息**:
```json
{
  "market": "BTC-USDC",
  "last_price": "50000.00",
  "price_change": "500.00",
  "price_change_percent": "1.01",
  "open": "49500.00",
  "high": "51000.00",
  "low": "49000.00",
  "volume": "1234.56",
  "quote_volume": "61728000.00",
  "best_bid": "49900.00",
  "best_bid_qty": "10.5",
  "best_ask": "50100.00",
  "best_ask_qty": "5.2",
  "trade_count": 5678,
  "timestamp": 1700000000000
}
```

**深度消息**:
```json
{
  "market": "BTC-USDC",
  "bids": [
    ["49900.00", "10.5"],
    ["49800.00", "20.0"]
  ],
  "asks": [
    ["50100.00", "5.2"],
    ["50200.00", "15.0"]
  ],
  "sequence": 12345,
  "timestamp": 1700000000000
}
```

**成交消息**:
```json
{
  "trade_id": "abc123",
  "market": "BTC-USDC",
  "price": "50000.00",
  "amount": "1.5",
  "side": 0,
  "timestamp": 1700000000000
}
```

**K 线消息**:
```json
{
  "market": "BTC-USDC",
  "interval": "1m",
  "open_time": 1700000000000,
  "open": "50000.00",
  "high": "50100.00",
  "low": "49900.00",
  "close": "50050.00",
  "volume": "123.45",
  "quote_volume": "6172500.00",
  "trade_count": 100,
  "close_time": 1700000059999
}
```

**eidos-api 实现要点**:
1. 使用 PSUBSCRIBE 订阅模式 `eidos:*`
2. 根据 WebSocket 客户端订阅情况过滤消息
3. 实现消息聚合（防止推送过快）
4. 处理重连和消息恢复

---

### 3.3 eidos-api - Redis 缓存读取

**状态**: 🟢 已实现

**说明**: eidos-api 可直接读取 Redis 缓存获取最新数据（可选优化）。

**缓存键**:
| 键模式 | 说明 | TTL |
|--------|------|-----|
| `eidos:ticker:{market}` | Ticker 数据 | 10s |
| `eidos:orderbook:{market}` | 深度数据 | 5s |
| `eidos:trades:{market}` | 最近成交（List） | 无（LTRIM 保留 100 条） |
| `eidos:kline:{market}:{interval}` | 当前 K 线 | 60s |

**使用建议**:
- 优先读取 Redis 缓存，缓存未命中时调用 gRPC
- 或直接调用 gRPC（eidos-market 内部有缓存）

---

## 四、基础设施依赖

### 4.1 Kafka

**Topic 配置**:
| Topic | Partitions | Replication | Retention |
|-------|------------|-------------|-----------|
| trade-results | 8 | 3 | 7d |
| orderbook-updates | 8 | 3 | 1d |

**Consumer Group**: `eidos-market-group`

### 4.2 Redis

**配置要求**:
- Redis 版本 >= 6.0（支持 Streams）
- 建议使用 Redis Cluster 或 Sentinel

### 4.3 PostgreSQL + TimescaleDB

**表列表**:
- `eidos_market_klines` - K 线数据（TimescaleDB 超表）
- `eidos_market_markets` - 交易对配置
- `eidos_market_trades` - 成交记录

**TimescaleDB 要求**:
- TimescaleDB 版本 >= 2.0
- 启用超表压缩（可选）

---

## 五、集成检查清单

### eidos-matching 开发清单

- [ ] 实现 Kafka producer 发送 `trade-results`
- [ ] 实现 Kafka producer 发送 `orderbook-updates`
- [x] 实现 `GetOrderbook` gRPC 接口（已实现，包含 sequence）
- [ ] 保证 sequence 严格递增
- [ ] 使用 market 作为 partition key

### eidos-api 开发清单

- [ ] 集成 eidos-market gRPC client
- [ ] 订阅 Redis Pub/Sub 频道
- [ ] 实现 WebSocket 行情推送
- [ ] 实现 REST 行情查询接口

### eidos-admin 开发清单

- [ ] 交易对配置管理界面
- [ ] 同步交易对配置到 eidos-market 数据库

---

## 六、接口版本与兼容性

- **gRPC 版本**: `eidos.market.v1`
- **向后兼容**: 新增字段使用 optional，不删除现有字段
- **Proto 文件位置**: `proto/market/v1/market_service.proto`

---

## 七、监控与告警

### eidos-market 暴露的指标

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `eidos_market_trades_processed_total` | Counter | 处理成交数 |
| `eidos_market_depth_updates_total` | Counter | 深度更新数 |
| `eidos_market_kline_flushes_total` | Counter | K 线刷盘数 |
| `eidos_market_sequence_gaps_total` | Counter | 序列号缺口数 |
| `eidos_market_grpc_requests_total` | Counter | gRPC 请求数 |
| `eidos_market_grpc_latency_seconds` | Histogram | gRPC 延迟 |

### 建议告警规则

```yaml
# Prometheus 告警规则示例
groups:
  - name: eidos-market
    rules:
      - alert: HighSequenceGaps
        expr: rate(eidos_market_sequence_gaps_total[5m]) > 1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "序列号缺口过多，可能存在消息丢失"

      - alert: KafkaConsumerLag
        expr: kafka_consumer_lag{group="eidos-market-group"} > 10000
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Kafka 消费延迟过大"
```

---

## 八、联系方式

如有集成问题，请联系：
- **eidos-market 负责人**: [待填写]
- **技术讨论群**: [待填写]

---

---

## 九、水平扩展方案

### 9.1 扩展架构

eidos-market 支持水平扩展，多实例部署时采用 **按市场分片** 策略：

```
                    ┌──────────────────────────────────────────┐
                    │             Kafka Consumer Group          │
                    │           (eidos-market-group)            │
                    └─────────────────────┬────────────────────┘
                                          │
            ┌─────────────────────────────┼─────────────────────────────┐
            │                             │                             │
            ▼                             ▼                             ▼
   ┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
   │ eidos-market-1  │         │ eidos-market-2  │         │ eidos-market-3  │
   │   Partition 0   │         │   Partition 1   │         │   Partition 2   │
   │   BTC-*, SOL-*  │         │   ETH-*, UNI-*  │         │   MATIC-*, ...  │
   └─────────────────┘         └─────────────────┘         └─────────────────┘
            │                             │                             │
            └─────────────────────────────┼─────────────────────────────┘
                                          │
                                          ▼
                              ┌─────────────────────┐
                              │   Shared Resources  │
                              │  - Redis (缓存/PubSub)
                              │  - PostgreSQL (K线)
                              │  - Nacos (服务发现)
                              └─────────────────────┘
```

### 9.2 分片策略

**Kafka Partition Key**: 使用 `market` 作为 partition key，确保：
- 同一市场的所有消息由同一实例处理
- 保证消息顺序（同一市场内）
- 负载自动均衡

**分区数量建议**:
| 市场数量 | 建议分区数 | 实例数 |
|----------|-----------|--------|
| < 10     | 4         | 2-4    |
| 10-50    | 8         | 4-8    |
| 50-100   | 16        | 8-16   |
| > 100    | 32        | 16+    |

### 9.3 配置示例

```yaml
# config/config.yaml (水平扩展配置)
kafka:
  consumer:
    group_id: "eidos-market-group"
    topics:
      - "trade-results"
      - "orderbook-updates"
    # 每个实例自动分配 partition
    auto_offset_reset: "latest"
    enable_auto_commit: true
    max_poll_records: 500

# 实例标识（可选，用于调试）
instance:
  id: "${HOSTNAME}-${RANDOM}"
```

### 9.4 扩展注意事项

1. **状态分片**: 每个实例只维护分配给它的市场状态
2. **Redis 缓存**: 所有实例共享 Redis，按 market 键隔离
3. **gRPC 路由**:
   - 方式一：客户端通过 Nacos 发现所有实例，随机选择（推荐）
   - 方式二：部署 gRPC 负载均衡器（如 Envoy）
4. **Rebalance 处理**: Kafka rebalance 时会触发状态重建
5. **监控**: 确保每个实例有独立的 metrics endpoint

### 9.5 扩缩容流程

**扩容**:
1. 启动新实例，自动注册到 Nacos
2. Kafka Consumer Group 触发 rebalance
3. 新实例接管部分 partition
4. 新实例从 Kafka 最新 offset 开始消费
5. 调用 eidos-matching 获取订单簿快照

**缩容**:
1. 标记实例为下线（graceful shutdown）
2. 等待当前消息处理完成
3. Kafka Consumer Group 触发 rebalance
4. 其他实例接管 partition
5. 停止实例

---

## 十、代码中的 TODO 标记

以下是 eidos-market 代码中的待对接项，搜索 `TODO:` 可找到具体位置：

### 10.1 Kafka 消费者 (internal/kafka/consumer.go)

```go
// TODO: 对接 eidos-matching 的 Kafka producer
// - Topic: trade-results (成交数据)
// - Topic: orderbook-updates (订单簿增量)
// 参见 INTEGRATION.md 第二节
```

### 10.2 深度快照同步 (internal/client/matching_client.go)

**状态**: ✅ 已实现

```go
// MatchingClient 实现了 aggregator.DepthSnapshotProvider 接口
// - GetSnapshot() 调用 eidos-matching 的 GetOrderbook gRPC 接口
// - 在检测到 sequence 缺口时自动触发
// - 配置: matching.enabled=true, matching.addr="eidos-matching:50052"
```

### 10.3 Redis Pub/Sub (internal/cache/pubsub.go)

```go
// TODO: 对接 eidos-api 的 WebSocket 服务
// - eidos-api 需订阅 Redis 频道
// - 频道格式: eidos:{type}:{market}
// 参见 INTEGRATION.md 3.2 节
```

### 10.4 交易对配置 (internal/repository/market_repository.go)

```go
// TODO: 对接 eidos-admin 的交易对管理
// - 可选：Kafka 事件同步
// - 可选：gRPC 接口查询
// - 当前：共享数据库表
// 参见 INTEGRATION.md 2.4 节
```

---

*文档版本: 1.1.0*
*最后更新: 2026-01-16*
