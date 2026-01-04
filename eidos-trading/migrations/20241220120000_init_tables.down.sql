-- Eidos Trading Service Database Migration (Rollback)
-- Version: 001
-- Description: Drop all tables
-- Warning: This will delete all data!

-- Drop tables in reverse order (respecting dependencies)
DROP TABLE IF EXISTS used_nonces CASCADE;
DROP TABLE IF EXISTS withdrawals CASCADE;
DROP TABLE IF EXISTS deposits CASCADE;
DROP TABLE IF EXISTS settlement_batches CASCADE;
DROP TABLE IF EXISTS trades CASCADE;
DROP TABLE IF EXISTS fee_accounts CASCADE;
DROP TABLE IF EXISTS balance_logs CASCADE;
DROP TABLE IF EXISTS balances CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
