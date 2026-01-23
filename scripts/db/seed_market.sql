-- Eidos Market Service - Seed Data
-- 用于开发和测试环境的行情初始数据
-- 使用: psql -h localhost -p 5433 -U eidos -d eidos_market -f scripts/db/seed_market.sql

-- ============================================
-- 行情系统种子数据 (eidos_market 数据库, TimescaleDB)
-- ============================================

-- 添加更多交易对
INSERT INTO market_markets (
    symbol, base_token, quote_token, price_decimals, size_decimals,
    min_size, min_notional, tick_size, maker_fee, taker_fee,
    status, trading_enabled, created_at, updated_at
) VALUES
    ('SOL-USDC', 'SOL', 'USDC', 2, 4, 0.01, 10, 0.01, 0.001, 0.001, 1, TRUE, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('LINK-USDC', 'LINK', 'USDC', 4, 4, 0.1, 10, 0.0001, 0.001, 0.001, 1, TRUE, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('ARB-USDC', 'ARB', 'USDC', 4, 4, 1, 10, 0.0001, 0.001, 0.001, 1, TRUE, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000)
ON CONFLICT (symbol) DO NOTHING;

-- ============================================
-- 生成模拟 K 线数据 (最近 24 小时, 1 分钟间隔)
-- ============================================
-- 注意: 需要 market_klines 表存在

DO $$
DECLARE
    base_time BIGINT;
    btc_price DECIMAL(36, 18) := 42000.00;
    eth_price DECIMAL(36, 18) := 2500.00;
    i INT;
    price_change DECIMAL(10, 4);
    open_price DECIMAL(36, 18);
    close_price DECIMAL(36, 18);
    high_price DECIMAL(36, 18);
    low_price DECIMAL(36, 18);
    volume DECIMAL(36, 18);
BEGIN
    base_time := EXTRACT(EPOCH FROM NOW() - INTERVAL '24 hours') * 1000;

    -- 生成 BTC-USDC K 线
    FOR i IN 0..1439 LOOP
        price_change := (RANDOM() - 0.5) * 100;  -- -50 到 +50
        open_price := btc_price;
        close_price := btc_price + price_change;
        high_price := GREATEST(open_price, close_price) + ABS(RANDOM() * 30);
        low_price := LEAST(open_price, close_price) - ABS(RANDOM() * 30);
        volume := 0.1 + RANDOM() * 2;  -- 0.1 到 2.1 BTC

        INSERT INTO market_klines (symbol, interval, open_time, open, high, low, close, volume, close_time, quote_volume, trade_count)
        VALUES (
            'BTC-USDC',
            '1m',
            base_time + i * 60000,
            open_price,
            high_price,
            low_price,
            close_price,
            volume,
            base_time + (i + 1) * 60000 - 1,
            volume * (open_price + close_price) / 2,
            FLOOR(10 + RANDOM() * 50)
        )
        ON CONFLICT DO NOTHING;

        btc_price := close_price;
    END LOOP;

    -- 生成 ETH-USDC K 线
    FOR i IN 0..1439 LOOP
        price_change := (RANDOM() - 0.5) * 20;  -- -10 到 +10
        open_price := eth_price;
        close_price := eth_price + price_change;
        high_price := GREATEST(open_price, close_price) + ABS(RANDOM() * 5);
        low_price := LEAST(open_price, close_price) - ABS(RANDOM() * 5);
        volume := 1 + RANDOM() * 20;  -- 1 到 21 ETH

        INSERT INTO market_klines (symbol, interval, open_time, open, high, low, close, volume, close_time, quote_volume, trade_count)
        VALUES (
            'ETH-USDC',
            '1m',
            base_time + i * 60000,
            open_price,
            high_price,
            low_price,
            close_price,
            volume,
            base_time + (i + 1) * 60000 - 1,
            volume * (open_price + close_price) / 2,
            FLOOR(10 + RANDOM() * 50)
        )
        ON CONFLICT DO NOTHING;

        eth_price := close_price;
    END LOOP;

    RAISE NOTICE 'Generated 1440 BTC-USDC and 1440 ETH-USDC 1-minute candles';
END $$;

-- ============================================
-- 打印统计信息
-- ============================================
SELECT 'Market seed data loaded!' AS message,
       (SELECT COUNT(*) FROM market_markets) AS markets,
       (SELECT COUNT(*) FROM market_klines) AS klines;
