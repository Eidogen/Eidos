// Package infra 提供统一的基础设施初始化
// 用于消除各服务中重复的 Redis、数据库、HTTP 健康检查等初始化代码
package infra

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/config"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	commonRedis "github.com/eidos-exchange/eidos/eidos-common/pkg/redis"
)

// RedisOptions Redis 初始化选项
type RedisOptions struct {
	Config *config.RedisConfig
	// 如果需要使用企业级 Client，设置为 true
	UseEnterpriseClient bool
}

// redisPoolConfig 获取 Redis 连接池配置（避免代码重复）
func redisPoolConfig(cfg *config.RedisConfig) (poolSize, minIdleConns, maxIdleConns int, connMaxLifetime, connMaxIdleTime, dialTimeout, readTimeout, writeTimeout time.Duration) {
	poolSize = cfg.PoolSize
	if poolSize <= 0 {
		poolSize = 100
	}
	minIdleConns = cfg.MinIdleConns
	if minIdleConns <= 0 {
		minIdleConns = 10
	}
	maxIdleConns = cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 50
	}
	connMaxLifetime = time.Duration(cfg.ConnMaxLifetime) * time.Second
	if connMaxLifetime <= 0 {
		connMaxLifetime = 30 * time.Minute
	}
	connMaxIdleTime = time.Duration(cfg.ConnMaxIdleTime) * time.Second
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = 5 * time.Minute
	}
	dialTimeout = time.Duration(cfg.DialTimeout) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	readTimeout = time.Duration(cfg.ReadTimeout) * time.Second
	if readTimeout <= 0 {
		readTimeout = 3 * time.Second
	}
	writeTimeout = time.Duration(cfg.WriteTimeout) * time.Second
	if writeTimeout <= 0 {
		writeTimeout = 3 * time.Second
	}
	return
}

// NewRedisClient 创建标准 Redis 客户端
// 使用 common 包中的标准配置，消除各服务中的重复初始化代码
func NewRedisClient(cfg *config.RedisConfig) *redis.Client {
	if cfg == nil {
		cfg = &config.RedisConfig{
			Addresses: []string{"localhost:6379"},
			PoolSize:  100,
		}
	}

	poolSize, minIdleConns, maxIdleConns, connMaxLifetime, connMaxIdleTime, dialTimeout, readTimeout, writeTimeout := redisPoolConfig(cfg)

	addr := "localhost:6379"
	if len(cfg.Addresses) > 0 {
		addr = cfg.Addresses[0]
	}

	client := redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
		ConnMaxIdleTime: connMaxIdleTime,
		DialTimeout:     dialTimeout,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})

	logger.Info("redis client initialized",
		"addr", addr,
		"pool_size", poolSize,
	)

	return client
}

// NewRedisUniversalClient 创建 Redis UniversalClient
// 自动识别单机/集群模式，适用于需要支持多种 Redis 部署模式的服务
func NewRedisUniversalClient(cfg *config.RedisConfig) redis.UniversalClient {
	if cfg == nil {
		cfg = &config.RedisConfig{
			Addresses: []string{"localhost:6379"},
			PoolSize:  100,
		}
	}

	poolSize, minIdleConns, maxIdleConns, connMaxLifetime, connMaxIdleTime, dialTimeout, readTimeout, writeTimeout := redisPoolConfig(cfg)

	opts := &redis.UniversalOptions{
		Addrs:           cfg.Addresses,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        poolSize,
		MinIdleConns:    minIdleConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
		ConnMaxIdleTime: connMaxIdleTime,
		DialTimeout:     dialTimeout,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	}

	if len(opts.Addrs) == 0 {
		opts.Addrs = []string{"localhost:6379"}
	}

	client := redis.NewUniversalClient(opts)

	logger.Info("redis universal client initialized",
		"addrs", opts.Addrs,
		"pool_size", poolSize,
	)

	return client
}

// NewEnterpriseRedisClient 创建企业级 Redis 客户端
// 支持单机、哨兵、集群模式，带健康检查和指标
func NewEnterpriseRedisClient(cfg *config.RedisConfig) (*commonRedis.Client, error) {
	if cfg == nil {
		return commonRedis.NewClient(commonRedis.DefaultConfig())
	}

	redisConfig := &commonRedis.Config{
		Mode:                commonRedis.ModeSingle,
		Addresses:           cfg.Addresses,
		Password:            cfg.Password,
		DB:                  cfg.DB,
		PoolSize:            cfg.PoolSize,
		MinIdleConns:        cfg.MinIdleConns,
		MaxIdleConns:        cfg.MaxIdleConns,
		ConnMaxLifetime:     time.Duration(cfg.ConnMaxLifetime) * time.Second,
		ConnMaxIdleTime:     time.Duration(cfg.ConnMaxIdleTime) * time.Second,
		DialTimeout:         time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:         time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout:        time.Duration(cfg.WriteTimeout) * time.Second,
		MaxRetries:          3,
		HealthCheckInterval: 30 * time.Second,
	}

	// 设置默认值
	if len(redisConfig.Addresses) == 0 {
		redisConfig.Addresses = []string{"localhost:6379"}
	}
	if redisConfig.PoolSize <= 0 {
		redisConfig.PoolSize = 100
	}

	return commonRedis.NewClient(redisConfig)
}

