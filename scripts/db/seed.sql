-- Eidos Trading System - Seed Data
-- 用于开发和测试环境的初始数据
-- 使用: psql -h localhost -U eidos -d eidos -f scripts/db/seed.sql

-- ============================================
-- 交易系统种子数据 (eidos 数据库)
-- ============================================

-- 测试钱包地址
-- Wallet 1: 0x1234567890123456789012345678901234567890 (主测试账户)
-- Wallet 2: 0xABCDEF1234567890ABCDEF1234567890ABCDEF12 (做市商账户)
-- Wallet 3: 0x9876543210987654321098765432109876543210 (普通用户账户)

-- 清理旧数据 (可选, 仅在需要重置时取消注释)
-- TRUNCATE trading_balances, trading_orders, trading_trades, trading_deposits, trading_withdrawals, trading_balance_logs, trading_used_nonces, trading_fee_accounts RESTART IDENTITY CASCADE;

-- ============================================
-- 初始化测试余额
-- ============================================
INSERT INTO trading_balances (wallet, token, settled_available, settled_frozen, pending_available, pending_frozen, pending_total, version, created_at, updated_at)
VALUES
    -- 主测试账户: 10000 USDT, 1 ETH, 0.1 BTC
    ('0x1234567890123456789012345678901234567890', 'USDT', 10000.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('0x1234567890123456789012345678901234567890', 'ETH', 1.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('0x1234567890123456789012345678901234567890', 'BTC', 0.100000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

    -- 做市商账户: 大量资金
    ('0xABCDEF1234567890ABCDEF1234567890ABCDEF12', 'USDT', 1000000.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('0xABCDEF1234567890ABCDEF1234567890ABCDEF12', 'ETH', 100.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('0xABCDEF1234567890ABCDEF1234567890ABCDEF12', 'BTC', 10.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

    -- 普通用户账户: 小额资金
    ('0x9876543210987654321098765432109876543210', 'USDT', 1000.000000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),
    ('0x9876543210987654321098765432109876543210', 'ETH', 0.500000000000000000, 0, 0, 0, 0, 1, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000)
ON CONFLICT (wallet, token) DO UPDATE SET
    settled_available = EXCLUDED.settled_available,
    updated_at = EXTRACT(EPOCH FROM NOW()) * 1000;

-- ============================================
-- 初始化手续费账户 (16个分桶)
-- ============================================
INSERT INTO trading_fee_accounts (bucket_id, token, balance, version, updated_at)
SELECT
    bucket,
    token,
    0,
    1,
    EXTRACT(EPOCH FROM NOW()) * 1000
FROM
    generate_series(0, 15) AS bucket,
    (VALUES ('USDT'), ('ETH'), ('BTC')) AS tokens(token)
ON CONFLICT (bucket_id, token) DO NOTHING;

-- ============================================
-- 模拟充值记录 (已确认)
-- ============================================
INSERT INTO trading_deposits (deposit_id, wallet, token, amount, tx_hash, log_index, block_num, status, detected_at, confirmed_at, credited_at, created_at, updated_at)
VALUES
    ('DEP-' || LPAD(FLOOR(RANDOM() * 1000000000)::TEXT, 19, '0'),
     '0x1234567890123456789012345678901234567890',
     'USDT',
     10000.000000000000000000,
     '0x' || encode(gen_random_bytes(32), 'hex'),
     0,
     1000000,
     2,  -- credited
     EXTRACT(EPOCH FROM NOW() - INTERVAL '1 hour') * 1000,
     EXTRACT(EPOCH FROM NOW() - INTERVAL '50 minutes') * 1000,
     EXTRACT(EPOCH FROM NOW() - INTERVAL '45 minutes') * 1000,
     EXTRACT(EPOCH FROM NOW() - INTERVAL '1 hour') * 1000,
     EXTRACT(EPOCH FROM NOW()) * 1000)
ON CONFLICT DO NOTHING;

-- ============================================
-- 打印统计信息
-- ============================================
DO $$
DECLARE
    balance_count INT;
    fee_account_count INT;
    deposit_count INT;
BEGIN
    SELECT COUNT(*) INTO balance_count FROM trading_balances;
    SELECT COUNT(*) INTO fee_account_count FROM trading_fee_accounts;
    SELECT COUNT(*) INTO deposit_count FROM trading_deposits;

    RAISE NOTICE '=== Seed Data Summary ===';
    RAISE NOTICE 'Balances: %', balance_count;
    RAISE NOTICE 'Fee Accounts: %', fee_account_count;
    RAISE NOTICE 'Deposits: %', deposit_count;
END $$;

SELECT 'Seed data loaded successfully!' AS message;
