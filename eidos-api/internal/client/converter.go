package client

import (
	"strconv"
	"strings"

	"github.com/eidos-exchange/eidos/eidos-api/internal/dto"
	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== 订单相关转换 ==========

func convertOrderSide(side string) commonv1.OrderSide {
	switch strings.ToLower(side) {
	case "buy":
		return commonv1.OrderSide_ORDER_SIDE_BUY
	case "sell":
		return commonv1.OrderSide_ORDER_SIDE_SELL
	default:
		return commonv1.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func convertOrderSideToString(side commonv1.OrderSide) string {
	switch side {
	case commonv1.OrderSide_ORDER_SIDE_BUY:
		return "buy"
	case commonv1.OrderSide_ORDER_SIDE_SELL:
		return "sell"
	default:
		return "unknown"
	}
}

func convertOrderType(orderType string) commonv1.OrderType {
	switch strings.ToLower(orderType) {
	case "limit":
		return commonv1.OrderType_ORDER_TYPE_LIMIT
	case "market":
		return commonv1.OrderType_ORDER_TYPE_MARKET
	default:
		return commonv1.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func convertOrderTypeToString(orderType commonv1.OrderType) string {
	switch orderType {
	case commonv1.OrderType_ORDER_TYPE_LIMIT:
		return "limit"
	case commonv1.OrderType_ORDER_TYPE_MARKET:
		return "market"
	default:
		return "unknown"
	}
}

func convertTimeInForce(tif string) commonv1.TimeInForce {
	switch strings.ToUpper(tif) {
	case "GTC", "":
		return commonv1.TimeInForce_TIME_IN_FORCE_GTC
	case "IOC":
		return commonv1.TimeInForce_TIME_IN_FORCE_IOC
	case "FOK":
		return commonv1.TimeInForce_TIME_IN_FORCE_FOK
	default:
		return commonv1.TimeInForce_TIME_IN_FORCE_GTC
	}
}

func convertTimeInForceToString(tif commonv1.TimeInForce) string {
	switch tif {
	case commonv1.TimeInForce_TIME_IN_FORCE_GTC:
		return "GTC"
	case commonv1.TimeInForce_TIME_IN_FORCE_IOC:
		return "IOC"
	case commonv1.TimeInForce_TIME_IN_FORCE_FOK:
		return "FOK"
	default:
		return "GTC"
	}
}

func convertOrderStatus(status string) commonv1.OrderStatus {
	switch strings.ToLower(status) {
	case "pending":
		return commonv1.OrderStatus_ORDER_STATUS_PENDING
	case "open":
		return commonv1.OrderStatus_ORDER_STATUS_OPEN
	case "partial":
		return commonv1.OrderStatus_ORDER_STATUS_PARTIAL
	case "filled":
		return commonv1.OrderStatus_ORDER_STATUS_FILLED
	case "cancelled":
		return commonv1.OrderStatus_ORDER_STATUS_CANCELLED
	case "expired":
		return commonv1.OrderStatus_ORDER_STATUS_EXPIRED
	case "rejected":
		return commonv1.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return commonv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func convertOrderStatusToString(status commonv1.OrderStatus) string {
	switch status {
	case commonv1.OrderStatus_ORDER_STATUS_PENDING:
		return "pending"
	case commonv1.OrderStatus_ORDER_STATUS_OPEN:
		return "open"
	case commonv1.OrderStatus_ORDER_STATUS_PARTIAL:
		return "partial"
	case commonv1.OrderStatus_ORDER_STATUS_FILLED:
		return "filled"
	case commonv1.OrderStatus_ORDER_STATUS_CANCELLED:
		return "cancelled"
	case commonv1.OrderStatus_ORDER_STATUS_EXPIRED:
		return "expired"
	case commonv1.OrderStatus_ORDER_STATUS_REJECTED:
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

func convertBalanceChangeType(logType string) commonv1.BalanceChangeType {
	switch strings.ToLower(logType) {
	case "deposit":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT
	case "withdraw":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW
	case "freeze":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE
	case "unfreeze":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE
	case "trade_debit":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT
	case "trade_credit":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_CREDIT
	case "fee":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE
	case "settlement":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_SETTLEMENT
	case "rollback":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_ROLLBACK
	case "withdraw_refund":
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND
	default:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNSPECIFIED
	}
}

func convertBalanceChangeTypeToString(logType commonv1.BalanceChangeType) string {
	switch logType {
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT:
		return "deposit"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW:
		return "withdraw"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE:
		return "freeze"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE:
		return "unfreeze"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT:
		return "trade_debit"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_CREDIT:
		return "trade_credit"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE:
		return "fee"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_SETTLEMENT:
		return "settlement"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_ROLLBACK:
		return "rollback"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND:
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
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT,
		commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_CREDIT,
		commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE:
		refID = log.TradeId
		refType = "trade"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE,
		commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE:
		refID = log.OrderId
		refType = "order"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT:
		refID = log.TxHash
		refType = "deposit"
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW,
		commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND:
		refID = log.TxHash
		refType = "withdraw"
	}

	return &dto.TransactionResponse{
		ID:        strconv.FormatInt(log.Id, 10),
		Wallet:    log.Wallet,
		Token:     log.Token,
		Type:      convertBalanceChangeTypeToString(log.Type),
		Amount:    log.Amount,
		Balance:   log.BalanceAfter,
		RefID:     refID,
		RefType:   refType,
		CreatedAt: log.CreatedAt,
	}
}

// ========== 成交相关转换 ==========

func convertSettlementStatusToString(status commonv1.SettlementStatus) string {
	switch status {
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN:
		return "matched_offchain"
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_PENDING:
		return "pending"
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED:
		return "submitted"
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_SETTLED:
		return "settled_onchain"
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_FAILED:
		return "failed"
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK:
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

func convertDepositStatus(status string) commonv1.DepositStatus {
	switch strings.ToLower(status) {
	case "pending":
		return commonv1.DepositStatus_DEPOSIT_STATUS_PENDING
	case "confirmed":
		return commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED
	case "credited":
		return commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED
	default:
		return commonv1.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

func convertDepositStatusToString(status commonv1.DepositStatus) string {
	switch status {
	case commonv1.DepositStatus_DEPOSIT_STATUS_PENDING:
		return "pending"
	case commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED:
		return "confirmed"
	case commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED:
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

func convertWithdrawStatus(status string) commonv1.WithdrawStatus {
	switch strings.ToLower(status) {
	case "pending":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING
	case "processing":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING
	case "submitted":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED
	case "confirmed":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED
	case "failed":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED
	case "cancelled":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED
	case "rejected":
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED
	default:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_UNSPECIFIED
	}
}

func convertWithdrawStatusToString(status commonv1.WithdrawStatus) string {
	switch status {
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING:
		return "pending"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING:
		return "processing"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED:
		return "submitted"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED:
		return "confirmed"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED:
		return "failed"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED:
		return "cancelled"
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED:
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
