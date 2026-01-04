package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
)

var (
	ErrTradeNotFound      = errors.New("trade not found")
	ErrTradeAlreadyExists = errors.New("trade already exists")
	ErrBatchNotFound      = errors.New("settlement batch not found")
)

// TradeRepository 成交仓储接口
type TradeRepository interface {
	// Create 创建成交记录
	Create(ctx context.Context, trade *model.Trade) error

	// BatchCreate 批量创建成交记录
	BatchCreate(ctx context.Context, trades []*model.Trade) error

	// GetByID 根据 ID 查询
	GetByID(ctx context.Context, id int64) (*model.Trade, error)

	// GetByTradeID 根据成交 ID 查询
	GetByTradeID(ctx context.Context, tradeID string) (*model.Trade, error)

	// ListByOrderID 查询订单相关成交
	ListByOrderID(ctx context.Context, orderID string) ([]*model.Trade, error)

	// ListByWallet 查询用户成交列表
	ListByWallet(ctx context.Context, wallet string, filter *TradeFilter, page *Pagination) ([]*model.Trade, error)

	// ListByMarket 查询市场成交列表
	ListByMarket(ctx context.Context, market string, filter *TradeFilter, page *Pagination) ([]*model.Trade, error)

	// ListBySettlementStatus 按结算状态查询
	ListBySettlementStatus(ctx context.Context, status model.SettlementStatus, limit int) ([]*model.Trade, error)

	// ListByBatchID 查询批次内成交
	ListByBatchID(ctx context.Context, batchID string) ([]*model.Trade, error)

	// UpdateSettlementStatus 更新结算状态
	UpdateSettlementStatus(ctx context.Context, tradeID string, oldStatus, newStatus model.SettlementStatus, batchID string) error

	// BatchUpdateSettlementStatus 批量更新结算状态
	BatchUpdateSettlementStatus(ctx context.Context, tradeIDs []string, newStatus model.SettlementStatus, txHash string) error

	// MarkSettled 标记为已结算
	MarkSettled(ctx context.Context, tradeID string, txHash string) error

	// CreateBatch 创建结算批次
	CreateBatch(ctx context.Context, batch *model.SettlementBatch) error

	// GetBatch 获取结算批次
	GetBatch(ctx context.Context, batchID string) (*model.SettlementBatch, error)

	// UpdateBatch 更新结算批次
	UpdateBatch(ctx context.Context, batch *model.SettlementBatch) error

	// ListPendingBatches 查询待处理批次
	ListPendingBatches(ctx context.Context, limit int) ([]*model.SettlementBatch, error)

	// GetRecentTrades 获取最近成交 (用于行情)
	GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error)

	// CountByMarket 统计市场成交数
	CountByMarket(ctx context.Context, market string, timeRange *TimeRange) (int64, error)
}

// TradeFilter 成交查询过滤条件
type TradeFilter struct {
	Market           string                  // 交易对
	SettlementStatus *model.SettlementStatus // 结算状态
	TimeRange        *TimeRange              // 时间范围
}

// tradeRepository 成交仓储实现
type tradeRepository struct {
	*Repository
}

// NewTradeRepository 创建成交仓储
func NewTradeRepository(db *gorm.DB) TradeRepository {
	return &tradeRepository{
		Repository: NewRepository(db),
	}
}

// Create 创建成交记录
func (r *tradeRepository) Create(ctx context.Context, trade *model.Trade) error {
	result := r.DB(ctx).Create(trade)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrTradeAlreadyExists
		}
		return fmt.Errorf("create trade failed: %w", result.Error)
	}
	return nil
}

// BatchCreate 批量创建成交记录
func (r *tradeRepository) BatchCreate(ctx context.Context, trades []*model.Trade) error {
	if len(trades) == 0 {
		return nil
	}

	result := r.DB(ctx).Clauses(clause.OnConflict{
		DoNothing: true,
	}).CreateInBatches(trades, 100)

	if result.Error != nil {
		return fmt.Errorf("batch create trades failed: %w", result.Error)
	}
	return nil
}

// GetByID 根据 ID 查询
func (r *tradeRepository) GetByID(ctx context.Context, id int64) (*model.Trade, error) {
	var trade model.Trade
	result := r.DB(ctx).Where("id = ?", id).First(&trade)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrTradeNotFound
		}
		return nil, fmt.Errorf("get trade by id failed: %w", result.Error)
	}
	return &trade, nil
}

