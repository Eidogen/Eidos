package handler

import (
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== Proto -> Model 转换 ==========

func protoToModelOrderSide(side commonv1.OrderSide) model.OrderSide {
	switch side {
	case commonv1.OrderSide_ORDER_SIDE_BUY:
		return model.OrderSideBuy
	case commonv1.OrderSide_ORDER_SIDE_SELL:
		return model.OrderSideSell
	default:
		return model.OrderSideBuy
	}
}

func protoToModelOrderType(orderType commonv1.OrderType) model.OrderType {
	switch orderType {
	case commonv1.OrderType_ORDER_TYPE_LIMIT:
		return model.OrderTypeLimit
	case commonv1.OrderType_ORDER_TYPE_MARKET:
		return model.OrderTypeMarket
	default:
		return model.OrderTypeLimit
	}
}

func protoToModelOrderStatus(status commonv1.OrderStatus) model.OrderStatus {
	switch status {
	case commonv1.OrderStatus_ORDER_STATUS_PENDING:
		return model.OrderStatusPending
	case commonv1.OrderStatus_ORDER_STATUS_OPEN:
		return model.OrderStatusOpen
	case commonv1.OrderStatus_ORDER_STATUS_PARTIAL:
		return model.OrderStatusPartial
	case commonv1.OrderStatus_ORDER_STATUS_FILLED:
		return model.OrderStatusFilled
	case commonv1.OrderStatus_ORDER_STATUS_CANCELLED:
		return model.OrderStatusCancelled
	case commonv1.OrderStatus_ORDER_STATUS_EXPIRED:
		return model.OrderStatusExpired
	case commonv1.OrderStatus_ORDER_STATUS_REJECTED:
		return model.OrderStatusRejected
	default:
		return model.OrderStatusPending
	}
}

func protoToModelBalanceChangeType(changeType commonv1.BalanceChangeType) model.BalanceLogType {
	switch changeType {
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT:
		return model.BalanceLogTypeDeposit
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW:
		return model.BalanceLogTypeWithdraw
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE:
		return model.BalanceLogTypeFreeze
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE:
		return model.BalanceLogTypeUnfreeze
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT:
		return model.BalanceLogTypeTrade
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE:
		return model.BalanceLogTypeFee
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_SETTLEMENT:
		return model.BalanceLogTypeSettlement
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_ROLLBACK:
		return model.BalanceLogTypeRollback
	case commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND:
		return model.BalanceLogTypeWithdrawRefund
	default:
		return model.BalanceLogTypeDeposit
	}
}

func protoToModelSettlementStatus(status commonv1.SettlementStatus) model.SettlementStatus {
	switch status {
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN:
		return model.SettlementStatusMatchedOffchain
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_PENDING:
		return model.SettlementStatusPending
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED:
		return model.SettlementStatusSubmitted
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_SETTLED:
		return model.SettlementStatusSettledOnchain
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_FAILED:
		return model.SettlementStatusFailed
	case commonv1.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK:
		return model.SettlementStatusRolledBack
	default:
		return model.SettlementStatusMatchedOffchain
	}
}

func protoToModelDepositStatus(status commonv1.DepositStatus) model.DepositStatus {
	switch status {
	case commonv1.DepositStatus_DEPOSIT_STATUS_PENDING:
		return model.DepositStatusPending
	case commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED:
		return model.DepositStatusConfirmed
	case commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED:
		return model.DepositStatusCredited
	default:
		return model.DepositStatusPending
	}
}

func protoToModelWithdrawStatus(status commonv1.WithdrawStatus) model.WithdrawStatus {
	switch status {
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING:
		return model.WithdrawStatusPending
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING:
		return model.WithdrawStatusProcessing
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED:
		return model.WithdrawStatusSubmitted
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED:
		return model.WithdrawStatusConfirmed
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED:
		return model.WithdrawStatusFailed
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED:
		return model.WithdrawStatusCancelled
	case commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED:
		return model.WithdrawStatusRejected
	default:
		return model.WithdrawStatusPending
	}
}

// ========== Model -> Proto 转换 ==========

func modelToProtoOrderSide(side model.OrderSide) commonv1.OrderSide {
	switch side {
	case model.OrderSideBuy:
		return commonv1.OrderSide_ORDER_SIDE_BUY
	case model.OrderSideSell:
		return commonv1.OrderSide_ORDER_SIDE_SELL
	default:
		return commonv1.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func modelToProtoOrderType(orderType model.OrderType) commonv1.OrderType {
	switch orderType {
	case model.OrderTypeLimit:
		return commonv1.OrderType_ORDER_TYPE_LIMIT
	case model.OrderTypeMarket:
		return commonv1.OrderType_ORDER_TYPE_MARKET
	default:
		return commonv1.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func modelToProtoOrderStatus(status model.OrderStatus) commonv1.OrderStatus {
	switch status {
	case model.OrderStatusPending:
		return commonv1.OrderStatus_ORDER_STATUS_PENDING
	case model.OrderStatusOpen:
		return commonv1.OrderStatus_ORDER_STATUS_OPEN
	case model.OrderStatusPartial:
		return commonv1.OrderStatus_ORDER_STATUS_PARTIAL
	case model.OrderStatusFilled:
		return commonv1.OrderStatus_ORDER_STATUS_FILLED
	case model.OrderStatusCancelled:
		return commonv1.OrderStatus_ORDER_STATUS_CANCELLED
	case model.OrderStatusExpired:
		return commonv1.OrderStatus_ORDER_STATUS_EXPIRED
	case model.OrderStatusRejected:
		return commonv1.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return commonv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func modelToProtoTimeInForce(tif model.TimeInForce) commonv1.TimeInForce {
	switch tif {
	case model.TimeInForceGTC:
		return commonv1.TimeInForce_TIME_IN_FORCE_GTC
	case model.TimeInForceIOC:
		return commonv1.TimeInForce_TIME_IN_FORCE_IOC
	case model.TimeInForceFOK:
		return commonv1.TimeInForce_TIME_IN_FORCE_FOK
	default:
		return commonv1.TimeInForce_TIME_IN_FORCE_UNSPECIFIED
	}
}

func modelToProtoBalanceChangeType(logType model.BalanceLogType) commonv1.BalanceChangeType {
	switch logType {
	case model.BalanceLogTypeDeposit:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT
	case model.BalanceLogTypeWithdraw:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW
	case model.BalanceLogTypeFreeze:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE
	case model.BalanceLogTypeUnfreeze:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE
	case model.BalanceLogTypeTrade:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT
	case model.BalanceLogTypeFee:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE
	case model.BalanceLogTypeSettlement:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_SETTLEMENT
	case model.BalanceLogTypeRollback:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_ROLLBACK
	case model.BalanceLogTypeWithdrawRefund:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND
	default:
		return commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNSPECIFIED
	}
}

func modelToProtoSettlementStatus(status model.SettlementStatus) commonv1.SettlementStatus {
	switch status {
	case model.SettlementStatusMatchedOffchain:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN
	case model.SettlementStatusPending:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_PENDING
	case model.SettlementStatusSubmitted:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED
	case model.SettlementStatusSettledOnchain:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_SETTLED
	case model.SettlementStatusFailed:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_FAILED
	case model.SettlementStatusRolledBack:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK
	default:
		return commonv1.SettlementStatus_SETTLEMENT_STATUS_UNSPECIFIED
	}
}

func modelToProtoDepositStatus(status model.DepositStatus) commonv1.DepositStatus {
	switch status {
	case model.DepositStatusPending:
		return commonv1.DepositStatus_DEPOSIT_STATUS_PENDING
	case model.DepositStatusConfirmed:
		return commonv1.DepositStatus_DEPOSIT_STATUS_CONFIRMED
	case model.DepositStatusCredited:
		return commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED
	default:
		return commonv1.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

func modelToProtoWithdrawStatus(status model.WithdrawStatus) commonv1.WithdrawStatus {
	switch status {
	case model.WithdrawStatusPending:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_PENDING
	case model.WithdrawStatusProcessing:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_PROCESSING
	case model.WithdrawStatusSubmitted:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED
	case model.WithdrawStatusConfirmed:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED
	case model.WithdrawStatusFailed:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_FAILED
	case model.WithdrawStatusCancelled:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_CANCELLED
	case model.WithdrawStatusRejected:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_REJECTED
	default:
		return commonv1.WithdrawStatus_WITHDRAW_STATUS_UNSPECIFIED
	}
}

// ========== 完整对象转换 ==========

func modelToProtoOrder(order *model.Order) *pb.Order {
	return &pb.Order{
		OrderId:         order.OrderID,
		Wallet:          order.Wallet,
		Market:          order.Market,
		Side:            modelToProtoOrderSide(order.Side),
		Type:            modelToProtoOrderType(order.Type),
		Price:           order.Price.String(),
		Amount:          order.Amount.String(),
		FilledAmount:    order.FilledAmount.String(),
		FilledQuote:     order.FilledQuote.String(),
		RemainingAmount: order.RemainingAmount.String(),
		AvgPrice:        order.AvgPrice.String(),
		Status:          modelToProtoOrderStatus(order.Status),
		TimeInForce:     modelToProtoTimeInForce(order.TimeInForce),
		Nonce:           order.Nonce,
		ClientOrderId:   order.ClientOrderID,
		ExpireAt:        order.ExpireAt,
		RejectReason:    order.RejectReason,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
	}
}

func modelToProtoBalance(balance *model.Balance) *pb.Balance {
	return &pb.Balance{
		Wallet:           balance.Wallet,
		Token:            balance.Token,
		SettledAvailable: balance.SettledAvailable.String(),
		SettledFrozen:    balance.SettledFrozen.String(),
		PendingAvailable: balance.PendingAvailable.String(),
		PendingFrozen:    balance.PendingFrozen.String(),
		TotalAvailable:   balance.TotalAvailable().String(),
		Total:            balance.Total().String(),
		Withdrawable:     balance.WithdrawableAmount().String(),
		UpdatedAt:        balance.UpdatedAt,
	}
}

func modelToProtoBalanceLog(log *model.BalanceLog) *pb.BalanceLog {
	return &pb.BalanceLog{
		Id:            log.ID,
		Wallet:        log.Wallet,
		Token:         log.Token,
		Type:          modelToProtoBalanceChangeType(log.Type),
		Amount:        log.Amount.String(),
		BalanceBefore: log.BalanceBefore.String(),
		BalanceAfter:  log.BalanceAfter.String(),
		OrderId:       log.OrderID,
		TradeId:       log.TradeID,
		TxHash:        log.TxHash,
		Remark:        log.Remark,
		CreatedAt:     log.CreatedAt,
	}
}

func modelToProtoTrade(trade *model.Trade) *pb.Trade {
	return &pb.Trade{
		TradeId:          trade.TradeID,
		Market:           trade.Market,
		MakerOrderId:     trade.MakerOrderID,
		TakerOrderId:     trade.TakerOrderID,
		MakerWallet:      trade.MakerWallet,
		TakerWallet:      trade.TakerWallet,
		Price:            trade.Price.String(),
		Amount:           trade.Amount.String(),
		QuoteAmount:      trade.QuoteAmount.String(),
		MakerFee:         trade.MakerFee.String(),
		TakerFee:         trade.TakerFee.String(),
		FeeToken:         trade.FeeToken,
		MakerSide:        modelToProtoOrderSide(trade.MakerSide),
		SettlementStatus: modelToProtoSettlementStatus(trade.SettlementStatus),
		BatchId:          trade.BatchID,
		TxHash:           trade.TxHash,
		MatchedAt:        trade.MatchedAt,
		SettledAt:        trade.SettledAt,
	}
}

func modelToProtoDeposit(deposit *model.Deposit) *pb.Deposit {
	return &pb.Deposit{
		DepositId:   deposit.DepositID,
		Wallet:      deposit.Wallet,
		Token:       deposit.Token,
		Amount:      deposit.Amount.String(),
		TxHash:      deposit.TxHash,
		LogIndex:    deposit.LogIndex,
		BlockNum:    deposit.BlockNum,
		Status:      modelToProtoDepositStatus(deposit.Status),
		DetectedAt:  deposit.DetectedAt,
		ConfirmedAt: deposit.ConfirmedAt,
		CreditedAt:  deposit.CreditedAt,
		CreatedAt:   deposit.CreatedAt,
	}
}

func modelToProtoWithdrawal(withdrawal *model.Withdrawal) *pb.Withdrawal {
	return &pb.Withdrawal{
		WithdrawId:   withdrawal.WithdrawID,
		Wallet:       withdrawal.Wallet,
		Token:        withdrawal.Token,
		Amount:       withdrawal.Amount.String(),
		ToAddress:    withdrawal.ToAddress,
		Nonce:        withdrawal.Nonce,
		Status:       modelToProtoWithdrawStatus(withdrawal.Status),
		TxHash:       withdrawal.TxHash,
		RejectReason: withdrawal.RejectReason,
		SubmittedAt:  withdrawal.SubmittedAt,
		ConfirmedAt:  withdrawal.ConfirmedAt,
		RefundedAt:   withdrawal.RefundedAt,
		CreatedAt:    withdrawal.CreatedAt,
	}
}
