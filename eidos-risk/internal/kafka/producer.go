// Package kafka 提供风控服务的 Kafka 消息处理
package kafka

import (
	"context"
	"encoding/json"

	"github.com/IBM/sarama"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/service"
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
