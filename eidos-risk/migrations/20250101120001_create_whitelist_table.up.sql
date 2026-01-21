-- eidos-risk whitelist table migration
-- Version: 002
-- Description: Create whitelist table for VIP, market maker, and address whitelists

-- =============================================================================
-- 1. Whitelist table
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_whitelist (
    id              BIGSERIAL PRIMARY KEY,
    entry_id        VARCHAR(64) NOT NULL UNIQUE,
    wallet_address  VARCHAR(42) NOT NULL,
    target_address  VARCHAR(42),
    list_type       VARCHAR(30) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    label           VARCHAR(100),
    remark          VARCHAR(500),
    metadata        JSONB,
    effective_from  BIGINT NOT NULL,
    effective_until BIGINT,
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL,
    approved_by     VARCHAR(42),
    approved_at     BIGINT,

    CONSTRAINT uk_whitelist_wallet_target_type UNIQUE (wallet_address, target_address, list_type)
);

CREATE INDEX idx_whitelist_wallet ON risk_whitelist (wallet_address);
CREATE INDEX idx_whitelist_target ON risk_whitelist (target_address);
CREATE INDEX idx_whitelist_type ON risk_whitelist (list_type);
CREATE INDEX idx_whitelist_status ON risk_whitelist (status);
CREATE INDEX idx_whitelist_effective ON risk_whitelist (effective_from, effective_until);

COMMENT ON TABLE risk_whitelist IS 'Risk whitelist table for VIP, market maker, and address whitelists';
COMMENT ON COLUMN risk_whitelist.list_type IS 'Whitelist type: address/vip/market_maker/internal';
COMMENT ON COLUMN risk_whitelist.status IS 'Status: active/inactive/pending/expired';
COMMENT ON COLUMN risk_whitelist.target_address IS 'Target address for address whitelist, NULL for other types';
COMMENT ON COLUMN risk_whitelist.metadata IS 'Additional metadata (custom limits, rate multipliers, etc.)';

-- =============================================================================
-- 2. Add alert rules table
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_alert_rules (
    id              BIGSERIAL PRIMARY KEY,
    rule_id         VARCHAR(64) NOT NULL UNIQUE,
    name            VARCHAR(100) NOT NULL,
    alert_type      VARCHAR(50) NOT NULL,
    severity        VARCHAR(20) NOT NULL,
    condition       JSONB NOT NULL,
    throttle_count  INT DEFAULT 0,
    throttle_window INT DEFAULT 3600,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    escalate        BOOLEAN NOT NULL DEFAULT FALSE,
    suppress        BOOLEAN NOT NULL DEFAULT FALSE,
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL
);

CREATE INDEX idx_alert_rules_type ON risk_alert_rules (alert_type);
CREATE INDEX idx_alert_rules_enabled ON risk_alert_rules (enabled);

COMMENT ON TABLE risk_alert_rules IS 'Alert rule configuration table';
COMMENT ON COLUMN risk_alert_rules.condition IS 'Alert trigger condition in JSON format';
COMMENT ON COLUMN risk_alert_rules.throttle_count IS 'Max alerts before throttling (0 = no throttle)';
COMMENT ON COLUMN risk_alert_rules.throttle_window IS 'Throttle window in seconds';

-- =============================================================================
-- 3. Add user limits table
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_user_limits (
    id              BIGSERIAL PRIMARY KEY,
    wallet_address  VARCHAR(42) NOT NULL,
    limit_type      VARCHAR(50) NOT NULL,
    token           VARCHAR(42),
    max_value       DECIMAL(36, 18) NOT NULL,
    is_custom       BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at      BIGINT,
    created_by      VARCHAR(42),
    created_at      BIGINT NOT NULL,
    updated_by      VARCHAR(42),
    updated_at      BIGINT NOT NULL,

    CONSTRAINT uk_user_limits UNIQUE (wallet_address, limit_type, token)
);

CREATE INDEX idx_user_limits_wallet ON risk_user_limits (wallet_address);
CREATE INDEX idx_user_limits_type ON risk_user_limits (limit_type);

COMMENT ON TABLE risk_user_limits IS 'Custom user limits table';
COMMENT ON COLUMN risk_user_limits.limit_type IS 'Limit type: daily_withdraw/single_order/pending_settle/etc.';

-- =============================================================================
-- 4. Insert default alert rules
-- =============================================================================
INSERT INTO risk_alert_rules (rule_id, name, alert_type, severity, condition, throttle_count, throttle_window, enabled, escalate, created_at, updated_at) VALUES
('alert_large_withdrawal', 'Large Withdrawal Alert', 'LARGE_WITHDRAWAL', 'warning',
 '{"field": "amount", "operator": "gte", "value": "10000"}',
 5, 3600, true, true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('alert_rate_limit', 'Rate Limit Exceeded', 'RATE_LIMIT_EXCEEDED', 'warning',
 '{"field": "count", "operator": "gte", "value": 100}',
 10, 300, true, false, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('alert_blacklist_hit', 'Blacklist Hit', 'BLACKLIST_HIT', 'critical',
 '{"field": "blacklisted", "operator": "eq", "value": true}',
 0, 0, true, false, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('alert_price_deviation', 'Price Deviation Alert', 'PRICE_DEVIATION', 'warning',
 '{"field": "deviation_percent", "operator": "gte", "value": 5}',
 20, 600, true, true, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000),

('alert_suspicious_activity', 'Suspicious Activity', 'SUSPICIOUS_ACTIVITY', 'critical',
 '{"field": "risk_score", "operator": "gte", "value": 80}',
 3, 3600, true, false, EXTRACT(EPOCH FROM NOW()) * 1000, EXTRACT(EPOCH FROM NOW()) * 1000);

-- =============================================================================
-- 5. Add more partitions for 2025
-- =============================================================================
CREATE TABLE IF NOT EXISTS risk_events_2025_04 PARTITION OF risk_events
    FOR VALUES FROM (1743465600000) TO (1746057600000);
CREATE TABLE IF NOT EXISTS risk_events_2025_05 PARTITION OF risk_events
    FOR VALUES FROM (1746057600000) TO (1748736000000);
CREATE TABLE IF NOT EXISTS risk_events_2025_06 PARTITION OF risk_events
    FOR VALUES FROM (1748736000000) TO (1751328000000);

CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_04 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1743465600000) TO (1746057600000);
CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_05 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1746057600000) TO (1748736000000);
CREATE TABLE IF NOT EXISTS risk_audit_logs_2025_06 PARTITION OF risk_audit_logs
    FOR VALUES FROM (1748736000000) TO (1751328000000);
