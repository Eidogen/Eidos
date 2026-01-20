-- eidos-risk 数据库迁移脚本
-- 版本: 001
-- 描述: 创建风控服务核心表

-- =============================================================================
-- 1. 风控规则表
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_rules (
    id              BIGSERIAL PRIMARY KEY,
    rule_id         VARCHAR(64) NOT NULL UNIQUE,
    rule_type       VARCHAR(50) NOT NULL,
    market          VARCHAR(20),
    description     VARCHAR(500),
    config          JSONB NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL
);

CREATE INDEX idx_risk_rules_type ON risk_rules (rule_type);
CREATE INDEX idx_risk_rules_market ON risk_rules (market);
CREATE INDEX idx_risk_rules_enabled ON risk_rules (enabled);

COMMENT ON TABLE risk_rules IS '风控规则配置表';
COMMENT ON COLUMN risk_rules.rule_type IS '规则类型: price_deviation/rate_limit/amount_limit/self_trade/blacklist/withdraw_limit';
COMMENT ON COLUMN risk_rules.market IS '适用交易对，NULL表示全局规则';
COMMENT ON COLUMN risk_rules.config IS '规则配置JSON，包含阈值等参数';

-- =============================================================================
-- 2. 规则版本历史表
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_rule_versions (
    id              BIGSERIAL PRIMARY KEY,
    rule_id         VARCHAR(64) NOT NULL,
    version         INT NOT NULL,
    config_snapshot JSONB NOT NULL,
    enabled         BOOLEAN NOT NULL,
    change_type     VARCHAR(20) NOT NULL,
    change_reason   VARCHAR(500),
    changed_by      VARCHAR(42) NOT NULL,
    changed_at      BIGINT NOT NULL,
    effective_at    BIGINT NOT NULL,

    CONSTRAINT uk_rule_version UNIQUE (rule_id, version)
);

CREATE INDEX idx_rule_versions_rule_id ON risk_rule_versions (rule_id);
CREATE INDEX idx_rule_versions_effective ON risk_rule_versions (effective_at);

COMMENT ON TABLE risk_rule_versions IS '风控规则版本历史表';
COMMENT ON COLUMN risk_rule_versions.change_type IS '变更类型: CREATE/UPDATE/ENABLE/DISABLE';

-- =============================================================================
-- 3. 黑名单表
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_blacklist (
    id              BIGSERIAL PRIMARY KEY,
    wallet_address  VARCHAR(42) NOT NULL,
    list_type       VARCHAR(20) NOT NULL,
    reason          VARCHAR(500) NOT NULL,
    source          VARCHAR(50) NOT NULL,
    effective_from  BIGINT NOT NULL,
    effective_until BIGINT,
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL,

    CONSTRAINT uk_blacklist_wallet_type UNIQUE (wallet_address, list_type)
);

CREATE INDEX idx_blacklist_wallet ON risk_blacklist (wallet_address);
CREATE INDEX idx_blacklist_status ON risk_blacklist (status);
CREATE INDEX idx_blacklist_type ON risk_blacklist (list_type);

COMMENT ON TABLE risk_blacklist IS '风控黑名单表';
COMMENT ON COLUMN risk_blacklist.list_type IS '黑名单类型: trade/withdraw/full';
COMMENT ON COLUMN risk_blacklist.source IS '来源: manual/auto/external';
COMMENT ON COLUMN risk_blacklist.status IS '状态: active/expired/removed';

