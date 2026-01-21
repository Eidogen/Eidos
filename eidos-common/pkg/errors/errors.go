package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error 业务错误
type Error struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	HTTPStatus int               `json:"-"`
	GRPCCode   codes.Code        `json:"-"`
	Cause      error             `json:"-"`
	Details    map[string]string `json:"details,omitempty"`
	Stack      string            `json:"-"`
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

// Is 实现 errors.Is 接口
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// WithDetails 添加详情
func (e *Error) WithDetails(details map[string]string) *Error {
	newErr := e.Copy()
	if newErr.Details == nil {
		newErr.Details = make(map[string]string)
	}
	for k, v := range details {
		newErr.Details[k] = v
	}
	return newErr
}

// WithDetail 添加单个详情
func (e *Error) WithDetail(key, value string) *Error {
	return e.WithDetails(map[string]string{key: value})
}

// WithMessage 替换错误消息
func (e *Error) WithMessage(message string) *Error {
	newErr := e.Copy()
	newErr.Message = message
	return newErr
}

// WithMessagef 格式化替换错误消息
func (e *Error) WithMessagef(format string, args ...interface{}) *Error {
	return e.WithMessage(fmt.Sprintf(format, args...))
}

// Copy 复制错误
func (e *Error) Copy() *Error {
	newErr := &Error{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		GRPCCode:   e.GRPCCode,
		Cause:      e.Cause,
		Stack:      e.Stack,
	}
	if e.Details != nil {
		newErr.Details = make(map[string]string)
		for k, v := range e.Details {
			newErr.Details[k] = v
		}
	}
	return newErr
}

// JSON 返回 JSON 格式
func (e *Error) JSON() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// MarshalJSON 实现 json.Marshaler
func (e *Error) MarshalJSON() ([]byte, error) {
	type Alias Error
	return json.Marshal(&struct {
		*Alias
		Error string `json:"error,omitempty"`
	}{
		Alias: (*Alias)(e),
		Error: e.Error(),
	})
}

// New 创建新错误
func New(code, message string) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		HTTPStatus: http.StatusInternalServerError,
		GRPCCode:   codes.Internal,
	}
}

// NewWithStatus 创建带状态码的错误
func NewWithStatus(code, message string, httpStatus int, grpcCode codes.Code) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		GRPCCode:   grpcCode,
	}
}

// Wrap 包装错误
func Wrap(err *Error, cause error) *Error {
	newErr := err.Copy()
	newErr.Cause = cause
	newErr.Stack = getStack()
	return newErr
}

// Wrapf 包装错误并添加信息
func Wrapf(err *Error, format string, args ...interface{}) *Error {
	newErr := err.Copy()
	newErr.Message = fmt.Sprintf("%s: %s", err.Message, fmt.Sprintf(format, args...))
	newErr.Stack = getStack()
	return newErr
}

// WrapWithCause 包装错误并添加原因和信息
func WrapWithCause(err *Error, cause error, format string, args ...interface{}) *Error {
	newErr := err.Copy()
	newErr.Message = fmt.Sprintf("%s: %s", err.Message, fmt.Sprintf(format, args...))
	newErr.Cause = cause
	newErr.Stack = getStack()
	return newErr
}

// getStack 获取调用栈
func getStack() string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var builder strings.Builder
	for {
		frame, more := frames.Next()
		builder.WriteString(fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}
	return builder.String()
}

// FromError 从标准错误转换
func FromError(err error) *Error {
	if err == nil {
		return nil
	}

	// 已经是 Error 类型
	var bizErr *Error
	if errors.As(err, &bizErr) {
		return bizErr
	}

	// 包装为内部错误
	return Wrap(ErrInternal, err)
}

// 通用错误码
var (
	ErrInternal           = NewWithStatus("INTERNAL_ERROR", "内部错误", http.StatusInternalServerError, codes.Internal)
	ErrInvalidRequest     = NewWithStatus("INVALID_REQUEST", "请求参数无效", http.StatusBadRequest, codes.InvalidArgument)
	ErrUnauthorized       = NewWithStatus("UNAUTHORIZED", "未授权", http.StatusUnauthorized, codes.Unauthenticated)
	ErrForbidden          = NewWithStatus("FORBIDDEN", "禁止访问", http.StatusForbidden, codes.PermissionDenied)
	ErrNotFound           = NewWithStatus("NOT_FOUND", "资源不存在", http.StatusNotFound, codes.NotFound)
	ErrConflict           = NewWithStatus("CONFLICT", "资源冲突", http.StatusConflict, codes.AlreadyExists)
	ErrRateLimited        = NewWithStatus("RATE_LIMITED", "请求过于频繁", http.StatusTooManyRequests, codes.ResourceExhausted)
	ErrServiceUnavailable = NewWithStatus("SERVICE_UNAVAILABLE", "服务不可用", http.StatusServiceUnavailable, codes.Unavailable)
	ErrTimeout            = NewWithStatus("TIMEOUT", "请求超时", http.StatusGatewayTimeout, codes.DeadlineExceeded)
	ErrCanceled           = NewWithStatus("CANCELED", "请求已取消", 499, codes.Canceled)
	ErrBadGateway         = NewWithStatus("BAD_GATEWAY", "网关错误", http.StatusBadGateway, codes.Unavailable)
	ErrPreconditionFailed = NewWithStatus("PRECONDITION_FAILED", "前置条件失败", http.StatusPreconditionFailed, codes.FailedPrecondition)
	ErrDataLoss           = NewWithStatus("DATA_LOSS", "数据丢失", http.StatusInternalServerError, codes.DataLoss)
)

