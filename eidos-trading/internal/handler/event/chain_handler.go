package event

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
	"go.uber.org/zap"
)

// DepositHandler 处理充值事件
type DepositHandler struct {
	depositService service.DepositService
}

// NewDepositHandler 创建充值处理器
func NewDepositHandler(depositService service.DepositService) *DepositHandler {
	return &DepositHandler{
		depositService: depositService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *DepositHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseDeposit(payload)
	if err != nil {
		return fmt.Errorf("parse deposit: %w", err)
	}

	logger.Info("processing deposit",
		zap.String("tx_hash", msg.TxHash),
		zap.String("wallet", msg.Wallet),
		zap.String("token", msg.Token),
		zap.String("amount", msg.Amount),
	)

	// 转换为服务层消息类型
	svcMsg := &service.DepositMessage{
		TxHash:      msg.TxHash,
		Wallet:      msg.Wallet,
		Token:       msg.Token,
		Amount:      msg.Amount,
		BlockNumber: msg.BlockNumber,
		Timestamp:   msg.Timestamp,
	}

	// 调用充值服务处理
	if err := h.depositService.ProcessDeposit(ctx, svcMsg); err != nil {
		return fmt.Errorf("process deposit: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *DepositHandler) Topic() string {
	return kafka.TopicDeposits
}

// WithdrawalConfirmedHandler 处理提现确认事件
type WithdrawalConfirmedHandler struct {
	withdrawalService service.WithdrawalService
}

// NewWithdrawalConfirmedHandler 创建提现确认处理器
func NewWithdrawalConfirmedHandler(withdrawalService service.WithdrawalService) *WithdrawalConfirmedHandler {
	return &WithdrawalConfirmedHandler{
		withdrawalService: withdrawalService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *WithdrawalConfirmedHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseWithdrawalConfirmed(payload)
	if err != nil {
		return fmt.Errorf("parse withdrawal confirmed: %w", err)
	}

	logger.Info("processing withdrawal confirmed",
		zap.String("withdrawal_id", msg.WithdrawalID),
		zap.String("tx_hash", msg.TxHash),
		zap.String("status", msg.Status),
	)

	// 转换为服务层消息类型
	svcMsg := &service.WithdrawalConfirmedMessage{
		WithdrawalID: msg.WithdrawalID,
		TxHash:       msg.TxHash,
		BlockNumber:  msg.BlockNumber,
		Status:       msg.Status,
		Timestamp:    msg.Timestamp,
	}

	// 调用提现服务处理确认
	if err := h.withdrawalService.HandleConfirm(ctx, svcMsg); err != nil {
		return fmt.Errorf("handle withdrawal confirm: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *WithdrawalConfirmedHandler) Topic() string {
	return kafka.TopicWithdrawalConfirmed
}

// SettlementConfirmedHandler 处理结算确认事件
type SettlementConfirmedHandler struct {
	clearingService service.ClearingService
}

// NewSettlementConfirmedHandler 创建结算确认处理器
func NewSettlementConfirmedHandler(clearingService service.ClearingService) *SettlementConfirmedHandler {
	return &SettlementConfirmedHandler{
		clearingService: clearingService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *SettlementConfirmedHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseSettlementConfirmed(payload)
	if err != nil {
		return fmt.Errorf("parse settlement confirmed: %w", err)
	}

	logger.Info("processing settlement confirmed",
		zap.String("settlement_id", msg.SettlementID),
		zap.String("tx_hash", msg.TxHash),
		zap.String("status", msg.Status),
	)

	// 调用清算服务处理结算确认
	if err := h.clearingService.HandleSettlementConfirm(ctx, msg); err != nil {
		return fmt.Errorf("handle settlement confirm: %w", err)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *SettlementConfirmedHandler) Topic() string {
	return kafka.TopicSettlementConfirmed
}
