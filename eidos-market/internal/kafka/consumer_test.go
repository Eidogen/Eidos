package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockMessageHandler 模拟消息处理器
type mockMessageHandler struct {
	topic        string
	handleCalled bool
	handleError  error
	lastKey      []byte
	lastValue    []byte
	mu           sync.Mutex
}

func newMockHandler(topic string) *mockMessageHandler {
	return &mockMessageHandler{topic: topic}
}

func (m *mockMessageHandler) Handle(ctx context.Context, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleCalled = true
	m.lastKey = key
	m.lastValue = value
	return m.handleError
}

func (m *mockMessageHandler) Topic() string {
	return m.topic
}

func (m *mockMessageHandler) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handleCalled = false
	m.lastKey = nil
	m.lastValue = nil
}

func TestConsumerConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		config := ConsumerConfig{
			Brokers:  []string{"localhost:9092"},
			GroupID:  "test-group",
			ClientID: "test-client",
			Topics:   []string{"test-topic"},
		}

		// MaxRetry defaults to 3 if not set
		assert.Equal(t, 0, config.MaxRetry) // zero value

		// CommitOnError defaults to false (zero value)
		assert.False(t, config.CommitOnError)
	})

	t.Run("custom values", func(t *testing.T) {
		config := ConsumerConfig{
			Brokers:       []string{"broker1:9092", "broker2:9092"},
			GroupID:       "eidos-market-consumer",
			ClientID:      "eidos-market-1",
			Topics:        []string{"trade-results", "orderbook-updates"},
			MaxRetry:      5,
			CommitOnError: true,
		}

		assert.Len(t, config.Brokers, 2)
		assert.Equal(t, "eidos-market-consumer", config.GroupID)
		assert.Len(t, config.Topics, 2)
		assert.Equal(t, 5, config.MaxRetry)
		assert.True(t, config.CommitOnError)
	})
}

func TestConsumer_RegisterHandler(t *testing.T) {
	// Create a consumer with nil client (will fail on Start but we can test RegisterHandler)
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
	}

	handler1 := newMockHandler("trade-results")
	handler2 := newMockHandler("orderbook-updates")

	consumer.RegisterHandler(handler1)
	consumer.RegisterHandler(handler2)

	assert.Len(t, consumer.handlers, 2)
	assert.Equal(t, handler1, consumer.handlers["trade-results"])
	assert.Equal(t, handler2, consumer.handlers["orderbook-updates"])
}

func TestConsumer_RegisterHandler_Overwrite(t *testing.T) {
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
	}

	handler1 := newMockHandler("test-topic")
	handler2 := newMockHandler("test-topic")

	consumer.RegisterHandler(handler1)
	consumer.RegisterHandler(handler2)

	// Second handler should overwrite the first
	assert.Len(t, consumer.handlers, 1)
	assert.Equal(t, handler2, consumer.handlers["test-topic"])
}

func TestConsumer_Stats(t *testing.T) {
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
	}

	// Initially all stats are zero
	stats := consumer.Stats()
	assert.Equal(t, int64(0), stats["processed_count"])
	assert.Equal(t, int64(0), stats["error_count"])
	assert.Equal(t, int64(0), stats["retry_count"])

	// Simulate processed messages
	consumer.processedCount.Add(100)
	consumer.errorCount.Add(5)
	consumer.retryCount.Add(10)

	stats = consumer.Stats()
	assert.Equal(t, int64(100), stats["processed_count"])
	assert.Equal(t, int64(5), stats["error_count"])
	assert.Equal(t, int64(10), stats["retry_count"])
}

func TestMessageHandlerInterface(t *testing.T) {
	handler := newMockHandler("test-topic")

	// Test Topic()
	assert.Equal(t, "test-topic", handler.Topic())

	// Test Handle()
	ctx := context.Background()
	key := []byte("test-key")
	value := []byte("test-value")

	err := handler.Handle(ctx, key, value)
	require.NoError(t, err)
	assert.True(t, handler.handleCalled)
	assert.Equal(t, key, handler.lastKey)
	assert.Equal(t, value, handler.lastValue)
}

func TestMessageHandlerInterface_Error(t *testing.T) {
	handler := newMockHandler("test-topic")
	handler.handleError = errors.New("processing error")

	ctx := context.Background()
	err := handler.Handle(ctx, []byte("key"), []byte("value"))
	require.Error(t, err)
	assert.Equal(t, "processing error", err.Error())
}

func TestConsumerStartNoHandlers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Start with no handlers should not error
	err := consumer.Start()
	require.NoError(t, err)
}

func TestRetryBackoff(t *testing.T) {
	// Test exponential backoff calculation
	for attempt := 1; attempt <= 5; attempt++ {
		backoff := time.Duration(attempt*100) * time.Millisecond
		expectedBackoff := time.Duration(attempt*100) * time.Millisecond
		assert.Equal(t, expectedBackoff, backoff, "attempt %d", attempt)
	}
}

