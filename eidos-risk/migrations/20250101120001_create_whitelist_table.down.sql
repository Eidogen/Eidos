-- Rollback: Drop whitelist and related tables

DROP TABLE IF EXISTS risk_whitelist CASCADE;
DROP TABLE IF EXISTS risk_alert_rules CASCADE;
DROP TABLE IF EXISTS risk_user_limits CASCADE;

-- Drop additional partitions
DROP TABLE IF EXISTS risk_events_2025_04 CASCADE;
DROP TABLE IF EXISTS risk_events_2025_05 CASCADE;
DROP TABLE IF EXISTS risk_events_2025_06 CASCADE;

DROP TABLE IF EXISTS risk_audit_logs_2025_04 CASCADE;
DROP TABLE IF EXISTS risk_audit_logs_2025_05 CASCADE;
DROP TABLE IF EXISTS risk_audit_logs_2025_06 CASCADE;
