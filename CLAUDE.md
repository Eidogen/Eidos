# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Eidos is a decentralized trading system using **off-chain matching + on-chain settlement** architecture. It's built with Go 1.22+ using a microservices pattern, with gRPC for service-to-service communication and Kafka for event-driven messaging.

## Architecture

### Core Services (8 services, all in Go)

| Service | gRPC Port | Description |
|---------|-----------|-------------|
| eidos-api | - (HTTP 8080) | REST + WebSocket gateway, EIP-712 signature verification |
| eidos-trading | 50051 | Orders, clearing, accounts, balance management |
| eidos-matching | 50052 | In-memory order book, matching engine |
| eidos-market | 50053 | K-line, depth, ticker data |
| eidos-chain | 50054 | On-chain settlement, blockchain event indexing |
| eidos-risk | 50055 | Pre-trade checks, post-trade monitoring |
| eidos-jobs | 50056 | Scheduled tasks (reconciliation, archiving) |
| eidos-admin | - (HTTP 8088) | Admin backend |

### Communication Patterns

- **Synchronous**: gRPC between services (via Nacos service discovery)
- **Asynchronous**: Kafka for events (orders, trades, settlements, balance-updates)
- **Real-time push**: Redis Pub/Sub → eidos-api WebSocket

### Infrastructure

- **Database**: PostgreSQL + TimescaleDB (cloud-managed RDS)
- **Cache**: Redis Cluster
- **Message Queue**: Kafka
- **Service Discovery**: Nacos

## Key Design Documents

Must-read documents (in order of priority):

1. **[协议总表](docs/3-开发规范/00-协议总表.md)** - Single source of truth for all configurations (ports, Kafka topics, EIP-712 specs)
2. **[状态机规范](docs/3-开发规范/04-状态机规范.md)** - Order/Trade/Settlement/Withdrawal state machines
3. **[一致性与可靠性](docs/4-服务设计/00-设计补充-一致性与可靠性.md)** - Distributed consistency patterns
4. **[设计规范](docs/3-开发规范/01-设计规范.md)** - Code structure, naming conventions, layered architecture

## Code Structure (per service)

```
eidos-xxx/
├── cmd/main.go              # Entry point
├── config/config.yaml       # Configuration
├── internal/
│   ├── config/              # Config loading
│   ├── handler/             # gRPC/HTTP handlers
│   ├── service/             # Business logic
│   ├── repository/          # Data access
│   ├── model/               # Data models
│   └── client/              # External service clients
├── pkg/                     # Reusable packages
├── api/proto/               # Proto definitions
├── migrations/              # DB migrations
├── Dockerfile
├── Makefile
└── go.mod
```

Shared code goes in `eidos-common/` with packages for: nacos, grpc, kafka, redis, postgres, crypto (EIP-712), decimal, errors, logger, metrics, middleware.

## Conventions

### Database
- Use BIGINT for timestamps (milliseconds)
- Use DECIMAL(36,18) for amounts/prices
- Use VARCHAR(42) for wallet addresses
- Required audit fields: `created_by`, `created_at`, `updated_by`, `updated_at`
- Migrations: `{timestamp}_{description}.{up|down}.sql`

### Proto/gRPC
- Package: `eidos.{service}.v1`
- Field names: snake_case
- Enums: `ORDER_STATUS_UNSPECIFIED = 0`

### Kafka Topics (no prefix)
- `orders`, `cancel-requests`, `trade-results`, `order-updates`
- `orderbook-updates`, `balance-updates`, `deposits`, `withdrawals`
- `settlements`, `settlement-confirmed`, `kline-1m`, `risk-alerts`

### State Enums
- Order: PENDING(0), OPEN(1), PARTIAL(2), FILLED(3), CANCELLED(4), EXPIRED(5), REJECTED(6)
- Settlement: MATCHED_OFFCHAIN(0), SETTLEMENT_PENDING(1), SETTLEMENT_SUBMITTED(2), SETTLED_ONCHAIN(3), SETTLEMENT_FAILED(4), ROLLED_BACK(5)

## EIP-712 Signature

Domain configuration (Mock mode uses zero address):
```go
{
  name: "EidosExchange",
  version: "1",
  chainId: 31337,  // dev: 31337, testnet: 421614, mainnet: 42161
  verifyingContract: "0x0000000000000000000000000000000000000000"  // zero = mock mode
}
```

Authorization header format: `Authorization: EIP712 {wallet}:{timestamp}:{signature}`

## Target Chain

Arbitrum (L2) - block confirmations: 0 (fast finality)
