package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	commonv1 "github.com/eidos-exchange/eidos/proto/common"
	pb "github.com/eidos-exchange/eidos/proto/trading/v1"
)

// ========== Order Converters Tests ==========

func TestConvertOrderSide(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"buy", 1},     // ORDER_SIDE_BUY
		{"BUY", 1},     // case insensitive
		{"Buy", 1},     // case insensitive
		{"sell", 2},    // ORDER_SIDE_SELL
		{"SELL", 2},    // case insensitive
		{"invalid", 0}, // ORDER_SIDE_UNSPECIFIED
		{"", 0},        // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertOrderSide(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertOrderSideToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "buy"},      // ORDER_SIDE_BUY
		{2, "sell"},     // ORDER_SIDE_SELL
		{0, "unknown"},  // ORDER_SIDE_UNSPECIFIED
		{99, "unknown"}, // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertOrderSideToString(commonv1.OrderSide(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertOrderType(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"limit", 1},   // ORDER_TYPE_LIMIT
		{"LIMIT", 1},   // case insensitive
		{"market", 2},  // ORDER_TYPE_MARKET
		{"MARKET", 2},  // case insensitive
		{"invalid", 0}, // ORDER_TYPE_UNSPECIFIED
		{"", 0},        // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertOrderType(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertOrderTypeToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "limit"},    // ORDER_TYPE_LIMIT
		{2, "market"},   // ORDER_TYPE_MARKET
		{0, "unknown"},  // ORDER_TYPE_UNSPECIFIED
		{99, "unknown"}, // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertOrderTypeToString(commonv1.OrderType(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertTimeInForce(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"GTC", 1},     // TIME_IN_FORCE_GTC
		{"gtc", 1},     // case insensitive
		{"", 1},        // default to GTC
		{"IOC", 2},     // TIME_IN_FORCE_IOC
		{"ioc", 2},     // case insensitive
		{"FOK", 3},     // TIME_IN_FORCE_FOK
		{"fok", 3},     // case insensitive
		{"invalid", 1}, // default to GTC
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertTimeInForce(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertTimeInForceToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "GTC"},  // TIME_IN_FORCE_GTC
		{2, "IOC"},  // TIME_IN_FORCE_IOC
		{3, "FOK"},  // TIME_IN_FORCE_FOK
		{0, "GTC"},  // default
		{99, "GTC"}, // unknown defaults to GTC
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertTimeInForceToString(commonv1.TimeInForce(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertOrderStatus(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"pending", 1},   // ORDER_STATUS_PENDING
		{"PENDING", 1},   // case insensitive
		{"open", 2},      // ORDER_STATUS_OPEN
		{"partial", 3},   // ORDER_STATUS_PARTIAL
		{"filled", 4},    // ORDER_STATUS_FILLED
		{"cancelled", 5}, // ORDER_STATUS_CANCELLED
		{"expired", 6},   // ORDER_STATUS_EXPIRED
		{"rejected", 7},  // ORDER_STATUS_REJECTED
		{"invalid", 0},   // ORDER_STATUS_UNSPECIFIED
		{"", 0},          // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertOrderStatus(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertOrderStatusToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "pending"},   // ORDER_STATUS_PENDING
		{2, "open"},      // ORDER_STATUS_OPEN
		{3, "partial"},   // ORDER_STATUS_PARTIAL
		{4, "filled"},    // ORDER_STATUS_FILLED
		{5, "cancelled"}, // ORDER_STATUS_CANCELLED
		{6, "expired"},   // ORDER_STATUS_EXPIRED
		{7, "rejected"},  // ORDER_STATUS_REJECTED
		{0, "unknown"},   // ORDER_STATUS_UNSPECIFIED
		{99, "unknown"},  // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertOrderStatusToString(commonv1.OrderStatus(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========== Balance Converters Tests ==========

func TestConvertBalanceChangeType(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"deposit", 1},         // BALANCE_CHANGE_TYPE_DEPOSIT
		{"DEPOSIT", 1},         // case insensitive
		{"withdraw", 2},        // BALANCE_CHANGE_TYPE_WITHDRAW
		{"withdraw_refund", 3}, // BALANCE_CHANGE_TYPE_WITHDRAW_REFUND
		{"trade_debit", 4},     // BALANCE_CHANGE_TYPE_TRADE_DEBIT
		{"trade_credit", 5},    // BALANCE_CHANGE_TYPE_TRADE_CREDIT
		{"fee", 6},             // BALANCE_CHANGE_TYPE_FEE
		{"freeze", 7},          // BALANCE_CHANGE_TYPE_FREEZE
		{"unfreeze", 8},        // BALANCE_CHANGE_TYPE_UNFREEZE
		{"settlement", 9},      // BALANCE_CHANGE_TYPE_SETTLEMENT
		{"rollback", 10},       // BALANCE_CHANGE_TYPE_ROLLBACK
		{"invalid", 0},         // BALANCE_CHANGE_TYPE_UNSPECIFIED
		{"", 0},                // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertBalanceChangeType(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertBalanceChangeTypeToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "deposit"},         // BALANCE_CHANGE_TYPE_DEPOSIT
		{2, "withdraw"},        // BALANCE_CHANGE_TYPE_WITHDRAW
		{3, "withdraw_refund"}, // BALANCE_CHANGE_TYPE_WITHDRAW_REFUND
		{4, "trade_debit"},     // BALANCE_CHANGE_TYPE_TRADE_DEBIT
		{5, "trade_credit"},    // BALANCE_CHANGE_TYPE_TRADE_CREDIT
		{6, "fee"},             // BALANCE_CHANGE_TYPE_FEE
		{7, "freeze"},          // BALANCE_CHANGE_TYPE_FREEZE
		{8, "unfreeze"},        // BALANCE_CHANGE_TYPE_UNFREEZE
		{9, "settlement"},      // BALANCE_CHANGE_TYPE_SETTLEMENT
		{10, "rollback"},       // BALANCE_CHANGE_TYPE_ROLLBACK
		{0, "unknown"},         // BALANCE_CHANGE_TYPE_UNSPECIFIED
		{99, "unknown"},        // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertBalanceChangeTypeToString(commonv1.BalanceChangeType(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========== Deposit Converters Tests ==========

func TestConvertDepositStatus(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"pending", 1},   // DEPOSIT_STATUS_PENDING
		{"PENDING", 1},   // case insensitive
		{"confirmed", 2}, // DEPOSIT_STATUS_CONFIRMED
		{"credited", 3},  // DEPOSIT_STATUS_CREDITED
		{"invalid", 0},   // DEPOSIT_STATUS_UNSPECIFIED
		{"", 0},          // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertDepositStatus(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertDepositStatusToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "pending"},   // DEPOSIT_STATUS_PENDING
		{2, "confirmed"}, // DEPOSIT_STATUS_CONFIRMED
		{3, "credited"},  // DEPOSIT_STATUS_CREDITED
		{0, "unknown"},   // DEPOSIT_STATUS_UNSPECIFIED
		{99, "unknown"},  // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertDepositStatusToString(commonv1.DepositStatus(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========== Withdraw Converters Tests ==========

func TestConvertWithdrawStatus(t *testing.T) {
	tests := []struct {
		input string
		want  int32
	}{
		{"pending", 1},    // WITHDRAW_STATUS_PENDING
		{"PENDING", 1},    // case insensitive
		{"processing", 2}, // WITHDRAW_STATUS_PROCESSING
		{"submitted", 3},  // WITHDRAW_STATUS_SUBMITTED
		{"confirmed", 4},  // WITHDRAW_STATUS_CONFIRMED
		{"failed", 5},     // WITHDRAW_STATUS_FAILED
		{"cancelled", 6},  // WITHDRAW_STATUS_CANCELLED
		{"rejected", 7},   // WITHDRAW_STATUS_REJECTED
		{"invalid", 0},    // WITHDRAW_STATUS_UNSPECIFIED
		{"", 0},           // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertWithdrawStatus(tt.input)
			assert.Equal(t, tt.want, int32(got))
		})
	}
}

func TestConvertWithdrawStatusToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "pending"},    // WITHDRAW_STATUS_PENDING
		{2, "processing"}, // WITHDRAW_STATUS_PROCESSING
		{3, "submitted"},  // WITHDRAW_STATUS_SUBMITTED
		{4, "confirmed"},  // WITHDRAW_STATUS_CONFIRMED
		{5, "failed"},     // WITHDRAW_STATUS_FAILED
		{6, "cancelled"},  // WITHDRAW_STATUS_CANCELLED
		{7, "rejected"},   // WITHDRAW_STATUS_REJECTED
		{0, "unknown"},    // WITHDRAW_STATUS_UNSPECIFIED
		{99, "unknown"},   // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertWithdrawStatusToString(commonv1.WithdrawStatus(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========== Settlement Converters Tests ==========

func TestConvertSettlementStatusToString(t *testing.T) {
	tests := []struct {
		input int32
		want  string
	}{
		{1, "matched_offchain"}, // SETTLEMENT_STATUS_MATCHED_OFFCHAIN
		{2, "pending"},          // SETTLEMENT_STATUS_PENDING
		{3, "submitted"},        // SETTLEMENT_STATUS_SUBMITTED
		{4, "settled_onchain"},  // SETTLEMENT_STATUS_SETTLED
		{5, "failed"},           // SETTLEMENT_STATUS_FAILED
		{6, "rolled_back"},      // SETTLEMENT_STATUS_ROLLED_BACK
		{0, "unknown"},          // SETTLEMENT_STATUS_UNSPECIFIED
		{99, "unknown"},         // unknown value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := convertSettlementStatusToString(commonv1.SettlementStatus(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========== Proto Object Converters Tests ==========

func TestConvertProtoOrder_Nil(t *testing.T) {
	result := convertProtoOrder(nil)
	assert.Nil(t, result)
}

func TestConvertProtoOrder(t *testing.T) {
	order := &pb.Order{
		OrderId:         "order-123",
		Wallet:          "0x1234",
		Market:          "BTC-USDC",
		Side:            commonv1.OrderSide_ORDER_SIDE_BUY,
		Type:            commonv1.OrderType_ORDER_TYPE_LIMIT,
		Price:           "42000.00",
		Amount:          "1.5",
		FilledAmount:    "0.5",
		FilledQuote:     "21000.00",
		RemainingAmount: "1.0",
		AvgPrice:        "42000.00",
		Status:          commonv1.OrderStatus_ORDER_STATUS_PARTIAL,
		TimeInForce:     commonv1.TimeInForce_TIME_IN_FORCE_GTC,
		Nonce:           12345,
		ClientOrderId:   "client-123",
		ExpireAt:        1704067200000,
		RejectReason:    "",
		CreatedAt:       1704067000000,
		UpdatedAt:       1704067100000,
	}

	result := convertProtoOrder(order)
	assert.NotNil(t, result)
	assert.Equal(t, "order-123", result.OrderID)
	assert.Equal(t, "0x1234", result.Wallet)
	assert.Equal(t, "BTC-USDC", result.Market)
	assert.Equal(t, "buy", result.Side)
	assert.Equal(t, "limit", result.Type)
	assert.Equal(t, "42000.00", result.Price)
	assert.Equal(t, "1.5", result.Amount)
	assert.Equal(t, "0.5", result.FilledAmount)
	assert.Equal(t, "21000.00", result.FilledQuote)
	assert.Equal(t, "1.0", result.RemainingAmount)
	assert.Equal(t, "42000.00", result.AvgPrice)
	assert.Equal(t, "partial", result.Status)
	assert.Equal(t, "GTC", result.TimeInForce)
	assert.Equal(t, uint64(12345), result.Nonce)
	assert.Equal(t, "client-123", result.ClientOrderID)
}

func TestConvertProtoBalance_Nil(t *testing.T) {
	result := convertProtoBalance(nil)
	assert.Nil(t, result)
}

func TestConvertProtoBalance(t *testing.T) {
	balance := &pb.Balance{
		Wallet:           "0x1234",
		Token:            "USDC",
		SettledAvailable: "1000.00",
		SettledFrozen:    "100.00",
		PendingAvailable: "50.00",
		PendingFrozen:    "0.00",
		TotalAvailable:   "1050.00",
		Total:            "1150.00",
		Withdrawable:     "900.00",
		UpdatedAt:        1704067200000,
	}

	result := convertProtoBalance(balance)
	assert.NotNil(t, result)
	assert.Equal(t, "0x1234", result.Wallet)
	assert.Equal(t, "USDC", result.Token)
	assert.Equal(t, "1000.00", result.SettledAvailable)
	assert.Equal(t, "100.00", result.SettledFrozen)
	assert.Equal(t, "50.00", result.PendingAvailable)
	assert.Equal(t, "0.00", result.PendingFrozen)
	assert.Equal(t, "1050.00", result.TotalAvailable)
	assert.Equal(t, "1150.00", result.Total)
	assert.Equal(t, "900.00", result.Withdrawable)
}

func TestConvertProtoBalanceLog_Nil(t *testing.T) {
	result := convertProtoBalanceLog(nil)
	assert.Nil(t, result)
}

func TestConvertProtoBalanceLog_TradeDebit(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           123,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_TRADE_DEBIT,
		Amount:       "100.00",
		BalanceAfter: "900.00",
		TradeId:      "trade-123",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "123", result.ID)
	assert.Equal(t, "0x1234", result.Wallet)
	assert.Equal(t, "USDC", result.Token)
	assert.Equal(t, "trade_debit", result.Type)
	assert.Equal(t, "trade-123", result.RefID)
	assert.Equal(t, "trade", result.RefType)
}

func TestConvertProtoBalanceLog_Fee(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           127,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FEE,
		Amount:       "-0.50",
		BalanceAfter: "1299.50",
		TradeId:      "trade-456",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "trade-456", result.RefID)
	assert.Equal(t, "trade", result.RefType)
}

