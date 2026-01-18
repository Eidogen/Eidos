// Package metrics Prometheus 监控指标定义
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "eidos"
	subsystem = "matching"
)

var (
	// ========== 订单处理指标 ==========

	// OrdersReceived 接收到的订单总数
	OrdersReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "orders_received_total",
			Help:      "Total number of orders received from Kafka",
		},
		[]string{"market", "side", "type"},
	)

	// OrdersProcessed 处理完成的订单总数
	OrdersProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "orders_processed_total",
			Help:      "Total number of orders processed",
		},
		[]string{"market", "status"}, // status: matched, queued, rejected
	)

	// OrderProcessLatency 订单处理延迟（从接收到处理完成）
	OrderProcessLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "order_process_latency_seconds",
			Help:      "Order processing latency in seconds",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}, // 0.1ms ~ 1s
		},
		[]string{"market"},
	)

	// ========== 撮合引擎指标 ==========

	// MatchesTotal 撮合成交总数
	MatchesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "matches_total",
			Help:      "Total number of matches (trades) executed",
		},
		[]string{"market"},
	)

	// MatchLatency 单次撮合耗时
	MatchLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "match_latency_seconds",
			Help:      "Time taken for a single match operation",
			Buckets:   []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01}, // 10µs ~ 10ms
		},
		[]string{"market"},
	)

	// TradeVolume 成交量（基础代币数量）
	TradeVolume = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "trade_volume_total",
			Help:      "Total trade volume in base token",
		},
		[]string{"market"},
	)

	// TradeValue 成交额（计价代币数量）
	TradeValue = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "trade_value_total",
			Help:      "Total trade value in quote token",
		},
		[]string{"market"},
	)

	// ========== 订单簿指标 ==========

	// OrderBookDepth 订单簿深度（买卖双方价格档位数）
	OrderBookDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "orderbook_depth",
			Help:      "Number of price levels in orderbook",
		},
		[]string{"market", "side"}, // side: bid, ask
	)

	// OrderBookOrders 订单簿中的订单数量
	OrderBookOrders = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "orderbook_orders",
			Help:      "Number of orders in orderbook",
		},
		[]string{"market", "side"},
	)

	// OrderBookSpread 买卖价差（百分比）
	OrderBookSpread = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "orderbook_spread_percent",
			Help:      "Bid-ask spread as percentage",
		},
		[]string{"market"},
	)

	// BestBidPrice 最佳买价
	BestBidPrice = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "best_bid_price",
			Help:      "Best bid price in orderbook",
		},
		[]string{"market"},
	)

	// BestAskPrice 最佳卖价
	BestAskPrice = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "best_ask_price",
			Help:      "Best ask price in orderbook",
		},
		[]string{"market"},
	)

	// ========== 取消订单指标 ==========

	// CancelsReceived 接收到的取消请求总数
	CancelsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cancels_received_total",
			Help:      "Total number of cancel requests received",
		},
		[]string{"market"},
	)

	// CancelsProcessed 处理完成的取消请求
	CancelsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cancels_processed_total",
			Help:      "Total number of cancel requests processed",
		},
		[]string{"market", "status"}, // status: success, not_found, already_filled
	)

	// ========== Kafka 指标 ==========

	// KafkaMessagesReceived Kafka 消息接收总数
	KafkaMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "kafka_messages_received_total",
			Help:      "Total number of Kafka messages received",
		},
		[]string{"topic"},
	)

	// KafkaMessagesSent Kafka 消息发送总数
	KafkaMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "kafka_messages_sent_total",
			Help:      "Total number of Kafka messages sent",
		},
		[]string{"topic"},
	)

	// KafkaErrors Kafka 错误计数
	KafkaErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "kafka_errors_total",
			Help:      "Total number of Kafka errors",
		},
		[]string{"topic", "type"}, // type: parse, send, consume
	)

	// KafkaConsumerLag Kafka 消费延迟
	KafkaConsumerLag = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "kafka_consumer_lag_seconds",
			Help:      "Kafka message consumption lag in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}, // 1ms ~ 10s
		},
		[]string{"topic"},
	)

	// ========== 快照指标 ==========

	// SnapshotSaveLatency 快照保存耗时
	SnapshotSaveLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "snapshot_save_latency_seconds",
			Help:      "Time taken to save snapshot",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10}, // 10ms ~ 10s
		},
		[]string{"market"},
	)

	// SnapshotLoadLatency 快照加载耗时
	SnapshotLoadLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "snapshot_load_latency_seconds",
			Help:      "Time taken to load snapshot",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
		[]string{"market"},
	)

	// SnapshotErrors 快照错误计数
	SnapshotErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "snapshot_errors_total",
			Help:      "Total number of snapshot errors",
		},
		[]string{"market", "operation"}, // operation: save, load
	)

	// ========== 引擎状态指标 ==========

	// EngineRunning 引擎运行状态
	EngineRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "engine_running",
			Help:      "Whether the matching engine is running (1=running, 0=stopped)",
		},
		[]string{"market"},
	)

	// EngineQueueSize 引擎队列大小
	EngineQueueSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "engine_queue_size",
			Help:      "Number of pending orders in engine queue",
		},
		[]string{"market"},
	)

	// ChannelOverflow 通道溢出计数 (消息丢失)
	ChannelOverflow = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "channel_overflow_total",
			Help:      "Total number of messages dropped due to channel overflow",
		},
		[]string{"market", "channel"}, // channel: trades, updates, cancels
	)

	// ========== gRPC 指标 ==========

	// GRPCRequestsTotal gRPC 请求总数
	GRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "grpc_requests_total",
			Help:      "Total number of gRPC requests",
		},
		[]string{"method", "code"},
	)

	// GRPCRequestLatency gRPC 请求延迟
	GRPCRequestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "grpc_request_latency_seconds",
			Help:      "gRPC request latency in seconds",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"method"},
	)
)

