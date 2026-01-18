package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "eidos"
	subsystem = "market"
)

var (
	// ==================== Trade Processing ====================

	// TradesProcessedTotal 处理的成交总数
	TradesProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "trades_processed_total",
		Help:      "Total number of trades processed",
	}, []string{"market"})

	// TradeProcessingDuration 成交处理耗时
	TradeProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "trade_processing_duration_seconds",
		Help:      "Trade processing duration in seconds",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	}, []string{"market"})

	// TradeProcessingErrors 成交处理错误数
	TradeProcessingErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "trade_processing_errors_total",
		Help:      "Total number of trade processing errors",
	}, []string{"market", "error_type"})

	// ==================== K-line Aggregation ====================

	// KlinesGeneratedTotal 生成的 K 线总数
	KlinesGeneratedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "klines_generated_total",
		Help:      "Total number of klines generated",
	}, []string{"market", "interval"})

	// KlineFlushDuration K 线刷新耗时
	KlineFlushDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kline_flush_duration_seconds",
		Help:      "Kline flush duration in seconds",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	}, []string{"market"})

	// KlineFlushErrors K 线刷新错误数
	KlineFlushErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kline_flush_errors_total",
		Help:      "Total number of kline flush errors",
	}, []string{"market", "error_type"})

	// ==================== Depth Management ====================

	// DepthUpdatesTotal 深度更新总数
	DepthUpdatesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "depth_updates_total",
		Help:      "Total number of depth updates processed",
	}, []string{"market"})

	// DepthLevels 深度档位数量
	DepthLevels = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "depth_levels",
		Help:      "Current number of depth levels",
	}, []string{"market", "side"})

	// DepthSequenceGaps 深度序列号缺口数
	DepthSequenceGaps = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "depth_sequence_gaps_total",
		Help:      "Total number of depth sequence gaps detected",
	}, []string{"market"})

	// DepthSnapshotRequests 深度快照请求数
	DepthSnapshotRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "depth_snapshot_requests_total",
		Help:      "Total number of depth snapshot requests",
	}, []string{"market", "result"})

	// ==================== Ticker Calculation ====================

	// TickerUpdatesTotal Ticker 更新总数
	TickerUpdatesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "ticker_updates_total",
		Help:      "Total number of ticker updates",
	}, []string{"market"})

	// TickerBucketsActive 活跃的 Ticker 桶数
	TickerBucketsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "ticker_buckets_active",
		Help:      "Number of active ticker buckets",
	}, []string{"market"})

	// ==================== Kafka Consumer ====================

	// KafkaMessagesConsumed Kafka 消费消息总数
	KafkaMessagesConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kafka_messages_consumed_total",
		Help:      "Total number of Kafka messages consumed",
	}, []string{"topic"})

	// KafkaMessageProcessingDuration Kafka 消息处理耗时
	KafkaMessageProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kafka_message_processing_duration_seconds",
		Help:      "Kafka message processing duration in seconds",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5},
	}, []string{"topic"})

	// KafkaConsumerErrors Kafka 消费错误数
	KafkaConsumerErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kafka_consumer_errors_total",
		Help:      "Total number of Kafka consumer errors",
	}, []string{"topic", "error_type"})

	// KafkaConsumerRetries Kafka 消费重试数
	KafkaConsumerRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kafka_consumer_retries_total",
		Help:      "Total number of Kafka consumer retries",
	}, []string{"topic"})

	// KafkaConsumerLag Kafka 消费延迟
	KafkaConsumerLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "kafka_consumer_lag",
		Help:      "Kafka consumer lag (messages behind)",
	}, []string{"topic", "partition"})

	// ==================== gRPC Handler ====================

	// GRPCRequestsTotal gRPC 请求总数
	GRPCRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "grpc_requests_total",
		Help:      "Total number of gRPC requests",
	}, []string{"method", "status"})

	// GRPCRequestDuration gRPC 请求耗时
	GRPCRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "grpc_request_duration_seconds",
		Help:      "gRPC request duration in seconds",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"method"})

	// ==================== Redis/PubSub ====================

	// PubSubPublishTotal PubSub 发布总数
	PubSubPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "pubsub_publish_total",
		Help:      "Total number of PubSub messages published",
	}, []string{"channel_type"})

	// PubSubPublishErrors PubSub 发布错误数
	PubSubPublishErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "pubsub_publish_errors_total",
		Help:      "Total number of PubSub publish errors",
	}, []string{"channel_type"})

	// CacheHits 缓存命中数
	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cache_hits_total",
		Help:      "Total number of cache hits",
	}, []string{"cache_type"})

	// CacheMisses 缓存未命中数
	CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cache_misses_total",
		Help:      "Total number of cache misses",
	}, []string{"cache_type"})

	// ==================== System ====================

	// ActiveMarkets 活跃市场数
	ActiveMarkets = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "active_markets",
		Help:      "Number of active markets",
	})

	// BackgroundTasksRunning 运行中的后台任务数
	BackgroundTasksRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "background_tasks_running",
		Help:      "Number of running background tasks",
	})
)
