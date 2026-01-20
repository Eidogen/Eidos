-- 成交记录表

CREATE TABLE IF NOT EXISTS market_trades (
    trade_id        VARCHAR(64) PRIMARY KEY,
    market          VARCHAR(20) NOT NULL,
    price           DECIMAL(36, 18) NOT NULL,
    amount          DECIMAL(36, 18) NOT NULL,
    quote_amount    DECIMAL(36, 18) NOT NULL,
    side            SMALLINT NOT NULL,
    timestamp       BIGINT NOT NULL,
    created_at      BIGINT NOT NULL
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_trades_market_timestamp
    ON market_trades (market, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_trades_timestamp
    ON market_trades (timestamp DESC);

-- TimescaleDB 超表（如果使用 TimescaleDB）
-- SELECT create_hypertable('market_trades', 'timestamp',
--     chunk_time_interval => 86400000,  -- 1天
--     if_not_exists => TRUE,
--     migrate_data => TRUE
-- );

-- 数据保留策略（保留 30 天）
-- SELECT add_retention_policy('market_trades', INTERVAL '30 days');

COMMENT ON TABLE market_trades IS '成交记录表（行情服务）';
COMMENT ON COLUMN market_trades.trade_id IS '成交 ID';
COMMENT ON COLUMN market_trades.market IS '交易对';
COMMENT ON COLUMN market_trades.price IS '成交价格';
COMMENT ON COLUMN market_trades.amount IS '成交数量（Base Token）';
COMMENT ON COLUMN market_trades.quote_amount IS '成交金额（Quote Token）';
COMMENT ON COLUMN market_trades.side IS 'Taker 方向：0=买, 1=卖';
COMMENT ON COLUMN market_trades.timestamp IS '成交时间（毫秒时间戳）';
