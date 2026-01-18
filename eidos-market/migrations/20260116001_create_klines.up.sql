-- K线数据表（TimescaleDB 超表）
-- 用于存储所有周期的 K 线数据

CREATE TABLE IF NOT EXISTS eidos_market_klines (
    market          VARCHAR(20) NOT NULL,
    interval        VARCHAR(5) NOT NULL,
    open_time       BIGINT NOT NULL,
    open            DECIMAL(36, 18) NOT NULL,
    high            DECIMAL(36, 18) NOT NULL,
    low             DECIMAL(36, 18) NOT NULL,
    close           DECIMAL(36, 18) NOT NULL,
    volume          DECIMAL(36, 18) NOT NULL DEFAULT 0,
    quote_volume    DECIMAL(36, 18) NOT NULL DEFAULT 0,
    trade_count     INT NOT NULL DEFAULT 0,
    close_time      BIGINT NOT NULL,
    PRIMARY KEY (market, interval, open_time)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_klines_market_interval_time
    ON eidos_market_klines (market, interval, open_time DESC);

CREATE INDEX IF NOT EXISTS idx_klines_open_time
    ON eidos_market_klines (open_time DESC);

-- TimescaleDB 超表（如果使用 TimescaleDB）
-- 需要先安装 TimescaleDB 扩展
-- CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- 将表转换为超表，按 open_time 分区（每天一个分区）
-- SELECT create_hypertable('eidos_market_klines', 'open_time',
--     chunk_time_interval => 86400000,  -- 1天 = 86400000 毫秒
--     if_not_exists => TRUE,
--     migrate_data => TRUE
-- );

-- 设置数据保留策略（可选，保留 365 天）
-- SELECT add_retention_policy('eidos_market_klines', INTERVAL '365 days');

-- 添加压缩策略（可选，7 天后压缩）
-- ALTER TABLE eidos_market_klines SET (
--     timescaledb.compress,
--     timescaledb.compress_segmentby = 'market, interval'
-- );
-- SELECT add_compression_policy('eidos_market_klines', INTERVAL '7 days');

COMMENT ON TABLE eidos_market_klines IS 'K线数据表';
COMMENT ON COLUMN eidos_market_klines.market IS '交易对，如 BTC-USDC';
COMMENT ON COLUMN eidos_market_klines.interval IS 'K线周期：1m, 5m, 15m, 30m, 1h, 4h, 1d, 1w';
COMMENT ON COLUMN eidos_market_klines.open_time IS 'K线开盘时间（毫秒时间戳）';
COMMENT ON COLUMN eidos_market_klines.open IS '开盘价';
COMMENT ON COLUMN eidos_market_klines.high IS '最高价';
COMMENT ON COLUMN eidos_market_klines.low IS '最低价';
COMMENT ON COLUMN eidos_market_klines.close IS '收盘价';
COMMENT ON COLUMN eidos_market_klines.volume IS '成交量（Base Token）';
COMMENT ON COLUMN eidos_market_klines.quote_volume IS '成交额（Quote Token）';
COMMENT ON COLUMN eidos_market_klines.trade_count IS '成交笔数';
COMMENT ON COLUMN eidos_market_klines.close_time IS 'K线收盘时间（毫秒时间戳）';
