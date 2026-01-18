package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
)

// StatsRepository 统计数据仓储
type StatsRepository struct {
	db *gorm.DB
}

// NewStatsRepository 创建统计数据仓储
func NewStatsRepository(db *gorm.DB) *StatsRepository {
	return &StatsRepository{db: db}
}

// Upsert 插入或更新日统计
func (r *StatsRepository) Upsert(ctx context.Context, stats *model.DailyStats) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "stat_date"},
			{Name: "market"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"trade_count", "trade_volume", "trade_amount",
			"order_count", "cancelled_count",
			"maker_fee_total", "taker_fee_total",
			"active_users", "new_users",
			"updated_at",
		}),
	}).Create(stats).Error
}

// GetByDate 根据日期获取统计
func (r *StatsRepository) GetByDate(ctx context.Context, date string) ([]*model.DailyStats, error) {
	var stats []*model.DailyStats
	err := r.db.WithContext(ctx).
		Where("stat_date = ?", date).
		Find(&stats).Error
	return stats, err
}

// GetByDateAndMarket 根据日期和市场获取统计
func (r *StatsRepository) GetByDateAndMarket(ctx context.Context, date string, market *string) (*model.DailyStats, error) {
	var stats model.DailyStats
	query := r.db.WithContext(ctx).Where("stat_date = ?", date)

	if market != nil {
		query = query.Where("market = ?", *market)
	} else {
		query = query.Where("market IS NULL")
	}

	err := query.First(&stats).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &stats, err
}

// GetByDateRange 根据日期范围获取统计
func (r *StatsRepository) GetByDateRange(ctx context.Context, startDate, endDate string, market *string) ([]*model.DailyStats, error) {
	var stats []*model.DailyStats
	query := r.db.WithContext(ctx).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate)

	if market != nil {
		query = query.Where("market = ?", *market)
	}

	err := query.Order("stat_date ASC").Find(&stats).Error
	return stats, err
}

// GetLatest 获取最新的统计
func (r *StatsRepository) GetLatest(ctx context.Context, limit int) ([]*model.DailyStats, error) {
	var stats []*model.DailyStats
	err := r.db.WithContext(ctx).
		Where("market IS NULL"). // 只获取汇总数据
		Order("stat_date DESC").
		Limit(limit).
		Find(&stats).Error
	return stats, err
}

// SumByDateRange 汇总日期范围内的数据
func (r *StatsRepository) SumByDateRange(ctx context.Context, startDate, endDate string) (*model.DailyStats, error) {
	var result model.DailyStats
	err := r.db.WithContext(ctx).Model(&model.DailyStats{}).
		Select(`
			SUM(trade_count) as trade_count,
			SUM(trade_volume::numeric)::text as trade_volume,
			SUM(trade_amount::numeric)::text as trade_amount,
			SUM(order_count) as order_count,
			SUM(cancelled_count) as cancelled_count,
			SUM(maker_fee_total::numeric)::text as maker_fee_total,
			SUM(taker_fee_total::numeric)::text as taker_fee_total,
			SUM(active_users) as active_users,
			SUM(new_users) as new_users
		`).
		Where("stat_date >= ? AND stat_date <= ?", startDate, endDate).
		Where("market IS NULL"). // 只汇总全局数据
		Scan(&result).Error

	return &result, err
}

// GetMarketStats 获取市场级别统计
func (r *StatsRepository) GetMarketStats(ctx context.Context, date string) ([]*model.DailyStats, error) {
	var stats []*model.DailyStats
	err := r.db.WithContext(ctx).
		Where("stat_date = ? AND market IS NOT NULL", date).
		Order("trade_volume DESC").
		Find(&stats).Error
	return stats, err
}

// DeleteByDateRange 删除日期范围内的统计
func (r *StatsRepository) DeleteByDateRange(ctx context.Context, beforeDate string) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("stat_date < ?", beforeDate).
		Delete(&model.DailyStats{})
	return result.RowsAffected, result.Error
}