// 业务错误码
var (
	// 签名相关
	ErrInvalidSignature = NewWithStatus("INVALID_SIGNATURE", "签名无效", http.StatusBadRequest, codes.InvalidArgument)
	ErrSignatureExpired = NewWithStatus("SIGNATURE_EXPIRED", "签名已过期", http.StatusBadRequest, codes.InvalidArgument)
	ErrInvalidNonce     = NewWithStatus("INVALID_NONCE", "Nonce 无效或已使用", http.StatusBadRequest, codes.InvalidArgument)

	// 余额相关
	ErrInsufficientBalance = NewWithStatus("INSUFFICIENT_BALANCE", "余额不足", http.StatusPaymentRequired, codes.FailedPrecondition)
	ErrBalanceFrozen       = NewWithStatus("BALANCE_FROZEN", "余额已冻结", http.StatusForbidden, codes.FailedPrecondition)

	// 订单相关
	ErrOrderNotFound      = NewWithStatus("ORDER_NOT_FOUND", "订单不存在", http.StatusNotFound, codes.NotFound)
	ErrOrderCancelled     = NewWithStatus("ORDER_CANCELLED", "订单已取消", http.StatusBadRequest, codes.FailedPrecondition)
	ErrOrderFilled        = NewWithStatus("ORDER_FILLED", "订单已成交", http.StatusBadRequest, codes.FailedPrecondition)
	ErrInvalidOrderStatus = NewWithStatus("INVALID_ORDER_STATUS", "订单状态无效", http.StatusBadRequest, codes.FailedPrecondition)
	ErrSelfTrade          = NewWithStatus("SELF_TRADE", "不能与自己成交", http.StatusBadRequest, codes.InvalidArgument)

	// 市场相关
	ErrMarketNotFound   = NewWithStatus("MARKET_NOT_FOUND", "交易对不存在", http.StatusNotFound, codes.NotFound)
	ErrMarketClosed     = NewWithStatus("MARKET_CLOSED", "交易对已关闭", http.StatusServiceUnavailable, codes.Unavailable)
	ErrInvalidPrice     = NewWithStatus("INVALID_PRICE", "价格无效", http.StatusBadRequest, codes.InvalidArgument)
	ErrInvalidAmount    = NewWithStatus("INVALID_AMOUNT", "数量无效", http.StatusBadRequest, codes.InvalidArgument)
	ErrBelowMinAmount   = NewWithStatus("BELOW_MIN_AMOUNT", "低于最小下单量", http.StatusBadRequest, codes.InvalidArgument)
	ErrBelowMinNotional = NewWithStatus("BELOW_MIN_NOTIONAL", "低于最小名义金额", http.StatusBadRequest, codes.InvalidArgument)

	// 风控相关
	ErrRiskRejected = NewWithStatus("RISK_REJECTED", "风控拦截", http.StatusForbidden, codes.PermissionDenied)
	ErrBlacklisted  = NewWithStatus("BLACKLISTED", "账户已被限制", http.StatusForbidden, codes.PermissionDenied)
	ErrExceedLimit  = NewWithStatus("EXCEED_LIMIT", "超过限额", http.StatusBadRequest, codes.ResourceExhausted)

	// 提现相关
	ErrWithdrawNotFound = NewWithStatus("WITHDRAW_NOT_FOUND", "提现记录不存在", http.StatusNotFound, codes.NotFound)
	ErrWithdrawDisabled = NewWithStatus("WITHDRAW_DISABLED", "提现已关闭", http.StatusServiceUnavailable, codes.Unavailable)
	ErrInvalidAddress   = NewWithStatus("INVALID_ADDRESS", "地址无效", http.StatusBadRequest, codes.InvalidArgument)

	// 数据库相关
	ErrDuplicateKey   = NewWithStatus("DUPLICATE_KEY", "数据已存在", http.StatusConflict, codes.AlreadyExists)
	ErrDBConnection   = NewWithStatus("DB_CONNECTION_ERROR", "数据库连接失败", http.StatusInternalServerError, codes.Internal)
	ErrDBTimeout      = NewWithStatus("DB_TIMEOUT", "数据库操作超时", http.StatusGatewayTimeout, codes.DeadlineExceeded)
	ErrDBTransaction  = NewWithStatus("DB_TRANSACTION_ERROR", "数据库事务失败", http.StatusInternalServerError, codes.Internal)

	// 缓存相关
	ErrCacheMiss      = NewWithStatus("CACHE_MISS", "缓存未命中", http.StatusNotFound, codes.NotFound)
	ErrCacheConnection = NewWithStatus("CACHE_CONNECTION_ERROR", "缓存连接失败", http.StatusInternalServerError, codes.Internal)

	// 消息队列相关
	ErrMQConnection   = NewWithStatus("MQ_CONNECTION_ERROR", "消息队列连接失败", http.StatusInternalServerError, codes.Internal)
	ErrMQPublish      = NewWithStatus("MQ_PUBLISH_ERROR", "消息发布失败", http.StatusInternalServerError, codes.Internal)
	ErrMQConsume      = NewWithStatus("MQ_CONSUME_ERROR", "消息消费失败", http.StatusInternalServerError, codes.Internal)
)

