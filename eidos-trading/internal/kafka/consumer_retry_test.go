package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProducer implements Producer for testing
type MockProducer struct {
	mock.Mock
}

func (m *MockProducer) SendWithContext(ctx context.Context, topic string, key, value []byte) error {
	args := m.Called(ctx, topic, key, value)
	return args.Error(0)
}

// TestHandler for testing
type TestHandler struct {
	handleFunc func(ctx context.Context, msg *Message) error
	callCount  int
}

func (h *TestHandler) Handle(ctx context.Context, msg *Message) error {
	h.callCount++
	return h.handleFunc(ctx, msg)
}

func (h *TestHandler) HandleWithRetry(ctx context.Context, msg *Message) error {
	return h.Handle(ctx, msg)
}

func TestRetryableError(t *testing.T) {
	t.Run("retryable error", func(t *testing.T) {
		err := NewRetryableError(errors.New("temporary error"))
		assert.True(t, IsRetryable(err))
		assert.Equal(t, "temporary error", err.Error())
	})

	t.Run("non-retryable error", func(t *testing.T) {
		err := NewNonRetryableError(errors.New("permanent error"))
		assert.False(t, IsRetryable(err))
		assert.Equal(t, "permanent error", err.Error())
	})

	t.Run("regular error is retryable by default", func(t *testing.T) {
		err := errors.New("some error")
		assert.True(t, IsRetryable(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.True(t, IsRetryable(nil))
	})
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, cfg.InitialBackoff)
	assert.Equal(t, 5*time.Second, cfg.MaxBackoff)
	assert.Equal(t, 2.0, cfg.BackoffFactor)
}

func TestRetryConsumerGroup_CalculateBackoff(t *testing.T) {
	rcg := &RetryConsumerGroup{
		retryConfig: &RetryConfig{
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     5 * time.Second,
			BackoffFactor:  2.0,
		},
	}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{10, 5 * time.Second}, // Capped at max
	}

	for _, tc := range testCases {
		result := rcg.calculateBackoff(tc.attempt)
		assert.Equal(t, tc.expected, result, "attempt %d", tc.attempt)
	}
}

func TestRetryConsumerGroup_HandleWithRetry_Success(t *testing.T) {
	producer := new(MockProducer)
	rcg := &RetryConsumerGroup{
		retryConfig: DefaultRetryConfig(),
		producer:    producer,
	}

	handler := &TestHandler{
		handleFunc: func(ctx context.Context, msg *Message) error {
			return nil // Success
		},
	}

	ctx := context.Background()
	msg := &Message{
		Topic:     "test-topic",
		Key:       []byte("key"),
		Value:     []byte("value"),
		Partition: 0,
		Offset:    1,
	}

	err := rcg.handleWithRetry(ctx, msg, handler)

	assert.NoError(t, err)
	assert.Equal(t, 1, handler.callCount)
}

func TestRetryConsumerGroup_HandleWithRetry_SuccessAfterRetry(t *testing.T) {
	producer := new(MockProducer)
	rcg := &RetryConsumerGroup{
		retryConfig: &RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			BackoffFactor:  2.0,
		},
		producer: producer,
	}

	attempts := 0
	handler := &TestHandler{
		handleFunc: func(ctx context.Context, msg *Message) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil // Success on 3rd attempt
		},
	}

	ctx := context.Background()
	msg := &Message{
		Topic:     "test-topic",
		Key:       []byte("key"),
		Value:     []byte("value"),
		Partition: 0,
		Offset:    1,
	}

	err := rcg.handleWithRetry(ctx, msg, handler)

	assert.NoError(t, err)
	assert.Equal(t, 3, handler.callCount)
}

