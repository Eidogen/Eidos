// Package kafka provides Kafka message producers for risk service
package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
)

const (
	TopicRiskAlerts        = "risk-alerts"
	TopicAccountStatus     = "account-status"
	TopicWithdrawalReview  = "withdrawal-review"
)

// AlertProducer produces risk alert messages to Kafka
type AlertProducer struct {
	producer sarama.SyncProducer
	enabled  bool
}

// NewAlertProducer creates a new alert producer
func NewAlertProducer(brokers []string) (*AlertProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 3
	config.Producer.Return.Successes = true
	config.Producer.Compression = sarama.CompressionSnappy
	config.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	return &AlertProducer{
		producer: producer,
		enabled:  true,
	}, nil
}

// Close closes the producer
func (p *AlertProducer) Close() error {
	if p.producer != nil {
		return p.producer.Close()
	}
	return nil
}

// SetEnabled enables or disables the producer
func (p *AlertProducer) SetEnabled(enabled bool) {
	p.enabled = enabled
}

// SendRiskAlert sends a risk alert message
func (p *AlertProducer) SendRiskAlert(ctx context.Context, alert *service.RiskAlertMessage) error {
	if !p.enabled {
		return nil
	}

	// Convert to Kafka message format
	kafkaAlert := &KafkaRiskAlert{
		AlertID:     alert.AlertID,
		Wallet:      alert.Wallet,
		AlertType:   alert.AlertType,
		Severity:    alert.Severity,
		RuleID:      alert.RuleID,
		Description: alert.Description,
		ActionTaken: alert.ActionTaken,
		Context:     alert.Context,
		OrderID:     alert.OrderID,
		WithdrawID:  alert.WithdrawID,
		CreatedAt:   alert.CreatedAt,
	}

	data, err := json.Marshal(kafkaAlert)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic:     TopicRiskAlerts,
		Key:       sarama.StringEncoder(alert.Wallet),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
		Headers: []sarama.RecordHeader{
			{Key: []byte("alert_type"), Value: []byte(alert.AlertType)},
			{Key: []byte("severity"), Value: []byte(alert.Severity)},
		},
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		logger.Error("failed to send risk alert",
			"alert_id", alert.AlertID,
			"error", err)
		return err
	}

	logger.Debug("risk alert sent",
		"alert_id", alert.AlertID,
		"partition", partition,
		"offset", offset)

	return nil
}

// SendAggregatedAlert sends an aggregated alert message
func (p *AlertProducer) SendAggregatedAlert(ctx context.Context, alert *service.AggregatedAlert) error {
	if !p.enabled {
		return nil
	}

	kafkaAlert := &KafkaAggregatedAlert{
		AlertID:     alert.AlertID,
		AlertType:   alert.AlertType,
		Severity:    string(alert.Severity),
		Count:       alert.Count,
		FirstSeen:   alert.FirstSeen,
		LastSeen:    alert.LastSeen,
		Wallets:     alert.Wallets,
		Description: alert.Description,
		Context:     alert.Context,
	}

	data, err := json.Marshal(kafkaAlert)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic:     TopicRiskAlerts,
		Key:       sarama.StringEncoder(alert.AlertType),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
		Headers: []sarama.RecordHeader{
			{Key: []byte("alert_type"), Value: []byte(alert.AlertType)},
			{Key: []byte("severity"), Value: []byte(alert.Severity)},
			{Key: []byte("aggregated"), Value: []byte("true")},
		},
	}

	_, _, err = p.producer.SendMessage(msg)
	return err
}

// SendAccountStatusChange sends an account status change message
func (p *AlertProducer) SendAccountStatusChange(ctx context.Context, change *AccountStatusChange) error {
	if !p.enabled {
		return nil
	}

	data, err := json.Marshal(change)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic:     TopicAccountStatus,
		Key:       sarama.StringEncoder(change.Wallet),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
		Headers: []sarama.RecordHeader{
			{Key: []byte("change_type"), Value: []byte(change.ChangeType)},
		},
	}

	_, _, err = p.producer.SendMessage(msg)
	return err
}

// SendWithdrawalReviewResult sends a withdrawal review result message
func (p *AlertProducer) SendWithdrawalReviewResult(ctx context.Context, result *WithdrawalReviewResult) error {
	if !p.enabled {
		return nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic:     TopicWithdrawalReview,
		Key:       sarama.StringEncoder(result.WithdrawalID),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
		Headers: []sarama.RecordHeader{
			{Key: []byte("status"), Value: []byte(result.Status)},
		},
	}

	_, _, err = p.producer.SendMessage(msg)
	return err
}

// KafkaRiskAlert is the Kafka message format for risk alerts
type KafkaRiskAlert struct {
	AlertID     string            `json:"alert_id"`
	Wallet      string            `json:"wallet"`
	AlertType   string            `json:"alert_type"`
	Severity    string            `json:"severity"`
	RuleID      string            `json:"rule_id"`
	Description string            `json:"description"`
	ActionTaken string            `json:"action_taken"`
	Context     map[string]string `json:"context"`
	OrderID     string            `json:"order_id,omitempty"`
	WithdrawID  string            `json:"withdraw_id,omitempty"`
	CreatedAt   int64             `json:"created_at"`
}

// KafkaAggregatedAlert is the Kafka message format for aggregated alerts
type KafkaAggregatedAlert struct {
	AlertID     string            `json:"alert_id"`
	AlertType   string            `json:"alert_type"`
	Severity    string            `json:"severity"`
	Count       int               `json:"count"`
	FirstSeen   int64             `json:"first_seen"`
	LastSeen    int64             `json:"last_seen"`
	Wallets     []string          `json:"wallets"`
	Description string            `json:"description"`
	Context     map[string]string `json:"context"`
}

// AccountStatusChange is the Kafka message format for account status changes
type AccountStatusChange struct {
	Wallet     string `json:"wallet"`
	ChangeType string `json:"change_type"` // frozen, unfrozen, blacklisted, etc.
	Reason     string `json:"reason"`
	OperatorID string `json:"operator_id"`
	ChangedAt  int64  `json:"changed_at"`
}

// WithdrawalReviewResult is the Kafka message format for withdrawal review results
type WithdrawalReviewResult struct {
	ReviewID     string `json:"review_id"`
	WithdrawalID string `json:"withdrawal_id"`
	Wallet       string `json:"wallet"`
	Status       string `json:"status"` // approved, rejected
	Reviewer     string `json:"reviewer"`
	Comment      string `json:"comment"`
	ReviewedAt   int64  `json:"reviewed_at"`
}
