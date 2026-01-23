package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 由于 redis.UniversalClient 接口非常庞大，我们通过不使用 Redis 来简化测试
// 健康监控的主要功能是 HTTP 端点检查，Redis 检查在集成测试中覆盖

func TestHealthMonitorJob_CheckEndpoint_Healthy(t *testing.T) {
	// 创建健康的服务
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "healthy"}`))
	}))
	defer server.Close()

	endpoint := ServiceEndpoint{
		Name:    "test-service",
		URL:     server.URL,
		Timeout: 5 * time.Second,
	}

	// 创建 job 但不传递 Redis 客户端（会跳过 Redis 检查）
	job := &HealthMonitorJob{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		endpoints:  []ServiceEndpoint{endpoint},
	}

	ctx := context.Background()
	status := job.checkEndpoint(ctx, endpoint)

	if !status.Healthy {
		t.Errorf("Expected endpoint to be healthy, got error: %s", status.Error)
	}

	// 注意：本地测试服务器响应可能非常快（<1ms），LatencyMs 可能为 0
	// 只要 Healthy 为 true，就说明检查正常工作
	if status.LatencyMs < 0 {
		t.Error("Expected latency to be non-negative")
	}
}

func TestHealthMonitorJob_CheckEndpoint_Unhealthy(t *testing.T) {
	// 创建返回错误的服务
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	endpoint := ServiceEndpoint{
		Name:    "unhealthy-service",
		URL:     server.URL,
		Timeout: 5 * time.Second,
	}

	job := &HealthMonitorJob{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		endpoints:  []ServiceEndpoint{endpoint},
	}

	ctx := context.Background()
	status := job.checkEndpoint(ctx, endpoint)

	if status.Healthy {
		t.Error("Expected endpoint to be unhealthy")
	}

	if status.Error == "" {
		t.Error("Expected error message for unhealthy endpoint")
	}
}

func TestHealthMonitorJob_CheckEndpoint_Timeout(t *testing.T) {
	// 创建慢响应服务
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := ServiceEndpoint{
		Name:    "slow-service",
		URL:     server.URL,
		Timeout: 100 * time.Millisecond, // 短超时
	}

	job := &HealthMonitorJob{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		endpoints:  []ServiceEndpoint{endpoint},
	}

	ctx := context.Background()
	status := job.checkEndpoint(ctx, endpoint)

	if status.Healthy {
		t.Error("Expected endpoint to be unhealthy due to timeout")
	}
}

func TestHealthMonitorJob_CheckEndpoint_Unreachable(t *testing.T) {
	endpoint := ServiceEndpoint{
		Name:    "unreachable-service",
		URL:     "http://localhost:59999", // 不存在的端口
		Timeout: 1 * time.Second,
	}

	job := &HealthMonitorJob{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		endpoints:  []ServiceEndpoint{endpoint},
	}

	ctx := context.Background()
	status := job.checkEndpoint(ctx, endpoint)

	if status.Healthy {
		t.Error("Expected endpoint to be unhealthy for unreachable service")
	}

	if status.Error == "" {
		t.Error("Expected error message for unreachable service")
	}
}

func TestServiceEndpoint(t *testing.T) {
	endpoint := ServiceEndpoint{
		Name:    "test-service",
		URL:     "http://localhost:8080/health",
		Timeout: 5 * time.Second,
	}

	if endpoint.Name != "test-service" {
		t.Errorf("Expected name 'test-service', got '%s'", endpoint.Name)
	}

	if endpoint.URL != "http://localhost:8080/health" {
		t.Errorf("Expected URL 'http://localhost:8080/health', got '%s'", endpoint.URL)
	}

	if endpoint.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", endpoint.Timeout)
	}
}

func TestComponentStatus(t *testing.T) {
	status := &ComponentStatus{
		Component: "database",
		Healthy:   true,
		LatencyMs: 50,
		CheckedAt: time.Now().UnixMilli(),
	}

	if status.Component != "database" {
		t.Errorf("Expected component 'database', got '%s'", status.Component)
	}

	if !status.Healthy {
		t.Error("Expected healthy to be true")
	}

	if status.LatencyMs != 50 {
		t.Errorf("Expected latency 50, got %d", status.LatencyMs)
	}
}

func TestHealthAlert(t *testing.T) {
	alert := &HealthAlert{
		Service:   "test-service",
		AlertType: "service_down",
		Severity:  "critical",
		Message:   "Service is unreachable",
		Details:   map[string]interface{}{"status_code": 503},
		Timestamp: time.Now().UnixMilli(),
	}

	if alert.Service != "test-service" {
		t.Errorf("Expected service 'test-service', got '%s'", alert.Service)
	}

	if alert.AlertType != "service_down" {
		t.Errorf("Expected alert type 'service_down', got '%s'", alert.AlertType)
	}

	if alert.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", alert.Severity)
	}

	if alert.Details["status_code"] != 503 {
		t.Errorf("Expected status_code 503, got %v", alert.Details["status_code"])
	}
}

func TestHealthMonitorJob_SetAlertFunc(t *testing.T) {
	job := &HealthMonitorJob{}

	alertCalled := false
	job.SetAlertFunc(func(ctx context.Context, alert *HealthAlert) error {
		alertCalled = true
		return nil
	})

	if job.alertFunc == nil {
		t.Error("Expected alertFunc to be set")
	}

	// 测试发送告警
	job.sendAlert(context.Background(), &HealthAlert{
		Service:   "test",
		AlertType: "test",
		Severity:  "warning",
		Message:   "test",
		Timestamp: time.Now().UnixMilli(),
	})

	if !alertCalled {
		t.Error("Expected alert function to be called")
	}
}
