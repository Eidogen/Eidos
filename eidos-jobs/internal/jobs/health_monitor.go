package jobs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-jobs/internal/scheduler"
	"gorm.io/gorm"
)

// CheckType 健康检查类型
type CheckType string

const (
	CheckTypeHTTP CheckType = "http"
	CheckTypeGRPC CheckType = "grpc"
)

// ServiceEndpoint 服务端点
type ServiceEndpoint struct {
	Name      string
	URL       string        // HTTP URL (用于 HTTP 检查)
	GRPCAddr  string        // gRPC 地址 (用于 gRPC 检查)
	CheckType CheckType     // 检查类型: http 或 grpc
	Timeout   time.Duration
}

// HealthMonitorJob 健康监控任务
type HealthMonitorJob struct {
	scheduler.BaseJob
	db          *gorm.DB
	redisClient redis.UniversalClient
	httpClient  *http.Client
	endpoints   []ServiceEndpoint
	alertFunc   func(ctx context.Context, alert *HealthAlert) error
}

// HealthAlert 健康告警
type HealthAlert struct {
	Service   string
	AlertType string // "service_down", "db_error", "redis_error", "high_latency"
	Severity  string // "warning", "critical"
	Message   string
	Details   map[string]interface{}
	Timestamp int64
}

// NewHealthMonitorJob 创建健康监控任务
func NewHealthMonitorJob(
	db *gorm.DB,
	redisClient redis.UniversalClient,
	endpoints []ServiceEndpoint,
) *HealthMonitorJob {
	cfg := scheduler.DefaultJobConfigs[scheduler.JobNameHealthMonitor]

	return &HealthMonitorJob{
		BaseJob: scheduler.NewBaseJob(
			scheduler.JobNameHealthMonitor,
			cfg.Timeout,
			cfg.LockTTL,
			cfg.UseWatchdog,
		),
		db:          db,
		redisClient: redisClient,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		endpoints: endpoints,
	}
}

// SetAlertFunc 设置告警回调
func (j *HealthMonitorJob) SetAlertFunc(f func(ctx context.Context, alert *HealthAlert) error) {
	j.alertFunc = f
}

// Execute 执行健康检查
func (j *HealthMonitorJob) Execute(ctx context.Context) (*scheduler.JobResult, error) {
	result := &scheduler.JobResult{
		Details: make(map[string]interface{}),
	}

	// 1. 检查数据库连接
	dbStatus := j.checkDatabase(ctx)
	result.Details["database"] = dbStatus
	if !dbStatus.Healthy {
		result.ErrorCount++
	}

	// 2. 检查 Redis 连接
	redisStatus := j.checkRedis(ctx)
	result.Details["redis"] = redisStatus
	if !redisStatus.Healthy {
		result.ErrorCount++
	}

	// 3. 检查各个服务端点
	serviceStatuses := make(map[string]interface{})
	for _, endpoint := range j.endpoints {
		status := j.checkEndpoint(ctx, endpoint)
		serviceStatuses[endpoint.Name] = status
		if !status.Healthy {
			result.ErrorCount++
		}
		result.ProcessedCount++
	}
	result.Details["services"] = serviceStatuses

	// 记录整体结果
	result.Details["total_checks"] = result.ProcessedCount + 2 // +2 for db and redis
	result.Details["failed_checks"] = result.ErrorCount
	result.Details["check_time"] = time.Now().Format(time.RFC3339)

	return result, nil
}

