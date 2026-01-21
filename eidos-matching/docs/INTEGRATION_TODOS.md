# eidos-matching 集成待办事项

本文档记录 eidos-matching 服务与其他服务对接所需的待办事项。

## 1. eidos-trading 服务对接

### 1.1 Kafka 消息生产

eidos-trading 需要向以下 Kafka Topic 发送消息：

| Topic | 消息格式 | 说明 |
|-------|---------|------|
| `orders` | `OrderMessage` | 新订单提交 |
| `cancel-requests` | `CancelMessage` | 取消订单请求 |

**OrderMessage 格式：**
```json
{
  "order_id": "uuid",
  "wallet": "0x...",
  "market": "BTC-USDC",
  "side": "buy|sell",
  "order_type": "limit|market",
  "time_in_force": "GTC|IOC|FOK",
  "price": "50000.00",
  "amount": "1.5",
  "timestamp": 1704067200000,
  "sequence": 12345
}
```

**CancelMessage 格式：**
```json
{
  "order_id": "uuid",
  "wallet": "0x...",
  "market": "BTC-USDC",
  "timestamp": 1704067200000,
  "sequence": 12346
}
```

### 1.2 Kafka 消息消费

eidos-trading 需要消费以下 Kafka Topic：

| Topic | 消息格式 | 说明 |
|-------|---------|------|
| `trade-results` | `TradeResult` | 成交结果 |
| `order-cancelled` | `CancelResult` | 取消结果 |

**TODO [eidos-trading]:**
- [ ] 实现 Kafka Producer 发送 `orders` 消息
- [ ] 实现 Kafka Producer 发送 `cancel-requests` 消息
- [ ] 实现 Kafka Consumer 消费 `trade-results` 更新订单状态
- [ ] 实现 Kafka Consumer 消费 `order-cancelled` 更新取消状态
- [ ] 实现全局序列号生成器（用于幂等性保证）

### 1.3 序列号设计

撮合引擎使用序列号实现幂等性：
- 每个市场独立维护 inputSequence
- 序列号必须严格递增
- 重复或过期的序列号会被跳过

**建议实现：**
```go
// eidos-trading 应实现全局序列号服务
type SequenceService interface {
    NextSequence(market string) (int64, error)
}
```

---

## 2. eidos-market 服务对接

### 2.1 Kafka 消息消费

eidos-market 需要消费以下 Kafka Topic：

| Topic | 消息格式 | 说明 |
|-------|---------|------|
| `trade-results` | `TradeResult` | 用于更新 K 线、Ticker |
| `orderbook-updates` | `OrderBookUpdate` | 用于推送深度数据 |

**TradeResult 格式：**
```json
{
  "trade_id": "trade-uuid",
  "market": "BTC-USDC",
  "maker_order_id": "uuid",
  "taker_order_id": "uuid",
  "maker_wallet": "0x...",
  "taker_wallet": "0x...",
  "side": "buy",
  "price": "50000.00",
  "amount": "1.5",
  "maker_fee": "0.75",
  "taker_fee": "1.50",
  "timestamp": 1704067200000,
  "sequence": 100
}
```

**OrderBookUpdate 格式：**
```json
{
  "market": "BTC-USDC",
  "type": "add|update|remove",
  "side": "buy|sell",
  "price": "50000.00",
  "amount": "1.5",
  "order_count": 3,
  "timestamp": 1704067200000,
  "sequence": 101
}
```

**TODO [eidos-market]:**
- [ ] 实现 Kafka Consumer 消费 `trade-results`
- [ ] 基于成交数据计算 K 线（1m, 5m, 15m, 1h, 4h, 1d）
- [ ] 基于成交数据更新 24h Ticker
- [ ] 实现 Kafka Consumer 消费 `orderbook-updates`
- [ ] 维护实时深度数据缓存
- [ ] 通过 Redis Pub/Sub 推送实时数据到 eidos-api

---

## 3. eidos-api 服务对接

### 3.1 WebSocket 推送

eidos-api 需要订阅 Redis Pub/Sub 并推送给客户端：

