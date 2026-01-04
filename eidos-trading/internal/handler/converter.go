package handler

import (
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"

	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== Proto -> Model 转换 ==========

func protoToModelOrderSide(side pb.OrderSide) model.OrderSide {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return model.OrderSideBuy
	case pb.OrderSide_ORDER_SIDE_SELL:
		return model.OrderSideSell
	default:
		return model.OrderSideBuy
	}
}

func protoToModelOrderType(orderType pb.OrderType) model.OrderType {
	switch orderType {
	case pb.OrderType_ORDER_TYPE_LIMIT:
		return model.OrderTypeLimit
	case pb.OrderType_ORDER_TYPE_MARKET:
		return model.OrderTypeMarket
	default:
		return model.OrderTypeLimit
	}
}

func protoToModelOrderStatus(status pb.OrderStatus) model.OrderStatus {
	switch status {
	case pb.OrderStatus_ORDER_STATUS_PENDING:
		return model.OrderStatusPending
	case pb.OrderStatus_ORDER_STATUS_OPEN:
		return model.OrderStatusOpen
	case pb.OrderStatus_ORDER_STATUS_PARTIAL:
		return model.OrderStatusPartial
	case pb.OrderStatus_ORDER_STATUS_FILLED:
		return model.OrderStatusFilled
	case pb.OrderStatus_ORDER_STATUS_CANCELLED:
		return model.OrderStatusCancelled
	case pb.OrderStatus_ORDER_STATUS_EXPIRED:
		return model.OrderStatusExpired
	case pb.OrderStatus_ORDER_STATUS_REJECTED:
		return model.OrderStatusRejected
	default:
		return model.OrderStatusPending
	}
}

func protoToModelBalanceLogType(logType pb.BalanceLogType) model.BalanceLogType {
	switch logType {
	case pb.BalanceLogType_BALANCE_LOG_TYPE_DEPOSIT:
		return model.BalanceLogTypeDeposit
	case pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW:
		return model.BalanceLogTypeWithdraw
	case pb.BalanceLogType_BALANCE_LOG_TYPE_FREEZE:
		return model.BalanceLogTypeFreeze
	case pb.BalanceLogType_BALANCE_LOG_TYPE_UNFREEZE:
		return model.BalanceLogTypeUnfreeze
	case pb.BalanceLogType_BALANCE_LOG_TYPE_TRADE:
		return model.BalanceLogTypeTrade
	case pb.BalanceLogType_BALANCE_LOG_TYPE_FEE:
		return model.BalanceLogTypeFee
	case pb.BalanceLogType_BALANCE_LOG_TYPE_SETTLEMENT:
		return model.BalanceLogTypeSettlement
	case pb.BalanceLogType_BALANCE_LOG_TYPE_ROLLBACK:
		return model.BalanceLogTypeRollback
	case pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW_REFUND:
		return model.BalanceLogTypeWithdrawRefund
	default:
		return model.BalanceLogTypeDeposit
	}
}

func protoToModelSettlementStatus(status pb.SettlementStatus) model.SettlementStatus {
	switch status {
	case pb.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN:
		return model.SettlementStatusMatchedOffchain
	case pb.SettlementStatus_SETTLEMENT_STATUS_PENDING:
		return model.SettlementStatusPending
	case pb.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED:
		return model.SettlementStatusSubmitted
	case pb.SettlementStatus_SETTLEMENT_STATUS_SETTLED_ONCHAIN:
		return model.SettlementStatusSettledOnchain
	case pb.SettlementStatus_SETTLEMENT_STATUS_FAILED:
		return model.SettlementStatusFailed
	case pb.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK:
		return model.SettlementStatusRolledBack
	default:
		return model.SettlementStatusMatchedOffchain
	}
}

func protoToModelDepositStatus(status pb.DepositStatus) model.DepositStatus {
	switch status {
	case pb.DepositStatus_DEPOSIT_STATUS_PENDING:
		return model.DepositStatusPending
	case pb.DepositStatus_DEPOSIT_STATUS_CONFIRMED:
		return model.DepositStatusConfirmed
	case pb.DepositStatus_DEPOSIT_STATUS_CREDITED:
		return model.DepositStatusCredited
	default:
		return model.DepositStatusPending
	}
}

func protoToModelWithdrawStatus(status pb.WithdrawStatus) model.WithdrawStatus {
	switch status {
	case pb.WithdrawStatus_WITHDRAW_STATUS_PENDING:
		return model.WithdrawStatusPending
	case pb.WithdrawStatus_WITHDRAW_STATUS_PROCESSING:
		return model.WithdrawStatusProcessing
	case pb.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED:
		return model.WithdrawStatusSubmitted
	case pb.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED:
		return model.WithdrawStatusConfirmed
	case pb.WithdrawStatus_WITHDRAW_STATUS_FAILED:
		return model.WithdrawStatusFailed
	case pb.WithdrawStatus_WITHDRAW_STATUS_CANCELLED:
		return model.WithdrawStatusCancelled
	case pb.WithdrawStatus_WITHDRAW_STATUS_REJECTED:
		return model.WithdrawStatusRejected
	default:
		return model.WithdrawStatusPending
	}
}

// ========== Model -> Proto 转换 ==========

