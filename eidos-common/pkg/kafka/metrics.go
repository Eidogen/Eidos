package kafka

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Kafka 相关 Prometheus 指标
var (
	// Producer 指标

	// ProducerMessagesSent 生产者发送消息总数
	ProducerMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_messages_sent_total",
			Help:      "Total number of messages sent by producer",
		},
		[]string{"topic", "client_id"},
	)

	// ProducerMessagesSucceeded 生产者发送成功消息数
	ProducerMessagesSucceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_messages_succeeded_total",
			Help:      "Total number of messages successfully sent by producer",
		},
		[]string{"topic", "client_id"},
	)

	// ProducerMessagesFailed 生产者发送失败消息数
	ProducerMessagesFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_messages_failed_total",
			Help:      "Total number of messages failed to send by producer",
		},
		[]string{"topic", "client_id", "error"},
	)

	// ProducerBytesSent 生产者发送字节数
	ProducerBytesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_bytes_sent_total",
			Help:      "Total bytes sent by producer",
		},
		[]string{"topic", "client_id"},
	)

	// ProducerBatchesSent 生产者发送批次数
	ProducerBatchesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_batches_sent_total",
			Help:      "Total number of batches sent by producer",
		},
		[]string{"topic", "client_id"},
	)

	// ProducerRetryCount 生产者重试次数
	ProducerRetryCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_retry_total",
			Help:      "Total number of retries by producer",
		},
		[]string{"topic", "client_id"},
	)

	// ProducerSendDuration 生产者发送延迟
	ProducerSendDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "producer_send_duration_seconds",
			Help:      "Time spent sending messages",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"topic", "client_id"},
	)

	// Consumer 指标

	// ConsumerMessagesReceived 消费者接收消息总数
	ConsumerMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_messages_received_total",
			Help:      "Total number of messages received by consumer",
		},
		[]string{"topic", "group_id", "partition"},
	)

	// ConsumerMessagesProcessed 消费者处理成功消息数
	ConsumerMessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_messages_processed_total",
			Help:      "Total number of messages successfully processed by consumer",
		},
		[]string{"topic", "group_id", "partition"},
	)

	// ConsumerMessagesFailed 消费者处理失败消息数
	ConsumerMessagesFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_messages_failed_total",
			Help:      "Total number of messages failed to process by consumer",
		},
		[]string{"topic", "group_id", "partition", "error"},
	)

	// ConsumerMessagesRetried 消费者重试消息数
	ConsumerMessagesRetried = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_messages_retried_total",
			Help:      "Total number of messages retried by consumer",
		},
		[]string{"topic", "group_id"},
	)

	// ConsumerMessagesDeadLetter 消费者死信消息数
	ConsumerMessagesDeadLetter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_messages_dead_letter_total",
			Help:      "Total number of messages sent to dead letter topic",
		},
		[]string{"topic", "group_id"},
	)

	// ConsumerBytesReceived 消费者接收字节数
	ConsumerBytesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_bytes_received_total",
			Help:      "Total bytes received by consumer",
		},
		[]string{"topic", "group_id", "partition"},
	)

	// ConsumerCommitCount 消费者提交次数
	ConsumerCommitCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_commit_total",
			Help:      "Total number of offset commits by consumer",
		},
		[]string{"topic", "group_id"},
	)

	// ConsumerRebalanceCount 消费者重平衡次数
	ConsumerRebalanceCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_rebalance_total",
			Help:      "Total number of rebalances",
		},
		[]string{"group_id"},
	)

	// ConsumerProcessingDuration 消费者消息处理延迟
	ConsumerProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_processing_duration_seconds",
			Help:      "Time spent processing messages",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"topic", "group_id"},
	)

	// ConsumerLag 消费者消息积压
	ConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_lag",
			Help:      "Current consumer lag (high watermark - committed offset)",
		},
		[]string{"topic", "group_id", "partition"},
	)

	// ConsumerCurrentOffset 消费者当前 offset
	ConsumerCurrentOffset = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "consumer_current_offset",
			Help:      "Current offset being consumed",
		},
		[]string{"topic", "group_id", "partition"},
	)

	// Connection 指标

	// KafkaConnectionsActive 活跃连接数
	KafkaConnectionsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "connections_active",
			Help:      "Number of active Kafka connections",
		},
		[]string{"client_id", "broker"},
	)

	// KafkaConnectionErrors 连接错误数
	KafkaConnectionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "eidos",
			Subsystem: "kafka",
			Name:      "connection_errors_total",
			Help:      "Total number of connection errors",
		},
		[]string{"client_id", "broker", "error"},
	)
)