| Redis Channel | 数据类型 | 说明 |
|---------------|---------|------|
| `depth:{market}` | OrderBookSnapshot | 深度快照 |
| `trade:{market}` | TradeResult | 最新成交 |
| `ticker:{market}` | Ticker | 24h 行情 |

**TODO [eidos-api]:**
- [ ] 实现 Redis Pub/Sub 订阅
- [ ] WebSocket 频道订阅管理
- [ ] 深度增量推送优化（差分更新）
- [ ] 实现订单簿本地快照（减少全量推送）

### 3.2 REST API

**TODO [eidos-api]:**
- [ ] GET /api/v1/depth/:market - 获取深度（从 eidos-market 或缓存）
- [ ] GET /api/v1/trades/:market - 获取最近成交
- [ ] POST /api/v1/orders - 提交订单（发送到 Kafka）
- [ ] DELETE /api/v1/orders/:id - 取消订单（发送到 Kafka）

---

## 4. eidos-chain 服务对接

### 4.1 链上结算

撮合引擎产生的成交需要通过 eidos-chain 进行链上结算。

**TODO [eidos-chain]:**
- [ ] 实现 Kafka Consumer 消费 `trade-results`
- [ ] 批量打包成交进行链上结算
- [ ] 实现结算状态机（MATCHED_OFFCHAIN → SETTLEMENT_PENDING → SETTLED_ONCHAIN）
- [ ] 发送结算结果到 `settlements` Topic

### 4.2 结算确认

**TODO [eidos-trading]:**
- [ ] 消费 `settlements` Topic 更新订单最终状态
- [ ] 实现结算失败回滚逻辑

---

## 5. eidos-risk 服务对接

### 5.1 风控检查

订单在进入撮合前需要通过风控检查。

**TODO [eidos-risk]:**
- [ ] 实现 gRPC 接口 `CheckOrder(order) -> (bool, reason)`
- [ ] 检查项目：
  - [ ] 余额充足性检查
  - [ ] 订单频率限制
  - [ ] 价格偏离检查（与市场价偏离过大）
  - [ ] 单笔数量限制
  - [ ] 持仓限制

**TODO [eidos-trading]:**
- [ ] 在发送订单到 Kafka 前调用 eidos-risk 进行检查
- [ ] 风控拒绝的订单直接返回错误，不进入撮合

---

## 6. 数据库迁移

### 6.1 eidos-trading 数据库

需要创建以下表来存储订单和成交记录：

**迁移文件：** `20240101_create_orders_table.up.sql`
```sql
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(36) NOT NULL UNIQUE,
    wallet VARCHAR(42) NOT NULL,
    market VARCHAR(20) NOT NULL,
    side SMALLINT NOT NULL,
    order_type SMALLINT NOT NULL,
    time_in_force SMALLINT NOT NULL,
    price DECIMAL(36, 18) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,
    filled_amount DECIMAL(36, 18) NOT NULL DEFAULT 0,
    status SMALLINT NOT NULL DEFAULT 0,
    sequence BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,

    INDEX idx_orders_wallet (wallet),
    INDEX idx_orders_market (market),
    INDEX idx_orders_status (status),
    INDEX idx_orders_created_at (created_at)
);
```

**迁移文件：** `20240101_create_trades_table.up.sql`
```sql
CREATE TABLE trades (
    id BIGSERIAL PRIMARY KEY,
    trade_id VARCHAR(36) NOT NULL UNIQUE,
    market VARCHAR(20) NOT NULL,
    maker_order_id VARCHAR(36) NOT NULL,
    taker_order_id VARCHAR(36) NOT NULL,
    maker_wallet VARCHAR(42) NOT NULL,
    taker_wallet VARCHAR(42) NOT NULL,
    side SMALLINT NOT NULL,
    price DECIMAL(36, 18) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,
    maker_fee DECIMAL(36, 18) NOT NULL,
    taker_fee DECIMAL(36, 18) NOT NULL,
    sequence BIGINT NOT NULL,
    settlement_status SMALLINT NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL,

    INDEX idx_trades_market (market),
    INDEX idx_trades_maker_order_id (maker_order_id),
    INDEX idx_trades_taker_order_id (taker_order_id),
    INDEX idx_trades_created_at (created_at)
);
```

