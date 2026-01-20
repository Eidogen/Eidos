-- eidos-jobs 数据库迁移脚本
-- 版本: 20250101120000
-- 描述: 创建定时任务服务核心表

-- =============================================================================
-- 1. 任务执行记录表
-- =============================================================================
CREATE TABLE IF NOT EXISTS jobs_executions (
    id              BIGSERIAL PRIMARY KEY,
    job_name        VARCHAR(100) NOT NULL,
    status          VARCHAR(20) NOT NULL,
    started_at      BIGINT NOT NULL,
    finished_at     BIGINT,
    duration_ms     INT,
    error_message   TEXT,
    result          JSONB,
    created_at      BIGINT NOT NULL
);

CREATE INDEX idx_jobs_executions_name ON jobs_executions (job_name);
CREATE INDEX idx_jobs_executions_status ON jobs_executions (status);
CREATE INDEX idx_jobs_executions_started ON jobs_executions (started_at DESC);

COMMENT ON TABLE jobs_executions IS '定时任务执行记录表';
COMMENT ON COLUMN jobs_executions.job_name IS '任务名称';
COMMENT ON COLUMN jobs_executions.status IS '执行状态: pending/running/success/failed/skipped';

-- =============================================================================
-- 2. 归档进度表
-- =============================================================================
CREATE TABLE IF NOT EXISTS jobs_archive_progress (
    id              BIGSERIAL PRIMARY KEY,
    table_name      VARCHAR(50) NOT NULL UNIQUE,
    last_id         BIGINT NOT NULL DEFAULT 0,
    started_at      BIGINT,
    updated_at      BIGINT NOT NULL
);

CREATE INDEX idx_archive_progress_table ON jobs_archive_progress (table_name);

COMMENT ON TABLE jobs_archive_progress IS '数据归档进度表';
COMMENT ON COLUMN jobs_archive_progress.table_name IS '源表名称';
COMMENT ON COLUMN jobs_archive_progress.last_id IS '最后处理的ID';

-- =============================================================================
-- 3. 统计数据表
-- =============================================================================
CREATE TABLE IF NOT EXISTS jobs_statistics (
    id              BIGSERIAL PRIMARY KEY,
    stat_type       VARCHAR(50) NOT NULL,
    stat_date       DATE NOT NULL,
    stat_hour       INT,
    market          VARCHAR(32),
    metric_name     VARCHAR(100) NOT NULL,
    metric_value    DECIMAL(36,18) NOT NULL,
    created_at      BIGINT NOT NULL
);

CREATE INDEX idx_statistics_type_date ON jobs_statistics (stat_type, stat_date);
CREATE INDEX idx_statistics_market ON jobs_statistics (market);
CREATE INDEX idx_statistics_metric ON jobs_statistics (metric_name);
CREATE UNIQUE INDEX uk_statistics ON jobs_statistics (stat_type, stat_date, COALESCE(stat_hour, -1), COALESCE(market, ''), metric_name);

COMMENT ON TABLE jobs_statistics IS '业务统计数据表';
COMMENT ON COLUMN jobs_statistics.stat_type IS '统计类型: hourly/daily';
COMMENT ON COLUMN jobs_statistics.stat_hour IS '小时 0-23，仅 hourly 类型';

-- =============================================================================
-- 4. 对账记录表
-- =============================================================================
CREATE TABLE IF NOT EXISTS jobs_reconciliation_records (
    id              BIGSERIAL PRIMARY KEY,
    job_type        VARCHAR(50) NOT NULL,
    reconcile_type  VARCHAR(20) NOT NULL,
    status          VARCHAR(20) NOT NULL,
    wallet_address  VARCHAR(42),
    token           VARCHAR(42),
    offchain_value  DECIMAL(36,18),
    onchain_value   DECIMAL(36,18),
    diff_value      DECIMAL(36,18),
    details         JSONB,
    created_at      BIGINT NOT NULL
);

CREATE INDEX idx_reconciliation_job_type ON jobs_reconciliation_records (job_type);
CREATE INDEX idx_reconciliation_status ON jobs_reconciliation_records (status);
CREATE INDEX idx_reconciliation_wallet ON jobs_reconciliation_records (wallet_address);
CREATE INDEX idx_reconciliation_created ON jobs_reconciliation_records (created_at DESC);

COMMENT ON TABLE jobs_reconciliation_records IS '对账记录表';
COMMENT ON COLUMN jobs_reconciliation_records.job_type IS '对账任务类型: balance/deposit/withdrawal/settlement';
COMMENT ON COLUMN jobs_reconciliation_records.reconcile_type IS '对账类型: incremental/full';
COMMENT ON COLUMN jobs_reconciliation_records.status IS '对账结果: matched/mismatched';

-- =============================================================================
-- 5. 对账检查点表
-- =============================================================================
CREATE TABLE IF NOT EXISTS jobs_reconciliation_checkpoints (
    id              BIGSERIAL PRIMARY KEY,
    job_type        VARCHAR(50) NOT NULL UNIQUE,
    last_time       BIGINT NOT NULL,
    last_block      BIGINT,
    updated_at      BIGINT NOT NULL
);

CREATE INDEX idx_checkpoint_job_type ON jobs_reconciliation_checkpoints (job_type);

COMMENT ON TABLE jobs_reconciliation_checkpoints IS '对账检查点表';
COMMENT ON COLUMN jobs_reconciliation_checkpoints.job_type IS '对账任务类型';
COMMENT ON COLUMN jobs_reconciliation_checkpoints.last_time IS '最后处理时间戳';
COMMENT ON COLUMN jobs_reconciliation_checkpoints.last_block IS '最后处理的区块号';
