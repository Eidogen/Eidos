package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Trading Service Metrics - 交易服务监控指标
var (
	// OrdersTotal 订单总数 (按状态、市场、方向分组)
	OrdersTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "orders_total",
			Help:      "订单总数，按状态(created/filled/cancelled/expired)、市场、买卖方向分组",
		},
		[]string{"status", "market", "side"},
	)

	// OrderLatency 订单处理延迟
	OrderLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "order_latency_seconds",
			Help:      "订单处理延迟(秒)，按操作类型(create/cancel/expire)分组",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to 16s
		},
		[]string{"operation"},
	)

	// TradesTotal 成交总数
	TradesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "trades_total",
			Help:      "成交总笔数，按市场分组",
		},
		[]string{"market"},
	)

	// TradeVolume 成交金额
	TradeVolume = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "trade_volume_total",
			Help:      "成交总金额(计价代币)，按市场分组",
		},
		[]string{"market"},
	)

	// BalanceOperations 余额操作计数
	BalanceOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "balance_operations_total",
			Help:      "余额操作总数，按操作类型(freeze/unfreeze/credit/debit/settle)和代币分组",
		},
		[]string{"operation", "token"},
	)

	// RiskControlRejections 风控拒绝计数
	RiskControlRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "risk_rejections_total",
			Help:      "风控拒绝总数，按原因分组: insufficient_balance(余额不足), pending_limit(用户待结算限额), global_limit(全局待结算限额), max_orders(最大订单数)",
		},
		[]string{"reason"},
	)

	// OutboxPendingGauge Outbox 待处理消息数
	OutboxPendingGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "outbox_pending",
			Help:      "Outbox 待处理消息数，按类型(order/cancel)和分片ID分组",
		},
		[]string{"type", "shard"},
	)

	// RedisOperationLatency Redis 操作延迟
	RedisOperationLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "redis_operation_latency_seconds",
			Help:      "Redis 操作延迟(秒)，按操作类型分组",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 12), // 0.1ms to 200ms
		},
		[]string{"operation"},
	)

	// ReconciliationDiffs 对账差异数
	ReconciliationDiffs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "reconciliation_diffs",
			Help:      "对账发现的余额差异数，按代币分组。非零值需要告警处理",
		},
		[]string{"token"},
	)

	// UserOpenOrdersGauge 用户活跃订单数
	// [Modification]: Removed 'wallet' label to avoid high cardinality.
	// This metric now tracks the histogram of open orders distribution or a summary,
	// BUT since the original was a GaugeVec by wallet, to replace it usefully without wallet,
	// we might want "TotalOpenOrders" (sum of all users) or "MaxOpenOrders".
	// For now, I will change it to "TotalUserOpenOrders" (gauge) and we can maybe add a Histogram for distribution if needed.
	// However, keeping consistent with the helper function `DecrUserOpenOrders`, I will change the metric definition.
	// Actually, if we remove the label, we can't track "per user".
	// The helper function `SetUserOpenOrders(wallet, count)` suggests we want to track it per user.
	// To fix high cardinality, we usually STOP tracking per user in Prometheus.
	// Instead, we track "Total Open Orders" across the system.
	TotalOpenOrdersGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "total_open_orders",
			Help:      "系统当前总活跃订单数",
		},
	)

	// DepositTotal 充值总数
	DepositTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "deposits_total",
			Help:      "充值总数，按状态(pending/confirmed/failed)和代币分组",
		},
		[]string{"status", "token"},
	)

	// WithdrawalTotal 提现总数
	WithdrawalTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "withdrawals_total",
			Help:      "提现总数，按状态(pending/submitted/confirmed/failed)和代币分组",
		},
		[]string{"status", "token"},
	)

	// SettlementTotal 结算总数
	SettlementTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "settlements_total",
			Help:      "链上结算总数，按状态(success/failed/rolled_back)分组",
		},
		[]string{"status"},
	)

	// ConsumerLag 消息消费延迟
	ConsumerLag = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "consumer_lag_seconds",
			Help:      "Kafka 消息消费延迟(秒)",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to 4s
		},
		[]string{"topic"},
	)

	// OutboxErrors Outbox 处理错误
	OutboxErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "outbox_errors_total",
			Help:      "Outbox 处理错误总数，按类型(order/cancel)和错误原因分组",
		},
		[]string{"type", "reason"},
	)

	// DataIntegrityCritical 数据一致性严重错误
	DataIntegrityCritical = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "data_integrity_critical_total",
			Help:      "数据一致性严重错误 (P0 级告警)，如回滚失败、资金不守恒等",
		},
		[]string{"type", "reason"},
	)

	// DBConnectionPool DB 连接池状态
	DBConnectionPool = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "db_connection_pool",
			Help:      "DB 连接池状态",
		},
		[]string{"state"}, // open, idle, in_use, max
	)

	// RedisPool Redis 连接池状态
	RedisPool = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "redis_pool",
			Help:      "Redis 连接池状态",
		},
		[]string{"state"}, // hits, misses, timeouts, total_conns, idle_conns, stale_conns
	)

	// AsyncTasksGauge 异步任务数
	AsyncTasksGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "async_tasks",
			Help:      "待处理异步任务数，按任务类型分组",
		},
		[]string{"type"},
	)

	// AsyncTaskErrors 异步任务错误数
	AsyncTaskErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "trading",
			Name:      "async_task_errors_total",
			Help:      "异步任务错误总数，按任务类型分组。需要告警监控",
		},
		[]string{"type"},
	)
)

