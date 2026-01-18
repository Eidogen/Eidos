// Package metrics 提供 eidos-api 服务的 Prometheus 监控指标
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "eidos_api"

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
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// HTTPRequestSize HTTP 请求大小
	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_size_bytes",
			Help:      "HTTP 请求大小(字节)",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"method", "path"},
	)

	// HTTPResponseSize HTTP 响应大小
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_response_size_bytes",
			Help:      "HTTP 响应大小(字节)",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"method", "path"},
	)
)

// WebSocket 指标
var (
	// WSConnectionsTotal 当前 WebSocket 连接数
	WSConnectionsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ws_connections_total",
			Help:      "当前 WebSocket 连接数",
		},
	)

	// WSConnectionsCreated WebSocket 连接创建总数
	WSConnectionsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ws_connections_created_total",
			Help:      "WebSocket 连接创建总数",
		},
	)

	// WSConnectionsClosed WebSocket 连接关闭总数
	WSConnectionsClosed = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ws_connections_closed_total",
			Help:      "WebSocket 连接关闭总数",
		},
	)

	// WSSubscriptionsTotal 当前 WebSocket 订阅数
	WSSubscriptionsTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ws_subscriptions_total",
			Help:      "当前 WebSocket 订阅数",
		},
		[]string{"channel"},
	)

	// WSMessagesReceived WebSocket 接收消息数
	WSMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ws_messages_received_total",
			Help:      "WebSocket 接收消息总数",
		},
		[]string{"type"},
	)

	// WSMessagesSent WebSocket 发送消息数
	WSMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ws_messages_sent_total",
			Help:      "WebSocket 发送消息总数",
		},
		[]string{"channel"},
	)

	// WSMessageDropped WebSocket 丢弃消息数（缓冲区满）
	WSMessagesDropped = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ws_messages_dropped_total",
			Help:      "WebSocket 丢弃消息总数（缓冲区满）",
		},
	)
)

// gRPC 客户端指标
var (
	// GRPCRequestsTotal gRPC 请求总数
	GRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_requests_total",
			Help:      "gRPC 请求总数",
		},
		[]string{"service", "method", "code"},
	)

	// GRPCRequestDuration gRPC 请求耗时
	GRPCRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "grpc_request_duration_seconds",
			Help:      "gRPC 请求耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"service", "method"},
	)
)

// Redis Pub/Sub 指标
var (
	// RedisMessagesReceived Redis Pub/Sub 接收消息数
	RedisMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "redis_pubsub_messages_received_total",
			Help:      "Redis Pub/Sub 接收消息总数",
		},
		[]string{"channel"},
	)

	// RedisMessageErrors Redis Pub/Sub 消息处理错误数
	RedisMessageErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "redis_pubsub_message_errors_total",
			Help:      "Redis Pub/Sub 消息处理错误数",
		},
		[]string{"channel", "reason"},
	)
)

// 认证指标
var (
	// AuthRequestsTotal 认证请求总数
	AuthRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_requests_total",
			Help:      "认证请求总数",
		},
		[]string{"result"}, // success, failed, skipped
	)

	// AuthLatency 认证耗时
	AuthLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "auth_latency_seconds",
			Help:      "认证耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
	)
)

// 限流指标
var (
	// RateLimitRequestsTotal 限流请求总数
	RateLimitRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ratelimit_requests_total",
			Help:      "限流请求总数",
		},
		[]string{"type", "result"}, // type: ip/wallet, result: allowed/rejected
	)
)

// RecordHTTPRequest 记录 HTTP 请求指标
func RecordHTTPRequest(method, path, status string, durationSeconds float64, reqSize, respSize int64) {
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(durationSeconds)
	if reqSize > 0 {
		HTTPRequestSize.WithLabelValues(method, path).Observe(float64(reqSize))
	}
	if respSize > 0 {
		HTTPResponseSize.WithLabelValues(method, path).Observe(float64(respSize))
	}
}

// RecordGRPCRequest 记录 gRPC 请求指标
func RecordGRPCRequest(service, method, code string, durationSeconds float64) {
	GRPCRequestsTotal.WithLabelValues(service, method, code).Inc()
	GRPCRequestDuration.WithLabelValues(service, method).Observe(durationSeconds)
}

// RecordWSConnection 记录 WebSocket 连接
func RecordWSConnection(connected bool) {
	if connected {
		WSConnectionsTotal.Inc()
		WSConnectionsCreated.Inc()
	} else {
		WSConnectionsTotal.Dec()
		WSConnectionsClosed.Inc()
	}
}

// RecordWSSubscription 记录 WebSocket 订阅
func RecordWSSubscription(channel string, subscribed bool) {
	if subscribed {
		WSSubscriptionsTotal.WithLabelValues(channel).Inc()
	} else {
		WSSubscriptionsTotal.WithLabelValues(channel).Dec()
	}
}

// RecordWSMessage 记录 WebSocket 消息
func RecordWSMessage(msgType string, sent bool) {
	if sent {
		WSMessagesSent.WithLabelValues(msgType).Inc()
	} else {
		WSMessagesReceived.WithLabelValues(msgType).Inc()
	}
}

// RecordRedisMessage 记录 Redis Pub/Sub 消息
func RecordRedisMessage(channel string, success bool, reason string) {
	RedisMessagesReceived.WithLabelValues(channel).Inc()
	if !success {
		RedisMessageErrors.WithLabelValues(channel, reason).Inc()
	}
}

// RecordAuth 记录认证结果
func RecordAuth(result string, durationSeconds float64) {
	AuthRequestsTotal.WithLabelValues(result).Inc()
	AuthLatency.Observe(durationSeconds)
}

// RecordRateLimit 记录限流结果
func RecordRateLimit(limitType, result string) {
	RateLimitRequestsTotal.WithLabelValues(limitType, result).Inc()
}
