# eidos-api 对接清单

## 一、服务对接状态总览

| 服务/组件 | 状态 | 优先级 | 负责人 | 备注 |
|-----------|------|--------|--------|------|
| eidos-trading gRPC | ✅ 已完成 | - | - | 包含 PrepareOrder |
| eidos-market gRPC | ✅ 已完成 | - | - | K线/深度/Ticker |
| eidos-risk gRPC | ✅ 已完成 | - | - | 订单/提现风控 |
| Kafka Consumer | ⏳ TODO | P2 | - | eidos-api 不需要直接消费，使用 Redis Pub/Sub |
| Redis Pub/Sub | ✅ 已完成 | - | - | 公开+私有频道 |
| Nacos 服务发现 | ✅ 已完成 | - | - | 服务注册与发现 |
| EIP-712 签名验证 | ✅ 已完成 | - | - | WebSocket 认证 |
| Prometheus 监控 | ✅ 已完成 | - | - | 完整指标 |
| WebSocket Origin 验证 | ✅ 已完成 | - | - | 白名单+通配符 |
| WebSocket 私有频道认证 | ✅ 已完成 | - | - | EIP-712 签名 |
| 请求验证中间件 | ✅ 已完成 | - | - | 参数校验 |
| 增强限流中间件 | ✅ 已完成 | - | - | 分层限流 |
| Swagger API 文档 | ✅ 已完成 | - | - | OpenAPI 3.0 |

---

## 二、已完成功能详情

### 2.1 PrepareOrder 接口

**文件:** `internal/client/trading_client_prepare.go`

实现了 PrepareOrder gRPC 调用，返回 EIP-712 TypedData 供前端签名：

```go
type PrepareOrderRequest struct {
    Wallet string
    Market string
    Side   string
    Type   string
    Price  string
    Amount string
}

type PrepareOrderResponse struct {
    OrderID   string
    TypedData *EIP712TypedData
    ExpiresAt int64
}
```

### 2.2 eidos-risk gRPC 对接

**文件:** `internal/service/risk_service.go`

实现了风控服务集成：

```go
type RiskService struct {
    client  *client.RiskClient
    enabled bool
}

func (s *RiskService) CheckOrder(ctx context.Context, req *CheckOrderRequest) (*CheckOrderResult, error)
func (s *RiskService) CheckWithdrawal(ctx context.Context, req *CheckWithdrawalRequest) (*CheckWithdrawalResult, error)
```

风控检查结果包含：
- `Approved`: 是否通过
- `RejectCode`: 拒绝代码 (15000-15003)
- `RejectReason`: 拒绝原因
- `RequiresReview`: 是否需要人工审核

### 2.3 WebSocket Origin 验证

**文件:** `internal/ws/handler.go`

实现了 Origin 白名单验证，支持通配符域名：

```go
func (h *Handler) checkOrigin(r *http.Request) bool {
    if h.cfg.AllowAllOrigins {
        return true
    }
    origin := r.Header.Get("Origin")
    for _, allowed := range h.cfg.AllowedOrigins {
        if matchOrigin(origin, allowed) {
            return true
        }
    }
    return false
}

// 支持 *.example.com 通配符模式
func matchOrigin(origin, pattern string) bool
```

配置示例：
```yaml
websocket:
  allowed_origins:
    - "localhost"
    - "localhost:3000"
    - "*.eidos.exchange"
  allow_all_origins: false  # 生产环境设为 false
```

### 2.4 WebSocket 私有频道认证

**文件:** `internal/ws/client.go`

实现了私有频道 EIP-712 签名认证：

```go
// 认证消息格式
{
    "type": "auth",
    "params": {
        "wallet": "0x1234...",
        "timestamp": 1234567890000,
        "signature": "0x..."
    }
}

// 认证成功后可订阅私有频道
{
    "type": "subscribe",
    "channel": "orders"  // 或 "balances"
}
```

签名消息格式：
```
Eidos Exchange WebSocket Authentication
Wallet: {wallet}
Timestamp: {timestamp}
```

### 2.5 请求验证中间件

**文件:** `internal/middleware/validator.go`

实现了多种验证中间件：

```go
// 市场参数验证
func ValidateMarketParam() gin.HandlerFunc

// 分页参数验证
func ValidatePagination(cfg *ValidatorConfig) gin.HandlerFunc

// 时间范围验证
func ValidateTimeRange(cfg *ValidatorConfig) gin.HandlerFunc

// K线间隔验证
func ValidateInterval() gin.HandlerFunc
```

### 2.6 增强限流中间件

**文件:** `internal/middleware/ratelimit_enhanced.go`

实现了分层限流：

```go
// 限流层级
var (
    TierDefault     = &RateLimitTier{Name: "default", OrdersPerSec: 10, QueriesPerSec: 100, ...}
    TierVIP         = &RateLimitTier{Name: "vip", OrdersPerSec: 50, QueriesPerSec: 500, ...}
    TierMarketMaker = &RateLimitTier{Name: "market_maker", OrdersPerSec: 200, QueriesPerSec: 2000, ...}
)

// 按钱包层级限流
func RateLimitByWalletTier(sw *ratelimit.SlidingWindow, resolver TierResolver, category string) gin.HandlerFunc

// 自适应限流（根据系统负载调整）
type AdaptiveRateLimit struct { ... }
```

**文件:** `internal/ratelimit/concurrency.go`

实现了并发请求限制：

```go
type ConcurrencyLimiter struct { ... }

func (l *ConcurrencyLimiter) Acquire(ctx context.Context, key string) (bool, error)
func (l *ConcurrencyLimiter) Release(ctx context.Context, key string) error
```

### 2.7 Swagger API 文档

