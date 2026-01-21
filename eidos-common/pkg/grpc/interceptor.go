package grpc

import (
	"context"
	"log/slog"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/circuitbreaker"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/errors"
	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// TraceIDKey 追踪 ID 键
	TraceIDKey = "x-trace-id"
	// RequestIDKey 请求 ID 键
	RequestIDKey = "x-request-id"
	// UserIDKey 用户 ID 键
	UserIDKey = "x-user-id"
	// WalletKey 钱包地址键
	WalletKey = "x-wallet"
)

// propagateTraceID 传递追踪 ID
func propagateTraceID(ctx context.Context) context.Context {
	traceID := extractFromContext(ctx, TraceIDKey)
	if traceID != "" {
		return metadata.AppendToOutgoingContext(ctx, TraceIDKey, traceID)
	}
	return ctx
}

// extractFromContext 从上下文提取元数据
func extractFromContext(ctx context.Context, key string) string {
	// 先从 incoming 获取
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}
	// 再从 outgoing 获取
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if values := md.Get(key); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// UnaryServerTraceInterceptor 服务端追踪拦截器
func UnaryServerTraceInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 提取或生成 trace_id
		traceID := extractFromContext(ctx, TraceIDKey)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// 提取其他元数据
		wallet := extractFromContext(ctx, WalletKey)
		userID := extractFromContext(ctx, UserIDKey)

		// 添加日志字段到 context
		ctx = logger.NewContext(ctx,
			slog.String("trace_id", traceID),
			slog.String("method", info.FullMethod),
			slog.String("wallet", wallet),
			slog.String("user_id", userID),
		)

		return handler(ctx, req)
	}
}

// StreamServerTraceInterceptor 流式服务端追踪拦截器
func StreamServerTraceInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		traceID := extractFromContext(ctx, TraceIDKey)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		logger.Info("grpc stream started",
			"trace_id", traceID,
			"method", info.FullMethod,
		)

		return handler(srv, &wrappedServerStream{
			ServerStream: ss,
			ctx:          logger.NewContext(ctx, slog.String("trace_id", traceID)),
		})
	}
}

// wrappedServerStream 包装的服务器流
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// UnaryServerErrorInterceptor 错误处理拦截器
func UnaryServerErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			// 将业务错误转换为 gRPC 错误
			return resp, errors.ToGRPCError(err)
		}
		return resp, nil
	}
}

// UnaryClientErrorInterceptor 客户端错误处理拦截器
func UnaryClientErrorInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			// 将 gRPC 错误转换为业务错误
			return errors.FromGRPCError(err)
		}
		return nil
	}
}

// TimeoutInterceptor 超时拦截器
func TimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		done := make(chan struct{})
		var resp interface{}
		var err error

		go func() {
			resp, err = handler(ctx, req)
			close(done)
		}()

		select {
		case <-done:
			return resp, err
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "request timeout")
		}
	}
}

// CircuitBreakerInterceptor 熔断器拦截器
func CircuitBreakerInterceptor(registry *circuitbreaker.BreakerRegistry) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		cb := registry.Get(method)

		return cb.Execute(func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		})
	}
}

// RateLimitInterceptor 限流拦截器
type RateLimiter interface {
	Allow() bool
}

func RateLimitInterceptor(limiter RateLimiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return handler(ctx, req)
	}
}

// AuthInterceptor 认证拦截器
type Authenticator interface {
	Authenticate(ctx context.Context) (context.Context, error)
}

func AuthInterceptor(auth Authenticator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		newCtx, err := auth.Authenticate(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		return handler(newCtx, req)
	}
}

// ValidatorInterceptor 请求验证拦截器
type Validator interface {
	Validate() error
}

func ValidatorInterceptor() grpc.UnaryServerInterceptor {
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

// RetryInterceptor 重试拦截器
func RetryInterceptor(maxAttempts int, backoff time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var lastErr error

		for attempt := 0; attempt < maxAttempts; attempt++ {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}

			// 检查是否可重试
			st, ok := status.FromError(err)
			if !ok {
				return err
			}

			// 只对特定错误码重试
			if !isRetryableCode(st.Code()) {
				return err
			}

			lastErr = err

			if attempt < maxAttempts-1 {
				logger.Debug("retrying grpc call",
					"method", method,
					"attempt", attempt+1,
					"max_attempts", maxAttempts,
					"error", err,
				)

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff * time.Duration(attempt+1)):
				}
			}
		}

		return lastErr
	}
}

// isRetryableCode 判断是否可重试的错误码
func isRetryableCode(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// ChainUnaryServer 链式一元服务器拦截器
func ChainUnaryServer(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 构建链式处理器
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return chain(ctx, req)
	}
}

// ChainStreamServer 链式流服务器拦截器
func ChainStreamServer(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(srv interface{}, ss grpc.ServerStream) error {
				return interceptor(srv, ss, info, next)
			}
		}
		return chain(srv, ss)
	}
}

// ChainUnaryClient 链式一元客户端拦截器
func ChainUnaryClient(interceptors ...grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		chain := invoker
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			next := chain
			chain = func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
				return interceptor(ctx, method, req, reply, cc, next, opts...)
			}
		}
		return chain(ctx, method, req, reply, cc, opts...)
	}
}
