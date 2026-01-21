// Package service provides alert aggregation and deduplication
package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	alertDedupeKeyPrefix   = "risk:alert:dedupe:"
	alertAggregateKeyPrefix = "risk:alert:agg:"
	alertCountKeyPrefix    = "risk:alert:count:"
	defaultDedupeWindow    = 60 * time.Second
	defaultAggregateWindow = 300 * time.Second
)

// AlertSeverity represents alert severity levels
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AggregatedAlert represents an aggregated alert
type AggregatedAlert struct {
	AlertID     string            `json:"alert_id"`
	AlertType   string            `json:"alert_type"`
	Severity    AlertSeverity     `json:"severity"`
	Count       int               `json:"count"`
	FirstSeen   int64             `json:"first_seen"`
	LastSeen    int64             `json:"last_seen"`
	Wallets     []string          `json:"wallets"`
	Description string            `json:"description"`
	Context     map[string]string `json:"context"`
	Samples     []*RiskAlertMessage `json:"samples"`
}

// AlertRule represents a rule for alert processing
type AlertRule struct {
	AlertType       string        // Alert type to match
	Severity        AlertSeverity // Severity level
	DedupeWindow    time.Duration // Deduplication window
	AggregateWindow time.Duration // Aggregation window
	ThrottleCount   int           // Max alerts per window before throttling
	Escalate        bool          // Whether to escalate to higher severity
	Suppress        bool          // Whether to suppress this alert type
}

// AlertService provides alert aggregation and deduplication
type AlertService struct {
	mu sync.RWMutex

	redisClient redis.UniversalClient
	rules       map[string]*AlertRule

	// Configuration
	dedupeWindow    time.Duration
	aggregateWindow time.Duration
	maxSamples      int

	// Output channel
	alertChan chan *AggregatedAlert

	// Callbacks
	onAlert func(ctx context.Context, alert *AggregatedAlert) error
}

// NewAlertService creates a new alert service
func NewAlertService(redisClient redis.UniversalClient) *AlertService {
	return &AlertService{
		redisClient:     redisClient,
		rules:           make(map[string]*AlertRule),
		dedupeWindow:    defaultDedupeWindow,
		aggregateWindow: defaultAggregateWindow,
		maxSamples:      5,
		alertChan:       make(chan *AggregatedAlert, 1000),
	}
}

// SetDedupeWindow sets the deduplication window
func (s *AlertService) SetDedupeWindow(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dedupeWindow = d
}

// SetAggregateWindow sets the aggregation window
func (s *AlertService) SetAggregateWindow(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aggregateWindow = d
}

// AddRule adds an alert processing rule
func (s *AlertService) AddRule(rule *AlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[rule.AlertType] = rule
}

// SetOnAlert sets the alert output callback
func (s *AlertService) SetOnAlert(fn func(ctx context.Context, alert *AggregatedAlert) error) {
	s.onAlert = fn
}

// GetAlertChannel returns the alert output channel
func (s *AlertService) GetAlertChannel() <-chan *AggregatedAlert {
	return s.alertChan
}

