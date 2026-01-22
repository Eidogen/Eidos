# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Eidos is a high-performance decentralized exchange (DEX) using **off-chain matching + on-chain settlement**. User funds are custodied in smart contracts while trading happens off-chain for speed.

## Common Commands

```bash
# Start/stop everything
make up                    # Start infrastructure + all services
make down                  # Stop everything

# Infrastructure only
make infra-up              # Start PostgreSQL, TimescaleDB, Redis, Kafka, Nacos, Jaeger
make infra-down            # Stop infrastructure (data preserved)

# Application services only
make services-up           # Start all 8 microservices
make services-down         # Stop services

# Build & Test
make build                 # Build all services
make build-api             # Build single service (replace 'api' with service name)
make test                  # Run all unit tests
make test-api              # Test single service
make test-integration      # Run integration tests

# Run single service locally
make run-api               # Run eidos-api locally (requires infra-up first)

# Proto generation
make proto                 # Generate all protobuf files

# Logs & Status
make logs                  # View all logs
make services-logs-trading # View logs for specific service
make health-check          # Run health check script
```

## Architecture

### Service Communication
- **Synchronous**: gRPC between services (eidos-api → eidos-trading → eidos-matching)
- **Asynchronous**: Kafka for events (orders, trades, settlements, balances)

### 8 Microservices

| Service | Port (gRPC/HTTP) | Role |
|---------|------------------|------|
| eidos-api | 8080 | REST/WebSocket gateway, EIP-712 auth |
| eidos-trading | 50051/8081 | Order management, clearing, balances |
| eidos-matching | 50052/8082 | In-memory orderbook, matching engine |
| eidos-market | 50053/8083 | Klines, depth, ticker data |
| eidos-chain | 50054/8084 | On-chain settlement, event indexing |
| eidos-risk | 50055/8085 | Pre/post trade risk checks |
| eidos-jobs | 50056/8086 | Scheduled tasks, reconciliation |
| eidos-admin | 8088 | Admin dashboard API |

### Module Structure

Each service follows this structure:
```
eidos-{service}/
├── cmd/main.go           # Entry point
├── config/config.yaml    # Service configuration
├── internal/
│   ├── app/app.go        # Application bootstrap
│   ├── config/           # Config structs
│   ├── handler/          # gRPC/HTTP handlers
│   ├── service/          # Business logic
│   ├── repository/       # Data access
│   └── client/           # gRPC clients to other services
└── Dockerfile
```

### Shared Code (`eidos-common/pkg/`)

| Package | Purpose |
|---------|---------|
| `config` | Unified config loading from YAML + env vars |
| `grpc` | gRPC client factory with service discovery |
| `kafka` | Producer/consumer with tracing |
| `tracing` | OpenTelemetry + Jaeger integration |
| `metrics` | Prometheus metrics |
| `discovery` | Nacos service discovery |
| `middleware` | Auth, logging, rate limiting |
| `crypto` | EIP-712 signature verification |

### Proto Definitions (`proto/`)

Each service has its own proto package under `proto/{service}/v1/`. Common types are in `proto/common/`.

## Key Technical Details

### Authentication
Uses EIP-712 typed data signatures. Auth header format:
```
Authorization: EIP712 {wallet}:{timestamp}:{signature}
```

### Go Workspace
This is a Go workspace project (`go.work`). Each service has its own `go.mod`. Run commands from respective service directories or use workspace-aware commands.

### Docker Compose Files
- `docker-compose.infra.yml` - Infrastructure (DB, cache, MQ, monitoring)
- `docker-compose.services.yml` - Application services

### Key Documentation
- `docs/3-开发规范/00-协议总表.md` - Authoritative source for all configs
- `docs/3-开发规范/04-状态机规范.md` - Order/trade/settlement state machines
