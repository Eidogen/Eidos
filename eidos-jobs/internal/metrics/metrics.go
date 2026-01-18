// Package metrics 提供 eidos-jobs 服务的 Prometheus 监控指标
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "eidos_jobs"

// 任务执行指标
var (
	// JobExecutionsTotal 任务执行总数
	JobExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "job_executions_total",
			Help:      "任务执行总数",
		},
		[]string{"job_name", "status"}, // status: success, failed, skipped, timeout
	)

	// JobDuration 任务执行耗时
	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "job_duration_seconds",
			Help:      "任务执行耗时(秒)",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"job_name"},
	)

	// JobLastExecutionTime 任务最后执行时间
	JobLastExecutionTime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "job_last_execution_timestamp",
			Help:      "任务最后执行时间戳",
		},
		[]string{"job_name"},
	)

	// JobNextExecutionTime 任务下次执行时间
	JobNextExecutionTime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "job_next_execution_timestamp",
			Help:      "任务下次执行时间戳",
		},
		[]string{"job_name"},
	)

	// RunningJobsGauge 当前运行中的任务数
	RunningJobsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "running_jobs_total",
			Help:      "当前运行中的任务数",
		},
	)

	// ScheduledJobsGauge 已调度任务数
	ScheduledJobsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scheduled_jobs_total",
			Help:      "已调度任务总数",
		},
	)
)

// 对账任务指标
var (
	// ReconciliationTotal 对账执行总数
	ReconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reconciliation_total",
			Help:      "对账执行总数",
		},
		[]string{"type", "result"}, // type: balance/order/settlement, result: matched/mismatched/error
	)

	// ReconciliationDiffs 对账差异数
	ReconciliationDiffs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "reconciliation_diffs",
			Help:      "对账发现的差异数量",
		},
		[]string{"type", "token"},
	)

	// ReconciliationDuration 对账耗时
	ReconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "reconciliation_duration_seconds",
			Help:      "对账任务耗时(秒)",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"type"},
	)

	// ReconciliationAccountsChecked 对账检查账户数
	ReconciliationAccountsChecked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reconciliation_accounts_checked_total",
			Help:      "对账检查的账户总数",
		},
		[]string{"type"},
	)
)

// 归档任务指标
var (
	// ArchiveTotal 归档执行总数
	ArchiveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "archive_total",
			Help:      "归档执行总数",
		},
		[]string{"type", "status"}, // type: orders/trades/klines, status: success/failed
	)

	// ArchiveRecordsProcessed 归档处理记录数
	ArchiveRecordsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "archive_records_processed_total",
			Help:      "归档处理的记录总数",
		},
		[]string{"type"},
	)

	// ArchiveDuration 归档耗时
	ArchiveDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "archive_duration_seconds",
			Help:      "归档任务耗时(秒)",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800},
		},
		[]string{"type"},
	)

	// ArchiveStorageBytes 归档存储大小
	ArchiveStorageBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "archive_storage_bytes",
			Help:      "归档数据存储大小(字节)",
		},
		[]string{"type"},
	)
)

// 清理任务指标
var (
	// CleanupTotal 清理执行总数
	CleanupTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cleanup_total",
			Help:      "清理执行总数",
		},
		[]string{"type", "status"}, // type: expired_orders/old_logs/temp_data, status: success/failed
	)

	// CleanupRecordsDeleted 清理删除记录数
	CleanupRecordsDeleted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cleanup_records_deleted_total",
			Help:      "清理删除的记录总数",
		},
		[]string{"type"},
	)

	// CleanupDuration 清理耗时
	CleanupDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "cleanup_duration_seconds",
			Help:      "清理任务耗时(秒)",
			Buckets:   []float64{0.5, 1, 5, 10, 30, 60, 120},
		},
		[]string{"type"},
	)
)

// 统计聚合任务指标
var (
	// StatsAggregationTotal 统计聚合执行总数
	StatsAggregationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stats_aggregation_total",
			Help:      "统计聚合执行总数",
		},
		[]string{"type", "status"}, // type: daily/hourly/market, status: success/failed
	)

	// StatsAggregationDuration 统计聚合耗时
	StatsAggregationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "stats_aggregation_duration_seconds",
			Help:      "统计聚合任务耗时(秒)",
			Buckets:   []float64{0.5, 1, 5, 10, 30, 60, 120},
		},
		[]string{"type"},
	)

	// StatsRecordsGenerated 统计生成记录数
	StatsRecordsGenerated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stats_records_generated_total",
			Help:      "统计聚合生成的记录总数",
		},
		[]string{"type"},
	)
)

// 缓存同步任务指标
var (
	// CacheSyncTotal 缓存同步执行总数
	CacheSyncTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_sync_total",
			Help:      "缓存同步执行总数",
		},
		[]string{"type", "status"}, // type: balance/orderbook/config, status: success/failed
	)

	// CacheSyncDuration 缓存同步耗时
	CacheSyncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "cache_sync_duration_seconds",
			Help:      "缓存同步任务耗时(秒)",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30},
		},
		[]string{"type"},
	)

	// CacheSyncKeysUpdated 缓存同步更新的 key 数量
	CacheSyncKeysUpdated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_sync_keys_updated_total",
			Help:      "缓存同步更新的 key 总数",
		},
		[]string{"type"},
	)
)