// ProcessAlert processes an incoming alert with deduplication and aggregation
func (s *AlertService) ProcessAlert(ctx context.Context, alert *RiskAlertMessage) error {
	// Check if alert type is suppressed
	if rule := s.getRule(alert.AlertType); rule != nil && rule.Suppress {
		logger.Debug("alert suppressed",
			zap.String("alert_type", alert.AlertType))
		return nil
	}

	// Check for deduplication
	dedupeKey := s.generateDedupeKey(alert)
	isDuplicate, err := s.checkDuplicate(ctx, dedupeKey)
	if err != nil {
		logger.Error("failed to check duplicate", zap.Error(err))
	}

	if isDuplicate {
		// Increment count for aggregation
		s.incrementAlertCount(ctx, alert)
		logger.Debug("alert deduplicated",
			zap.String("alert_type", alert.AlertType),
			zap.String("wallet", alert.Wallet))
		return nil
	}

	// Mark as seen for deduplication
	if err := s.markAsSeen(ctx, dedupeKey); err != nil {
		logger.Error("failed to mark alert as seen", zap.Error(err))
	}

	// Check if we should aggregate
	count := s.incrementAlertCount(ctx, alert)

	// Check throttle
	if rule := s.getRule(alert.AlertType); rule != nil {
		if rule.ThrottleCount > 0 && count > rule.ThrottleCount {
			// Throttle exceeded, aggregate instead
			s.aggregateAlert(ctx, alert)
			return nil
		}
	}

	// Convert to aggregated alert and send
	aggAlert := &AggregatedAlert{
		AlertID:     alert.AlertID,
		AlertType:   alert.AlertType,
		Severity:    AlertSeverity(alert.Severity),
		Count:       1,
		FirstSeen:   alert.CreatedAt,
		LastSeen:    alert.CreatedAt,
		Wallets:     []string{alert.Wallet},
		Description: alert.Description,
		Context:     alert.Context,
		Samples:     []*RiskAlertMessage{alert},
	}

	s.outputAlert(ctx, aggAlert)
	return nil
}

// generateDedupeKey generates a deduplication key for an alert
func (s *AlertService) generateDedupeKey(alert *RiskAlertMessage) string {
	// Create a hash of alert type, wallet, and key context fields
	data := fmt.Sprintf("%s:%s:%s", alert.AlertType, alert.Wallet, alert.Context["rule_id"])
	hash := md5.Sum([]byte(data))
	return alertDedupeKeyPrefix + hex.EncodeToString(hash[:])
}

// checkDuplicate checks if an alert is a duplicate
func (s *AlertService) checkDuplicate(ctx context.Context, key string) (bool, error) {
	exists, err := s.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// markAsSeen marks an alert as seen for deduplication
func (s *AlertService) markAsSeen(ctx context.Context, key string) error {
	window := s.dedupeWindow
	if rule := s.getRule(""); rule != nil && rule.DedupeWindow > 0 {
		window = rule.DedupeWindow
	}
	return s.redisClient.Set(ctx, key, "1", window).Err()
}

// incrementAlertCount increments the alert count for aggregation
func (s *AlertService) incrementAlertCount(ctx context.Context, alert *RiskAlertMessage) int {
	key := fmt.Sprintf("%s%s:%s", alertCountKeyPrefix, alert.AlertType, alert.Wallet)

	pipe := s.redisClient.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, s.aggregateWindow)
	pipe.Exec(ctx)

	count, _ := incr.Result()
	return int(count)
}

// aggregateAlert adds an alert to the aggregation buffer
func (s *AlertService) aggregateAlert(ctx context.Context, alert *RiskAlertMessage) {
	key := fmt.Sprintf("%s%s", alertAggregateKeyPrefix, alert.AlertType)

	// Get or create aggregation
	data, err := s.redisClient.Get(ctx, key).Bytes()

	var agg AggregatedAlert
	if err == nil {
		json.Unmarshal(data, &agg)
	} else {
		agg = AggregatedAlert{
			AlertID:     uuid.New().String(),
			AlertType:   alert.AlertType,
			Severity:    AlertSeverity(alert.Severity),
			Count:       0,
			FirstSeen:   alert.CreatedAt,
			Wallets:     make([]string, 0),
			Context:     make(map[string]string),
			Samples:     make([]*RiskAlertMessage, 0),
		}
	}

	// Update aggregation
	agg.Count++
	agg.LastSeen = alert.CreatedAt

	// Add wallet if not already present
	walletExists := false
	for _, w := range agg.Wallets {
		if w == alert.Wallet {
			walletExists = true
			break
		}
	}
	if !walletExists && len(agg.Wallets) < 10 {
		agg.Wallets = append(agg.Wallets, alert.Wallet)
	}

	// Add sample if not full
	if len(agg.Samples) < s.maxSamples {
		agg.Samples = append(agg.Samples, alert)
	}

	// Update description
	agg.Description = fmt.Sprintf("%s (aggregated: %d alerts from %d wallets)",
		alert.AlertType, agg.Count, len(agg.Wallets))

	// Save aggregation
	aggData, _ := json.Marshal(agg)
	s.redisClient.Set(ctx, key, aggData, s.aggregateWindow)
}

