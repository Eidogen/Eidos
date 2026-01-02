package errors

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error 业务错误
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (cause: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// New 创建新错误
func New(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 包装错误
func Wrap(err *Error, cause error) *Error {
	return &Error{
		Code:    err.Code,
		Message: err.Message,
		Cause:   cause,
	}
}

// Wrapf 包装错误并添加信息
func Wrapf(err *Error, format string, args ...interface{}) *Error {
	return &Error{
		Code:    err.Code,
		Message: fmt.Sprintf("%s: %s", err.Message, fmt.Sprintf(format, args...)),
		Cause:   err.Cause,
	}
}

// 通用错误码
var (
	ErrInternal           = New("INTERNAL_ERROR", "内部错误")
	ErrInvalidRequest     = New("INVALID_REQUEST", "请求参数无效")
	ErrUnauthorized       = New("UNAUTHORIZED", "未授权")
	ErrForbidden          = New("FORBIDDEN", "禁止访问")
	ErrNotFound           = New("NOT_FOUND", "资源不存在")
	ErrConflict           = New("CONFLICT", "资源冲突")
	ErrRateLimited        = New("RATE_LIMITED", "请求过于频繁")
	ErrServiceUnavailable = New("SERVICE_UNAVAILABLE", "服务不可用")
)

// 业务错误码
var (
	// 签名相关
	ErrInvalidSignature = New("INVALID_SIGNATURE", "签名无效")
	ErrSignatureExpired = New("SIGNATURE_EXPIRED", "签名已过期")
	ErrInvalidNonce     = New("INVALID_NONCE", "Nonce 无效或已使用")

	// 余额相关
	ErrInsufficientBalance = New("INSUFFICIENT_BALANCE", "余额不足")
	ErrBalanceFrozen       = New("BALANCE_FROZEN", "余额已冻结")

	// 订单相关
	ErrOrderNotFound      = New("ORDER_NOT_FOUND", "订单不存在")
	ErrOrderCancelled     = New("ORDER_CANCELLED", "订单已取消")
	ErrOrderFilled        = New("ORDER_FILLED", "订单已成交")
	ErrInvalidOrderStatus = New("INVALID_ORDER_STATUS", "订单状态无效")
	ErrSelfTrade          = New("SELF_TRADE", "不能与自己成交")

	// 市场相关
	ErrMarketNotFound   = New("MARKET_NOT_FOUND", "交易对不存在")
	ErrMarketClosed     = New("MARKET_CLOSED", "交易对已关闭")
	ErrInvalidPrice     = New("INVALID_PRICE", "价格无效")
	ErrInvalidAmount    = New("INVALID_AMOUNT", "数量无效")
	ErrBelowMinAmount   = New("BELOW_MIN_AMOUNT", "低于最小下单量")
	ErrBelowMinNotional = New("BELOW_MIN_NOTIONAL", "低于最小名义金额")

	// 风控相关
	ErrRiskRejected = New("RISK_REJECTED", "风控拦截")
	ErrBlacklisted  = New("BLACKLISTED", "账户已被限制")
	ErrExceedLimit  = New("EXCEED_LIMIT", "超过限额")

	// 提现相关
	ErrWithdrawNotFound = New("WITHDRAW_NOT_FOUND", "提现记录不存在")
	ErrWithdrawDisabled = New("WITHDRAW_DISABLED", "提现已关闭")
	ErrInvalidAddress   = New("INVALID_ADDRESS", "地址无效")
)

// ToGRPCError 转换为 gRPC 错误
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	bizErr, ok := err.(*Error)
	if !ok {
		return status.Error(codes.Internal, err.Error())
	}

	var code codes.Code
	switch bizErr.Code {
	case ErrInvalidRequest.Code, ErrInvalidSignature.Code, ErrInvalidNonce.Code,
		ErrInvalidPrice.Code, ErrInvalidAmount.Code, ErrInvalidAddress.Code:
		code = codes.InvalidArgument
	case ErrUnauthorized.Code, ErrSignatureExpired.Code:
		code = codes.Unauthenticated
	case ErrForbidden.Code, ErrBlacklisted.Code, ErrRiskRejected.Code:
		code = codes.PermissionDenied
	case ErrNotFound.Code, ErrOrderNotFound.Code, ErrMarketNotFound.Code, ErrWithdrawNotFound.Code:
		code = codes.NotFound
	case ErrConflict.Code, ErrSelfTrade.Code:
		code = codes.AlreadyExists
	case ErrRateLimited.Code, ErrExceedLimit.Code:
		code = codes.ResourceExhausted
	case ErrInsufficientBalance.Code, ErrBelowMinAmount.Code, ErrBelowMinNotional.Code:
		code = codes.FailedPrecondition
	case ErrServiceUnavailable.Code, ErrMarketClosed.Code, ErrWithdrawDisabled.Code:
		code = codes.Unavailable
	default:
		code = codes.Internal
	}

	return status.Error(code, bizErr.Error())
}

// FromGRPCError 从 gRPC 错误恢复
func FromGRPCError(err error) *Error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return Wrap(ErrInternal, err)
	}

	return New(st.Code().String(), st.Message())
}

// Is 判断错误类型
func Is(err error, target *Error) bool {
	if err == nil || target == nil {
		return false
	}
	bizErr, ok := err.(*Error)
	if !ok {
		return false
	}
	return bizErr.Code == target.Code
}
