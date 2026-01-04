-- Eidos Trading Service Database Migration
-- Version: 001
-- Description: Initial tables creation
-- Created: 2024

-- ============================================
-- 订单表 (orders)
-- ============================================
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL,
    wallet VARCHAR(42) NOT NULL,
    market VARCHAR(20) NOT NULL,
    side SMALLINT NOT NULL,                          -- 1: buy, 2: sell
    order_type SMALLINT NOT NULL,                    -- 1: limit, 2: market
    price DECIMAL(36, 18),                           -- 价格 (市价单为 null)
    amount DECIMAL(36, 18) NOT NULL,                 -- 数量
    filled_amount DECIMAL(36, 18) NOT NULL DEFAULT 0,  -- 已成交数量
    filled_quote DECIMAL(36, 18) NOT NULL DEFAULT 0,   -- 已成交金额 (quote token)
    remaining_amount DECIMAL(36, 18) NOT NULL,         -- 剩余数量
    avg_price DECIMAL(36, 18) NOT NULL DEFAULT 0,      -- 成交均价
    status SMALLINT NOT NULL DEFAULT 0,              -- 0: pending, 1: open, 2: partial, 3: filled, 4: cancelled, 5: expired, 6: rejected
    time_in_force SMALLINT NOT NULL DEFAULT 1,       -- 1: GTC, 2: IOC, 3: FOK
    nonce BIGINT NOT NULL,                           -- 用户 Nonce
    client_order_id VARCHAR(64),                     -- 客户端订单ID (幂等键)
    expire_at BIGINT NOT NULL,                       -- 过期时间 (毫秒)
    signature BYTEA,                                 -- EIP-712 签名
    reject_reason VARCHAR(255),                      -- 拒绝原因
    freeze_token VARCHAR(20) NOT NULL,               -- 冻结的 Token
    freeze_amount DECIMAL(36, 18) NOT NULL,          -- 冻结数量
    created_at BIGINT NOT NULL,                      -- 创建时间 (毫秒)
    updated_at BIGINT NOT NULL,                      -- 更新时间 (毫秒)
    created_by VARCHAR(64),
    updated_by VARCHAR(64)
);

-- 订单表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_orders_order_id ON orders(order_id);
CREATE INDEX IF NOT EXISTS idx_orders_wallet ON orders(wallet);
CREATE INDEX IF NOT EXISTS idx_orders_market ON orders(market);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_wallet_client_order ON orders(wallet, client_order_id);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);

