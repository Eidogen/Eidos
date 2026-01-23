package service

import (
	"context"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/circuitbreaker"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// DegradationMode 服务降级模式
type DegradationMode int

const (
	// ModeNormal 正常模式 - 所有功能可用
	ModeNormal DegradationMode = iota
	// ModeDegraded 降级模式 - 非关键功能禁用
	ModeDegraded
	// ModeSafeMode 安全模式 - 仅允许已知安全操作
	ModeSafeMode
	// ModeReadOnly 只读模式 - 禁止所有写操作
	ModeReadOnly
)

// String 返回模式名称
func (m DegradationMode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeDegraded:
		return "degraded"
	case ModeSafeMode:
		return "safe_mode"
	case ModeReadOnly:
		return "read_only"
	default:
		return "unknown"
	}
}

// DependencyStatus 依赖状态
type DependencyStatus struct {
	Name      string    // 依赖名称
	Healthy   bool      // 是否健康
	LastCheck time.Time // 最后检查时间
	LastError error     // 最后错误
	Latency   time.Duration // 最后延迟
}

// DegradationConfig 降级配置
type DegradationConfig struct {
	// 健康检查间隔
	HealthCheckInterval time.Duration
	// Redis 熔断配置
	RedisFailureThreshold int
	RedisSuccessThreshold int
	RedisTimeout          time.Duration
	// DB 熔断配置
	DBFailureThreshold int
	DBSuccessThreshold int
	DBTimeout          time.Duration
	// Kafka 熔断配置
	KafkaFailureThreshold int
	KafkaSuccessThreshold int
	KafkaTimeout          time.Duration
	// 风控策略
	FailSafe bool // true: 依赖失败时拒绝 (更安全), false: 依赖失败时放行 (更可用)
}

// DefaultDegradationConfig 默认降级配置
func DefaultDegradationConfig() *DegradationConfig {
	return &DegradationConfig{
		HealthCheckInterval:   10 * time.Second,
		RedisFailureThreshold: 5,
		RedisSuccessThreshold: 2,
		RedisTimeout:          30 * time.Second,
		DBFailureThreshold:    3,
		DBSuccessThreshold:    2,
		DBTimeout:             60 * time.Second,
		KafkaFailureThreshold: 5,
		KafkaSuccessThreshold: 2,
		KafkaTimeout:          30 * time.Second,
		FailSafe:              true, // 默认安全模式: 依赖失败时拒绝请求
	}
}

// DegradationService 服务降级管理服务
type DegradationService struct {
	mu sync.RWMutex

	// 当前模式
	mode DegradationMode

	// 依赖状态
	redisStatus DependencyStatus
	dbStatus    DependencyStatus
	kafkaStatus DependencyStatus

	// 熔断器
	redisBreaker *circuitbreaker.CircuitBreaker
	dbBreaker    *circuitbreaker.CircuitBreaker
	kafkaBreaker *circuitbreaker.CircuitBreaker

	// 本地缓存 (L2 cache for critical data)
	blacklistL2    map[string]bool // wallet -> isBlacklisted
	blacklistL2Mu  sync.RWMutex
	blacklistL2TTL time.Time

	// 依赖
	redis redis.UniversalClient
	db    *gorm.DB

	// 配置
	config *DegradationConfig

	// 停止信号
	stopCh chan struct{}
}

// NewDegradationService 创建降级服务
func NewDegradationService(redis redis.UniversalClient, db *gorm.DB, config *DegradationConfig) *DegradationService {
	if config == nil {
		config = DefaultDegradationConfig()
	}

	s := &DegradationService{
		mode:         ModeNormal,
		redis:        redis,
		db:           db,
		config:       config,
		blacklistL2:  make(map[string]bool),
		stopCh:       make(chan struct{}),
	}

	// 初始化熔断器
	s.redisBreaker = circuitbreaker.New(&circuitbreaker.Config{
		FailureThreshold: config.RedisFailureThreshold,
		SuccessThreshold: config.RedisSuccessThreshold,
		Timeout:          config.RedisTimeout,
	})

	s.dbBreaker = circuitbreaker.New(&circuitbreaker.Config{
		FailureThreshold: config.DBFailureThreshold,
		SuccessThreshold: config.DBSuccessThreshold,
		Timeout:          config.DBTimeout,
	})

	s.kafkaBreaker = circuitbreaker.New(&circuitbreaker.Config{
		FailureThreshold: config.KafkaFailureThreshold,
		SuccessThreshold: config.KafkaSuccessThreshold,
		Timeout:          config.KafkaTimeout,
	})

	return s
}

// Start 启动健康检查
func (s *DegradationService) Start(ctx context.Context) {
	go s.healthCheckLoop(ctx)
}