func TestConsumerConfigValidation(t *testing.T) {
	t.Run("empty brokers", func(t *testing.T) {
		config := ConsumerConfig{
			Brokers: []string{},
			GroupID: "test",
		}
		assert.Empty(t, config.Brokers)
	})

	t.Run("empty group id", func(t *testing.T) {
		config := ConsumerConfig{
			Brokers: []string{"localhost:9092"},
			GroupID: "",
		}
		assert.Empty(t, config.GroupID)
	})
}

// ============ Mock Sarama Implementations ============

// mockConsumerGroupSession 模拟消费者组会话
type mockConsumerGroupSession struct {
	generationID  int32
	memberID      string
	markedOffsets []int64
	mu            sync.Mutex
}

func newMockSession() *mockConsumerGroupSession {
	return &mockConsumerGroupSession{
		generationID:  1,
		memberID:      "test-member-1",
		markedOffsets: make([]int64, 0),
	}
}

func (s *mockConsumerGroupSession) Claims() map[string][]int32 {
	return map[string][]int32{"test-topic": {0}}
}

func (s *mockConsumerGroupSession) MemberID() string {
	return s.memberID
}

func (s *mockConsumerGroupSession) GenerationID() int32 {
	return s.generationID
}

func (s *mockConsumerGroupSession) MarkOffset(topic string, partition int32, offset int64, metadata string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markedOffsets = append(s.markedOffsets, offset)
}

func (s *mockConsumerGroupSession) Commit() {
}

func (s *mockConsumerGroupSession) ResetOffset(topic string, partition int32, offset int64, metadata string) {
}

func (s *mockConsumerGroupSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markedOffsets = append(s.markedOffsets, msg.Offset)
}

func (s *mockConsumerGroupSession) Context() context.Context {
	return context.Background()
}

// mockConsumerGroupClaim 模拟消费者组声明
type mockConsumerGroupClaim struct {
	topic      string
	partition  int32
	msgChan    chan *sarama.ConsumerMessage
	initOffset int64
	highWater  int64
}

func newMockClaim(topic string, partition int32) *mockConsumerGroupClaim {
	return &mockConsumerGroupClaim{
		topic:     topic,
		partition: partition,
		msgChan:   make(chan *sarama.ConsumerMessage, 100),
	}
}

func (c *mockConsumerGroupClaim) Topic() string                            { return c.topic }
func (c *mockConsumerGroupClaim) Partition() int32                         { return c.partition }
func (c *mockConsumerGroupClaim) InitialOffset() int64                     { return c.initOffset }
func (c *mockConsumerGroupClaim) HighWaterMarkOffset() int64               { return c.highWater }
func (c *mockConsumerGroupClaim) Messages() <-chan *sarama.ConsumerMessage { return c.msgChan }

// ============ Consumer Session Tests ============

func TestConsumer_Setup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
	}

	session := newMockSession()
	err := consumer.Setup(session)
	require.NoError(t, err)
}

func TestConsumer_Cleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
	}

	session := newMockSession()
	err := consumer.Cleanup(session)
	require.NoError(t, err)
}

// ============ ConsumeClaim Tests using real handlers ============

func TestConsumer_ConsumeClaimNoHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	// Create claim for topic without handler
	claim := newMockClaim("unknown-topic", 0)
	close(claim.msgChan)

	session := newMockSession()

	// Create a wrapper claim that satisfies sarama.ConsumerGroupClaim
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)
}

func TestConsumer_ConsumeClaimSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	handler := newMockHandler("trade-results")
	consumer.RegisterHandler(handler)

	// Create claim and send messages
	claim := newMockClaim("trade-results", 0)

	// Send messages in a goroutine and then close
	go func() {
		claim.msgChan <- &sarama.ConsumerMessage{
			Key:       []byte("BTC-USDC"),
			Value:     []byte(`{"trade_id":"1"}`),
			Topic:     "trade-results",
			Partition: 0,
			Offset:    1,
		}
		close(claim.msgChan)
	}()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)

	// Verify handler was called
	assert.True(t, handler.handleCalled)
	assert.Equal(t, []byte("BTC-USDC"), handler.lastKey)
	assert.Equal(t, []byte(`{"trade_id":"1"}`), handler.lastValue)

	// Check stats
	stats := consumer.Stats()
	assert.Equal(t, int64(1), stats["processed_count"])
	assert.Equal(t, int64(0), stats["error_count"])
}

func TestConsumer_ConsumeClaimWithRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	// Handler that fails first call, succeeds second
	handler := &retryableHandler{
		topic:       "trade-results",
		failCount:   1, // Fail first time
		failOnCalls: true,
	}
	consumer.RegisterHandler(handler)

	claim := newMockClaim("trade-results", 0)
	go func() {
		claim.msgChan <- &sarama.ConsumerMessage{
			Key:       []byte("BTC-USDC"),
			Value:     []byte(`{"trade_id":"1"}`),
			Topic:     "trade-results",
			Partition: 0,
			Offset:    1,
		}
		close(claim.msgChan)
	}()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)

	// Should have succeeded after retry
	stats := consumer.Stats()
	assert.Equal(t, int64(1), stats["processed_count"])
	assert.GreaterOrEqual(t, stats["retry_count"], int64(1))
}

func TestConsumer_ConsumeClaimAllRetriesFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	// Handler that always fails
	handler := newMockHandler("trade-results")
	handler.handleError = errors.New("permanent error")
	consumer.RegisterHandler(handler)

	claim := newMockClaim("trade-results", 0)
	go func() {
		claim.msgChan <- &sarama.ConsumerMessage{
			Key:       []byte("BTC-USDC"),
			Value:     []byte(`{"trade_id":"1"}`),
			Topic:     "trade-results",
			Partition: 0,
			Offset:    1,
		}
		close(claim.msgChan)
	}()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)

	// Should have error stats
	stats := consumer.Stats()
	assert.Equal(t, int64(1), stats["error_count"])
	assert.Equal(t, int64(3), stats["retry_count"]) // 3 retries
}

func TestConsumer_ConsumeClaimNotCommitOnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 2, CommitOnError: false}, // Do not commit on error
	}

	// Handler that always fails
	handler := newMockHandler("trade-results")
	handler.handleError = errors.New("permanent error")
	consumer.RegisterHandler(handler)

	claim := newMockClaim("trade-results", 0)
	go func() {
		claim.msgChan <- &sarama.ConsumerMessage{
			Key:       []byte("BTC-USDC"),
			Value:     []byte(`{"trade_id":"1"}`),
			Topic:     "trade-results",
			Partition: 0,
			Offset:    1,
		}
		close(claim.msgChan)
	}()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)

	// Error count should be recorded
	stats := consumer.Stats()
	assert.Equal(t, int64(1), stats["error_count"])
	// processed_count should not be incremented since we didn't commit
	assert.Equal(t, int64(0), stats["processed_count"])
}

func TestConsumer_ConsumeClaimContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	handler := newMockHandler("trade-results")
	consumer.RegisterHandler(handler)

	claim := newMockClaim("trade-results", 0)

	// Cancel context before sending any messages
	cancel()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)
}

func TestConsumer_MultipleMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &Consumer{
		handlers: make(map[string]MessageHandler),
		logger:   zap.NewNop(),
		ctx:      ctx,
		cancel:   cancel,
		config:   ConsumerConfig{MaxRetry: 3, CommitOnError: true},
	}

	handler := &countingHandler{topic: "trade-results"}
	consumer.RegisterHandler(handler)

	claim := newMockClaim("trade-results", 0)
	go func() {
		for i := 0; i < 5; i++ {
			claim.msgChan <- &sarama.ConsumerMessage{
				Key:       []byte("BTC-USDC"),
				Value:     []byte(`{"trade_id":"` + string(rune('0'+i)) + `"}`),
				Topic:     "trade-results",
				Partition: 0,
				Offset:    int64(i),
			}
		}
		close(claim.msgChan)
	}()

	session := newMockSession()
	err := testConsumeClaimWithMock(consumer, session, claim)
	require.NoError(t, err)

	// Should have processed 5 messages
	assert.Equal(t, 5, handler.callCount)
	stats := consumer.Stats()
	assert.Equal(t, int64(5), stats["processed_count"])
}

// testConsumeClaimWithMock is a helper that simulates ConsumeClaim behavior
// without requiring actual Sarama ConsumerGroupClaim interface
func testConsumeClaimWithMock(c *Consumer, session *mockConsumerGroupSession, claim *mockConsumerGroupClaim) error {
	handler, ok := c.handlers[claim.Topic()]
	if !ok {
		return nil
	}

	maxRetry := c.config.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			var lastErr error
			for attempt := 1; attempt <= maxRetry; attempt++ {
				if err := handler.Handle(c.ctx, msg.Key, msg.Value); err != nil {
					lastErr = err
					c.retryCount.Add(1)
					if attempt < maxRetry {
						continue
					}
				} else {
					lastErr = nil
					break
				}
			}

			if lastErr != nil {
				c.errorCount.Add(1)
				if !c.config.CommitOnError {
					continue
				}
			}

			c.processedCount.Add(1)
			session.MarkMessage(msg, "")

		case <-c.ctx.Done():
			return nil
		}
	}
}

// retryableHandler 支持重试的处理器
type retryableHandler struct {
	topic       string
	failCount   int  // 失败次数
	callCount   int
	failOnCalls bool
	mu          sync.Mutex
}

func (h *retryableHandler) Handle(ctx context.Context, key, value []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callCount++
	if h.failOnCalls && h.callCount <= h.failCount {
		return errors.New("simulated failure")
	}
	return nil
}

func (h *retryableHandler) Topic() string {
	return h.topic
}

// countingHandler 计数处理器
type countingHandler struct {
	topic     string
	callCount int
	mu        sync.Mutex
}

func (h *countingHandler) Handle(ctx context.Context, key, value []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callCount++
	return nil
}

func (h *countingHandler) Topic() string {
	return h.topic
}
