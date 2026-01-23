// Package kafka 提供风控服务的 Kafka 消息处理
package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
)

// Kafka Topic 常量
const (
	// TopicWithdrawalReviewResults 提现审核结果主题 (发送到 eidos-trading)
	TopicWithdrawalReviewResults = "withdrawal-review-results"
)

// Producer Kafka 生产者
type Producer struct {
	producer sarama.SyncProducer
}

// NewProducer 创建 Kafka 生产者
func NewProducer(brokers []string, clientID string) (*Producer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 3
	config.ClientID = clientID

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	return &Producer{producer: producer}, nil
}

// Close 关闭生产者
func (p *Producer) Close() error {
	return p.producer.Close()
}

// SendRiskAlert 发送风控告警
func (p *Producer) SendRiskAlert(ctx context.Context, alert *service.RiskAlertMessage) error {
	data, err := json.Marshal(alert)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: TopicRiskAlerts,
		Key:   sarama.StringEncoder(alert.Wallet),
		Value: sarama.ByteEncoder(data),
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

// RiskAlertCallback 创建风控告警回调函数
func (p *Producer) RiskAlertCallback() func(ctx context.Context, alert *service.RiskAlertMessage) error {
	return func(ctx context.Context, alert *service.RiskAlertMessage) error {
		return p.SendRiskAlert(ctx, alert)
	}
}

// WithdrawalReviewResultMessage 提现审核结果消息
type WithdrawalReviewResultMessage struct {
	WithdrawalID string `json:"withdrawal_id"`
	ReviewID     string `json:"review_id"`
	Result       string `json:"result"` // approved, rejected
	Reviewer     string `json:"reviewer"`
	Comment      string `json:"comment"`
	RiskScore    int    `json:"risk_score"`
	Timestamp    int64  `json:"timestamp"`
}

// SendWithdrawalReviewResult 发送提现审核结果到 Kafka
// eidos-trading 消费此消息来更新提现状态
func (p *Producer) SendWithdrawalReviewResult(ctx context.Context, result *WithdrawalReviewResultMessage) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: TopicWithdrawalReviewResults,
		Key:   sarama.StringEncoder(result.WithdrawalID),
		Value: sarama.ByteEncoder(data),
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		logger.Error("发送提现审核结果失败",
			"withdrawal_id", result.WithdrawalID,
			"error", err)
		return err
	}

	logger.Info("提现审核结果已发送",
		"withdrawal_id", result.WithdrawalID,
		"result", result.Result,
		"partition", partition,
		"offset", offset)

	return nil
}

// WithdrawalApprovedCallback 创建提现通过回调函数
func (p *Producer) WithdrawalApprovedCallback() func(ctx context.Context, review *model.WithdrawalReview) error {
	return func(ctx context.Context, review *model.WithdrawalReview) error {
		return p.SendWithdrawalReviewResult(ctx, &WithdrawalReviewResultMessage{
			WithdrawalID: review.WithdrawalID,
			ReviewID:     review.ReviewID,
			Result:       "approved",
			Reviewer:     review.Reviewer,
			Comment:      review.ReviewComment,
			RiskScore:    review.RiskScore,
			Timestamp:    time.Now().UnixMilli(),
		})
	}
}

// WithdrawalRejectedCallback 创建提现拒绝回调函数
func (p *Producer) WithdrawalRejectedCallback() func(ctx context.Context, review *model.WithdrawalReview) error {
	return func(ctx context.Context, review *model.WithdrawalReview) error {
		return p.SendWithdrawalReviewResult(ctx, &WithdrawalReviewResultMessage{
			WithdrawalID: review.WithdrawalID,
			ReviewID:     review.ReviewID,
			Result:       "rejected",
			Reviewer:     review.Reviewer,
			Comment:      review.ReviewComment,
			RiskScore:    review.RiskScore,
			Timestamp:    time.Now().UnixMilli(),
		})
	}
}
