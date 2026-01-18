package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
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

func TestExecutionRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	exec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}

	err := repo.Create(ctx, exec)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if exec.ID == 0 {
		t.Error("Expected ID to be set after create")
	}
}

func TestExecutionRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	// 创建记录
	exec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}
	repo.Create(ctx, exec)

	// 更新记录
	finishedAt := time.Now().UnixMilli()
	durationMs := 100
	exec.Status = model.JobStatusSuccess
	exec.FinishedAt = &finishedAt
	exec.DurationMs = &durationMs

	err := repo.Update(ctx, exec)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 验证更新
	var updated model.JobExecution
	db.First(&updated, exec.ID)
	if updated.Status != model.JobStatusSuccess {
		t.Errorf("Expected status %s, got %s", model.JobStatusSuccess, updated.Status)
	}
}

func TestExecutionRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	exec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusSuccess,
		StartedAt: time.Now().UnixMilli(),
	}
	repo.Create(ctx, exec)

	found, err := repo.GetByID(ctx, exec.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.JobName != exec.JobName {
		t.Errorf("Expected job name %s, got %s", exec.JobName, found.JobName)
	}
}

func TestExecutionRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	found, err := repo.GetByID(ctx, 9999)
	if err != nil {
		t.Fatalf("GetByID should not return error for not found: %v", err)
	}

	if found != nil {
		t.Error("Expected nil for not found record")
	}
}

func TestExecutionRepository_GetLatestByJobName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()

	// 创建多条记录
	for i := 0; i < 3; i++ {
		exec := &model.JobExecution{
			JobName:   "test-job",
			Status:    model.JobStatusSuccess,
			StartedAt: now - int64(i*1000),
		}
		repo.Create(ctx, exec)
	}

	// 获取最新的记录
	last, err := repo.GetLatestByJobName(ctx, "test-job")
	if err != nil {
		t.Fatalf("GetLatestByJobName failed: %v", err)
	}

	if last.StartedAt != now {
		t.Error("Expected the most recent record")
	}
}

func TestExecutionRepository_GetLatestByJobName_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	last, err := repo.GetLatestByJobName(ctx, "non-existent")
	if err != nil {
		t.Fatalf("GetLatestByJobName should not error: %v", err)
	}

	if last != nil {
		t.Error("Expected nil for non-existent job")
	}
}

func TestExecutionRepository_GetRunningByJobName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	// 创建一个正在运行的任务
	runningExec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}
	repo.Create(ctx, runningExec)

	// 创建一个已完成的任务
	completedExec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusSuccess,
		StartedAt: time.Now().UnixMilli() - 1000,
	}
	repo.Create(ctx, completedExec)

	running, err := repo.GetRunningByJobName(ctx, "test-job")
	if err != nil {
		t.Fatalf("GetRunningByJobName failed: %v", err)
	}

	if running == nil {
		t.Fatal("Expected to find running job")
	}

	if running.Status != model.JobStatusRunning {
		t.Errorf("Expected running status, got %s", running.Status)
	}
}

func TestExecutionRepository_ListByJobName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	// 创建多条记录
	for i := 0; i < 5; i++ {
		exec := &model.JobExecution{
			JobName:   "job-a",
			Status:    model.JobStatusSuccess,
			StartedAt: time.Now().UnixMilli() - int64(i*1000),
		}
		repo.Create(ctx, exec)
	}

	for i := 0; i < 3; i++ {
		exec := &model.JobExecution{
			JobName:   "job-b",
			Status:    model.JobStatusSuccess,
			StartedAt: time.Now().UnixMilli() - int64(i*1000),
		}
		repo.Create(ctx, exec)
	}

	// 查询 job-a
	results, err := repo.ListByJobName(ctx, "job-a", 10)
	if err != nil {
		t.Fatalf("ListByJobName failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// 验证排序 (按 started_at 降序)
	for i := 1; i < len(results); i++ {
		if results[i].StartedAt > results[i-1].StartedAt {
			t.Error("Results should be sorted by started_at desc")
		}
	}
}

func TestExecutionRepository_ListByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	// 创建不同状态的记录
	statuses := []model.JobStatus{
		model.JobStatusSuccess,
		model.JobStatusSuccess,
		model.JobStatusFailed,
		model.JobStatusRunning,
	}

	for i, status := range statuses {
		exec := &model.JobExecution{
			JobName:   "test-job",
			Status:    status,
			StartedAt: time.Now().UnixMilli() - int64(i*1000),
		}
		repo.Create(ctx, exec)
	}

	// 查询成功的记录
	results, err := repo.ListByStatus(ctx, model.JobStatusSuccess, 10)
	if err != nil {
		t.Fatalf("ListByStatus failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 success records, got %d", len(results))
	}
}

