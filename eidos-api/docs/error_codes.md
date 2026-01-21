# Eidos Exchange API Error Codes

## Error Response Format

```json
{
  "code": 10003,
  "message": "INVALID_PARAMS",
  "data": null
}
```

## Error Code Categories

| Range | Category |
|-------|----------|
| 10xxx | Authentication & General |
| 11xxx | Order Errors |
| 12xxx | Asset Errors |
| 13xxx | Market Errors |
| 14xxx | Trade Errors |
| 15xxx | Risk Control Errors |
| 20xxx | System Errors |

## Authentication Errors (10xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 10001 | INVALID_SIGNATURE | 401 | EIP-712 signature verification failed |
| 10002 | SIGNATURE_EXPIRED | 401 | Signature timestamp outside tolerance window |
| 10003 | INVALID_PARAMS | 400 | Request parameters validation failed |
| 10004 | UNAUTHORIZED | 401 | Authentication required |
| 10005 | FORBIDDEN | 403 | Access denied |
| 10006 | SIGNATURE_REPLAY | 401 | Signature has been used before |
| 10007 | INVALID_TIMESTAMP | 400 | Invalid timestamp format |
| 10008 | MISSING_AUTH_HEADER | 401 | Authorization header missing |
| 10009 | INVALID_AUTH_FORMAT | 401 | Invalid Authorization header format |
| 10010 | INVALID_WALLET_ADDRESS | 400 | Invalid Ethereum wallet address format |

## Order Errors (11xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 11001 | ORDER_NOT_FOUND | 404 | Order does not exist |
| 11002 | ORDER_ALREADY_CANCELLED | 400 | Order has already been cancelled |
| 11003 | ORDER_ALREADY_FILLED | 400 | Order has already been fully filled |
| 11004 | INVALID_PRICE | 400 | Invalid price format or value |
| 11005 | INVALID_AMOUNT | 400 | Invalid amount format or value |
| 11006 | PRICE_TOO_HIGH | 400 | Price exceeds maximum allowed |
| 11007 | PRICE_TOO_LOW | 400 | Price below minimum allowed |
| 11008 | AMOUNT_TOO_SMALL | 400 | Amount below minimum order size |
| 11009 | AMOUNT_TOO_LARGE | 400 | Amount exceeds maximum order size |
| 11010 | NOTIONAL_TOO_SMALL | 400 | Order value below minimum notional |
| 11011 | ORDER_EXPIRED | 400 | Order has expired |
| 11012 | DUPLICATE_ORDER | 409 | Order with same ID already exists |
| 11013 | ORDER_NOT_CANCELLABLE | 400 | Order cannot be cancelled in current state |
| 11014 | INVALID_ORDER_SIDE | 400 | Invalid order side (must be buy or sell) |
| 11015 | INVALID_ORDER_TYPE | 400 | Invalid order type (must be limit or market) |
| 11016 | INVALID_TIME_IN_FORCE | 400 | Invalid time in force value |

## Asset Errors (12xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 12001 | INSUFFICIENT_BALANCE | 400 | Insufficient balance for operation |
| 12002 | WITHDRAW_LIMIT_EXCEEDED | 400 | Withdrawal limit exceeded |
| 12003 | TOKEN_NOT_SUPPORTED | 400 | Token not supported |
| 12004 | WITHDRAW_AMOUNT_TOO_SMALL | 400 | Withdrawal amount below minimum |
| 12005 | WITHDRAW_NOT_CANCELLABLE | 400 | Withdrawal cannot be cancelled |
| 12006 | WITHDRAW_NOT_FOUND | 404 | Withdrawal not found |
| 12007 | DEPOSIT_NOT_FOUND | 404 | Deposit not found |
| 12008 | INVALID_WITHDRAW_ADDRESS | 400 | Invalid withdrawal address |

## Market Errors (13xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 13001 | MARKET_NOT_FOUND | 404 | Trading pair not found |
| 13002 | MARKET_SUSPENDED | 400 | Trading pair is suspended |
| 13003 | INVALID_MARKET | 400 | Invalid market symbol format |

## Trade Errors (14xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 14001 | TRADE_NOT_FOUND | 404 | Trade not found |

## Risk Control Errors (15xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 15000 | RISK_REJECTED | 400 | Rejected by risk control |
| 15001 | RISK_REASON | 400 | Custom risk rejection message |
| 15002 | MANUAL_REVIEW_REQUIRED | 202 | Operation requires manual review |
| 15003 | WITHDRAWAL_REJECTED | 400 | Withdrawal rejected by risk control |

## System Errors (20xxx)

| Code | Message | HTTP Status | Description |
|------|---------|-------------|-------------|
| 20001 | RATE_LIMIT_EXCEEDED | 429 | Too many requests |
| 20002 | SERVICE_UNAVAILABLE | 503 | Service temporarily unavailable |
| 20003 | INTERNAL_ERROR | 500 | Internal server error |
| 20004 | TIMEOUT | 504 | Request timeout |
| 20005 | UPSTREAM_ERROR | 502 | Upstream service error |
| 20006 | NOT_IMPLEMENTED | 501 | Feature not implemented |

## Rate Limit Response

When rate limited, the response includes additional information:

```json
{
  "code": 20001,
  "message": "RATE_LIMIT_EXCEEDED",
  "data": {
    "limit": 100,
    "window": "1s",
    "retry_after": 1
  }
}
```

Response Headers:
- `X-RateLimit-Limit`: Maximum requests allowed
- `X-RateLimit-Remaining`: Remaining requests in window
- `X-RateLimit-Reset`: Unix timestamp when limit resets
- `Retry-After`: Seconds until limit resets
