package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 创建测试数据库
func setupSchedulerTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&model.JobExecution{})
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	return db
}

// mockJob 模拟任务用于测试
type mockJob struct {
	BaseJob
	executeFunc func(ctx context.Context) (*JobResult, error)
	execCount   int64
	mu          sync.Mutex
}

func newMockJob(name string, executeFunc func(ctx context.Context) (*JobResult, error)) *mockJob {
	return &mockJob{
		BaseJob: NewBaseJob(
			name,
			30*time.Second,
			60*time.Second,
			false,
		),
		executeFunc: executeFunc,
	}
}

// newMockJobNoLock 创建不需要锁的 mock job
func newMockJobNoLock(name string, executeFunc func(ctx context.Context) (*JobResult, error)) *mockJobNoLock {
	return &mockJobNoLock{
		name:        name,
		timeout:     30 * time.Second,
		executeFunc: executeFunc,
	}
}

// mockJobNoLock 不需要锁的 mock job
type mockJobNoLock struct {
	name        string
	timeout     time.Duration
	executeFunc func(ctx context.Context) (*JobResult, error)
	execCount   int64
}

func (j *mockJobNoLock) Name() string          { return j.name }
func (j *mockJobNoLock) Timeout() time.Duration { return j.timeout }
func (j *mockJobNoLock) LockTTL() time.Duration { return 60 * time.Second }
func (j *mockJobNoLock) UseWatchdog() bool      { return false }
func (j *mockJobNoLock) RequiresLock() bool     { return false }
func (j *mockJobNoLock) Execute(ctx context.Context) (*JobResult, error) {
	atomic.AddInt64(&j.execCount, 1)
	if j.executeFunc != nil {
		return j.executeFunc(ctx)
	}
	return &JobResult{ProcessedCount: 1, AffectedCount: 1}, nil
}
func (j *mockJobNoLock) GetExecCount() int64 {
	return atomic.LoadInt64(&j.execCount)
}

func (j *mockJob) Execute(ctx context.Context) (*JobResult, error) {
	atomic.AddInt64(&j.execCount, 1)
	if j.executeFunc != nil {
		return j.executeFunc(ctx)
	}
	return &JobResult{ProcessedCount: 1, AffectedCount: 1}, nil
}

func (j *mockJob) GetExecCount() int64 {
	return atomic.LoadInt64(&j.execCount)
}

func TestScheduler_RegisterJob(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	job := newMockJob("test-job", nil)
	config := JobConfig{
		Cron:    "*/5 * * * * *",
		Enabled: true,
	}

	err := scheduler.RegisterJob(job, config)
	if err != nil {
		t.Fatalf("RegisterJob failed: %v", err)
	}

	// 验证任务已注册（直接检查内部状态，避免调用 GetJobStatus 因为没有 Redis）
	scheduler.mu.RLock()
	_, exists := scheduler.jobs["test-job"]
	jobConfig := scheduler.jobConfigs["test-job"]
	scheduler.mu.RUnlock()

	if !exists {
		t.Fatal("Expected job to be registered")
	}

	if !jobConfig.Enabled {
		t.Error("Expected job to be enabled")
	}
}

func TestScheduler_RegisterJob_Disabled(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	job := newMockJob("disabled-job", nil)
	config := JobConfig{
		Cron:    "*/5 * * * * *",
		Enabled: false,
	}

	err := scheduler.RegisterJob(job, config)
	if err != nil {
		t.Fatalf("RegisterJob failed: %v", err)
	}

	// 直接检查配置
	scheduler.mu.RLock()
	jobConfig := scheduler.jobConfigs["disabled-job"]
	scheduler.mu.RUnlock()

	if jobConfig.Enabled {
		t.Error("Expected job to be disabled")
	}
}

func TestScheduler_RegisterJob_Duplicate(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	job := newMockJob("dup-job", nil)
	config := JobConfig{
		Cron:    "*/5 * * * * *",
		Enabled: true,
	}

	// 首次注册
	err := scheduler.RegisterJob(job, config)
	if err != nil {
		t.Fatalf("First RegisterJob failed: %v", err)
	}

	// 重复注册应该失败
	err = scheduler.RegisterJob(job, config)
	if err == nil {
		t.Error("Expected error for duplicate job registration")
	}
}

