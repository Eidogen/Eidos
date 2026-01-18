// Package metrics 提供 eidos-risk 服务的 Prometheus 监控指标
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "eidos_risk"

// 风控检查指标
var (
	// OrderChecksTotal 订单风控检查总数
	OrderChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "order_checks_total",
			Help:      "订单风控检查总数",
		},
		[]string{"result", "rule"}, // result: passed/rejected/warning, rule: blacklist/ratelimit/amount/selftrade/price
	)

	// OrderCheckDuration 订单风控检查耗时
	OrderCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "order_check_duration_seconds",
			Help:      "订单风控检查耗时(秒)",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
		[]string{"market"},
	)

	// WithdrawChecksTotal 提现风控检查总数
	WithdrawChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "withdraw_checks_total",
			Help:      "提现风控检查总数",
		},
		[]string{"result", "rule"}, // result: passed/rejected/manual_review
	)

	// WithdrawCheckDuration 提现风控检查耗时
	WithdrawCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "withdraw_check_duration_seconds",
			Help:      "提现风控检查耗时(秒)",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
		[]string{"token"},
	)
)

// 规则引擎指标
var (
	// RuleChecksTotal 规则检查总数
	RuleChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rule_checks_total",
			Help:      "规则检查总数",
		},
		[]string{"rule_name", "result"},
	)

	// RuleCheckDuration 单个规则检查耗时
	RuleCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "rule_check_duration_seconds",
			Help:      "单个规则检查耗时(秒)",
			Buckets:   []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01},
		},
		[]string{"rule_name"},
	)

	// ActiveRulesGauge 当前活跃规则数
	ActiveRulesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_rules_total",
			Help:      "当前活跃规则数",
		},
	)
)

// 黑名单指标
var (
	// BlacklistEntriesGauge 黑名单条目数
	BlacklistEntriesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "blacklist_entries_total",
			Help:      "黑名单条目数",
		},
		[]string{"type"}, // full, withdraw, trade
	)

	// BlacklistHitsTotal 黑名单命中总数
	BlacklistHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "blacklist_hits_total",
			Help:      "黑名单命中总数",
		},
		[]string{"type"},
	)
)

// 速率限制指标
var (
	// RateLimitHitsTotal 速率限制命中总数
	RateLimitHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ratelimit_hits_total",
			Help:      "速率限制命中总数",
		},
		[]string{"type"}, // order_per_second, order_per_minute, cancel_per_minute
	)

	// RateLimitCurrentGauge 当前速率
	RateLimitCurrentGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ratelimit_current",
			Help:      "当前速率计数",
		},
		[]string{"wallet", "type"},
	)
)

// 金额限制指标
var (
	// AmountLimitHitsTotal 金额限制命中总数
	AmountLimitHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "amount_limit_hits_total",
			Help:      "金额限制命中总数",
		},
		[]string{"type"}, // single_order, daily_withdraw, single_withdraw
	)

	// LargeTradeAlertsTotal 大额交易告警总数
	LargeTradeAlertsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "large_trade_alerts_total",
			Help:      "大额交易告警总数",
		},
	)
)

// 自成交检查指标
var (
	// SelfTradeChecksTotal 自成交检查总数
	SelfTradeChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "selftrade_checks_total",
			Help:      "自成交检查总数",
		},
		[]string{"result"}, // passed, rejected
	)

	// PendingOrdersGauge 用户待成交订单数
	PendingOrdersGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_orders_total",
			Help:      "用户待成交订单数",
		},
		[]string{"market"},
	)
)

// 价格检查指标
var (
	// PriceDeviationChecksTotal 价格偏离检查总数
	PriceDeviationChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "price_deviation_checks_total",
			Help:      "价格偏离检查总数",
		},
		[]string{"market", "result"}, // passed, warning, rejected
	)

	// PriceDeviationHistogram 价格偏离分布
	PriceDeviationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "price_deviation_percent",
			Help:      "价格偏离百分比分布",
			Buckets:   []float64{0.01, 0.02, 0.03, 0.05, 0.07, 0.1, 0.15, 0.2, 0.3},
		},
		[]string{"market"},
	)
)