// ToGRPCError 转换为 gRPC 错误
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	var bizErr *Error
	if errors.As(err, &bizErr) {
		return status.Error(bizErr.GRPCCode, bizErr.Error())
	}

	return status.Error(codes.Internal, err.Error())
}

// ToHTTPStatus 获取 HTTP 状态码
func ToHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	var bizErr *Error
	if errors.As(err, &bizErr) {
		if bizErr.HTTPStatus != 0 {
			return bizErr.HTTPStatus
		}
	}

	return http.StatusInternalServerError
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

	// 尝试根据 gRPC 状态码匹配已知错误
	return &Error{
		Code:       st.Code().String(),
		Message:    st.Message(),
		HTTPStatus: grpcCodeToHTTP(st.Code()),
		GRPCCode:   st.Code(),
	}
}

// grpcCodeToHTTP gRPC 状态码转 HTTP 状态码
func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// Is 判断错误类型
func Is(err error, target *Error) bool {
	if err == nil || target == nil {
		return false
	}
	return errors.Is(err, target)
}

// As 提取错误类型
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// GetCode 获取错误码
func GetCode(err error) string {
	if err == nil {
		return ""
	}
	var bizErr *Error
	if errors.As(err, &bizErr) {
		return bizErr.Code
	}
	return "UNKNOWN"
}

// GetMessage 获取错误消息
func GetMessage(err error) string {
	if err == nil {
		return ""
	}
	var bizErr *Error
	if errors.As(err, &bizErr) {
		return bizErr.Message
	}
	return err.Error()
}

// IsNotFound 判断是否为未找到错误
func IsNotFound(err error) bool {
	return Is(err, ErrNotFound) || Is(err, ErrOrderNotFound) || Is(err, ErrMarketNotFound) || Is(err, ErrWithdrawNotFound)
}

// IsUnauthorized 判断是否为未授权错误
func IsUnauthorized(err error) bool {
	return Is(err, ErrUnauthorized) || Is(err, ErrSignatureExpired)
}

// IsForbidden 判断是否为禁止访问错误
func IsForbidden(err error) bool {
	return Is(err, ErrForbidden) || Is(err, ErrBlacklisted) || Is(err, ErrRiskRejected)
}

// IsRateLimited 判断是否为限流错误
func IsRateLimited(err error) bool {
	return Is(err, ErrRateLimited) || Is(err, ErrExceedLimit)
}

// IsInvalidArgument 判断是否为参数错误
func IsInvalidArgument(err error) bool {
	var bizErr *Error
	if !errors.As(err, &bizErr) {
		return false
	}
	return bizErr.GRPCCode == codes.InvalidArgument
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var bizErr *Error
	if errors.As(err, &bizErr) {
		switch bizErr.GRPCCode {
		case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.DeadlineExceeded:
			return true
		}
	}
	return false
}

// ErrorRegistry 错误码注册表
type ErrorRegistry struct {
	errors map[string]*Error
}

// NewErrorRegistry 创建错误码注册表
func NewErrorRegistry() *ErrorRegistry {
	return &ErrorRegistry{
		errors: make(map[string]*Error),
	}
}

// Register 注册错误码
func (r *ErrorRegistry) Register(err *Error) {
	r.errors[err.Code] = err
}

// Get 获取错误码
func (r *ErrorRegistry) Get(code string) *Error {
	return r.errors[code]
}

// Has 检查错误码是否存在
func (r *ErrorRegistry) Has(code string) bool {
	_, ok := r.errors[code]
	return ok
}

// List 列出所有错误码
func (r *ErrorRegistry) List() []*Error {
	result := make([]*Error, 0, len(r.errors))
	for _, err := range r.errors {
		result = append(result, err)
	}
	return result
}
