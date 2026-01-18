package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
)

// AuditLogRepository 审计日志仓储
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计日志仓储
func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// Create 创建审计日志
func (r *AuditLogRepository) Create(ctx context.Context, log *model.AuditLog) error {
	if log.CreatedAt == 0 {
		log.CreatedAt = time.Now().UnixMilli()
	}

	result := r.db.WithContext(ctx).Create(log)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// BatchCreate 批量创建审计日志
func (r *AuditLogRepository) BatchCreate(ctx context.Context, logs []*model.AuditLog) error {
	if len(logs) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	for _, log := range logs {
		if log.CreatedAt == 0 {
			log.CreatedAt = now
		}
	}

	result := r.db.WithContext(ctx).CreateInBatches(logs, 100)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// ListByWallet 根据钱包地址查询审计日志
func (r *AuditLogRepository) ListByWallet(ctx context.Context, walletAddress string, pagination *Pagination) ([]*model.AuditLog, int64, error) {
	var logs []*model.AuditLog
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Where("wallet_address = ?", walletAddress)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&logs).Error

	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ListByAction 根据操作类型查询
func (r *AuditLogRepository) ListByAction(ctx context.Context, action string, pagination *Pagination) ([]*model.AuditLog, int64, error) {
	var logs []*model.AuditLog
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Where("action = ?", action)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&logs).Error

	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ListRejected 查询被拒绝的请求
func (r *AuditLogRepository) ListRejected(ctx context.Context, pagination *Pagination) ([]*model.AuditLog, int64, error) {
	var logs []*model.AuditLog
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Where("result = ?", model.AuditResultRejected)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&logs).Error

	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ListByTimeRange 根据时间范围查询
func (r *AuditLogRepository) ListByTimeRange(ctx context.Context, startTime, endTime int64, pagination *Pagination) ([]*model.AuditLog, int64, error) {
	var logs []*model.AuditLog
	var total int64

	query := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Where("created_at >= ? AND created_at < ?", startTime, endTime)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("created_at DESC").
		Find(&logs).Error

	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// CountByResult 按结果统计
func (r *AuditLogRepository) CountByResult(ctx context.Context, since int64) (map[string]int64, error) {
	type result struct {
		Result string
		Count  int64
	}
	var results []result

	err := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Select("result, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("result").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.Result] = r.Count
	}
	return counts, nil
}

// GetAverageLatency 获取平均延迟
func (r *AuditLogRepository) GetAverageLatency(ctx context.Context, action string, since int64) (float64, error) {
	var avg float64
	err := r.db.WithContext(ctx).
		Model(&model.AuditLog{}).
		Select("AVG(duration_ms)").
		Where("action = ?", action).
		Where("created_at >= ?", since).
		Scan(&avg).Error

	if err != nil {
		return 0, err
	}
	return avg, nil
}

// GetP99Latency 获取P99延迟
func (r *AuditLogRepository) GetP99Latency(ctx context.Context, action string, since int64) (int, error) {
	var p99 int
	err := r.db.WithContext(ctx).
		Raw(`
			SELECT PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_ms) as p99
			FROM eidos_risk_audit_logs
			WHERE action = ? AND created_at >= ?
		`, action, since).
		Scan(&p99).Error

	if err != nil {
		return 0, err
	}
	return p99, nil
}
