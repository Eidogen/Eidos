package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
)

// tradeRepository 成交记录仓储实现
type tradeRepository struct {
	db *gorm.DB
}

// NewTradeRepository 创建成交记录仓储
func NewTradeRepository(db *gorm.DB) TradeRepository {
	return &tradeRepository{db: db}
}

// Create 创建成交记录
func (r *tradeRepository) Create(ctx context.Context, trade *model.Trade) error {
	return r.db.WithContext(ctx).Create(trade).Error
}

// BatchCreate 批量创建成交记录
func (r *tradeRepository) BatchCreate(ctx context.Context, trades []*model.Trade) error {
	if len(trades) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(trades, 100).Error
}

// ListRecent 获取最近的成交记录
func (r *tradeRepository) ListRecent(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	var trades []*model.Trade
	err := r.db.WithContext(ctx).
		Where("market = ?", market).
		Order("timestamp DESC").
		Limit(limit).
		Find(&trades).Error

	if err != nil {
		return nil, err
	}

	// 反转为时间升序
	for i, j := 0, len(trades)-1; i < j; i, j = i+1, j-1 {
		trades[i], trades[j] = trades[j], trades[i]
	}

	return trades, nil
}

// GetByID 根据 ID 获取成交记录
func (r *tradeRepository) GetByID(ctx context.Context, tradeID string) (*model.Trade, error) {
	var trade model.Trade
	err := r.db.WithContext(ctx).
		Where("trade_id = ?", tradeID).
		First(&trade).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &trade, nil
}

// ListByTimeRange 根据时间范围查询成交记录
func (r *tradeRepository) ListByTimeRange(ctx context.Context, market string,
	startTime, endTime int64, limit int) ([]*model.Trade, error) {

	if limit <= 0 {
		limit = 100
	}

	var trades []*model.Trade
	query := r.db.WithContext(ctx).Where("market = ?", market)

	if startTime > 0 {
		query = query.Where("timestamp >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("timestamp <= ?", endTime)
	}

	err := query.Order("timestamp DESC").
		Limit(limit).
		Find(&trades).Error

	if err != nil {
		return nil, err
	}

	return trades, nil
}
