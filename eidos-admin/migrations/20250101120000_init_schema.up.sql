-- 管理员表
CREATE TABLE IF NOT EXISTS admin_admins (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(256) NOT NULL,
    email VARCHAR(128),
    phone VARCHAR(32),
    role VARCHAR(32) NOT NULL DEFAULT 'viewer',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    last_login_at BIGINT,
    last_login_ip VARCHAR(64),
    created_by BIGINT,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    updated_by BIGINT,
    updated_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_admins_username ON admin_admins(username);
CREATE INDEX idx_admin_admins_role ON admin_admins(role);
CREATE INDEX idx_admin_admins_status ON admin_admins(status);

-- 插入默认超级管理员 (密码: admin123)
INSERT INTO admin_admins (username, password_hash, role, status, created_at, updated_at)
VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'super_admin', 'active',
        (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT);

-- 市场配置表
CREATE TABLE IF NOT EXISTS admin_market_configs (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(32) NOT NULL UNIQUE,
    base_asset VARCHAR(16) NOT NULL,
    quote_asset VARCHAR(16) NOT NULL,
    price_precision INT NOT NULL DEFAULT 8,
    quantity_precision INT NOT NULL DEFAULT 8,
    min_quantity DECIMAL(36,18) NOT NULL DEFAULT 0,
    max_quantity DECIMAL(36,18) NOT NULL DEFAULT 0,
    min_notional DECIMAL(36,18) NOT NULL DEFAULT 0,
    maker_fee_rate DECIMAL(18,8) NOT NULL DEFAULT 0,
    taker_fee_rate DECIMAL(18,8) NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'inactive',
    metadata JSONB,
    created_by BIGINT,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    updated_by BIGINT,
    updated_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_market_configs_symbol ON admin_market_configs(symbol);
CREATE INDEX idx_admin_market_configs_status ON admin_market_configs(status);
CREATE INDEX idx_admin_market_configs_base_asset ON admin_market_configs(base_asset);
CREATE INDEX idx_admin_market_configs_quote_asset ON admin_market_configs(quote_asset);

-- 系统配置表
CREATE TABLE IF NOT EXISTS admin_system_configs (
    id BIGSERIAL PRIMARY KEY,
    config_key VARCHAR(128) NOT NULL UNIQUE,
    config_value TEXT NOT NULL,
    value_type VARCHAR(16) NOT NULL DEFAULT 'string',
    category VARCHAR(64),
    description VARCHAR(512),
    is_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    created_by BIGINT,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    updated_by BIGINT,
    updated_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_system_configs_key ON admin_system_configs(config_key);
CREATE INDEX idx_admin_system_configs_category ON admin_system_configs(category);

-- 配置版本表
CREATE TABLE IF NOT EXISTS admin_config_versions (
    id BIGSERIAL PRIMARY KEY,
    version BIGINT NOT NULL UNIQUE,
    change_type VARCHAR(32) NOT NULL,
    change_detail TEXT,
    changed_by BIGINT NOT NULL,
    changed_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_config_versions_version ON admin_config_versions(version);

-- 初始化配置版本
INSERT INTO admin_config_versions (version, change_type, change_detail, changed_by, changed_at)
VALUES (1, 'init', 'Initial configuration version', 1, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT);

-- 审计日志表
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL,
    admin_username VARCHAR(64) NOT NULL,
    action VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(128),
    old_value JSONB,
    new_value JSONB,
    ip_address VARCHAR(64),
    user_agent VARCHAR(512),
    request_id VARCHAR(64),
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_audit_logs_admin_id ON admin_audit_logs(admin_id);
CREATE INDEX idx_admin_audit_logs_action ON admin_audit_logs(action);
CREATE INDEX idx_admin_audit_logs_resource_type ON admin_audit_logs(resource_type);
CREATE INDEX idx_admin_audit_logs_resource_id ON admin_audit_logs(resource_id);
CREATE INDEX idx_admin_audit_logs_created_at ON admin_audit_logs(created_at);

-- 每日统计表
CREATE TABLE IF NOT EXISTS admin_daily_stats (
    id BIGSERIAL PRIMARY KEY,
    stats_date VARCHAR(10) NOT NULL UNIQUE,
    total_users BIGINT NOT NULL DEFAULT 0,
    new_users BIGINT NOT NULL DEFAULT 0,
    active_users BIGINT NOT NULL DEFAULT 0,
    total_orders BIGINT NOT NULL DEFAULT 0,
    total_trades BIGINT NOT NULL DEFAULT 0,
    total_volume DECIMAL(36,18) NOT NULL DEFAULT 0,
    total_fees DECIMAL(36,18) NOT NULL DEFAULT 0,
    total_deposits DECIMAL(36,18) NOT NULL DEFAULT 0,
    total_withdrawals DECIMAL(36,18) NOT NULL DEFAULT 0,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    updated_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_daily_stats_date ON admin_daily_stats(stats_date);

-- 添加注释
COMMENT ON TABLE admin_admins IS '管理员表';
COMMENT ON TABLE admin_market_configs IS '市场/交易对配置表';
COMMENT ON TABLE admin_system_configs IS '系统配置表';
COMMENT ON TABLE admin_config_versions IS '配置版本表';
COMMENT ON TABLE admin_audit_logs IS '审计日志表';
COMMENT ON TABLE admin_daily_stats IS '每日统计表';
