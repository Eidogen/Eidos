-- Eidos Trading Service Database Migration
-- Version: 002
-- Description: Rollback outbox_messages table
-- Created: 2024

DROP TABLE IF EXISTS outbox_messages;
