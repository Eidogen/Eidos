# Eidos API 接口规范

> **Base URL**: `https://api.eidos.exchange`
> **WebSocket**: `wss://api.eidos.exchange/ws` (详见 00-协议总表.md)

---

## 一、OpenAPI 规范

### 1.1 Swagger 集成

后端使用 [swaggo/swag](https://github.com/swaggo/swag) 自动生成 OpenAPI 文档。

```go
// main.go
import (
    _ "eidos/docs" // swagger docs
    "github.com/gin-gonic/gin"
    swaggerFiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Eidos Exchange API
// @version         1.0.0
// @description     Eidos 现货交易所 REST API
// @host            api.eidos.exchange
// @BasePath        /api/v1
// @securityDefinitions.apikey WalletAuth
// @in header
// @name Authorization
// @description EIP-712 签名认证

func main() {
    r := gin.Default()
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    // ...
}
```

### 1.2 OpenAPI YAML

```yaml
openapi: 3.0.3
info:
  title: Eidos Exchange API
  version: 1.0.0
  description: |
    Eidos 现货交易所 REST API

    ## 认证方式
    所有写操作需要 EIP-712 签名认证。

    ## 错误码
    参见错误码章节。

servers:
  - url: https://api.eidos.exchange/api/v1
    description: Production
  - url: https://testnet-api.eidos.exchange/api/v1
    description: Testnet

security:
  - WalletAuth: []

components:
  securitySchemes:
    WalletAuth:
      type: apiKey
      in: header
      name: Authorization
      description: "格式: EIP712 {wallet}:{timestamp}:{signature}"

  schemas:
    # ============================================
    # 通用响应
    # ============================================
    Response:
      type: object
      properties:
        code:
          type: integer
          description: 错误码，0 表示成功
        message:
          type: string
          description: 错误信息
        data:
          type: object
          description: 响应数据

    PaginatedResponse:
      allOf:
        - $ref: '#/components/schemas/Response'
        - type: object
          properties:
            data:
              type: object
              properties:
                items:
                  type: array
                  items: {}
                pagination:
                  $ref: '#/components/schemas/Pagination'

    Pagination:
      type: object
      description: |
        分页参数规范:
        - 列表接口 (订单/余额/流水等): 使用 page + page_size
        - 行情接口 (K线/最近成交等): 使用 limit (无需分页，只取最新N条)
      properties:
        total:
          type: integer
        page:
          type: integer
        page_size:
          type: integer
        total_pages:
          type: integer

    # ============================================
    # 订单相关
    # ============================================
    OrderSide:
      type: string
      enum: [buy, sell]

    OrderType:
      type: string
      enum: [limit, market]

    OrderStatus:
      type: string
      enum: [pending, open, partial, filled, cancelled, expired, rejected]

    TimeInForce:
      type: string
      enum: [GTC, IOC, FOK]

    PrepareOrderRequest:
      type: object
      required:
        - market
        - side
        - type
        - size
      properties:
        market:
          type: string
          example: "BTC-USDC"
        side:
          $ref: '#/components/schemas/OrderSide'
        type:
          $ref: '#/components/schemas/OrderType'
        price:
          type: string
          description: 限价单必填
          example: "42000.00"
        size:
          type: string
          example: "0.1"
        time_in_force:
          $ref: '#/components/schemas/TimeInForce'
          default: GTC

    PrepareOrderResponse:
      type: object
      properties:
        order_id:
          type: string
          format: uuid
        typed_data:
          type: object
          description: EIP-712 TypedData，前端直接用于签名
        expires_at:
          type: integer
          format: int64
          description: Unix timestamp

    SubmitOrderRequest:
      type: object
      required:
        - order_id
        - signature
      properties:
        order_id:
          type: string
          format: uuid
        signature:
          type: string
          description: "0x 开头的签名"

    Order:
      type: object
      properties:
        order_id:
          type: string
        market:
          type: string
        side:
          $ref: '#/components/schemas/OrderSide'
        type:
          $ref: '#/components/schemas/OrderType'
        price:
          type: string
        size:
          type: string
        filled_size:
          type: string
        remaining_size:
          type: string
        avg_price:
          type: string
        status:
          $ref: '#/components/schemas/OrderStatus'
        created_at:
          type: integer
          format: int64
        updated_at:
          type: integer
          format: int64

    # ============================================
    # 余额相关
    # ============================================
    Balance:
      type: object
      properties:
        token:
          type: string
          example: "USDC"
        available:
          type: string
          example: "10000.00"
        frozen:
          type: string
          example: "500.00"
        total:
          type: string
          example: "10500.00"

    # ============================================
    # 行情相关
    # ============================================
    Ticker:
      type: object
      properties:
        market:
          type: string
        last_price:
          type: string
        price_change:
          type: string
        price_change_percent:
          type: string
        high_24h:
          type: string
        low_24h:
          type: string
        volume_24h:
          type: string
        quote_volume_24h:
          type: string
        timestamp:
          type: integer
          format: int64

    Depth:
      type: object
      properties:
        market:
          type: string
        bids:
          type: array
          items:
            type: array
            items:
              type: string
            minItems: 2
            maxItems: 2
          description: "[price, size]"
        asks:
          type: array
          items:
            type: array
            items:
              type: string
        timestamp:
          type: integer
          format: int64

    Kline:
      type: object
      properties:
        time:
          type: integer
          format: int64
        open:
          type: string
        high:
          type: string
        low:
          type: string
        close:
          type: string
        volume:
          type: string
        quote_volume:
          type: string
        trades:
          type: integer

paths:
  # ============================================
  # 订单接口
  # ============================================
  /orders/prepare:
    post:
      tags: [Orders]
      summary: 准备订单 (获取待签名数据)
      description: 返回 EIP-712 TypedData，前端签名后提交
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PrepareOrderRequest'
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        $ref: '#/components/schemas/PrepareOrderResponse'

  /orders:
    post:
      tags: [Orders]
      summary: 提交订单
      security:
        - WalletAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SubmitOrderRequest'
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        $ref: '#/components/schemas/Order'

    get:
      tags: [Orders]
      summary: 查询订单列表
      security:
        - WalletAuth: []
      parameters:
        - name: market
          in: query
          schema:
            type: string
        - name: status
          in: query
          schema:
            $ref: '#/components/schemas/OrderStatus'
        - name: page
          in: query
          schema:
            type: integer
            default: 1
        - name: page_size
          in: query
          schema:
            type: integer
            default: 20
            maximum: 100
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PaginatedResponse'

    delete:
      tags: [Orders]
      summary: 批量取消订单
      security:
        - WalletAuth: []
      parameters:
        - name: market
          in: query
          schema:
            type: string
          description: 不传则取消所有
      responses:
        '200':
          description: 成功

  /orders/{order_id}:
    get:
      tags: [Orders]
      summary: 查询单个订单
      security:
        - WalletAuth: []
      parameters:
        - name: order_id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        $ref: '#/components/schemas/Order'

    delete:
      tags: [Orders]
      summary: 取消订单
      security:
        - WalletAuth: []
      parameters:
        - name: order_id
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 成功

  /orders/open:
    get:
      tags: [Orders]
      summary: 查询当前挂单
      security:
        - WalletAuth: []
      parameters:
        - name: market
          in: query
          schema:
            type: string
      responses:
        '200':
          description: 成功

  # ============================================
  # 余额接口
  # ============================================
  /balances:
    get:
      tags: [Balances]
      summary: 查询所有余额
      security:
        - WalletAuth: []
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/Balance'

  /balances/{token}:
    get:
      tags: [Balances]
      summary: 查询单个余额
      security:
        - WalletAuth: []
      parameters:
        - name: token
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 成功

  # ============================================
  # 行情接口 (公开)
  # ============================================
  /markets:
    get:
      tags: [Market Data]
      summary: 交易对列表
      security: []
      responses:
        '200':
          description: 成功

  /tickers:
    get:
      tags: [Market Data]
      summary: 所有 Ticker
      security: []
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/Ticker'

  /ticker/{market}:
    get:
      tags: [Market Data]
      summary: 单个 Ticker
      security: []
      parameters:
        - name: market
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: 成功

  /depth/{market}:
    get:
      tags: [Market Data]
      summary: 订单簿深度
      security: []
      parameters:
        - name: market
          in: path
          required: true
          schema:
            type: string
        - name: level
          in: query
          schema:
            type: integer
            enum: [5, 10, 20, 50, 100]
            default: 20
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        $ref: '#/components/schemas/Depth'

  /klines/{market}:
    get:
      tags: [Market Data]
      summary: K线数据
      security: []
      parameters:
        - name: market
          in: path
          required: true
          schema:
            type: string
        - name: interval
          in: query
          required: true
          schema:
            type: string
            enum: [1m, 5m, 15m, 30m, 1h, 4h, 1d, 1w]
        - name: start_time
          in: query
          schema:
            type: integer
            format: int64
        - name: end_time
          in: query
          schema:
            type: integer
            format: int64
        - name: limit
          in: query
          schema:
            type: integer
            default: 500
            maximum: 1500
      responses:
        '200':
          description: 成功
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Response'
                  - properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/Kline'

  /trades/{market}:
    get:
      tags: [Market Data]
      summary: 最近成交
      security: []
      parameters:
        - name: market
          in: path
          required: true
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
            maximum: 500
      responses:
        '200':
          description: 成功
```

---

## 二、WebSocket 协议

### 2.1 连接

```
wss://api.eidos.exchange/ws
```

### 2.2 消息格式

```typescript
// 客户端 → 服务端
interface ClientMessage {
  type: "subscribe" | "unsubscribe" | "ping" | "auth";
  channels?: string[];      // subscribe/unsubscribe 时使用
  timestamp?: number;       // auth 时使用
  signature?: string;       // auth 时使用
  wallet?: string;          // auth 时使用
}

// 服务端 → 客户端
interface ServerMessage {
  type: "subscribed" | "unsubscribed" | "pong" | "error" | "data";
  channel?: string;
  data?: any;
  error?: {
    code: number;
    message: string;
  };
  timestamp: number;
}
```

### 2.3 频道定义

| 频道 | 格式 | 认证 | 说明 |
|------|------|------|------|
| ticker | `ticker.{market}` | 否 | 实时行情 |
| depth | `depth.{market}.{level}` | 否 | 订单簿深度 (level: 5/10/20/50/100) |
| trades | `trades.{market}` | 否 | 最新成交 |
| kline | `kline.{market}.{interval}` | 否 | K线更新 |

### 2.4 协议流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         WebSocket 协议流程                                   │
│                                                                              │
│  1. 建立连接                                                                 │
│  ─────────────                                                               │
│  Client ──────────────────────────────────────────────────────▶ Server      │
│          WS Connect: wss://api.eidos.exchange/ws                             │
│                                                                              │
│  Server ◀────────────────────────────────────────────────────── Client      │
│          {"type":"connected","timestamp":1704067200000}                      │
│                                                                              │
│  2. 订阅公开频道                                                             │
│  ─────────────────                                                           │
│  Client ──────────────────────────────────────────────────────▶ Server      │
│          {                                                                   │
│            "type": "subscribe",                                              │
│            "channels": ["ticker.BTC-USDC", "depth.BTC-USDC.20"]             │
│          }                                                                   │
│                                                                              │
│  Server ◀────────────────────────────────────────────────────── Client      │
│          {"type":"subscribed","channels":["ticker.BTC-USDC",...]}           │
│                                                                              │
│  3. 接收数据                                                                 │
│  ────────────                                                                │
│  Server ◀────────────────────────────────────────────────────── Client      │
│          {                                                                   │
│            "type": "data",                                                   │
│            "channel": "ticker.BTC-USDC",                                     │
│            "data": {"last_price":"42000.00",...},                           │
│            "timestamp": 1704067200000                                        │
│          }                                                                   │
│                                                                              │
│  4. 心跳保活 (每 30 秒)                                                      │
│  ──────────────────────                                                      │
│  Client ──────────────────────────────────────────────────────▶ Server      │
│          {"type":"ping"}                                                     │
│                                                                              │
│  Server ◀────────────────────────────────────────────────────── Client      │
│          {"type":"pong","timestamp":1704067230000}                          │
│                                                                              │
│  5. 断线重连                                                                 │
│  ────────────                                                                │
│  - 客户端检测到连接断开后，使用指数退避重连                                  │
│  - 退避时间: 1s, 2s, 4s, 8s, 16s, 32s (max)                                 │
│  - 重连后需重新订阅频道                                                      │
│  - 认证状态不保留，需重新认证                                                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.5 数据格式示例

```json
// ticker 推送
{
  "type": "data",
  "channel": "ticker.BTC-USDC",
  "data": {
    "market": "BTC-USDC",
    "last_price": "42000.00",
    "price_change": "500.00",
    "price_change_percent": "1.20",
    "high_24h": "42500.00",
    "low_24h": "41000.00",
    "volume_24h": "1234.56",
    "quote_volume_24h": "51851520.00"
  },
  "timestamp": 1704067200000
}

// depth 推送
{
  "type": "data",
  "channel": "depth.BTC-USDC.20",
  "data": {
    "bids": [["41999.00", "1.5"], ["41998.00", "2.3"]],
    "asks": [["42001.00", "0.8"], ["42002.00", "1.2"]],
    "sequence": 123456
  },
  "timestamp": 1704067200000
}

// orders 推送
{
  "type": "data",
  "channel": "orders.0x1234...abcd",
  "data": {
    "order_id": "uuid-xxx",
    "status": "partial",
    "filled_size": "0.05",
    "remaining_size": "0.05",
    "avg_price": "42000.00"
  },
  "timestamp": 1704067200000
}
```

---

## 三、错误码

### 3.1 通用错误码

| Code | Message | 说明 |
|------|---------|------|
| 0 | success | 成功 |
| 10001 | INVALID_SIGNATURE | 签名无效 |
| 10002 | SIGNATURE_EXPIRED | 签名过期 |
| 10003 | SIGNER_MISMATCH | 签名者不匹配 |
| 10004 | UNAUTHORIZED | 未认证 |
| 10005 | FORBIDDEN | 无权限 |
| 10006 | RATE_LIMITED | 请求过于频繁 |
| 10007 | INTERNAL_ERROR | 内部错误 |

### 3.2 订单错误码

| Code | Message | 说明 |
|------|---------|------|
| 20001 | ORDER_NOT_FOUND | 订单不存在 |
| 20002 | ORDER_EXPIRED | 订单过期 |
| 20003 | NONCE_ALREADY_USED | Nonce 已使用 |
| 20004 | INSUFFICIENT_BALANCE | 余额不足 |
| 20005 | INVALID_MARKET | 无效交易对 |
| 20006 | INVALID_PRICE | 无效价格 |
| 20007 | INVALID_SIZE | 无效数量 |
| 20008 | SIZE_TOO_SMALL | 数量过小 |
| 20009 | NOTIONAL_TOO_SMALL | 订单金额过小 |
| 20010 | MARKET_CLOSED | 市场已关闭 |
| 20011 | SELF_TRADE | 自成交 |
| 20012 | TOO_MANY_OPEN_ORDERS | 挂单数量超限 |
| 20013 | ORDER_ALREADY_FILLED | 订单已成交 |
| 20014 | ORDER_ALREADY_CANCELLED | 订单已取消 |

### 3.3 余额错误码

| Code | Message | 说明 |
|------|---------|------|
| 30001 | TOKEN_NOT_SUPPORTED | 不支持的代币 |
| 30002 | WITHDRAW_LIMIT_EXCEEDED | 提现超限 |
| 30003 | DEPOSIT_NOT_FOUND | 充值记录不存在 |
| 30004 | WITHDRAW_NOT_FOUND | 提现记录不存在 |

---

## 四、认证方式

### 4.1 EIP-712 签名认证

```typescript
// 前端签名
const typedData = {
  types: {
    EIP712Domain: [
      { name: "name", type: "string" },
      { name: "version", type: "string" },
      { name: "chainId", type: "uint256" }
    ],
    Request: [
      { name: "method", type: "string" },
      { name: "path", type: "string" },
      { name: "timestamp", type: "uint256" }
    ]
  },
  primaryType: "Request",
  domain: {
    name: "EidosExchange",
    version: "1",
    chainId: 42161
  },
  message: {
    method: "GET",
    path: "/api/v1/balances",
    timestamp: Date.now()
  }
};

const signature = await wallet.signTypedData(
  typedData.domain,
  { Request: typedData.types.Request },
  typedData.message
);

// 请求头
Authorization: EIP712 {wallet}:{timestamp}:{signature}
```

### 4.2 签名有效期

- 签名有效期: 5 分钟
- 服务端验证: `|server_time - timestamp| < 300000`

---

## 五、限流规则

| 类型 | 限制 | 说明 |
|------|------|------|
| 下单 | 10/s per wallet | 每个钱包每秒 10 单 |
| 查询 | 100/s per wallet | 每个钱包每秒 100 次查询 |
| 提现 | 1/s per wallet | 每个钱包每秒 1 次提现 |
| 全局下单 | 10000/s | 全局每秒 10000 单 |
| WebSocket | 5 conn/min per IP | 每 IP 每分钟 5 个新连接 |
