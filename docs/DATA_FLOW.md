# Eidos Trading System - Data Flow Documentation

This document describes the data flow between services in the Eidos trading system.

## Table of Contents

- [System Overview](#system-overview)
- [Order Flow](#order-flow)
- [Deposit Flow](#deposit-flow)
- [Withdrawal Flow](#withdrawal-flow)
- [Settlement Flow](#settlement-flow)
- [Market Data Flow](#market-data-flow)
- [Kafka Topics](#kafka-topics)
- [Message Formats](#message-formats)

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              External Clients                                │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                                    ▼
                          ┌─────────────────┐
                          │    eidos-api    │
                          │  (API Gateway)  │
                          └────────┬────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  eidos-trading  │     │ eidos-matching  │     │  eidos-market   │
│ (Order/Account) │     │    (Engine)     │     │  (Market Data)  │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │              ┌────────┴────────┐              │
         │              │     Kafka       │              │
         │              │  Message Bus    │              │
         │              └────────┬────────┘              │
         │                       │                       │
┌────────┴────────┐     ┌────────┴────────┐     ┌────────┴────────┐
│   eidos-chain   │     │   eidos-risk    │     │   eidos-jobs    │
│  (Settlement)   │     │ (Risk Control)  │     │   (Scheduler)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Order Flow

The order flow handles the complete lifecycle of a trading order from creation to settlement.

### Flow Diagram

```
┌──────────┐      ┌─────────────┐      ┌──────────────┐      ┌──────────────┐
│  Client  │─────►│  eidos-api  │─────►│eidos-trading │─────►│ eidos-risk   │
│          │      │   (REST)    │      │   (gRPC)     │      │   (gRPC)     │
└──────────┘      └─────────────┘      └──────┬───────┘      └──────────────┘
                                              │
                                              │ Kafka: orders
                                              ▼
                                       ┌──────────────┐
                                       │eidos-matching│
                                       │   (Engine)   │
                                       └──────┬───────┘
                                              │
              ┌───────────────────────────────┼───────────────────────────────┐
              │ Kafka: trade-results          │ Kafka: orderbook-updates      │
              ▼                               ▼                               │
       ┌──────────────┐              ┌──────────────┐              ┌──────────┴───┐
       │eidos-trading │              │ eidos-market │              │  WebSocket   │
       │  (Update)    │              │  (K-lines)   │              │   Clients    │
       └──────┬───────┘              └──────────────┘              └──────────────┘
              │
              │ Kafka: settlements
              ▼
       ┌──────────────┐
       │ eidos-chain  │
       │ (Settlement) │
       └──────────────┘
```

### Step-by-Step Process

1. **Order Submission** (Client -> API -> Trading)
   - Client sends order via REST API
   - `eidos-api` validates signature and rate limits
   - `eidos-api` calls `eidos-trading` via gRPC

2. **Order Validation** (Trading -> Risk)
   - `eidos-trading` calls `eidos-risk` for risk check
   - Risk service validates: balance, limits, blacklists
   - If rejected, return error to client

3. **Balance Freeze** (Trading)
   - `eidos-trading` freezes required balance
   - Creates order record in PostgreSQL
   - Publishes order to Kafka `orders` topic

4. **Order Matching** (Kafka -> Matching)
   - `eidos-matching` consumes from `orders` topic
   - Matches against orderbook
   - Publishes results to `trade-results` and `orderbook-updates`

5. **Trade Processing** (Trading)
   - `eidos-trading` consumes `trade-results`
   - Updates order status
   - Adjusts frozen/available balances
   - Creates settlement batch

6. **On-Chain Settlement** (Chain)
   - `eidos-chain` consumes settlements
   - Submits transactions to blockchain
   - Confirms settlement on-chain

### Kafka Topics Used

| Topic | Producer | Consumer | Description |
|-------|----------|----------|-------------|
| orders | trading | matching | New orders |
| cancel-requests | trading | matching | Cancel requests |
| trade-results | matching | trading, market | Trade executions |
| order-updates | trading | api | Order status changes |
| orderbook-updates | matching | market, api | Depth changes |
| settlements | trading | chain | Settlement batches |
| settlement-confirmed | chain | trading | Settlement confirmations |

## Deposit Flow

Deposits are detected from the blockchain and credited to user accounts.

### Flow Diagram

```
┌────────────┐      ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│ Blockchain │─────►│ eidos-chain  │─────►│    Kafka     │─────►│eidos-trading │
│            │      │  (Indexer)   │      │  deposits    │      │  (Credit)    │
└────────────┘      └──────────────┘      └──────────────┘      └──────────────┘
```

### Step-by-Step Process

1. **Event Detection** (Chain Indexer)
   - `eidos-chain` monitors blockchain for Deposit events
   - Detects transfer to vault contract
   - Extracts: wallet, token, amount, tx_hash

2. **Confirmation Tracking** (Chain)
   - Tracks block confirmations
   - Waits for required confirmations (e.g., 12 blocks)
   - Publishes to `deposits` topic with status updates

3. **Balance Credit** (Trading)
   - `eidos-trading` consumes `deposits` topic
   - Creates deposit record
   - Credits settled balance to user account
   - Publishes `balance-updates` notification

### Status Transitions

```
DETECTED -> PENDING -> CONFIRMED -> CREDITED
                          |
                          v
                       FAILED (reorg detected)
```

### Kafka Topics Used

| Topic | Producer | Consumer | Description |
|-------|----------|----------|-------------|
| deposits | chain | trading | Detected deposits |
| deposit-confirmed | chain | trading | Confirmed deposits |
| balance-updates | trading | api | Balance notifications |

## Withdrawal Flow

Users can withdraw their settled balance to the blockchain.

### Flow Diagram

```
┌──────────┐     ┌───────────┐     ┌──────────────┐     ┌──────────────┐
│  Client  │────►│ eidos-api │────►│eidos-trading │────►│ eidos-risk   │
│          │     │   (REST)  │     │   (gRPC)     │     │  (Validate)  │
└──────────┘     └───────────┘     └──────┬───────┘     └──────────────┘
                                          │
                                          │ Kafka: withdrawals
                                          ▼
                                   ┌──────────────┐
                                   │ eidos-chain  │
                                   │  (Execute)   │
                                   └──────┬───────┘
                                          │
                                          │ Blockchain TX
                                          ▼
                                   ┌──────────────┐
                                   │  Blockchain  │
                                   └──────────────┘
```

### Step-by-Step Process

1. **Request Submission** (Client -> API -> Trading)
   - Client signs withdrawal request
   - `eidos-api` validates signature
   - `eidos-trading` receives request

2. **Risk Validation** (Trading -> Risk)
   - Checks: withdrawal limits, daily limits, AML
   - May require manual review for large amounts

3. **Balance Freeze** (Trading)
   - Deducts from settled balance
   - Adds to pending withdrawal
   - Creates withdrawal record
   - Publishes to `withdrawals` topic

4. **On-Chain Execution** (Chain)
   - `eidos-chain` consumes withdrawal request
   - Signs and submits transaction
   - Monitors for confirmation

5. **Confirmation** (Chain -> Trading)
   - `eidos-chain` publishes status updates
   - `eidos-trading` updates withdrawal status
   - Notifies user via WebSocket

### Status Transitions

```
PENDING -> PROCESSING -> SUBMITTED -> CONFIRMED
    |          |             |
    v          v             v
CANCELLED   REJECTED      FAILED
```

### Kafka Topics Used

| Topic | Producer | Consumer | Description |
|-------|----------|----------|-------------|
| withdrawals | trading | chain | Withdrawal requests |
| withdrawal-submitted | chain | trading | TX submitted |
| withdrawal-confirmed | chain | trading | TX confirmed |
| withdrawal-status | chain | trading, api | Status updates |

## Settlement Flow

Trades are settled on-chain in batches for efficiency.

### Flow Diagram

```
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│eidos-trading │─────►│    Kafka     │─────►│ eidos-chain  │
│ (Batch)      │      │ settlements  │      │  (Execute)   │
└──────────────┘      └──────────────┘      └──────┬───────┘
                                                   │
                                                   ▼
                                            ┌──────────────┐
                                            │  Blockchain  │
                                            │  (Contract)  │
                                            └──────┬───────┘
                                                   │
                                                   ▼
                                            ┌──────────────┐
                                            │    Kafka     │
                                            │ confirmed    │
                                            └──────┬───────┘
                                                   │
                                                   ▼
                                            ┌──────────────┐
                                            │eidos-trading │
                                            │   (Update)   │
                                            └──────────────┘
```

### Batching Logic

- Trades are accumulated until batch threshold
- Batch triggers: size limit (100 trades) or time limit (5 min)
- Each batch becomes one blockchain transaction

### Settlement Contract

The smart contract atomically:
1. Transfers base token from seller to buyer
2. Transfers quote token from buyer to seller
3. Collects fees to fee wallet

### Status Transitions

```
MATCHED_OFFCHAIN -> PENDING -> SUBMITTED -> SETTLED
                       |           |
                       v           v
                   RETRYING     FAILED -> ROLLED_BACK
```

## Market Data Flow

Market data is aggregated and distributed to clients.

### Flow Diagram

```
┌──────────────┐                           ┌──────────────┐
│eidos-matching│──── orderbook-updates ───►│ eidos-market │
│              │                           │              │
│              │──── trade-results ───────►│  (Aggregate) │
└──────────────┘                           └──────┬───────┘
                                                  │
                    ┌─────────────────────────────┼─────────────────┐
                    │                             │                 │
                    ▼                             ▼                 ▼
             ┌──────────────┐           ┌──────────────┐    ┌──────────────┐
             │    Redis     │           │ TimescaleDB  │    │    Kafka     │
             │   (Cache)    │           │  (K-lines)   │    │ market-stats │
             └──────────────┘           └──────────────┘    └──────────────┘
                    │
                    ▼
             ┌──────────────┐
             │  eidos-api   │
             │  (WebSocket) │
             └──────────────┘
```

### Data Types

1. **Orderbook Depth**
   - Real-time bid/ask levels
   - Cached in Redis
   - Pushed via WebSocket

2. **Recent Trades**
   - Last N trades
   - Stored in Redis list
   - Pushed via WebSocket

3. **Tickers (24h)**
   - Aggregated stats
   - Updated every trade
   - Cached in Redis

4. **K-lines (Candlesticks)**
   - OHLCV data
   - Stored in TimescaleDB
   - Multiple intervals: 1m, 5m, 15m, 30m, 1h, 4h, 1d, 1w

### Redis Keys

| Key Pattern | Description |
|-------------|-------------|
| `orderbook:{market}` | Current orderbook snapshot |
| `depth:{market}:{level}` | Aggregated depth |
| `ticker:{market}` | 24h ticker |
| `trades:{market}` | Recent trades list |
| `kline:{market}:{interval}:latest` | Latest K-line |

## Kafka Topics

### Complete Topic List

| Topic | Partitions | Producers | Consumers | Description |
|-------|------------|-----------|-----------|-------------|
| orders | 16 | trading | matching | New orders |
| cancel-requests | 8 | trading | matching | Cancel requests |
| order-accepted | 16 | matching | trading | Order accepted |
| order-cancelled | 8 | matching | trading | Order cancelled |
| order-rejected | 8 | matching | trading | Order rejected |
| order-updates | 16 | trading | api | Order status |
| trade-results | 16 | matching | trading, market | Trades |
| orderbook-updates | 16 | matching | market, api | Depth changes |
| balance-updates | 8 | trading | api | Balance changes |
| deposits | 4 | chain | trading | Deposits |
| deposit-confirmed | 4 | chain | trading | Confirmed |
| withdrawals | 4 | trading | chain | Withdrawals |
| withdrawal-status | 4 | chain | trading | Status |
| settlements | 8 | trading | chain | Batches |
| settlement-confirmed | 4 | chain | trading | Confirmed |
| risk-alerts | 4 | risk | admin | Alerts |
| notifications | 8 | various | api | Notifications |
| market-stats | 4 | market | cache | Statistics |

### Partitioning Strategy

- **orders**: Partitioned by `market` for ordering within same market
- **trade-results**: Partitioned by `market` for sequential processing
- **balance-updates**: Partitioned by `wallet` for user-level ordering
- **settlements**: Partitioned by `batch_id` for parallel processing

## Message Formats

All Kafka messages use Protocol Buffers for serialization.
See `proto/common/kafka.proto` for complete message definitions.

### Example: TradeResultMessage

```protobuf
message TradeResultMessage {
    string trade_id = 1;
    string market = 2;
    string maker_order_id = 3;
    string taker_order_id = 4;
    string maker_wallet = 5;
    string taker_wallet = 6;
    OrderSide taker_side = 7;
    string price = 8;       // decimal string
    string amount = 9;      // decimal string
    string quote_amount = 10;
    string maker_fee = 11;
    string taker_fee = 12;
    bool maker_filled = 14;
    bool taker_filled = 15;
    int64 matched_at = 18;  // Unix ms
    uint64 sequence = 19;
}
```

### Example: OrderbookUpdateMessage

```protobuf
message OrderbookUpdateMessage {
    string market = 1;
    uint64 sequence = 2;
    repeated PriceLevelUpdate bids = 3;
    repeated PriceLevelUpdate asks = 4;
    int64 timestamp = 5;
}

message PriceLevelUpdate {
    string price = 1;
    string amount = 2;  // "0" = remove level
    int32 order_count = 3;
}
```

## Monitoring Data Flow

### Health Check Script

Run the data flow verification script:

```bash
./scripts/verify_data_flow.sh
```

This checks:
- Kafka topic existence and partitions
- Consumer group status and lag
- Redis key patterns
- Database table row counts

### Integration Tests

Run integration tests to verify data flow:

```bash
make test-order-flow      # Order lifecycle
make test-deposit-flow    # Deposit processing
make test-withdrawal-flow # Withdrawal processing
make test-market-flow     # Market data
```

### Grafana Dashboards

Monitor data flow metrics in Grafana:
- Kafka consumer lag
- Message throughput
- Processing latency
- Error rates
