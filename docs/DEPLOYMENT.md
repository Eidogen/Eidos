# Eidos Trading System - Deployment Guide

This document describes how to deploy and run the Eidos trading system.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Prerequisites](#prerequisites)
- [Service Dependencies](#service-dependencies)
- [Deployment Order](#deployment-order)
- [Local Development](#local-development)
- [Docker Deployment](#docker-deployment)
- [Production Deployment](#production-deployment)
- [Configuration](#configuration)
- [Health Checks](#health-checks)
- [Troubleshooting](#troubleshooting)

## Architecture Overview

The Eidos trading system consists of the following components:

### Infrastructure Components

| Component | Port | Description |
|-----------|------|-------------|
| PostgreSQL | 5432 | Main database for orders, balances, etc. |
| TimescaleDB | 5433 | Time-series database for market data |
| Redis | 6379 | Cache, rate limiting, real-time data |
| Kafka | 9092/29092 | Message queue for async communication |
| Nacos | 8848 | Service registry and configuration |
| Prometheus | 9090 | Metrics collection |
| Grafana | 3000 | Metrics visualization |

### Application Services

| Service | gRPC Port | HTTP Port | Description |
|---------|-----------|-----------|-------------|
| eidos-api | - | 8080 | REST + WebSocket API Gateway |
| eidos-trading | 50051 | - | Order management, clearing, accounts |
| eidos-matching | 50052 | - | Order matching engine |
| eidos-market | 50053 | - | Market data service |
| eidos-chain | 50054 | - | On-chain settlement, indexer |
| eidos-risk | 50055 | - | Risk control |
| eidos-jobs | 50056 | - | Scheduled jobs |
| eidos-admin | - | 8088 | Admin backend |

## Prerequisites

### Software Requirements

- Docker 20.10+
- Docker Compose 2.0+
- Go 1.24+ (for local development)
- Make
- protoc (Protocol Buffers compiler)

### System Requirements

**Development:**
- CPU: 4 cores
- Memory: 8GB RAM
- Disk: 20GB free space

**Production:**
- CPU: 8+ cores per service
- Memory: 16GB+ RAM per node
- Disk: SSD with 100GB+ per database node

## Service Dependencies

```
eidos-api
  |-- eidos-trading (gRPC)
  |-- eidos-matching (gRPC)
  |-- eidos-market (gRPC)
  |-- eidos-risk (gRPC)
  |-- Redis

eidos-trading
  |-- PostgreSQL
  |-- Redis
  |-- Kafka
  |-- Nacos

eidos-matching
  |-- Redis
  |-- Kafka
  |-- Nacos

eidos-market
  |-- TimescaleDB
  |-- Redis
  |-- Kafka
  |-- Nacos

eidos-chain
  |-- PostgreSQL
  |-- Redis
  |-- Kafka
  |-- Nacos
  |-- Blockchain RPC

eidos-risk
  |-- PostgreSQL
  |-- Redis
  |-- Kafka
  |-- Nacos

eidos-jobs
  |-- PostgreSQL
  |-- Redis
  |-- Kafka
  |-- Nacos

eidos-admin
  |-- PostgreSQL
  |-- Redis
  |-- Nacos
```

## Deployment Order

### Phase 1: Infrastructure

Start infrastructure components in this order:

1. **PostgreSQL** - Main database
2. **TimescaleDB** - Market data database
3. **Redis** - Cache
4. **Kafka** - Message queue
5. **Nacos** - Service registry

```bash
# Start all infrastructure
make infra-up

# Or start individually
docker-compose up -d postgres
docker-compose up -d timescaledb
docker-compose up -d redis
docker-compose up -d kafka
docker-compose up -d nacos
```

Wait for health checks to pass:

```bash
make infra-status
```

### Phase 2: Kafka Topics

Initialize Kafka topics:

```bash
docker-compose up kafka-init
```

### Phase 3: Application Services

Start services in this order:

1. **eidos-trading** - Must start first (manages orders and accounts)
2. **eidos-matching** - Order matching engine
3. **eidos-market** - Market data service
4. **eidos-risk** - Risk control
5. **eidos-chain** - Blockchain integration
6. **eidos-jobs** - Scheduled tasks
7. **eidos-api** - API gateway (last, depends on all others)
8. **eidos-admin** - Admin panel

```bash
# Start all services
make services-up

# Or start individually
docker-compose --profile services up -d eidos-trading
docker-compose --profile services up -d eidos-matching
docker-compose --profile services up -d eidos-market
docker-compose --profile services up -d eidos-risk
docker-compose --profile services up -d eidos-chain
docker-compose --profile services up -d eidos-jobs
docker-compose --profile services up -d eidos-api
docker-compose --profile services up -d eidos-admin
```

### Phase 4: Monitoring

Start monitoring stack:

```bash
docker-compose up -d prometheus grafana
```

## Local Development

### Setup

1. Install dependencies:

```bash
make install-tools
make mod-download
```

2. Generate protobuf code:

```bash
make proto
```

3. Start infrastructure:

```bash
make infra-up
```

4. Run a service locally:

```bash
# Run trading service
make run-trading

# Run matching service
make run-matching

# Run API gateway
make run-api
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires infrastructure)
make test-integration

# Specific integration test
make test-order-flow
make test-deposit-flow
make test-market-flow
```

## Docker Deployment

### Build Images

```bash
# Build all images
make docker-build

# Build specific image
make docker-build-trading
make docker-build-api
```

### Run with Docker Compose

```bash
# Start everything
make up

# View logs
make logs

# Stop everything
make down
```

### Using Profiles

Docker Compose uses profiles to separate infrastructure from services:

```bash
# Infrastructure only
docker-compose up -d

# With services
docker-compose --profile services up -d

# With debug tools (Kafka UI)
docker-compose --profile debug up -d
```

## Production Deployment

### Kubernetes (Recommended)

For production, deploy to Kubernetes with:

1. Helm charts (in `deploy/helm/`)
2. Horizontal Pod Autoscaling
3. Pod Disruption Budgets
4. Resource limits and requests

### High Availability Setup

```
                    ┌─────────────────┐
                    │   Load Balancer │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
       ┌──────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
       │  eidos-api  │ │ eidos-api  │ │ eidos-api  │
       │   (Pod 1)   │ │  (Pod 2)   │ │  (Pod 3)   │
       └──────┬──────┘ └─────┬──────┘ └─────┬──────┘
              │              │              │
              └──────────────┼──────────────┘
                             │
       ┌─────────────────────┼─────────────────────┐
       │                     │                     │
┌──────▼──────┐       ┌──────▼──────┐       ┌──────▼──────┐
│eidos-trading│       │eidos-trading│       │eidos-trading│
│  (Pod 1)    │       │  (Pod 2)    │       │  (Pod 3)    │
└─────────────┘       └─────────────┘       └─────────────┘
```

### Database Replication

- PostgreSQL: Primary-Replica setup with streaming replication
- TimescaleDB: Primary-Replica with continuous aggregates
- Redis: Sentinel or Cluster mode

### Kafka Cluster

Production Kafka should have:
- 3+ brokers
- Replication factor: 3
- Min ISR: 2

## Configuration

### Environment Variables

Each service reads configuration from environment variables. Key variables:

**Database:**
```bash
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=eidos
POSTGRES_PASSWORD=eidos123
POSTGRES_DATABASE=eidos
```

**Redis:**
```bash
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
```

**Kafka:**
```bash
KAFKA_BROKERS=localhost:29092
```

**Nacos:**
```bash
NACOS_SERVER_ADDR=localhost:8848
NACOS_NAMESPACE=eidos-dev
```

### Configuration Files

Each service has a `config/config.yaml` file with default settings.
Environment variables override file configuration.

## Health Checks

### Service Health Endpoints

| Service | Health Check |
|---------|--------------|
| eidos-api | GET /health |
| eidos-trading | gRPC health check on :50051 |
| eidos-matching | gRPC health check on :50052 |
| eidos-market | gRPC health check on :50053 |
| eidos-chain | gRPC health check on :50054 |
| eidos-risk | gRPC health check on :50055 |
| eidos-jobs | gRPC health check on :50056 |
| eidos-admin | TCP check on :8088 |

### Running Health Checks

```bash
# Check all services
make health-check

# Check data flow
make verify-data-flow
```

## Troubleshooting

### Common Issues

**1. Services can not connect to PostgreSQL**

```bash
# Check PostgreSQL is running
docker-compose logs postgres

# Check connection
psql -h localhost -U eidos -d eidos
```

**2. Kafka topics not created**

```bash
# Run topic initialization manually
docker-compose up kafka-init

# Or create topics manually
docker exec -it eidos-kafka /opt/kafka/bin/kafka-topics.sh \
  --bootstrap-server localhost:9092 --list
```

**3. Services can not find each other**

```bash
# Check Nacos registration
curl http://localhost:8848/nacos/v1/ns/instance/list?serviceName=eidos-trading
```

**4. gRPC connection refused**

```bash
# Check if service is listening
nc -z localhost 50051

# Check service logs
docker-compose logs eidos-trading
```

### Logs

```bash
# All logs
make logs

# Specific service
docker-compose logs -f eidos-trading

# Last 100 lines
docker-compose logs --tail=100 eidos-api
```

### Metrics

Access Grafana at http://localhost:3000 (admin/admin123) for:
- Service metrics
- Database metrics
- Kafka metrics
- Custom dashboards

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug make run-trading
```

Start Kafka UI for message inspection:

```bash
make infra-kafka-ui
# Access at http://localhost:8090
```
