package metrics

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor 返回 gRPC 一元服务端拦截器
// 用于收集请求指标
func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// 增加正在处理的请求数
		RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Inc()
		defer RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Dec()

		// 开始计时
		timer := NewTimer()

		// 调用实际处理程序
		resp, err := handler(ctx, req)

		// 获取状态码
		code := status.Code(err).String()

		// 记录指标
		duration := timer.ObserveSeconds()
		RecordRequest(serviceName, info.FullMethod, code, duration)

		return resp, err
	}
}

// StreamServerInterceptor 返回 gRPC 流式服务端拦截器
func StreamServerInterceptor(serviceName string) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// 增加正在处理的请求数
		RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Inc()
		defer RequestsInFlight.WithLabelValues(serviceName, info.FullMethod).Dec()

		// 开始计时
		timer := NewTimer()

		// 调用实际处理程序
		err := handler(srv, ss)

		// 获取状态码
		code := status.Code(err).String()

		// 记录指标
		duration := timer.ObserveSeconds()
		RecordRequest(serviceName, info.FullMethod, code, duration)

		return err
	}
}

// UnaryClientInterceptor 返回 gRPC 一元客户端拦截器
// 用于收集对外部服务的请求指标
func UnaryClientInterceptor(clientName string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 开始计时
		timer := NewTimer()

		// 调用实际处理程序
		err := invoker(ctx, method, req, reply, cc, opts...)

		// 获取状态码
		code := status.Code(err).String()

		// 记录指标
		duration := timer.ObserveSeconds()
		RecordExternalRequest(clientName, method, code, duration)

		return err
	}
}

// StreamClientInterceptor 返回 gRPC 流式客户端拦截器
func StreamClientInterceptor(clientName string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// 开始计时
		start := time.Now()

		// 调用实际处理程序
		stream, err := streamer(ctx, desc, cc, method, opts...)

		// 获取状态码
		code := status.Code(err).String()

		// 记录指标
		duration := time.Since(start).Seconds()
		RecordExternalRequest(clientName, method, code, duration)

		return stream, err
	}
}