// Stop 停止服务
func (s *DegradationService) Stop() {
	close(s.stopCh)
}

// healthCheckLoop 健康检查循环
func (s *DegradationService) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	// 启动时立即检查
	s.checkDependencies(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkDependencies(ctx)
		}
	}
}

// checkDependencies 检查所有依赖
func (s *DegradationService) checkDependencies(ctx context.Context) {
	// 检查 Redis
	s.checkRedis(ctx)

	// 检查数据库
	s.checkDB(ctx)

	// 更新降级模式
	s.updateMode()
}

// checkRedis 检查 Redis 健康状态
func (s *DegradationService) checkRedis(ctx context.Context) {
	if s.redis == nil {
		s.setRedisStatus(false, nil, 0)
		return
	}

	start := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err := s.redis.Ping(checkCtx).Err()
	latency := time.Since(start)

	s.setRedisStatus(err == nil, err, latency)

	if err == nil {
		s.redisBreaker.Success()
	} else {
		s.redisBreaker.Failure()
		logger.Warn("redis health check failed",
			"error", err,
			"latency_ms", latency.Milliseconds())
	}
}

// checkDB 检查数据库健康状态
func (s *DegradationService) checkDB(ctx context.Context) {
	if s.db == nil {
		s.setDBStatus(false, nil, 0)
		return
	}

	start := time.Now()
	sqlDB, err := s.db.DB()
	if err != nil {
		s.setDBStatus(false, err, 0)
		s.dbBreaker.Failure()
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err = sqlDB.PingContext(checkCtx)
	latency := time.Since(start)

	s.setDBStatus(err == nil, err, latency)

	if err == nil {
		s.dbBreaker.Success()
	} else {
		s.dbBreaker.Failure()
		logger.Warn("database health check failed",
			"error", err,
			"latency_ms", latency.Milliseconds())
	}
}

// setRedisStatus 设置 Redis 状态
func (s *DegradationService) setRedisStatus(healthy bool, err error, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.redisStatus = DependencyStatus{
		Name:      "redis",
		Healthy:   healthy,
		LastCheck: time.Now(),
		LastError: err,
		Latency:   latency,
	}
}

// setDBStatus 设置数据库状态
func (s *DegradationService) setDBStatus(healthy bool, err error, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dbStatus = DependencyStatus{
		Name:      "database",
		Healthy:   healthy,
		LastCheck: time.Now(),
		LastError: err,
		Latency:   latency,
	}
}

// SetKafkaStatus 设置 Kafka 状态 (由 Kafka producer/consumer 调用)
func (s *DegradationService) SetKafkaStatus(healthy bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.kafkaStatus = DependencyStatus{
		Name:      "kafka",
		Healthy:   healthy,
		LastCheck: time.Now(),
		LastError: err,
	}

	if healthy {
		s.kafkaBreaker.Success()
	} else {
		s.kafkaBreaker.Failure()
	}
}

// updateMode 根据依赖状态更新降级模式
func (s *DegradationService) updateMode() {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldMode := s.mode

	// 判断依赖状态
	redisHealthy := s.redisStatus.Healthy
	dbHealthy := s.dbStatus.Healthy

	switch {
	case redisHealthy && dbHealthy:
		// 所有核心依赖正常
		s.mode = ModeNormal

	case !redisHealthy && dbHealthy:
		// Redis 故障，可以降级到数据库
		s.mode = ModeDegraded

	case redisHealthy && !dbHealthy:
		// 数据库故障，只读模式
		s.mode = ModeReadOnly

	case !redisHealthy && !dbHealthy:
		// 所有核心依赖故障，安全模式
		s.mode = ModeSafeMode
	}

	if oldMode != s.mode {
		logger.Warn("degradation mode changed",
			"old_mode", oldMode.String(),
			"new_mode", s.mode.String(),
			"redis_healthy", redisHealthy,
			"db_healthy", dbHealthy)
	}
}

// GetMode 获取当前降级模式
func (s *DegradationService) GetMode() DegradationMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// IsRedisHealthy 检查 Redis 是否健康
func (s *DegradationService) IsRedisHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.redisStatus.Healthy
}

// IsDBHealthy 检查数据库是否健康
func (s *DegradationService) IsDBHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbStatus.Healthy
}

// IsKafkaHealthy 检查 Kafka 是否健康
func (s *DegradationService) IsKafkaHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.kafkaStatus.Healthy
}

// GetDependencyStatus 获取所有依赖状态
func (s *DegradationService) GetDependencyStatus() map[string]DependencyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]DependencyStatus{
		"redis":    s.redisStatus,
		"database": s.dbStatus,
		"kafka":    s.kafkaStatus,
	}
}