// MetricsCollector Kafka 指标收集器
type MetricsCollector struct {
	clientID string
	groupID  string
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector(clientID, groupID string) *MetricsCollector {
	return &MetricsCollector{
		clientID: clientID,
		groupID:  groupID,
	}
}

// RecordProducerSend 记录生产者发送
func (c *MetricsCollector) RecordProducerSend(topic string, bytes int64, success bool, err error, duration float64) {
	ProducerMessagesSent.WithLabelValues(topic, c.clientID).Inc()
	ProducerSendDuration.WithLabelValues(topic, c.clientID).Observe(duration)

	if success {
		ProducerMessagesSucceeded.WithLabelValues(topic, c.clientID).Inc()
		ProducerBytesSent.WithLabelValues(topic, c.clientID).Add(float64(bytes))
	} else {
		errStr := "unknown"
		if err != nil {
			errStr = err.Error()
		}
		ProducerMessagesFailed.WithLabelValues(topic, c.clientID, errStr).Inc()
	}
}

// RecordProducerBatch 记录生产者批量发送
func (c *MetricsCollector) RecordProducerBatch(topic string) {
	ProducerBatchesSent.WithLabelValues(topic, c.clientID).Inc()
}

// RecordProducerRetry 记录生产者重试
func (c *MetricsCollector) RecordProducerRetry(topic string) {
	ProducerRetryCount.WithLabelValues(topic, c.clientID).Inc()
}

// RecordConsumerReceive 记录消费者接收
func (c *MetricsCollector) RecordConsumerReceive(topic string, partition int32, bytes int64) {
	partitionStr := formatPartition(partition)
	ConsumerMessagesReceived.WithLabelValues(topic, c.groupID, partitionStr).Inc()
	ConsumerBytesReceived.WithLabelValues(topic, c.groupID, partitionStr).Add(float64(bytes))
}

// RecordConsumerProcess 记录消费者处理
func (c *MetricsCollector) RecordConsumerProcess(topic string, partition int32, success bool, err error, duration float64) {
	partitionStr := formatPartition(partition)
	ConsumerProcessingDuration.WithLabelValues(topic, c.groupID).Observe(duration)

	if success {
		ConsumerMessagesProcessed.WithLabelValues(topic, c.groupID, partitionStr).Inc()
	} else {
		errStr := "unknown"
		if err != nil {
			errStr = err.Error()
		}
		ConsumerMessagesFailed.WithLabelValues(topic, c.groupID, partitionStr, errStr).Inc()
	}
}

// RecordConsumerRetry 记录消费者重试
func (c *MetricsCollector) RecordConsumerRetry(topic string) {
	ConsumerMessagesRetried.WithLabelValues(topic, c.groupID).Inc()
}

// RecordConsumerDeadLetter 记录消费者死信
func (c *MetricsCollector) RecordConsumerDeadLetter(topic string) {
	ConsumerMessagesDeadLetter.WithLabelValues(topic, c.groupID).Inc()
}

// RecordConsumerCommit 记录消费者提交
func (c *MetricsCollector) RecordConsumerCommit(topic string) {
	ConsumerCommitCount.WithLabelValues(topic, c.groupID).Inc()
}

// RecordConsumerRebalance 记录消费者重平衡
func (c *MetricsCollector) RecordConsumerRebalance() {
	ConsumerRebalanceCount.WithLabelValues(c.groupID).Inc()
}

// SetConsumerLag 设置消费者积压
func (c *MetricsCollector) SetConsumerLag(topic string, partition int32, lag int64) {
	partitionStr := formatPartition(partition)
	ConsumerLag.WithLabelValues(topic, c.groupID, partitionStr).Set(float64(lag))
}

// SetConsumerOffset 设置消费者当前 offset
func (c *MetricsCollector) SetConsumerOffset(topic string, partition int32, offset int64) {
	partitionStr := formatPartition(partition)
	ConsumerCurrentOffset.WithLabelValues(topic, c.groupID, partitionStr).Set(float64(offset))
}

// RecordConnectionError 记录连接错误
func (c *MetricsCollector) RecordConnectionError(broker string, err error) {
	errStr := "unknown"
	if err != nil {
		errStr = err.Error()
	}
	KafkaConnectionErrors.WithLabelValues(c.clientID, broker, errStr).Inc()
}

// SetActiveConnections 设置活跃连接数
func (c *MetricsCollector) SetActiveConnections(broker string, count int) {
	KafkaConnectionsActive.WithLabelValues(c.clientID, broker).Set(float64(count))
}

// formatPartition 格式化分区号
func formatPartition(partition int32) string {
	return fmt.Sprintf("%d", partition)
}
