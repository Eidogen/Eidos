package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eidos-exchange/eidos/eidos-admin/internal/metrics"
)

// MetricsMiddleware returns a middleware that records HTTP metrics
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Record metrics after request is processed
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()

		// If no route matched, use request path
		if path == "" {
			path = c.Request.URL.Path
		}

		// Normalize path for metrics to avoid high cardinality
		path = normalizePath(path)

		metrics.RecordHTTPRequest(c.Request.Method, path, status, duration)
	}
}

// normalizePath normalizes request path for metrics
// This helps reduce cardinality by replacing dynamic path segments
func normalizePath(path string) string {
	// Common patterns to normalize
	// Most paths are already defined by gin.FullPath() which includes :param placeholders
	// But we need to handle 404 paths
	if len(path) > 100 {
		return "/..."
	}
	return path
}
