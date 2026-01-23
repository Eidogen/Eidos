package event

import (
	"context"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/service"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/worker"
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
		"tx_hash", msg.TxHash,
		"wallet", msg.Wallet,
		"token", msg.Token,
		"amount", msg.Amount,
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
		"withdrawal_id", msg.WithdrawalID,
		"tx_hash", msg.TxHash,
		"status", msg.Status,
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
		"settlement_id", msg.SettlementID,
		"tx_hash", msg.TxHash,
		"status", msg.Status,
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

// WithdrawalReviewResultHandler 处理提现审核结果事件 (来自 eidos-risk)
type WithdrawalReviewResultHandler struct {
	withdrawalService service.WithdrawalService
}

// NewWithdrawalReviewResultHandler 创建提现审核结果处理器
func NewWithdrawalReviewResultHandler(withdrawalService service.WithdrawalService) *WithdrawalReviewResultHandler {
	return &WithdrawalReviewResultHandler{
		withdrawalService: withdrawalService,
	}
}

// HandleEvent 实现 EventHandler 接口
func (h *WithdrawalReviewResultHandler) HandleEvent(ctx context.Context, eventType string, payload []byte) error {
	msg, err := worker.ParseWithdrawalReviewResult(payload)
	if err != nil {
		return fmt.Errorf("parse withdrawal review result: %w", err)
	}

	logger.Info("处理提现审核结果",
		"withdrawal_id", msg.WithdrawalID,
		"result", msg.Result,
		"reviewer", msg.Reviewer,
		"risk_score", msg.RiskScore,
	)

	// 根据审核结果调用对应服务方法
	switch msg.Result {
	case "approved":
		if err := h.withdrawalService.ApproveWithdrawal(ctx, msg.WithdrawalID); err != nil {
			return fmt.Errorf("approve withdrawal: %w", err)
		}
		logger.Info("提现审核通过已处理", "withdrawal_id", msg.WithdrawalID)

	case "rejected":
		reason := msg.Comment
		if reason == "" {
			reason = "风控审核拒绝"
		}
		if err := h.withdrawalService.RejectWithdrawal(ctx, msg.WithdrawalID, reason); err != nil {
			return fmt.Errorf("reject withdrawal: %w", err)
		}
		logger.Info("提现审核拒绝已处理", "withdrawal_id", msg.WithdrawalID)

	default:
		logger.Warn("未知的审核结果",
			"withdrawal_id", msg.WithdrawalID,
			"result", msg.Result)
		return fmt.Errorf("unknown review result: %s", msg.Result)
	}

	return nil
}

// Topic 返回处理的 topic
func (h *WithdrawalReviewResultHandler) Topic() string {
	return kafka.TopicWithdrawalReviewResults
}