// RecordOrderReceived 记录接收到的订单
func RecordOrderReceived(market, side, orderType string) {
	OrdersReceived.WithLabelValues(market, side, orderType).Inc()
}

// RecordOrderProcessed 记录处理完成的订单
func RecordOrderProcessed(market, status string, latencySeconds float64) {
	OrdersProcessed.WithLabelValues(market, status).Inc()
	OrderProcessLatency.WithLabelValues(market).Observe(latencySeconds)
}

// RecordMatch 记录撮合成交
func RecordMatch(market string, volume, value float64, latencySeconds float64) {
	MatchesTotal.WithLabelValues(market).Inc()
	TradeVolume.WithLabelValues(market).Add(volume)
	TradeValue.WithLabelValues(market).Add(value)
	MatchLatency.WithLabelValues(market).Observe(latencySeconds)
}

// UpdateOrderBookMetrics 更新订单簿指标
func UpdateOrderBookMetrics(market string, bidLevels, askLevels, bidOrders, askOrders int, spreadPercent, bestBid, bestAsk float64) {
	OrderBookDepth.WithLabelValues(market, "bid").Set(float64(bidLevels))
	OrderBookDepth.WithLabelValues(market, "ask").Set(float64(askLevels))
	OrderBookOrders.WithLabelValues(market, "bid").Set(float64(bidOrders))
	OrderBookOrders.WithLabelValues(market, "ask").Set(float64(askOrders))
	OrderBookSpread.WithLabelValues(market).Set(spreadPercent)
	BestBidPrice.WithLabelValues(market).Set(bestBid)
	BestAskPrice.WithLabelValues(market).Set(bestAsk)
}

// RecordCancelReceived 记录接收到的取消请求
func RecordCancelReceived(market string) {
	CancelsReceived.WithLabelValues(market).Inc()
}

// RecordCancelProcessed 记录处理完成的取消请求
func RecordCancelProcessed(market, status string) {
	CancelsProcessed.WithLabelValues(market, status).Inc()
}

// RecordKafkaMessage 记录 Kafka 消息
func RecordKafkaMessage(topic string, received bool) {
	if received {
		KafkaMessagesReceived.WithLabelValues(topic).Inc()
	} else {
		KafkaMessagesSent.WithLabelValues(topic).Inc()
	}
}

// RecordKafkaError 记录 Kafka 错误
func RecordKafkaError(topic, errorType string) {
	KafkaErrors.WithLabelValues(topic, errorType).Inc()
}

// RecordKafkaLag 记录 Kafka 消费延迟
func RecordKafkaLag(topic string, lagSeconds float64) {
	KafkaConsumerLag.WithLabelValues(topic).Observe(lagSeconds)
}

// RecordSnapshotSave 记录快照保存
func RecordSnapshotSave(market string, latencySeconds float64, err error) {
	SnapshotSaveLatency.WithLabelValues(market).Observe(latencySeconds)
	if err != nil {
		SnapshotErrors.WithLabelValues(market, "save").Inc()
	}
}

// RecordSnapshotLoad 记录快照加载
func RecordSnapshotLoad(market string, latencySeconds float64, err error) {
	SnapshotLoadLatency.WithLabelValues(market).Observe(latencySeconds)
	if err != nil {
		SnapshotErrors.WithLabelValues(market, "load").Inc()
	}
}

// SetEngineRunning 设置引擎运行状态
func SetEngineRunning(market string, running bool) {
	val := 0.0
	if running {
		val = 1.0
	}
	EngineRunning.WithLabelValues(market).Set(val)
}

// SetEngineQueueSize 设置引擎队列大小
func SetEngineQueueSize(market string, size int) {
	EngineQueueSize.WithLabelValues(market).Set(float64(size))
}

// RecordGRPCRequest 记录 gRPC 请求
func RecordGRPCRequest(method, code string, latencySeconds float64) {
	GRPCRequestsTotal.WithLabelValues(method, code).Inc()
	GRPCRequestLatency.WithLabelValues(method).Observe(latencySeconds)
}

// RecordChannelOverflow 记录通道溢出 (消息丢失)
func RecordChannelOverflow(market, channel string) {
	ChannelOverflow.WithLabelValues(market, channel).Inc()
}