// 告警通知指标
var (
	// AlertNotificationsTotal 告警通知总数
	AlertNotificationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "alert_notifications_total",
			Help:      "告警通知总数",
		},
		[]string{"type", "channel", "status"}, // type: reconciliation_diff/job_failed, channel: slack/email/webhook, status: sent/failed
	)
)

// 数据库指标
var (
	// DBQueryDuration 数据库查询耗时
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "数据库查询耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5},
		},
		[]string{"operation", "table"},
	)

	// DBConnectionsGauge 数据库连接数
	DBConnectionsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_connections",
			Help:      "数据库连接数",
		},
		[]string{"state"}, // idle, in_use
	)
)

// Redis 指标
var (
	// RedisOperationDuration Redis 操作耗时
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "redis_operation_duration_seconds",
			Help:      "Redis 操作耗时(秒)",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
		[]string{"operation"},
	)
)

// Helper functions

// RecordJobExecution 记录任务执行
func RecordJobExecution(jobName, status string, durationSeconds float64) {
	JobExecutionsTotal.WithLabelValues(jobName, status).Inc()
	JobDuration.WithLabelValues(jobName).Observe(durationSeconds)
	JobLastExecutionTime.WithLabelValues(jobName).SetToCurrentTime()
}

// UpdateJobSchedule 更新任务调度信息
func UpdateJobSchedule(jobName string, nextExecutionTimestamp float64) {
	JobNextExecutionTime.WithLabelValues(jobName).Set(nextExecutionTimestamp)
}

// RecordReconciliation 记录对账
func RecordReconciliation(reconcType, result string, diffsCount int, durationSeconds float64, accountsChecked int) {
	ReconciliationTotal.WithLabelValues(reconcType, result).Inc()
	ReconciliationDuration.WithLabelValues(reconcType).Observe(durationSeconds)
	ReconciliationAccountsChecked.WithLabelValues(reconcType).Add(float64(accountsChecked))
}

// UpdateReconciliationDiffs 更新对账差异
func UpdateReconciliationDiffs(reconcType, token string, count int) {
	ReconciliationDiffs.WithLabelValues(reconcType, token).Set(float64(count))
}

// RecordArchive 记录归档
func RecordArchive(archiveType, status string, recordsProcessed int, durationSeconds float64) {
	ArchiveTotal.WithLabelValues(archiveType, status).Inc()
	ArchiveRecordsProcessed.WithLabelValues(archiveType).Add(float64(recordsProcessed))
	ArchiveDuration.WithLabelValues(archiveType).Observe(durationSeconds)
}

// UpdateArchiveStorage 更新归档存储大小
func UpdateArchiveStorage(archiveType string, bytes int64) {
	ArchiveStorageBytes.WithLabelValues(archiveType).Set(float64(bytes))
}

// RecordCleanup 记录清理
func RecordCleanup(cleanupType, status string, recordsDeleted int, durationSeconds float64) {
	CleanupTotal.WithLabelValues(cleanupType, status).Inc()
	CleanupRecordsDeleted.WithLabelValues(cleanupType).Add(float64(recordsDeleted))
	CleanupDuration.WithLabelValues(cleanupType).Observe(durationSeconds)
}

// RecordStatsAggregation 记录统计聚合
func RecordStatsAggregation(statsType, status string, recordsGenerated int, durationSeconds float64) {
	StatsAggregationTotal.WithLabelValues(statsType, status).Inc()
	StatsRecordsGenerated.WithLabelValues(statsType).Add(float64(recordsGenerated))
	StatsAggregationDuration.WithLabelValues(statsType).Observe(durationSeconds)
}

// RecordCacheSync 记录缓存同步
func RecordCacheSync(syncType, status string, keysUpdated int, durationSeconds float64) {
	CacheSyncTotal.WithLabelValues(syncType, status).Inc()
	CacheSyncKeysUpdated.WithLabelValues(syncType).Add(float64(keysUpdated))
	CacheSyncDuration.WithLabelValues(syncType).Observe(durationSeconds)
}

// RecordAlertNotification 记录告警通知
func RecordAlertNotification(alertType, channel, status string) {
	AlertNotificationsTotal.WithLabelValues(alertType, channel, status).Inc()
}

// UpdateRunningJobs 更新运行中任务数
func UpdateRunningJobs(count int) {
	RunningJobsGauge.Set(float64(count))
}

// UpdateScheduledJobs 更新已调度任务数
func UpdateScheduledJobs(count int) {
	ScheduledJobsGauge.Set(float64(count))
}

// RecordDBQuery 记录数据库查询
func RecordDBQuery(operation, table string, durationSeconds float64) {
	DBQueryDuration.WithLabelValues(operation, table).Observe(durationSeconds)
}

// RecordRedisOperation 记录 Redis 操作
func RecordRedisOperation(operation string, durationSeconds float64) {
	RedisOperationDuration.WithLabelValues(operation).Observe(durationSeconds)
}

// UpdateDBConnections 更新数据库连接数
func UpdateDBConnections(idle, inUse int) {
	DBConnectionsGauge.WithLabelValues("idle").Set(float64(idle))
	DBConnectionsGauge.WithLabelValues("in_use").Set(float64(inUse))
}
