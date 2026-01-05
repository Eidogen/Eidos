// Package metrics 提供通用的 Prometheus 监控基础组件
// 业务特定的指标定义请在各服务的 internal/metrics 包中实现，
// 以保持微服务间的解耦。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Standard RPC Metrics - 通用 RPC/HTTP 服务监控指标
var (
	// RequestDuration RPC/HTTP 请求耗时
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Name:      "request_duration_seconds",
			Help:      "RPC/HTTP 请求耗时(秒)",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"service", "method", "code"},
	)

	// RequestsTotal 请求总数
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Name:      "requests_total",
			Help:      "RPC/HTTP 请求总数",
		},
		[]string{"service", "method", "code"},
	)
)

// RecordRequest 记录请求 (Helper function)
func RecordRequest(service, method, code string, durationSeconds float64) {
	RequestsTotal.WithLabelValues(service, method, code).Inc()
	RequestDuration.WithLabelValues(service, method, code).Observe(durationSeconds)
}
