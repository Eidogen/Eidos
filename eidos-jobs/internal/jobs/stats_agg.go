package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
)

// StatsDataProvider 统计数据提供者接口
type StatsDataProvider interface {
	// GetHourlyTradeStats 获取小时交易统计
	GetHourlyTradeStats(ctx context.Context, startTime, endTime int64) ([]*TradeStats, error)
	// GetDailyTradeStats 获取日交易统计
	GetDailyTradeStats(ctx context.Context, date string) ([]*TradeStats, error)
	// GetActiveUserCount 获取活跃用户数
	GetActiveUserCount(ctx context.Context, startTime, endTime int64) (int64, error)
	// GetNewUserCount 获取新增用户数
	GetNewUserCount(ctx context.Context, date string) (int64, error)
}

// TradeStats 交易统计数据
type TradeStats struct {
	Market       string
	TradeVolume  decimal.Decimal
	TradeCount   int64
	FeeTotal     decimal.Decimal
	OrderCount   int64
	CancelCount  int64
}

// StatsAggJob 统计汇总任务
type StatsAggJob struct {
	scheduler.BaseJob
	statsRepo    *repository.StatisticsRepository
	dataProvider StatsDataProvider
}

// NewStatsAggJob 创建统计汇总任务
func NewStatsAggJob(
	statsRepo *repository.StatisticsRepository,
	dataProvider StatsDataProvider,
) *StatsAggJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameStatsAgg]

	return &StatsAggJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameStatsAgg,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		statsRepo:    statsRepo,
		dataProvider: dataProvider,
	}
}

// Execute 执行统计汇总
func (j *StatsAggJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	now := time.Now()

	// 1. 生成小时统计 (过去一小时)
	hourlyCount, err := j.generateHourlyStats(ctx, now)
	if err != nil {
		logger.Error("failed to generate hourly stats", "error", err)
		result.ErrorCount++
	}
	result.ProcessedCount += hourlyCount
	result.Details["hourly_stats"] = hourlyCount

	// 2. 如果是每天 00:05，额外生成昨日日统计
	if now.Hour() == 0 && now.Minute() >= 5 && now.Minute() < 10 {
		dailyCount, err := j.generateDailyStats(ctx, now.AddDate(0, 0, -1))
		if err != nil {
			logger.Error("failed to generate daily stats", "error", err)
			result.ErrorCount++
		}
		result.ProcessedCount += dailyCount
		result.Details["daily_stats"] = dailyCount
	}

	result.AffectedCount = result.ProcessedCount

	logger.Info("stats aggregation completed",
		"processed", result.ProcessedCount,
		"errors", result.ErrorCount)

	return result, nil
}

// generateHourlyStats 生成小时统计
func (j *StatsAggJob) generateHourlyStats(ctx context.Context, now time.Time) (int, error) {
	// 计算过去一小时的时间范围
	endTime := now.Truncate(time.Hour)
	startTime := endTime.Add(-time.Hour)

	statDate := startTime.Format("2006-01-02")
	statHour := startTime.Hour()

	// 获取交易统计数据
	tradeStats, err := j.dataProvider.GetHourlyTradeStats(ctx, startTime.UnixMilli(), endTime.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("failed to get hourly trade stats: %w", err)
	}

	stats := make([]*model.Statistics, 0)

	// 按市场生成统计
	for _, ts := range tradeStats {
		market := ts.Market

		stats = append(stats,
			&model.Statistics{
				StatType:    string(model.StatTypeHourly),
				StatDate:    statDate,
				StatHour:    &statHour,
				Market:      &market,
				MetricName:  model.MetricTradeVolume,
				MetricValue: ts.TradeVolume.String(),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeHourly),
				StatDate:    statDate,
				StatHour:    &statHour,
				Market:      &market,
				MetricName:  model.MetricTradeCount,
				MetricValue: fmt.Sprintf("%d", ts.TradeCount),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeHourly),
				StatDate:    statDate,
				StatHour:    &statHour,
				Market:      &market,
				MetricName:  model.MetricFeeTotal,
				MetricValue: ts.FeeTotal.String(),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeHourly),
				StatDate:    statDate,
				StatHour:    &statHour,
				Market:      &market,
				MetricName:  model.MetricOrderCount,
				MetricValue: fmt.Sprintf("%d", ts.OrderCount),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeHourly),
				StatDate:    statDate,
				StatHour:    &statHour,
				Market:      &market,
				MetricName:  model.MetricCancelCount,
				MetricValue: fmt.Sprintf("%d", ts.CancelCount),
			},
		)
	}

	// 批量保存
	if len(stats) > 0 {
		if err := j.statsRepo.BatchUpsert(ctx, stats); err != nil {
			return 0, fmt.Errorf("failed to save hourly stats: %w", err)
		}
	}

	return len(stats), nil
}

