package scheduler

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
)

// Job 任务接口
type Job interface {
	// Name 任务名称
	Name() string
	// Execute 执行任务
	Execute(ctx context.Context) (*JobResult, error)
	// Timeout 任务超时时间
	Timeout() time.Duration
	// RequiresLock 是否需要分布式锁
	RequiresLock() bool
	// LockTTL 锁的TTL (仅在 RequiresLock() 返回 true 时有效)
	LockTTL() time.Duration
	// UseWatchdog 是否使用 Watchdog 锁续期 (长时间运行任务)
	UseWatchdog() bool
}

// JobResult 任务执行结果
type JobResult struct {
	// ProcessedCount 处理的记录数
	ProcessedCount int
	// AffectedCount 影响的记录数
	AffectedCount int
	// ErrorCount 错误数
	ErrorCount int
	// Details 详细信息
	Details map[string]interface{}
}

// ToJSONResult 转换为 JSONResult
func (r *JobResult) ToJSONResult() model.JSONResult {
	if r == nil {
		return nil
	}
	result := model.JSONResult{
		"processed_count": r.ProcessedCount,
		"affected_count":  r.AffectedCount,
		"error_count":     r.ErrorCount,
	}
	for k, v := range r.Details {
		result[k] = v
	}
	return result
}

// BaseJob 基础任务实现
type BaseJob struct {
	name        string
	timeout     time.Duration
	lockTTL     time.Duration
	useWatchdog bool
}

// NewBaseJob 创建基础任务
func NewBaseJob(name string, timeout, lockTTL time.Duration, useWatchdog bool) BaseJob {
	return BaseJob{
		name:        name,
		timeout:     timeout,
		lockTTL:     lockTTL,
		useWatchdog: useWatchdog,
	}
}

// Name 任务名称
func (j BaseJob) Name() string {
	return j.name
}

// Timeout 任务超时时间
func (j BaseJob) Timeout() time.Duration {
	return j.timeout
}

// RequiresLock 是否需要分布式锁
func (j BaseJob) RequiresLock() bool {
	return j.lockTTL > 0
}

// LockTTL 锁的TTL
func (j BaseJob) LockTTL() time.Duration {
	return j.lockTTL
}

// UseWatchdog 是否使用 Watchdog 锁续期
func (j BaseJob) UseWatchdog() bool {
	return j.useWatchdog
}

// JobNames 任务名称常量
const (
	JobNameReconciliation     = "reconciliation"
	JobNameCleanupOrders      = "cleanup-orders"
	JobNameArchiveData        = "archive-data"
	JobNameStatsAgg           = "stats-agg"
	JobNameKlineAgg           = "kline-agg"
	JobNameHealthMonitor      = "health-monitor"
	JobNamePartitionManage    = "partition-manage"
	JobNameDataCleanup        = "data-cleanup"
	JobNameBalanceScan        = "balance-scan"
	JobNameSettlementTrigger  = "settlement-trigger"
)

// DefaultJobConfigs 默认任务配置
var DefaultJobConfigs = map[string]struct {
	Cron        string
	Timeout     time.Duration
	LockTTL     time.Duration
	UseWatchdog bool
}{
	JobNameReconciliation: {
		Cron:        "0 0 * * * *",      // 每小时
		Timeout:     10 * time.Minute,
		LockTTL:     15 * time.Minute,
		UseWatchdog: true,
	},
	JobNameCleanupOrders: {
		Cron:        "*/2 * * * *",      // 每2分钟
		Timeout:     1 * time.Minute,
		LockTTL:     1 * time.Minute,
		UseWatchdog: false,
	},
	JobNameArchiveData: {
		Cron:        "0 0 3 * * *",      // 每日凌晨3点
		Timeout:     30 * time.Minute,
		LockTTL:     35 * time.Minute,
		UseWatchdog: true,
	},
	JobNameStatsAgg: {
		Cron:        "0 5 * * * *",      // 每小时第5分钟
		Timeout:     5 * time.Minute,
		LockTTL:     5 * time.Minute,
		UseWatchdog: false,
	},
	JobNameKlineAgg: {
		Cron:        "0 1 * * * *",      // 每小时第1分钟
		Timeout:     5 * time.Minute,
		LockTTL:     5 * time.Minute,
		UseWatchdog: false,
	},
	JobNameHealthMonitor: {
		Cron:        "*/30 * * * * *",   // 每30秒
		Timeout:     10 * time.Second,
		LockTTL:     0,                  // 无需锁
		UseWatchdog: false,
	},
	JobNamePartitionManage: {
		Cron:        "0 0 1 * *",        // 每月1日
		Timeout:     10 * time.Minute,
		LockTTL:     10 * time.Minute,
		UseWatchdog: false,
	},
	JobNameDataCleanup: {
		Cron:        "0 0 4 * * *",      // 每日凌晨4点
		Timeout:     30 * time.Minute,
		LockTTL:     35 * time.Minute,
		UseWatchdog: true,
	},
	JobNameBalanceScan: {
		Cron:        "0 */5 * * * *",    // 每5分钟
		Timeout:     4 * time.Minute,
		LockTTL:     5 * time.Minute,
		UseWatchdog: false,
	},
	JobNameSettlementTrigger: {
		Cron:        "0 */1 * * * *",    // 每1分钟
		Timeout:     50 * time.Second,
		LockTTL:     1 * time.Minute,
		UseWatchdog: false,
	},
}
