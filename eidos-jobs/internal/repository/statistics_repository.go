package repository

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
)

// StatisticsRepository 统计数据仓储
type StatisticsRepository struct {
	db *gorm.DB
}

// NewStatisticsRepository 创建统计数据仓储
func NewStatisticsRepository(db *gorm.DB) *StatisticsRepository {
	return &StatisticsRepository{db: db}
}

// Upsert 插入或更新统计数据 (幂等)
func (r *StatisticsRepository) Upsert(ctx context.Context, stat *model.Statistics) error {
	stat.CreatedAt = time.Now().UnixMilli()

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "stat_type"},
			{Name: "stat_date"},
			{Name: "stat_hour"},
			{Name: "market"},
			{Name: "metric_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"metric_value", "created_at"}),
	}).Create(stat).Error
}

// BatchUpsert 批量插入或更新统计数据
func (r *StatisticsRepository) BatchUpsert(ctx context.Context, stats []*model.Statistics) error {
	if len(stats) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	for _, s := range stats {
		s.CreatedAt = now
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "stat_type"},
			{Name: "stat_date"},
			{Name: "stat_hour"},
			{Name: "market"},
			{Name: "metric_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"metric_value", "created_at"}),
	}).Create(&stats).Error
}

// GetByDateAndType 根据日期和类型查询统计数据
func (r *StatisticsRepository) GetByDateAndType(ctx context.Context, statDate string, statType string) ([]*model.Statistics, error) {
	var stats []*model.Statistics
	err := r.db.WithContext(ctx).
		Where("stat_date = ? AND stat_type = ?", statDate, statType).
		Find(&stats).Error
	return stats, err
}

// GetByDateRangeAndMarket 根据日期范围和市场查询统计数据
func (r *StatisticsRepository) GetByDateRangeAndMarket(ctx context.Context, startDate, endDate, market string) ([]*model.Statistics, error) {
	var stats []*model.Statistics
	query := r.db.WithContext(ctx).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate)

	if market != "" {
		query = query.Where("market = ?", market)
	}

	err := query.Order("stat_date ASC, stat_hour ASC").Find(&stats).Error
	return stats, err
}

// GetMetricByDateRange 获取特定指标的日期范围数据
func (r *StatisticsRepository) GetMetricByDateRange(ctx context.Context, metricName, startDate, endDate string, market *string) ([]*model.Statistics, error) {
	var stats []*model.Statistics
	query := r.db.WithContext(ctx).
		Where("metric_name = ?", metricName).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate)

	if market != nil {
		query = query.Where("market = ?", *market)
	}

	err := query.Order("stat_date ASC, stat_hour ASC").Find(&stats).Error
	return stats, err
}

// GetLatestDaily 获取最新的日统计
func (r *StatisticsRepository) GetLatestDaily(ctx context.Context, market *string) ([]*model.Statistics, error) {
	var stats []*model.Statistics

	// 先获取最新日期 (使用 sql.NullString 处理无数据情况)
	var latestDate sql.NullString
	subQuery := r.db.WithContext(ctx).
		Model(&model.Statistics{}).
		Select("MAX(stat_date)").
		Where("stat_type = ?", model.StatTypeDaily)

	if market != nil {
		subQuery = subQuery.Where("market = ?", *market)
	}

	if err := subQuery.Scan(&latestDate).Error; err != nil {
		return nil, err
	}

	if !latestDate.Valid || latestDate.String == "" {
		return nil, nil
	}

	// 查询该日期的所有统计
	query := r.db.WithContext(ctx).
		Where("stat_type = ? AND stat_date = ?", model.StatTypeDaily, latestDate.String)

	if market != nil {
		query = query.Where("market = ?", *market)
	}

	err := query.Find(&stats).Error
	return stats, err
}

// SumMetricByDateRange 汇总日期范围内的指标值
func (r *StatisticsRepository) SumMetricByDateRange(ctx context.Context, metricName, startDate, endDate string, market *string) (string, error) {
	var sum string
	query := r.db.WithContext(ctx).
		Model(&model.Statistics{}).
		Select("COALESCE(SUM(metric_value::numeric), 0)::text").
		Where("metric_name = ?", metricName).
		Where("stat_type = ?", model.StatTypeDaily).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate)

	if market != nil {
		query = query.Where("market = ?", *market)
	}

	err := query.Scan(&sum).Error
	return sum, err
}

// Delete 删除统计数据
func (r *StatisticsRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Statistics{}, id).Error
}

// DeleteByDateRange 删除日期范围内的统计数据
func (r *StatisticsRepository) DeleteByDateRange(ctx context.Context, beforeDate string) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("stat_date < ?", beforeDate).
		Delete(&model.Statistics{})
	return result.RowsAffected, result.Error
}