**TODO [eidos-trading]:**
- [ ] 创建数据库迁移文件
- [ ] 实现 Order Repository
- [ ] 实现 Trade Repository

### 6.2 eidos-market 数据库

**迁移文件：** `20240101_create_klines_table.up.sql`
```sql
CREATE TABLE klines (
    id BIGSERIAL PRIMARY KEY,
    market VARCHAR(20) NOT NULL,
    interval VARCHAR(5) NOT NULL,
    open_time BIGINT NOT NULL,
    open DECIMAL(36, 18) NOT NULL,
    high DECIMAL(36, 18) NOT NULL,
    low DECIMAL(36, 18) NOT NULL,
    close DECIMAL(36, 18) NOT NULL,
    volume DECIMAL(36, 18) NOT NULL,
    quote_volume DECIMAL(36, 18) NOT NULL,
    trade_count INT NOT NULL,

    UNIQUE (market, interval, open_time),
    INDEX idx_klines_market_interval (market, interval)
);
```

**TODO [eidos-market]:**
- [ ] 创建 K 线数据库迁移
- [ ] 实现 KLine Repository
- [ ] 实现 TimescaleDB 时序优化

---

## 7. 配置同步

### 7.1 市场配置

撮合引擎需要的市场配置应该与其他服务保持一致。

**建议：** 通过 Nacos 配置中心统一管理市场配置。

**配置项：**
```yaml
markets:
  - symbol: BTC-USDC
    base_token: BTC
    quote_token: USDC
    price_decimals: 2
    size_decimals: 4
    min_size: "0.0001"
    tick_size: "0.01"
    maker_fee_rate: "0.001"
    taker_fee_rate: "0.002"
    max_slippage: "0.05"
```

**TODO [所有服务]:**
- [ ] 从 Nacos 读取市场配置
- [ ] 实现配置热更新（新增市场无需重启）

---

## 8. 监控与告警

### 8.1 Prometheus 指标

eidos-matching 暴露以下指标：

| 指标名 | 类型 | 说明 |
|-------|------|------|
| `matching_orders_processed_total` | Counter | 处理的订单总数 |
| `matching_trades_generated_total` | Counter | 生成的成交总数 |
| `matching_order_latency_us` | Histogram | 订单处理延迟（微秒） |
| `matching_orderbook_depth` | Gauge | 订单簿深度 |
| `matching_orderbook_spread` | Gauge | 买卖价差 |

**TODO [运维]:**
- [ ] 配置 Prometheus 抓取 eidos-matching 指标
- [ ] 配置 Grafana 仪表盘
- [ ] 设置告警规则：
  - [ ] 订单处理延迟 > 1ms
  - [ ] Kafka 消费延迟 > 100条
  - [ ] 快照保存失败

---

## 9. 测试环境

### 9.1 集成测试环境

**TODO [测试]:**
- [ ] 搭建独立的 Kafka 集群（或使用 Testcontainers）
- [ ] 搭建独立的 Redis 实例
- [ ] 编写端到端集成测试：
  - [ ] 订单提交 → 撮合 → 成交通知
  - [ ] 取消订单流程
  - [ ] 快照恢复测试
  - [ ] 故障恢复测试

---

## 10. 待优化项

### 10.1 eidos-matching 内部 TODO

以下是 eidos-matching 代码中标记的 TODO 项：

| 文件 | 位置 | 说明 | 状态 |
|-----|------|------|------|
| `internal/kafka/consumer.go` | L148 | 记录错误日志 | 待完成 |
| `internal/kafka/consumer.go` | L156 | 解析错误发送到死信队列 | 待完成 |
| `internal/kafka/consumer.go` | L163 | 订单处理错误策略 | 待完成 |
| `internal/kafka/consumer.go` | L259 | 提交失败记录 | 待完成 |
| `internal/kafka/consumer.go` | L286-290 | 实现 SeekToOffset | 待完成 |

**优先级排序：**
1. 高优先级：死信队列、错误重试策略
2. 中优先级：日志集成、SeekToOffset 实现
3. 低优先级：两阶段快照完善

