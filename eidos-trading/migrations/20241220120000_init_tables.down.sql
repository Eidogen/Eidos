-- Eidos Trading Service Database Migration (Rollback)
-- Version: 001
-- Description: Drop all tables
-- Warning: This will delete all data!

-- Drop tables in reverse order (respecting dependencies)
DROP TABLE IF EXISTS trading_used_nonces CASCADE;
DROP TABLE IF EXISTS trading_withdrawals CASCADE;
DROP TABLE IF EXISTS trading_deposits CASCADE;
DROP TABLE IF EXISTS trading_settlement_batches CASCADE;
DROP TABLE IF EXISTS trading_trades CASCADE;
DROP TABLE IF EXISTS trading_fee_accounts CASCADE;
DROP TABLE IF EXISTS trading_balance_logs CASCADE;
DROP TABLE IF EXISTS trading_balances CASCADE;
DROP TABLE IF EXISTS trading_orders CASCADE;
