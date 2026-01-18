package service

import (
	"context"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
)

// EventService 风控事件服务
type EventService struct {
	repo *repository.RiskEventRepository
}

// NewEventService 创建事件服务
func NewEventService(repo *repository.RiskEventRepository) *EventService {
	return &EventService{repo: repo}
}

// ListEvents 获取风控事件列表
func (s *EventService) ListEvents(ctx context.Context, req *ListEventsRequest) ([]*model.RiskEvent, int64, error) {
	pagination := &repository.Pagination{
		Page:     req.Page,
		PageSize: req.Size,
	}

	// 根据筛选条件查询
	if req.Wallet != "" {
		return s.repo.ListByWallet(ctx, req.Wallet, pagination)
	}

	if req.EventType != "" {
		return s.repo.ListByType(ctx, req.EventType, pagination)
	}

	if req.StartTime > 0 && req.EndTime > 0 {
		return s.repo.ListByTimeRange(ctx, req.StartTime, req.EndTime, pagination)
	}

	// 默认返回待处理的事件
	return s.repo.ListPending(ctx, pagination)
}

// GetEvent 获取单个事件
func (s *EventService) GetEvent(ctx context.Context, eventID string) (*model.RiskEvent, error) {
	return s.repo.GetByEventID(ctx, eventID)
}

// ResolveEvent 解决事件
func (s *EventService) ResolveEvent(ctx context.Context, req *ResolveEventRequest) error {
	return s.repo.Resolve(ctx, req.EventID, req.ProcessedBy, req.ProcessingNote)
}

// IgnoreEvent 忽略事件
func (s *EventService) IgnoreEvent(ctx context.Context, req *IgnoreEventRequest) error {
	return s.repo.Ignore(ctx, req.EventID, req.ProcessedBy, req.ProcessingNote)
}

// GetEventStats 获取事件统计
func (s *EventService) GetEventStats(ctx context.Context, since int64) (*EventStats, error) {
	counts, err := s.repo.CountPendingByLevel(ctx)
	if err != nil {
		return nil, err
	}

	return &EventStats{
		PendingByLevel: counts,
	}, nil
}

// ListEventsRequest 事件列表请求
type ListEventsRequest struct {
	Wallet    string
	EventType string
	StartTime int64
	EndTime   int64
	Page      int
	Size      int
}

// ResolveEventRequest 解决事件请求
type ResolveEventRequest struct {
	EventID        string
	ProcessedBy    string
	ProcessingNote string
}

// IgnoreEventRequest 忽略事件请求
type IgnoreEventRequest struct {
	EventID        string
	ProcessedBy    string
	ProcessingNote string
}

// EventStats 事件统计
type EventStats struct {
	PendingByLevel map[string]int64
}
