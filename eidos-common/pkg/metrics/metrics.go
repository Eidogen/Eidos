// Package metrics 提供通用的 Prometheus 监控基础组件
// 业务特定的指标定义请在各服务的 internal/metrics 包中实现，
// 以保持微服务间的解耦。
package metrics

import (
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// Namespace 指标命名空间
	Namespace = "eidos"
)

// Standard RPC Metrics - 通用 RPC/HTTP 服务监控指标
var (
	// RequestDuration RPC/HTTP 请求耗时
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "request_duration_seconds",
			Help:      "RPC/HTTP 请求耗时(秒)",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "code"},
	)

	// RequestsTotal 请求总数
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "requests_total",
			Help:      "RPC/HTTP 请求总数",
		},
		[]string{"service", "method", "code"},
	)

	// RequestsInFlight 当前处理中的请求数
	RequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "requests_in_flight",
			Help:      "当前处理中的请求数",
		},
		[]string{"service", "method"},
	)

	// RequestSize 请求大小
	RequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "request_size_bytes",
			Help:      "请求大小(字节)",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"service", "method"},
	)

	// ResponseSize 响应大小
	ResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "response_size_bytes",
			Help:      "响应大小(字节)",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"service", "method"},
	)
)

// Database Metrics - 数据库监控指标
var (
	// DBQueryDuration 数据库查询耗时
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "db_query_duration_seconds",
			Help:      "数据库查询耗时(秒)",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"operation", "table", "status"},
	)

	// DBQueryTotal 数据库查询总数
	DBQueryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "db_queries_total",
			Help:      "数据库查询总数",
		},
		[]string{"operation", "table", "status"},
	)

	// DBConnectionsOpen 打开的数据库连接数
	DBConnectionsOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "db_connections_open",
			Help:      "打开的数据库连接数",
		},
		[]string{"database"},
	)

	// DBConnectionsIdle 空闲的数据库连接数
	DBConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "db_connections_idle",
			Help:      "空闲的数据库连接数",
		},
		[]string{"database"},
	)

	// DBConnectionsInUse 使用中的数据库连接数
	DBConnectionsInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "db_connections_in_use",
			Help:      "使用中的数据库连接数",
		},
		[]string{"database"},
	)
)

// Cache Metrics - 缓存监控指标
// 注意: Redis 连接池指标已在 eidos-common/pkg/infra 包中定义，使用 infra.NewPoolMetricsCollector() 收集
var (
	// CacheHits 缓存命中数
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "cache_hits_total",
			Help:      "缓存命中总数",
		},
		[]string{"cache", "operation"},
	)

	// CacheMisses 缓存未命中数
	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "cache_misses_total",
			Help:      "缓存未命中总数",
		},
		[]string{"cache", "operation"},
	)

	// CacheLatency 缓存操作延迟
	CacheLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "cache_operation_duration_seconds",
			Help:      "缓存操作延迟(秒)",
			Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1},
		},
		[]string{"cache", "operation"},
	)
)

// Message Queue Metrics - 消息队列监控指标
var (
	// MQMessagesProduced 生产的消息数
	MQMessagesProduced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "mq_messages_produced_total",
			Help:      "生产的消息总数",
		},
		[]string{"topic", "status"},
	)

	// MQMessagesConsumed 消费的消息数
	MQMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "mq_messages_consumed_total",
			Help:      "消费的消息总数",
		},
		[]string{"topic", "group", "status"},
	)

	// MQConsumerLag 消费者延迟
	MQConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "mq_consumer_lag",
			Help:      "消费者延迟(消息数)",
		},
		[]string{"topic", "group", "partition"},
	)

	// MQProcessingDuration 消息处理耗时
	MQProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "mq_processing_duration_seconds",
			Help:      "消息处理耗时(秒)",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"topic", "group"},
	)
)

// External Service Metrics - 外部服务监控指标
var (
	// ExternalRequestDuration 外部服务请求耗时
	ExternalRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "external_request_duration_seconds",
			Help:      "外部服务请求耗时(秒)",
			Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "status"},
	)

	// ExternalRequestTotal 外部服务请求总数
	ExternalRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "external_requests_total",
			Help:      "外部服务请求总数",
		},
		[]string{"service", "method", "status"},
	)

	// CircuitBreakerState 熔断器状态
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "circuit_breaker_state",
			Help:      "熔断器状态 (0=closed, 1=open, 2=half-open)",
		},
		[]string{"service"},
	)
)

// Business Metrics - 业务监控指标
var (
	// OrdersCreated 创建的订单数
	OrdersCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "orders_created_total",
			Help:      "创建的订单总数",
		},
		[]string{"market", "side", "type"},
	)

	// OrdersCancelled 取消的订单数
	OrdersCancelled = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "orders_cancelled_total",
			Help:      "取消的订单总数",
		},
		[]string{"market", "side", "reason"},
	)

	// TradeVolume 交易量
	TradeVolume = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "trade_volume_total",
			Help:      "交易量",
		},
		[]string{"market"},
	)

	// ActiveUsers 活跃用户数
	ActiveUsers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "active_users",
			Help:      "活跃用户数",
		},
		[]string{"type"},
	)
)

