# Eidos API 服务设计

> 服务名: eidos-api
> 语言: Go

---

## 一、服务职责

### 1.1 核心定位

eidos-api 是面向外部的 API 服务，提供 REST API 和 WebSocket 两种接入方式，是用户与系统交互的统一入口。

### 1.2 职责边界

| 职责 | 说明 | 属于本服务 |
|------|------|-----------|
| REST API | 下单、查询、取消等操作 | ✅ |
| WebSocket | 行情推送、订单状态推送 | ✅ |
| 请求转发 | 转发请求到后端 gRPC 服务 | ✅ |
| 限流控制 | 按用户/IP 限流 | ✅ |
| 协议转换 | REST → gRPC | ✅ |
| 业务逻辑 | 订单撮合、余额计算等 | ❌ (各业务服务) |

### 1.3 对外提供的能力

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        eidos-api 对外能力                                    │
│                                                                              │
│  REST API:                                                                   │
│  ──────────                                                                  │
│  交易接口:                                                                   │
│    POST   /api/v1/orders/prepare      获取订单 EIP-712 签名摘要             │
│    POST   /api/v1/orders              创建订单 (需附带签名)                 │
│    DELETE /api/v1/orders/{id}         取消订单                              │
│    DELETE /api/v1/orders              批量取消                              │
│    GET    /api/v1/orders              查询订单列表                          │
│    GET    /api/v1/orders/{id}         查询单个订单                          │
│    GET    /api/v1/orders/open         查询当前挂单                          │
│    GET    /api/v1/trades              查询成交记录                          │
│                                                                              │
│  资产接口:                                                                   │
│    GET    /api/v1/balances            查询所有余额                          │
│    GET    /api/v1/balances/{token}    查询单个余额                          │
│    GET    /api/v1/transactions        查询资金流水                          │
│    POST   /api/v1/withdrawals         提现请求                              │
│    GET    /api/v1/deposits            查询充值记录                          │
│    GET    /api/v1/withdrawals         查询提现记录                          │
│                                                                              │
│  行情接口:                                                                   │
│    GET    /api/v1/markets             交易对列表                            │
│    GET    /api/v1/ticker/{symbol}     单个 Ticker                           │
│    GET    /api/v1/tickers             所有 Ticker                           │
│    GET    /api/v1/depth/{symbol}      订单簿深度                            │
│    GET    /api/v1/klines/{symbol}     K线数据                               │
│    GET    /api/v1/trades/{symbol}     最近成交                              │
│                                                                              │
│  WebSocket:                                                                  │
│  ────────────                                                                │
│  连接地址: wss://api.eidos.exchange/ws                                      │
│                                                                              │
│  公共频道 (无需认证):                                                        │
│    - ticker.{symbol}           Ticker 更新                                  │
│    - depth.{symbol}.{level}    订单簿深度                                   │
│    - kline.{symbol}.{interval} K线更新                                      │
│    - trades.{symbol}           成交流                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、架构设计

