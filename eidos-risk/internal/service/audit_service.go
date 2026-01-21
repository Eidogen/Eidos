// Package service provides audit logging functionality
package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
)

// AuditService provides audit logging functionality
type AuditService struct {
	mu sync.Mutex

	repo *repository.AuditLogRepository

	// Async logging
	logChan   chan *model.AuditLog
	batchSize int
	ticker    *time.Ticker
	stopChan  chan struct{}
}

// NewAuditService creates a new audit service
func NewAuditService(repo *repository.AuditLogRepository) *AuditService {
	return &AuditService{
		repo:      repo,
		logChan:   make(chan *model.AuditLog, 10000),
		batchSize: 100,
		stopChan:  make(chan struct{}),
	}
}

// Start starts the async audit log processor
func (s *AuditService) Start(ctx context.Context) {
	s.ticker = time.NewTicker(5 * time.Second)
	go s.processLogs(ctx)
	logger.Info("audit service started")
}

// Stop stops the audit service
func (s *AuditService) Stop() {
	close(s.stopChan)
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

// LogOrderCheck logs an order check action
func (s *AuditService) LogOrderCheck(ctx context.Context, req *CheckOrderRequest, resp *CheckOrderResponse, durationMs int) {
	reqJSON, _ := json.Marshal(req)

	result := model.AuditResultAllowed
	reason := ""
	if !resp.Approved {
		result = model.AuditResultRejected
		reason = resp.RejectReason
	}

	log := &model.AuditLog{
		Action:        model.AuditActionCheckOrder,
		WalletAddress: req.Wallet,
		Request:       string(reqJSON),
		Result:        result,
		Reason:        reason,
		DurationMs:    durationMs,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// LogWithdrawCheck logs a withdrawal check action
func (s *AuditService) LogWithdrawCheck(ctx context.Context, req *CheckWithdrawRequest, resp *CheckWithdrawResponse, durationMs int) {
	reqJSON, _ := json.Marshal(req)

	result := model.AuditResultAllowed
	reason := ""
	if !resp.Approved {
		result = model.AuditResultRejected
		reason = resp.RejectReason
	} else if resp.RequireManualReview {
		result = model.AuditResultPending
		reason = resp.ReviewReason
	}

	log := &model.AuditLog{
		Action:        model.AuditActionCheckWithdraw,
		WalletAddress: req.Wallet,
		Request:       string(reqJSON),
		Result:        result,
		Reason:        reason,
		DurationMs:    durationMs,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// LogBlacklistAdd logs a blacklist addition
func (s *AuditService) LogBlacklistAdd(ctx context.Context, req *AddToBlacklistRequest) {
	reqJSON, _ := json.Marshal(req)

	log := &model.AuditLog{
		Action:        model.AuditActionAddBlacklist,
		WalletAddress: req.Wallet,
		Request:       string(reqJSON),
		Result:        model.AuditResultAllowed,
		Reason:        req.Reason,
		DurationMs:    0,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// LogBlacklistRemove logs a blacklist removal
func (s *AuditService) LogBlacklistRemove(ctx context.Context, req *RemoveFromBlacklistRequest) {
	reqJSON, _ := json.Marshal(req)

	log := &model.AuditLog{
		Action:        model.AuditActionRemoveBlack,
		WalletAddress: req.Wallet,
		Request:       string(reqJSON),
		Result:        model.AuditResultAllowed,
		Reason:        req.Reason,
		DurationMs:    0,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// LogRuleUpdate logs a rule update
func (s *AuditService) LogRuleUpdate(ctx context.Context, req *UpdateRuleRequest) {
	reqJSON, _ := json.Marshal(req)

	log := &model.AuditLog{
		Action:        model.AuditActionUpdateRule,
		WalletAddress: req.OperatorID,
		Request:       string(reqJSON),
		Result:        model.AuditResultAllowed,
		DurationMs:    0,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// LogEventResolve logs an event resolution
func (s *AuditService) LogEventResolve(ctx context.Context, eventID, operatorID, note string) {
	reqJSON, _ := json.Marshal(map[string]string{
		"event_id":    eventID,
		"operator_id": operatorID,
		"note":        note,
	})

	log := &model.AuditLog{
		Action:        model.AuditActionResolveEvent,
		WalletAddress: operatorID,
		Request:       string(reqJSON),
		Result:        model.AuditResultAllowed,
		DurationMs:    0,
		CreatedAt:     time.Now().UnixMilli(),
	}

	s.submitLog(log)
}

// submitLog submits a log to the async processor
func (s *AuditService) submitLog(log *model.AuditLog) {
	select {
	case s.logChan <- log:
	default:
		// Channel full, log synchronously
		logger.Warn("audit log channel full, logging synchronously")
		s.saveLogs([]*model.AuditLog{log})
	}
}

// processLogs processes logs asynchronously
func (s *AuditService) processLogs(ctx context.Context) {
	batch := make([]*model.AuditLog, 0, s.batchSize)

	for {
		select {
		case <-ctx.Done():
			// Flush remaining logs
			if len(batch) > 0 {
				s.saveLogs(batch)
			}
			return
		case <-s.stopChan:
			// Flush remaining logs
			if len(batch) > 0 {
				s.saveLogs(batch)
			}
			return
		case log := <-s.logChan:
			batch = append(batch, log)
			if len(batch) >= s.batchSize {
				s.saveLogs(batch)
				batch = make([]*model.AuditLog, 0, s.batchSize)
			}
		case <-s.ticker.C:
			if len(batch) > 0 {
				s.saveLogs(batch)
				batch = make([]*model.AuditLog, 0, s.batchSize)
			}
		}
	}
}

// saveLogs saves a batch of logs to the database
func (s *AuditService) saveLogs(logs []*model.AuditLog) {
	if len(logs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.repo.BatchCreate(ctx, logs); err != nil {
		logger.Error("failed to save audit logs",
			"count", len(logs),
			"error", err)
		return
	}

	logger.Debug("audit logs saved",
		"count", len(logs))
}

// Query queries audit logs
func (s *AuditService) Query(ctx context.Context, filter *AuditLogFilter, page, pageSize int) ([]*model.AuditLog, int64, error) {
	repoFilter := &repository.AuditLogFilter{
		Wallet:    filter.Wallet,
		Action:    filter.Action,
		Result:    filter.Result,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	}
	return s.repo.Query(ctx, repoFilter, &repository.Pagination{Page: page, PageSize: pageSize})
}

// AuditLogFilter represents audit log query filter
type AuditLogFilter struct {
	Wallet    string
	Action    model.AuditAction
	Result    model.AuditResult
	StartTime int64
	EndTime   int64
}

// GetStats returns audit statistics
func (s *AuditService) GetStats(ctx context.Context, startTime, endTime int64) (*AuditStats, error) {
	repoStats, err := s.repo.GetStats(ctx, startTime, endTime)
	if err != nil {
		return nil, err
	}
	return &AuditStats{
		TotalChecks:        repoStats.TotalChecks,
		OrderChecks:        repoStats.OrderChecks,
		WithdrawChecks:     repoStats.WithdrawChecks,
		Rejections:         repoStats.Rejections,
		RejectionRate:      repoStats.RejectionRate,
		AvgDurationMs:      repoStats.AvgDurationMs,
		BlacklistAdditions: repoStats.BlacklistAdditions,
		BlacklistRemovals:  repoStats.BlacklistRemovals,
		RuleUpdates:        repoStats.RuleUpdates,
	}, nil
}

// AuditStats represents audit statistics
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
