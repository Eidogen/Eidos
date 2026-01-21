# Eidos

<p align="center">
  <strong>链下撮合 + 链上结算的高性能去中心化交易系统</strong>
</p>

<p align="center">
  <a href="#特性">特性</a> •
  <a href="#系统架构">架构</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#项目进度">项目进度</a> •
  <a href="#文档">文档</a>
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go" alt="Go Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
  <img src="https://img.shields.io/badge/Status-开发中-yellow" alt="Status">
</p>

> **⚠️ 注意：本项目目前处于开发阶段，正在进行服务间对接测试，暂不可用于生产环境。**

---

## 项目简介

Eidos 是一个高性能的去中心化交易系统，采用**链下撮合 + 链上结算**的混合架构。在保证资金安全的前提下，实现中心化交易所级别的交易性能。

用户资金始终托管在链上智能合约中，系统无法挪用；同时通过链下撮合引擎实现毫秒级订单处理，仅最终结算上链，大幅降低 Gas 费用。

### 为什么选择 Eidos？

| 特性 | 传统 CEX | Eidos | 纯链上 DEX |
|------|---------|-------|-----------|
| 资金安全 | 托管在交易所 | **链上合约托管** | 链上 |
| 交易速度 | 快 | **快** | 慢 |
| Gas 费用 | 无 | **低（仅结算）** | 高 |
| 用户体验 | 简单 | **简单** | 复杂 |

## 特性

- **非托管**: 用户资金存放在链上智能合约，交易所无法挪用
- **高性能**: 链下撮合引擎，毫秒级订单处理
- **低成本**: 仅最终结算上链，Gas 费用极低
- **可验证**: 所有交易结果链上可查，完全透明
- **钱包原生**: EIP-712 签名认证，无需注册，连接钱包即可交易
- **生产就绪**: 完整的微服务架构，配备监控、日志、高可用方案

## 系统架构

Eidos 由 8 个微服务组成，通过 gRPC（同步）和 Kafka（异步）通信：

```
┌─────────────────────────────────────────────────────────────────┐
│                          用户层                                  │
│            Web App  •  Mobile App  •  API 客户端                 │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     API 网关 (eidos-api)                         │
│          REST + WebSocket • EIP-712 认证 • 限流熔断               │
└─────────────────────────────┬───────────────────────────────────┘
                              │ gRPC
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         核心服务层                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │  交易服务    │  │  撮合引擎    │  │  风控服务    │             │
│  │  Trading    │──│  Matching   │──│    Risk     │             │
│  └──────┬──────┘  └──────┬──────┘  └─────────────┘             │
│         │                │                                      │
│  ┌──────▼──────┐  ┌──────▼──────┐  ┌─────────────┐             │
│  │  链上服务    │  │  行情服务    │  │  定时任务    │             │
│  │   Chain     │  │   Market    │  │    Jobs     │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
│  ┌─────────────┐                                               │
│  │  管理后台    │                                               │
│  │   Admin     │                                               │
│  └─────────────┘                                               │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       消息队列 (Kafka)                           │
│       orders • trades • orderbook • settlements • balances       │
└─────────────────────────────┬───────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         基础设施层                               │
│        PostgreSQL + TimescaleDB  •  Redis  •  Nacos             │
└─────────────────────────────────────────────────────────────────┘
```

### 服务列表

| 服务 | 端口 | 说明 |
|------|------|------|
| **eidos-api** | 8080 | REST + WebSocket 网关，EIP-712 签名验证 |
| **eidos-trading** | 50051 | 订单管理、清算、账户余额 |
| **eidos-matching** | 50052 | 内存订单簿、撮合引擎 |
| **eidos-market** | 50053 | K线、深度、Ticker 行情数据 |
| **eidos-chain** | 50054 | 链上结算、区块链事件索引 |
| **eidos-risk** | 50055 | 交易前/后风控检查 |
| **eidos-jobs** | 50056 | 定时对账、数据归档、统计汇总 |
| **eidos-admin** | 8088 | 运营管理后台 |

## 技术栈

| 层级 | 技术选型 |
|------|---------|
| **后端语言** | Go 1.22+ |
| **RPC 框架** | gRPC + Protocol Buffers |
| **HTTP 框架** | Gin |
| **消息队列** | Apache Kafka |
| **数据库** | PostgreSQL + TimescaleDB（时序数据） |
| **缓存** | Redis Cluster |
| **服务发现** | Nacos |
| **目标链** | Arbitrum (Ethereum L2) |
| **监控** | Prometheus + Grafana |
| **部署** | Docker Compose / Kubernetes |

## 项目进度

### 当前进度（v1.0 现货交易）

| 模块 | 状态 | 说明 |
|------|------|------|
| **eidos-common** | ✅ 已完成 | 公共库：EIP-712、Kafka、Redis、gRPC 等 |
| **eidos-api** | ✅ 已完成 | REST API、WebSocket 推送、签名验证 |
| **eidos-trading** | ✅ 已完成 | 订单管理、清算、账户余额、资金流水 |
| **eidos-matching** | ✅ 已完成 | 内存订单簿、价格时间优先撮合 |
| **eidos-market** | ✅ 已完成 | K线聚合、深度行情、Ticker |
| **eidos-chain** | ✅ 已完成 | 链上结算框架、事件索引 |
| **eidos-risk** | ✅ 已完成 | 风控规则引擎、审核工作流 |
| **eidos-jobs** | ✅ 已完成 | 对账任务、数据归档、统计 |
| **eidos-admin** | ✅ 已完成 | 管理后台 API |
| **智能合约** | 🚧 开发中 | 托管合约、结算合约 |
| **前端交易界面** | 📋 待开发 | Web 交易界面 |
| **集成测试** | 🚧 进行中 | 全链路测试 |