func TestConvertProtoBalanceLog_Freeze(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           124,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_FREEZE,
		Amount:       "100.00",
		BalanceAfter: "800.00",
		OrderId:      "order-123",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "order-123", result.RefID)
	assert.Equal(t, "order", result.RefType)
}

func TestConvertProtoBalanceLog_Unfreeze(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           128,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_UNFREEZE,
		Amount:       "50.00",
		BalanceAfter: "850.00",
		OrderId:      "order-456",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "order-456", result.RefID)
	assert.Equal(t, "order", result.RefType)
}

func TestConvertProtoBalanceLog_Deposit(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           125,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_DEPOSIT,
		Amount:       "500.00",
		BalanceAfter: "1500.00",
		TxHash:       "0xabc123",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "0xabc123", result.RefID)
	assert.Equal(t, "deposit", result.RefType)
}

func TestConvertProtoBalanceLog_Withdraw(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           126,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW,
		Amount:       "-200.00",
		BalanceAfter: "1300.00",
		TxHash:       "0xdef456",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "0xdef456", result.RefID)
	assert.Equal(t, "withdraw", result.RefType)
}

func TestConvertProtoBalanceLog_WithdrawRefund(t *testing.T) {
	log := &pb.BalanceLog{
		Id:           129,
		Wallet:       "0x1234",
		Token:        "USDC",
		Type:         commonv1.BalanceChangeType_BALANCE_CHANGE_TYPE_WITHDRAW_REFUND,
		Amount:       "100.00",
		BalanceAfter: "1400.00",
		TxHash:       "0xrefund789",
		CreatedAt:    1704067200000,
	}

	result := convertProtoBalanceLog(log)
	assert.NotNil(t, result)
	assert.Equal(t, "0xrefund789", result.RefID)
	assert.Equal(t, "withdraw", result.RefType)
}