func TestScheduler_JobNotFound(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	// TriggerJob for non-existent should fail
	err := scheduler.TriggerJob("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

func TestScheduler_ListJobs(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	// 注册多个任务
	for i := 0; i < 3; i++ {
		job := newMockJob("test-job-"+string(rune('a'+i)), nil)
		scheduler.RegisterJob(job, JobConfig{
			Cron:    "*/5 * * * * *",
			Enabled: true,
		})
	}

	// 验证注册的任务数量
	scheduler.mu.RLock()
	count := len(scheduler.jobs)
	scheduler.mu.RUnlock()

	if count != 3 {
		t.Errorf("Expected 3 jobs registered, got %d", count)
	}
}

func TestScheduler_TriggerJob(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	executed := make(chan struct{}, 1)
	// 使用不需要锁的 job
	job := newMockJobNoLock("trigger-test", func(ctx context.Context) (*JobResult, error) {
		executed <- struct{}{}
		return &JobResult{ProcessedCount: 1}, nil
	})

	scheduler.RegisterJob(job, JobConfig{
		Cron:    "0 0 0 1 1 *", // 每年1月1日 0时0分0秒 (几乎不会触发)
		Enabled: true,
	})

	scheduler.Start()
	defer scheduler.Stop()

	// 手动触发
	err := scheduler.TriggerJob("trigger-test")
	if err != nil {
		t.Fatalf("TriggerJob failed: %v", err)
	}

	// 等待执行
	select {
	case <-executed:
		// 成功
	case <-time.After(2 * time.Second):
		t.Error("Job was not executed within timeout")
	}
}

func TestScheduler_TriggerJob_NotFound(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	err := scheduler.TriggerJob("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent job")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 3}, execRepo)

	// 使用不需要锁的 job
	job := newMockJobNoLock("start-stop-test", nil)
	scheduler.RegisterJob(job, JobConfig{
		Cron:    "*/1 * * * * *", // 每秒
		Enabled: true,
	})

	scheduler.Start()

	// 等待几次执行
	time.Sleep(3 * time.Second)

	scheduler.Stop()

	execCount := job.GetExecCount()
	if execCount < 2 {
		t.Errorf("Expected at least 2 executions, got %d", execCount)
	}

	// 停止后不应再有执行
	countAfterStop := job.GetExecCount()
	time.Sleep(2 * time.Second)
	if job.GetExecCount() != countAfterStop {
		t.Error("Job should not execute after scheduler is stopped")
	}
}

func TestScheduler_Concurrency(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 2}, execRepo)

	executing := int64(0)
	maxConcurrent := int64(0)
	var mu sync.Mutex

	slowJob := func(ctx context.Context) (*JobResult, error) {
		current := atomic.AddInt64(&executing, 1)

		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		time.Sleep(500 * time.Millisecond)
		atomic.AddInt64(&executing, -1)
		return &JobResult{}, nil
	}

	// 注册多个慢任务（不需要锁）
	for i := 0; i < 5; i++ {
		job := newMockJobNoLock("slow-job-"+string(rune('a'+i)), slowJob)
		scheduler.RegisterJob(job, JobConfig{
			Cron:    "*/1 * * * * *",
			Enabled: true,
		})
	}

	scheduler.Start()
	time.Sleep(3 * time.Second)
	scheduler.Stop()

	mu.Lock()
	maxConc := maxConcurrent
	mu.Unlock()

	if maxConc > 2 {
		t.Errorf("Max concurrent jobs exceeded limit: %d > 2", maxConc)
	}
}

func TestJobResult(t *testing.T) {
	result := &JobResult{
		ProcessedCount: 10,
		AffectedCount:  5,
		ErrorCount:     1,
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	if result.ProcessedCount != 10 {
		t.Errorf("Expected ProcessedCount 10, got %d", result.ProcessedCount)
	}

	if result.AffectedCount != 5 {
		t.Errorf("Expected AffectedCount 5, got %d", result.AffectedCount)
	}

	if result.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount 1, got %d", result.ErrorCount)
	}

	if result.Details["key"] != "value" {
		t.Error("Expected Details to contain key=value")
	}
}

func TestJobResult_ToJSONResult(t *testing.T) {
	result := &JobResult{
		ProcessedCount: 10,
		AffectedCount:  5,
		ErrorCount:     1,
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	jsonResult := result.ToJSONResult()
	if jsonResult == nil {
		t.Fatal("Expected non-nil JSON result")
	}

	// 验证转换后的格式 (使用类型断言避免类型不匹配)
	if pc, ok := jsonResult["processed_count"].(int); !ok || pc != 10 {
		t.Errorf("Expected processed_count 10, got %v", jsonResult["processed_count"])
	}

	if ac, ok := jsonResult["affected_count"].(int); !ok || ac != 5 {
		t.Errorf("Expected affected_count 5, got %v", jsonResult["affected_count"])
	}

	if ec, ok := jsonResult["error_count"].(int); !ok || ec != 1 {
		t.Errorf("Expected error_count 1, got %v", jsonResult["error_count"])
	}
}

func TestBaseJob(t *testing.T) {
	job := NewBaseJob("test", 30*time.Second, 60*time.Second, true)

	if job.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", job.Name())
	}

	if job.Timeout() != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", job.Timeout())
	}

	if job.LockTTL() != 60*time.Second {
		t.Errorf("Expected lockTTL 60s, got %v", job.LockTTL())
	}

	if !job.UseWatchdog() {
		t.Error("Expected UseWatchdog to be true")
	}
}

