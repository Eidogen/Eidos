# eidos-trading 集成 TODO 清单

> 更新日期: 2026-01-16

---

## 一、代码中已标记的 TODO

| 文件 | 行号 | TODO 内容 | 优先级 |
|------|------|-----------|--------|
| `service/order_service.go` | 525 | 实现 EIP-712 签名验证 | P0 |
| `service/withdrawal_service.go` | 225 | 发送提现请求到风控服务 | P1 |
| `service/deposit_service.go` | 151 | 发送充值事件到 Kafka 用于通知用户 | P2 |
| `worker/outbox_relay.go` | 190 | 发送告警通知 | P2 |
| `worker/cancel_outbox_relay.go` | 659 | 发送告警通知 | P2 |
| `kafka/producer.go` | 161 | 将失败消息写入重试队列 | P3 |

---

## 二、需要新增的功能

### P0 - 阻塞上线

| 功能 | 文件 | 依赖服务 | 说明 |
|------|------|----------|------|
| EIP-712 签名验证 | `service/order_service.go` | eidos-common | 当前 Mock 模式，需实现真实验签 |
| 结算批次生成 | 新建 `service/settlement_service.go` | eidos-chain | 周期性打包待结算成交，发送到 Kafka |
| 提现 Kafka 发送 | `service/withdrawal_service.go` | eidos-chain | 创建提现后发送到 `withdrawals` topic |

### P1 - 上线后优化

| 功能 | 文件 | 依赖服务 | 说明 |
|------|------|----------|------|
| 风控服务对接 | 新建 `client/risk_client.go` | eidos-risk | 下单/提现前调用风控校验 |
| 订单状态推送 | 新建 `publisher/order_publisher.go` | eidos-api | 发送到 `order-updates` topic |
| 余额变更推送 | 新建 `publisher/balance_publisher.go` | eidos-api | 发送到 `balance-updates` topic |

### P2 - 长期优化

| 功能 | 说明 |
|------|------|
| 分布式追踪 | 接入 OpenTelemetry (Jaeger/Zipkin) |
| 告警通知 | 对账不一致、消息重试失败等场景 |
| 性能压测 | 单机 10000 TPS 下单目标 |

---

## 三、Kafka 消息对接

### 3.1 消费的 Topic (Inbound)

| Topic | 来源服务 | 处理器 | 状态 |
|-------|----------|--------|------|
| `trade-results` | eidos-matching | TradeEventHandler | 🟡 待联调 |
| `order-cancelled` | eidos-matching | OrderCancelledHandler | 🟡 待联调 |
| `order-accepted` | eidos-matching | OrderAcceptedHandler | 🟡 待联调 |
| `deposits` | eidos-chain | DepositHandler | 🟡 待联调 |
| `settlement-confirmed` | eidos-chain | SettlementConfirmedHandler | 🟡 待联调 |
| `withdrawal-confirmed` | eidos-chain | WithdrawalConfirmedHandler | 🟡 待联调 |

### 3.2 生产的 Topic (Outbound)

| Topic | 目标服务 | 触发时机 | 状态 |
|-------|----------|----------|------|
| `orders` | eidos-matching | 订单创建后 | 🟡 待联调 |
| `cancel-requests` | eidos-matching | 取消请求后 | 🟡 待联调 |
| `settlements` | eidos-chain | 批量结算时 | ⚪ **未实现** |
| `withdrawals` | eidos-chain | 提现创建后 | ⚪ **未实现** |
| `order-updates` | eidos-api | 订单状态变更 | ⚪ **未实现** |
| `balance-updates` | eidos-api | 余额变更 | ⚪ **未实现** |

### 3.3 消息格式

#### trade-results (from eidos-matching)
```json
{
  "trade_id": "T1234567890123456789",
  "market": "ETH-USDC",
  "maker_order_id": "O1234567890123456789",
  "taker_order_id": "O1234567890123456790",
  "maker": "0x1234...abcd",
  "taker": "0x5678...efgh",
  "price": "3000.50",
  "size": "1.5",
  "quote_amount": "4500.75",
  "maker_fee": "2.25",
  "taker_fee": "4.50",
  "timestamp": 1705401600000,
  "maker_is_buyer": true
}
```

#### deposits (from eidos-chain)
```json
{
  "tx_hash": "0xabc123...",
  "wallet": "0x1234...abcd",
  "token": "USDC",
  "amount": "1000.00",
  "block_number": 12345678,
  "timestamp": 1705401600000
}
```

#### settlement-confirmed (from eidos-chain)
```json
{
  "settlement_id": "S1234567890",
  "trade_ids": ["T123", "T124", "T125"],
  "tx_hash": "0xdef456...",
  "block_number": 12345679,
  "status": "confirmed",
  "timestamp": 1705401700000
}
```

---

## 四、服务对接清单

### 4.1 eidos-matching (撮合引擎)

| 对接项 | 方向 | 协议 | 状态 |
|--------|------|------|------|
| 订单投递 | Trading → Matching | Kafka `orders` | 🟡 确认消息格式 |
| 取消请求 | Trading → Matching | Kafka `cancel-requests` | 🟡 确认消息格式 |
| 成交结果 | Matching → Trading | Kafka `trade-results` | 🟡 联调测试 |
| 订单确认 | Matching → Trading | Kafka `order-cancelled` | 🟡 联调测试 |

### 4.2 eidos-chain (链上服务)

| 对接项 | 方向 | 协议 | 状态 |
|--------|------|------|------|
| 充值事件 | Chain → Trading | Kafka `deposits` | 🟡 联调测试 |
| 结算请求 | Trading → Chain | Kafka `settlements` | ⚪ **需实现** |
| 结算确认 | Chain → Trading | Kafka `settlement-confirmed` | 🟡 联调测试 |
| 提现请求 | Trading → Chain | Kafka `withdrawals` | ⚪ **需实现** |
| 提现确认 | Chain → Trading | Kafka `withdrawal-confirmed` | 🟡 联调测试 |

### 4.3 eidos-risk (风控服务)

| 对接项 | 方向 | 协议 | 状态 |
|--------|------|------|------|
| 下单前校验 | Trading → Risk | gRPC | ⚪ **需实现** |
| 提现审核 | Trading → Risk | gRPC | ⚪ **需实现** |

### 4.4 eidos-api (API 网关)

| 对接项 | 方向 | 协议 | 状态 |
|--------|------|------|------|
| gRPC 接口 | API → Trading | gRPC | ✅ 已实现 |
| 订单状态推送 | Trading → API | Kafka `order-updates` | ⚪ **需实现** |
| 余额变更推送 | Trading → API | Kafka `balance-updates` | ⚪ **需实现** |

---

## 五、联调检查清单

### 与 eidos-matching 联调
- [ ] 确认 Kafka topic 名称一致
- [ ] 确认消息 JSON 字段名一致 (价格/数量用 string)
- [ ] 测试: 正常下单 → 成交 → 清算
- [ ] 测试: 下单 → 部分成交 → 取消剩余
- [ ] 测试: 并发下单 (100 TPS)

### 与 eidos-chain 联调
- [ ] 确认 Kafka topic 名称一致
- [ ] 确认消息 JSON 字段名一致 (地址用 0x...)
- [ ] 测试: 充值检测 → 入金
- [ ] 测试: 提现申请 → 链上确认
- [ ] 测试: 结算批次 → 链上确认
- [ ] 测试: 结算失败 → 回滚

### 与 eidos-risk 联调
- [ ] 定义 proto 接口
- [ ] 确认降级策略 (服务不可用时放行 or 拒绝?)
