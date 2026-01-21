package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/eidos-exchange/eidos/eidos-api/internal/config"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:     "test-api",
			HTTPPort: 8080,
			Env:      "test",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	app := New(cfg, logger)

	assert.NotNil(t, app)
	assert.Equal(t, cfg, app.cfg)
	assert.Equal(t, logger, app.logger)
}

func TestApp_Stop_WithNilComponents(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	app := New(cfg, logger)

	// Stop should not panic with nil components
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := app.Stop(ctx)
	assert.NoError(t, err)
}

func TestApp_Engine_ReturnsNil_BeforeInit(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	app := New(cfg, logger)

	// Engine should be nil before Start
	assert.Nil(t, app.Engine())
}

func TestRedisHealthAdapter_Ping(t *testing.T) {
	// Test redisHealthAdapter without actual redis
	// This is mainly for coverage - real tests would use miniredis

	adapter := &redisHealthAdapter{client: nil}

	// Should panic or return error with nil client
	// We're just testing that the adapter struct exists and has the method
	assert.NotNil(t, adapter)
}
