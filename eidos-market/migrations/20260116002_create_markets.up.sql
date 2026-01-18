-- 交易对配置表

CREATE TABLE IF NOT EXISTS eidos_market_markets (
    id              SERIAL PRIMARY KEY,
    symbol          VARCHAR(20) NOT NULL UNIQUE,
    base_token      VARCHAR(10) NOT NULL,
    quote_token     VARCHAR(10) NOT NULL,
    price_decimals  SMALLINT NOT NULL DEFAULT 2,
    size_decimals   SMALLINT NOT NULL DEFAULT 6,
    min_size        DECIMAL(36, 18) NOT NULL,
    max_size        DECIMAL(36, 18),
    min_notional    DECIMAL(36, 18) NOT NULL,
    tick_size       DECIMAL(36, 18) NOT NULL,
    maker_fee       DECIMAL(10, 6) NOT NULL DEFAULT 0.001,
    taker_fee       DECIMAL(10, 6) NOT NULL DEFAULT 0.001,
    status          SMALLINT NOT NULL DEFAULT 1,
    trading_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_markets_status ON eidos_market_markets (status);
CREATE INDEX IF NOT EXISTS idx_markets_symbol ON eidos_market_markets (symbol);

-- 插入默认交易对
INSERT INTO eidos_market_markets (
    symbol, base_token, quote_token, price_decimals, size_decimals,
    min_size, min_notional, tick_size, maker_fee, taker_fee,
    status, trading_enabled, created_at, updated_at
) VALUES
    ('BTC-USDC', 'BTC', 'USDC', 2, 6, 0.0001, 10, 0.01, 0.001, 0.001, 1, TRUE, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('ETH-USDC', 'ETH', 'USDC', 2, 6, 0.001, 10, 0.01, 0.001, 0.001, 1, TRUE, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000)
ON CONFLICT (symbol) DO NOTHING;

COMMENT ON TABLE eidos_market_markets IS '交易对配置表';
COMMENT ON COLUMN eidos_market_markets.symbol IS '交易对符号，如 BTC-USDC';
COMMENT ON COLUMN eidos_market_markets.base_token IS '基础代币';
COMMENT ON COLUMN eidos_market_markets.quote_token IS '计价代币';
COMMENT ON COLUMN eidos_market_markets.price_decimals IS '价格小数位数';
COMMENT ON COLUMN eidos_market_markets.size_decimals IS '数量小数位数';
COMMENT ON COLUMN eidos_market_markets.min_size IS '最小下单数量';
COMMENT ON COLUMN eidos_market_markets.max_size IS '最大下单数量';
COMMENT ON COLUMN eidos_market_markets.min_notional IS '最小下单金额';
COMMENT ON COLUMN eidos_market_markets.tick_size IS '价格最小变动单位';
COMMENT ON COLUMN eidos_market_markets.maker_fee IS 'Maker 手续费率';
COMMENT ON COLUMN eidos_market_markets.taker_fee IS 'Taker 手续费率';
COMMENT ON COLUMN eidos_market_markets.status IS '状态：0=未激活, 1=已激活, 2=暂停交易';
COMMENT ON COLUMN eidos_market_markets.trading_enabled IS '是否启用交易';
