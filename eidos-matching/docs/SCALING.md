# eidos-matching 水平扩展方案

## 架构概述

撮合引擎采用 **按市场分区** 的水平扩展策略，每个市场由单独的引擎实例处理，保证订单簿状态的一致性和撮合的确定性。

```
                    ┌─────────────────────────────────────────────────┐
                    │                   Kafka                          │
                    │  ┌─────────┐ ┌─────────┐ ┌─────────────────┐    │
                    │  │ orders  │ │ cancels │ │  trade-results  │    │
                    │  │ (P0-P7) │ │ (P0-P7) │ │    (P0-P7)      │    │
                    │  └────┬────┘ └────┬────┘ └───────▲─────────┘    │
                    └───────┼───────────┼─────────────┼───────────────┘
                            │           │             │
              ┌─────────────┼───────────┼─────────────┼─────────────┐
              │             ▼           ▼             │             │
              │  ┌──────────────────────────────┐     │             │
              │  │      Consumer Group          │     │             │
              │  │   (按 market key 路由)        │     │             │
              │  └──────────────────────────────┘     │             │
              │             │                         │             │
              │    ┌────────┼────────┐                │             │
              │    ▼        ▼        ▼                │             │
              │ ┌──────┐ ┌──────┐ ┌──────┐           │             │
              │ │ Inst │ │ Inst │ │ Inst │           │             │
              │ │  1   │ │  2   │ │  3   │           │             │
              │ │      │ │      │ │      │           │             │
              │ │BTC   │ │ETH   │ │SOL   │───────────┘             │
              │ │USDC  │ │USDC  │ │USDC  │                         │
              │ └──────┘ └──────┘ └──────┘                         │
              │     eidos-matching 实例集群                         │
              └─────────────────────────────────────────────────────┘
```

## 分区策略

### Kafka 分区

| Topic | 分区键 (Key) | 分区数 | 说明 |
|-------|-------------|--------|------|
| `orders` | `market` | 8+ | 订单按市场路由，保证同市场订单顺序消费 |
| `cancel-requests` | `market` | 8+ | 取消请求按市场路由 |
| `trade-results` | `market` | 8+ | 成交结果按市场分区 |
| `orderbook-updates` | `market` | 8+ | 订单簿更新按市场分区 |

### 实例分配

```yaml
# 实例1: 处理 BTC-USDC, LINK-USDC
instance-1:
  markets:
    - BTC-USDC
    - LINK-USDC

# 实例2: 处理 ETH-USDC, UNI-USDC
instance-2:
  markets:
    - ETH-USDC
    - UNI-USDC

# 实例3: 处理 SOL-USDC, AVAX-USDC
instance-3:
  markets:
    - SOL-USDC
    - AVAX-USDC
```

## 扩展场景

### 场景1: 新增市场

1. 在 Kafka 中创建分区 (如果需要)
2. 更新配置，将新市场分配给某个实例
3. 重启对应实例或热加载配置

```go
// 热加载新市场 (需实现)
engineManager.AddMarket(&MarketConfig{
    Symbol: "DOGE-USDC",
    // ...
})
```

### 场景2: 新增实例 (负载均衡)

1. 部署新实例
2. 重新分配市场：从高负载实例迁移部分市场到新实例
3. 平滑迁移流程：
   - 旧实例停止消费该市场
   - 新实例从快照恢复
   - 新实例开始消费

### 场景3: 故障转移

1. 健康检查检测到实例故障
2. 备用实例或其他实例接管故障实例的市场
3. 从 Redis 快照恢复订单簿状态
4. 从 Kafka 偏移量继续消费

## 状态管理

### 订单簿状态

- **存储**: Redis (快照)
- **恢复**: 启动时从最新快照加载
- **一致性**: 两阶段快照机制，确保快照与 Kafka 偏移量原子性

### Kafka 偏移量

- **管理**: Consumer Group 自动管理
- **快照关联**: 快照中记录对应的 Kafka 偏移量
- **恢复**: 从快照中的偏移量继续消费

## 配置示例

### 单实例多市场

```yaml
service:
  name: eidos-matching
  instance_id: matching-1

markets:
  - symbol: BTC-USDC
    base_token: BTC
    quote_token: USDC
    price_decimals: 2
    size_decimals: 8
  - symbol: ETH-USDC
    base_token: ETH
    quote_token: USDC
    price_decimals: 2
    size_decimals: 8

kafka:
  brokers:
    - kafka-1:9092
    - kafka-2:9092
    - kafka-3:9092
  consumer:
    group_id: eidos-matching-group
    # 通过 group 内的 partition 分配自动路由

snapshot:
  interval: 1m
  two_phase: true  # 生产环境建议开启
```

### 多实例配置 (K8s ConfigMap)

```yaml
# configmap-matching-1.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: matching-1-config
data:
  config.yaml: |
    service:
      instance_id: matching-1
    markets:
      - symbol: BTC-USDC
      - symbol: LINK-USDC

---
# configmap-matching-2.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: matching-2-config
data:
  config.yaml: |
    service:
      instance_id: matching-2
    markets:
      - symbol: ETH-USDC
      - symbol: UNI-USDC
```

## 监控指标

### 实例级别

| 指标 | 说明 |
|------|------|
| `matching_orders_processed_total` | 处理的订单总数 |
| `matching_trades_generated_total` | 生成的成交总数 |
| `matching_latency_us` | 撮合延迟 (微秒) |
| `matching_orderbook_depth` | 订单簿深度 |

### 市场级别

| 指标 | 说明 |
|------|------|
| `matching_market_orders_total{market="BTC-USDC"}` | 各市场订单数 |
| `matching_market_trades_total{market="BTC-USDC"}` | 各市场成交数 |
| `matching_market_bid_levels{market="BTC-USDC"}` | 买单档位数 |
| `matching_market_ask_levels{market="BTC-USDC"}` | 卖单档位数 |

## 限制和约束

1. **单市场单实例**: 同一市场只能被一个实例处理，不支持市场级别的并行
2. **顺序保证**: 依赖 Kafka 分区顺序，同市场订单必须顺序处理
3. **状态本地化**: 订单簿状态在内存中，需要快照机制保证持久化
4. **迁移成本**: 市场迁移需要停止-恢复流程，存在短暂不可用

## 容量规划

### 单实例容量参考

| 配置 | 容量 |
|------|------|
| 4 核 8GB | 10K 订单/秒, 5 个市场 |
| 8 核 16GB | 50K 订单/秒, 20 个市场 |
| 16 核 32GB | 100K 订单/秒, 50 个市场 |

### 扩展计算

```
总市场数 = N
单实例市场数 = M
所需实例数 = ceil(N / M)

示例: 100 个市场, 每实例 20 个市场
所需实例 = ceil(100 / 20) = 5 个实例
```