// IsFailSafe 是否启用失败安全模式
func (s *DegradationService) IsFailSafe() bool {
	return s.config.FailSafe
}

// ExecuteWithRedisBreaker 使用 Redis 熔断器执行操作
func (s *DegradationService) ExecuteWithRedisBreaker(fn func() error) error {
	return s.redisBreaker.Execute(fn)
}

// ExecuteWithDBBreaker 使用数据库熔断器执行操作
func (s *DegradationService) ExecuteWithDBBreaker(fn func() error) error {
	return s.dbBreaker.Execute(fn)
}

// ExecuteWithKafkaBreaker 使用 Kafka 熔断器执行操作
func (s *DegradationService) ExecuteWithKafkaBreaker(fn func() error) error {
	return s.kafkaBreaker.Execute(fn)
}

// GetRedisBreaker 获取 Redis 熔断器
func (s *DegradationService) GetRedisBreaker() *circuitbreaker.CircuitBreaker {
	return s.redisBreaker
}

// GetDBBreaker 获取数据库熔断器
func (s *DegradationService) GetDBBreaker() *circuitbreaker.CircuitBreaker {
	return s.dbBreaker
}

// GetKafkaBreaker 获取 Kafka 熔断器
func (s *DegradationService) GetKafkaBreaker() *circuitbreaker.CircuitBreaker {
	return s.kafkaBreaker
}

// ============================================================
// L2 缓存操作 (用于 Redis 故障时的降级)
// ============================================================

// UpdateBlacklistL2 更新黑名单 L2 缓存
func (s *DegradationService) UpdateBlacklistL2(wallet string, isBlacklisted bool) {
	s.blacklistL2Mu.Lock()
	defer s.blacklistL2Mu.Unlock()
	s.blacklistL2[wallet] = isBlacklisted
	s.blacklistL2TTL = time.Now().Add(5 * time.Minute) // 5分钟 TTL
}

// GetBlacklistL2 从 L2 缓存获取黑名单状态
func (s *DegradationService) GetBlacklistL2(wallet string) (isBlacklisted bool, found bool) {
	s.blacklistL2Mu.RLock()
	defer s.blacklistL2Mu.RUnlock()

	// 检查 TTL
	if time.Now().After(s.blacklistL2TTL) {
		return false, false
	}

	val, ok := s.blacklistL2[wallet]
	return val, ok
}

// ClearBlacklistL2 清空 L2 缓存
func (s *DegradationService) ClearBlacklistL2() {
	s.blacklistL2Mu.Lock()
	defer s.blacklistL2Mu.Unlock()
	s.blacklistL2 = make(map[string]bool)
}

// LoadBlacklistL2FromDB 从数据库加载黑名单到 L2 缓存
func (s *DegradationService) LoadBlacklistL2FromDB(ctx context.Context) error {
	if s.db == nil {
		return nil
	}

	// 查询所有活跃黑名单
	var wallets []string
	err := s.db.WithContext(ctx).
		Table("risk_blacklists").
		Where("status = ?", "active").
		Pluck("wallet", &wallets).Error
	if err != nil {
		return err
	}

	s.blacklistL2Mu.Lock()
	defer s.blacklistL2Mu.Unlock()

	// 重建 L2 缓存
	s.blacklistL2 = make(map[string]bool)
	for _, wallet := range wallets {
		s.blacklistL2[wallet] = true
	}
	s.blacklistL2TTL = time.Now().Add(5 * time.Minute)

	logger.Info("blacklist L2 cache loaded from database",
		"count", len(wallets))

	return nil
}

// ============================================================
// 风控检查降级策略
// ============================================================

// ShouldRejectOnError 依赖失败时是否应该拒绝请求
// 当 FailSafe=true 时，依赖失败应拒绝请求以确保安全
// 当 FailSafe=false 时，依赖失败应放行请求以保证可用性
func (s *DegradationService) ShouldRejectOnError() bool {
	return s.config.FailSafe
}

// CanPerformRiskCheck 是否可以执行风控检查
func (s *DegradationService) CanPerformRiskCheck() bool {
	mode := s.GetMode()
	// 安全模式下不进行复杂风控，直接拒绝
	return mode != ModeSafeMode
}

// CanWriteToDatabase 是否可以写入数据库
func (s *DegradationService) CanWriteToDatabase() bool {
	mode := s.GetMode()
	return mode == ModeNormal || mode == ModeDegraded
}

// GetRiskCheckFallbackResult 获取风控检查的降级结果
func (s *DegradationService) GetRiskCheckFallbackResult() (approved bool, reason string) {
	if s.config.FailSafe {
		// 安全模式: 拒绝请求
		return false, "风控服务降级中，请稍后重试"
	}
	// 可用性优先: 放行但记录
	return true, "风控服务降级，跳过部分检查"
}