// Runtime Metrics - 运行时监控指标
var (
	// GoRoutines goroutine 数量
	GoRoutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "goroutines",
			Help:      "当前 goroutine 数量",
		},
	)

	// GoMemAlloc 内存分配
	GoMemAlloc = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "go_mem_alloc_bytes",
			Help:      "当前分配的内存(字节)",
		},
	)

	// GoMemSys 系统内存
	GoMemSys = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "go_mem_sys_bytes",
			Help:      "系统分配的内存(字节)",
		},
	)

	// GoGCPause GC 暂停时间
	GoGCPause = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "go_gc_pause_seconds",
			Help:      "GC 暂停时间(秒)",
			Buckets:   []float64{.00001, .00005, .0001, .0005, .001, .005, .01, .05, .1},
		},
	)
)

// RecordRequest 记录请求 (Helper function)
func RecordRequest(service, method, code string, durationSeconds float64) {
	RequestsTotal.WithLabelValues(service, method, code).Inc()
	RequestDuration.WithLabelValues(service, method, code).Observe(durationSeconds)
}

// RecordDBQuery 记录数据库查询
func RecordDBQuery(operation, table, status string, durationSeconds float64) {
	DBQueryTotal.WithLabelValues(operation, table, status).Inc()
	DBQueryDuration.WithLabelValues(operation, table, status).Observe(durationSeconds)
}

// RecordCacheOperation 记录缓存操作
func RecordCacheOperation(cache, operation string, hit bool, durationSeconds float64) {
	if hit {
		CacheHits.WithLabelValues(cache, operation).Inc()
	} else {
		CacheMisses.WithLabelValues(cache, operation).Inc()
	}
	CacheLatency.WithLabelValues(cache, operation).Observe(durationSeconds)
}

// RecordMQProduce 记录消息生产
func RecordMQProduce(topic, status string) {
	MQMessagesProduced.WithLabelValues(topic, status).Inc()
}

// RecordMQConsume 记录消息消费
func RecordMQConsume(topic, group, status string, durationSeconds float64) {
	MQMessagesConsumed.WithLabelValues(topic, group, status).Inc()
	MQProcessingDuration.WithLabelValues(topic, group).Observe(durationSeconds)
}

// RecordExternalRequest 记录外部服务请求
func RecordExternalRequest(service, method, status string, durationSeconds float64) {
	ExternalRequestTotal.WithLabelValues(service, method, status).Inc()
	ExternalRequestDuration.WithLabelValues(service, method, status).Observe(durationSeconds)
}

// UpdateRuntimeMetrics 更新运行时指标
func UpdateRuntimeMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	GoRoutines.Set(float64(runtime.NumGoroutine()))
	GoMemAlloc.Set(float64(m.Alloc))
	GoMemSys.Set(float64(m.Sys))
}

// StartRuntimeMetricsCollector 启动运行时指标收集器
func StartRuntimeMetricsCollector(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			UpdateRuntimeMetrics()
		}
	}()
}

// Handler 返回 Prometheus HTTP 处理器
func Handler() http.Handler {
	return promhttp.Handler()
}

// HandlerFor 返回自定义注册表的 HTTP 处理器
func HandlerFor(registry *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// Timer 计时器
type Timer struct {
	start time.Time
}

// NewTimer 创建计时器
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ObserveDuration 观察持续时间
func (t *Timer) ObserveDuration() time.Duration {
	return time.Since(t.start)
}

// ObserveSeconds 观察持续时间(秒)
func (t *Timer) ObserveSeconds() float64 {
	return t.ObserveDuration().Seconds()
}

// ObserveDurationAndRecord 观察并记录到 Histogram
func (t *Timer) ObserveDurationAndRecord(histogram prometheus.Observer) {
	histogram.Observe(t.ObserveSeconds())
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status    string            `json:"status"`
	Checks    map[string]bool   `json:"checks"`
	Details   map[string]string `json:"details,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	checks map[string]func() error
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make(map[string]func() error),
	}
}

// AddCheck 添加健康检查
func (h *HealthChecker) AddCheck(name string, check func() error) {
	h.checks[name] = check
}

// Check 执行健康检查
func (h *HealthChecker) Check() *HealthStatus {
	status := &HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]bool),
		Details:   make(map[string]string),
		Timestamp: time.Now(),
	}

	for name, check := range h.checks {
		if err := check(); err != nil {
			status.Checks[name] = false
			status.Details[name] = err.Error()
			status.Status = "unhealthy"
		} else {
			status.Checks[name] = true
		}
	}

	return status
}

// Handler 返回健康检查 HTTP 处理器
func (h *HealthChecker) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := h.Check()

		w.Header().Set("Content-Type", "application/json")
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		// 简单的 JSON 响应
		w.Write([]byte(`{"status":"` + status.Status + `"}`))
	})
}
