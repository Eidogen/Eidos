-- 添加过期订单查询索引
-- 用于快速扫描过期的 PENDING, OPEN, PARTIAL 订单

-- 复合索引: expire_at + status (部分索引，只包含可过期的状态)
CREATE INDEX IF NOT EXISTS idx_trading_orders_expire_status
ON trading_orders(expire_at, status)
WHERE status IN (0, 1, 2);  -- PENDING=0, OPEN=1, PARTIAL=2

-- 说明:
-- 1. 查询条件: WHERE expire_at < NOW() AND status IN (0,1,2)
-- 2. 部分索引减少索引大小（已完成/取消的订单不进入索引）
-- 3. expire_at 在前便于范围扫描
