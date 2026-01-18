package client

import (
	"strconv"
	"strings"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== 订单相关转换 ==========

func convertOrderSide(side string) pb.OrderSide {
	switch strings.ToLower(side) {
	case "buy":
		return pb.OrderSide_ORDER_SIDE_BUY
	case "sell":
		return pb.OrderSide_ORDER_SIDE_SELL
	default:
		return pb.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func convertOrderSideToString(side pb.OrderSide) string {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return "buy"
	case pb.OrderSide_ORDER_SIDE_SELL:
		return "sell"
	default:
		return "unknown"
	}
}

func convertOrderType(orderType string) pb.OrderType {
	switch strings.ToLower(orderType) {
	case "limit":
		return pb.OrderType_ORDER_TYPE_LIMIT
	case "market":
		return pb.OrderType_ORDER_TYPE_MARKET
	default:
		return pb.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func convertOrderTypeToString(orderType pb.OrderType) string {
	switch orderType {
	case pb.OrderType_ORDER_TYPE_LIMIT:
		return "limit"
	case pb.OrderType_ORDER_TYPE_MARKET:
		return "market"
	default:
		return "unknown"
	}
}

func convertTimeInForce(tif string) pb.TimeInForce {
	switch strings.ToUpper(tif) {
	case "GTC", "":
		return pb.TimeInForce_TIME_IN_FORCE_GTC
	case "IOC":
		return pb.TimeInForce_TIME_IN_FORCE_IOC
	case "FOK":
		return pb.TimeInForce_TIME_IN_FORCE_FOK
	default:
		return pb.TimeInForce_TIME_IN_FORCE_GTC
	}
}

func convertTimeInForceToString(tif pb.TimeInForce) string {
	switch tif {
	case pb.TimeInForce_TIME_IN_FORCE_GTC:
		return "GTC"
	case pb.TimeInForce_TIME_IN_FORCE_IOC:
		return "IOC"
	case pb.TimeInForce_TIME_IN_FORCE_FOK:
		return "FOK"
	default:
		return "GTC"
	}
}

func convertOrderStatus(status string) pb.OrderStatus {
	switch strings.ToLower(status) {
	case "pending":
		return pb.OrderStatus_ORDER_STATUS_PENDING
	case "open":
		return pb.OrderStatus_ORDER_STATUS_OPEN
	case "partial":
		return pb.OrderStatus_ORDER_STATUS_PARTIAL
	case "filled":
		return pb.OrderStatus_ORDER_STATUS_FILLED
	case "cancelled":
		return pb.OrderStatus_ORDER_STATUS_CANCELLED
	case "expired":
		return pb.OrderStatus_ORDER_STATUS_EXPIRED
	case "rejected":
		return pb.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func convertOrderStatusToString(status pb.OrderStatus) string {
	switch status {
	case pb.OrderStatus_ORDER_STATUS_PENDING:
		return "pending"
	case pb.OrderStatus_ORDER_STATUS_OPEN:
		return "open"
	case pb.OrderStatus_ORDER_STATUS_PARTIAL:
		return "partial"
	case pb.OrderStatus_ORDER_STATUS_FILLED:
		return "filled"
	case pb.OrderStatus_ORDER_STATUS_CANCELLED:
		return "cancelled"
	case pb.OrderStatus_ORDER_STATUS_EXPIRED:
		return "expired"
	case pb.OrderStatus_ORDER_STATUS_REJECTED:
		return "rejected"
	default:
		return "unknown"
	}
}

func convertProtoOrder(order *pb.Order) *dto.OrderResponse {
	if order == nil {
		return nil
	}
	return &dto.OrderResponse{
		OrderID:         order.OrderId,
		Wallet:          order.Wallet,
		Market:          order.Market,
		Side:            convertOrderSideToString(order.Side),
		Type:            convertOrderTypeToString(order.Type),
		Price:           order.Price,
		Amount:          order.Amount,
		FilledAmount:    order.FilledAmount,
		FilledQuote:     order.FilledQuote,
		RemainingAmount: order.RemainingAmount,
		AvgPrice:        order.AvgPrice,
		Status:          convertOrderStatusToString(order.Status),
		TimeInForce:     convertTimeInForceToString(order.TimeInForce),
		Nonce:           order.Nonce,
		ClientOrderID:   order.ClientOrderId,
		ExpireAt:        order.ExpireAt,
		RejectReason:    order.RejectReason,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
	}
}

// ========== 余额相关转换 ==========

func convertProtoBalance(balance *pb.Balance) *dto.BalanceResponse {
	if balance == nil {
		return nil
	}
	return &dto.BalanceResponse{
		Wallet:           balance.Wallet,
		Token:            balance.Token,
		SettledAvailable: balance.SettledAvailable,
		SettledFrozen:    balance.SettledFrozen,
		PendingAvailable: balance.PendingAvailable,
		PendingFrozen:    balance.PendingFrozen,
		TotalAvailable:   balance.TotalAvailable,
		Total:            balance.Total,
		Withdrawable:     balance.Withdrawable,
		UpdatedAt:        balance.UpdatedAt,
	}
}

func convertBalanceLogType(logType string) pb.BalanceLogType {
	switch strings.ToLower(logType) {
	case "deposit":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_DEPOSIT
	case "withdraw":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW
	case "freeze":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_FREEZE
	case "unfreeze":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_UNFREEZE
	case "trade":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_TRADE
	case "fee":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_FEE
	case "settlement":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_SETTLEMENT
	case "rollback":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_ROLLBACK
	case "withdraw_refund":
		return pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW_REFUND
	default:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_UNSPECIFIED
	}
}

func convertBalanceLogTypeToString(logType pb.BalanceLogType) string {
	switch logType {
	case pb.BalanceLogType_BALANCE_LOG_TYPE_DEPOSIT:
		return "deposit"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW:
		return "withdraw"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_FREEZE:
		return "freeze"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_UNFREEZE:
		return "unfreeze"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_TRADE:
		return "trade"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_FEE:
		return "fee"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_SETTLEMENT:
		return "settlement"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_ROLLBACK:
		return "rollback"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW_REFUND:
		return "withdraw_refund"
	default:
		return "unknown"
	}
}

func convertProtoBalanceLog(log *pb.BalanceLog) *dto.TransactionResponse {
	if log == nil {
		return nil
	}

	// 根据类型确定 RefID 和 RefType
	var refID, refType string
	switch log.Type {
	case pb.BalanceLogType_BALANCE_LOG_TYPE_TRADE, pb.BalanceLogType_BALANCE_LOG_TYPE_FEE:
		refID = log.TradeId
		refType = "trade"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_FREEZE, pb.BalanceLogType_BALANCE_LOG_TYPE_UNFREEZE:
		refID = log.OrderId
		refType = "order"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_DEPOSIT:
		refID = log.TxHash
		refType = "deposit"
	case pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW, pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW_REFUND:
		refID = log.TxHash
		refType = "withdraw"
	}

	return &dto.TransactionResponse{
		ID:        strconv.FormatInt(log.Id, 10),
		Wallet:    log.Wallet,
		Token:     log.Token,
		Type:      convertBalanceLogTypeToString(log.Type),
		Amount:    log.Amount,
		Balance:   log.BalanceAfter,
		RefID:     refID,
		RefType:   refType,
		CreatedAt: log.CreatedAt,
	}
}

// ========== 成交相关转换 ==========

func convertSettlementStatusToString(status pb.SettlementStatus) string {
	switch status {
	case pb.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN:
		return "matched_offchain"
	case pb.SettlementStatus_SETTLEMENT_STATUS_PENDING:
		return "pending"
	case pb.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED:
		return "submitted"
	case pb.SettlementStatus_SETTLEMENT_STATUS_SETTLED_ONCHAIN:
		return "settled_onchain"
	case pb.SettlementStatus_SETTLEMENT_STATUS_FAILED:
		return "failed"
	case pb.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK:
		return "rolled_back"
	default:
		return "unknown"
	}
}

func convertProtoTrade(trade *pb.Trade) *dto.TradeResponse {
	if trade == nil {
		return nil
	}
	return &dto.TradeResponse{
		TradeID:          trade.TradeId,
		Market:           trade.Market,
		MakerOrderID:     trade.MakerOrderId,
		TakerOrderID:     trade.TakerOrderId,
		MakerWallet:      trade.MakerWallet,
		TakerWallet:      trade.TakerWallet,
		Price:            trade.Price,
		Amount:           trade.Amount,
		QuoteAmount:      trade.QuoteAmount,
		MakerFee:         trade.MakerFee,
		TakerFee:         trade.TakerFee,
		FeeToken:         trade.FeeToken,
		Side:             convertOrderSideToString(trade.MakerSide),
		SettlementStatus: convertSettlementStatusToString(trade.SettlementStatus),
		MatchedAt:        trade.MatchedAt,
		SettledAt:        trade.SettledAt,
	}
}

// ========== 充值相关转换 ==========

func convertDepositStatus(status string) pb.DepositStatus {
	switch strings.ToLower(status) {
	case "pending":
		return pb.DepositStatus_DEPOSIT_STATUS_PENDING
	case "confirmed":
		return pb.DepositStatus_DEPOSIT_STATUS_CONFIRMED
	case "credited":
		return pb.DepositStatus_DEPOSIT_STATUS_CREDITED
	default:
		return pb.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

func convertDepositStatusToString(status pb.DepositStatus) string {
	switch status {
	case pb.DepositStatus_DEPOSIT_STATUS_PENDING:
		return "pending"
	case pb.DepositStatus_DEPOSIT_STATUS_CONFIRMED:
		return "confirmed"
	case pb.DepositStatus_DEPOSIT_STATUS_CREDITED:
		return "credited"
	default:
		return "unknown"
	}
}

func convertProtoDeposit(deposit *pb.Deposit) *dto.DepositResponse {
	if deposit == nil {
		return nil
	}
	return &dto.DepositResponse{
		DepositID:   deposit.DepositId,
		Wallet:      deposit.Wallet,
		Token:       deposit.Token,
		Amount:      deposit.Amount,
		TxHash:      deposit.TxHash,
		LogIndex:    deposit.LogIndex,
		BlockNum:    deposit.BlockNum,
		Status:      convertDepositStatusToString(deposit.Status),
		DetectedAt:  deposit.DetectedAt,
		ConfirmedAt: deposit.ConfirmedAt,
		CreditedAt:  deposit.CreditedAt,
		CreatedAt:   deposit.CreatedAt,
	}
}

// ========== 提现相关转换 ==========

func convertWithdrawStatus(status string) pb.WithdrawStatus {
	switch strings.ToLower(status) {
	case "pending":
		return pb.WithdrawStatus_WITHDRAW_STATUS_PENDING
	case "processing":
		return pb.WithdrawStatus_WITHDRAW_STATUS_PROCESSING
	case "submitted":
		return pb.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED
	case "confirmed":
		return pb.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED
	case "failed":
		return pb.WithdrawStatus_WITHDRAW_STATUS_FAILED
	case "cancelled":
		return pb.WithdrawStatus_WITHDRAW_STATUS_CANCELLED
	case "rejected":
		return pb.WithdrawStatus_WITHDRAW_STATUS_REJECTED
	default:
		return pb.WithdrawStatus_WITHDRAW_STATUS_UNSPECIFIED
	}
}

func convertWithdrawStatusToString(status pb.WithdrawStatus) string {
	switch status {
	case pb.WithdrawStatus_WITHDRAW_STATUS_PENDING:
		return "pending"
	case pb.WithdrawStatus_WITHDRAW_STATUS_PROCESSING:
		return "processing"
	case pb.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED:
		return "submitted"
	case pb.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED:
		return "confirmed"
	case pb.WithdrawStatus_WITHDRAW_STATUS_FAILED:
		return "failed"
	case pb.WithdrawStatus_WITHDRAW_STATUS_CANCELLED:
		return "cancelled"
	case pb.WithdrawStatus_WITHDRAW_STATUS_REJECTED:
		return "rejected"
	default:
		return "unknown"
	}
}

func convertProtoWithdrawal(withdrawal *pb.Withdrawal) *dto.WithdrawalResponse {
	if withdrawal == nil {
		return nil
	}
	return &dto.WithdrawalResponse{
		WithdrawID:   withdrawal.WithdrawId,
		Wallet:       withdrawal.Wallet,
		Token:        withdrawal.Token,
		Amount:       withdrawal.Amount,
		ToAddress:    withdrawal.ToAddress,
		Nonce:        withdrawal.Nonce,
		Status:       convertWithdrawStatusToString(withdrawal.Status),
		TxHash:       withdrawal.TxHash,
		RejectReason: withdrawal.RejectReason,
		SubmittedAt:  withdrawal.SubmittedAt,
		ConfirmedAt:  withdrawal.ConfirmedAt,
		RefundedAt:   withdrawal.RefundedAt,
		CreatedAt:    withdrawal.CreatedAt,
	}
}
