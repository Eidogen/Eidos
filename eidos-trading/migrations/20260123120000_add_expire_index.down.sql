-- 回滚: 删除过期订单查询索引
DROP INDEX IF EXISTS idx_trading_orders_expire_status;