// DatabaseOptions 数据库初始化选项
type DatabaseOptions struct {
	Config   *config.PostgresConfig
	LogLevel gormlogger.LogLevel
}

// NewDatabase 创建 GORM 数据库连接
// 使用 common 包中的标准配置，消除各服务中的重复初始化代码
func NewDatabase(cfg *config.PostgresConfig) (*gorm.DB, error) {
	return NewDatabaseWithOptions(&DatabaseOptions{
		Config:   cfg,
		LogLevel: gormlogger.Warn,
	})
}

// NewDatabaseWithOptions 创建 GORM 数据库连接（带选项）
func NewDatabaseWithOptions(opts *DatabaseOptions) (*gorm.DB, error) {
	if opts == nil || opts.Config == nil {
		return nil, fmt.Errorf("database config is required")
	}

	cfg := opts.Config

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.Database,
	)

	gormConfig := &gorm.Config{
		Logger: gormlogger.Default.LogMode(opts.LogLevel),
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	// 连接池配置
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 10
	}
	maxOpenConns := cfg.MaxConnections
	if maxOpenConns <= 0 {
		maxOpenConns = 100
	}
	connMaxLifetime := time.Duration(cfg.ConnMaxLifetime) * time.Second
	if connMaxLifetime <= 0 {
		connMaxLifetime = 30 * time.Minute
	}
	connMaxIdleTime := time.Duration(cfg.ConnMaxIdleTime) * time.Second
	if connMaxIdleTime <= 0 {
		connMaxIdleTime = 5 * time.Minute
	}

	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	logger.Info("database connected",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Database,
		"max_open_conns", maxOpenConns,
	)

	return db, nil
}

// HealthCheckHandler HTTP 健康检查处理器
type HealthCheckHandler struct {
	db            *gorm.DB
	rdb           *redis.Client
	rdbUniversal  redis.UniversalClient
	customChecker func(ctx context.Context) error
}

// NewHealthCheckHandler 创建健康检查处理器
func NewHealthCheckHandler(db *gorm.DB, rdb *redis.Client) *HealthCheckHandler {
	return &HealthCheckHandler{db: db, rdb: rdb}
}

// NewHealthCheckHandlerUniversal 创建支持 UniversalClient 的健康检查处理器
func NewHealthCheckHandlerUniversal(db *gorm.DB, rdb redis.UniversalClient) *HealthCheckHandler {
	return &HealthCheckHandler{db: db, rdbUniversal: rdb}
}

// WithCustomChecker 添加自定义检查函数
func (h *HealthCheckHandler) WithCustomChecker(checker func(ctx context.Context) error) *HealthCheckHandler {
	h.customChecker = checker
	return h
}

