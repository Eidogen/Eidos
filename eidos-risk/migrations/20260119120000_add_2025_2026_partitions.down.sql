-- eidos-risk 数据库回滚脚本
-- 版本: 002
-- 描述: 删除 2025 年剩余分区和 2026 年分区

-- 2026 年分区
DROP TABLE IF EXISTS risk_events_2026_12;
DROP TABLE IF EXISTS risk_events_2026_11;
DROP TABLE IF EXISTS risk_events_2026_10;
DROP TABLE IF EXISTS risk_events_2026_09;
DROP TABLE IF EXISTS risk_events_2026_08;
DROP TABLE IF EXISTS risk_events_2026_07;
DROP TABLE IF EXISTS risk_events_2026_06;
DROP TABLE IF EXISTS risk_events_2026_05;
DROP TABLE IF EXISTS risk_events_2026_04;
DROP TABLE IF EXISTS risk_events_2026_03;
DROP TABLE IF EXISTS risk_events_2026_02;
DROP TABLE IF EXISTS risk_events_2026_01;

-- 2025 年剩余分区
DROP TABLE IF EXISTS risk_events_2025_12;
DROP TABLE IF EXISTS risk_events_2025_11;
DROP TABLE IF EXISTS risk_events_2025_10;
DROP TABLE IF EXISTS risk_events_2025_09;
DROP TABLE IF EXISTS risk_events_2025_08;
DROP TABLE IF EXISTS risk_events_2025_07;
DROP TABLE IF EXISTS risk_events_2025_06;
DROP TABLE IF EXISTS risk_events_2025_05;
DROP TABLE IF EXISTS risk_events_2025_04;

-- audit_logs 2026 年分区
DROP TABLE IF EXISTS risk_audit_logs_2026_12;
DROP TABLE IF EXISTS risk_audit_logs_2026_11;
DROP TABLE IF EXISTS risk_audit_logs_2026_10;
DROP TABLE IF EXISTS risk_audit_logs_2026_09;
DROP TABLE IF EXISTS risk_audit_logs_2026_08;
DROP TABLE IF EXISTS risk_audit_logs_2026_07;
DROP TABLE IF EXISTS risk_audit_logs_2026_06;
DROP TABLE IF EXISTS risk_audit_logs_2026_05;
DROP TABLE IF EXISTS risk_audit_logs_2026_04;
DROP TABLE IF EXISTS risk_audit_logs_2026_03;
DROP TABLE IF EXISTS risk_audit_logs_2026_02;
DROP TABLE IF EXISTS risk_audit_logs_2026_01;

-- audit_logs 2025 年剩余分区
DROP TABLE IF EXISTS risk_audit_logs_2025_12;
DROP TABLE IF EXISTS risk_audit_logs_2025_11;
DROP TABLE IF EXISTS risk_audit_logs_2025_10;
DROP TABLE IF EXISTS risk_audit_logs_2025_09;
DROP TABLE IF EXISTS risk_audit_logs_2025_08;
DROP TABLE IF EXISTS risk_audit_logs_2025_07;
DROP TABLE IF EXISTS risk_audit_logs_2025_06;
DROP TABLE IF EXISTS risk_audit_logs_2025_05;
DROP TABLE IF EXISTS risk_audit_logs_2025_04;
