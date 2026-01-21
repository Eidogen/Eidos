package middleware

import (
	"context"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// MetricsUnaryServerInterceptor 指标收集拦截器
func MetricsUnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 记录正在处理的请求
		metrics.RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Inc()
		defer metrics.RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Dec()

		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := "OK"
		if err != nil {
			st, _ := status.FromError(err)
			code = st.Code().String()
		}

		// 记录请求指标
		metrics.RecordRequest(serviceName, info.FullMethod, code, duration.Seconds())

		return resp, err
	}
}

// MetricsStreamServerInterceptor 流式指标收集拦截器
func MetricsStreamServerInterceptor(serviceName string) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 记录正在处理的流
		metrics.RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Inc()
		defer metrics.RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Dec()

		start := time.Now()

		err := handler(srv, ss)

		duration := time.Since(start)
		code := "OK"
		if err != nil {
			st, _ := status.FromError(err)
			code = st.Code().String()
		}

		// 记录请求指标
		metrics.RecordRequest(serviceName+"_stream", info.FullMethod, code, duration.Seconds())

		return err
	}
}

// MetricsUnaryClientInterceptor 客户端指标收集拦截器
func MetricsUnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		err := invoker(ctx, method, req, reply, cc, opts...)

		duration := time.Since(start)
		code := "OK"
		if err != nil {
			st, _ := status.FromError(err)
			code = st.Code().String()
		}

		// 记录外部服务请求指标
		metrics.RecordExternalRequest(serviceName, method, code, duration.Seconds())

		return err
	}
}

// MetricsStreamClientInterceptor 客户端流式指标收集拦截器
func MetricsStreamClientInterceptor(serviceName string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()

		stream, err := streamer(ctx, desc, cc, method, opts...)

		duration := time.Since(start)
		code := "OK"
		if err != nil {
			st, _ := status.FromError(err)
			code = st.Code().String()
		}

		// 记录流建立的指标
		metrics.RecordExternalRequest(serviceName+"_stream", method, code, duration.Seconds())

		return stream, err
	}
}
