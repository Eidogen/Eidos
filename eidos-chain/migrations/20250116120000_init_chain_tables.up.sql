-- Eidos Chain Service Database Migration
-- Version: 001
-- Description: Initial tables for chain settlement and indexer

-- ============================================
-- 结算批次表 (settlement_batches)
-- 记录批量结算交易
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_settlement_batches (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        VARCHAR(64) NOT NULL,
    trade_count     INT NOT NULL,
    trade_ids       TEXT NOT NULL,                          -- JSON 数组格式
    chain_id        INT NOT NULL,
    tx_hash         VARCHAR(66),
    block_number    BIGINT,
    gas_used        BIGINT,
    gas_price       VARCHAR(36),
    status          SMALLINT NOT NULL DEFAULT 0,            -- 0=待提交, 1=已提交, 2=已确认, 3=失败
    error_message   VARCHAR(500),
    retry_count     INT NOT NULL DEFAULT 0,
    submitted_at    BIGINT,
    confirmed_at    BIGINT,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_settlement_batches_batch_id ON eidos_chain_settlement_batches(batch_id);
CREATE INDEX IF NOT EXISTS idx_settlement_batches_status ON eidos_chain_settlement_batches(status);
CREATE INDEX IF NOT EXISTS idx_settlement_batches_created_at ON eidos_chain_settlement_batches(created_at);

-- ============================================
-- 提现交易表 (withdrawal_txs)
-- 记录提现链上交易
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_withdrawal_txs (
    id              BIGSERIAL PRIMARY KEY,
    withdraw_id     VARCHAR(64) NOT NULL,
    wallet_address  VARCHAR(42) NOT NULL,
    to_address      VARCHAR(42) NOT NULL,
    token           VARCHAR(20) NOT NULL,
    token_address   VARCHAR(42) NOT NULL,
    amount          DECIMAL(36, 18) NOT NULL,
    chain_id        INT NOT NULL,
    tx_hash         VARCHAR(66),
    block_number    BIGINT,
    gas_used        BIGINT,
    status          SMALLINT NOT NULL DEFAULT 0,            -- 0=待提交, 1=已提交, 2=已确认, 3=失败
    error_message   VARCHAR(500),
    retry_count     INT NOT NULL DEFAULT 0,
    submitted_at    BIGINT,
    confirmed_at    BIGINT,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_withdrawal_txs_withdraw_id ON eidos_chain_withdrawal_txs(withdraw_id);
CREATE INDEX IF NOT EXISTS idx_withdrawal_txs_wallet ON eidos_chain_withdrawal_txs(wallet_address);
CREATE INDEX IF NOT EXISTS idx_withdrawal_txs_status ON eidos_chain_withdrawal_txs(status);
CREATE INDEX IF NOT EXISTS idx_withdrawal_txs_created_at ON eidos_chain_withdrawal_txs(created_at);

-- ============================================
-- 充值记录表 (deposit_records)
-- 按月分区，记录链上充值事件
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records (
    id                      BIGSERIAL,
    deposit_id              VARCHAR(64) NOT NULL,
    wallet_address          VARCHAR(42) NOT NULL,
    token                   VARCHAR(20) NOT NULL,
    token_address           VARCHAR(42) NOT NULL,
    amount                  DECIMAL(36, 18) NOT NULL,
    chain_id                INT NOT NULL,
    tx_hash                 VARCHAR(66) NOT NULL,
    block_number            BIGINT NOT NULL,
    block_hash              VARCHAR(66) NOT NULL,
    log_index               INT NOT NULL,
    confirmations           INT NOT NULL DEFAULT 0,
    required_confirmations  INT NOT NULL,
    status                  SMALLINT NOT NULL DEFAULT 0,    -- 0=待确认, 1=已确认, 2=已入账
    credited_at             BIGINT,
    created_at              BIGINT NOT NULL,
    updated_at              BIGINT NOT NULL,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS idx_deposit_records_deposit_id ON eidos_chain_deposit_records(deposit_id);
CREATE INDEX IF NOT EXISTS idx_deposit_records_tx_hash ON eidos_chain_deposit_records(tx_hash);
CREATE INDEX IF NOT EXISTS idx_deposit_records_wallet ON eidos_chain_deposit_records(wallet_address, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deposit_records_status ON eidos_chain_deposit_records(status) WHERE status < 2;
CREATE INDEX IF NOT EXISTS idx_deposit_records_block ON eidos_chain_deposit_records(chain_id, block_number);

-- 创建 2025 年分区
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_01 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1735689600000) TO (1738368000000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_02 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1738368000000) TO (1740787200000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_03 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1740787200000) TO (1743465600000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_04 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1743465600000) TO (1746057600000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_05 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1746057600000) TO (1748736000000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_06 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1748736000000) TO (1751328000000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_07 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1751328000000) TO (1754006400000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_08 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1754006400000) TO (1756684800000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_09 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1756684800000) TO (1759276800000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_10 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1759276800000) TO (1761955200000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_11 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1761955200000) TO (1764547200000);
CREATE TABLE IF NOT EXISTS eidos_chain_deposit_records_2025_12 PARTITION OF eidos_chain_deposit_records
    FOR VALUES FROM (1764547200000) TO (1767225600000);

-- ============================================
-- 区块检查点表 (block_checkpoints)
-- 记录索引器进度
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_block_checkpoints (
    id              BIGSERIAL PRIMARY KEY,
    chain_id        INT NOT NULL,
    block_number    BIGINT NOT NULL,
    block_hash      VARCHAR(66) NOT NULL,
    processed_at    BIGINT NOT NULL,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_block_checkpoints_chain_id ON eidos_chain_block_checkpoints(chain_id);

-- ============================================
-- 链上事件表 (events)
-- 按月分区，记录所有链上事件
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_events (
    id              BIGSERIAL,
    chain_id        INT NOT NULL,
    block_number    BIGINT NOT NULL,
    tx_hash         VARCHAR(66) NOT NULL,
    log_index       INT NOT NULL,
    event_type      VARCHAR(50) NOT NULL,                   -- Deposit/Withdraw/Settlement
    event_data      JSONB NOT NULL,
    processed       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS idx_events_block ON eidos_chain_events(chain_id, block_number);
CREATE INDEX IF NOT EXISTS idx_events_type ON eidos_chain_events(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_tx ON eidos_chain_events(tx_hash);
CREATE INDEX IF NOT EXISTS idx_events_unprocessed ON eidos_chain_events(processed) WHERE processed = FALSE;

-- 创建 2025 年分区
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_01 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1735689600000) TO (1738368000000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_02 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1738368000000) TO (1740787200000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_03 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1740787200000) TO (1743465600000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_04 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1743465600000) TO (1746057600000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_05 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1746057600000) TO (1748736000000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_06 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1748736000000) TO (1751328000000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_07 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1751328000000) TO (1754006400000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_08 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1754006400000) TO (1756684800000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_09 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1756684800000) TO (1759276800000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_10 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1759276800000) TO (1761955200000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_11 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1761955200000) TO (1764547200000);
CREATE TABLE IF NOT EXISTS eidos_chain_events_2025_12 PARTITION OF eidos_chain_events
    FOR VALUES FROM (1764547200000) TO (1767225600000);

-- ============================================
-- 结算回滚审计表 (settlement_rollback_logs)
-- 记录结算回滚操作
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_settlement_rollback_logs (
    id                  BIGSERIAL PRIMARY KEY,
    batch_id            VARCHAR(64) NOT NULL,
    trade_count         INT NOT NULL,
    affected_users      INT NOT NULL,
    total_base_amount   DECIMAL(36, 18) NOT NULL,
    total_quote_amount  DECIMAL(36, 18) NOT NULL,
    failure_reason      VARCHAR(500) NOT NULL,
    rollback_reason     VARCHAR(500) NOT NULL,
    operator            VARCHAR(42) NOT NULL,
    approved_by         VARCHAR(42) NOT NULL,
    rollback_at         BIGINT NOT NULL,
    original_tx_hash    VARCHAR(66),
    created_at          BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rollback_logs_batch ON eidos_chain_settlement_rollback_logs(batch_id);
CREATE INDEX IF NOT EXISTS idx_rollback_logs_time ON eidos_chain_settlement_rollback_logs(rollback_at);

-- ============================================
-- 对账记录表 (reconciliation_records)
-- 记录链上链下对账结果
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_reconciliation_records (
    id                  BIGSERIAL PRIMARY KEY,
    wallet_address      VARCHAR(42) NOT NULL,
    token               VARCHAR(20) NOT NULL,
    on_chain_balance    DECIMAL(36, 18) NOT NULL,
    off_chain_settled   DECIMAL(36, 18) NOT NULL,
    off_chain_available DECIMAL(36, 18) NOT NULL,
    off_chain_frozen    DECIMAL(36, 18) NOT NULL,
    pending_settle      DECIMAL(36, 18) NOT NULL,
    difference          DECIMAL(36, 18) NOT NULL,
    status              VARCHAR(20) NOT NULL,               -- OK/DISCREPANCY/RESOLVED/IGNORED
    resolution          VARCHAR(500),
    resolved_by         VARCHAR(42),
    resolved_at         BIGINT,
    checked_at          BIGINT NOT NULL,
    created_at          BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_wallet ON eidos_chain_reconciliation_records(wallet_address);
CREATE INDEX IF NOT EXISTS idx_reconciliation_status ON eidos_chain_reconciliation_records(status);
CREATE INDEX IF NOT EXISTS idx_reconciliation_checked ON eidos_chain_reconciliation_records(checked_at);

-- ============================================
-- 热钱包 Nonce 表 (wallet_nonces)
-- 管理热钱包 nonce
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_wallet_nonces (
    id              BIGSERIAL PRIMARY KEY,
    wallet_address  VARCHAR(42) NOT NULL,
    chain_id        INT NOT NULL,
    current_nonce   BIGINT NOT NULL DEFAULT 0,
    synced_at       BIGINT NOT NULL,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_wallet_nonces_wallet_chain ON eidos_chain_wallet_nonces(wallet_address, chain_id);

-- ============================================
-- 待确认交易表 (pending_txs)
-- 跟踪已提交但未确认的交易
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_pending_txs (
    id              BIGSERIAL PRIMARY KEY,
    tx_hash         VARCHAR(66) NOT NULL,
    tx_type         VARCHAR(20) NOT NULL,                   -- settlement/withdrawal
    ref_id          VARCHAR(64) NOT NULL,                   -- batch_id 或 withdraw_id
    wallet_address  VARCHAR(42) NOT NULL,
    chain_id        INT NOT NULL,
    nonce           BIGINT NOT NULL,
    gas_price       VARCHAR(36) NOT NULL,
    gas_limit       BIGINT NOT NULL,
    submitted_at    BIGINT NOT NULL,
    timeout_at      BIGINT NOT NULL,
    status          SMALLINT NOT NULL DEFAULT 0,            -- 0=pending, 1=confirmed, 2=failed, 3=replaced
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_pending_txs_tx_hash ON eidos_chain_pending_txs(tx_hash);
CREATE INDEX IF NOT EXISTS idx_pending_txs_status ON eidos_chain_pending_txs(status);
CREATE INDEX IF NOT EXISTS idx_pending_txs_ref ON eidos_chain_pending_txs(tx_type, ref_id);
CREATE INDEX IF NOT EXISTS idx_pending_txs_timeout ON eidos_chain_pending_txs(timeout_at) WHERE status = 0;

-- ============================================
-- RPC 节点健康状态表 (rpc_endpoints)
-- ============================================
CREATE TABLE IF NOT EXISTS eidos_chain_rpc_endpoints (
    id              BIGSERIAL PRIMARY KEY,
    chain_id        INT NOT NULL,
    endpoint_url    VARCHAR(255) NOT NULL,
    is_healthy      BOOLEAN NOT NULL DEFAULT TRUE,
    latency_ms      INT,
    last_block      BIGINT,
    error_count     INT NOT NULL DEFAULT 0,
    last_check_at   BIGINT NOT NULL,
    created_at      BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_rpc_endpoints_chain_url ON eidos_chain_rpc_endpoints(chain_id, endpoint_url);
CREATE INDEX IF NOT EXISTS idx_rpc_endpoints_healthy ON eidos_chain_rpc_endpoints(chain_id, is_healthy);

-- ============================================
-- 注释说明
-- ============================================
COMMENT ON TABLE eidos_chain_settlement_batches IS '结算批次表 - 记录批量链上结算';
COMMENT ON TABLE eidos_chain_withdrawal_txs IS '提现交易表 - 记录链上提现交易';
COMMENT ON TABLE eidos_chain_deposit_records IS '充值记录表 - 按月分区，记录链上充值事件';
COMMENT ON TABLE eidos_chain_block_checkpoints IS '区块检查点表 - 记录索引器扫描进度';
COMMENT ON TABLE eidos_chain_events IS '链上事件表 - 按月分区，记录所有解析的链上事件';
COMMENT ON TABLE eidos_chain_settlement_rollback_logs IS '结算回滚审计表 - 记录回滚操作用于审计';
COMMENT ON TABLE eidos_chain_reconciliation_records IS '对账记录表 - 链上链下余额对账';
COMMENT ON TABLE eidos_chain_wallet_nonces IS '热钱包 Nonce 表 - 管理交易 nonce';
COMMENT ON TABLE eidos_chain_pending_txs IS '待确认交易表 - 跟踪已提交未确认的交易';
COMMENT ON TABLE eidos_chain_rpc_endpoints IS 'RPC 节点表 - 管理 RPC 端点健康状态';