// checkDatabase 检查数据库
func (j *HealthMonitorJob) checkDatabase(ctx context.Context) *ComponentStatus {
	start := time.Now()
	status := &ComponentStatus{
		Component: "database",
		CheckedAt: time.Now().UnixMilli(),
	}

	sqlDB, err := j.db.DB()
	if err != nil {
		status.Error = err.Error()
		j.sendAlert(ctx, &HealthAlert{
			Service:   "database",
			AlertType: "db_error",
			Severity:  "critical",
			Message:   "Failed to get database connection: " + err.Error(),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		status.Error = err.Error()
		j.sendAlert(ctx, &HealthAlert{
			Service:   "database",
			AlertType: "db_error",
			Severity:  "critical",
			Message:   "Database ping failed: " + err.Error(),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	status.Healthy = true
	status.LatencyMs = int(time.Since(start).Milliseconds())

	// 检查高延迟
	if status.LatencyMs > 1000 {
		j.sendAlert(ctx, &HealthAlert{
			Service:   "database",
			AlertType: "high_latency",
			Severity:  "warning",
			Message:   fmt.Sprintf("Database latency is high: %dms", status.LatencyMs),
			Details:   map[string]interface{}{"latency_ms": status.LatencyMs},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	return status
}

// checkRedis 检查 Redis
func (j *HealthMonitorJob) checkRedis(ctx context.Context) *ComponentStatus {
	start := time.Now()
	status := &ComponentStatus{
		Component: "redis",
		CheckedAt: time.Now().UnixMilli(),
	}

	if err := j.redisClient.Ping(ctx).Err(); err != nil {
		status.Error = err.Error()
		j.sendAlert(ctx, &HealthAlert{
			Service:   "redis",
			AlertType: "redis_error",
			Severity:  "critical",
			Message:   "Redis ping failed: " + err.Error(),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	status.Healthy = true
	status.LatencyMs = int(time.Since(start).Milliseconds())

	// 检查高延迟
	if status.LatencyMs > 100 {
		j.sendAlert(ctx, &HealthAlert{
			Service:   "redis",
			AlertType: "high_latency",
			Severity:  "warning",
			Message:   fmt.Sprintf("Redis latency is high: %dms", status.LatencyMs),
			Details:   map[string]interface{}{"latency_ms": status.LatencyMs},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	return status
}

// checkEndpoint 检查服务端点
func (j *HealthMonitorJob) checkEndpoint(ctx context.Context, endpoint ServiceEndpoint) *ComponentStatus {
	// 根据检查类型选择不同的检查方法
	if endpoint.CheckType == CheckTypeGRPC {
		return j.checkGRPCEndpoint(ctx, endpoint)
	}
	return j.checkHTTPEndpoint(ctx, endpoint)
}

// checkHTTPEndpoint 检查 HTTP 服务端点
func (j *HealthMonitorJob) checkHTTPEndpoint(ctx context.Context, endpoint ServiceEndpoint) *ComponentStatus {
	start := time.Now()
	status := &ComponentStatus{
		Component: endpoint.Name,
		CheckedAt: time.Now().UnixMilli(),
	}

	timeout := endpoint.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint.URL, nil)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	resp, err := j.httpClient.Do(req)
	if err != nil {
		status.Error = err.Error()
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "service_down",
			Severity:  "critical",
			Message:   fmt.Sprintf("Service %s is unreachable: %s", endpoint.Name, err.Error()),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}
	defer resp.Body.Close()

	status.LatencyMs = int(time.Since(start).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		status.Error = fmt.Sprintf("unhealthy status code: %d", resp.StatusCode)
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "service_down",
			Severity:  "warning",
			Message:   fmt.Sprintf("Service %s returned unhealthy status: %d", endpoint.Name, resp.StatusCode),
			Details:   map[string]interface{}{"status_code": resp.StatusCode},
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	status.Healthy = true

	// 检查高延迟
	if status.LatencyMs > 5000 {
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "high_latency",
			Severity:  "warning",
			Message:   fmt.Sprintf("Service %s latency is high: %dms", endpoint.Name, status.LatencyMs),
			Details:   map[string]interface{}{"latency_ms": status.LatencyMs},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	return status
}

// checkGRPCEndpoint 检查 gRPC 服务端点
func (j *HealthMonitorJob) checkGRPCEndpoint(ctx context.Context, endpoint ServiceEndpoint) *ComponentStatus {
	start := time.Now()
	status := &ComponentStatus{
		Component: endpoint.Name,
		CheckedAt: time.Now().UnixMilli(),
	}

	timeout := endpoint.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 建立 gRPC 连接
	conn, err := grpc.DialContext(reqCtx, endpoint.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		status.Error = fmt.Sprintf("failed to connect: %s", err.Error())
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "service_down",
			Severity:  "critical",
			Message:   fmt.Sprintf("Service %s gRPC connection failed: %s", endpoint.Name, err.Error()),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}
	defer conn.Close()

	// 调用健康检查
	healthClient := grpc_health_v1.NewHealthClient(conn)
	resp, err := healthClient.Check(reqCtx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // 空字符串检查整体服务状态
	})
	if err != nil {
		status.Error = fmt.Sprintf("health check failed: %s", err.Error())
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "service_down",
			Severity:  "critical",
			Message:   fmt.Sprintf("Service %s health check failed: %s", endpoint.Name, err.Error()),
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	status.LatencyMs = int(time.Since(start).Milliseconds())

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		status.Error = fmt.Sprintf("unhealthy status: %s", resp.Status.String())
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "service_down",
			Severity:  "warning",
			Message:   fmt.Sprintf("Service %s is not serving: %s", endpoint.Name, resp.Status.String()),
			Details:   map[string]interface{}{"grpc_status": resp.Status.String()},
			Timestamp: time.Now().UnixMilli(),
		})
		return status
	}

	status.Healthy = true

	// 检查高延迟
	if status.LatencyMs > 5000 {
		j.sendAlert(ctx, &HealthAlert{
			Service:   endpoint.Name,
			AlertType: "high_latency",
			Severity:  "warning",
			Message:   fmt.Sprintf("Service %s latency is high: %dms", endpoint.Name, status.LatencyMs),
			Details:   map[string]interface{}{"latency_ms": status.LatencyMs},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	return status
}

// sendAlert 发送告警
func (j *HealthMonitorJob) sendAlert(ctx context.Context, alert *HealthAlert) {
	if j.alertFunc == nil {
		logger.Warn("health alert (no handler configured)",
			"service", alert.Service,
			"type", alert.AlertType,
			"severity", alert.Severity,
			"message", alert.Message)
		return
	}

	if err := j.alertFunc(ctx, alert); err != nil {
		logger.Error("failed to send health alert",
			"service", alert.Service,
			"error", err)
	}
}

// ComponentStatus 组件状态
type ComponentStatus struct {
	Component string `json:"component"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int    `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
	CheckedAt int64  `json:"checked_at"`
}