// generateDailyStats 生成日统计
func (j *StatsAggJob) generateDailyStats(ctx context.Context, date time.Time) (int, error) {
	statDate := date.Format("2006-01-02")

	// 获取交易统计数据
	tradeStats, err := j.dataProvider.GetDailyTradeStats(ctx, statDate)
	if err != nil {
		return 0, fmt.Errorf("failed to get daily trade stats: %w", err)
	}

	stats := make([]*model.Statistics, 0)

	// 按市场生成统计
	for _, ts := range tradeStats {
		market := ts.Market

		stats = append(stats,
			&model.Statistics{
				StatType:    string(model.StatTypeDaily),
				StatDate:    statDate,
				Market:      &market,
				MetricName:  model.MetricTradeVolume,
				MetricValue: ts.TradeVolume.String(),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeDaily),
				StatDate:    statDate,
				Market:      &market,
				MetricName:  model.MetricTradeCount,
				MetricValue: fmt.Sprintf("%d", ts.TradeCount),
			},
			&model.Statistics{
				StatType:    string(model.StatTypeDaily),
				StatDate:    statDate,
				Market:      &market,
				MetricName:  model.MetricFeeTotal,
				MetricValue: ts.FeeTotal.String(),
			},
		)
	}

	// 获取活跃用户数
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.AddDate(0, 0, 1)
	activeUsers, err := j.dataProvider.GetActiveUserCount(ctx, startOfDay.UnixMilli(), endOfDay.UnixMilli())
	if err != nil {
		logger.Warn("failed to get active user count", "error", err)
	} else {
		stats = append(stats, &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    statDate,
			MetricName:  model.MetricActiveUsers,
			MetricValue: fmt.Sprintf("%d", activeUsers),
		})
	}

	// 获取新增用户数
	newUsers, err := j.dataProvider.GetNewUserCount(ctx, statDate)
	if err != nil {
		logger.Warn("failed to get new user count", "error", err)
	} else {
		stats = append(stats, &model.Statistics{
			StatType:    string(model.StatTypeDaily),
			StatDate:    statDate,
			MetricName:  model.MetricNewUsers,
			MetricValue: fmt.Sprintf("%d", newUsers),
		})
	}

	// 批量保存
	if len(stats) > 0 {
		if err := j.statsRepo.BatchUpsert(ctx, stats); err != nil {
			return 0, fmt.Errorf("failed to save daily stats: %w", err)
		}
	}

	return len(stats), nil
}

// MockStatsDataProvider 模拟数据提供者 (用于测试)
// 实际实现见 eidos-jobs/internal/client/trading_client.go StatsDataProviderImpl
// 及 eidos-jobs/internal/jobs/stats_agg.go DBStatsDataProvider
type MockStatsDataProvider struct{}

func (p *MockStatsDataProvider) GetHourlyTradeStats(ctx context.Context, startTime, endTime int64) ([]*TradeStats, error) {
	return nil, nil
}

func (p *MockStatsDataProvider) GetDailyTradeStats(ctx context.Context, date string) ([]*TradeStats, error) {
	return nil, nil
}

func (p *MockStatsDataProvider) GetActiveUserCount(ctx context.Context, startTime, endTime int64) (int64, error) {
	return 0, nil
}

func (p *MockStatsDataProvider) GetNewUserCount(ctx context.Context, date string) (int64, error) {
	return 0, nil
}

// DBStatsDataProvider 数据库统计数据提供者
type DBStatsDataProvider struct {
	db *gorm.DB
}

// NewDBStatsDataProvider 创建数据库统计数据提供者
func NewDBStatsDataProvider(db *gorm.DB) *DBStatsDataProvider {
	return &DBStatsDataProvider{db: db}
}