func TestRetryConsumerGroup_HandleWithRetry_MaxRetriesExceeded(t *testing.T) {
	producer := new(MockProducer)
	producer.On("SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything).Return(nil)

	rcg := &RetryConsumerGroup{
		retryConfig: &RetryConfig{
			MaxRetries:     2,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			BackoffFactor:  2.0,
		},
		producer:        producer,
		deadLetterTopic: TopicDeadLetter,
	}

	handler := &TestHandler{
		handleFunc: func(ctx context.Context, msg *Message) error {
			return errors.New("permanent error")
		},
	}

	ctx := context.Background()
	msg := &Message{
		Topic:     "test-topic",
		Key:       []byte("key"),
		Value:     []byte("value"),
		Partition: 0,
		Offset:    1,
	}

	err := rcg.handleWithRetry(ctx, msg, handler)

	assert.NoError(t, err) // Returns nil after sending to DLQ
	assert.Equal(t, 3, handler.callCount) // Initial + 2 retries
	producer.AssertCalled(t, "SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything)
}

func TestRetryConsumerGroup_HandleWithRetry_NonRetryableError(t *testing.T) {
	producer := new(MockProducer)
	producer.On("SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything).Return(nil)

	rcg := &RetryConsumerGroup{
		retryConfig:     DefaultRetryConfig(),
		producer:        producer,
		deadLetterTopic: TopicDeadLetter,
	}

	handler := &TestHandler{
		handleFunc: func(ctx context.Context, msg *Message) error {
			return NewNonRetryableError(errors.New("non-retryable"))
		},
	}

	ctx := context.Background()
	msg := &Message{
		Topic:     "test-topic",
		Key:       []byte("key"),
		Value:     []byte("value"),
		Partition: 0,
		Offset:    1,
	}

	err := rcg.handleWithRetry(ctx, msg, handler)

	assert.NoError(t, err) // Returns nil after sending to DLQ
	assert.Equal(t, 1, handler.callCount) // No retries for non-retryable
	producer.AssertCalled(t, "SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything)
}

func TestRetryConsumerGroup_HandleWithRetry_ContextCancelled(t *testing.T) {
	producer := new(MockProducer)
	rcg := &RetryConsumerGroup{
		retryConfig: &RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 1 * time.Second, // Long backoff
			MaxBackoff:     5 * time.Second,
			BackoffFactor:  2.0,
		},
		producer: producer,
	}

	handler := &TestHandler{
		handleFunc: func(ctx context.Context, msg *Message) error {
			return errors.New("temporary error")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	msg := &Message{
		Topic:     "test-topic",
		Key:       []byte("key"),
		Value:     []byte("value"),
		Partition: 0,
		Offset:    1,
	}

	err := rcg.handleWithRetry(ctx, msg, handler)

	assert.Error(t, err) // Should return context error
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestRetryConsumerGroup_SendToDeadLetter_NoProducer(t *testing.T) {
	rcg := &RetryConsumerGroup{
		retryConfig: DefaultRetryConfig(),
		producer:    nil, // No producer
	}

	ctx := context.Background()
	msg := &Message{
		Topic: "test-topic",
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	// Should not panic, just log warning
	rcg.sendToDeadLetter(ctx, msg, 1, errors.New("test error"))
}

func TestRetryConsumerGroup_SendToDeadLetter_ProducerError(t *testing.T) {
	producer := new(MockProducer)
	producer.On("SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything).Return(errors.New("producer error"))

	rcg := &RetryConsumerGroup{
		retryConfig:     DefaultRetryConfig(),
		producer:        producer,
		deadLetterTopic: TopicDeadLetter,
	}

	ctx := context.Background()
	msg := &Message{
		Topic: "test-topic",
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	// Should not panic, just log error
	rcg.sendToDeadLetter(ctx, msg, 1, errors.New("test error"))

	producer.AssertCalled(t, "SendWithContext", mock.Anything, TopicDeadLetter, mock.Anything, mock.Anything)
}

func TestDeadLetterMessage_Serialization(t *testing.T) {
	dlqMsg := &DeadLetterMessage{
		OriginalTopic: "test-topic",
		Key:           []byte("key"),
		Value:         []byte("value"),
		Partition:     1,
		Offset:        100,
		RetryCount:    3,
		LastError:     "some error",
		FirstFailedAt: 1000,
		LastFailedAt:  2000,
	}

	assert.Equal(t, "test-topic", dlqMsg.OriginalTopic)
	assert.Equal(t, []byte("key"), dlqMsg.Key)
	assert.Equal(t, []byte("value"), dlqMsg.Value)
	assert.Equal(t, int32(1), dlqMsg.Partition)
	assert.Equal(t, int64(100), dlqMsg.Offset)
	assert.Equal(t, 3, dlqMsg.RetryCount)
	assert.Equal(t, "some error", dlqMsg.LastError)
}

func TestRetryableHandlerAdapter(t *testing.T) {
	baseHandler := HandlerFunc(func(ctx context.Context, msg *Message) error {
		return nil
	})

	adapter := NewRetryableHandlerAdapter(baseHandler)

	ctx := context.Background()
	msg := &Message{Topic: "test"}

	err := adapter.Handle(ctx, msg)
	assert.NoError(t, err)

	err = adapter.HandleWithRetry(ctx, msg)
	assert.NoError(t, err)
}

func TestUnwrapRetryableError(t *testing.T) {
	originalErr := errors.New("original error")
	retryErr := NewRetryableError(originalErr)

	// Unwrap should return the original error
	assert.Equal(t, originalErr, errors.Unwrap(retryErr))

	// errors.Is should work
	assert.True(t, errors.Is(retryErr, originalErr))
}
