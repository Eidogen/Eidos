package handler

import (
	"context"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	jobsv1 "github.com/eidos-exchange/eidos/proto/jobs/v1"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/model"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
)

// JobsHandler gRPC 服务处理器
type JobsHandler struct {
	jobsv1.UnimplementedJobsServiceServer
	scheduler  *scheduler.Scheduler
	execRepo   *repository.ExecutionRepository
	statsRepo  *repository.StatisticsRepository
	reconRepo  *repository.ReconciliationRepository
}

// NewJobsHandler 创建 gRPC 服务处理器
func NewJobsHandler(
	scheduler *scheduler.Scheduler,
	execRepo *repository.ExecutionRepository,
	statsRepo *repository.StatisticsRepository,
	reconRepo *repository.ReconciliationRepository,
) *JobsHandler {
	return &JobsHandler{
		scheduler: scheduler,
		execRepo:  execRepo,
		statsRepo: statsRepo,
		reconRepo: reconRepo,
	}
}

// GetJobStatus 获取任务状态
func (h *JobsHandler) GetJobStatus(ctx context.Context, req *jobsv1.GetJobStatusRequest) (*jobsv1.GetJobStatusResponse, error) {
	if req.JobName == "" {
		return nil, status.Error(codes.InvalidArgument, "job_name is required")
	}

	jobStatus, err := h.scheduler.GetJobStatus(req.JobName)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %v", err)
	}

	return &jobsv1.GetJobStatusResponse{
		Status: convertJobStatus(jobStatus),
	}, nil
}

// ListJobStatus 列出所有任务状态
func (h *JobsHandler) ListJobStatus(ctx context.Context, req *jobsv1.ListJobStatusRequest) (*jobsv1.ListJobStatusResponse, error) {
	statuses, err := h.scheduler.ListJobStatus()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list job statuses: %v", err)
	}

	result := make([]*jobsv1.JobStatus, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, convertJobStatus(s))
	}

	return &jobsv1.ListJobStatusResponse{
		Statuses: result,
	}, nil
}

// TriggerJob 手动触发任务
func (h *JobsHandler) TriggerJob(ctx context.Context, req *jobsv1.TriggerJobRequest) (*jobsv1.TriggerJobResponse, error) {
	if req.JobName == "" {
		return nil, status.Error(codes.InvalidArgument, "job_name is required")
	}

	if err := h.scheduler.TriggerJob(req.JobName); err != nil {
		return &jobsv1.TriggerJobResponse{
			Accepted: false,
			Message:  err.Error(),
		}, nil
	}

	return &jobsv1.TriggerJobResponse{
		Accepted: true,
		Message:  "job triggered successfully",
	}, nil
}

// ListJobExecutions 列出任务执行历史
func (h *JobsHandler) ListJobExecutions(ctx context.Context, req *jobsv1.ListJobExecutionsRequest) (*jobsv1.ListJobExecutionsResponse, error) {
	limit := 100
	offset := 0

	if req.Pagination != nil {
		if req.Pagination.PageSize > 0 {
			limit = int(req.Pagination.PageSize)
		}
		if req.Pagination.Page > 0 {
			offset = (int(req.Pagination.Page) - 1) * limit
		}
	}

	var executions []*model.JobExecution
	var err error

	// 根据筛选条件查询
	if req.JobName != "" {
		executions, err = h.execRepo.ListByJobName(ctx, req.JobName, limit)
	} else if req.Status != "" {
		executions, err = h.execRepo.ListByStatus(ctx, model.JobStatus(req.Status), limit)
	} else if req.StartTime > 0 && req.EndTime > 0 {
		executions, err = h.execRepo.ListByTimeRange(ctx, req.StartTime, req.EndTime, limit)
	} else {
		executions, err = h.execRepo.ListByTimeRange(ctx, 0, 9999999999999, limit)
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list job executions: %v", err)
	}

	// 分页处理
	total := len(executions)
	if offset < len(executions) {
		end := offset + limit
		if end > len(executions) {
			end = len(executions)
		}
		executions = executions[offset:end]
	} else {
		executions = nil
	}

	result := make([]*jobsv1.JobExecution, 0, len(executions))
	for _, exec := range executions {
		result = append(result, convertJobExecution(exec))
	}

	return &jobsv1.ListJobExecutionsResponse{
		Executions: result,
		Pagination: &commonv1.PaginationResponse{
			Total:    int64(total),
			Page:     req.GetPagination().GetPage(),
			PageSize: int32(limit),
		},
	}, nil
}