// ========== Helper functions 辅助函数 ==========

// RecordOrderCreated 记录订单创建
func RecordOrderCreated(market, side string) {
	OrdersTotal.WithLabelValues("created", market, side).Inc()
}

// RecordOrderFilled 记录订单完全成交
func RecordOrderFilled(market, side string) {
	OrdersTotal.WithLabelValues("filled", market, side).Inc()
}

// RecordOrderCancelled 记录订单取消
func RecordOrderCancelled(market, side string) {
	OrdersTotal.WithLabelValues("cancelled", market, side).Inc()
}

// RecordOrderExpired 记录订单过期
func RecordOrderExpired(market, side string) {
	OrdersTotal.WithLabelValues("expired", market, side).Inc()
}

// RecordRiskRejection 记录风控拒绝
// reason 取值: insufficient_balance, pending_limit, global_limit, max_orders
func RecordRiskRejection(reason string) {
	RiskControlRejections.WithLabelValues(reason).Inc()
}

// RecordTrade 记录成交
func RecordTrade(market string, volume float64) {
	TradesTotal.WithLabelValues(market).Inc()
	TradeVolume.WithLabelValues(market).Add(volume)
}

// RecordSettlement 记录链上结算
// status 取值: success, failed, rolled_back
func RecordSettlement(status string) {
	SettlementTotal.WithLabelValues(status).Inc()
}

// RecordBalanceOperation 记录余额操作
// operation 取值: freeze, unfreeze, credit, debit, settle
func RecordBalanceOperation(operation, token string) {
	BalanceOperations.WithLabelValues(operation, token).Inc()
}

// RecordDeposit 记录充值
// status 取值: pending, confirmed, failed
func RecordDeposit(status, token string) {
	DepositTotal.WithLabelValues(status, token).Inc()
}

// RecordWithdrawal 记录提现
// status 取值: pending, submitted, confirmed, failed
func RecordWithdrawal(status, token string) {
	WithdrawalTotal.WithLabelValues(status, token).Inc()
}

// RecordAsyncTaskError 记录异步任务错误
func RecordAsyncTaskError(taskType string) {
	AsyncTaskErrors.WithLabelValues(taskType).Inc()
}

// SetAsyncTasksPending 设置待处理异步任务数
func SetAsyncTasksPending(taskType string, count float64) {
	AsyncTasksGauge.WithLabelValues(taskType).Set(count)
}

// SetReconciliationDiff 设置对账差异数
func SetReconciliationDiff(token string, count float64) {
	ReconciliationDiffs.WithLabelValues(token).Set(count)
}

// SetOutboxPending 设置 Outbox 待处理消息数
func SetOutboxPending(msgType, shard string, count float64) {
	OutboxPendingGauge.WithLabelValues(msgType, shard).Set(count)
}

// RecordConsumerLag 记录消费延迟
func RecordConsumerLag(topic string, lagSeconds float64) {
	ConsumerLag.WithLabelValues(topic).Observe(lagSeconds)
}

// RecordOutboxError 记录 Outbox 错误
func RecordOutboxError(msgType, reason string) {
	OutboxErrors.WithLabelValues(msgType, reason).Inc()
}

// RecordDataIntegrityCritical 记录数据一致性严重错误
func RecordDataIntegrityCritical(errType, reason string) {
	DataIntegrityCritical.WithLabelValues(errType, reason).Inc()
}

// Helper needed: We removed UserOpenOrdersGauge (Vec), replaced with TotalOpenOrdersGauge.
// The previous code called `DecrUserOpenOrders(ctx, wallet)`.
// We can't do that per-user logic here anymore for the metrics.
// But maybe we can track "Total Open Orders" by incrementing/decrementing globally.

// IncTotalOpenOrders 增加全局活跃订单数
func IncTotalOpenOrders() {
	TotalOpenOrdersGauge.Inc()
}

// DecTotalOpenOrders 减少全局活跃订单数
func DecTotalOpenOrders() {
	TotalOpenOrdersGauge.Dec()
}
