// Package metrics 提供 eidos-admin 服务的 Prometheus 监控指标
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "eidos_admin"

// HTTP 请求指标
var (
	// HTTPRequestsTotal HTTP 请求总数
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "HTTP 请求总数",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration HTTP 请求耗时
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP 请求耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)
)

// 认证指标
var (
	// LoginAttemptsTotal 登录尝试总数
	LoginAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "login_attempts_total",
			Help:      "登录尝试总数",
		},
		[]string{"result"}, // success, failed, locked
	)

	// LoginDuration 登录耗时
	LoginDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "login_duration_seconds",
			Help:      "登录处理耗时(秒)",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
	)

	// ActiveSessionsGauge 活跃会话数
	ActiveSessionsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_sessions_total",
			Help:      "当前活跃会话数",
		},
	)

	// TokenValidationsTotal Token 验证总数
	TokenValidationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "token_validations_total",
			Help:      "Token 验证总数",
		},
		[]string{"result"}, // valid, invalid, expired
	)
)

// 管理员操作指标
var (
	// AdminOperationsTotal 管理员操作总数
	AdminOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "admin_operations_total",
			Help:      "管理员操作总数",
		},
		[]string{"operation", "resource"}, // operation: create/update/delete, resource: admin/market/config
	)

	// AdminsGauge 管理员总数
	AdminsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "admins_total",
			Help:      "管理员总数",
		},
		[]string{"role", "status"}, // role: super_admin/operator/support/viewer, status: active/disabled
	)
)

// 交易对配置指标
var (
	// MarketConfigOperationsTotal 交易对配置操作总数
	MarketConfigOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "market_config_operations_total",
			Help:      "交易对配置操作总数",
		},
		[]string{"operation"}, // create, update, status_change, delete
	)

	// MarketsGauge 交易对总数
	MarketsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "markets_total",
			Help:      "交易对总数",
		},
		[]string{"status"}, // active, suspended, offline
	)

	// ConfigVersionGauge 配置版本号
	ConfigVersionGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "config_version",
			Help:      "配置版本号",
		},
		[]string{"scope"}, // market_configs, system_configs
	)
)

// 系统配置指标
var (
	// SystemConfigOperationsTotal 系统配置操作总数
	SystemConfigOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "system_config_operations_total",
			Help:      "系统配置操作总数",
		},
		[]string{"operation", "category"},
	)

	// SystemConfigsGauge 系统配置项总数
	SystemConfigsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "system_configs_total",
			Help:      "系统配置项总数",
		},
		[]string{"category"},
	)
)

// 审计日志指标
var (
	// AuditLogsTotal 审计日志总数
	AuditLogsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "audit_logs_total",
			Help:      "审计日志总数",
		},
		[]string{"action", "resource_type", "status"},
	)

	// AuditLogQueriesTotal 审计日志查询总数
	AuditLogQueriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "audit_log_queries_total",
			Help:      "审计日志查询总数",
		},
	)
)

// 统计查询指标
var (
	// StatsQueriesTotal 统计查询总数
	StatsQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stats_queries_total",
			Help:      "统计查询总数",
		},
		[]string{"type"}, // overview, daily, trading, users, settlements
	)

	// StatsQueryDuration 统计查询耗时
	StatsQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "stats_query_duration_seconds",
			Help:      "统计查询耗时(秒)",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		},
		[]string{"type"},
	)
)

// 权限检查指标
var (
	// PermissionChecksTotal 权限检查总数
	PermissionChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "permission_checks_total",
			Help:      "权限检查总数",
		},
		[]string{"permission", "result"}, // result: allowed, denied
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
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
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

// Helper functions

// RecordHTTPRequest 记录 HTTP 请求
func RecordHTTPRequest(method, path, status string, durationSeconds float64) {
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(durationSeconds)
}

// RecordLogin 记录登录
func RecordLogin(result string, durationSeconds float64) {
	LoginAttemptsTotal.WithLabelValues(result).Inc()
	LoginDuration.Observe(durationSeconds)
}

// RecordTokenValidation 记录 Token 验证
func RecordTokenValidation(result string) {
	TokenValidationsTotal.WithLabelValues(result).Inc()
}

// RecordAdminOperation 记录管理员操作
func RecordAdminOperation(operation, resource string) {
	AdminOperationsTotal.WithLabelValues(operation, resource).Inc()
}

// RecordMarketConfigOperation 记录交易对配置操作
func RecordMarketConfigOperation(operation string) {
	MarketConfigOperationsTotal.WithLabelValues(operation).Inc()
}

// RecordSystemConfigOperation 记录系统配置操作
func RecordSystemConfigOperation(operation, category string) {
	SystemConfigOperationsTotal.WithLabelValues(operation, category).Inc()
}

// RecordAuditLog 记录审计日志
func RecordAuditLog(action, resourceType, status string) {
	AuditLogsTotal.WithLabelValues(action, resourceType, status).Inc()
}

// RecordStatsQuery 记录统计查询
func RecordStatsQuery(queryType string, durationSeconds float64) {
	StatsQueriesTotal.WithLabelValues(queryType).Inc()
	StatsQueryDuration.WithLabelValues(queryType).Observe(durationSeconds)
}

// RecordPermissionCheck 记录权限检查
func RecordPermissionCheck(permission string, allowed bool) {
	result := "allowed"
	if !allowed {
		result = "denied"
	}
	PermissionChecksTotal.WithLabelValues(permission, result).Inc()
}

// RecordDBQuery 记录数据库查询
func RecordDBQuery(operation, table string, durationSeconds float64) {
	DBQueryDuration.WithLabelValues(operation, table).Observe(durationSeconds)
}

// UpdateAdminsGauge 更新管理员数量
func UpdateAdminsGauge(role, status string, count float64) {
	AdminsGauge.WithLabelValues(role, status).Set(count)
}

// UpdateMarketsGauge 更新交易对数量
func UpdateMarketsGauge(status string, count float64) {
	MarketsGauge.WithLabelValues(status).Set(count)
}

// UpdateConfigVersion 更新配置版本
func UpdateConfigVersion(scope string, version float64) {
	ConfigVersionGauge.WithLabelValues(scope).Set(version)
}
