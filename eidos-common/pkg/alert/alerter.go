// Package alert provides common alerting interfaces and implementations
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
)

// Severity represents alert severity levels
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Alert represents an alert message
type Alert struct {
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Severity    Severity          `json:"severity"`
	Source      string            `json:"source"`      // Service name
	Environment string            `json:"environment"` // dev/staging/prod
	Tags        map[string]string `json:"tags,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
}

// Alerter is the interface for sending alerts
type Alerter interface {
	// Send sends an alert
	Send(ctx context.Context, alert *Alert) error

	// SendAsync sends an alert asynchronously
	SendAsync(ctx context.Context, alert *Alert)
}

// Config holds alerter configuration
type Config struct {
	Enabled     bool   `mapstructure:"enabled" yaml:"enabled"`
	Environment string `mapstructure:"environment" yaml:"environment"`
	ServiceName string `mapstructure:"service_name" yaml:"service_name"`

	// Webhook configuration (DingTalk, Slack, etc.)
	WebhookURL     string `mapstructure:"webhook_url" yaml:"webhook_url"`
	WebhookType    string `mapstructure:"webhook_type" yaml:"webhook_type"` // dingtalk, slack, generic
	WebhookTimeout int    `mapstructure:"webhook_timeout" yaml:"webhook_timeout"`

	// Rate limiting
	RateLimitPerMinute int `mapstructure:"rate_limit_per_minute" yaml:"rate_limit_per_minute"`
}

// webhookAlerter implements Alerter using webhooks
type webhookAlerter struct {
	cfg    *Config
	client *http.Client

	// Rate limiting
	mu            sync.Mutex
	alertCount    int
	windowStart   time.Time
	asyncAlertCh  chan *Alert
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewAlerter creates a new alerter based on configuration
func NewAlerter(cfg *Config) Alerter {
	if cfg == nil || !cfg.Enabled {
		return &noopAlerter{}
	}

	timeout := 10 * time.Second
	if cfg.WebhookTimeout > 0 {
		timeout = time.Duration(cfg.WebhookTimeout) * time.Second
	}

	a := &webhookAlerter{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
		windowStart:  time.Now(),
		asyncAlertCh: make(chan *Alert, 100),
		stopCh:       make(chan struct{}),
	}

	// Start async worker
	a.wg.Add(1)
	go a.asyncWorker()

	return a
}

// Send sends an alert synchronously
func (a *webhookAlerter) Send(ctx context.Context, alert *Alert) error {
	if !a.checkRateLimit() {
		logger.Warn("alert rate limited",
			"title", alert.Title,
			"severity", string(alert.Severity))
		return nil
	}

	alert.Source = a.cfg.ServiceName
	alert.Environment = a.cfg.Environment
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	return a.sendWebhook(ctx, alert)
}

// SendAsync sends an alert asynchronously
func (a *webhookAlerter) SendAsync(ctx context.Context, alert *Alert) {
	alert.Source = a.cfg.ServiceName
	alert.Environment = a.cfg.Environment
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	select {
	case a.asyncAlertCh <- alert:
	default:
		logger.Warn("alert channel full, dropping alert",
			"title", alert.Title)
	}
}

// asyncWorker processes async alerts
func (a *webhookAlerter) asyncWorker() {
	defer a.wg.Done()

	for {
		select {
		case <-a.stopCh:
			return
		case alert := <-a.asyncAlertCh:
			if a.checkRateLimit() {
				if err := a.sendWebhook(context.Background(), alert); err != nil {
					logger.Error("async alert send failed",
						"title", alert.Title,
						"error", err)
				}
			}
		}
	}
}

// checkRateLimit checks if we're within rate limits
func (a *webhookAlerter) checkRateLimit() bool {
	if a.cfg.RateLimitPerMinute <= 0 {
		return true
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	if now.Sub(a.windowStart) > time.Minute {
		a.windowStart = now
		a.alertCount = 0
	}

	if a.alertCount >= a.cfg.RateLimitPerMinute {
		return false
	}

	a.alertCount++
	return true
}

// sendWebhook sends alert via webhook
func (a *webhookAlerter) sendWebhook(ctx context.Context, alert *Alert) error {
	var payload []byte
	var err error

	switch a.cfg.WebhookType {
	case "dingtalk":
		payload, err = a.formatDingTalk(alert)
	case "slack":
		payload, err = a.formatSlack(alert)
	default:
		payload, err = json.Marshal(alert)
	}

	if err != nil {
		return fmt.Errorf("format alert failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// formatDingTalk formats alert for DingTalk webhook
func (a *webhookAlerter) formatDingTalk(alert *Alert) ([]byte, error) {
	icon := "ℹ️"
	switch alert.Severity {
	case SeverityWarning:
		icon = "⚠️"
	case SeverityCritical:
		icon = "🚨"
	}

	text := fmt.Sprintf("### %s %s\n\n**环境**: %s\n**服务**: %s\n**时间**: %s\n\n%s",
		icon, alert.Title,
		alert.Environment,
		alert.Source,
		alert.Timestamp.Format("2006-01-02 15:04:05"),
		alert.Message)

	// Add tags
	if len(alert.Tags) > 0 {
		text += "\n\n**标签**:\n"
		for k, v := range alert.Tags {
			text += fmt.Sprintf("- %s: %s\n", k, v)
		}
	}

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": alert.Title,
			"text":  text,
		},
	}

	return json.Marshal(payload)
}

// formatSlack formats alert for Slack webhook
func (a *webhookAlerter) formatSlack(alert *Alert) ([]byte, error) {
	color := "#36a64f" // green for info
	switch alert.Severity {
	case SeverityWarning:
		color = "#ffc107" // yellow
	case SeverityCritical:
		color = "#dc3545" // red
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  alert.Title,
				"text":   alert.Message,
				"fields": []map[string]interface{}{
					{"title": "Environment", "value": alert.Environment, "short": true},
					{"title": "Service", "value": alert.Source, "short": true},
				},
				"footer": "Eidos Alert System",
				"ts":     alert.Timestamp.Unix(),
			},
		},
	}

	return json.Marshal(payload)
}

// Stop stops the alerter gracefully
func (a *webhookAlerter) Stop() {
	close(a.stopCh)
	a.wg.Wait()
}

// noopAlerter is a no-op implementation when alerting is disabled
type noopAlerter struct{}

func (n *noopAlerter) Send(ctx context.Context, alert *Alert) error {
	return nil
}

func (n *noopAlerter) SendAsync(ctx context.Context, alert *Alert) {}