// GetReconciliationReport 获取对账报告
func (h *JobsHandler) GetReconciliationReport(ctx context.Context, req *jobsv1.GetReconciliationReportRequest) (*jobsv1.GetReconciliationReportResponse, error) {
	if req.JobType == "" {
		return nil, status.Error(codes.InvalidArgument, "job_type is required")
	}

	// 获取该类型的对账记录
	records, err := h.reconRepo.ListByTimeRange(ctx, req.StartTime, req.EndTime, 10000)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get reconciliation records: %v", err)
	}

	// 筛选并统计
	var totalChecked, matchedCount, mismatchedCount int64
	tokenStats := make(map[string]*tokenStat)

	for _, rec := range records {
		if rec.JobType != req.JobType {
			continue
		}

		totalChecked++
		token := ""
		if rec.Token != nil {
			token = *rec.Token
		}

		stat, ok := tokenStats[token]
		if !ok {
			stat = &tokenStat{token: token}
			tokenStats[token] = stat
		}
		stat.checkedCount++

		if rec.Status == string(model.ReconciliationStatusMatched) {
			matchedCount++
		} else {
			mismatchedCount++
			stat.mismatchedCount++
			if rec.DiffValue != nil {
				stat.totalDiff = addDecimals(stat.totalDiff, *rec.DiffValue)
			}
		}
	}

	// 汇总差异
	totalDiff, err := h.reconRepo.SumDiffByJobType(ctx, req.JobType)
	if err != nil {
		totalDiff = "0"
	}

	// 构建返回结果
	byToken := make([]*jobsv1.ReconciliationSummary, 0, len(tokenStats))
	for _, stat := range tokenStats {
		byToken = append(byToken, &jobsv1.ReconciliationSummary{
			Token:           stat.token,
			CheckedCount:    stat.checkedCount,
			MismatchedCount: stat.mismatchedCount,
			TotalDiff:       stat.totalDiff,
		})
	}

	return &jobsv1.GetReconciliationReportResponse{
		JobType:         req.JobType,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		TotalChecked:    totalChecked,
		MatchedCount:    matchedCount,
		MismatchedCount: mismatchedCount,
		TotalDiff:       totalDiff,
		ByToken:         byToken,
	}, nil
}

// ListReconciliationRecords 列出对账记录
func (h *JobsHandler) ListReconciliationRecords(ctx context.Context, req *jobsv1.ListReconciliationRecordsRequest) (*jobsv1.ListReconciliationRecordsResponse, error) {
	limit := 100
	if req.Pagination != nil && req.Pagination.PageSize > 0 {
		limit = int(req.Pagination.PageSize)
	}

	var records []*model.ReconciliationRecord
	var err error

	// 根据筛选条件查询
	if req.WalletAddress != "" {
		records, err = h.reconRepo.ListByWallet(ctx, req.WalletAddress, limit)
	} else if req.Status != "" {
		if req.Status == "mismatched" {
			records, err = h.reconRepo.ListMismatched(ctx, limit)
		} else {
			records, err = h.reconRepo.ListByJobType(ctx, req.JobType, limit)
		}
	} else if req.JobType != "" {
		records, err = h.reconRepo.ListByJobType(ctx, req.JobType, limit)
	} else if req.StartTime > 0 && req.EndTime > 0 {
		records, err = h.reconRepo.ListByTimeRange(ctx, req.StartTime, req.EndTime, limit)
	} else {
		records, err = h.reconRepo.ListByTimeRange(ctx, 0, 9999999999999, limit)
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reconciliation records: %v", err)
	}

	result := make([]*jobsv1.ReconciliationRecord, 0, len(records))
	for _, rec := range records {
		result = append(result, convertReconciliationRecord(rec))
	}

	return &jobsv1.ListReconciliationRecordsResponse{
		Records: result,
		Pagination: &commonv1.PaginationResponse{
			Total:    int64(len(records)),
			Page:     req.GetPagination().GetPage(),
			PageSize: int32(limit),
		},
	}, nil
}

