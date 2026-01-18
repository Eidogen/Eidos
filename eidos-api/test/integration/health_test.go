//go:build integration

package integration

import (
	"net/http"
)

// TestHealthLive 测试存活探针
func (s *IntegrationSuite) TestHealthLive() {
	w := s.Request(http.MethodGet, "/health/live", nil, nil)
	s.Equal(http.StatusOK, w.Code)
}

// TestHealthReady_NotReady 测试就绪探针（未就绪时）
func (s *IntegrationSuite) TestHealthReady_NotReady() {
	// 默认情况下 HealthHandler 未设置为 ready
	w := s.Request(http.MethodGet, "/health/ready", nil, nil)
	s.Equal(http.StatusServiceUnavailable, w.Code)
}

// TestHealthReady_Ready 测试就绪探针（就绪时）
func (s *IntegrationSuite) TestHealthReady_Ready() {
	// 设置为 ready 状态
	s.healthHandler.SetReady(true)
	defer s.healthHandler.SetReady(false)

	w := s.Request(http.MethodGet, "/health/ready", nil, nil)
	s.Equal(http.StatusOK, w.Code)
}