// FlushAggregations flushes all aggregated alerts
func (s *AlertService) FlushAggregations(ctx context.Context) error {
	// Find all aggregation keys
	keys, err := s.redisClient.Keys(ctx, alertAggregateKeyPrefix+"*").Result()
	if err != nil {
		return err
	}

	for _, key := range keys {
		data, err := s.redisClient.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var agg AggregatedAlert
		if err := json.Unmarshal(data, &agg); err != nil {
			continue
		}

		if agg.Count > 0 {
			s.outputAlert(ctx, &agg)
		}

		// Delete the aggregation
		s.redisClient.Del(ctx, key)
	}

	return nil
}

// outputAlert outputs an aggregated alert
func (s *AlertService) outputAlert(ctx context.Context, alert *AggregatedAlert) {
	// Apply escalation if configured
	if rule := s.getRule(alert.AlertType); rule != nil && rule.Escalate {
		if alert.Count >= 10 && alert.Severity == AlertSeverityWarning {
			alert.Severity = AlertSeverityCritical
		} else if alert.Count >= 5 && alert.Severity == AlertSeverityInfo {
			alert.Severity = AlertSeverityWarning
		}
	}

	// Send to callback
	if s.onAlert != nil {
		if err := s.onAlert(ctx, alert); err != nil {
			logger.Error("failed to send aggregated alert", zap.Error(err))
		}
	}

	// Send to channel
	select {
	case s.alertChan <- alert:
	default:
		logger.Warn("alert channel full, dropping alert",
			zap.String("alert_type", alert.AlertType))
	}

	logger.Debug("alert output",
		zap.String("alert_type", alert.AlertType),
		zap.String("severity", string(alert.Severity)),
		zap.Int("count", alert.Count))
}

// getRule gets the rule for an alert type
func (s *AlertService) getRule(alertType string) *AlertRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rules[alertType]
}

// StartBackgroundTasks starts background tasks
func (s *AlertService) StartBackgroundTasks(ctx context.Context) {
	// Flush aggregations periodically
	go func() {
		ticker := time.NewTicker(s.aggregateWindow)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.FlushAggregations(ctx)
			}
		}
	}()

	logger.Info("alert service background tasks started")
}

// GetAlertStats gets alert statistics
func (s *AlertService) GetAlertStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count dedupe keys
	dedupeKeys, _ := s.redisClient.Keys(ctx, alertDedupeKeyPrefix+"*").Result()
	stats["dedupe_count"] = len(dedupeKeys)

	// Count aggregation keys
	aggKeys, _ := s.redisClient.Keys(ctx, alertAggregateKeyPrefix+"*").Result()
	stats["aggregation_count"] = len(aggKeys)

	// Count alert counts
	countKeys, _ := s.redisClient.Keys(ctx, alertCountKeyPrefix+"*").Result()
	stats["alert_count_entries"] = len(countKeys)

	return stats, nil
}

// ClearAlertStats clears all alert statistics from cache
func (s *AlertService) ClearAlertStats(ctx context.Context) error {
	pipe := s.redisClient.Pipeline()

	// Clear dedupe keys
	dedupeKeys, _ := s.redisClient.Keys(ctx, alertDedupeKeyPrefix+"*").Result()
	for _, key := range dedupeKeys {
		pipe.Del(ctx, key)
	}

	// Clear aggregation keys
	aggKeys, _ := s.redisClient.Keys(ctx, alertAggregateKeyPrefix+"*").Result()
	for _, key := range aggKeys {
		pipe.Del(ctx, key)
	}

	// Clear count keys
	countKeys, _ := s.redisClient.Keys(ctx, alertCountKeyPrefix+"*").Result()
	for _, key := range countKeys {
		pipe.Del(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	return err
}
