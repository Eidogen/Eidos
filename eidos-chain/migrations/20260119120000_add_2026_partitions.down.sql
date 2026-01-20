-- Eidos Chain Service Database Rollback
-- Version: 002
-- Description: Remove 2026 partitions

-- Drop 2026 partitions for deposit_records
DROP TABLE IF EXISTS chain_deposit_records_2026_12;
DROP TABLE IF EXISTS chain_deposit_records_2026_11;
DROP TABLE IF EXISTS chain_deposit_records_2026_10;
DROP TABLE IF EXISTS chain_deposit_records_2026_09;
DROP TABLE IF EXISTS chain_deposit_records_2026_08;
DROP TABLE IF EXISTS chain_deposit_records_2026_07;
DROP TABLE IF EXISTS chain_deposit_records_2026_06;
DROP TABLE IF EXISTS chain_deposit_records_2026_05;
DROP TABLE IF EXISTS chain_deposit_records_2026_04;
DROP TABLE IF EXISTS chain_deposit_records_2026_03;
DROP TABLE IF EXISTS chain_deposit_records_2026_02;
DROP TABLE IF EXISTS chain_deposit_records_2026_01;

-- Drop 2026 partitions for events
DROP TABLE IF EXISTS chain_events_2026_12;
DROP TABLE IF EXISTS chain_events_2026_11;
DROP TABLE IF EXISTS chain_events_2026_10;
DROP TABLE IF EXISTS chain_events_2026_09;
DROP TABLE IF EXISTS chain_events_2026_08;
DROP TABLE IF EXISTS chain_events_2026_07;
DROP TABLE IF EXISTS chain_events_2026_06;
DROP TABLE IF EXISTS chain_events_2026_05;
DROP TABLE IF EXISTS chain_events_2026_04;
DROP TABLE IF EXISTS chain_events_2026_03;
DROP TABLE IF EXISTS chain_events_2026_02;
DROP TABLE IF EXISTS chain_events_2026_01;
