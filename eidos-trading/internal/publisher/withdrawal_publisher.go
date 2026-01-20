// Package publisher 提供 Kafka 消息发布功能
package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"go.uber.org/zap"
)

// WithdrawalPublisher 提现消息发布者
// 发布消息到 withdrawals topic，供 eidos-chain 执行链上提现
type WithdrawalPublisher struct {
	producer KafkaProducer
}

// NewWithdrawalPublisher 创建提现发布者
func NewWithdrawalPublisher(producer KafkaProducer) *WithdrawalPublisher {
	return &WithdrawalPublisher{
		producer: producer,
	}
}

// WithdrawalRequestMessage 提现请求消息 (发送到 eidos-chain)
// 格式对应 eidos-chain/internal/model.WithdrawalRequest
type WithdrawalRequestMessage struct {
	WithdrawID  string `json:"withdraw_id"`
	Wallet      string `json:"wallet"`
	Token       string `json:"token"`
	Amount      string `json:"amount"`
	ToAddress   string `json:"to_address"`
	Nonce       uint64 `json:"nonce"`
	Signature   string `json:"signature"` // hex encoded
	RequestedAt int64  `json:"requested_at"`
}

// PublishWithdrawalRequest 发布提现请求到 withdrawals topic
func (p *WithdrawalPublisher) PublishWithdrawalRequest(ctx context.Context, withdrawal *model.Withdrawal) error {
	if p.producer == nil {
		return nil // Kafka 未启用
	}

	msg := &WithdrawalRequestMessage{
		WithdrawID:  withdrawal.WithdrawID,
		Wallet:      withdrawal.Wallet,
		Token:       withdrawal.Token,
		Amount:      withdrawal.Amount.String(),
		ToAddress:   withdrawal.ToAddress,
		Nonce:       withdrawal.Nonce,
		Signature:   fmt.Sprintf("0x%x", withdrawal.Signature), // convert []byte to hex string
		RequestedAt: withdrawal.CreatedAt,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal withdrawal request message: %w", err)
	}

	// 使用钱包地址作为分区键，保证同一用户的提现顺序
	if err := p.producer.SendWithContext(ctx, kafka.TopicWithdrawals, []byte(withdrawal.Wallet), data); err != nil {
		logger.Error("publish withdrawal request failed",
			zap.String("withdraw_id", withdrawal.WithdrawID),
			zap.String("wallet", withdrawal.Wallet),
			zap.Error(err))
		return fmt.Errorf("send withdrawal request: %w", err)
	}

	logger.Info("withdrawal request published",
		zap.String("withdraw_id", withdrawal.WithdrawID),
		zap.String("wallet", withdrawal.Wallet),
		zap.String("token", withdrawal.Token),
		zap.String("amount", withdrawal.Amount.String()))

	return nil
}
