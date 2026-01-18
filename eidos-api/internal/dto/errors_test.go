package dto

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBizError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *BizError
		wantMsg string
	}{
		{"invalid_signature", ErrInvalidSignature, "INVALID_SIGNATURE"},
		{"signature_expired", ErrSignatureExpired, "SIGNATURE_EXPIRED"},
		{"invalid_params", ErrInvalidParams, "INVALID_PARAMS"},
		{"unauthorized", ErrUnauthorized, "UNAUTHORIZED"},
		{"forbidden", ErrForbidden, "FORBIDDEN"},
		{"signature_replay", ErrSignatureReplay, "SIGNATURE_REPLAY"},
		{"order_not_found", ErrOrderNotFound, "ORDER_NOT_FOUND"},
		{"insufficient_balance", ErrInsufficientBalance, "INSUFFICIENT_BALANCE"},
		{"market_not_found", ErrMarketNotFound, "MARKET_NOT_FOUND"},
		{"trade_not_found", ErrTradeNotFound, "TRADE_NOT_FOUND"},
		{"rate_limit_exceeded", ErrRateLimitExceeded, "RATE_LIMIT_EXCEEDED"},
		{"internal_error", ErrInternalError, "INTERNAL_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

func TestNewBizError(t *testing.T) {
	err := NewBizError(99999, "CUSTOM_ERROR", http.StatusTeapot)
	assert.Equal(t, 99999, err.Code)
	assert.Equal(t, "CUSTOM_ERROR", err.Message)
	assert.Equal(t, http.StatusTeapot, err.HTTPStatus)
	assert.Equal(t, "CUSTOM_ERROR", err.Error())
}

func TestBizError_WithMessage(t *testing.T) {
	original := ErrInvalidParams
	customMsg := "field 'amount' must be positive"

	customErr := original.WithMessage(customMsg)

	// Original should be unchanged
	assert.Equal(t, "INVALID_PARAMS", original.Message)

	// Custom error should have new message but same code and status
	assert.Equal(t, original.Code, customErr.Code)
	assert.Equal(t, customMsg, customErr.Message)
	assert.Equal(t, original.HTTPStatus, customErr.HTTPStatus)

	// Error() should return the custom message
	assert.Equal(t, customMsg, customErr.Error())
}

func TestBizError_Codes(t *testing.T) {
	// Test that error codes are in expected ranges
	tests := []struct {
		name      string
		err       *BizError
		codeRange string
		minCode   int
		maxCode   int
	}{
		// 通用错误 (10xxx)
		{"invalid_signature", ErrInvalidSignature, "10xxx", 10000, 10999},
		{"signature_expired", ErrSignatureExpired, "10xxx", 10000, 10999},
		{"invalid_params", ErrInvalidParams, "10xxx", 10000, 10999},
		{"unauthorized", ErrUnauthorized, "10xxx", 10000, 10999},
		{"forbidden", ErrForbidden, "10xxx", 10000, 10999},
		{"missing_auth_header", ErrMissingAuthHeader, "10xxx", 10000, 10999},
		{"invalid_auth_format", ErrInvalidAuthFormat, "10xxx", 10000, 10999},
		{"invalid_wallet_addr", ErrInvalidWalletAddr, "10xxx", 10000, 10999},

		// 订单错误 (11xxx)
		{"order_not_found", ErrOrderNotFound, "11xxx", 11000, 11999},
		{"order_already_cancelled", ErrOrderAlreadyCancelled, "11xxx", 11000, 11999},
		{"order_already_filled", ErrOrderAlreadyFilled, "11xxx", 11000, 11999},
		{"invalid_price", ErrInvalidPrice, "11xxx", 11000, 11999},
		{"invalid_amount", ErrInvalidAmount, "11xxx", 11000, 11999},
		{"duplicate_order", ErrDuplicateOrder, "11xxx", 11000, 11999},
		{"order_not_cancellable", ErrOrderNotCancellable, "11xxx", 11000, 11999},

		// 资产错误 (12xxx)
		{"insufficient_balance", ErrInsufficientBalance, "12xxx", 12000, 12999},
		{"withdraw_limit_exceeded", ErrWithdrawLimitExceeded, "12xxx", 12000, 12999},
		{"token_not_supported", ErrTokenNotSupported, "12xxx", 12000, 12999},
		{"withdraw_not_found", ErrWithdrawNotFound, "12xxx", 12000, 12999},
		{"deposit_not_found", ErrDepositNotFound, "12xxx", 12000, 12999},

		// 市场错误 (13xxx)
		{"market_not_found", ErrMarketNotFound, "13xxx", 13000, 13999},
		{"market_suspended", ErrMarketSuspended, "13xxx", 13000, 13999},
		{"invalid_market", ErrInvalidMarket, "13xxx", 13000, 13999},

		// 成交错误 (14xxx)
		{"trade_not_found", ErrTradeNotFound, "14xxx", 14000, 14999},

		// 系统错误 (20xxx)
		{"rate_limit_exceeded", ErrRateLimitExceeded, "20xxx", 20000, 20999},
		{"service_unavailable", ErrServiceUnavailable, "20xxx", 20000, 20999},
		{"internal_error", ErrInternalError, "20xxx", 20000, 20999},
		{"timeout", ErrTimeout, "20xxx", 20000, 20999},
		{"upstream_error", ErrUpstreamError, "20xxx", 20000, 20999},
		{"not_implemented", ErrNotImplemented, "20xxx", 20000, 20999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.GreaterOrEqual(t, tt.err.Code, tt.minCode,
				"Error code for %s should be >= %d", tt.name, tt.minCode)
			assert.LessOrEqual(t, tt.err.Code, tt.maxCode,
				"Error code for %s should be <= %d", tt.name, tt.maxCode)
		})
	}
}

func TestBizError_HTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        *BizError
		wantStatus int
	}{
		// 400 Bad Request
		{"invalid_params", ErrInvalidParams, http.StatusBadRequest},
		{"invalid_price", ErrInvalidPrice, http.StatusBadRequest},
		{"invalid_amount", ErrInvalidAmount, http.StatusBadRequest},
		{"insufficient_balance", ErrInsufficientBalance, http.StatusBadRequest},
		{"order_already_cancelled", ErrOrderAlreadyCancelled, http.StatusBadRequest},
		{"order_already_filled", ErrOrderAlreadyFilled, http.StatusBadRequest},

		// 401 Unauthorized
		{"invalid_signature", ErrInvalidSignature, http.StatusUnauthorized},
		{"signature_expired", ErrSignatureExpired, http.StatusUnauthorized},
		{"unauthorized", ErrUnauthorized, http.StatusUnauthorized},
		{"signature_replay", ErrSignatureReplay, http.StatusUnauthorized},
		{"missing_auth_header", ErrMissingAuthHeader, http.StatusUnauthorized},
		{"invalid_auth_format", ErrInvalidAuthFormat, http.StatusUnauthorized},

		// 403 Forbidden
		{"forbidden", ErrForbidden, http.StatusForbidden},

		// 404 Not Found
		{"order_not_found", ErrOrderNotFound, http.StatusNotFound},
		{"market_not_found", ErrMarketNotFound, http.StatusNotFound},
		{"trade_not_found", ErrTradeNotFound, http.StatusNotFound},
		{"withdraw_not_found", ErrWithdrawNotFound, http.StatusNotFound},
		{"deposit_not_found", ErrDepositNotFound, http.StatusNotFound},

		// 409 Conflict
		{"duplicate_order", ErrDuplicateOrder, http.StatusConflict},

		// 429 Too Many Requests
		{"rate_limit_exceeded", ErrRateLimitExceeded, http.StatusTooManyRequests},

		// 500 Internal Server Error
		{"internal_error", ErrInternalError, http.StatusInternalServerError},

		// 501 Not Implemented
		{"not_implemented", ErrNotImplemented, http.StatusNotImplemented},

		// 502 Bad Gateway
		{"upstream_error", ErrUpstreamError, http.StatusBadGateway},

		// 503 Service Unavailable
		{"service_unavailable", ErrServiceUnavailable, http.StatusServiceUnavailable},

		// 504 Gateway Timeout
		{"timeout", ErrTimeout, http.StatusGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantStatus, tt.err.HTTPStatus)
		})
	}
}

func TestBizError_ErrorInterface(t *testing.T) {
	// Verify BizError implements error interface
	var err error = ErrInternalError
	assert.NotNil(t, err)
	assert.Equal(t, "INTERNAL_ERROR", err.Error())
}