// GetStatistics 获取统计数据
func (h *JobsHandler) GetStatistics(ctx context.Context, req *jobsv1.GetStatisticsRequest) (*jobsv1.GetStatisticsResponse, error) {
	if req.StatType == "" || req.StartDate == "" || req.EndDate == "" {
		return nil, status.Error(codes.InvalidArgument, "stat_type, start_date, and end_date are required")
	}

	var market *string
	if req.Market != "" {
		market = &req.Market
	}

	stats, err := h.statsRepo.GetByDateRangeAndMarket(ctx, req.StartDate, req.EndDate, req.Market)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get statistics: %v", err)
	}

	// 根据 stat_type 过滤
	filtered := make([]*model.Statistics, 0)
	for _, s := range stats {
		if s.StatType != req.StatType {
			continue
		}

		// 根据 metric_names 过滤
		if len(req.MetricNames) > 0 {
			found := false
			for _, name := range req.MetricNames {
				if s.MetricName == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// 根据 market 过滤
		if market != nil && (s.Market == nil || *s.Market != *market) {
			continue
		}

		filtered = append(filtered, s)
	}

	result := make([]*jobsv1.StatisticsData, 0, len(filtered))
	for _, s := range filtered {
		data := &jobsv1.StatisticsData{
			StatType:    s.StatType,
			StatDate:    s.StatDate,
			MetricName:  s.MetricName,
			MetricValue: s.MetricValue,
		}
		if s.StatHour != nil {
			data.StatHour = int32(*s.StatHour)
		}
		if s.Market != nil {
			data.Market = *s.Market
		}
		result = append(result, data)
	}

	return &jobsv1.GetStatisticsResponse{
		Data: result,
	}, nil
}

// Helper functions

type tokenStat struct {
	token           string
	checkedCount    int64
	mismatchedCount int64
	totalDiff       string
}

func convertJobStatus(s *scheduler.JobStatus) *jobsv1.JobStatus {
	if s == nil {
		return nil
	}
	return &jobsv1.JobStatus{
		JobName:        s.Name,
		Enabled:        s.Enabled,
		Cron:           s.Cron,
		TimeoutSeconds: int32(s.Timeout.Seconds()),
		IsLocked:       s.IsLocked,
		LastStatus:     s.LastStatus,
		LastStartedAt:  s.LastStartedAt,
		LastFinishedAt: s.LastFinishedAt,
		LastDurationMs: int32(s.LastDurationMs),
		LastError:      s.LastError,
	}
}

func convertJobExecution(exec *model.JobExecution) *jobsv1.JobExecution {
	if exec == nil {
		return nil
	}

	result := &jobsv1.JobExecution{
		Id:        exec.ID,
		JobName:   exec.JobName,
		Status:    string(exec.Status),
		StartedAt: exec.StartedAt,
	}

	if exec.FinishedAt != nil {
		result.FinishedAt = *exec.FinishedAt
	}
	if exec.DurationMs != nil {
		result.DurationMs = int32(*exec.DurationMs)
	}
	if exec.ErrorMessage != nil {
		result.ErrorMessage = *exec.ErrorMessage
	}
	if exec.Result != nil {
		result.Result = make(map[string]string)
		for k, v := range exec.Result {
			if s, ok := v.(string); ok {
				result.Result[k] = s
			}
		}
	}

	return result
}

func convertReconciliationRecord(rec *model.ReconciliationRecord) *jobsv1.ReconciliationRecord {
	if rec == nil {
		return nil
	}

	result := &jobsv1.ReconciliationRecord{
		Id:            rec.ID,
		JobType:       rec.JobType,
		ReconcileType: rec.ReconcileType,
		Status:        rec.Status,
		CreatedAt:     rec.CreatedAt,
	}

	if rec.WalletAddress != nil {
		result.WalletAddress = *rec.WalletAddress
	}
	if rec.Token != nil {
		result.Token = *rec.Token
	}
	if rec.OffchainValue != nil {
		result.OffchainValue = *rec.OffchainValue
	}
	if rec.OnchainValue != nil {
		result.OnchainValue = *rec.OnchainValue
	}
	if rec.DiffValue != nil {
		result.DiffValue = *rec.DiffValue
	}

	return result
}

// addDecimals 十进制精确加法 (用于汇总差异)
func addDecimals(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aVal, err := decimal.NewFromString(a)
	if err != nil {
		return b
	}
	bVal, err := decimal.NewFromString(b)
	if err != nil {
		return a
	}
	return aVal.Add(bVal).String()
}
