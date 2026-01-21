package middleware

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Validator 验证器接口
type Validator interface {
	Validate() error
}

// ValidatorFunc 验证函数类型
type ValidatorFunc func(interface{}) error

// ValidationError 验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors 验证错误列表
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// ValidatorUnaryServerInterceptor 验证拦截器
func ValidatorUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if v, ok := req.(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}
		return handler(ctx, req)
	}
}

// ValidatorStreamServerInterceptor 流式验证拦截器
func ValidatorStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 对于流式请求，我们包装 stream 来验证每条消息
		return handler(srv, &validatingServerStream{ServerStream: ss})
	}
}

// validatingServerStream 验证服务器流
type validatingServerStream struct {
	grpc.ServerStream
}

func (s *validatingServerStream) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}

	if v, ok := m.(Validator); ok {
		if err := v.Validate(); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	return nil
}

// CustomValidatorUnaryServerInterceptor 自定义验证拦截器
func CustomValidatorUnaryServerInterceptor(validators map[string]ValidatorFunc) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 先检查自定义验证器
		if validator, ok := validators[info.FullMethod]; ok {
			if err := validator(req); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}

		// 再检查请求自带的验证方法
		if v, ok := req.(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
		}

		return handler(ctx, req)
	}
}

// SimpleValidator 简单验证器
type SimpleValidator struct {
	errors ValidationErrors
}

// NewSimpleValidator 创建简单验证器
func NewSimpleValidator() *SimpleValidator {
	return &SimpleValidator{}
}

// Required 必填字段验证
func (v *SimpleValidator) Required(field string, value interface{}) *SimpleValidator {
	if isEmpty(value) {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "is required",
		})
	}
	return v
}

// MinLength 最小长度验证
func (v *SimpleValidator) MinLength(field string, value string, min int) *SimpleValidator {
	if len(value) < min {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at least %d characters", min),
		})
	}
	return v
}

// MaxLength 最大长度验证
func (v *SimpleValidator) MaxLength(field string, value string, max int) *SimpleValidator {
	if len(value) > max {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at most %d characters", max),
		})
	}
	return v
}

// Range 范围验证
func (v *SimpleValidator) Range(field string, value int64, min, max int64) *SimpleValidator {
	if value < min || value > max {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be between %d and %d", min, max),
		})
	}
	return v
}

// Min 最小值验证
func (v *SimpleValidator) Min(field string, value int64, min int64) *SimpleValidator {
	if value < min {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at least %d", min),
		})
	}
	return v
}

// Max 最大值验证
func (v *SimpleValidator) Max(field string, value int64, max int64) *SimpleValidator {
	if value > max {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("must be at most %d", max),
		})
	}
	return v
}

// Positive 正数验证
func (v *SimpleValidator) Positive(field string, value int64) *SimpleValidator {
	if value <= 0 {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be positive",
		})
	}
	return v
}

// NonNegative 非负数验证
func (v *SimpleValidator) NonNegative(field string, value int64) *SimpleValidator {
	if value < 0 {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be non-negative",
		})
	}
	return v
}

// OneOf 枚举验证
func (v *SimpleValidator) OneOf(field string, value string, allowed []string) *SimpleValidator {
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	v.errors = append(v.errors, &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")),
	})
	return v
}

// Email 邮箱验证 (简单验证)
func (v *SimpleValidator) Email(field string, value string) *SimpleValidator {
	if value != "" && !strings.Contains(value, "@") {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: "must be a valid email address",
		})
	}
	return v
}

// Custom 自定义验证
func (v *SimpleValidator) Custom(field string, valid bool, message string) *SimpleValidator {
	if !valid {
		v.errors = append(v.errors, &ValidationError{
			Field:   field,
			Message: message,
		})
	}
	return v
}

// Error 返回验证错误
func (v *SimpleValidator) Error() error {
	if len(v.errors) == 0 {
		return nil
	}
	return v.errors
}

// HasErrors 检查是否有错误
func (v *SimpleValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors 返回错误列表
func (v *SimpleValidator) Errors() ValidationErrors {
	return v.errors
}

// isEmpty 检查值是否为空
func isEmpty(value interface{}) bool {
	if value == nil {
		return true
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	}

	return false
}