func modelToProtoOrderSide(side model.OrderSide) pb.OrderSide {
	switch side {
	case model.OrderSideBuy:
		return pb.OrderSide_ORDER_SIDE_BUY
	case model.OrderSideSell:
		return pb.OrderSide_ORDER_SIDE_SELL
	default:
		return pb.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func modelToProtoOrderType(orderType model.OrderType) pb.OrderType {
	switch orderType {
	case model.OrderTypeLimit:
		return pb.OrderType_ORDER_TYPE_LIMIT
	case model.OrderTypeMarket:
		return pb.OrderType_ORDER_TYPE_MARKET
	default:
		return pb.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func modelToProtoOrderStatus(status model.OrderStatus) pb.OrderStatus {
	switch status {
	case model.OrderStatusPending:
		return pb.OrderStatus_ORDER_STATUS_PENDING
	case model.OrderStatusOpen:
		return pb.OrderStatus_ORDER_STATUS_OPEN
	case model.OrderStatusPartial:
		return pb.OrderStatus_ORDER_STATUS_PARTIAL
	case model.OrderStatusFilled:
		return pb.OrderStatus_ORDER_STATUS_FILLED
	case model.OrderStatusCancelled:
		return pb.OrderStatus_ORDER_STATUS_CANCELLED
	case model.OrderStatusExpired:
		return pb.OrderStatus_ORDER_STATUS_EXPIRED
	case model.OrderStatusRejected:
		return pb.OrderStatus_ORDER_STATUS_REJECTED
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func modelToProtoTimeInForce(tif model.TimeInForce) pb.TimeInForce {
	switch tif {
	case model.TimeInForceGTC:
		return pb.TimeInForce_TIME_IN_FORCE_GTC
	case model.TimeInForceIOC:
		return pb.TimeInForce_TIME_IN_FORCE_IOC
	case model.TimeInForceFOK:
		return pb.TimeInForce_TIME_IN_FORCE_FOK
	default:
		return pb.TimeInForce_TIME_IN_FORCE_UNSPECIFIED
	}
}

func modelToProtoBalanceLogType(logType model.BalanceLogType) pb.BalanceLogType {
	switch logType {
	case model.BalanceLogTypeDeposit:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_DEPOSIT
	case model.BalanceLogTypeWithdraw:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW
	case model.BalanceLogTypeFreeze:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_FREEZE
	case model.BalanceLogTypeUnfreeze:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_UNFREEZE
	case model.BalanceLogTypeTrade:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_TRADE
	case model.BalanceLogTypeFee:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_FEE
	case model.BalanceLogTypeSettlement:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_SETTLEMENT
	case model.BalanceLogTypeRollback:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_ROLLBACK
	case model.BalanceLogTypeWithdrawRefund:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_WITHDRAW_REFUND
	default:
		return pb.BalanceLogType_BALANCE_LOG_TYPE_UNSPECIFIED
	}
}

func modelToProtoSettlementStatus(status model.SettlementStatus) pb.SettlementStatus {
	switch status {
	case model.SettlementStatusMatchedOffchain:
		return pb.SettlementStatus_SETTLEMENT_STATUS_MATCHED_OFFCHAIN
	case model.SettlementStatusPending:
		return pb.SettlementStatus_SETTLEMENT_STATUS_PENDING
	case model.SettlementStatusSubmitted:
		return pb.SettlementStatus_SETTLEMENT_STATUS_SUBMITTED
	case model.SettlementStatusSettledOnchain:
		return pb.SettlementStatus_SETTLEMENT_STATUS_SETTLED_ONCHAIN
	case model.SettlementStatusFailed:
		return pb.SettlementStatus_SETTLEMENT_STATUS_FAILED
	case model.SettlementStatusRolledBack:
		return pb.SettlementStatus_SETTLEMENT_STATUS_ROLLED_BACK
	default:
		return pb.SettlementStatus_SETTLEMENT_STATUS_UNSPECIFIED
	}
}

func modelToProtoDepositStatus(status model.DepositStatus) pb.DepositStatus {
	switch status {
	case model.DepositStatusPending:
		return pb.DepositStatus_DEPOSIT_STATUS_PENDING
	case model.DepositStatusConfirmed:
		return pb.DepositStatus_DEPOSIT_STATUS_CONFIRMED
	case model.DepositStatusCredited:
		return pb.DepositStatus_DEPOSIT_STATUS_CREDITED
	default:
		return pb.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

func modelToProtoWithdrawStatus(status model.WithdrawStatus) pb.WithdrawStatus {
	switch status {
	case model.WithdrawStatusPending:
		return pb.WithdrawStatus_WITHDRAW_STATUS_PENDING
	case model.WithdrawStatusProcessing:
		return pb.WithdrawStatus_WITHDRAW_STATUS_PROCESSING
	case model.WithdrawStatusSubmitted:
		return pb.WithdrawStatus_WITHDRAW_STATUS_SUBMITTED
	case model.WithdrawStatusConfirmed:
		return pb.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED
	case model.WithdrawStatusFailed:
		return pb.WithdrawStatus_WITHDRAW_STATUS_FAILED
	case model.WithdrawStatusCancelled:
		return pb.WithdrawStatus_WITHDRAW_STATUS_CANCELLED
	case model.WithdrawStatusRejected:
		return pb.WithdrawStatus_WITHDRAW_STATUS_REJECTED
	default:
		return pb.WithdrawStatus_WITHDRAW_STATUS_UNSPECIFIED
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
		Type:          modelToProtoBalanceLogType(log.Type),
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
