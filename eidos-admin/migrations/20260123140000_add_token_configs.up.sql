-- 代币配置表
CREATE TABLE IF NOT EXISTS admin_token_configs (
    id BIGSERIAL PRIMARY KEY,
    symbol VARCHAR(20) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    contract_address VARCHAR(42) NOT NULL,
    decimals INT NOT NULL DEFAULT 18,
    chain_id BIGINT NOT NULL DEFAULT 42161,
    status INT NOT NULL DEFAULT 1,
    deposit_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    withdraw_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    min_deposit VARCHAR(36) NOT NULL DEFAULT '0',
    min_withdraw VARCHAR(36) NOT NULL DEFAULT '0',
    max_withdraw VARCHAR(36) NOT NULL DEFAULT '0',
    withdraw_fee VARCHAR(36) NOT NULL DEFAULT '0',
    confirmations INT NOT NULL DEFAULT 12,
    sort_order INT NOT NULL DEFAULT 0,
    icon_url VARCHAR(256),
    metadata JSONB,
    created_by BIGINT,
    created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT,
    updated_by BIGINT,
    updated_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT
);

CREATE INDEX idx_admin_token_configs_symbol ON admin_token_configs(symbol);
CREATE INDEX idx_admin_token_configs_status ON admin_token_configs(status);
CREATE INDEX idx_admin_token_configs_chain_id ON admin_token_configs(chain_id);
CREATE INDEX idx_admin_token_configs_contract_address ON admin_token_configs(contract_address);

-- 添加注释
COMMENT ON TABLE admin_token_configs IS '代币配置表';
COMMENT ON COLUMN admin_token_configs.symbol IS '代币符号';
COMMENT ON COLUMN admin_token_configs.name IS '代币名称';
COMMENT ON COLUMN admin_token_configs.contract_address IS '合约地址';
COMMENT ON COLUMN admin_token_configs.decimals IS '小数位数';
COMMENT ON COLUMN admin_token_configs.chain_id IS '链ID';
COMMENT ON COLUMN admin_token_configs.status IS '状态: 1=活跃, 2=禁用';
COMMENT ON COLUMN admin_token_configs.deposit_enabled IS '是否允许充值';
COMMENT ON COLUMN admin_token_configs.withdraw_enabled IS '是否允许提现';
COMMENT ON COLUMN admin_token_configs.min_deposit IS '最小充值金额';
COMMENT ON COLUMN admin_token_configs.min_withdraw IS '最小提现金额';
COMMENT ON COLUMN admin_token_configs.max_withdraw IS '最大提现金额';
COMMENT ON COLUMN admin_token_configs.withdraw_fee IS '提现手续费';
COMMENT ON COLUMN admin_token_configs.confirmations IS '充值确认数';
COMMENT ON COLUMN admin_token_configs.sort_order IS '排序顺序';
COMMENT ON COLUMN admin_token_configs.icon_url IS '图标URL';
COMMENT ON COLUMN admin_token_configs.metadata IS '扩展元数据';

-- 插入默认代币配置
INSERT INTO admin_token_configs (symbol, name, contract_address, decimals, chain_id, status, deposit_enabled, withdraw_enabled, min_deposit, min_withdraw, max_withdraw, withdraw_fee, confirmations, sort_order, created_at, updated_at)
VALUES
    ('USDC', 'USD Coin', '0xaf88d065e77c8cC2239327C5EDb3A432268e5831', 6, 42161, 1, TRUE, TRUE, '1', '10', '1000000', '1', 12, 1, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT),
    ('WETH', 'Wrapped Ether', '0x82aF49447D8a07e3bd95BD0d56f35241523fBab1', 18, 42161, 1, TRUE, TRUE, '0.001', '0.01', '1000', '0.001', 12, 2, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT),
    ('WBTC', 'Wrapped Bitcoin', '0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f', 8, 42161, 1, TRUE, TRUE, '0.0001', '0.001', '100', '0.0001', 12, 3, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT, (EXTRACT(EPOCH FROM NOW()) * 1000)::BIGINT);