// GetByTradeID 根据成交 ID 查询
func (r *tradeRepository) GetByTradeID(ctx context.Context, tradeID string) (*model.Trade, error) {
	var trade model.Trade
	result := r.DB(ctx).Where("trade_id = ?", tradeID).First(&trade)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrTradeNotFound
		}
		return nil, fmt.Errorf("get trade by trade_id failed: %w", result.Error)
	}
	return &trade, nil
}

// ListByOrderID 查询订单相关成交
func (r *tradeRepository) ListByOrderID(ctx context.Context, orderID string) ([]*model.Trade, error) {
	var trades []*model.Trade
	result := r.DB(ctx).
		Where("maker_order_id = ? OR taker_order_id = ?", orderID, orderID).
		Order("matched_at ASC").
		Find(&trades)

	if result.Error != nil {
		return nil, fmt.Errorf("list trades by order_id failed: %w", result.Error)
	}
	return trades, nil
}

// ListByWallet 查询用户成交列表
func (r *tradeRepository) ListByWallet(ctx context.Context, wallet string, filter *TradeFilter, page *Pagination) ([]*model.Trade, error) {
	db := r.DB(ctx).Where("maker_wallet = ? OR taker_wallet = ?", wallet, wallet)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Trade{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count trades failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var trades []*model.Trade
	db = db.Order("matched_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&trades).Error; err != nil {
		return nil, fmt.Errorf("list trades by wallet failed: %w", err)
	}
	return trades, nil
}

// ListByMarket 查询市场成交列表
func (r *tradeRepository) ListByMarket(ctx context.Context, market string, filter *TradeFilter, page *Pagination) ([]*model.Trade, error) {
	db := r.DB(ctx).Where("market = ?", market)
	db = r.applyFilter(db, filter)

	// 统计总数
	if page != nil {
		var total int64
		if err := db.Model(&model.Trade{}).Count(&total).Error; err != nil {
			return nil, fmt.Errorf("count trades failed: %w", err)
		}
		page.Total = total
	}

	// 查询列表
	var trades []*model.Trade
	db = db.Order("matched_at DESC")
	if page != nil {
		db = db.Offset(page.Offset()).Limit(page.Limit())
	}

	if err := db.Find(&trades).Error; err != nil {
		return nil, fmt.Errorf("list trades by market failed: %w", err)
	}
	return trades, nil
}

// ListBySettlementStatus 按结算状态查询
func (r *tradeRepository) ListBySettlementStatus(ctx context.Context, status model.SettlementStatus, limit int) ([]*model.Trade, error) {
	var trades []*model.Trade
	result := r.DB(ctx).
		Where("settlement_status = ?", status).
		Order("matched_at ASC").
		Limit(limit).
		Find(&trades)

	if result.Error != nil {
		return nil, fmt.Errorf("list trades by settlement status failed: %w", result.Error)
	}
	return trades, nil
}

// ListByBatchID 查询批次内成交
func (r *tradeRepository) ListByBatchID(ctx context.Context, batchID string) ([]*model.Trade, error) {
	var trades []*model.Trade
	result := r.DB(ctx).
		Where("batch_id = ?", batchID).
		Order("matched_at ASC").
		Find(&trades)

	if result.Error != nil {
		return nil, fmt.Errorf("list trades by batch_id failed: %w", result.Error)
	}
	return trades, nil
}

// UpdateSettlementStatus 更新结算状态
func (r *tradeRepository) UpdateSettlementStatus(ctx context.Context, tradeID string, oldStatus, newStatus model.SettlementStatus, batchID string) error {
	updates := map[string]interface{}{
		"settlement_status": newStatus,
		"updated_at":        time.Now().UnixMilli(),
	}
	if batchID != "" {
		updates["batch_id"] = batchID
	}

	result := r.DB(ctx).Model(&model.Trade{}).
		Where("trade_id = ? AND settlement_status = ?", tradeID, oldStatus).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("update settlement status failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOptimisticLock
	}
	return nil
}

// BatchUpdateSettlementStatus 批量更新结算状态
func (r *tradeRepository) BatchUpdateSettlementStatus(ctx context.Context, tradeIDs []string, newStatus model.SettlementStatus, txHash string) error {
	if len(tradeIDs) == 0 {
		return nil
	}

	updates := map[string]interface{}{
		"settlement_status": newStatus,
		"updated_at":        time.Now().UnixMilli(),
	}
	if txHash != "" {
		updates["tx_hash"] = txHash
	}
	if newStatus == model.SettlementStatusSettledOnchain {
		updates["settled_at"] = time.Now().UnixMilli()
	}

	result := r.DB(ctx).Model(&model.Trade{}).
		Where("trade_id IN ?", tradeIDs).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("batch update settlement status failed: %w", result.Error)
	}
	return nil
}

