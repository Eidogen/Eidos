package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupStatisticsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// 创建表结构
	err = db.AutoMigrate(&model.Statistics{})
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// 为 SQLite 创建唯一索引 (支持 ON CONFLICT)
	err = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_stats_unique
		ON eidos_jobs_statistics(stat_type, stat_date, COALESCE(stat_hour, -1), COALESCE(market, ''), metric_name)
	`).Error
	if err != nil {
		// 如果创建索引失败，记录但继续（某些测试可能不需要 upsert）
		t.Logf("Note: Could not create unique index: %v", err)
	}

	return db
}

// createStatistics 辅助函数：直接插入统计数据
func createStatistics(ctx context.Context, db *gorm.DB, stat *model.Statistics) error {
	stat.CreatedAt = time.Now().UnixMilli()
	return db.WithContext(ctx).Create(stat).Error
}

func TestStatisticsRepository_Create(t *testing.T) {
	db := setupStatisticsTestDB(t)
	ctx := context.Background()

	hour := 10
	stat := &model.Statistics{
		StatType:    string(model.StatTypeHourly),
		StatDate:    "2024-01-15",
		StatHour:    &hour,
		MetricName:  model.MetricTradeVolume,
		MetricValue: "1000.00",
		CreatedAt:   time.Now().UnixMilli(),
	}

	err := createStatistics(ctx, db, stat)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if stat.ID == 0 {
		t.Error("Expected ID to be set after create")
	}

	var count int64
	db.Model(&model.Statistics{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 record, got %d", count)
	}
}

func TestStatisticsRepository_BatchUpsert_Empty(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	// 空列表不应该出错
	err := repo.BatchUpsert(ctx, []*model.Statistics{})
	if err != nil {
		t.Fatalf("BatchUpsert with empty list should not fail: %v", err)
	}
}

func TestStatisticsRepository_GetByDateAndType(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	// 创建测试数据
	metrics := []string{model.MetricTradeVolume, model.MetricTradeCount, model.MetricActiveUsers}
	for _, metric := range metrics {
		stat := &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    "2024-01-15",
			MetricName:  metric,
			MetricValue: "100.00",
			CreatedAt:   time.Now().UnixMilli(),
		}
		createStatistics(ctx, db, stat)
	}

	// 创建其他日期的数据
	stat := &model.Statistics{
		StatType:    string(model.StatTypeDaily),
		StatDate:    "2024-01-16",
		MetricName:  model.MetricTradeVolume,
		MetricValue: "200.00",
		CreatedAt:   time.Now().UnixMilli(),
	}
	createStatistics(ctx, db, stat)

	results, err := repo.GetByDateAndType(ctx, "2024-01-15", string(model.StatTypeDaily))
	if err != nil {
		t.Fatalf("GetByDateAndType failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestStatisticsRepository_GetByDateRangeAndMarket(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	market1 := "BTC-USDT"
	market2 := "ETH-USDT"

	// 创建多个市场的数据
	dates := []string{"2024-01-14", "2024-01-15", "2024-01-16"}
	for _, date := range dates {
		for _, market := range []*string{&market1, &market2, nil} {
			stat := &model.Statistics{
				StatType:    string(model.StatTypeDaily),
				StatDate:    date,
				Market:      market,
				MetricName:  model.MetricTradeVolume,
				MetricValue: "100.00",
				CreatedAt:   time.Now().UnixMilli(),
			}
			createStatistics(ctx, db, stat)
		}
	}

	// 查询特定市场
	results, err := repo.GetByDateRangeAndMarket(ctx, "2024-01-14", "2024-01-16", market1)
	if err != nil {
		t.Fatalf("GetByDateRangeAndMarket failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results for market %s, got %d", market1, len(results))
	}

	// 查询所有市场
	allResults, err := repo.GetByDateRangeAndMarket(ctx, "2024-01-14", "2024-01-16", "")
	if err != nil {
		t.Fatalf("GetByDateRangeAndMarket (all) failed: %v", err)
	}

	if len(allResults) != 9 {
		t.Errorf("Expected 9 results for all markets, got %d", len(allResults))
	}
}

func TestStatisticsRepository_GetMetricByDateRange(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	market := "BTC-USDT"

	// 创建多天的数据
	dates := []string{"2024-01-13", "2024-01-14", "2024-01-15"}
	for _, date := range dates {
		stat := &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    date,
			Market:      &market,
			MetricName:  model.MetricTradeVolume,
			MetricValue: "100.00",
			CreatedAt:   time.Now().UnixMilli(),
		}
		createStatistics(ctx, db, stat)
	}

	// 查询特定指标
	results, err := repo.GetMetricByDateRange(ctx, model.MetricTradeVolume, "2024-01-13", "2024-01-15", &market)
	if err != nil {
		t.Fatalf("GetMetricByDateRange failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// 验证排序
	for i := 1; i < len(results); i++ {
		if results[i].StatDate < results[i-1].StatDate {
			t.Error("Results should be sorted by stat_date asc")
		}
	}
}

func TestStatisticsRepository_GetLatestDaily(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	// 创建多天的数据
	dates := []string{"2024-01-13", "2024-01-14", "2024-01-15"}
	for _, date := range dates {
		stat := &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    date,
			MetricName:  model.MetricTradeVolume,
			MetricValue: date, // 用日期作为值便于验证
			CreatedAt:   time.Now().UnixMilli(),
		}
		createStatistics(ctx, db, stat)
	}

	latest, err := repo.GetLatestDaily(ctx, nil)
	if err != nil {
		t.Fatalf("GetLatestDaily failed: %v", err)
	}

	if len(latest) == 0 {
		t.Fatal("Expected non-empty result")
	}

	// 验证是最新日期（检查包含日期，因为 SQLite 可能返回带时间的格式）
	for _, s := range latest {
		if !strings.HasPrefix(s.StatDate, "2024-01-15") {
			t.Errorf("Expected latest date starting with 2024-01-15, got %s", s.StatDate)
		}
	}
}

func TestStatisticsRepository_GetLatestDaily_WithMarket(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	market := "BTC-USDT"

	// 创建带市场的数据
	stat := &model.Statistics{
		StatType:    string(model.StatTypeDaily),
		StatDate:    "2024-01-15",
		Market:      &market,
		MetricName:  model.MetricTradeVolume,
		MetricValue: "1000.00",
		CreatedAt:   time.Now().UnixMilli(),
	}
	createStatistics(ctx, db, stat)

	latest, err := repo.GetLatestDaily(ctx, &market)
	if err != nil {
		t.Fatalf("GetLatestDaily with market failed: %v", err)
	}

	if len(latest) != 1 {
		t.Errorf("Expected 1 result, got %d", len(latest))
	}
}

func TestStatisticsRepository_GetLatestDaily_NoData(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	// 无数据时不应报错
	latest, err := repo.GetLatestDaily(ctx, nil)
	if err != nil {
		t.Fatalf("GetLatestDaily with no data should not fail: %v", err)
	}

	if latest != nil && len(latest) != 0 {
		t.Errorf("Expected empty result, got %d records", len(latest))
	}
}

func TestStatisticsRepository_Delete(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	stat := &model.Statistics{
		StatType:    string(model.StatTypeDaily),
		StatDate:    "2024-01-15",
		MetricName:  model.MetricTradeVolume,
		MetricValue: "100.00",
		CreatedAt:   time.Now().UnixMilli(),
	}
	createStatistics(ctx, db, stat)

	err := repo.Delete(ctx, stat.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	var count int64
	db.Model(&model.Statistics{}).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 records after delete, got %d", count)
	}
}

func TestStatisticsRepository_DeleteByDateRange(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	dates := []string{"2024-01-10", "2024-01-12", "2024-01-14", "2024-01-15"}
	for _, date := range dates {
		stat := &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    date,
			MetricName:  model.MetricTradeVolume,
			MetricValue: "100.00",
			CreatedAt:   time.Now().UnixMilli(),
		}
		createStatistics(ctx, db, stat)
	}

	deleted, err := repo.DeleteByDateRange(ctx, "2024-01-13")
	if err != nil {
		t.Fatalf("DeleteByDateRange failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected to delete 2 records, deleted %d", deleted)
	}

	var remaining int64
	db.Model(&model.Statistics{}).Count(&remaining)
	if remaining != 2 {
		t.Errorf("Expected 2 remaining records, got %d", remaining)
	}
}

func TestStatisticsRepository_HourlyStats(t *testing.T) {
	db := setupStatisticsTestDB(t)
	repo := NewStatisticsRepository(db)
	ctx := context.Background()

	// 创建小时级别的统计数据
	for hour := 0; hour < 24; hour++ {
		h := hour
		stat := &model.Statistics{
			StatType:    string(model.StatTypeHourly),
			StatDate:    "2024-01-15",
			StatHour:    &h,
			MetricName:  model.MetricTradeVolume,
			MetricValue: "100.00",
			CreatedAt:   time.Now().UnixMilli(),
		}
		createStatistics(ctx, db, stat)
	}

	results, err := repo.GetByDateAndType(ctx, "2024-01-15", string(model.StatTypeHourly))
	if err != nil {
		t.Fatalf("GetByDateAndType for hourly failed: %v", err)
	}

	if len(results) != 24 {
		t.Errorf("Expected 24 hourly records, got %d", len(results))
	}
}

func TestStatisticsModel(t *testing.T) {
	hour := 10
	market := "BTC-USDT"

	stat := &model.Statistics{
		ID:          1,
		StatType:    string(model.StatTypeHourly),
		StatDate:    "2024-01-15",
		StatHour:    &hour,
		Market:      &market,
		MetricName:  model.MetricTradeVolume,
		MetricValue: "1000.50",
		CreatedAt:   time.Now().UnixMilli(),
	}

	if stat.StatType != string(model.StatTypeHourly) {
		t.Errorf("Expected stat type %s, got %s", model.StatTypeHourly, stat.StatType)
	}

	if stat.StatDate != "2024-01-15" {
		t.Errorf("Expected stat date 2024-01-15, got %s", stat.StatDate)
	}

	if *stat.StatHour != 10 {
		t.Errorf("Expected stat hour 10, got %d", *stat.StatHour)
	}

	if *stat.Market != "BTC-USDT" {
		t.Errorf("Expected market BTC-USDT, got %s", *stat.Market)
	}

	if stat.MetricName != model.MetricTradeVolume {
		t.Errorf("Expected metric name %s, got %s", model.MetricTradeVolume, stat.MetricName)
	}

	if stat.MetricValue != "1000.50" {
		t.Errorf("Expected metric value 1000.50, got %s", stat.MetricValue)
	}
}

func TestStatisticsConstants(t *testing.T) {
	// 验证常量定义
	if model.StatTypeDaily != "daily" {
		t.Errorf("Expected StatTypeDaily 'daily', got '%s'", model.StatTypeDaily)
	}

	if model.StatTypeHourly != "hourly" {
		t.Errorf("Expected StatTypeHourly 'hourly', got '%s'", model.StatTypeHourly)
	}

	// 验证指标名称常量
	expectedMetrics := map[string]string{
		"trade_volume": model.MetricTradeVolume,
		"trade_count":  model.MetricTradeCount,
		"active_users": model.MetricActiveUsers,
	}

	for expected, actual := range expectedMetrics {
		if actual != expected {
			t.Errorf("Expected metric '%s', got '%s'", expected, actual)
		}
	}
}
