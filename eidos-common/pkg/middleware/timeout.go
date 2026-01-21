package middleware

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TimeoutConfig 超时配置
type TimeoutConfig struct {
	// 默认超时时间
	DefaultTimeout time.Duration
	// 方法级别超时配置
	MethodTimeouts map[string]time.Duration
}

// DefaultTimeoutConfig 默认超时配置
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
		MethodTimeouts: make(map[string]time.Duration),
	}
}

// TimeoutUnaryServerInterceptor 超时拦截器
func TimeoutUnaryServerInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
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
			logger.WithContext(ctx).Warn("request timeout",
				zap.String("method", info.FullMethod),
				zap.Duration("timeout", timeout),
			)
			return nil, status.Error(codes.DeadlineExceeded, "request timeout")
		}
	}
}

// ConfigurableTimeoutUnaryServerInterceptor 可配置的超时拦截器
func ConfigurableTimeoutUnaryServerInterceptor(cfg *TimeoutConfig) grpc.UnaryServerInterceptor {
	if cfg == nil {
		cfg = DefaultTimeoutConfig()
	}

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 获取方法级别超时或默认超时
		timeout := cfg.DefaultTimeout
		if methodTimeout, ok := cfg.MethodTimeouts[info.FullMethod]; ok {
			timeout = methodTimeout
		}

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
			logger.WithContext(ctx).Warn("request timeout",
				zap.String("method", info.FullMethod),
				zap.Duration("timeout", timeout),
			)
			return nil, status.Error(codes.DeadlineExceeded, "request timeout")
		}
	}
}

// TimeoutStreamServerInterceptor 流式超时拦截器
func TimeoutStreamServerInterceptor(timeout time.Duration) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx, cancel := context.WithTimeout(ss.Context(), timeout)
		defer cancel()

		done := make(chan error, 1)

		go func() {
			done <- handler(srv, &timeoutServerStream{
				ServerStream: ss,
				ctx:          ctx,
			})
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			logger.Info("stream timeout",
				zap.String("method", info.FullMethod),
				zap.Duration("timeout", timeout),
			)
			return status.Error(codes.DeadlineExceeded, "stream timeout")
		}
	}
}

// timeoutServerStream 带超时的服务器流
type timeoutServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *timeoutServerStream) Context() context.Context {
	return s.ctx
}

// ClientTimeoutUnaryInterceptor 客户端超时拦截器
func ClientTimeoutUnaryInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 如果 context 已经有超时，使用较短的那个
		if deadline, ok := ctx.Deadline(); ok {
			if time.Until(deadline) < timeout {
				return invoker(ctx, method, req, reply, cc, opts...)
			}
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// ClientTimeoutStreamInterceptor 客户端流式超时拦截器
func ClientTimeoutStreamInterceptor(timeout time.Duration) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// 如果 context 已经有超时，使用较短的那个
		if deadline, ok := ctx.Deadline(); ok {
			if time.Until(deadline) < timeout {
				return streamer(ctx, desc, cc, method, opts...)
			}
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		// 注意：流式调用不能在这里 defer cancel()
		// 因为 stream 可能会持续使用这个 context
		_ = cancel // 由调用者管理取消

		return streamer(ctx, desc, cc, method, opts...)
	}
}