-- =============================================================================
-- 4. 风险事件表 (按月分区)
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_events (
    id              BIGSERIAL,
    event_id        VARCHAR(64) NOT NULL,
    event_type      VARCHAR(50) NOT NULL,
    risk_level      VARCHAR(20) NOT NULL,
    wallet_address  VARCHAR(42) NOT NULL,
    ref_type        VARCHAR(20),
    ref_id          VARCHAR(64),
    details         JSONB NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    resolved_by     VARCHAR(42),
    resolved_at     BIGINT,
    resolution      VARCHAR(500),
    created_at      BIGINT NOT NULL,

    PRIMARY KEY (created_at, id)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_risk_events_event_id ON risk_events (event_id);
CREATE INDEX idx_risk_events_wallet ON risk_events (wallet_address, created_at DESC);
CREATE INDEX idx_risk_events_level_status ON risk_events (risk_level, status) WHERE status = 'pending';
CREATE INDEX idx_risk_events_type ON risk_events (event_type, created_at DESC);

COMMENT ON TABLE risk_events IS '风险事件记录表(按月分区)';
COMMENT ON COLUMN risk_events.event_type IS '事件类型: PRICE_DEVIATION/RATE_LIMIT/SELF_TRADE/WASH_TRADE';
COMMENT ON COLUMN risk_events.risk_level IS '风险级别: LOW/MEDIUM/HIGH/CRITICAL';
COMMENT ON COLUMN risk_events.status IS '状态: pending/resolved/ignored';

-- 创建初始分区 (2025年1月-3月)
CREATE TABLE IF NOT EXISTS risk_events_2025_01 PARTITION OF risk_events
    FOR VALUES FROM (1735689600000) TO (1738368000000);
CREATE TABLE IF NOT EXISTS risk_events_2025_02 PARTITION OF risk_events
    FOR VALUES FROM (1738368000000) TO (1740787200000);
CREATE TABLE IF NOT EXISTS risk_events_2025_03 PARTITION OF risk_events
    FOR VALUES FROM (1740787200000) TO (1743465600000);

-- =============================================================================
-- 5. 风控审计日志表 (按月分区)
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_audit_logs (
    id              BIGSERIAL,
    action          VARCHAR(50) NOT NULL,
    wallet_address  VARCHAR(42),
    request         JSONB NOT NULL,
    result          VARCHAR(20) NOT NULL,
    reason          VARCHAR(500),
    duration_ms     INT NOT NULL,
    created_at      BIGINT NOT NULL,

    PRIMARY KEY (created_at, id)
) PARTITION BY RANGE (created_at);

CREATE INDEX idx_audit_logs_wallet_time ON risk_audit_logs (wallet_address, created_at DESC);
CREATE INDEX idx_audit_logs_result ON risk_audit_logs (result) WHERE result = 'REJECTED';

COMMENT ON TABLE risk_audit_logs IS '风控审计日志表(按月分区)';
COMMENT ON COLUMN risk_audit_logs.action IS '操作类型: CHECK_ORDER/CHECK_WITHDRAW/ADD_BLACKLIST';
COMMENT ON COLUMN risk_audit_logs.result IS '结果: ALLOWED/REJECTED/PENDING';

-- 创建初始分区 (2025年1月-3月)
CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_01 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1735689600000) TO (1738368000000);
CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_02 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1738368000000) TO (1740787200000);
CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_03 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1740787200000) TO (1743465600000);

-- =============================================================================
-- 6. 提现审核表
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_withdrawal_reviews (
    id              BIGSERIAL PRIMARY KEY,
    review_id       VARCHAR(64) NOT NULL UNIQUE,
    withdrawal_id   VARCHAR(64) NOT NULL UNIQUE,
    wallet_address  VARCHAR(42) NOT NULL,
    token           VARCHAR(42) NOT NULL,
    amount          DECIMAL(36, 18) NOT NULL,
    to_address      VARCHAR(42) NOT NULL,
    risk_score      INT NOT NULL,
    risk_factors    JSONB NOT NULL,
    auto_decision   VARCHAR(20),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewer        VARCHAR(42),
    review_comment  VARCHAR(500),
    reviewed_at     BIGINT,
    created_at      BIGINT NOT NULL,
    expires_at      BIGINT NOT NULL
);

CREATE INDEX idx_withdrawal_reviews_wallet ON risk_withdrawal_reviews (wallet_address);
CREATE INDEX idx_withdrawal_reviews_status ON risk_withdrawal_reviews (status);
CREATE INDEX idx_withdrawal_reviews_expires ON risk_withdrawal_reviews (expires_at) WHERE status = 'pending';

COMMENT ON TABLE risk_withdrawal_reviews IS '提现风控审核表';
COMMENT ON COLUMN risk_withdrawal_reviews.auto_decision IS '自动决策: AUTO_APPROVE/MANUAL_REVIEW/AUTO_REJECT';
COMMENT ON COLUMN risk_withdrawal_reviews.status IS '状态: pending/approved/rejected';

-- =============================================================================
-- 7. 插入默认风控规则
-- =============================================================================
INSERT INTO risk_rules (rule_id, rule_type, market, description, config, enabled, created_at, updated_at) VALUES
-- 价格偏离规则
('rule_price_deviation_global', 'price_deviation', NULL, '全局价格偏离检查',
 '{"warning_threshold": 0.05, "reject_threshold": 0.10, "market_order_threshold": 0.03}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

-- 频率限制规则
('rule_rate_limit_order', 'rate_limit', NULL, '下单频率限制',
 '{"per_second": 10, "per_minute": 200}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('rule_rate_limit_cancel', 'rate_limit', NULL, '取消订单频率限制',
 '{"per_second": 20, "per_minute": 500}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('rule_rate_limit_withdraw', 'rate_limit', NULL, '提现频率限制',
 '{"per_hour": 10, "per_day": 50}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

-- 金额限制规则
('rule_amount_limit_order', 'amount_limit', NULL, '订单金额限制',
 '{"min_value": "10", "max_value": "100000"}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('rule_amount_limit_withdraw', 'withdraw_limit', NULL, '提现金额限制',
 '{"single_max": "50000", "daily_max": "500000", "large_threshold": "10000"}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

-- 自成交规则
('rule_self_trade', 'self_trade', NULL, '自成交检测',
 '{"enabled": true, "action": "reject"}',
 true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000);