### 10.2 已完成的功能 (v2.0.0)

以下功能已在本版本中完成：

| 功能 | 文件 | 说明 |
|-----|------|------|
| **市场配置热加载** | `internal/config/nacos_loader.go` | 从 Nacos 动态加载市场配置，支持不停机添加/更新交易对 |
| **外部指数价格** | `internal/price/index_price.go` | 多数据源聚合的指数价格管理器，支持中位数/加权平均聚合 |
| **Kafka 消息发送** | `internal/engine/manager.go` | 完善的 trade-results, orderbook-updates, order-updates 发送 |
| **错误处理** | `internal/engine/manager.go` | CollectTrades/CollectUpdates/CollectCancels 带重试的错误处理 |
| **Leader 选举** | `internal/ha/leader_election.go` | 基于 Redis 的 Leader 选举机制 |
| **Standby 模式** | `internal/ha/standby_manager.go` | 支持主备切换和状态同步 |
| **性能指标** | `internal/metrics/metrics.go` | 撮合延迟、订单簿深度、吞吐量等完整指标 |

#### 10.2.1 市场配置热加载

- 支持从 Nacos 配置中心动态加载市场配置
- 配置变更回调机制，自动处理新增/更新/删除市场
- 轮询和监听两种模式
- 配置验证和错误处理

使用方式：
```yaml
# Nacos 配置 DataID: eidos-matching-markets
{
  "markets": [
    {
      "symbol": "BTC-USDC",
      "base_token": "BTC",
      "quote_token": "USDC",
      "price_decimals": 8,
      "size_decimals": 8,
      "min_size": "0.0001",
      "tick_size": "0.01",
      "maker_fee_rate": "0.0005",
      "taker_fee_rate": "0.001",
      "max_slippage": "0.05"
    }
  ]
}
```

#### 10.2.2 外部指数价格

- 支持多数据源聚合（中位数、加权平均、最优数据源）
- 异常价格过滤（偏离阈值检测）
- 价格置信度计算
- 可扩展的数据源接口

配置示例：
```yaml
index_price:
  enabled: true
  update_interval: 1s
  stale_threshold: 30s
  aggregation_method: median
  min_valid_sources: 1
  deviation_threshold: "0.1"
```

#### 10.2.3 高可用故障转移

- 基于 Redis 的 Leader 选举
- 毫秒级故障检测和切换
- Standby 节点状态同步
- 故障转移指标监控

配置示例：
```yaml
ha:
  enabled: true
  heartbeat_interval: 100ms
  failover_timeout: 500ms
```

#### 10.2.4 新增 Prometheus 指标

| 指标名 | 类型 | 说明 |
|-------|------|------|
| `eidos_matching_match_latency_microseconds` | Histogram | 撮合延迟（微秒级） |
| `eidos_matching_order_process_latency_microseconds` | Histogram | 订单处理延迟 |
| `eidos_matching_end_to_end_latency_milliseconds` | Histogram | 端到端延迟 |
| `eidos_matching_orderbook_bid_volume` | Gauge | 买单总量 |
| `eidos_matching_orderbook_ask_volume` | Gauge | 卖单总量 |
| `eidos_matching_orderbook_mid_price` | Gauge | 中间价 |
| `eidos_matching_orderbook_imbalance` | Gauge | 订单簿不平衡度 |
| `eidos_matching_orders_per_second` | Gauge | 每秒订单数 |
| `eidos_matching_trades_per_second` | Gauge | 每秒成交数 |
| `eidos_matching_leader_election_state` | Gauge | Leader 状态 |
| `eidos_matching_failover_total` | Counter | 故障转移次数 |
| `eidos_matching_index_price_deviation_percent` | Gauge | 指数价格偏差 |

---

## 变更日志

| 日期 | 版本 | 变更内容 |
|-----|------|---------|
| 2024-01-01 | v1.0.0 | 初始版本 |
| 2024-01-20 | v2.0.0 | 新增市场配置热加载、外部指数价格、高可用故障转移、完整性能指标 |
