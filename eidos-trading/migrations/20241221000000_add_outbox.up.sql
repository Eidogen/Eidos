-- Eidos Trading Service Database Migration
-- Version: 002
-- Description: Add outbox_messages table for reliable async writes (Transactional Outbox Pattern)
-- Created: 2024

-- ============================================
-- Outbox Messages 表 (transactional outbox pattern)
-- ============================================
CREATE TABLE IF NOT EXISTS outbox_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(64) NOT NULL,                  -- 全局唯一 ID (幂等)
    topic VARCHAR(100) NOT NULL,                      -- Kafka topic / 操作类型
    partition_key VARCHAR(100) NOT NULL,              -- 分区键 (wallet:token)
    payload JSONB NOT NULL,                           -- 消息内容 / 操作载荷
    aggregate_type VARCHAR(50) NOT NULL,              -- 聚合类型: balance, order, trade, withdrawal
    aggregate_id VARCHAR(64) NOT NULL,                -- 聚合 ID (wallet:token, order_id)
    status VARCHAR(20) NOT NULL DEFAULT 'pending',    -- pending, sent, failed
    retry_count INT NOT NULL DEFAULT 0,               -- 重试次数
    max_retries INT NOT NULL DEFAULT 5,               -- 最大重试次数
    last_error VARCHAR(500),                          -- 最后错误信息
    created_at BIGINT NOT NULL,
    sent_at BIGINT                                    -- 发送/处理时间
);

-- Outbox Messages 表索引
CREATE UNIQUE INDEX IF NOT EXISTS uk_outbox_message_id ON outbox_messages(message_id);
CREATE INDEX IF NOT EXISTS idx_outbox_status_created ON outbox_messages(status, created_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_messages(aggregate_type, aggregate_id);
CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox_messages(created_at);

COMMENT ON TABLE outbox_messages IS 'Transactional Outbox 表，保证异步写入可靠性';
