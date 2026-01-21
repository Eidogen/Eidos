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

// AuditLogFilter 审计日志查询过滤器
type AuditLogFilter struct {
	Wallet    string
	Action    model.AuditAction
	Result    model.AuditResult
	StartTime int64
	EndTime   int64
}

// Query 查询审计日志
func (r *AuditLogRepository) Query(ctx context.Context, filter *AuditLogFilter, pagination *Pagination) ([]*model.AuditLog, int64, error) {
	var logs []*model.AuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&model.AuditLog{})

	// 应用过滤条件
	if filter != nil {
		if filter.Wallet != "" {
			query = query.Where("wallet_address = ?", filter.Wallet)
		}
		if filter.Action != "" {
			query = query.Where("action = ?", filter.Action)
		}
		if filter.Result != "" {
			query = query.Where("result = ?", filter.Result)
		}
		if filter.StartTime > 0 {
			query = query.Where("created_at >= ?", filter.StartTime)
		}
		if filter.EndTime > 0 {
			query = query.Where("created_at <= ?", filter.EndTime)
		}
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
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

// AuditStats 审计统计
type AuditStats struct {
	TotalChecks        int64
	OrderChecks        int64
	WithdrawChecks     int64
	Rejections         int64
	RejectionRate      float64
	AvgDurationMs      int
	BlacklistAdditions int64
	BlacklistRemovals  int64
	RuleUpdates        int64
}

// GetStats 获取审计统计
func (r *AuditLogRepository) GetStats(ctx context.Context, startTime, endTime int64) (*AuditStats, error) {
	stats := &AuditStats{}

	baseQuery := r.db.WithContext(ctx).Model(&model.AuditLog{})
	if startTime > 0 {
		baseQuery = baseQuery.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		baseQuery = baseQuery.Where("created_at <= ?", endTime)
	}

	// 总检查数
	baseQuery.Count(&stats.TotalChecks)

	// 订单检查数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ?", model.AuditActionCheckOrder).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.OrderChecks)

	// 提现检查数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ?", model.AuditActionCheckWithdraw).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.WithdrawChecks)

	// 拒绝数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("result = ?", model.AuditResultRejected).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.Rejections)

	// 黑名单添加数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ?", model.AuditActionAddBlacklist).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.BlacklistAdditions)

	// 黑名单移除数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ?", model.AuditActionRemoveBlack).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.BlacklistRemovals)

	// 规则更新数
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ?", model.AuditActionUpdateRule).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&stats.RuleUpdates)

	// 计算拒绝率
	if stats.TotalChecks > 0 {
		stats.RejectionRate = float64(stats.Rejections) / float64(stats.TotalChecks) * 100
	}

	// 平均延迟
	var avgDuration struct {
		AvgDuration float64
	}
	r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Select("AVG(duration_ms) as avg_duration").
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Scan(&avgDuration)
	stats.AvgDurationMs = int(avgDuration.AvgDuration)

	return stats, nil
}

// CleanupOldLogs 清理旧日志
func (r *AuditLogRepository) CleanupOldLogs(ctx context.Context, olderThan int64) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("created_at < ?", olderThan).
		Delete(&model.AuditLog{})

	return result.RowsAffected, result.Error
}