### 2.1 服务架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          eidos-api 服务架构                                  │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         外部请求入口                                 │    │
│  │                                                                      │    │
│  │     REST API (Gin)              WebSocket (Gorilla)                 │    │
│  │     :8080                       :8080/ws (同端口，路径区分)         │    │
│  └───────────────────────────────────┬─────────────────────────────────┘    │
│                                      │                                      │
│                                      ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         中间件层                                     │    │
│  │                                                                      │    │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐           │    │
│  │  │ 限流      │ │ 认证      │ │ 日志      │ │ Trace     │           │    │
│  │  │ RateLimit │ │ WalletAuth│ │ Logger    │ │ Tracing   │           │    │
│  │  └───────────┘ └───────────┘ └───────────┘ └───────────┘           │    │
│  └───────────────────────────────────┬─────────────────────────────────┘    │
│                                      │                                      │
│                                      ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Handler 层                                   │    │
│  │                                                                      │    │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐           │    │
│  │  │ Order     │ │ Balance   │ │ Market    │ │ WebSocket │           │    │
│  │  │ Handler   │ │ Handler   │ │ Handler   │ │ Handler   │           │    │
│  │  └───────────┘ └───────────┘ └───────────┘ └───────────┘           │    │
│  └───────────────────────────────────┬─────────────────────────────────┘    │
│                                      │                                      │
│                                      ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     gRPC Client 层 (通过 Nacos 发现)                 │    │
│  │                                                                      │    │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────┐                          │    │
│  │  │eidos-     │ │eidos-     │ │eidos-     │                          │    │
│  │  │trading    │ │market     │ │risk       │                          │    │
│  │  │Client     │ │Client     │ │Client     │                          │    │
│  │  └───────────┘ └───────────┘ └───────────┘                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     Redis Pub/Sub (WebSocket 推送)                   │    │
│  │                                                                      │    │
│  │  订阅: ticker.*, depth.*, kline.*, trades.*, orders.*, balances.*   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 请求处理流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         REST API 请求流程                                    │
│                                                                              │
│  Client              eidos-api             后端服务              响应        │
│    │                     │                    │                   │          │
│    │ POST /api/v1/orders │                    │                   │          │
│    │ Authorization: ...  │                    │                   │          │
│    │ Body: {...}         │                    │                   │          │
│    │────────────────────>│                    │                   │          │
│    │                     │                    │                   │          │
│    │                     │ 1. 限流检查        │                   │          │
│    │                     │    Redis INCR      │                   │          │
│    │                     │                    │                   │          │
│    │                     │ 2. 解析请求体      │                   │          │
│    │                     │    参数校验        │                   │          │
│    │                     │                    │                   │          │
│    │                     │ 3. gRPC 调用       │                   │          │
│    │                     │    eidos-trading     │                   │          │
│    │                     │───────────────────>│                   │          │
│    │                     │                    │                   │          │
│    │                     │ 4. 返回结果        │                   │          │
│    │                     │<───────────────────│                   │          │
│    │                     │                    │                   │          │
│    │                     │ 5. 转换响应格式    │                   │          │
│    │ 200 OK              │                    │                   │          │
│    │ {"order_id": "..."}│                    │                   │          │
│    │<────────────────────│                    │                   │          │
│    │                     │                    │                   │          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 WebSocket 管理

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        WebSocket 连接管理                                    │
│                                                                              │
│  连接生命周期:                                                               │
│  ──────────────                                                              │
│  1. 客户端建立连接                                                           │
│  2. 发送订阅消息: {"type":"subscribe","channel":"ticker.BTC-USDC"}          │
│  3. 服务端确认订阅                                                           │
│  4. 推送实时数据                                                             │
│  5. 心跳保活 (30s ping/pong)                                                │
│  6. 客户端断开或超时                                                         │
│                                                                              │
│  连接管理器:                                                                 │
│  ────────────                                                                │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  ConnectionManager                                                    │   │
│  │                                                                       │   │
│  │  connections map[string]*Connection     // connID -> Connection       │   │
│  │  subscriptions map[string]map[string]   // channel -> connIDs         │   │
│  │                                                                       │   │
│  │  - AddConnection(conn)                                                │   │
│  │  - RemoveConnection(connID)                                           │   │
│  │  - Subscribe(connID, channel)                                         │   │
│  │  - Unsubscribe(connID, channel)                                       │   │
│  │  - Broadcast(channel, message)                                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Redis Pub/Sub 集成:                                                         │
│  ───────────────────                                                         │
│  - 订阅 Redis 频道，接收行情/订单更新                                        │
│  - 根据本地订阅关系，推送给对应的 WebSocket 连接                             │
│  - 支持多实例部署，通过 Redis 同步                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、API 设计

### 3.1 认证方式

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          认证设计                                            │
│                                                                              │
│  写操作 (下单、取消、提现):                                                  │
│  ═══════════════════════════                                                 │
│  - 每次请求携带 EIP-712 签名                                                 │
│  - 后端验证签名，确认请求来自钱包持有者                                      │
│                                                                              │
│  请求头 (参考: 3-开发规范/00-协议总表.md):                                   │
│    Authorization: EIP712 {wallet}:{timestamp}:{signature}                   │
│    示例: Authorization: EIP712 0x1234...abcd:1704067200000:0xabcd...def      │
│                                                                              │
│  读操作 (查询余额、订单):                                                    │
│  ═══════════════════════                                                     │
│  - 方式1: 签名认证 (同写操作)                                                │
│  - 方式2: 简化认证 (时间戳签名)                                              │
│                                                                              │
│  签名消息格式:                                                               │
│  {                                                                           │
│    "types": {                                                                │
│      "Request": [                                                            │
│        {"name": "method", "type": "string"},                                 │
│        {"name": "path", "type": "string"},                                   │
│        {"name": "timestamp", "type": "uint256"}                              │
│      ]                                                                       │
│    },                                                                        │
│    "message": {                                                              │
│      "method": "GET",                                                        │
│      "path": "/api/v1/balances",                                             │
│      "timestamp": 1704067200000                                              │
│    }                                                                         │
│  }                                                                           │
│                                                                              │
│  公开接口 (无需认证):                                                        │
│  ═══════════════════                                                         │
│  - 行情数据: tickers, depth, klines, trades                                 │
│  - 交易对信息: markets                                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 限流策略

