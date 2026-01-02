package middleware

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	TraceIDKey = "x-trace-id"
	WalletKey  = "x-wallet"
)

// UnaryServerInterceptor 统一的 gRPC 一元拦截器
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// 提取或生成 trace_id
		traceID := extractTraceID(ctx)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// 提取 wallet
		wallet := extractWallet(ctx)

		// 添加日志字段到 context
		ctx = logger.NewContext(ctx,
			zap.String("trace_id", traceID),
			zap.String("method", info.FullMethod),
			zap.String("wallet", wallet),
		)

		// 调用处理器
		resp, err := handler(ctx, req)

		// 记录日志
		duration := time.Since(start)
		fields := []zap.Field{
			zap.String("trace_id", traceID),
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}

		if err != nil {
			st, _ := status.FromError(err)
			fields = append(fields, zap.String("code", st.Code().String()))
			fields = append(fields, zap.String("error", st.Message()))
			logger.Error("grpc request failed", fields...)
		} else {
			logger.Info("grpc request completed", fields...)
		}

		return resp, err
	}
}

// StreamServerInterceptor 统一的 gRPC 流拦截器
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		ctx := ss.Context()

		traceID := extractTraceID(ctx)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		logger.Info("grpc stream started",
			zap.String("trace_id", traceID),
			zap.String("method", info.FullMethod),
		)

		err := handler(srv, ss)

		duration := time.Since(start)
		if err != nil {
			logger.Error("grpc stream failed",
				zap.String("trace_id", traceID),
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			logger.Info("grpc stream completed",
				zap.String("trace_id", traceID),
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
			)
		}

		return err
	}
}

// UnaryClientInterceptor 客户端一元拦截器 (传递 trace_id)
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 传递 trace_id
		traceID := extractTraceID(ctx)
		if traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, TraceIDKey, traceID)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func extractTraceID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(TraceIDKey)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func extractWallet(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(WalletKey)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}