-- ============================================
-- 余额表 (balances)
-- ============================================
CREATE TABLE IF NOT EXISTS balances (
    id BIGSERIAL PRIMARY KEY,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    settled_available DECIMAL(36, 18) NOT NULL DEFAULT 0,  -- 已结算可用 (可提现)
    settled_frozen DECIMAL(36, 18) NOT NULL DEFAULT 0,     -- 已结算冻结
    pending_available DECIMAL(36, 18) NOT NULL DEFAULT 0,  -- 待结算可用
    pending_frozen DECIMAL(36, 18) NOT NULL DEFAULT 0,     -- 待结算冻结
    pending_total DECIMAL(36, 18) NOT NULL DEFAULT 0,      -- 待结算总额 (用于风控限额)
    version BIGINT NOT NULL DEFAULT 1,                     -- 乐观锁版本号
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- 余额表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_balances_wallet_token ON balances(wallet, token);
CREATE INDEX IF NOT EXISTS idx_balances_wallet ON balances(wallet);

-- ============================================
-- 余额流水表 (balance_logs)
-- ============================================
CREATE TABLE IF NOT EXISTS balance_logs (
    id BIGSERIAL PRIMARY KEY,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    type SMALLINT NOT NULL,                             -- 流水类型
    amount DECIMAL(36, 18) NOT NULL,                    -- 变动金额 (正数加，负数减)
    balance_before DECIMAL(36, 18) NOT NULL,            -- 变动前余额
    balance_after DECIMAL(36, 18) NOT NULL,             -- 变动后余额
    order_id VARCHAR(64),                               -- 关联订单 ID
    trade_id VARCHAR(64),                               -- 关联成交 ID (幂等键)
    tx_hash VARCHAR(66),                                -- 链上交易哈希
    remark VARCHAR(255),                                -- 备注
    created_at BIGINT NOT NULL
);

-- 余额流水表索引
CREATE INDEX IF NOT EXISTS idx_balance_logs_wallet ON balance_logs(wallet);
CREATE INDEX IF NOT EXISTS idx_balance_logs_type ON balance_logs(type);
CREATE INDEX IF NOT EXISTS idx_balance_logs_order_id ON balance_logs(order_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_balance_logs_trade_wallet_token ON balance_logs(trade_id, wallet, token) WHERE trade_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_balance_logs_created_at ON balance_logs(created_at);

-- ============================================
-- 手续费账户表 (fee_accounts)
-- ============================================
CREATE TABLE IF NOT EXISTS fee_accounts (
    id BIGSERIAL PRIMARY KEY,
    bucket_id INT NOT NULL,                            -- 分桶 ID (0-15)
    token VARCHAR(20) NOT NULL,
    balance DECIMAL(36, 18) NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,                 -- 乐观锁
    updated_at BIGINT NOT NULL
);

-- 手续费账户表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_fee_accounts_bucket_token ON fee_accounts(bucket_id, token);

-- ============================================
-- 成交记录表 (trades)
-- ============================================
CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    trade_id VARCHAR(64) NOT NULL,
    market VARCHAR(20) NOT NULL,
    maker_order_id VARCHAR(64) NOT NULL,
    taker_order_id VARCHAR(64) NOT NULL,
    maker_wallet VARCHAR(42) NOT NULL,
    taker_wallet VARCHAR(42) NOT NULL,
    price DECIMAL(36, 18) NOT NULL,                    -- 成交价格
    amount DECIMAL(36, 18) NOT NULL,                   -- 成交数量 (Base Token)
    quote_amount DECIMAL(36, 18) NOT NULL,             -- 成交金额 (Quote Token)
    maker_fee DECIMAL(36, 18) NOT NULL DEFAULT 0,      -- Maker 手续费
    taker_fee DECIMAL(36, 18) NOT NULL DEFAULT 0,      -- Taker 手续费
    fee_token VARCHAR(20),                             -- 手续费 Token
    maker_side SMALLINT NOT NULL,                      -- Maker 方向 (1: buy, 2: sell)
    settlement_status SMALLINT NOT NULL DEFAULT 0,    -- 结算状态
    batch_id VARCHAR(64),                              -- 结算批次 ID
    tx_hash VARCHAR(66),                               -- 链上交易哈希
    matched_at BIGINT NOT NULL,                        -- 撮合时间 (毫秒)
    settled_at BIGINT,                                 -- 结算确认时间 (毫秒)
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- 成交记录表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_trades_trade_id ON trades(trade_id);
CREATE INDEX IF NOT EXISTS idx_trades_market ON trades(market);
CREATE INDEX IF NOT EXISTS idx_trades_maker_order_id ON trades(maker_order_id);
CREATE INDEX IF NOT EXISTS idx_trades_taker_order_id ON trades(taker_order_id);
CREATE INDEX IF NOT EXISTS idx_trades_maker_wallet ON trades(maker_wallet);
CREATE INDEX IF NOT EXISTS idx_trades_taker_wallet ON trades(taker_wallet);
CREATE INDEX IF NOT EXISTS idx_trades_settlement_status ON trades(settlement_status);
CREATE INDEX IF NOT EXISTS idx_trades_batch_id ON trades(batch_id);
CREATE INDEX IF NOT EXISTS idx_trades_matched_at ON trades(matched_at);

-- ============================================
-- 结算批次表 (settlement_batches)
-- ============================================
CREATE TABLE IF NOT EXISTS settlement_batches (
    id BIGSERIAL PRIMARY KEY,
    batch_id VARCHAR(64) NOT NULL,
    trade_count INT NOT NULL,                          -- 成交笔数
    total_amount DECIMAL(36, 18) NOT NULL,             -- 总金额
    status SMALLINT NOT NULL,                          -- 批次状态
    tx_hash VARCHAR(66),                               -- 链上交易哈希
    gas_price BIGINT,                                  -- Gas 价格
    gas_used BIGINT,                                   -- Gas 使用量
    retry_count INT NOT NULL DEFAULT 0,                -- 重试次数
    last_error TEXT,                                   -- 最后错误信息
    submitted_at BIGINT,                               -- 提交时间
    confirmed_at BIGINT,                               -- 确认时间
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- 结算批次表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_settlement_batches_batch_id ON settlement_batches(batch_id);
CREATE INDEX IF NOT EXISTS idx_settlement_batches_status ON settlement_batches(status);

-- ============================================
-- 充值记录表 (deposits)
-- ============================================
CREATE TABLE IF NOT EXISTS deposits (
    id BIGSERIAL PRIMARY KEY,
    deposit_id VARCHAR(64) NOT NULL,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,                   -- 充值金额
    tx_hash VARCHAR(66) NOT NULL,                      -- 链上交易哈希
    log_index INT NOT NULL,                            -- 事件日志索引
    block_num BIGINT NOT NULL,                         -- 区块高度
    status SMALLINT NOT NULL DEFAULT 0,                -- 状态: 0: pending, 1: confirmed, 2: credited
    detected_at BIGINT NOT NULL,                       -- 检测时间
    confirmed_at BIGINT,                               -- 确认时间
    credited_at BIGINT,                                -- 入账时间
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- 充值记录表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_deposits_deposit_id ON deposits(deposit_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_deposits_tx_log ON deposits(tx_hash, log_index);
CREATE INDEX IF NOT EXISTS idx_deposits_wallet ON deposits(wallet);
CREATE INDEX IF NOT EXISTS idx_deposits_status ON deposits(status);

-- ============================================
-- 提现记录表 (withdrawals)
-- ============================================
CREATE TABLE IF NOT EXISTS withdrawals (
    id BIGSERIAL PRIMARY KEY,
    withdraw_id VARCHAR(64) NOT NULL,
    wallet VARCHAR(42) NOT NULL,
    token VARCHAR(20) NOT NULL,
    amount DECIMAL(36, 18) NOT NULL,                   -- 提现金额
    to_address VARCHAR(42) NOT NULL,                   -- 目标地址
    nonce BIGINT NOT NULL,                             -- 用户 Nonce
    signature BYTEA,                                   -- EIP-712 签名
    status SMALLINT NOT NULL DEFAULT 0,                -- 状态
    tx_hash VARCHAR(66),                               -- 链上交易哈希
    reject_reason VARCHAR(255),                        -- 拒绝原因
    refunded_at BIGINT,                                -- 退回时间 (失败时)
    submitted_at BIGINT,                               -- 提交时间
    confirmed_at BIGINT,                               -- 确认时间
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

-- 提现记录表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_withdrawals_withdraw_id ON withdrawals(withdraw_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_withdrawals_wallet_nonce ON withdrawals(wallet, nonce);
CREATE INDEX IF NOT EXISTS idx_withdrawals_status ON withdrawals(status);

-- ============================================
-- 已使用 Nonce 表 (used_nonces)
-- ============================================
CREATE TABLE IF NOT EXISTS used_nonces (
    id BIGSERIAL PRIMARY KEY,
    wallet VARCHAR(42) NOT NULL,
    usage SMALLINT NOT NULL,                           -- 用途: 1: order, 2: cancel, 3: withdraw
    nonce BIGINT NOT NULL,
    order_id VARCHAR(64),                              -- 关联订单 ID
    created_at BIGINT NOT NULL
);

-- 已使用 Nonce 表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_used_nonces_wallet_usage_nonce ON used_nonces(wallet, usage, nonce);
CREATE INDEX IF NOT EXISTS idx_used_nonces_created_at ON used_nonces(created_at);

-- ============================================
-- 注释说明
-- ============================================
COMMENT ON TABLE orders IS '订单表';
COMMENT ON TABLE balances IS '用户余额表 (四字段模型: settled/pending × available/frozen)';
COMMENT ON TABLE balance_logs IS '余额流水表';
COMMENT ON TABLE fee_accounts IS '手续费账户表 (分桶设计，减少热点)';
COMMENT ON TABLE trades IS '成交记录表';
COMMENT ON TABLE settlement_batches IS '结算批次表';
COMMENT ON TABLE deposits IS '充值记录表 (幂等键: tx_hash + log_index)';
COMMENT ON TABLE withdrawals IS '提现记录表 (幂等键: wallet + nonce)';
COMMENT ON TABLE used_nonces IS '已使用 Nonce 表 (防重放攻击)';