```yaml
# 限流配置
rate_limit:
  # 按钱包地址限流
  per_wallet:
    orders: 10/s        # 下单 10次/秒
    queries: 100/s      # 查询 100次/秒
    withdrawals: 1/s    # 提现 1次/秒

  # 按 IP 限流
  per_ip:
    total: 1000/min     # 总请求 1000次/分钟
    websocket: 5/min    # WebSocket 连接 5次/分钟

  # 全局限流
  global:
    orders: 10000/s     # 全局下单 10000次/秒
```

### 3.3 响应格式

```json
// 成功响应
{
    "code": 0,
    "message": "success",
    "data": { ... }
}

// 分页响应
{
    "code": 0,
    "message": "success",
    "data": {
        "items": [ ... ],
        "pagination": {
            "total": 100,
            "page": 1,
            "page_size": 20,
            "total_pages": 5
        }
    }
}

// 错误响应
{
    "code": 10001,
    "message": "INVALID_SIGNATURE",
    "data": null
}
```

### 3.4 订单签名流程

```
订单创建采用两步流程，确保签名安全:

1. POST /api/v1/orders/prepare (获取待签名数据)
   ─────────────────────────────
   请求:
   {
     "market": "BTC-USDC",
     "side": "buy",
     "type": "limit",
     "price": "42000.00",
     "size": "0.1"
   }

   响应:
   {
     "code": 0,
     "data": {
       "order_id": "uuid-xxx",
       "typed_data": {
         "types": {
           "EIP712Domain": [...],
           "Order": [
             {"name": "orderId", "type": "bytes32"},
             {"name": "market", "type": "string"},
             {"name": "side", "type": "uint8"},
             {"name": "price", "type": "uint256"},
             {"name": "size", "type": "uint256"},
             {"name": "nonce", "type": "uint256"},
             {"name": "expiration", "type": "uint256"}
           ]
         },
         "primaryType": "Order",
         "domain": {
           "name": "EidosExchange",
           "version": "1",
           "chainId": 42161,
           "verifyingContract": "0x..."
         },
         "message": {
           "orderId": "0x...",
           "market": "BTC-USDC",
           "side": 0,
           "price": "42000000000",
           "size": "100000000",
           "nonce": 1,
           "expiration": 1704153600
         }
       },
       "expires_at": 1704153600
     }
   }

2. POST /api/v1/orders (提交已签名订单)
   ─────────────────────────────
   请求:
   {
     "order_id": "uuid-xxx",
     "signature": "0x..."
   }

   响应:
   {
     "code": 0,
     "data": {
       "order_id": "uuid-xxx",
       "status": "pending"
     }
   }

注意事项:
- prepare 返回的 typed_data 有效期为 5 分钟 (expires_at)
- 过期后提交签名返回错误码 10002 (SIGNATURE_EXPIRED)
- 需重新调用 prepare 获取新的签名数据
```

### 3.5 签名时间窗口

```
签名有效性规则:
═══════════════════════

1. 请求签名 (Authorization Header):
   - timestamp 必须在当前时间 ±5 分钟内
   - 超出范围返回 10002 (SIGNATURE_EXPIRED)

2. 订单签名 (prepare + submit):
   - prepare 返回 expires_at (默认当前时间 + 5 分钟)
   - 签名提交时检查是否过期

3. 重放保护:
   - 相同签名内容 (wallet + timestamp + signature) 5 分钟内不可重复使用
   - 使用 Redis 缓存已使用的签名哈希
```

### 3.6 错误码定义