// GetHourlyTradeStats 获取小时交易统计
func (p *DBStatsDataProvider) GetHourlyTradeStats(ctx context.Context, startTime, endTime int64) ([]*TradeStats, error) {
	var results []*TradeStats

	query := `
		SELECT
			market,
			COALESCE(SUM(amount * price), 0) as trade_volume,
			COUNT(*) as trade_count,
			COALESCE(SUM(fee), 0) as fee_total,
			0 as order_count,
			0 as cancel_count
		FROM trades
		WHERE created_at >= ? AND created_at < ?
		GROUP BY market
	`

	rows, err := p.db.WithContext(ctx).Raw(query, startTime, endTime).Rows()
	if err != nil {
		return nil, fmt.Errorf("query hourly trade stats failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stats TradeStats
		var tradeVolume, feeTotal float64
		if err := rows.Scan(&stats.Market, &tradeVolume, &stats.TradeCount, &feeTotal, &stats.OrderCount, &stats.CancelCount); err != nil {
			return nil, fmt.Errorf("scan trade stats failed: %w", err)
		}
		stats.TradeVolume = decimal.NewFromFloat(tradeVolume)
		stats.FeeTotal = decimal.NewFromFloat(feeTotal)
		results = append(results, &stats)
	}

	return results, rows.Err()
}

// GetDailyTradeStats 获取日交易统计
func (p *DBStatsDataProvider) GetDailyTradeStats(ctx context.Context, date string) ([]*TradeStats, error) {
	// 解析日期获取时间范围
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	startTime := t.UnixMilli()
	endTime := t.Add(24 * time.Hour).UnixMilli()

	var results []*TradeStats

	// 交易量和手续费
	tradeQuery := `
		SELECT
			market,
			COALESCE(SUM(amount * price), 0) as trade_volume,
			COUNT(*) as trade_count,
			COALESCE(SUM(fee), 0) as fee_total
		FROM trades
		WHERE created_at >= ? AND created_at < ?
		GROUP BY market
	`

	tradeRows, err := p.db.WithContext(ctx).Raw(tradeQuery, startTime, endTime).Rows()
	if err != nil {
		return nil, fmt.Errorf("query daily trade stats failed: %w", err)
	}
	defer tradeRows.Close()

	marketStats := make(map[string]*TradeStats)
	for tradeRows.Next() {
		var market string
		var tradeVolume, feeTotal float64
		var tradeCount int64
		if err := tradeRows.Scan(&market, &tradeVolume, &tradeCount, &feeTotal); err != nil {
			return nil, fmt.Errorf("scan trade stats failed: %w", err)
		}
		marketStats[market] = &TradeStats{
			Market:      market,
			TradeVolume: decimal.NewFromFloat(tradeVolume),
			TradeCount:  tradeCount,
			FeeTotal:    decimal.NewFromFloat(feeTotal),
		}
	}

	// 订单统计
	orderQuery := `
		SELECT
			market,
			COUNT(*) as order_count,
			SUM(CASE WHEN status = 'CANCELLED' THEN 1 ELSE 0 END) as cancel_count
		FROM orders
		WHERE created_at >= ? AND created_at < ?
		GROUP BY market
	`

	orderRows, err := p.db.WithContext(ctx).Raw(orderQuery, startTime, endTime).Rows()
	if err != nil {
		return nil, fmt.Errorf("query order stats failed: %w", err)
	}
	defer orderRows.Close()

	for orderRows.Next() {
		var market string
		var orderCount, cancelCount int64
		if err := orderRows.Scan(&market, &orderCount, &cancelCount); err != nil {
			return nil, fmt.Errorf("scan order stats failed: %w", err)
		}
		if stats, ok := marketStats[market]; ok {
			stats.OrderCount = orderCount
			stats.CancelCount = cancelCount
		} else {
			marketStats[market] = &TradeStats{
				Market:      market,
				OrderCount:  orderCount,
				CancelCount: cancelCount,
			}
		}
	}

	for _, stats := range marketStats {
		results = append(results, stats)
	}

	return results, nil
}

// GetActiveUserCount 获取活跃用户数 (有下单或成交的用户)
func (p *DBStatsDataProvider) GetActiveUserCount(ctx context.Context, startTime, endTime int64) (int64, error) {
	var count int64

	query := `
		SELECT COUNT(DISTINCT wallet) FROM (
			SELECT wallet FROM orders WHERE created_at >= ? AND created_at < ?
			UNION
			SELECT maker_wallet as wallet FROM trades WHERE created_at >= ? AND created_at < ?
			UNION
			SELECT taker_wallet as wallet FROM trades WHERE created_at >= ? AND created_at < ?
		) AS active_users
	`

	if err := p.db.WithContext(ctx).Raw(query, startTime, endTime, startTime, endTime, startTime, endTime).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("query active user count failed: %w", err)
	}

	return count, nil
}

// GetNewUserCount 获取新增用户数
func (p *DBStatsDataProvider) GetNewUserCount(ctx context.Context, date string) (int64, error) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, fmt.Errorf("invalid date format: %w", err)
	}

	startTime := t.UnixMilli()
	endTime := t.Add(24 * time.Hour).UnixMilli()

	var count int64

	// 查询当天首次有交易的用户数
	query := `
		SELECT COUNT(DISTINCT wallet) FROM (
			SELECT wallet, MIN(created_at) as first_order_time
			FROM orders
			GROUP BY wallet
			HAVING MIN(created_at) >= ? AND MIN(created_at) < ?
		) AS new_users
	`

	if err := p.db.WithContext(ctx).Raw(query, startTime, endTime).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("query new user count failed: %w", err)
	}

	return count, nil
}
