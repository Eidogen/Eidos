package kafka

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/proto"
)

// Header 消息头
type Header struct {
	Key   string
	Value []byte
}

// Message Kafka 消息
type Message struct {
	// Topic 主题
	Topic string
	// Key 消息键 (用于分区路由)
	Key []byte
	// Value 消息值
	Value []byte
	// Headers 消息头
	Headers []Header
	// Partition 分区 (可选，-1 表示自动选择)
	Partition int32
	// Offset 消息偏移量 (仅消费时有效)
	Offset int64
	// Timestamp 消息时间戳
	Timestamp time.Time

	// 以下字段用于消息追踪和重试
	// TraceID 追踪 ID
	TraceID string
	// RetryCount 重试次数
	RetryCount int
	// OriginalTopic 原始主题 (重试消息使用)
	OriginalTopic string
}

// Serializer 消息序列化器接口
type Serializer interface {
	// Serialize 序列化
	Serialize(v interface{}) ([]byte, error)
	// Deserialize 反序列化
	Deserialize(data []byte, v interface{}) error
	// ContentType 内容类型
	ContentType() string
}

// JSONSerializer JSON 序列化器
type JSONSerializer struct{}

// NewJSONSerializer 创建 JSON 序列化器
func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

// Serialize 序列化
func (s *JSONSerializer) Serialize(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Deserialize 反序列化
func (s *JSONSerializer) Deserialize(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// ContentType 内容类型
func (s *JSONSerializer) ContentType() string {
	return "application/json"
}

// ProtobufSerializer Protobuf 序列化器
type ProtobufSerializer struct{}

// NewProtobufSerializer 创建 Protobuf 序列化器
func NewProtobufSerializer() *ProtobufSerializer {
	return &ProtobufSerializer{}
}

// Serialize 序列化
func (s *ProtobufSerializer) Serialize(v interface{}) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		// 如果不是 protobuf 消息，fallback 到 JSON
		return json.Marshal(v)
	}
	return proto.Marshal(msg)
}

// Deserialize 反序列化
func (s *ProtobufSerializer) Deserialize(data []byte, v interface{}) error {
	msg, ok := v.(proto.Message)
	if !ok {
		// 如果不是 protobuf 消息，fallback 到 JSON
		return json.Unmarshal(data, v)
	}
	return proto.Unmarshal(data, msg)
}

// ContentType 内容类型
func (s *ProtobufSerializer) ContentType() string {
	return "application/x-protobuf"
}

// MessageBuilder 消息构建器
type MessageBuilder struct {
	msg        *Message
	serializer Serializer
}

// NewMessageBuilder 创建消息构建器
func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{
		msg: &Message{
			Partition: -1,
			Timestamp: time.Now(),
			Headers:   make([]Header, 0),
		},
		serializer: NewJSONSerializer(),
	}
}

// WithTopic 设置主题
func (b *MessageBuilder) WithTopic(topic string) *MessageBuilder {
	b.msg.Topic = topic
	return b
}

// WithKey 设置键
func (b *MessageBuilder) WithKey(key string) *MessageBuilder {
	b.msg.Key = []byte(key)
	return b
}

// WithKeyBytes 设置键 (字节)
func (b *MessageBuilder) WithKeyBytes(key []byte) *MessageBuilder {
	b.msg.Key = key
	return b
}

// WithValue 设置值 (会被序列化)
func (b *MessageBuilder) WithValue(value interface{}) *MessageBuilder {
	data, err := b.serializer.Serialize(value)
	if err != nil {
		// 记录错误但不中断构建
		b.msg.Value = nil
	} else {
		b.msg.Value = data
	}
	return b
}

// WithValueBytes 设置值 (原始字节)
func (b *MessageBuilder) WithValueBytes(value []byte) *MessageBuilder {
	b.msg.Value = value
	return b
}

// WithHeader 添加消息头
func (b *MessageBuilder) WithHeader(key string, value []byte) *MessageBuilder {
	b.msg.Headers = append(b.msg.Headers, Header{Key: key, Value: value})
	return b
}

// WithHeaderString 添加字符串消息头
func (b *MessageBuilder) WithHeaderString(key, value string) *MessageBuilder {
	return b.WithHeader(key, []byte(value))
}

// WithPartition 设置分区
func (b *MessageBuilder) WithPartition(partition int32) *MessageBuilder {
	b.msg.Partition = partition
	return b
}

// WithTimestamp 设置时间戳
func (b *MessageBuilder) WithTimestamp(ts time.Time) *MessageBuilder {
	b.msg.Timestamp = ts
	return b
}

// WithTraceID 设置追踪 ID
func (b *MessageBuilder) WithTraceID(traceID string) *MessageBuilder {
	b.msg.TraceID = traceID
	b.WithHeaderString("trace_id", traceID)
	return b
}

// WithSerializer 设置序列化器
func (b *MessageBuilder) WithSerializer(s Serializer) *MessageBuilder {
	b.serializer = s
	return b
}

// Build 构建消息
func (b *MessageBuilder) Build() *Message {
	// 添加内容类型头
	b.WithHeaderString("content_type", b.serializer.ContentType())
	return b.msg
}

// GetHeader 获取消息头值
func (m *Message) GetHeader(key string) ([]byte, bool) {
	for _, h := range m.Headers {
		if h.Key == key {
			return h.Value, true
		}
	}
	return nil, false
}

// GetHeaderString 获取字符串消息头值
func (m *Message) GetHeaderString(key string) (string, bool) {
	if v, ok := m.GetHeader(key); ok {
		return string(v), true
	}
	return "", false
}

// Clone 克隆消息
func (m *Message) Clone() *Message {
	clone := &Message{
		Topic:         m.Topic,
		Partition:     m.Partition,
		Offset:        m.Offset,
		Timestamp:     m.Timestamp,
		TraceID:       m.TraceID,
		RetryCount:    m.RetryCount,
		OriginalTopic: m.OriginalTopic,
	}

	if m.Key != nil {
		clone.Key = make([]byte, len(m.Key))
		copy(clone.Key, m.Key)
	}

	if m.Value != nil {
		clone.Value = make([]byte, len(m.Value))
		copy(clone.Value, m.Value)
	}

	if m.Headers != nil {
		clone.Headers = make([]Header, len(m.Headers))
		for i, h := range m.Headers {
			clone.Headers[i] = Header{
				Key:   h.Key,
				Value: make([]byte, len(h.Value)),
			}
			copy(clone.Headers[i].Value, h.Value)
		}
	}

	return clone
}

// BatchMessages 批量消息
type BatchMessages struct {
	Messages []*Message
	Topic    string
}

// NewBatchMessages 创建批量消息
func NewBatchMessages(topic string, messages ...*Message) *BatchMessages {
	for _, msg := range messages {
		if msg.Topic == "" {
			msg.Topic = topic
		}
	}
	return &BatchMessages{
		Topic:    topic,
		Messages: messages,
	}
}

// Add 添加消息
func (b *BatchMessages) Add(msg *Message) {
	if msg.Topic == "" {
		msg.Topic = b.Topic
	}
	b.Messages = append(b.Messages, msg)
}

// Size 获取消息数量
func (b *BatchMessages) Size() int {
	return len(b.Messages)
}
