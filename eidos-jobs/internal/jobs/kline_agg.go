package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
)

// Kline K线数据
type Kline struct {
	Market    string
	Interval  string
	OpenTime  int64
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    decimal.Decimal
	CloseTime int64
}

// KlineDataProvider K线数据提供者接口
type KlineDataProvider interface {
	// GetKlines 获取指定时间范围的K线数据
	GetKlines(ctx context.Context, market, interval string, startTime, endTime int64) ([]*Kline, error)
	// SaveKline 保存K线数据
	SaveKline(ctx context.Context, kline *Kline) error
	// GetMarkets 获取所有交易对
	GetMarkets(ctx context.Context) ([]string, error)
}

// KlineAggJob K线聚合任务
type KlineAggJob struct {
	scheduler.BaseJob
	dataProvider KlineDataProvider
}

// NewKlineAggJob 创建K线聚合任务
func NewKlineAggJob(dataProvider KlineDataProvider) *KlineAggJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameKlineAgg]

	return &KlineAggJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameKlineAgg,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		dataProvider: dataProvider,
	}
}

// Execute 执行K线聚合
func (j *KlineAggJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	now := time.Now()

	// 获取所有交易对
	markets, err := j.dataProvider.GetMarkets(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to get markets: %w", err)
	}

	if len(markets) == 0 {
		logger.Info("no markets found, skipping kline aggregation")
		return result, nil
	}

	// 1. 每小时生成 1h K线
	hourlyCount, err := j.generateKlines(ctx, markets, "1m", "1h", now)
	if err != nil {
		logger.Error("failed to generate 1h klines", "error", err)
		result.ErrorCount++
	}
	result.Details["1h_klines"] = hourlyCount
	result.ProcessedCount += hourlyCount

	// 2. 在 0/4/8/12/16/20 点额外生成 4h K线
	if now.Hour()%4 == 0 {
		fourHourCount, err := j.generateKlines(ctx, markets, "1h", "4h", now)
		if err != nil {
			logger.Error("failed to generate 4h klines", "error", err)
			result.ErrorCount++
		}
		result.Details["4h_klines"] = fourHourCount
		result.ProcessedCount += fourHourCount
	}

	// 3. 在 00:01 额外生成昨日 1d K线
	if now.Hour() == 0 && now.Minute() >= 1 && now.Minute() < 5 {
		dailyCount, err := j.generateDailyKlines(ctx, markets, now.AddDate(0, 0, -1))
		if err != nil {
			logger.Error("failed to generate 1d klines", "error", err)
			result.ErrorCount++
		}
		result.Details["1d_klines"] = dailyCount
		result.ProcessedCount += dailyCount
	}

	result.AffectedCount = result.ProcessedCount

	logger.Info("kline aggregation completed",
		"processed", result.ProcessedCount,
		"errors", result.ErrorCount)

	return result, nil
}

// generateKlines 生成K线 (从小周期聚合到大周期)
func (j *KlineAggJob) generateKlines(ctx context.Context, markets []string, sourceInterval, targetInterval string, now time.Time) (int, error) {
	count := 0

	// 计算目标时间范围
	var startTime, endTime time.Time
	switch targetInterval {
	case "1h":
		endTime = now.Truncate(time.Hour)
		startTime = endTime.Add(-time.Hour)
	case "4h":
		endTime = now.Truncate(4 * time.Hour)
		startTime = endTime.Add(-4 * time.Hour)
	default:
		return 0, fmt.Errorf("unsupported target interval: %s", targetInterval)
	}

	for _, market := range markets {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// 获取源K线数据
		klines, err := j.dataProvider.GetKlines(ctx, market, sourceInterval, startTime.UnixMilli(), endTime.UnixMilli())
		if err != nil {
			logger.Warn("failed to get source klines",
				"market", market,
				"interval", sourceInterval,
				"error", err)
			continue
		}

		if len(klines) == 0 {
			continue
		}

		// 聚合K线
		aggregated := j.aggregateKlines(klines, market, targetInterval, startTime.UnixMilli(), endTime.UnixMilli()-1)

		// 保存聚合后的K线
		if err := j.dataProvider.SaveKline(ctx, aggregated); err != nil {
			logger.Warn("failed to save aggregated kline",
				"market", market,
				"interval", targetInterval,
				"error", err)
			continue
		}

		count++
	}

	return count, nil
}

// generateDailyKlines 生成日K线
func (j *KlineAggJob) generateDailyKlines(ctx context.Context, markets []string, date time.Time) (int, error) {
	count := 0

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.AddDate(0, 0, 1)

	for _, market := range markets {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		// 获取1小时K线数据
		klines, err := j.dataProvider.GetKlines(ctx, market, "1h", startOfDay.UnixMilli(), endOfDay.UnixMilli())
		if err != nil {
			logger.Warn("failed to get hourly klines for daily aggregation",
				"market", market,
				"error", err)
			continue
		}

		if len(klines) == 0 {
			continue
		}

		// 聚合日K线
		aggregated := j.aggregateKlines(klines, market, "1d", startOfDay.UnixMilli(), endOfDay.UnixMilli()-1)

		// 保存日K线
		if err := j.dataProvider.SaveKline(ctx, aggregated); err != nil {
			logger.Warn("failed to save daily kline",
				"market", market,
				"error", err)
			continue
		}

		count++
	}

	return count, nil
}

// aggregateKlines 聚合K线数据
func (j *KlineAggJob) aggregateKlines(klines []*Kline, market, interval string, openTime, closeTime int64) *Kline {
	if len(klines) == 0 {
		return nil
	}

	result := &Kline{
		Market:    market,
		Interval:  interval,
		OpenTime:  openTime,
		CloseTime: closeTime,
		Open:      klines[0].Open,
		High:      klines[0].High,
		Low:       klines[0].Low,
		Close:     klines[len(klines)-1].Close,
		Volume:    decimal.Zero,
	}

	for _, k := range klines {
		if k.High.GreaterThan(result.High) {
			result.High = k.High
		}
		if k.Low.LessThan(result.Low) {
			result.Low = k.Low
		}
		result.Volume = result.Volume.Add(k.Volume)
	}

	return result
}

// MockKlineDataProvider 模拟K线数据提供者 (用于测试)
// 实际实现见 eidos-jobs/internal/client/market_client.go
type MockKlineDataProvider struct{}

func (p *MockKlineDataProvider) GetKlines(ctx context.Context, market, interval string, startTime, endTime int64) ([]*Kline, error) {
	return nil, nil
}

func (p *MockKlineDataProvider) SaveKline(ctx context.Context, kline *Kline) error {
	return nil
}

func (p *MockKlineDataProvider) GetMarkets(ctx context.Context) ([]string, error) {
	return nil, nil
}