func TestConvertProtoTrade_Nil(t *testing.T) {
	result := convertProtoTrade(nil)
	assert.Nil(t, result)
}

func TestConvertProtoTrade(t *testing.T) {
	trade := &pb.Trade{
		TradeId:          "trade-123",
		Market:           "BTC-USDC",
		MakerOrderId:     "order-1",
		TakerOrderId:     "order-2",
		MakerWallet:      "0x1111",
		TakerWallet:      "0x2222",
		Price:            "42000.00",
		Amount:           "0.5",
		QuoteAmount:      "21000.00",
		MakerFee:         "10.50",
		TakerFee:         "21.00",
		FeeToken:         "USDC",
		MakerSide:        commonv1.OrderSide_ORDER_SIDE_SELL,
		SettlementStatus: commonv1.SettlementStatus_SETTLEMENT_STATUS_SETTLED,
		MatchedAt:        1704067100000,
		SettledAt:        1704067200000,
	}

	result := convertProtoTrade(trade)
	assert.NotNil(t, result)
	assert.Equal(t, "trade-123", result.TradeID)
	assert.Equal(t, "BTC-USDC", result.Market)
	assert.Equal(t, "order-1", result.MakerOrderID)
	assert.Equal(t, "order-2", result.TakerOrderID)
	assert.Equal(t, "0x1111", result.MakerWallet)
	assert.Equal(t, "0x2222", result.TakerWallet)
	assert.Equal(t, "42000.00", result.Price)
	assert.Equal(t, "0.5", result.Amount)
	assert.Equal(t, "21000.00", result.QuoteAmount)
	assert.Equal(t, "10.50", result.MakerFee)
	assert.Equal(t, "21.00", result.TakerFee)
	assert.Equal(t, "USDC", result.FeeToken)
	assert.Equal(t, "sell", result.Side)
	assert.Equal(t, "settled_onchain", result.SettlementStatus)
}