### 版本规划

| 版本 | 范围 | 核心功能 | 状态 |
|------|------|---------|------|
| **v1.0** | 现货交易 | EIP-712 签名下单、限价/市价单、撮合引擎、链上结算 | 🚧 开发中 |
| **v2.0** | 永续合约 | 杠杆交易、资金费率、强制清算、保证金管理 | 📋 规划中 |
| **v3.0** | 预测市场 | 预测市场创建、事件结算、流动性池 | 📋 规划中 |

详细规划见 [版本规划文档](docs/1-产品需求/02-版本规划.md)

## 快速开始

### 环境要求

- Go 1.22+
- Docker & Docker Compose
- Make

### 1. 克隆仓库

```bash
git clone https://github.com/Eidogen/Eidos.git
cd Eidos
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件配置你的环境
```

### 3. 启动基础设施

```bash
# 启动 PostgreSQL, Redis, Kafka, Nacos
make infra-up
```

### 4. 初始化数据库

```bash
# 运行数据库迁移
make migrate
```

### 5. 构建并运行服务

```bash
# 构建所有服务
make build

# 启动所有服务
make services-up
```

### 6. 验证安装

```bash
# 健康检查
make health-check

# 查看日志
make services-logs
```

API 网关地址：`http://localhost:8080`

### 常用命令

```bash
make build              # 构建所有服务
make test               # 运行单元测试
make test-integration   # 运行集成测试
make lint               # 代码检查
make proto              # 生成 protobuf 代码
make fmt                # 格式化代码
make clean              # 清理构建产物
make infra-up           # 启动基础设施
make infra-down         # 停止基础设施
make services-up        # 启动应用服务
make services-logs      # 查看服务日志
```

## 核心流程

### 下单流程

```
钱包签名 → eidos-api → eidos-trading → [验证 + 冻结 + 风控] → Kafka → eidos-matching
```

### 成交流程

```
eidos-matching → Kafka → eidos-trading → [解冻 + 扣款/加款 + 流水]
                           ↓
                     eidos-market (更新K线)
                           ↓
                     eidos-api (WebSocket 推送)
```

### 结算流程

```
eidos-trading → Kafka → eidos-chain → [链上结算] → 智能合约
```

## EIP-712 认证

Eidos 使用 EIP-712 类型化数据签名进行认证，无需注册，连接钱包即可交易。

```
Authorization: EIP712 {钱包地址}:{时间戳}:{签名}
```

域配置：
```json
{
  "name": "EidosExchange",
  "version": "1",
  "chainId": 42161,
  "verifyingContract": "0x..."
}
```

## 文档

### 必读文档

1. **[协议总表](docs/3-开发规范/00-协议总表.md)** - 所有配置的唯一真源
2. **[状态机规范](docs/3-开发规范/04-状态机规范.md)** - 订单/成交/结算状态流转
3. **[一致性与可靠性](docs/4-服务设计/00-设计补充-一致性与可靠性.md)** - 分布式一致性设计

### 完整文档

```
docs/
├── 1-产品需求/          # 产品需求与版本规划
├── 2-系统架构/          # 系统技术架构
├── 3-开发规范/          # 开发规范与标准
├── 4-服务设计/          # 服务详细设计（8个服务）
├── 5-前端设计/          # 前端设计
├── 6-智能合约/          # 智能合约设计
└── 7-运维部署/          # 运维与部署
```

## 技术风险与解决方案

| 风险 | 解决方案 | 文档位置 |
|------|---------|---------|
| 撮合引擎单点故障 | 热备切换 + Kafka 重放重建订单簿 | [运维部署](docs/7-运维部署/01-运维部署.md) |
| 链下/链上数据不一致 | 定时对账 + 冲突解决策略 | [链上服务](docs/4-服务设计/04-链上服务.md) |
| 签名验证 CPU 瓶颈 | 批量验证 + 结果缓存 | [交易服务](docs/4-服务设计/01-交易服务.md) |
| 跨服务事务一致性 | Outbox Pattern + 幂等处理 | [一致性与可靠性](docs/4-服务设计/00-设计补充-一致性与可靠性.md) |
| 热钱包私钥安全 | AWS KMS / HashiCorp Vault | [运维部署](docs/7-运维部署/01-运维部署.md) |

## 联系方式

- 邮箱：shang990909@gmail.com
- GitHub：[@Eidogen](https://github.com/Eidogen)

## 许可证

本项目采用 Apache License 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 致谢

本项目由 [Claude Code](https://claude.ai/code) 辅助开发。

同时感谢以下优秀的开源项目：

- [Go](https://golang.org/) - 编程语言
- [gRPC](https://grpc.io/) - RPC 框架
- [Apache Kafka](https://kafka.apache.org/) - 消息队列
- [PostgreSQL](https://www.postgresql.org/) - 数据库
- [TimescaleDB](https://www.timescale.com/) - 时序数据库扩展
- [Redis](https://redis.io/) - 缓存
- [Nacos](https://nacos.io/) - 服务发现