**文件:** `docs/swagger.yaml`

完整的 OpenAPI 3.0 规范，包含：

- 所有 REST API 端点
- 请求/响应模型
- 认证方式 (EIP-712)
- 错误代码定义
- 限流说明

### 2.8 错误代码文档

**文件:** `docs/error_codes.md`

完整的错误代码参考：

| 范围 | 分类 |
|------|------|
| 10xxx | 认证与通用错误 |
| 11xxx | 订单错误 |
| 12xxx | 资产错误 |
| 13xxx | 市场错误 |
| 14xxx | 交易错误 |
| 15xxx | 风控错误 |
| 20xxx | 系统错误 |

### 2.9 WebSocket 文档

**文件:** `docs/websocket.md`

完整的 WebSocket API 文档，包含：

- 连接方式
- 消息格式
- 认证流程
- 公开频道 (ticker, depth, kline, trades)
- 私有频道 (orders, balances)
- 错误代码
- 限流说明
- 心跳机制

---

## 三、配置更新

### 3.1 风控配置

```yaml
risk:
  enabled: true           # 是否启用风控服务
  fail_open: false        # 风控服务不可用时是否放行（false=拒绝）
```

### 3.2 WebSocket 配置

```yaml
websocket:
  allowed_origins:
    - "localhost"
    - "*.eidos.exchange"
  allow_all_origins: false
  auth_timeout: 30
  enable_private_auth: true
```

---

## 四、架构图

```
                                    ┌─────────────┐
                                    │  Frontend   │
                                    └──────┬──────┘
                                           │
                           ┌───────────────┼───────────────┐
                           │               │               │
                    ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
                    │  REST API   │ │  WebSocket  │ │  /metrics   │
                    └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
                           │               │               │
                    ┌──────▼───────────────▼───────────────▼──────┐
                    │                 eidos-api                    │
                    │  ┌─────────────────────────────────────────┐ │
                    │  │           Middleware Layer              │ │
                    │  │  - Auth (EIP-712)                       │ │
                    │  │  - RateLimit (Sliding Window)           │ │
                    │  │  - Validator                            │ │
                    │  │  - Metrics                              │ │
                    │  └─────────────────────────────────────────┘ │
                    │  ┌─────────────────────────────────────────┐ │
                    │  │            Service Layer                │ │
                    │  │  - OrderService (+ RiskService)         │ │
                    │  │  - BalanceService                       │ │
                    │  │  - WithdrawalService (+ RiskService)    │ │
                    │  │  - MarketService                        │ │
                    │  │  - TradeService                         │ │
                    │  └─────────────────────────────────────────┘ │
                    └──────────────────────┬───────────────────────┘
                                           │
            ┌──────────────────────────────┼──────────────────────────────┐
            │                              │                              │
     ┌──────▼──────┐               ┌───────▼───────┐              ┌──────▼──────┐
     │eidos-trading│               │ eidos-market  │              │ eidos-risk  │
     │   (gRPC)    │               │    (gRPC)     │              │   (gRPC)    │
     └──────┬──────┘               └───────┬───────┘              └─────────────┘
            │                              │
            │                       ┌──────▼──────┐
            │                       │    Redis    │
            │                       │  Pub/Sub    │
            │                       └──────┬──────┘
            │                              │
            └──────────────────────────────┘
                     (Private channel updates via Redis)
```

---

## 五、待办事项优先级

### P0 - 必须完成 (上线阻塞)

- [x] eidos-market gRPC 对接
- [x] Redis Pub/Sub 订阅
- [x] EIP-712 签名验证 (WebSocket)
- [x] WebSocket Origin 验证
- [x] WebSocket 私有频道认证
- [x] eidos-risk gRPC 对接
- [x] 请求验证中间件
- [x] Swagger API 文档

### P1 - 重要 (上线后优化)

- [x] Nacos 服务发现
- [x] Prometheus 监控
- [ ] 测试覆盖率提升到 75%
- [ ] 性能压测与优化

### P2 - 可选 (后续迭代)

- [ ] Kafka Consumer 实现 (当前使用 Redis Pub/Sub)
- [ ] 请求日志采样
- [ ] 灰度发布支持
- [ ] A/B 测试支持

---

## 六、测试覆盖率目标

| 模块 | 当前覆盖率 | 目标覆盖率 |
|------|-----------|-----------|
| middleware | ~60% | 80% |
| handler | ~50% | 75% |
| client | ~30% | 70% |
| ws | ~40% | 70% |
| cache | ~80% | 85% |
| ratelimit | ~70% | 80% |
| service | ~50% | 75% |
| **总体** | **~45%** | **≥75%** |

---

## 七、更新日志

### 2026-01-20

- ✅ 实现 PrepareOrder 接口 (`internal/client/trading_client_prepare.go`)
- ✅ 实现 eidos-risk gRPC 对接 (`internal/service/risk_service.go`)
- ✅ 实现 WebSocket Origin 验证 (`internal/ws/handler.go`)
- ✅ 实现 WebSocket 私有频道认证 (`internal/ws/client.go`)
- ✅ 实现请求验证中间件 (`internal/middleware/validator.go`)
- ✅ 实现增强限流中间件 (`internal/middleware/ratelimit_enhanced.go`)
- ✅ 实现并发请求限制 (`internal/ratelimit/concurrency.go`)
- ✅ 添加 Swagger API 文档 (`docs/swagger.yaml`)
- ✅ 添加 WebSocket 文档 (`docs/websocket.md`)
- ✅ 添加错误代码文档 (`docs/error_codes.md`)
- ✅ 更新配置支持 (`internal/config/config.go`, `config/config.yaml`)
- ✅ 集成风控服务到应用启动流程 (`internal/app/app.go`)
