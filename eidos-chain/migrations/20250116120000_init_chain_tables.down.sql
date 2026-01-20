-- Eidos Chain Service Database Migration - Rollback
-- Version: 001
-- Description: Drop all chain service tables

DROP TABLE IF EXISTS chain_rpc_endpoints;
DROP TABLE IF EXISTS chain_pending_txs;
DROP TABLE IF EXISTS chain_wallet_nonces;
DROP TABLE IF EXISTS chain_reconciliation_records;
DROP TABLE IF EXISTS chain_settlement_rollback_logs;
DROP TABLE IF EXISTS chain_events;
DROP TABLE IF EXISTS chain_block_checkpoints;
DROP TABLE IF EXISTS chain_deposit_records;
DROP TABLE IF EXISTS chain_withdrawal_txs;
DROP TABLE IF EXISTS chain_settlement_batches;
