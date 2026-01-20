-- Eidos Trading Service Database Migration
-- Version: 002
-- Description: Rollback trading_outbox_messages table
-- Created: 2024

DROP TABLE IF EXISTS trading_outbox_messages;
