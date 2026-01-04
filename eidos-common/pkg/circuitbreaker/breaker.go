package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrCircuitOpen 熔断器打开
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// State 熔断器状态
type State int

const (
	// StateClosed 关闭状态 (正常)
	StateClosed State = iota
	// StateOpen 打开状态 (熔断)
	StateOpen
	// StateHalfOpen 半开状态 (尝试恢复)
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config 熔断器配置
type Config struct {
	// 失败阈值：连续失败多少次后打开熔断器
	FailureThreshold int
	// 成功阈值：半开状态下连续成功多少次后关闭熔断器
	SuccessThreshold int
	// 超时时间：熔断器打开后多久进入半开状态
	Timeout time.Duration
	// 半开状态最大请求数
	MaxHalfOpenRequests int
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 3,
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	config *Config

	mu               sync.RWMutex
	state            State
	failures         int
	successes        int
	lastFailureTime  time.Time
	halfOpenRequests int
}

// New 创建熔断器
func New(config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// currentState 获取当前状态 (内部使用，不加锁)
func (cb *CircuitBreaker) currentState() State {
	switch cb.state {
	case StateClosed:
		return StateClosed
	case StateOpen:
		// 检查是否应该进入半开状态
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			return StateHalfOpen
		}
		return StateOpen
	case StateHalfOpen:
		return StateHalfOpen
	default:
		return StateClosed
	}
}

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()
	switch state {
	case StateClosed:
		return nil
	case StateOpen:
		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.halfOpenRequests >= cb.config.MaxHalfOpenRequests {
			return ErrCircuitOpen
		}
		cb.halfOpenRequests++
		// 更新状态
		if cb.state == StateOpen {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.halfOpenRequests = 1
		}
		return nil
	default:
		return nil
	}
}

// Success 记录成功
func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateClosed:
		// 重置失败计数
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			// 恢复到关闭状态
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenRequests = 0
		}
	}
}

// Failure 记录失败
func (cb *CircuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			// 打开熔断器
			cb.state = StateOpen
			cb.lastFailureTime = time.Now()
		}
	case StateHalfOpen:
		// 半开状态下失败，回到打开状态
		cb.state = StateOpen
		cb.lastFailureTime = time.Now()
		cb.successes = 0
		cb.halfOpenRequests = 0
	}
}

// Execute 执行函数并自动记录结果
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn()
	if err != nil {
		cb.Failure()
		return err
	}

	cb.Success()
	return nil
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
}

// Stats 统计信息
type Stats struct {
	State            State
	Failures         int
	Successes        int
	HalfOpenRequests int
}

// Stats 获取统计信息
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		State:            cb.currentState(),
		Failures:         cb.failures,
		Successes:        cb.successes,
		HalfOpenRequests: cb.halfOpenRequests,
	}
}

// BreakerRegistry 熔断器注册表
type BreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   *Config
}

// NewRegistry 创建熔断器注册表
func NewRegistry(config *Config) *BreakerRegistry {
	if config == nil {
		config = DefaultConfig()
	}
	return &BreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get 获取或创建熔断器
func (r *BreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[name]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// 再次检查
	if cb, ok := r.breakers[name]; ok {
		return cb
	}

	cb := New(r.config)
	r.breakers[name] = cb
	return cb
}

// Execute 通过名称执行函数
func (r *BreakerRegistry) Execute(name string, fn func() error) error {
	return r.Get(name).Execute(fn)
}
