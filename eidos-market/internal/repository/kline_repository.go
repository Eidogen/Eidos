package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// klineRepository K 线仓储实现
type klineRepository struct {
	db *gorm.DB
}

// NewKlineRepository 创建 K 线仓储
func NewKlineRepository(db *gorm.DB) KlineRepository {
	return &klineRepository{db: db}
}

// Upsert 更新或插入 K 线
func (r *klineRepository) Upsert(ctx context.Context, kline *model.Kline) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "market"},
			{Name: "interval"},
			{Name: "open_time"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"open", "high", "low", "close",
			"volume", "quote_volume", "trade_count",
		}),
	}).Create(kline).Error
}

// BatchUpsert 批量更新或插入 K 线
func (r *klineRepository) BatchUpsert(ctx context.Context, klines []*model.Kline) error {
	if len(klines) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "market"},
			{Name: "interval"},
			{Name: "open_time"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"open", "high", "low", "close",
			"volume", "quote_volume", "trade_count",
		}),
	}).CreateInBatches(klines, 100).Error
}

// Query 查询 K 线
func (r *klineRepository) Query(ctx context.Context, market string, interval model.KlineInterval,
	startTime, endTime int64, limit int) ([]*model.Kline, error) {

	var klines []*model.Kline
	query := r.db.WithContext(ctx).
		Where("market = ?", market).
		Where("interval = ?", interval)

	if startTime > 0 {
		query = query.Where("open_time >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("open_time <= ?", endTime)
	}

	if limit <= 0 {
		limit = 500
	}
	if limit > 1500 {
		limit = 1500
	}

	err := query.Order("open_time DESC").
		Limit(limit).
		Find(&klines).Error

	if err != nil {
		return nil, err
	}

	// 反转为时间升序
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}

	return klines, nil
}

// GetLatest 获取最新的 K 线
func (r *klineRepository) GetLatest(ctx context.Context, market string, interval model.KlineInterval) (*model.Kline, error) {
	var kline model.Kline
	err := r.db.WithContext(ctx).
		Where("market = ?", market).
		Where("interval = ?", interval).
		Order("open_time DESC").
		First(&kline).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &kline, nil
}

// CleanOldKlines 清理旧的 K 线数据（可选的维护方法）
func (r *klineRepository) CleanOldKlines(ctx context.Context, interval model.KlineInterval, beforeTime time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("interval = ?", interval).
		Where("open_time < ?", beforeTime.UnixMilli()).
		Delete(&model.Kline{})

	return result.RowsAffected, result.Error
}
