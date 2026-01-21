package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Pool PostgreSQL 连接池
type Pool struct {
	db     *sql.DB
	config *Config

	// 统计信息
	queryCount    uint64
	errorCount    uint64
	slowQueryCount uint64

	// 健康检查
	healthy     atomic.Bool
	healthCheck chan struct{}
	closeOnce   sync.Once
}

// NewPool 创建数据库连接池
func NewPool(cfg *Config) (*Pool, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger.Info("connecting to postgresql",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Database,
		"max_open_conns", cfg.MaxOpenConns,
	)

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	pool := &Pool{
		db:          db,
		config:      cfg,
		healthCheck: make(chan struct{}),
	}
	pool.healthy.Store(true)

	// 启动健康检查
	if cfg.HealthCheckInterval > 0 {
		go pool.runHealthCheck()
	}

	logger.Info("postgresql connection pool created",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Database,
	)

	return pool, nil
}

// DB 返回底层数据库连接
func (p *Pool) DB() *sql.DB {
	return p.db
}

// Config 返回配置
func (p *Pool) Config() *Config {
	return p.config
}

// IsHealthy 返回连接池健康状态
func (p *Pool) IsHealthy() bool {
	return p.healthy.Load()
}

// Ping 测试数据库连接
func (p *Pool) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Stats 返回连接池统计信息
func (p *Pool) Stats() sql.DBStats {
	return p.db.Stats()
}

// QueryStats 返回查询统计信息
type QueryStats struct {
	QueryCount     uint64
	ErrorCount     uint64
	SlowQueryCount uint64
	DBStats        sql.DBStats
}

// QueryStats 返回查询统计信息
func (p *Pool) QueryStats() QueryStats {
	return QueryStats{
		QueryCount:     atomic.LoadUint64(&p.queryCount),
		ErrorCount:     atomic.LoadUint64(&p.errorCount),
		SlowQueryCount: atomic.LoadUint64(&p.slowQueryCount),
		DBStats:        p.db.Stats(),
	}
}

// Close 关闭连接池
func (p *Pool) Close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.healthCheck)
		err = p.db.Close()
		logger.Info("postgresql connection pool closed")
	})
	return err
}

// runHealthCheck 运行健康检查
func (p *Pool) runHealthCheck() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.healthCheck:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), p.config.HealthCheckTimeout)
			err := p.db.PingContext(ctx)
			cancel()

			wasHealthy := p.healthy.Load()
			isHealthy := err == nil

			if wasHealthy != isHealthy {
				if isHealthy {
					logger.Info("postgresql connection recovered")
				} else {
					logger.Error("postgresql connection lost", "error", err)
				}
			}

			p.healthy.Store(isHealthy)

			// 更新健康检查指标
			if isHealthy {
				metrics.RequestsTotal.WithLabelValues("postgres", "health_check", "ok").Inc()
			} else {
				metrics.RequestsTotal.WithLabelValues("postgres", "health_check", "error").Inc()
			}
		}
	}
}

// ExecContext 执行 SQL 语句
func (p *Pool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	atomic.AddUint64(&p.queryCount, 1)

	result, err := p.db.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	p.logQuery(ctx, "exec", query, duration, err)

	return result, err
}

// QueryContext 执行查询
func (p *Pool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	atomic.AddUint64(&p.queryCount, 1)

	rows, err := p.db.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	p.logQuery(ctx, "query", query, duration, err)

	return rows, err
}

// QueryRowContext 执行单行查询
func (p *Pool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	atomic.AddUint64(&p.queryCount, 1)

	row := p.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	p.logQuery(ctx, "query_row", query, duration, nil)

	return row
}

// logQuery 记录查询日志
func (p *Pool) logQuery(ctx context.Context, operation, query string, duration time.Duration, err error) {
	// 更新指标
	status := "ok"
	if err != nil {
		status = "error"
		atomic.AddUint64(&p.errorCount, 1)
	}

	metrics.RecordRequest("postgres", operation, status, duration.Seconds())

	// 检查慢查询
	if duration >= p.config.SlowQueryThreshold {
		atomic.AddUint64(&p.slowQueryCount, 1)
		logger.Warn("slow query detected",
			"operation", operation,
			"duration", duration,
			"query", truncateQuery(query, 200),
		)
	}

	// 记录错误
	if err != nil && p.config.LogLevel != "silent" {
		logger.Error("database query failed",
			"operation", operation,
			"duration", duration,
			"query", truncateQuery(query, 200),
			"error", err,
		)
	}
}

// truncateQuery 截断查询语句用于日志
func truncateQuery(query string, maxLen int) string {
	if len(query) <= maxLen {
		return query
	}
	return query[:maxLen] + "..."
}

// NewPoolWithRetry 创建数据库连接池，支持重试
func NewPoolWithRetry(cfg *Config) (*Pool, error) {
	var pool *Pool
	var err error

	for i := 0; i <= cfg.MaxRetries; i++ {
		pool, err = NewPool(cfg)
		if err == nil {
			return pool, nil
		}

		if i < cfg.MaxRetries {
			logger.Warn("failed to connect to postgresql, retrying",
				"attempt", i+1,
				"max_retries", cfg.MaxRetries,
				"retry_interval", cfg.RetryInterval,
				"error", err,
			)
			time.Sleep(cfg.RetryInterval)
		}
	}

	return nil, fmt.Errorf("failed to connect to postgresql after %d retries: %w", cfg.MaxRetries, err)
}