func TestBaseJob_RequiresLock(t *testing.T) {
	job := NewBaseJob("test", 30*time.Second, 60*time.Second, false)

	// 默认情况下任务需要锁
	if !job.RequiresLock() {
		t.Error("Expected RequiresLock to be true by default")
	}
}

func TestDefaultJobConfigs(t *testing.T) {
	// 验证默认配置存在
	expectedJobs := []string{
		JobNameHealthMonitor,
		JobNameCleanupOrders,
		JobNameStatsAgg,
		JobNameKlineAgg,
		JobNameReconciliation,
		JobNameArchiveData,
		JobNameDataCleanup,
		JobNamePartitionManage,
	}

	// 不需要分布式锁的任务
	noLockJobs := map[string]bool{
		JobNameHealthMonitor: true, // 健康检查无需锁
	}

	for _, jobName := range expectedJobs {
		config, ok := DefaultJobConfigs[jobName]
		if !ok {
			t.Errorf("Missing default config for job: %s", jobName)
			continue
		}

		if config.Cron == "" {
			t.Errorf("Job %s has empty cron expression", jobName)
		}

		if config.Timeout <= 0 {
			t.Errorf("Job %s has invalid timeout: %v", jobName, config.Timeout)
		}

		// 只有需要锁的任务才验证 LockTTL
		if !noLockJobs[jobName] && config.LockTTL <= 0 {
			t.Errorf("Job %s has invalid lockTTL: %v", jobName, config.LockTTL)
		}
	}
}

func TestJobStatus(t *testing.T) {
	status := &JobStatus{
		Name:           "test-job",
		Enabled:        true,
		Cron:           "*/5 * * * * *",
		Timeout:        30 * time.Second,
		IsLocked:       false,
		LastStatus:     "success",
		LastStartedAt:  time.Now().UnixMilli() - 1000,
		LastFinishedAt: time.Now().UnixMilli(),
		LastDurationMs: 100,
		LastError:      "",
	}

	if status.Name != "test-job" {
		t.Errorf("Expected name 'test-job', got '%s'", status.Name)
	}

	if !status.Enabled {
		t.Error("Expected job to be enabled")
	}

	if status.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", status.Timeout)
	}

	if status.LastDurationMs != 100 {
		t.Errorf("Expected duration 100ms, got %d", status.LastDurationMs)
	}
}

func TestSchedulerConfig(t *testing.T) {
	config := &SchedulerConfig{
		MaxConcurrentJobs: 5,
		RedisClient:       nil,
	}

	if config.MaxConcurrentJobs != 5 {
		t.Errorf("Expected MaxConcurrentJobs 5, got %d", config.MaxConcurrentJobs)
	}
}

func TestScheduler_DefaultConcurrency(t *testing.T) {
	db := setupSchedulerTestDB(t)
	execRepo := repository.NewExecutionRepository(db)

	// 不设置 MaxConcurrentJobs，应使用默认值
	scheduler := NewScheduler(&SchedulerConfig{MaxConcurrentJobs: 0}, execRepo)

	// 验证调度器正常创建
	job := newMockJob("test", nil)
	err := scheduler.RegisterJob(job, JobConfig{
		Cron:    "*/5 * * * * *",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("RegisterJob failed: %v", err)
	}
}

func TestLockManager_NewLock(t *testing.T) {
	// 测试 LockManager 创建锁（不需要实际 Redis）
	manager := NewLockManager(nil)
	lock := manager.NewLock("test-job", 60*time.Second, true)

	if lock == nil {
		t.Fatal("Expected non-nil lock")
	}

	if lock.key != "eidos:job:lock:test-job" {
		t.Errorf("Expected key 'eidos:job:lock:test-job', got '%s'", lock.key)
	}

	if lock.ttl != 60*time.Second {
		t.Errorf("Expected TTL 60s, got %v", lock.ttl)
	}

	if !lock.useWatchdog {
		t.Error("Expected useWatchdog to be true")
	}
}

func TestDistributedLock_Fields(t *testing.T) {
	lock := NewDistributedLock(nil, "test", 30*time.Second, false)

	if lock.key != "eidos:job:lock:test" {
		t.Errorf("Expected key 'eidos:job:lock:test', got '%s'", lock.key)
	}

	if lock.ttl != 30*time.Second {
		t.Errorf("Expected TTL 30s, got %v", lock.ttl)
	}

	if lock.useWatchdog {
		t.Error("Expected useWatchdog to be false")
	}

	if lock.value == "" {
		t.Error("Expected value to be set")
	}
}