func TestConvertProtoDeposit_Nil(t *testing.T) {
	result := convertProtoDeposit(nil)
	assert.Nil(t, result)
}

func TestConvertProtoDeposit(t *testing.T) {
	deposit := &pb.Deposit{
		DepositId:   "deposit-123",
		Wallet:      "0x1234",
		Token:       "USDC",
		Amount:      "1000.00",
		TxHash:      "0xabc123",
		LogIndex:    5,
		BlockNum:    12345678,
		Status:      commonv1.DepositStatus_DEPOSIT_STATUS_CREDITED,
		DetectedAt:  1704067000000,
		ConfirmedAt: 1704067100000,
		CreditedAt:  1704067200000,
		CreatedAt:   1704067000000,
	}

	result := convertProtoDeposit(deposit)
	assert.NotNil(t, result)
	assert.Equal(t, "deposit-123", result.DepositID)
	assert.Equal(t, "0x1234", result.Wallet)
	assert.Equal(t, "USDC", result.Token)
	assert.Equal(t, "1000.00", result.Amount)
	assert.Equal(t, "0xabc123", result.TxHash)
	assert.Equal(t, uint32(5), result.LogIndex)
	assert.Equal(t, int64(12345678), result.BlockNum)
	assert.Equal(t, "credited", result.Status)
}

func TestConvertProtoWithdrawal_Nil(t *testing.T) {
	result := convertProtoWithdrawal(nil)
	assert.Nil(t, result)
}

func TestConvertProtoWithdrawal(t *testing.T) {
	withdrawal := &pb.Withdrawal{
		WithdrawId:   "withdraw-123",
		Wallet:       "0x1234",
		Token:        "USDC",
		Amount:       "500.00",
		ToAddress:    "0x5678",
		Nonce:        12345,
		Status:       commonv1.WithdrawStatus_WITHDRAW_STATUS_CONFIRMED,
		TxHash:       "0xdef456",
		RejectReason: "",
		SubmittedAt:  1704067100000,
		ConfirmedAt:  1704067200000,
		RefundedAt:   0,
		CreatedAt:    1704067000000,
	}

	result := convertProtoWithdrawal(withdrawal)
	assert.NotNil(t, result)
	assert.Equal(t, "withdraw-123", result.WithdrawID)
	assert.Equal(t, "0x1234", result.Wallet)
	assert.Equal(t, "USDC", result.Token)
	assert.Equal(t, "500.00", result.Amount)
	assert.Equal(t, "0x5678", result.ToAddress)
	assert.Equal(t, uint64(12345), result.Nonce)
	assert.Equal(t, "confirmed", result.Status)
	assert.Equal(t, "0xdef456", result.TxHash)
}