// LivenessHandler 存活检查
func (h *HealthCheckHandler) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// ReadinessHandler 就绪检查
func (h *HealthCheckHandler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// 检查数据库连接
	if h.db != nil {
		sqlDB, err := h.db.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("DB ERROR"))
			return
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("DB NOT READY"))
			return
		}
	}

	// 检查 Redis 连接 (支持两种客户端类型)
	if h.rdb != nil {
		if err := h.rdb.Ping(ctx).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("REDIS NOT READY"))
			return
		}
	}
	if h.rdbUniversal != nil {
		if err := h.rdbUniversal.Ping(ctx).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("REDIS NOT READY"))
			return
		}
	}

	// 自定义检查
	if h.customChecker != nil {
		if err := h.customChecker(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(err.Error()))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// HTTPServerConfig HTTP 服务器配置
type HTTPServerConfig struct {
	Port             int
	DB               *gorm.DB
	Redis            *redis.Client
	RedisUniversal   redis.UniversalClient
	EnableMetrics    bool
	EnableHealth     bool
	CustomChecker    func(ctx context.Context) error
	AdditionalRoutes func(mux *http.ServeMux)
}

// NewHTTPServer 创建标准 HTTP 服务器
// 包含 /metrics 和 /health/* 端点，消除各服务中的重复代码
func NewHTTPServer(cfg *HTTPServerConfig) *http.Server {
	mux := http.NewServeMux()

	// Prometheus metrics 端点
	if cfg.EnableMetrics {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// 健康检查端点
	if cfg.EnableHealth {
		var healthHandler *HealthCheckHandler
		if cfg.RedisUniversal != nil {
			healthHandler = NewHealthCheckHandlerUniversal(cfg.DB, cfg.RedisUniversal)
		} else {
			healthHandler = NewHealthCheckHandler(cfg.DB, cfg.Redis)
		}
		if cfg.CustomChecker != nil {
			healthHandler.WithCustomChecker(cfg.CustomChecker)
		}
		mux.HandleFunc("/health/live", healthHandler.LivenessHandler)
		mux.HandleFunc("/health/ready", healthHandler.ReadinessHandler)
		// 兼容 k8s 标准路径
		mux.HandleFunc("/healthz", healthHandler.LivenessHandler)
		mux.HandleFunc("/readyz", healthHandler.ReadinessHandler)
	}

	// 额外路由
	if cfg.AdditionalRoutes != nil {
		cfg.AdditionalRoutes(mux)
	}

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}
}

// StartHTTPServer 启动 HTTP 服务器（非阻塞）
func StartHTTPServer(server *http.Server) {
	go func() {
		logger.Info("HTTP server listening",
			"addr", server.Addr,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
		}
	}()
}

// ShutdownHTTPServer 优雅关闭 HTTP 服务器
func ShutdownHTTPServer(server *http.Server, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return server.Shutdown(ctx)
}

// PoolMetricsCollector 连接池指标收集器
type PoolMetricsCollector struct {
	db           *gorm.DB
	dbName       string
	rdb          *redis.Client
	rdbUniversal redis.UniversalClient
	redisAddrs   []string
	stopCh       chan struct{}
	interval     time.Duration
}

// PoolMetricsConfig 连接池指标配置
type PoolMetricsConfig struct {
	DB             *gorm.DB
	DBName         string
	Redis          *redis.Client
	RedisUniversal redis.UniversalClient
	RedisAddrs     []string
	Interval       time.Duration // 默认 15 秒
}

// NewPoolMetricsCollector 创建连接池指标收集器
func NewPoolMetricsCollector(cfg *PoolMetricsConfig) *PoolMetricsCollector {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 15 * time.Second
	}

	addrs := cfg.RedisAddrs
	if len(addrs) == 0 {
		addrs = []string{"localhost:6379"}
	}

	return &PoolMetricsCollector{
		db:           cfg.DB,
		dbName:       cfg.DBName,
		rdb:          cfg.Redis,
		rdbUniversal: cfg.RedisUniversal,
		redisAddrs:   addrs,
		stopCh:       make(chan struct{}),
		interval:     interval,
	}
}

// Start 启动指标收集
func (c *PoolMetricsCollector) Start() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// 立即收集一次
		c.collect()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
	logger.Info("pool metrics collector started", "interval", c.interval)
}

// Stop 停止指标收集
func (c *PoolMetricsCollector) Stop() {
	close(c.stopCh)
}

// collect 收集指标
func (c *PoolMetricsCollector) collect() {
	// 收集数据库连接池指标
	if c.db != nil {
		c.collectDBMetrics()
	}

	// 收集 Redis 连接池指标
	if c.rdb != nil {
		c.collectRedisMetrics(c.rdb)
	}
	if c.rdbUniversal != nil {
		c.collectRedisUniversalMetrics(c.rdbUniversal)
	}
}

// collectDBMetrics 收集数据库连接池指标
func (c *PoolMetricsCollector) collectDBMetrics() {
	sqlDB, err := c.db.DB()
	if err != nil {
		return
	}

	stats := sqlDB.Stats()
	dbName := c.dbName
	if dbName == "" {
		dbName = "default"
	}

	// 使用 metrics 包中定义的指标
	setDBMetric("db_connections_open", dbName, float64(stats.OpenConnections))
	setDBMetric("db_connections_idle", dbName, float64(stats.Idle))
	setDBMetric("db_connections_in_use", dbName, float64(stats.InUse))
}

// collectRedisMetrics 收集 Redis 连接池指标 (标准客户端)
func (c *PoolMetricsCollector) collectRedisMetrics(client *redis.Client) {
	stats := client.PoolStats()
	addr := client.Options().Addr

	setRedisPoolMetrics(addr, stats)
}

// collectRedisUniversalMetrics 收集 Redis 连接池指标 (UniversalClient)
func (c *PoolMetricsCollector) collectRedisUniversalMetrics(client redis.UniversalClient) {
	// UniversalClient 可能是 *redis.Client 或 *redis.ClusterClient
	switch v := client.(type) {
	case *redis.Client:
		stats := v.PoolStats()
		addr := v.Options().Addr
		setRedisPoolMetrics(addr, stats)
	case *redis.ClusterClient:
		// 集群模式下收集每个节点的指标
		err := v.ForEachShard(context.Background(), func(ctx context.Context, shard *redis.Client) error {
			stats := shard.PoolStats()
			addr := shard.Options().Addr
			setRedisPoolMetrics(addr, stats)
			return nil
		})
		if err != nil {
			logger.Warn("failed to collect cluster redis metrics", "error", err)
		}
	}
}

