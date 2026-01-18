package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"go.uber.org/zap"
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
		logger.Error("failed to generate hourly stats", zap.Error(err))
		result.ErrorCount++
	}
	result.ProcessedCount += hourlyCount
	result.Details["hourly_stats"] = hourlyCount

	// 2. 如果是每天 00:05，额外生成昨日日统计
	if now.Hour() == 0 && now.Minute() >= 5 && now.Minute() < 10 {
		dailyCount, err := j.generateDailyStats(ctx, now.AddDate(0, 0, -1))
		if err != nil {
			logger.Error("failed to generate daily stats", zap.Error(err))
			result.ErrorCount++
		}
		result.ProcessedCount += dailyCount
		result.Details["daily_stats"] = dailyCount
	}

	result.AffectedCount = result.ProcessedCount

	logger.Info("stats aggregation completed",
		zap.Int("processed", result.ProcessedCount),
		zap.Int("errors", result.ErrorCount))

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
		logger.Warn("failed to get active user count", zap.Error(err))
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
		logger.Warn("failed to get new user count", zap.Error(err))
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
type MockStatsDataProvider struct{}

func (p *MockStatsDataProvider) GetHourlyTradeStats(ctx context.Context, startTime, endTime int64) ([]*TradeStats, error) {
	// TODO: 实际实现通过 gRPC 调用 trading/market 服务
	return nil, nil
}

func (p *MockStatsDataProvider) GetDailyTradeStats(ctx context.Context, date string) ([]*TradeStats, error) {
	// TODO: 实际实现通过 gRPC 调用 trading/market 服务
	return nil, nil
}

func (p *MockStatsDataProvider) GetActiveUserCount(ctx context.Context, startTime, endTime int64) (int64, error) {
	// TODO: 实际实现通过查询数据库
	return 0, nil
}

func (p *MockStatsDataProvider) GetNewUserCount(ctx context.Context, date string) (int64, error) {
	// TODO: 实际实现通过查询数据库
	return 0, nil
}
