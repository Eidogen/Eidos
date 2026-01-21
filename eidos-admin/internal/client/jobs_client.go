package client

import (
	"context"

	"google.golang.org/grpc"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	jobsv1 "github.com/eidos-exchange/eidos/proto/jobs/v1"
)

// JobsClient wraps the jobs service gRPC client
type JobsClient struct {
	conn   *grpc.ClientConn
	client jobsv1.JobsServiceClient
}

// NewJobsClient creates a new jobs client
func NewJobsClient(ctx context.Context, target string) (*JobsClient, error) {
	conn, err := dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &JobsClient{
		conn:   conn,
		client: jobsv1.NewJobsServiceClient(conn),
	}, nil
}

// Close closes the connection
func (c *JobsClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetJobStatus retrieves status of a specific job
func (c *JobsClient) GetJobStatus(ctx context.Context, jobName string) (*jobsv1.JobStatus, error) {
	resp, err := c.client.GetJobStatus(ctx, &jobsv1.GetJobStatusRequest{
		JobName: jobName,
	})
	if err != nil {
		return nil, err
	}
	return resp.Status, nil
}

// ListJobStatus retrieves status of all jobs
func (c *JobsClient) ListJobStatus(ctx context.Context) ([]*jobsv1.JobStatus, error) {
	resp, err := c.client.ListJobStatus(ctx, &jobsv1.ListJobStatusRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Statuses, nil
}

// TriggerJob manually triggers a job
func (c *JobsClient) TriggerJob(ctx context.Context, jobName string) (*jobsv1.TriggerJobResponse, error) {
	return c.client.TriggerJob(ctx, &jobsv1.TriggerJobRequest{
		JobName: jobName,
	})
}

// ListJobExecutions retrieves job execution history
func (c *JobsClient) ListJobExecutions(ctx context.Context, jobName, status string, startTime, endTime int64, page, pageSize int32) (*jobsv1.ListJobExecutionsResponse, error) {
	return c.client.ListJobExecutions(ctx, &jobsv1.ListJobExecutionsRequest{
		JobName:   jobName,
		Status:    status,
		StartTime: startTime,
		EndTime:   endTime,
		Pagination: &commonv1.PaginationRequest{
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// GetReconciliationReport retrieves reconciliation report
func (c *JobsClient) GetReconciliationReport(ctx context.Context, jobType string, startTime, endTime int64) (*jobsv1.GetReconciliationReportResponse, error) {
	return c.client.GetReconciliationReport(ctx, &jobsv1.GetReconciliationReportRequest{
		JobType:   jobType,
		StartTime: startTime,
		EndTime:   endTime,
	})
}

// ListReconciliationRecords retrieves reconciliation records
func (c *JobsClient) ListReconciliationRecords(ctx context.Context, jobType, status, wallet string, startTime, endTime int64, page, pageSize int32) (*jobsv1.ListReconciliationRecordsResponse, error) {
	return c.client.ListReconciliationRecords(ctx, &jobsv1.ListReconciliationRecordsRequest{
		JobType:       jobType,
		Status:        status,
		WalletAddress: wallet,
		StartTime:     startTime,
		EndTime:       endTime,
		Pagination: &commonv1.PaginationRequest{
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// GetStatistics retrieves statistics data
func (c *JobsClient) GetStatistics(ctx context.Context, statType, startDate, endDate, market string, metricNames []string) (*jobsv1.GetStatisticsResponse, error) {
	return c.client.GetStatistics(ctx, &jobsv1.GetStatisticsRequest{
		StatType:    statType,
		StartDate:   startDate,
		EndDate:     endDate,
		Market:      market,
		MetricNames: metricNames,
	})
}
