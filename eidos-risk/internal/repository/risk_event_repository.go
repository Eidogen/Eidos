package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
)

var (
	ErrEventNotFound = errors.New("risk event not found")
)

// RiskEventRepository 风险事件仓储
type RiskEventRepository struct {
	db *gorm.DB
}

// NewRiskEventRepository 创建风险事件仓储
func NewRiskEventRepository(db *gorm.DB) *RiskEventRepository {
	return &RiskEventRepository{db: db}
}

// Create 创建风险事件
func (r *RiskEventRepository) Create(ctx context.Context, event *model.RiskEvent) error {
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}

	result := r.db.WithContext(ctx).Create(event)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// BatchCreate 批量创建风险事件
func (r *RiskEventRepository) BatchCreate(ctx context.Context, events []*model.RiskEvent) error {
	if len(events) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	for _, event := range events {
		if event.CreatedAt == 0 {
			event.CreatedAt = now
		}
	}

	result := r.db.WithContext(ctx).CreateInBatches(events, 100)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// GetByEventID 根据事件ID获取
func (r *RiskEventRepository) GetByEventID(ctx context.Context, eventID string) (*model.RiskEvent, error) {
	var event model.RiskEvent
	err := r.db.WithContext(ctx).
		Where("event_id = ?", eventID).
		First(&event).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return &event, nil
}

// ListByWallet 根据钱包地址查询事件
func (r *RiskEventRepository) ListByWallet(ctx context.Context, walletAddress string, pagination *Pagination) ([]*model.RiskEvent, int64, error) {
	var events []*model.RiskEvent
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("wallet = ?", walletAddress)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&events).Error

	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListByType 根据事件类型查询
func (r *RiskEventRepository) ListByType(ctx context.Context, eventType string, pagination *Pagination) ([]*model.RiskEvent, int64, error) {
	var events []*model.RiskEvent
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("type = ?", eventType)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&events).Error

	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListByLevel 根据风险级别查询
func (r *RiskEventRepository) ListByLevel(ctx context.Context, riskLevel string, pagination *Pagination) ([]*model.RiskEvent, int64, error) {
	var events []*model.RiskEvent
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("risk_level = ?", riskLevel)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&events).Error

	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListPending 查询待处理事件
func (r *RiskEventRepository) ListPending(ctx context.Context, pagination *Pagination) ([]*model.RiskEvent, int64, error) {
	var events []*model.RiskEvent
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("status = ?", model.RiskEventStatusPending)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("risk_level DESC, created_at ASC"). // 高风险优先，先创建的优先
		Find(&events).Error

	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListByTimeRange 根据时间范围查询
func (r *RiskEventRepository) ListByTimeRange(ctx context.Context, startTime, endTime int64, pagination *Pagination) ([]*model.RiskEvent, int64, error) {
	var events []*model.RiskEvent
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("created_at >= ? AND created_at < ?", startTime, endTime)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&events).Error

	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// Resolve 解决风险事件
func (r *RiskEventRepository) Resolve(ctx context.Context, eventID string, processedBy string, processingNote string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("event_id = ?", eventID).
		Where("status = ?", model.RiskEventStatusPending).
		Updates(map[string]interface{}{
			"status":          model.RiskEventStatusResolved,
			"processed_by":    processedBy,
			"processed_at":    now,
			"processing_note": processingNote,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrEventNotFound
	}
	return nil
}

// Ignore 忽略风险事件
func (r *RiskEventRepository) Ignore(ctx context.Context, eventID string, processedBy string, processingNote string) error {
	now := time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("event_id = ?", eventID).
		Where("status = ?", model.RiskEventStatusPending).
		Updates(map[string]interface{}{
			"status":          model.RiskEventStatusIgnored,
			"processed_by":    processedBy,
			"processed_at":    now,
			"processing_note": processingNote,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrEventNotFound
	}
	return nil
}

// CountByWalletAndType 统计指定钱包和类型的事件数量
func (r *RiskEventRepository) CountByWalletAndType(ctx context.Context, walletAddress string, eventType model.RiskEventType, since int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Where("wallet = ?", walletAddress).
		Where("type = ?", eventType).
		Where("created_at >= ?", since).
		Count(&count).Error

	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountPendingByLevel 统计各风险级别的待处理数量
func (r *RiskEventRepository) CountPendingByLevel(ctx context.Context) (map[string]int64, error) {
	type result struct {
		RiskLevel string
		Count     int64
	}
	var results []result

	err := r.db.WithContext(ctx).
		Model(&model.RiskEvent{}).
		Select("risk_level, COUNT(*) as count").
		Where("status = ?", model.RiskEventStatusPending).
		Group("risk_level").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.RiskLevel] = r.Count
	}
	return counts, nil
}