| 错误码 | 名称 | 说明 |
|--------|------|------|
| **0** | SUCCESS | 成功 |
| **通用错误 (10xxx)** | | |
| 10001 | INVALID_SIGNATURE | 签名验证失败 |
| 10002 | SIGNATURE_EXPIRED | 签名/请求过期 |
| 10003 | INVALID_PARAMS | 参数错误 |
| 10004 | UNAUTHORIZED | 未授权访问 |
| 10005 | FORBIDDEN | 禁止访问 |
| **订单错误 (11xxx)** | | |
| 11001 | ORDER_NOT_FOUND | 订单不存在 |
| 11002 | ORDER_ALREADY_CANCELLED | 订单已取消 |
| 11003 | ORDER_ALREADY_FILLED | 订单已完全成交 |
| 11004 | INVALID_PRICE | 价格无效 |
| 11005 | INVALID_SIZE | 数量无效 |
| 11006 | PRICE_OUT_OF_RANGE | 价格超出限制 |
| 11007 | SIZE_TOO_SMALL | 数量低于最小值 |
| 11008 | SIZE_TOO_LARGE | 数量超出限制 |
| 11009 | NOTIONAL_TOO_SMALL | 名义价值低于最小值 |
| 11010 | MAX_OPEN_ORDERS | 挂单数量超出限制 |
| **资产错误 (12xxx)** | | |
| 12001 | INSUFFICIENT_BALANCE | 余额不足 |
| 12002 | WITHDRAW_LIMIT_EXCEEDED | 提现超出限额 |
| 12003 | TOKEN_NOT_SUPPORTED | 代币不支持 |
| **市场错误 (13xxx)** | | |
| 13001 | MARKET_NOT_FOUND | 交易对不存在 |
| 13002 | MARKET_SUSPENDED | 交易对暂停交易 |
| **系统错误 (20xxx)** | | |
| 20001 | RATE_LIMIT_EXCEEDED | 请求过于频繁 |
| 20002 | SERVICE_UNAVAILABLE | 服务暂时不可用 |
| 20003 | INTERNAL_ERROR | 内部错误 |

### 3.7 限流规则

```yaml
# 限流配置详情
rate_limit:
  # 按钱包地址限流
  per_wallet:
    orders: 10/s          # 下单 10次/秒
    cancels: 20/s         # 取消 20次/秒
    queries: 100/s        # 查询 100次/秒
    withdrawals: 1/s      # 提现 1次/秒

  # 按 IP 限流
  per_ip:
    total: 1000/min       # 总请求 1000次/分钟
    websocket_conn: 5/min # WebSocket 连接 5次/分钟
    public_api: 100/s     # 公开接口 100次/秒

  # 全局限流
  global:
    orders: 10000/s       # 全局下单 10000次/秒
```

**限流响应**:
```json
HTTP/1.1 429 Too Many Requests
Retry-After: 5

{
    "code": 20001,
    "message": "RATE_LIMIT_EXCEEDED",
    "data": {
        "limit": 10,
        "window": "1s",
        "retry_after": 5
    }
}
```

**限流算法**: 滑动窗口 (Sliding Window)

### 3.8 WebSocket 消息格式

**说明**: 当前版本 WebSocket 暂不需要认证，所有频道均可直接订阅。

#### 3.8.1 客户端消息

```json
// 订阅
{
    "type": "subscribe",
    "channels": ["ticker.BTC-USDC", "depth.BTC-USDC.20"]
}

// 取消订阅
{
    "type": "unsubscribe",
    "channels": ["ticker.BTC-USDC"]
}

// 心跳 (可选，服务端也会主动 ping)
{
    "type": "ping"
}
```

#### 3.8.2 服务端消息

