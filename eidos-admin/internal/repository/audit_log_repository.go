package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
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
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByID 根据 ID 获取审计日志
func (r *AuditLogRepository) GetByID(ctx context.Context, id int64) (*model.AuditLog, error) {
	var log model.AuditLog
	err := r.db.WithContext(ctx).First(&log, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &log, err
}

// AuditLogFilter 审计日志过滤条件
type AuditLogFilter struct {
	AdminID      *int64
	Action       model.AuditAction
	ResourceType model.ResourceType
	ResourceID   string
	Status       model.AuditStatus
	StartTime    *int64
	EndTime      *int64
}

// List 获取审计日志列表
func (r *AuditLogRepository) List(ctx context.Context, page *model.Pagination, filter *AuditLogFilter) ([]*model.AuditLog, error) {
	var logs []*model.AuditLog

	query := r.db.WithContext(ctx).Model(&model.AuditLog{})

	if filter != nil {
		if filter.AdminID != nil {
			query = query.Where("admin_id = ?", *filter.AdminID)
		}
		if filter.Action != "" {
			query = query.Where("action = ?", filter.Action)
		}
		if filter.ResourceType != "" {
			query = query.Where("resource_type = ?", filter.ResourceType)
		}
		if filter.ResourceID != "" {
			query = query.Where("resource_id = ?", filter.ResourceID)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.StartTime != nil {
			query = query.Where("created_at >= ?", *filter.StartTime)
		}
		if filter.EndTime != nil {
			query = query.Where("created_at <= ?", *filter.EndTime)
		}
	}

	// 计算总数
	if err := query.Count(&page.Total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	err := query.Order("created_at DESC").
		Offset(page.GetOffset()).
		Limit(page.GetLimit()).
		Find(&logs).Error

	return logs, err
}

// ListByAdmin 根据管理员 ID 获取审计日志
func (r *AuditLogRepository) ListByAdmin(ctx context.Context, adminID int64, limit int) ([]*model.AuditLog, error) {
	var logs []*model.AuditLog
	err := r.db.WithContext(ctx).
		Where("admin_id = ?", adminID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// ListByResource 根据资源获取审计日志
func (r *AuditLogRepository) ListByResource(ctx context.Context, resourceType model.ResourceType, resourceID string, limit int) ([]*model.AuditLog, error) {
	var logs []*model.AuditLog
	err := r.db.WithContext(ctx).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// CountByAction 按操作类型统计
func (r *AuditLogRepository) CountByAction(ctx context.Context, action model.AuditAction, startTime, endTime int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AuditLog{}).
		Where("action = ? AND created_at >= ? AND created_at <= ?", action, startTime, endTime).
		Count(&count).Error
	return count, err
}

// DeleteOldLogs 删除旧日志 (清理用)
func (r *AuditLogRepository) DeleteOldLogs(ctx context.Context, beforeTime int64) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("created_at < ?", beforeTime).
		Delete(&model.AuditLog{})
	return result.RowsAffected, result.Error
}
