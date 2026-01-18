-- eidos-risk 数据库回滚脚本
-- 版本: 001
-- 描述: 删除风控服务核心表

-- 删除分区表
DROP TABLE IF EXISTS eidos_risk_audit_logs_2025_01;
DROP TABLE IF EXISTS eidos_risk_audit_logs_2025_02;
DROP TABLE IF EXISTS eidos_risk_audit_logs_2025_03;
DROP TABLE IF EXISTS eidos_risk_events_2025_01;
DROP TABLE IF EXISTS eidos_risk_events_2025_02;
DROP TABLE IF EXISTS eidos_risk_events_2025_03;

-- 删除主表
DROP TABLE IF EXISTS eidos_risk_withdrawal_reviews;
DROP TABLE IF EXISTS eidos_risk_audit_logs;
DROP TABLE IF EXISTS eidos_risk_events;
DROP TABLE IF EXISTS eidos_risk_blacklist;
DROP TABLE IF EXISTS eidos_risk_rule_versions;
DROP TABLE IF EXISTS eidos_risk_rules;
