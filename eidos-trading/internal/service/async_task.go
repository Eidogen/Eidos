// Package service 提供业务服务
package service

import (
	"context"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/metrics"
)

// AsyncTaskManager 异步任务管理器
// 统一管理所有异步 DB 写入任务，提供：
// - 超时控制
// - 错误日志
// - 优雅关闭
// - 任务数量监控
type AsyncTaskManager struct {
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	timeout      time.Duration
	pendingTasks int64 // 原子计数
	mu           sync.Mutex
}

// NewAsyncTaskManager 创建异步任务管理器
func NewAsyncTaskManager(timeout time.Duration) *AsyncTaskManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncTaskManager{
		ctx:     ctx,
		cancel:  cancel,
		timeout: timeout,
	}
}

// Submit 提交异步任务
// taskName: 任务名称 (用于日志)
// taskID: 任务 ID (如 orderID)
// fn: 任务函数，接收带超时的 context
func (m *AsyncTaskManager) Submit(taskName, taskID string, fn func(ctx context.Context) error) {
	m.wg.Add(1)
	m.mu.Lock()
	m.pendingTasks++
	metrics.SetAsyncTasksPending(taskName, float64(m.pendingTasks))
	m.mu.Unlock()

	go func() {
		defer func() {
			m.wg.Done()
			m.mu.Lock()
			m.pendingTasks--
			metrics.SetAsyncTasksPending(taskName, float64(m.pendingTasks))
			m.mu.Unlock()
		}()

		// 检查管理器是否已关闭
		select {
		case <-m.ctx.Done():
			logger.Warn("async task skipped due to shutdown",
				"task", taskName,
				"id", taskID,
			)
			return
		default:
		}

		// 创建带超时的 context
		taskCtx, cancel := context.WithTimeout(m.ctx, m.timeout)
		defer cancel()

		startTime := time.Now()
		err := fn(taskCtx)
		duration := time.Since(startTime)

		if err != nil {
			// 记录异步任务错误指标
			metrics.RecordAsyncTaskError(taskName)

			if taskCtx.Err() == context.DeadlineExceeded {
				logger.Error("async task timeout",
					"task", taskName,
					"id", taskID,
					"timeout", m.timeout,
					"error", err,
				)
			} else {
				logger.Error("async task failed",
					"task", taskName,
					"id", taskID,
					"duration", duration,
					"error", err,
				)
			}
		} else if duration > m.timeout/2 {
			// 任务耗时超过一半超时时间，记录警告
			logger.Warn("async task slow",
				"task", taskName,
				"id", taskID,
				"duration", duration,
			)
		}
	}()
}

// Shutdown 优雅关闭
// 等待所有进行中的任务完成，或超时后强制返回
func (m *AsyncTaskManager) Shutdown(timeout time.Duration) {
	// 停止接受新任务
	m.cancel()

	// 等待现有任务完成
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("async task manager shutdown complete")
	case <-time.After(timeout):
		m.mu.Lock()
		pending := m.pendingTasks
		m.mu.Unlock()
		logger.Warn("async task manager shutdown timeout",
			"pending_tasks", pending,
		)
	}
}

// PendingCount 返回待处理任务数
func (m *AsyncTaskManager) PendingCount() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pendingTasks
}

// DefaultAsyncTimeout 默认异步任务超时
const DefaultAsyncTimeout = 30 * time.Second

// asyncTaskManager 全局异步任务管理器
var asyncTaskManager *AsyncTaskManager
var asyncTaskManagerOnce sync.Once

// GetAsyncTaskManager 获取全局异步任务管理器
func GetAsyncTaskManager() *AsyncTaskManager {
	asyncTaskManagerOnce.Do(func() {
		asyncTaskManager = NewAsyncTaskManager(DefaultAsyncTimeout)
	})
	return asyncTaskManager
}

// InitAsyncTaskManager 初始化全局异步任务管理器 (带自定义超时)
func InitAsyncTaskManager(timeout time.Duration) *AsyncTaskManager {
	asyncTaskManagerOnce.Do(func() {
		asyncTaskManager = NewAsyncTaskManager(timeout)
	})
	return asyncTaskManager
}