// 风险告警指标
var (
	// RiskAlertsTotal 风险告警总数
	RiskAlertsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "risk_alerts_total",
			Help:      "风险告警总数",
		},
		[]string{"type", "severity"}, // type: order_rejected/withdraw_blocked/large_trade, severity: info/warning/critical
	)

	// RiskScoreHistogram 风险评分分布
	RiskScoreHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "risk_score_distribution",
			Help:      "风险评分分布",
			Buckets:   []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		},
	)
)

// gRPC 服务指标
var (
	// GRPCRequestsTotal gRPC 请求总数
	GRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_requests_total",
			Help:      "gRPC 请求总数",
		},
		[]string{"method", "code"},
	)

	// GRPCRequestDuration gRPC 请求耗时
	GRPCRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "grpc_request_duration_seconds",
			Help:      "gRPC 请求耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"method"},
	)
)

// Kafka 消费指标
var (
	// KafkaMessagesConsumed Kafka 消费消息数
	KafkaMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_consumed_total",
			Help:      "Kafka 消费消息总数",
		},
		[]string{"topic"},
	)

	// KafkaMessagesProduced Kafka 生产消息数
	KafkaMessagesProduced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_produced_total",
			Help:      "Kafka 生产消息总数",
		},
		[]string{"topic"},
	)

	// KafkaConsumerLag Kafka 消费延迟
	KafkaConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "kafka_consumer_lag",
			Help:      "Kafka 消费延迟",
		},
		[]string{"topic", "partition"},
	)
)

// Helper functions

// RecordOrderCheck 记录订单风控检查
func RecordOrderCheck(result, rule, market string, durationSeconds float64) {
	OrderChecksTotal.WithLabelValues(result, rule).Inc()
	OrderCheckDuration.WithLabelValues(market).Observe(durationSeconds)
}

// RecordWithdrawCheck 记录提现风控检查
func RecordWithdrawCheck(result, rule, token string, durationSeconds float64) {
	WithdrawChecksTotal.WithLabelValues(result, rule).Inc()
	WithdrawCheckDuration.WithLabelValues(token).Observe(durationSeconds)
}

// RecordRuleCheck 记录规则检查
func RecordRuleCheck(ruleName, result string, durationSeconds float64) {
	RuleChecksTotal.WithLabelValues(ruleName, result).Inc()
	RuleCheckDuration.WithLabelValues(ruleName).Observe(durationSeconds)
}

// RecordBlacklistHit 记录黑名单命中
func RecordBlacklistHit(listType string) {
	BlacklistHitsTotal.WithLabelValues(listType).Inc()
}

// RecordRateLimitHit 记录速率限制命中
func RecordRateLimitHit(limitType string) {
	RateLimitHitsTotal.WithLabelValues(limitType).Inc()
}

// RecordAmountLimitHit 记录金额限制命中
func RecordAmountLimitHit(limitType string) {
	AmountLimitHitsTotal.WithLabelValues(limitType).Inc()
}

// RecordSelfTradeCheck 记录自成交检查
func RecordSelfTradeCheck(rejected bool) {
	result := "passed"
	if rejected {
		result = "rejected"
	}
	SelfTradeChecksTotal.WithLabelValues(result).Inc()
}

// RecordPriceDeviation 记录价格偏离
func RecordPriceDeviation(market, result string, deviationPercent float64) {
	PriceDeviationChecksTotal.WithLabelValues(market, result).Inc()
	PriceDeviationHistogram.WithLabelValues(market).Observe(deviationPercent)
}

// RecordRiskAlert 记录风险告警
func RecordRiskAlert(alertType, severity string) {
	RiskAlertsTotal.WithLabelValues(alertType, severity).Inc()
}

// RecordRiskScore 记录风险评分
func RecordRiskScore(score int) {
	RiskScoreHistogram.Observe(float64(score))
}

// RecordGRPCRequest 记录 gRPC 请求
func RecordGRPCRequest(method, code string, durationSeconds float64) {
	GRPCRequestsTotal.WithLabelValues(method, code).Inc()
	GRPCRequestDuration.WithLabelValues(method).Observe(durationSeconds)
}

// RecordKafkaMessage 记录 Kafka 消息
func RecordKafkaMessage(topic string, produced bool) {
	if produced {
		KafkaMessagesProduced.WithLabelValues(topic).Inc()
	} else {
		KafkaMessagesConsumed.WithLabelValues(topic).Inc()
	}
}
