package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy      bool      `json:"healthy"`
	Message      string    `json:"message,omitempty"`
	Latency      int64     `json:"latency_ms"`
	LastChecked  time.Time `json:"last_checked"`
	Connections  int       `json:"connections"`
	IdleConns    int       `json:"idle_connections"`
	MaxOpenConns int       `json:"max_open_connections"`
	InUse        int       `json:"in_use"`
	WaitCount    int64     `json:"wait_count"`
	WaitDuration int64     `json:"wait_duration_ms"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	pool    *Pool
	timeout time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(pool *Pool, timeout time.Duration) *HealthChecker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HealthChecker{
		pool:    pool,
		timeout: timeout,
	}
}

// Check 执行健康检查
func (h *HealthChecker) Check(ctx context.Context) *HealthStatus {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	status := &HealthStatus{
		LastChecked: time.Now(),
	}

	// 获取连接池统计
	stats := h.pool.Stats()
	status.Connections = stats.OpenConnections
	status.IdleConns = stats.Idle
	status.MaxOpenConns = stats.MaxOpenConnections
	status.InUse = stats.InUse
	status.WaitCount = stats.WaitCount
	status.WaitDuration = stats.WaitDuration.Milliseconds()

	// 执行 ping 检查
	err := h.pool.Ping(ctx)
	status.Latency = time.Since(start).Milliseconds()

	if err != nil {
		status.Healthy = false
		status.Message = fmt.Sprintf("ping failed: %v", err)
		return status
	}

	// 执行简单查询验证
	var result int
	err = h.pool.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		status.Healthy = false
		status.Message = fmt.Sprintf("query failed: %v", err)
		return status
	}

	status.Healthy = true
	status.Message = "OK"
	return status
}

// CheckWithDetails 执行详细健康检查
func (h *HealthChecker) CheckWithDetails(ctx context.Context) (*DetailedHealthStatus, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	status := &DetailedHealthStatus{
		Basic:       h.Check(ctx),
		CheckedAt:   time.Now(),
		CheckDuration: time.Since(start),
	}

	if !status.Basic.Healthy {
		return status, nil
	}

	// 检查复制状态
	replicationStatus, err := h.checkReplication(ctx)
	if err == nil {
		status.Replication = replicationStatus
	}

	// 检查数据库大小
	dbSize, err := h.checkDatabaseSize(ctx)
	if err == nil {
		status.DatabaseSize = dbSize
	}

	// 检查活动连接
	activeConns, err := h.checkActiveConnections(ctx)
	if err == nil {
		status.ActiveConnections = activeConns
	}

	// 检查锁等待
	locks, err := h.checkLocks(ctx)
	if err == nil {
		status.Locks = locks
	}

	status.CheckDuration = time.Since(start)
	return status, nil
}

// DetailedHealthStatus 详细健康状态
type DetailedHealthStatus struct {
	Basic             *HealthStatus          `json:"basic"`
	Replication       *ReplicationStatus     `json:"replication,omitempty"`
	DatabaseSize      int64                  `json:"database_size_bytes,omitempty"`
	ActiveConnections int                    `json:"active_connections,omitempty"`
	Locks             *LockInfo              `json:"locks,omitempty"`
	CheckedAt         time.Time              `json:"checked_at"`
	CheckDuration     time.Duration          `json:"check_duration"`
}

// ReplicationStatus 复制状态
type ReplicationStatus struct {
	IsPrimary     bool   `json:"is_primary"`
	ReplicaCount  int    `json:"replica_count,omitempty"`
	ReplayLag     int64  `json:"replay_lag_bytes,omitempty"`
	State         string `json:"state,omitempty"`
}

// LockInfo 锁信息
type LockInfo struct {
	TotalLocks    int `json:"total_locks"`
	WaitingLocks  int `json:"waiting_locks"`
	BlockingLocks int `json:"blocking_locks"`
}

// checkReplication 检查复制状态
func (h *HealthChecker) checkReplication(ctx context.Context) (*ReplicationStatus, error) {
	status := &ReplicationStatus{}

	// 检查是否是主库
	var isInRecovery bool
	err := h.pool.QueryRowContext(ctx, "SELECT pg_is_in_recovery()").Scan(&isInRecovery)
	if err != nil {
		return nil, err
	}
	status.IsPrimary = !isInRecovery

	if status.IsPrimary {
		// 主库：检查副本数量
		var count int
		err = h.pool.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pg_stat_replication",
		).Scan(&count)
		if err == nil {
			status.ReplicaCount = count
		}
	} else {
		// 从库：检查复制延迟
		var lag sql.NullInt64
		err = h.pool.QueryRowContext(ctx,
			"SELECT pg_wal_lsn_diff(pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn())",
		).Scan(&lag)
		if err == nil && lag.Valid {
			status.ReplayLag = lag.Int64
		}
	}

	return status, nil
}

// checkDatabaseSize 检查数据库大小
func (h *HealthChecker) checkDatabaseSize(ctx context.Context) (int64, error) {
	var size int64
	err := h.pool.QueryRowContext(ctx,
		"SELECT pg_database_size(current_database())",
	).Scan(&size)
	return size, err
}

// checkActiveConnections 检查活动连接数
func (h *HealthChecker) checkActiveConnections(ctx context.Context) (int, error) {
	var count int
	err := h.pool.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active'",
	).Scan(&count)
	return count, err
}

// checkLocks 检查锁信息
func (h *HealthChecker) checkLocks(ctx context.Context) (*LockInfo, error) {
	info := &LockInfo{}

	// 总锁数
	err := h.pool.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_locks",
	).Scan(&info.TotalLocks)
	if err != nil {
		return nil, err
	}

	// 等待中的锁
	err = h.pool.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_locks WHERE NOT granted",
	).Scan(&info.WaitingLocks)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// ReadinessCheck 就绪检查 (用于 K8s readiness probe)
func (h *HealthChecker) ReadinessCheck(ctx context.Context) error {
	status := h.Check(ctx)
	if !status.Healthy {
		return fmt.Errorf("database not ready: %s", status.Message)
	}
	return nil
}

// LivenessCheck 存活检查 (用于 K8s liveness probe)
func (h *HealthChecker) LivenessCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	return h.pool.Ping(ctx)
}
