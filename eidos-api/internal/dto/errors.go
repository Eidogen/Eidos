// Package dto 提供数据传输对象定义
package dto

import "net/http"

// BizError 业务错误
type BizError struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

// Error 实现 error 接口
func (e *BizError) Error() string {
	return e.Message
}

// 通用错误 (10xxx)
var (
	ErrInvalidSignature   = &BizError{10001, "INVALID_SIGNATURE", http.StatusUnauthorized}
	ErrSignatureExpired   = &BizError{10002, "SIGNATURE_EXPIRED", http.StatusUnauthorized}
	ErrInvalidParams      = &BizError{10003, "INVALID_PARAMS", http.StatusBadRequest}
	ErrUnauthorized       = &BizError{10004, "UNAUTHORIZED", http.StatusUnauthorized}
	ErrForbidden          = &BizError{10005, "FORBIDDEN", http.StatusForbidden}
	ErrSignatureReplay    = &BizError{10006, "SIGNATURE_REPLAY", http.StatusUnauthorized}
	ErrInvalidTimestamp   = &BizError{10007, "INVALID_TIMESTAMP", http.StatusBadRequest}
	ErrMissingAuthHeader  = &BizError{10008, "MISSING_AUTH_HEADER", http.StatusUnauthorized}
	ErrInvalidAuthFormat  = &BizError{10009, "INVALID_AUTH_FORMAT", http.StatusUnauthorized}
	ErrInvalidWalletAddr  = &BizError{10010, "INVALID_WALLET_ADDRESS", http.StatusBadRequest}
)

// 订单错误 (11xxx)
var (
	ErrOrderNotFound        = &BizError{11001, "ORDER_NOT_FOUND", http.StatusNotFound}
	ErrOrderAlreadyCancelled = &BizError{11002, "ORDER_ALREADY_CANCELLED", http.StatusBadRequest}
	ErrOrderAlreadyFilled   = &BizError{11003, "ORDER_ALREADY_FILLED", http.StatusBadRequest}
	ErrInvalidPrice         = &BizError{11004, "INVALID_PRICE", http.StatusBadRequest}
	ErrInvalidAmount        = &BizError{11005, "INVALID_AMOUNT", http.StatusBadRequest}
	ErrPriceTooHigh         = &BizError{11006, "PRICE_TOO_HIGH", http.StatusBadRequest}
	ErrPriceTooLow          = &BizError{11007, "PRICE_TOO_LOW", http.StatusBadRequest}
	ErrAmountTooSmall       = &BizError{11008, "AMOUNT_TOO_SMALL", http.StatusBadRequest}
	ErrAmountTooLarge       = &BizError{11009, "AMOUNT_TOO_LARGE", http.StatusBadRequest}
	ErrNotionalTooSmall     = &BizError{11010, "NOTIONAL_TOO_SMALL", http.StatusBadRequest}
	ErrOrderExpired         = &BizError{11011, "ORDER_EXPIRED", http.StatusBadRequest}
	ErrDuplicateOrder       = &BizError{11012, "DUPLICATE_ORDER", http.StatusConflict}
	ErrOrderNotCancellable  = &BizError{11013, "ORDER_NOT_CANCELLABLE", http.StatusBadRequest}
	ErrInvalidOrderSide     = &BizError{11014, "INVALID_ORDER_SIDE", http.StatusBadRequest}
	ErrInvalidOrderType     = &BizError{11015, "INVALID_ORDER_TYPE", http.StatusBadRequest}
	ErrInvalidTimeInForce   = &BizError{11016, "INVALID_TIME_IN_FORCE", http.StatusBadRequest}
)

// 资产错误 (12xxx)
var (
	ErrInsufficientBalance   = &BizError{12001, "INSUFFICIENT_BALANCE", http.StatusBadRequest}
	ErrWithdrawLimitExceeded = &BizError{12002, "WITHDRAW_LIMIT_EXCEEDED", http.StatusBadRequest}
	ErrTokenNotSupported     = &BizError{12003, "TOKEN_NOT_SUPPORTED", http.StatusBadRequest}
	ErrWithdrawAmountTooSmall = &BizError{12004, "WITHDRAW_AMOUNT_TOO_SMALL", http.StatusBadRequest}
	ErrWithdrawNotCancellable = &BizError{12005, "WITHDRAW_NOT_CANCELLABLE", http.StatusBadRequest}
	ErrWithdrawNotFound      = &BizError{12006, "WITHDRAW_NOT_FOUND", http.StatusNotFound}
	ErrDepositNotFound       = &BizError{12007, "DEPOSIT_NOT_FOUND", http.StatusNotFound}
	ErrInvalidWithdrawAddress = &BizError{12008, "INVALID_WITHDRAW_ADDRESS", http.StatusBadRequest}
)

// 市场错误 (13xxx)
var (
	ErrMarketNotFound    = &BizError{13001, "MARKET_NOT_FOUND", http.StatusNotFound}
	ErrMarketSuspended   = &BizError{13002, "MARKET_SUSPENDED", http.StatusBadRequest}
	ErrInvalidMarket     = &BizError{13003, "INVALID_MARKET", http.StatusBadRequest}
)

// 成交错误 (14xxx)
var (
	ErrTradeNotFound = &BizError{14001, "TRADE_NOT_FOUND", http.StatusNotFound}
)

// 系统错误 (20xxx)
var (
	ErrRateLimitExceeded  = &BizError{20001, "RATE_LIMIT_EXCEEDED", http.StatusTooManyRequests}
	ErrServiceUnavailable = &BizError{20002, "SERVICE_UNAVAILABLE", http.StatusServiceUnavailable}
	ErrInternalError      = &BizError{20003, "INTERNAL_ERROR", http.StatusInternalServerError}
	ErrTimeout            = &BizError{20004, "TIMEOUT", http.StatusGatewayTimeout}
	ErrUpstreamError      = &BizError{20005, "UPSTREAM_ERROR", http.StatusBadGateway}
	ErrNotImplemented     = &BizError{20006, "NOT_IMPLEMENTED", http.StatusNotImplemented}
)

// NewBizError 创建自定义业务错误
func NewBizError(code int, message string, httpStatus int) *BizError {
	return &BizError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// WithMessage 返回带自定义消息的错误副本
func (e *BizError) WithMessage(msg string) *BizError {
	return &BizError{
		Code:       e.Code,
		Message:    msg,
		HTTPStatus: e.HTTPStatus,
	}
}
