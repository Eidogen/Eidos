# Eidos Exchange WebSocket API

## Connection

```
wss://api.eidos.exchange/ws
```

## Message Format

All messages are JSON formatted.

### Client Messages

```json
{
  "type": "subscribe|unsubscribe|ping|auth",
  "id": "optional-request-id",
  "channel": "ticker|depth|kline|trades|orders|balances",
  "market": "BTC-USDC",
  "params": {}
}
```

### Server Messages

```json
{
  "type": "pong|error|snapshot|update|ack|auth",
  "id": "request-id",
  "channel": "ticker",
  "market": "BTC-USDC",
  "data": {},
  "timestamp": 1234567890000,
  "code": 0,
  "message": ""
}
```

## Authentication

For private channels (orders, balances), authentication is required.

### Auth Message

```json
{
  "type": "auth",
  "id": "auth-1",
  "params": {
    "wallet": "0x1234567890123456789012345678901234567890",
    "timestamp": 1234567890000,
    "signature": "0x..."
  }
}
```

### Auth Response

```json
{
  "type": "auth",
  "id": "auth-1",
  "message": "authenticated",
  "data": {
    "wallet": "0x1234...",
    "authenticated": true
  }
}
```

### Signature Format

The signature is created by signing the following message:

```
Eidos Exchange WebSocket Authentication
Wallet: {wallet}
Timestamp: {timestamp}
```

## Channels

### Public Channels

#### Ticker

Real-time price updates for a trading pair.

```json
// Subscribe
{
  "type": "subscribe",
  "channel": "ticker",
  "market": "BTC-USDC"
}

// Update
{
  "type": "update",
  "channel": "ticker",
  "market": "BTC-USDC",
  "data": {
    "market": "BTC-USDC",
    "last_price": "50000.00",
    "high_24h": "51000.00",
    "low_24h": "49000.00",
    "volume_24h": "1234.56",
    "change_24h": "500.00",
    "change_rate": "0.01",
    "best_bid": "49999.00",
    "best_ask": "50001.00"
  },
  "timestamp": 1234567890000
}
```

#### Depth

Order book depth updates.

```json
// Subscribe
{
  "type": "subscribe",
  "channel": "depth",
  "market": "BTC-USDC",
  "params": {
    "depth": 20
  }
}

// Snapshot (on subscribe)
{
  "type": "snapshot",
  "channel": "depth",
  "market": "BTC-USDC",
  "data": {
    "market": "BTC-USDC",
    "bids": [["49999.00", "1.5"], ["49998.00", "2.0"]],
    "asks": [["50001.00", "1.2"], ["50002.00", "0.8"]]
  },
  "timestamp": 1234567890000
}

// Update
{
  "type": "update",
  "channel": "depth",
  "market": "BTC-USDC",
  "data": {
    "bids": [["49999.00", "1.8"]],
    "asks": [["50001.00", "0.0"]]
  },
  "timestamp": 1234567890000
}
```

#### K-line

Candlestick/K-line data.

```json
// Subscribe
{
  "type": "subscribe",
  "channel": "kline",
  "market": "BTC-USDC",
  "params": {
    "interval": "1m"
  }
}

// Update
{
  "type": "update",
  "channel": "kline",
  "market": "BTC-USDC",
  "data": {
    "market": "BTC-USDC",
    "interval": "1m",
    "open_time": 1234567800000,
    "open": "50000.00",
    "high": "50100.00",
    "low": "49900.00",
    "close": "50050.00",
    "volume": "12.34",
    "close_time": 1234567860000
  },
  "timestamp": 1234567890000
}
```

#### Trades

Recent trades stream.

```json
// Subscribe
{
  "type": "subscribe",
  "channel": "trades",
  "market": "BTC-USDC"
}

// Update
{
  "type": "update",
  "channel": "trades",
  "market": "BTC-USDC",
  "data": {
    "market": "BTC-USDC",
    "trade_id": "t-123456",
    "price": "50000.00",
    "amount": "0.5",
    "side": "buy",
    "timestamp": 1234567890000
  },
  "timestamp": 1234567890000
}
```

### Private Channels

Requires authentication.

#### Orders

Order updates for the authenticated user.

```json
// Subscribe (after auth)
{
  "type": "subscribe",
  "channel": "orders"
}

// Update
{
  "type": "update",
  "channel": "orders",
  "data": {
    "order_id": "o-123456",
    "market": "BTC-USDC",
    "side": "buy",
    "type": "limit",
    "price": "50000.00",
    "amount": "1.0",
    "filled_amount": "0.5",
    "remaining_amount": "0.5",
    "status": "partial",
    "updated_at": 1234567890000
  },
  "timestamp": 1234567890000
}
```

#### Balances

Balance updates for the authenticated user.

```json
// Subscribe (after auth)
{
  "type": "subscribe",
  "channel": "balances"
}

// Update
{
  "type": "update",
  "channel": "balances",
  "data": {
    "wallet": "0x1234...",
    "token": "USDC",
    "available": "10000.00",
    "locked": "500.00",
    "total": "10500.00",
    "updated_at": 1234567890000
  },
  "timestamp": 1234567890000
}
```

## Error Codes

| Code | Message | Description |
|------|---------|-------------|
| 400 | invalid message format | Malformed JSON |
| 400 | invalid channel | Unknown channel name |
| 400 | market is required | Missing market for public channels |
| 400 | invalid auth params | Missing auth parameters |
| 401 | authentication required | Auth needed for private channels |
| 401 | invalid signature | Signature verification failed |
| 401 | signature expired | Auth timestamp too old |
| 429 | max subscriptions exceeded | Too many subscriptions |

## Rate Limits

- Max connections per IP: 100/minute
- Max subscriptions per connection: 50
- Max message rate: 10/second

## Heartbeat

Send ping messages to keep connection alive:

```json
// Ping
{"type": "ping"}

// Pong (response)
{"type": "pong"}
```

The server sends WebSocket ping frames every 30 seconds. Connections will be closed if no pong is received within 10 seconds.
