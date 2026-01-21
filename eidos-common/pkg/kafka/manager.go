package kafka

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Manager Kafka 管理器 (管理多个生产者和消费者)
type Manager struct {
	producers map[string]*Producer
	consumers map[string]*Consumer

	producerMu sync.RWMutex
	consumerMu sync.RWMutex

	closed bool
}

// NewManager 创建 Kafka 管理器
func NewManager() *Manager {
	return &Manager{
		producers: make(map[string]*Producer),
		consumers: make(map[string]*Consumer),
	}
}

// RegisterProducer 注册生产者
func (m *Manager) RegisterProducer(name string, cfg *ProducerConfig) (*Producer, error) {
	m.producerMu.Lock()
	defer m.producerMu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("manager is closed")
	}

	if _, exists := m.producers[name]; exists {
		return nil, fmt.Errorf("producer %s already registered", name)
	}

	producer, err := NewProducer(cfg)
	if err != nil {
		return nil, fmt.Errorf("create producer %s failed: %w", name, err)
	}

	m.producers[name] = producer
	logger.Info("producer registered", zap.String("name", name))

	return producer, nil
}

// GetProducer 获取生产者
func (m *Manager) GetProducer(name string) (*Producer, bool) {
	m.producerMu.RLock()
	defer m.producerMu.RUnlock()

	producer, ok := m.producers[name]
	return producer, ok
}

// RegisterConsumer 注册消费者
func (m *Manager) RegisterConsumer(name string, cfg *ConsumerConfig) (*Consumer, error) {
	m.consumerMu.Lock()
	defer m.consumerMu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("manager is closed")
	}

	if _, exists := m.consumers[name]; exists {
		return nil, fmt.Errorf("consumer %s already registered", name)
	}

	consumer, err := NewConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("create consumer %s failed: %w", name, err)
	}

	m.consumers[name] = consumer
	logger.Info("consumer registered", zap.String("name", name))

	return consumer, nil
}

// GetConsumer 获取消费者
func (m *Manager) GetConsumer(name string) (*Consumer, bool) {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()

	consumer, ok := m.consumers[name]
	return consumer, ok
}

// StartConsumer 启动消费者
func (m *Manager) StartConsumer(ctx context.Context, name string, handler MessageHandler) error {
	consumer, ok := m.GetConsumer(name)
	if !ok {
		return fmt.Errorf("consumer %s not found", name)
	}

	consumer.SetHandler(handler)
	return consumer.Start(ctx)
}

// StartAllConsumers 启动所有消费者
func (m *Manager) StartAllConsumers(ctx context.Context) error {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()

	var errs []error
	for name, consumer := range m.consumers {
		if consumer.handler == nil {
			logger.Warn("consumer has no handler, skipping", zap.String("name", name))
			continue
		}
		if err := consumer.Start(ctx); err != nil {
			errs = append(errs, fmt.Errorf("start consumer %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("start consumers failed: %v", errs)
	}

	return nil
}

// ProducerNames 返回所有生产者名称
func (m *Manager) ProducerNames() []string {
	m.producerMu.RLock()
	defer m.producerMu.RUnlock()

	names := make([]string, 0, len(m.producers))
	for name := range m.producers {
		names = append(names, name)
	}
	return names
}

// ConsumerNames 返回所有消费者名称
func (m *Manager) ConsumerNames() []string {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()

	names := make([]string, 0, len(m.consumers))
	for name := range m.consumers {
		names = append(names, name)
	}
	return names
}

// ProducerMetrics 返回所有生产者指标
func (m *Manager) ProducerMetrics() map[string]*ProducerMetrics {
	m.producerMu.RLock()
	defer m.producerMu.RUnlock()

	metrics := make(map[string]*ProducerMetrics)
	for name, producer := range m.producers {
		metrics[name] = producer.Metrics()
	}
	return metrics
}

// ConsumerMetrics 返回所有消费者指标
func (m *Manager) ConsumerMetrics() map[string]*ConsumerMetrics {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()

	metrics := make(map[string]*ConsumerMetrics)
	for name, consumer := range m.consumers {
		metrics[name] = consumer.Metrics()
	}
	return metrics
}

// Close 关闭所有生产者和消费者
func (m *Manager) Close() error {
	m.producerMu.Lock()
	m.consumerMu.Lock()
	defer m.producerMu.Unlock()
	defer m.consumerMu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	logger.Info("closing kafka manager")

	var errs []error

	// 先关闭消费者
	for name, consumer := range m.consumers {
		if err := consumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close consumer %s: %w", name, err))
		} else {
			logger.Info("consumer closed", zap.String("name", name))
		}
	}

	// 再关闭生产者
	for name, producer := range m.producers {
		if err := producer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close producer %s: %w", name, err))
		} else {
			logger.Info("producer closed", zap.String("name", name))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close manager errors: %v", errs)
	}

	logger.Info("kafka manager closed")
	return nil
}

// HealthCheck 健康检查
type HealthCheck struct {
	Producers map[string]bool `json:"producers"`
	Consumers map[string]bool `json:"consumers"`
	Healthy   bool            `json:"healthy"`
}

// HealthCheck 执行健康检查
func (m *Manager) HealthCheck() *HealthCheck {
	m.producerMu.RLock()
	m.consumerMu.RLock()
	defer m.producerMu.RUnlock()
	defer m.consumerMu.RUnlock()

	check := &HealthCheck{
		Producers: make(map[string]bool),
		Consumers: make(map[string]bool),
		Healthy:   true,
	}

	for name, producer := range m.producers {
		healthy := !producer.IsClosed()
		check.Producers[name] = healthy
		if !healthy {
			check.Healthy = false
		}
	}

	for name, consumer := range m.consumers {
		healthy := !consumer.IsClosed()
		check.Consumers[name] = healthy
		if !healthy {
			check.Healthy = false
		}
	}

	return check
}

// 全局管理器 (可选使用)
var globalManager *Manager
var globalManagerOnce sync.Once

// Global 获取全局管理器
func Global() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}

// CloseGlobal 关闭全局管理器
func CloseGlobal() error {
	if globalManager != nil {
		return globalManager.Close()
	}
	return nil
}