// setDBMetric 设置数据库指标 (通过 prometheus 直接设置)
func setDBMetric(name, dbName string, value float64) {
	// 这里我们直接使用 metrics 包中的指标
	switch name {
	case "db_connections_open":
		dbConnectionsOpen.WithLabelValues(dbName).Set(value)
	case "db_connections_idle":
		dbConnectionsIdle.WithLabelValues(dbName).Set(value)
	case "db_connections_in_use":
		dbConnectionsInUse.WithLabelValues(dbName).Set(value)
	}
}

// setRedisPoolMetrics 设置 Redis 连接池指标
func setRedisPoolMetrics(addr string, stats *redis.PoolStats) {
	redisPoolSize.WithLabelValues(addr).Set(float64(stats.TotalConns))
	redisPoolIdleConns.WithLabelValues(addr).Set(float64(stats.IdleConns))
	redisPoolStaleConns.WithLabelValues(addr).Set(float64(stats.StaleConns))
	// 累计值作为 Gauge 记录当前快照
	redisPoolHits.WithLabelValues(addr).Set(float64(stats.Hits))
	redisPoolMisses.WithLabelValues(addr).Set(float64(stats.Misses))
	redisPoolTimeouts.WithLabelValues(addr).Set(float64(stats.Timeouts))
}

// ConfigReloader 配置热更新回调接口
type ConfigReloader interface {
	// OnConfigChange 当配置变更时调用
	// newConfig 是新的配置内容（已解析的结构体指针）
	OnConfigChange(newConfig interface{}) error
}

// PoolConfigReloader 连接池配置热更新器
// 用于在配置变更时动态调整数据库连接池参数
type PoolConfigReloader struct {
	db     *gorm.DB
	logger func(msg string, args ...interface{})
}

// NewPoolConfigReloader 创建连接池配置热更新器
func NewPoolConfigReloader(db *gorm.DB) *PoolConfigReloader {
	return &PoolConfigReloader{
		db: db,
		logger: func(msg string, args ...interface{}) {
			logger.Info(msg, args...)
		},
	}
}

// UpdateDBPool 动态更新数据库连接池配置
// 注意：某些参数（如 maxOpenConns）可以动态调整，但需要等待现有连接释放
func (r *PoolConfigReloader) UpdateDBPool(maxIdleConns, maxOpenConns int, connMaxLifetime, connMaxIdleTime time.Duration) error {
	if r.db == nil {
		return nil
	}

	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}

	// 动态调整连接池参数
	if maxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(maxIdleConns)
	}
	if maxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(maxOpenConns)
	}
	if connMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(connMaxLifetime)
	}
	if connMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
	}

	r.logger("database pool config updated",
		"max_idle_conns", maxIdleConns,
		"max_open_conns", maxOpenConns,
		"conn_max_lifetime", connMaxLifetime,
		"conn_max_idle_time", connMaxIdleTime,
	)

	return nil
}

// 连接池监控指标（使用 promauto 自动注册，避免重复注册 panic）
var (
	dbConnectionsOpen = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "db_connections_open",
			Help:      "打开的数据库连接数",
		},
		[]string{"database"},
	)

	dbConnectionsIdle = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "db_connections_idle",
			Help:      "空闲的数据库连接数",
		},
		[]string{"database"},
	)

	dbConnectionsInUse = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "db_connections_in_use",
			Help:      "使用中的数据库连接数",
		},
		[]string{"database"},
	)

	redisPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_total_conns",
			Help:      "Redis 连接池总连接数",
		},
		[]string{"addr"},
	)

	redisPoolIdleConns = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_idle_conns",
			Help:      "Redis 空闲连接数",
		},
		[]string{"addr"},
	)

	redisPoolStaleConns = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_stale_conns",
			Help:      "Redis 过期连接数(累计)",
		},
		[]string{"addr"},
	)

	redisPoolHits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_hits",
			Help:      "Redis 连接池命中数(累计)",
		},
		[]string{"addr"},
	)

	redisPoolMisses = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_misses",
			Help:      "Redis 连接池未命中数(累计)",
		},
		[]string{"addr"},
	)

	redisPoolTimeouts = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "eidos",
			Subsystem: "infra",
			Name:      "redis_pool_timeouts",
			Help:      "Redis 连接池超时数(累计)",
		},
		[]string{"addr"},
	)
)
