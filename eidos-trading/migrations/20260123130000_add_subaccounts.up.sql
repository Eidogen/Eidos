-- 子账户表
-- 一个钱包可以创建多个独立的子账户进行交易
CREATE TABLE IF NOT EXISTS trading_subaccounts (
    id BIGSERIAL PRIMARY KEY,
    sub_account_id VARCHAR(64) NOT NULL UNIQUE,
    wallet VARCHAR(42) NOT NULL,
    name VARCHAR(50) NOT NULL,
    type SMALLINT NOT NULL DEFAULT 1,       -- 1=现货, 2=保证金, 3=合约
    status SMALLINT NOT NULL DEFAULT 1,     -- 1=活跃, 2=冻结, 3=已删除
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    remark VARCHAR(255),
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

CREATE INDEX idx_subaccounts_wallet ON trading_subaccounts(wallet);
CREATE INDEX idx_subaccounts_status ON trading_subaccounts(status);

-- 子账户余额表
-- 每个子账户有独立的余额，与主账户分开管理
CREATE TABLE IF NOT EXISTS trading_subaccount_balances (
    id BIGSERIAL PRIMARY KEY,
    sub_account_id VARCHAR(64) NOT NULL,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    available DECIMAL(36,18) NOT NULL DEFAULT 0,
    frozen DECIMAL(36,18) NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    UNIQUE(sub_account_id, token)
);

CREATE INDEX idx_subaccount_balances_wallet ON trading_subaccount_balances(wallet);

-- 子账户划转记录表
-- 记录主账户与子账户之间的资金划转
CREATE TABLE IF NOT EXISTS trading_subaccount_transfers (
    id BIGSERIAL PRIMARY KEY,
    transfer_id VARCHAR(64) NOT NULL UNIQUE,
    wallet VARCHAR(42) NOT NULL,
    sub_account_id VARCHAR(64) NOT NULL,
    type SMALLINT NOT NULL,                 -- 1=划入, 2=划出
    token VARCHAR(20) NOT NULL,
    amount DECIMAL(36,18) NOT NULL,
    remark VARCHAR(255),
    created_at BIGINT NOT NULL
);

CREATE INDEX idx_subaccount_transfers_wallet ON trading_subaccount_transfers(wallet);
CREATE INDEX idx_subaccount_transfers_subaccount ON trading_subaccount_transfers(sub_account_id);
CREATE INDEX idx_subaccount_transfers_created ON trading_subaccount_transfers(created_at);

-- 子账户余额流水表
-- 记录子账户的所有余额变动
CREATE TABLE IF NOT EXISTS trading_subaccount_balance_logs (
    id BIGSERIAL PRIMARY KEY,
    sub_account_id VARCHAR(64) NOT NULL,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    type SMALLINT NOT NULL,
    amount DECIMAL(36,18) NOT NULL,
    balance_before DECIMAL(36,18) NOT NULL,
    balance_after DECIMAL(36,18) NOT NULL,
    order_id VARCHAR(64),
    trade_id VARCHAR(64),
    transfer_id VARCHAR(64),
    remark VARCHAR(255),
    created_at BIGINT NOT NULL
);

CREATE INDEX idx_subaccount_balance_logs_subaccount ON trading_subaccount_balance_logs(sub_account_id);
CREATE INDEX idx_subaccount_balance_logs_wallet ON trading_subaccount_balance_logs(wallet);
CREATE INDEX idx_subaccount_balance_logs_created ON trading_subaccount_balance_logs(created_at);
CREATE UNIQUE INDEX uk_trade_subaccount_token ON trading_subaccount_balance_logs(trade_id, sub_account_id, token) WHERE trade_id IS NOT NULL;

-- 注释说明
COMMENT ON TABLE trading_subaccounts IS '子账户表 - 一个钱包可创建多个独立交易账户';
COMMENT ON COLUMN trading_subaccounts.sub_account_id IS '子账户ID (格式: 钱包前缀+序号)';
COMMENT ON COLUMN trading_subaccounts.type IS '账户类型: 1=现货, 2=保证金, 3=合约';
COMMENT ON COLUMN trading_subaccounts.status IS '状态: 1=活跃, 2=冻结, 3=已删除';
COMMENT ON COLUMN trading_subaccounts.is_default IS '是否为默认子账户';

COMMENT ON TABLE trading_subaccount_balances IS '子账户余额表';
COMMENT ON TABLE trading_subaccount_transfers IS '子账户划转记录表';
COMMENT ON TABLE trading_subaccount_balance_logs IS '子账户余额流水表';
