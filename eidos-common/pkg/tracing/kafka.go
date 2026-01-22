package tracing

import (
	"context"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// KafkaProducerTracer Kafka 生产者追踪器
type KafkaProducerTracer struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewKafkaProducerTracer 创建 Kafka 生产者追踪器
func NewKafkaProducerTracer(serviceName string) *KafkaProducerTracer {
	return &KafkaProducerTracer{
		tracer:     otel.Tracer(serviceName),
		propagator: otel.GetTextMapPropagator(),
	}
}

// StartProduceSpan 开始生产消息的 span
func (t *KafkaProducerTracer) StartProduceSpan(ctx context.Context, topic string, key []byte) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "kafka.produce",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKafka,
			semconv.MessagingDestinationName(topic),
			semconv.MessagingOperationPublish,
		),
	)

	if len(key) > 0 {
		span.SetAttributes(attribute.String("messaging.kafka.message.key", string(key)))
	}

	return ctx, span
}

// InjectContext 注入 trace context 到 Kafka 消息头
func (t *KafkaProducerTracer) InjectContext(ctx context.Context, headers []sarama.RecordHeader) []sarama.RecordHeader {
	carrier := &kafkaHeaderCarrier{headers: headers}
	t.propagator.Inject(ctx, carrier)
	return carrier.headers
}

// KafkaConsumerTracer Kafka 消费者追踪器
type KafkaConsumerTracer struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewKafkaConsumerTracer 创建 Kafka 消费者追踪器
func NewKafkaConsumerTracer(serviceName string) *KafkaConsumerTracer {
	return &KafkaConsumerTracer{
		tracer:     otel.Tracer(serviceName),
		propagator: otel.GetTextMapPropagator(),
	}
}

// StartConsumeSpan 开始消费消息的 span
func (t *KafkaConsumerTracer) StartConsumeSpan(ctx context.Context, topic string, partition int32, offset int64, headers []*sarama.RecordHeader) (context.Context, trace.Span) {
	// 从消息头提取 trace context
	carrier := &kafkaRecordHeaderCarrier{headers: headers}
	ctx = t.propagator.Extract(ctx, carrier)

	ctx, span := t.tracer.Start(ctx, "kafka.consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			semconv.MessagingSystemKafka,
			semconv.MessagingDestinationName(topic),
			semconv.MessagingOperationReceive,
			attribute.Int("messaging.kafka.partition", int(partition)),
			attribute.Int64("messaging.kafka.offset", offset),
		),
	)

	return ctx, span
}

// ExtractContext 从 Kafka 消息头提取 trace context
func (t *KafkaConsumerTracer) ExtractContext(ctx context.Context, headers []*sarama.RecordHeader) context.Context {
	carrier := &kafkaRecordHeaderCarrier{headers: headers}
	return t.propagator.Extract(ctx, carrier)
}

// kafkaHeaderCarrier 用于生产者 ([]sarama.RecordHeader)
type kafkaHeaderCarrier struct {
	headers []sarama.RecordHeader
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range c.headers {
		if string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	// 检查是否已存在
	for i, h := range c.headers {
		if string(h.Key) == key {
			c.headers[i].Value = []byte(value)
			return
		}
	}
	// 添加新的 header
	c.headers = append(c.headers, sarama.RecordHeader{
		Key:   []byte(key),
		Value: []byte(value),
	})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(c.headers))
	for i, h := range c.headers {
		keys[i] = string(h.Key)
	}
	return keys
}

// kafkaRecordHeaderCarrier 用于消费者 ([]*sarama.RecordHeader)
type kafkaRecordHeaderCarrier struct {
	headers []*sarama.RecordHeader
}

func (c *kafkaRecordHeaderCarrier) Get(key string) string {
	for _, h := range c.headers {
		if h != nil && string(h.Key) == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaRecordHeaderCarrier) Set(key, value string) {
	// 消费者端通常不需要 Set，但为了接口完整性实现
	for _, h := range c.headers {
		if h != nil && string(h.Key) == key {
			h.Value = []byte(value)
			return
		}
	}
	c.headers = append(c.headers, &sarama.RecordHeader{
		Key:   []byte(key),
		Value: []byte(value),
	})
}

func (c *kafkaRecordHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for _, h := range c.headers {
		if h != nil {
			keys = append(keys, string(h.Key))
		}
	}
	return keys
}
