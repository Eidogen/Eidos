package middleware

import (
	"context"
	"runtime/debug"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RecoveryUnaryServerInterceptor panic 恢复拦截器
func RecoveryUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("grpc panic recovered",
					"panic", r,
					"method", info.FullMethod,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal error")
			}
		}()

		return handler(ctx, req)
	}
}

// RecoveryStreamServerInterceptor 流式 panic 恢复拦截器
func RecoveryStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("grpc stream panic recovered",
					"panic", r,
					"method", info.FullMethod,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "internal error")
			}
		}()

		return handler(srv, ss)
	}
}