// MarkSettled 标记为已结算
func (r *tradeRepository) MarkSettled(ctx context.Context, tradeID string, txHash string) error {
	now := time.Now().UnixMilli()
	result := r.DB(ctx).Model(&model.Trade{}).
		Where("trade_id = ?", tradeID).
		Updates(map[string]interface{}{
			"settlement_status": model.SettlementStatusSettledOnchain,
			"tx_hash":           txHash,
			"settled_at":        now,
			"updated_at":        now,
		})

	if result.Error != nil {
		return fmt.Errorf("mark trade settled failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrTradeNotFound
	}
	return nil
}

// CreateBatch 创建结算批次
func (r *tradeRepository) CreateBatch(ctx context.Context, batch *model.SettlementBatch) error {
	result := r.DB(ctx).Create(batch)
	if result.Error != nil {
		return fmt.Errorf("create settlement batch failed: %w", result.Error)
	}
	return nil
}

// GetBatch 获取结算批次
func (r *tradeRepository) GetBatch(ctx context.Context, batchID string) (*model.SettlementBatch, error) {
	var batch model.SettlementBatch
	result := r.DB(ctx).Where("batch_id = ?", batchID).First(&batch)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrBatchNotFound
		}
		return nil, fmt.Errorf("get settlement batch failed: %w", result.Error)
	}
	return &batch, nil
}

// UpdateBatch 更新结算批次
func (r *tradeRepository) UpdateBatch(ctx context.Context, batch *model.SettlementBatch) error {
	batch.UpdatedAt = time.Now().UnixMilli()
	result := r.DB(ctx).Save(batch)
	if result.Error != nil {
		return fmt.Errorf("update settlement batch failed: %w", result.Error)
	}
	return nil
}

// ListPendingBatches 查询待处理批次
func (r *tradeRepository) ListPendingBatches(ctx context.Context, limit int) ([]*model.SettlementBatch, error) {
	var batches []*model.SettlementBatch
	result := r.DB(ctx).
		Where("status IN ?", []model.SettlementStatus{
			model.SettlementStatusPending,
			model.SettlementStatusSubmitted,
		}).
		Order("created_at ASC").
		Limit(limit).
		Find(&batches)

	if result.Error != nil {
		return nil, fmt.Errorf("list pending batches failed: %w", result.Error)
	}
	return batches, nil
}

// GetRecentTrades 获取最近成交
func (r *tradeRepository) GetRecentTrades(ctx context.Context, market string, limit int) ([]*model.Trade, error) {
	var trades []*model.Trade
	result := r.DB(ctx).
		Where("market = ?", market).
		Order("matched_at DESC").
		Limit(limit).
		Find(&trades)

	if result.Error != nil {
		return nil, fmt.Errorf("get recent trades failed: %w", result.Error)
	}
	return trades, nil
}

// CountByMarket 统计市场成交数
func (r *tradeRepository) CountByMarket(ctx context.Context, market string, timeRange *TimeRange) (int64, error) {
	db := r.DB(ctx).Model(&model.Trade{}).Where("market = ?", market)

	if timeRange != nil && timeRange.IsValid() {
		db = db.Where("matched_at >= ? AND matched_at <= ?", timeRange.Start, timeRange.End)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count trades by market failed: %w", err)
	}
	return count, nil
}

// applyFilter 应用过滤条件
func (r *tradeRepository) applyFilter(db *gorm.DB, filter *TradeFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	if filter.Market != "" {
		db = db.Where("market = ?", filter.Market)
	}
	if filter.SettlementStatus != nil {
		db = db.Where("settlement_status = ?", *filter.SettlementStatus)
	}
	if filter.TimeRange != nil && filter.TimeRange.IsValid() {
		db = db.Where("matched_at >= ? AND matched_at <= ?", filter.TimeRange.Start, filter.TimeRange.End)
	}

	return db
}
