package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/model"
	"github.com/eidos-exchange/eidos/eidos-admin/internal/repository"
)

// AuditService 审计服务
type AuditService struct {
	auditRepo *repository.AuditLogRepository
}

// NewAuditService 创建审计服务
func NewAuditService(auditRepo *repository.AuditLogRepository) *AuditService {
	return &AuditService{
		auditRepo: auditRepo,
	}
}

// Create 创建审计日志
func (s *AuditService) Create(ctx context.Context, log *model.AuditLog) error {
	return s.auditRepo.Create(ctx, log)
}

// AuditLogQuery 审计日志查询条件
type AuditLogQuery struct {
	AdminID      *int64             `form:"admin_id"`
	Action       model.AuditAction  `form:"action"`
	ResourceType model.ResourceType `form:"resource_type"`
	ResourceID   string             `form:"resource_id"`
	Status       model.AuditStatus  `form:"status"`
	StartTime    *int64             `form:"start_time"`
	EndTime      *int64             `form:"end_time"`
}

// List 获取审计日志列表
func (s *AuditService) List(ctx context.Context, page *model.Pagination, query *AuditLogQuery) ([]*model.AuditLog, error) {
	filter := &repository.AuditLogFilter{}

	if query != nil {
		filter.AdminID = query.AdminID
		filter.Action = query.Action
		filter.ResourceType = query.ResourceType
		filter.ResourceID = query.ResourceID
		filter.Status = query.Status
		filter.StartTime = query.StartTime
		filter.EndTime = query.EndTime
	}

	return s.auditRepo.List(ctx, page, filter)
}

// GetByID 根据 ID 获取审计日志
func (s *AuditService) GetByID(ctx context.Context, id int64) (*model.AuditLog, error) {
	return s.auditRepo.GetByID(ctx, id)
}

// GetByAdmin 根据管理员获取审计日志
func (s *AuditService) GetByAdmin(ctx context.Context, adminID int64, limit int) ([]*model.AuditLog, error) {
	return s.auditRepo.ListByAdmin(ctx, adminID, limit)
}

// GetByResource 根据资源获取审计日志
func (s *AuditService) GetByResource(ctx context.Context, resourceType model.ResourceType, resourceID string, limit int) ([]*model.AuditLog, error) {
	return s.auditRepo.ListByResource(ctx, resourceType, resourceID, limit)
}

// GetByAdminID 根据管理员ID获取审计日志（分页）
func (s *AuditService) GetByAdminID(ctx context.Context, adminID int64, page *model.Pagination) ([]*model.AuditLog, error) {
	query := &AuditLogQuery{
		AdminID: &adminID,
	}
	return s.List(ctx, page, query)
}

// GetByResourcePaged 根据资源获取审计日志（分页）
func (s *AuditService) GetByResourcePaged(ctx context.Context, resourceType, resourceID string, page *model.Pagination) ([]*model.AuditLog, error) {
	query := &AuditLogQuery{
		ResourceType: model.ResourceType(resourceType),
		ResourceID:   resourceID,
	}
	return s.List(ctx, page, query)
}

// Export 导出审计日志
func (s *AuditService) Export(ctx context.Context, query *AuditLogQuery, format string) ([]byte, string, error) {
	// 获取所有符合条件的日志（不分页）
	page := &model.Pagination{
		Page:     1,
		PageSize: 10000, // 最大导出数量
	}

	logs, err := s.List(ctx, page, query)
	if err != nil {
		return nil, "", err
	}

	var data []byte
	var filename string

	if format == "json" {
		// JSON 格式导出
		data, err = json.Marshal(logs)
		if err != nil {
			return nil, "", err
		}
		filename = "audit_logs.json"
	} else {
		// CSV 格式导出
		var buf bytes.Buffer
		buf.WriteString("ID,管理员ID,操作,资源类型,资源ID,描述,状态,创建时间\n")
		for _, log := range logs {
			buf.WriteString(fmt.Sprintf("%d,%d,%s,%s,%s,%s,%s,%d\n",
				log.ID, log.AdminID, log.Action, log.ResourceType, log.ResourceID,
				log.Description, log.Status, log.CreatedAt))
		}
		data = buf.Bytes()
		filename = "audit_logs.csv"
	}

	return data, filename, nil
}
