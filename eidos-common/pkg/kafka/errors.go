package kafka

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	// ErrProducerClosed 生产者已关闭
	ErrProducerClosed = errors.New("kafka producer is closed")
	// ErrConsumerClosed 消费者已关闭
	ErrConsumerClosed = errors.New("kafka consumer is closed")
	// ErrMessageNil 消息为空
	ErrMessageNil = errors.New("message is nil")
	// ErrTopicRequired 主题必填
	ErrTopicRequired = errors.New("topic is required")
	// ErrBrokersRequired brokers 必填
	ErrBrokersRequired = errors.New("kafka brokers is required")
	// ErrGroupIDRequired 消费者组 ID 必填
	ErrGroupIDRequired = errors.New("consumer group id is required")
	// ErrHandlerRequired 消息处理函数必填
	ErrHandlerRequired = errors.New("message handler is required")
	// ErrTopicsRequired 主题列表必填
	ErrTopicsRequired = errors.New("topics is required")
	// ErrAlreadyStarted 已经启动
	ErrAlreadyStarted = errors.New("consumer already started")
	// ErrBatchChannelFull 批量通道已满
	ErrBatchChannelFull = errors.New("batch channel is full")
	// ErrSendTimeout 发送超时
	ErrSendTimeout = errors.New("send message timeout")
	// ErrMaxRetriesExceeded 超过最大重试次数
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// SendError 发送错误
type SendError struct {
	Topic     string
	Partition int32
	Offset    int64
	Err       error
}

// Error 实现 error 接口
func (e *SendError) Error() string {
	return fmt.Sprintf("send to topic %s partition %d failed: %v", e.Topic, e.Partition, e.Err)
}

// Unwrap 返回原始错误
func (e *SendError) Unwrap() error {
	return e.Err
}

// NewSendError 创建发送错误
func NewSendError(topic string, partition int32, offset int64, err error) *SendError {
	return &SendError{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
		Err:       err,
	}
}

// ConsumeError 消费错误
type ConsumeError struct {
	Topic     string
	Partition int32
	Offset    int64
	Err       error
}

// Error 实现 error 接口
func (e *ConsumeError) Error() string {
	return fmt.Sprintf("consume from topic %s partition %d offset %d failed: %v",
		e.Topic, e.Partition, e.Offset, e.Err)
}

// Unwrap 返回原始错误
func (e *ConsumeError) Unwrap() error {
	return e.Err
}

// NewConsumeError 创建消费错误
func NewConsumeError(topic string, partition int32, offset int64, err error) *ConsumeError {
	return &ConsumeError{
		Topic:     topic,
		Partition: partition,
		Offset:    offset,
		Err:       err,
	}
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 不可重试的错误
	if errors.Is(err, ErrProducerClosed) ||
		errors.Is(err, ErrConsumerClosed) ||
		errors.Is(err, ErrMessageNil) ||
		errors.Is(err, ErrTopicRequired) ||
		errors.Is(err, ErrBrokersRequired) ||
		errors.Is(err, ErrGroupIDRequired) ||
		errors.Is(err, ErrHandlerRequired) ||
		errors.Is(err, ErrTopicsRequired) {
		return false
	}

	// 其他错误默认可重试
	return true
}

// BatchSendError 批量发送错误
type BatchSendError struct {
	Errors []*SendError
}

// Error 实现 error 接口
func (e *BatchSendError) Error() string {
	if len(e.Errors) == 0 {
		return "batch send failed with unknown errors"
	}
	return fmt.Sprintf("batch send failed with %d errors, first: %v", len(e.Errors), e.Errors[0])
}

// FailedCount 返回失败数量
func (e *BatchSendError) FailedCount() int {
	return len(e.Errors)
}

// HasErrors 是否有错误
func (e *BatchSendError) HasErrors() bool {
	return len(e.Errors) > 0
}

// Add 添加错误
func (e *BatchSendError) Add(err *SendError) {
	e.Errors = append(e.Errors, err)
}

// NewBatchSendError 创建批量发送错误
func NewBatchSendError() *BatchSendError {
	return &BatchSendError{
		Errors: make([]*SendError, 0),
	}
}
