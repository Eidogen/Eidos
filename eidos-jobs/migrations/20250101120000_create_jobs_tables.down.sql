-- eidos-jobs 数据库回滚脚本
-- 版本: 20250101120000
-- 描述: 删除定时任务服务核心表

DROP TABLE IF EXISTS jobs_reconciliation_checkpoints;
DROP TABLE IF EXISTS jobs_reconciliation_records;
DROP TABLE IF EXISTS jobs_statistics;
DROP TABLE IF EXISTS jobs_archive_progress;
DROP TABLE IF EXISTS jobs_executions;
