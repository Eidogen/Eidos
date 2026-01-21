package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
)

// Scheduler 任务调度器
type Scheduler struct {
	cron          *cron.Cron
	lockManager   *LockManager
	execRepo      *repository.ExecutionRepository
	jobs          map[string]Job
	jobConfigs    map[string]JobConfig
	mu            sync.RWMutex
	maxConcurrent int
	running       chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
}

// JobConfig 任务配置
type JobConfig struct {
	Cron    string
	Enabled bool
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	MaxConcurrentJobs int
	RedisClient       redis.UniversalClient
}

// NewScheduler 创建调度器
func NewScheduler(cfg *SchedulerConfig, execRepo *repository.ExecutionRepository) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	maxConcurrent := cfg.MaxConcurrentJobs
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	return &Scheduler{
		cron:          cron.New(cron.WithSeconds()), // 支持秒级调度
		lockManager:   NewLockManager(cfg.RedisClient),
		execRepo:      execRepo,
		jobs:          make(map[string]Job),
		jobConfigs:    make(map[string]JobConfig),
		maxConcurrent: maxConcurrent,
		running:       make(chan struct{}, maxConcurrent),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// RegisterJob 注册任务
func (s *Scheduler) RegisterJob(job Job, config JobConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[job.Name()]; exists {
		return fmt.Errorf("job %s already registered", job.Name())
	}

	s.jobs[job.Name()] = job
	s.jobConfigs[job.Name()] = config

	if !config.Enabled {
		logger.Info("job registered but disabled", "job", job.Name())
		return nil
	}

	// 添加到 cron 调度
	_, err := s.cron.AddFunc(config.Cron, func() {
		s.executeJob(job)
	})
	if err != nil {
		delete(s.jobs, job.Name())
		delete(s.jobConfigs, job.Name())
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	logger.Info("job registered",
		"job", job.Name(),
		"cron", config.Cron)

	return nil
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.cron.Start()
	logger.Info("scheduler started")
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.cancel()
	ctx := s.cron.Stop()
	<-ctx.Done()
	logger.Info("scheduler stopped")
}

// TriggerJob 手动触发任务
func (s *Scheduler) TriggerJob(jobName string) error {
	s.mu.RLock()
	job, exists := s.jobs[jobName]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not found", jobName)
	}

	go s.executeJob(job)
	return nil
}

// executeJob 执行任务
func (s *Scheduler) executeJob(job Job) {
	// 检查是否达到最大并发数
	select {
	case s.running <- struct{}{}:
		defer func() { <-s.running }()
	default:
		logger.Warn("max concurrent jobs reached, skipping",
			"job", job.Name())
		s.recordExecution(job.Name(), model.JobStatusSkipped, nil, nil, "max concurrent jobs reached")
		return
	}

	// 检查调度器是否已停止
	select {
	case <-s.ctx.Done():
		return
	default:
	}

	// 创建执行上下文
	ctx, cancel := context.WithTimeout(s.ctx, job.Timeout())
	defer cancel()

	// 获取分布式锁
	var lock *DistributedLock
	if job.RequiresLock() {
		lock = s.lockManager.NewLock(job.Name(), job.LockTTL(), job.UseWatchdog())
		acquired, err := lock.TryLock(ctx)
		if err != nil {
			logger.Error("failed to acquire lock",
				"job", job.Name(),
				"error", err)
			s.recordExecution(job.Name(), model.JobStatusFailed, nil, nil, "failed to acquire lock: "+err.Error())
			return
		}
		if !acquired {
			logger.Debug("job is already running on another instance",
				"job", job.Name())
			s.recordExecution(job.Name(), model.JobStatusSkipped, nil, nil, "job is running on another instance")
			return
		}
		defer func() {
			if err := lock.Unlock(context.Background()); err != nil {
				logger.Error("failed to release lock",
					"job", job.Name(),
					"error", err)
			}
		}()
	}

	// 记录开始执行
	startTime := time.Now()
	exec := &model.JobExecution{
		JobName:   job.Name(),
		Status:    model.JobStatusRunning,
		StartedAt: startTime.UnixMilli(),
	}
	if err := s.execRepo.Create(ctx, exec); err != nil {
		logger.Error("failed to record job start",
			"job", job.Name(),
			"error", err)
	}

	// 执行任务
	logger.Info("starting job", "job", job.Name())

	result, err := job.Execute(ctx)

	// 更新执行记录
	finishTime := time.Now()
	duration := int(finishTime.Sub(startTime).Milliseconds())
	exec.FinishedAt = ptrInt64(finishTime.UnixMilli())
	exec.DurationMs = &duration

	if err != nil {
		exec.Status = model.JobStatusFailed
		errMsg := err.Error()
		exec.ErrorMessage = &errMsg
		logger.Error("job failed",
			"job", job.Name(),
			"duration", finishTime.Sub(startTime),
			"error", err)
	} else {
		exec.Status = model.JobStatusSuccess
		if result != nil {
			exec.Result = result.ToJSONResult()
		}
		logger.Info("job completed",
			"job", job.Name(),
			"duration", finishTime.Sub(startTime),
			"result", result)
	}

	if err := s.execRepo.Update(context.Background(), exec); err != nil {
		logger.Error("failed to update job execution",
			"job", job.Name(),
			"error", err)
	}
}

// recordExecution 记录执行状态 (用于跳过等情况)
func (s *Scheduler) recordExecution(jobName string, status model.JobStatus, result *JobResult, errResult error, message string) {
	now := time.Now().UnixMilli()
	exec := &model.JobExecution{
		JobName:   jobName,
		Status:    status,
		StartedAt: now,
		FinishedAt: ptrInt64(now),
		DurationMs: ptrInt(0),
	}

	if message != "" {
		exec.ErrorMessage = &message
	}
	if errResult != nil {
		errMsg := errResult.Error()
		exec.ErrorMessage = &errMsg
	}
	if result != nil {
		exec.Result = result.ToJSONResult()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.execRepo.Create(ctx, exec); err != nil {
		logger.Error("failed to record job execution",
			"job", jobName,
			"error", err)
	}
}

// GetJobStatus 获取任务状态
func (s *Scheduler) GetJobStatus(jobName string) (*JobStatus, error) {
	s.mu.RLock()
	job, exists := s.jobs[jobName]
	config := s.jobConfigs[jobName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("job %s not found", jobName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取最近执行记录
	lastExec, err := s.execRepo.GetLatestByJobName(ctx, jobName)
	if err != nil {
		return nil, err
	}

	// 检查是否正在运行
	isLocked, _ := s.lockManager.IsLocked(ctx, jobName)

	status := &JobStatus{
		Name:     jobName,
		Enabled:  config.Enabled,
		Cron:     config.Cron,
		Timeout:  job.Timeout(),
		IsLocked: isLocked,
	}

	if lastExec != nil {
		status.LastStatus = string(lastExec.Status)
		status.LastStartedAt = lastExec.StartedAt
		if lastExec.FinishedAt != nil {
			status.LastFinishedAt = *lastExec.FinishedAt
		}
		if lastExec.DurationMs != nil {
			status.LastDurationMs = *lastExec.DurationMs
		}
		if lastExec.ErrorMessage != nil {
			status.LastError = *lastExec.ErrorMessage
		}
	}

	return status, nil
}

// ListJobStatus 列出所有任务状态
func (s *Scheduler) ListJobStatus() ([]*JobStatus, error) {
	s.mu.RLock()
	jobNames := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		jobNames = append(jobNames, name)
	}
	s.mu.RUnlock()

	statuses := make([]*JobStatus, 0, len(jobNames))
	for _, name := range jobNames {
		status, err := s.GetJobStatus(name)
		if err != nil {
			logger.Error("failed to get job status",
				"job", name,
				"error", err)
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// JobStatus 任务状态
type JobStatus struct {
	Name           string
	Enabled        bool
	Cron           string
	Timeout        time.Duration
	IsLocked       bool
	LastStatus     string
	LastStartedAt  int64
	LastFinishedAt int64
	LastDurationMs int
	LastError      string
}

// Helper functions
func ptrInt64(v int64) *int64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}