func TestExecutionRepository_ListByTimeRange(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	now := time.Now()
	baseTime := now.UnixMilli()

	// 创建不同时间的记录
	times := []int64{
		baseTime - 3600000, // 1小时前
		baseTime - 1800000, // 30分钟前
		baseTime - 600000,  // 10分钟前
		baseTime,           // 现在
	}

	for _, ts := range times {
		exec := &model.JobExecution{
			JobName:   "test-job",
			Status:    model.JobStatusSuccess,
			StartedAt: ts,
		}
		repo.Create(ctx, exec)
	}

	// 查询最近30分钟
	startTime := baseTime - 1800000
	endTime := baseTime + 1000

	results, err := repo.ListByTimeRange(ctx, startTime, endTime, 10)
	if err != nil {
		t.Fatalf("ListByTimeRange failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 records in range, got %d", len(results))
	}
}

func TestExecutionRepository_CountByJobNameAndStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	// 创建记录
	for i := 0; i < 5; i++ {
		exec := &model.JobExecution{
			JobName:   "count-test",
			Status:    model.JobStatusSuccess,
			StartedAt: time.Now().UnixMilli(),
		}
		repo.Create(ctx, exec)
	}

	// 创建一个失败的记录
	exec := &model.JobExecution{
		JobName:   "count-test",
		Status:    model.JobStatusFailed,
		StartedAt: time.Now().UnixMilli(),
	}
	repo.Create(ctx, exec)

	count, err := repo.CountByJobNameAndStatus(ctx, "count-test", model.JobStatusSuccess)
	if err != nil {
		t.Fatalf("CountByJobNameAndStatus failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

func TestExecutionRepository_GetLastSuccessTime(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	finishedAt := now

	// 创建成功的记录
	exec := &model.JobExecution{
		JobName:    "test-job",
		Status:     model.JobStatusSuccess,
		StartedAt:  now - 1000,
		FinishedAt: &finishedAt,
	}
	repo.Create(ctx, exec)

	// 创建失败的记录 (更新)
	failedExec := &model.JobExecution{
		JobName:   "test-job",
		Status:    model.JobStatusFailed,
		StartedAt: now,
	}
	repo.Create(ctx, failedExec)

	lastSuccess, err := repo.GetLastSuccessTime(ctx, "test-job")
	if err != nil {
		t.Fatalf("GetLastSuccessTime failed: %v", err)
	}

	if lastSuccess != finishedAt {
		t.Errorf("Expected last success time %d, got %d", finishedAt, lastSuccess)
	}
}

func TestExecutionRepository_CleanupOldRecords(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	now := time.Now().UnixMilli()

	// 创建记录
	times := []int64{
		now - 3600000*24*10, // 10天前
		now - 3600000*24*5,  // 5天前
		now - 3600000*24*2,  // 2天前
		now,                 // 现在
	}

	for _, ts := range times {
		exec := &model.JobExecution{
			JobName:   "delete-test",
			Status:    model.JobStatusSuccess,
			StartedAt: ts,
		}
		repo.Create(ctx, exec)
	}

	// 删除3天前的记录
	cutoff := now - 3600000*24*3
	deleted, err := repo.CleanupOldRecords(ctx, cutoff, 100)
	if err != nil {
		t.Fatalf("CleanupOldRecords failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected to delete 2 records, deleted %d", deleted)
	}

	// 验证剩余记录
	var remaining int64
	db.Model(&model.JobExecution{}).Count(&remaining)
	if remaining != 2 {
		t.Errorf("Expected 2 remaining records, got %d", remaining)
	}
}

func TestExecutionRepository_MarkStaleRunningAsFailed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExecutionRepository(db)
	ctx := context.Background()

	now := time.Now()

	// 创建一个很久以前开始的运行中任务
	staleExec := &model.JobExecution{
		JobName:   "stale-job",
		Status:    model.JobStatusRunning,
		StartedAt: now.Add(-2 * time.Hour).UnixMilli(),
	}
	repo.Create(ctx, staleExec)

	// 创建一个刚开始的运行中任务
	recentExec := &model.JobExecution{
		JobName:   "recent-job",
		Status:    model.JobStatusRunning,
		StartedAt: now.UnixMilli(),
	}
	repo.Create(ctx, recentExec)

	// 标记超过1小时的任务为失败
	affected, err := repo.MarkStaleRunningAsFailed(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("MarkStaleRunningAsFailed failed: %v", err)
	}

	if affected != 1 {
		t.Errorf("Expected 1 affected record, got %d", affected)
	}

	// 验证旧任务已被标记为失败
	updated, _ := repo.GetByID(ctx, staleExec.ID)
	if updated.Status != model.JobStatusFailed {
		t.Errorf("Expected stale job to be marked as failed, got %s", updated.Status)
	}

	// 验证新任务仍在运行
	recent, _ := repo.GetByID(ctx, recentExec.ID)
	if recent.Status != model.JobStatusRunning {
		t.Errorf("Expected recent job to still be running, got %s", recent.Status)
	}
}