```json
// 订阅确认
{
    "type": "subscribed",
    "channel": "ticker.BTC-USDC"
}

// 取消订阅确认
{
    "type": "unsubscribed",
    "channel": "ticker.BTC-USDC"
}

// 心跳响应
{
    "type": "pong",
    "ts": 1704067200000
}

// 错误消息
{
    "type": "error",
    "code": 10003,
    "message": "Invalid channel format"
}

// Ticker 推送
{
    "type": "data",
    "channel": "ticker.BTC-USDC",
    "data": {
        "symbol": "BTC-USDC",
        "last": "42000.00",
        "open_24h": "41000.00",
        "high_24h": "42500.00",
        "low_24h": "40500.00",
        "volume_24h": "1234.56",
        "change_24h": "2.44",
        "ts": 1704067200000
    }
}

// 深度推送
{
    "type": "data",
    "channel": "depth.BTC-USDC.20",
    "data": {
        "symbol": "BTC-USDC",
        "bids": [["41990.00", "1.5"], ["41980.00", "2.0"]],
        "asks": [["42010.00", "1.2"], ["42020.00", "1.8"]],
        "ts": 1704067200000
    }
}

// 成交推送
{
    "type": "data",
    "channel": "trades.BTC-USDC",
    "data": {
        "trade_id": "xxx",
        "price": "42000.00",
        "size": "0.1",
        "side": "buy",
        "ts": 1704067200000
    }
}
```

#### 3.8.3 连接管理

| 参数 | 值 | 说明 |
|------|-----|------|
| 最大订阅频道数 | 50 | 超出返回错误 |
| 心跳间隔 | 30s | 服务端主动 ping |
| Pong 超时 | 10s | 未收到 pong 断开连接 |
| 重连策略 | 断开后需重新订阅 | 连接不保持状态 |

---

## 四、数据库设计

eidos-api 本身不持有业务数据，仅存储连接和限流相关数据。

### 4.1 Redis 数据结构

```
# 限流计数
ratelimit:{wallet}:orders     # 钱包下单计数
ratelimit:{ip}:total          # IP 总请求计数
TTL: 1 秒/1 分钟

# WebSocket 会话
ws:session:{conn_id}
Value: {
    "wallet": "0x...",
    "channels": ["ticker.BTC-USDC", "depth.BTC-USDC.20"],
    "connected_at": 1704067200000
}
TTL: 会话有效期

# 在线用户
ws:online:{wallet}
Value: conn_id
TTL: 心跳超时时间
```

---

## 五、服务依赖

### 5.1 gRPC 调用关系

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        eidos-api 服务调用                                    │
│                                                                              │
│                              eidos-api                                       │
│                                  │                                           │
│         ┌────────────────────────┼────────────────────────┐                  │
│         │                        │                        │                  │
│         ▼                        ▼                        ▼                  │
│  ┌────────────────────────────────────────────┐   ┌──────────────┐           │
│  │              eidos-trading                 │   │ eidos-market │           │
│  │                                            │   │              │           │
│  │ CreateOrder / CancelOrder / GetOrder       │   │ GetKlines    │           │
│  │ ListOrders / GetOpenOrders / GetUserTrades │   │ GetTicker    │           │
│  │ GetBalance / GetBalances / GetDeposits     │   │ GetOrderBook │           │
│  │ GetWithdrawals / RequestWithdraw           │   │ GetTrades    │           │
│  │ GetTransactions                            │   │              │           │
│  └────────────────────────────────────────────┘   └──────────────┘           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Redis 订阅关系

```
eidos-api 订阅 Redis Pub/Sub 频道 (参考: 3-开发规范/00-协议总表.md):
─────────────────────────────────────────────────────────────────────
- eidos:ticker:{market}         → 推送给订阅 ticker.{market} 的客户端
- eidos:depth:{market}          → 推送给订阅 depth.{market}.* 的客户端
- eidos:kline:{market}:{interval} → 推送给订阅 kline.{market}.{interval} 的客户端
- eidos:trades:{market}         → 推送给订阅 trades.{market} 的客户端
```

---

## 六、配置项

```yaml
service:
  name: eidos-api
  port: 8080  # REST + WebSocket 同端口，WS 通过 /ws 路径区分
  # 详见: 3-开发规范/00-协议总表.md

nacos:
  server_addr: "nacos:8848"
  namespace: "eidos-prod"
  group: "EIDOS_GROUP"

# gRPC 客户端配置 (通过 Nacos 发现)
grpc_clients:
  eidos-trading:
    timeout: 5s
  eidos-market:
    timeout: 3s

redis:
  cluster:
    nodes:
      - redis-1:6379
      - redis-2:6379
      - redis-3:6379

rate_limit:
  per_wallet:
    orders: 10
    queries: 100
  per_ip:
    total: 1000

websocket:
  ping_interval: 30s
  pong_timeout: 10s
  max_connections: 100000
```
